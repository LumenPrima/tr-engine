# IRC Client Overhaul Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add channel search, favorites, smart collapse with group unread counts, scanner mode with audio queue, and mobile responsive layout to `web/irc-radio-live.html`.

**Architecture:** All changes are within the single-file SPA. New CSS for mobile breakpoint and new UI components. New JS state for favorites, scan list, and audio queue. Existing SSE/API infrastructure unchanged. localStorage for persistence.

**Tech Stack:** Vanilla HTML/CSS/JS, CSS media queries, touch events, Web Audio API (existing `<audio>` element).

**Design doc:** `docs/plans/2026-02-24-irc-client-overhaul-design.md`

---

## Task 1: Channel Search Box

**Files:**
- Modify: `web/irc-radio-live.html:557-565` (HTML — channel tree panel)
- Modify: `web/irc-radio-live.html:82-92` (CSS — channel tree styles)
- Modify: `web/irc-radio-live.html:1256-1382` (JS — `renderTree()`)

**Step 1: Add search input HTML**

Insert a search input inside `.channel-tree`, between `.tree-header-row` and `#tree-scroll` (after line 561):

```html
<div class="tree-search">
  <input class="tree-search-input" id="tree-search" type="text"
         placeholder="Search channels..." spellcheck="false" autocomplete="off">
  <span class="tree-search-clear" id="tree-search-clear">✕</span>
</div>
```

**Step 2: Add search CSS**

Add after the `.tree-header-row` styles (around line 99):

```css
.tree-search {
  padding: 4px 8px;
  border-bottom: 1px solid var(--border);
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}
.tree-search-input {
  flex: 1;
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 3px;
  color: var(--text);
  font-family: inherit;
  font-size: 13px;
  padding: 3px 6px;
  outline: none;
}
.tree-search-input:focus { border-color: var(--cyan); }
.tree-search-input::placeholder { color: var(--text-faint); }
.tree-search-clear {
  cursor: pointer;
  color: var(--text-muted);
  font-size: 14px;
  visibility: hidden;
  user-select: none;
}
.tree-search-clear.visible { visibility: visible; }
```

**Step 3: Add search state and DOM refs**

Add to the STATE section (after line 702):

```javascript
// Search state
let treeSearchQuery = '';
const treeSearchInput = document.getElementById('tree-search');
const treeSearchClear = document.getElementById('tree-search-clear');
```

**Step 4: Add search event handlers**

Add after the DOM refs:

```javascript
treeSearchInput.addEventListener('input', () => {
  treeSearchQuery = treeSearchInput.value.trim().toLowerCase();
  treeSearchClear.classList.toggle('visible', treeSearchQuery.length > 0);
  renderTree();
});
treeSearchClear.addEventListener('click', () => {
  treeSearchInput.value = '';
  treeSearchQuery = '';
  treeSearchClear.classList.remove('visible');
  renderTree();
});
```

**Step 5: Modify `renderTree()` to support filtered mode**

In `renderTree()` (line 1256), after building the `groups` Map (line 1264), add search filtering:

```javascript
// If searching, show flat filtered results
if (treeSearchQuery) {
  treeScroll.innerHTML = '';
  const matches = [];
  for (const [name, ch] of channels) {
    if (name === STATUS_CHANNEL) continue;
    const searchable = (name + ' ' + (ch.alpha_tag || '') + ' ' + (ch.group || '')).toLowerCase();
    if (searchable.includes(treeSearchQuery)) matches.push(name);
  }
  matches.sort();
  for (const ch of matches) {
    treeScroll.appendChild(createChannelItem(ch));
  }
  if (matches.length === 0) {
    const empty = document.createElement('div');
    empty.className = 'tree-more';
    empty.textContent = 'No matching channels';
    treeScroll.appendChild(empty);
  }
  return; // skip grouped rendering
}
```

**Step 6: Test in browser**

Navigate to the live instance, verify:
- Search box appears below the header row
- Typing filters channels to a flat list
- Clear button resets to grouped view
- Empty search shows "No matching channels"

**Step 7: Commit**

```bash
git add web/irc-radio-live.html
git commit -m "feat(irc): add channel search box with instant filtering"
```

---

## Task 2: Favorites System

**Files:**
- Modify: `web/irc-radio-live.html` (CSS, HTML, JS throughout)

**Step 1: Add favorites state**

Add to the STATE section (after search state):

```javascript
// Favorites: Set of "system_id:tgid" keys
const favorites = new Set(JSON.parse(localStorage.getItem('irc_favorites') || '[]'));

function saveFavorites() {
  localStorage.setItem('irc_favorites', JSON.stringify([...favorites]));
}

function isFavorite(chanName) {
  const ch = channels.get(chanName);
  if (!ch) return false;
  return favorites.has(`${ch.system_id}:${ch.tgid}`);
}

function toggleFavorite(chanName) {
  const ch = channels.get(chanName);
  if (!ch) return;
  const key = `${ch.system_id}:${ch.tgid}`;
  if (favorites.has(key)) favorites.delete(key);
  else favorites.add(key);
  saveFavorites();
  renderTree();
}
```

**Step 2: Add star CSS**

```css
.fav-star {
  margin-left: auto;
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.15s;
  color: var(--text-muted);
  font-size: 12px;
  padding: 0 2px;
  flex-shrink: 0;
}
.channel-item:hover .fav-star,
.fav-star.active { opacity: 1; }
.fav-star.active { color: var(--warning); }
```

**Step 3: Modify `createChannelItem()` to include star**

In `createChannelItem()` (line 1240), add the star element:

```javascript
function createChannelItem(ch) {
  const el = document.createElement('div');
  const unread = channelUnread.get(ch) || 0;
  el.className = 'channel-item' + (ch === activeChannel ? ' active' : '') + (unread > 0 && ch !== activeChannel ? ' has-activity' : '');
  el.innerHTML = `<span class="chan-prefix">#</span><span class="chan-label">${esc(ch.slice(1))}</span>`;
  if (unread > 0 && ch !== activeChannel) {
    el.innerHTML += `<span class="unread">${unread > 99 ? '99+' : unread}</span>`;
  }
  // Favorite star
  const star = document.createElement('span');
  star.className = 'fav-star' + (isFavorite(ch) ? ' active' : '');
  star.textContent = isFavorite(ch) ? '★' : '☆';
  star.addEventListener('click', (e) => {
    e.stopPropagation();
    toggleFavorite(ch);
  });
  el.appendChild(star);

  el.addEventListener('click', () => switchChannel(ch));
  return el;
}
```

**Step 4: Add Favorites section to `renderTree()`**

In `renderTree()`, after rendering the System/status section (after line 1284) and before the sorted groups loop, add:

```javascript
// Favorites section
const favChans = [...channels.keys()].filter(ch => ch !== STATUS_CHANNEL && isFavorite(ch));
if (favChans.length > 0) {
  const favSection = document.createElement('div');
  favSection.className = 'tree-section';
  const favLabel = document.createElement('div');
  favLabel.className = 'tree-section-label fav-label';
  favLabel.innerHTML = '★ Favorites';
  favSection.appendChild(favLabel);

  favChans.sort();
  for (const ch of favChans) {
    favSection.appendChild(createChannelItem(ch));
  }
  treeScroll.appendChild(favSection);
}
```

**Step 5: Add `/fav` command**

In the command handler (search for `/ops` command), add:

```javascript
case 'fav':
case 'favorite': {
  if (activeChannel === STATUS_CHANNEL) {
    serverMsg('Cannot favorite *status');
    break;
  }
  toggleFavorite(activeChannel);
  serverMsgTo(activeChannel, isFavorite(activeChannel)
    ? `Added ${activeChannel} to favorites`
    : `Removed ${activeChannel} from favorites`);
  break;
}
```

**Step 6: Test in browser**

- Stars appear on hover, clicking toggles favorite
- Favorites section appears at top of tree
- `/fav` command works in input bar
- Favorites persist across page reload

**Step 7: Commit**

```bash
git add web/irc-radio-live.html
git commit -m "feat(irc): add channel favorites with star toggle and /fav command"
```

---

## Task 3: Smart Collapse with Group Unread Counts

**Files:**
- Modify: `web/irc-radio-live.html` (JS — `renderTree()`, CSS for group badges)

**Step 1: Add group unread count CSS**

```css
.group-unread {
  background: var(--orange);
  color: var(--bg);
  font-size: 10px;
  font-weight: 600;
  padding: 0 5px;
  border-radius: 8px;
  margin-left: 6px;
  min-width: 16px;
  text-align: center;
  display: inline-block;
  line-height: 16px;
}
```

**Step 2: Modify `renderTree()` group rendering**

Replace the group header rendering logic. For each group, compute total unread and show it in the header. Change the collapse logic so groups with zero unread auto-collapse (unless manually expanded), and groups with unread auto-expand showing only active channels:

In the group loop (line 1289), replace the section label creation to include unread count:

```javascript
// Compute group-level unread total
const groupUnread = chans.reduce((sum, ch) => sum + (channelUnread.get(ch) || 0), 0);

// Build label with optional unread badge
const label = document.createElement('div');
label.className = 'tree-section-label';
const arrow = (isExpanded || (!allInactive)) ? '▾' : '▸';
let labelContent = `${arrow} ${groupLabelHtml(groupName)}`;
if (groupUnread > 0) {
  labelContent += `<span class="group-unread">${groupUnread > 99 ? '99+' : groupUnread}</span>`;
}
label.innerHTML = labelContent;
```

**Step 3: Adjust collapse defaults**

Change the auto-collapse logic so groups with zero unread AND no manual expand collapse by default, even if they have <= 3 channels. The current behavior shows small groups always expanded — change so only groups with activity or manual expand show channels:

- `allInactive && !isExpanded` → collapse (show header + "N more..." or nothing for small groups)
- `!allInactive` → auto-expand, show only active channels + "N more..."
- `isExpanded` → show all

This is already largely the existing behavior, but ensure the group unread badge is wired in everywhere.

**Step 4: Test in browser**

- Groups with active channels show unread count badge in header
- Quiet groups are collapsed with just the header visible
- Expanding a quiet group shows all channels
- Group unread count updates in real-time as events arrive

**Step 5: Commit**

```bash
git add web/irc-radio-live.html
git commit -m "feat(irc): add group-level unread counts and smart auto-collapse"
```

---

## Task 4: Scanner Mode — State and UI Controls

**Files:**
- Modify: `web/irc-radio-live.html` (CSS, HTML, JS)

**Step 1: Add scanner state**

Add to STATE section:

```javascript
// Scanner mode
const scanChannels = new Set(JSON.parse(localStorage.getItem('irc_scan_channels') || '[]'));
let scannerEnabled = localStorage.getItem('irc_scanner_enabled') === '1';
let scannerAutoSwitch = localStorage.getItem('irc_scanner_auto_switch') !== '0'; // default ON

function saveScanChannels() {
  localStorage.setItem('irc_scan_channels', JSON.stringify([...scanChannels]));
}

function isScanned(chanName) {
  const ch = channels.get(chanName);
  if (!ch) return false;
  return scanChannels.has(`${ch.system_id}:${ch.tgid}`);
}

function toggleScan(chanName) {
  const ch = channels.get(chanName);
  if (!ch) return;
  const key = `${ch.system_id}:${ch.tgid}`;
  if (scanChannels.has(key)) scanChannels.delete(key);
  else scanChannels.add(key);
  saveScanChannels();
  renderTree();
}
```

**Step 2: Add scanner CSS**

```css
.scan-icon {
  margin-left: 2px;
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.15s;
  color: var(--text-muted);
  font-size: 11px;
  flex-shrink: 0;
}
.channel-item:hover .scan-icon,
.scan-icon.active { opacity: 1; }
.scan-icon.active { color: var(--green); }

.scanner-btn {
  background: none;
  border: 1px solid var(--border);
  color: var(--text-muted);
  font-family: inherit;
  font-size: 12px;
  padding: 0 6px;
  cursor: pointer;
  border-radius: 3px;
  margin-left: 4px;
}
.scanner-btn.active {
  color: var(--green);
  border-color: var(--green);
}
.scanner-btn.active::before {
  content: '📡 ';
}

.scanner-queue-info {
  color: var(--text-muted);
  font-size: 12px;
  margin-left: 4px;
}
```

**Step 3: Add scan icon to channel items**

In `createChannelItem()`, add a scan toggle icon before the star:

```javascript
// Scan icon
const scanIcon = document.createElement('span');
scanIcon.className = 'scan-icon' + (isScanned(ch) ? ' active' : '');
scanIcon.textContent = isScanned(ch) ? '📡' : '🔇';
scanIcon.title = isScanned(ch) ? 'Remove from scanner' : 'Add to scanner';
scanIcon.addEventListener('click', (e) => {
  e.stopPropagation();
  toggleScan(ch);
});
el.appendChild(scanIcon);
```

**Step 4: Add scanner toggle to status bar**

In the status bar HTML (line 603-615), add a scanner button before the nerd button:

```html
<button class="scanner-btn" id="scanner-btn" title="Toggle scanner mode">scan</button>
<span class="scanner-queue-info" id="scanner-queue-info"></span>
```

**Step 5: Wire up scanner toggle**

```javascript
const scannerBtn = document.getElementById('scanner-btn');
const scannerQueueInfo = document.getElementById('scanner-queue-info');

function updateScannerUI() {
  scannerBtn.classList.toggle('active', scannerEnabled);
  scannerBtn.textContent = scannerEnabled ? 'scan' : 'scan';
}

scannerBtn.addEventListener('click', () => {
  scannerEnabled = !scannerEnabled;
  localStorage.setItem('irc_scanner_enabled', scannerEnabled ? '1' : '0');
  updateScannerUI();
  if (!scannerEnabled) {
    scannerQueue.length = 0;
    scannerQueueInfo.textContent = '';
  }
});

// Init on load
updateScannerUI();
```

**Step 6: Add `/scan` and `/scanner` commands**

```javascript
case 'scan': {
  if (activeChannel === STATUS_CHANNEL) {
    serverMsg('Cannot scan *status');
    break;
  }
  toggleScan(activeChannel);
  serverMsgTo(activeChannel, isScanned(activeChannel)
    ? `Added ${activeChannel} to scan list`
    : `Removed ${activeChannel} from scan list`);
  break;
}
case 'scanner': {
  scannerEnabled = !scannerEnabled;
  localStorage.setItem('irc_scanner_enabled', scannerEnabled ? '1' : '0');
  updateScannerUI();
  serverMsg(`Scanner ${scannerEnabled ? 'enabled' : 'disabled'}`);
  break;
}
```

**Step 7: Test in browser**

- Scan icon appears on channel hover, toggles scan membership
- Scanner button in status bar toggles on/off
- `/scan` and `/scanner` commands work
- State persists across reload

**Step 8: Commit**

```bash
git add web/irc-radio-live.html
git commit -m "feat(irc): add scanner mode UI — scan list, toggle, and /scan commands"
```

---

## Task 5: Scanner Mode — Audio Queue and Auto-Play

**Files:**
- Modify: `web/irc-radio-live.html` (JS — audio queue, `handleCallEnd` integration)

**Step 1: Add audio queue state**

```javascript
// Scanner audio queue
const scannerQueue = []; // Array of { callId, nick, chanName, emergency }
const SCANNER_MAX_QUEUE = 10;
```

**Step 2: Add `scannerEnqueue()` function**

```javascript
function scannerEnqueue(callId, nick, chanName, emergency) {
  if (!scannerEnabled) return;
  if (!isScanned(chanName)) return;

  // Emergency calls go to front of queue (after currently playing)
  if (emergency) {
    scannerQueue.splice(0, 0, { callId, nick, chanName, emergency });
  } else {
    // Overflow protection
    if (scannerQueue.length >= SCANNER_MAX_QUEUE) {
      const dropped = scannerQueue.length - SCANNER_MAX_QUEUE + 1;
      scannerQueue.splice(0, dropped);
      serverMsg(`Scanner: skipped ${dropped} transmission(s) — queue overflow`);
    }
    scannerQueue.push({ callId, nick, chanName, emergency });
  }
  updateScannerQueueUI();

  // If nothing is playing, start playback
  if (!scannerPlaying) scannerPlayNext();
}
```

**Step 3: Add `scannerPlayNext()` function**

```javascript
let scannerPlaying = false;

function scannerPlayNext() {
  if (scannerQueue.length === 0) {
    scannerPlaying = false;
    updateScannerQueueUI();
    return;
  }

  scannerPlaying = true;
  const item = scannerQueue.shift();
  updateScannerQueueUI();

  // Auto-switch channel if enabled
  if (scannerAutoSwitch && item.chanName !== activeChannel) {
    switchChannel(item.chanName);
  }

  // Find the message element for this call in the active channel
  let msgEl = null;
  if (item.chanName === activeChannel) {
    msgEl = messagesEl.querySelector(`[data-call-id="${item.callId}"]`);
  }

  // Play the call audio
  playCall(item.callId, item.nick, item.chanName, msgEl);
}

function updateScannerQueueUI() {
  if (!scannerEnabled || (scannerQueue.length === 0 && !scannerPlaying)) {
    scannerQueueInfo.textContent = '';
  } else {
    const total = scannerQueue.length + (scannerPlaying ? 1 : 0);
    scannerQueueInfo.textContent = total > 1 ? `(${total} queued)` : '';
  }
}
```

**Step 4: Wire scanner into `audioEl.ended` event**

Modify the existing `audioEl.addEventListener('ended', ...)` (line 2840) to chain scanner playback:

```javascript
audioEl.addEventListener('ended', () => {
  playerPlayBtn.textContent = '▶';
  playerProgressFill.style.width = '100%';
  if (currentPlayingMsgEl) currentPlayingMsgEl.classList.remove('now-playing');
  currentPlayingMsgEl = null;
  currentCallId = null;

  // Scanner: play next in queue
  if (scannerEnabled && scannerQueue.length > 0) {
    setTimeout(scannerPlayNext, 300); // small gap between transmissions
  } else {
    scannerPlaying = false;
    updateScannerQueueUI();
  }
});
```

**Step 5: Wire scanner into `handleCallEnd()`**

At the end of `handleCallEnd()` (around line 2280), after rendering messages, add scanner enqueue:

```javascript
// Scanner: enqueue for auto-play if this channel is scanned
if (call.audio_file_path || (srcList && srcList.length > 0)) {
  const nick = (srcList && srcList.length > 0)
    ? resolveNick(call.system_id, srcList[0].src, srcList[0].tag)
    : (call.tg_alpha_tag || `TG ${call.tgid}`);
  scannerEnqueue(call.call_id, nick, chanName, isEmergency);
}
```

**Step 6: Add skip/clear buttons to player bar**

Add a skip button next to the close button in the player bar HTML:

```html
<button class="player-btn player-skip" id="player-skip" title="Skip to next">⏭</button>
```

Wire it:

```javascript
document.getElementById('player-skip').addEventListener('click', () => {
  audioEl.pause();
  audioEl.currentTime = audioEl.duration || 0; // triggers 'ended'
  // The 'ended' event handler will call scannerPlayNext()
});
```

**Step 7: Test in browser**

- Add channels to scan list, enable scanner
- When calls arrive on scanned channels, audio auto-plays
- Queue indicator shows count
- Skip button advances to next
- Emergency calls jump to front of queue
- Queue overflow drops oldest with notice

**Step 8: Commit**

```bash
git add web/irc-radio-live.html
git commit -m "feat(irc): implement scanner audio queue with auto-play, skip, and emergency priority"
```

---

## Task 6: Mobile Responsive — Layout and CSS

**Files:**
- Modify: `web/irc-radio-live.html` (CSS, HTML)

**Step 1: Add mobile CSS media query**

Add a large media query block at the end of the `<style>` section (before `</style>`):

```css
/* ══════════════════════════════════════════════════════════════ */
/* MOBILE RESPONSIVE — below 768px                               */
/* ══════════════════════════════════════════════════════════════ */

@media (max-width: 768px) {
  /* Hide CRT effects on mobile for performance */
  body::before, body::after, .vignette-overlay { display: none; }

  /* Titlebar becomes mobile nav */
  .titlebar {
    height: 44px;
    padding: 0 8px;
    justify-content: space-between;
  }
  .titlebar .topic { display: none; }

  /* Hamburger button */
  .mobile-hamburger {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    font-size: 20px;
    cursor: pointer;
    color: var(--text);
    border: none;
    background: none;
  }

  /* Channel tree becomes slide-out drawer */
  .channel-tree {
    position: fixed;
    top: 0;
    left: 0;
    bottom: 0;
    width: 280px;
    z-index: 1000;
    transform: translateX(-100%);
    transition: transform 0.25s ease;
    box-shadow: none;
  }
  .channel-tree.open {
    transform: translateX(0);
    box-shadow: 4px 0 20px rgba(0,0,0,0.3);
  }

  /* Nicklist becomes slide-out drawer from right */
  .nicklist {
    position: fixed;
    top: 0;
    right: 0;
    bottom: 0;
    width: 240px;
    z-index: 1000;
    transform: translateX(100%);
    transition: transform 0.25s ease;
    box-shadow: none;
  }
  .nicklist.open {
    transform: translateX(0);
    box-shadow: -4px 0 20px rgba(0,0,0,0.3);
  }

  /* Drawer backdrop */
  .drawer-backdrop {
    display: none;
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.5);
    z-index: 999;
  }
  .drawer-backdrop.visible { display: block; }

  /* Messages take full width */
  .chat-area { flex: 1; min-width: 0; }
  .messages { font-size: 14px; }

  /* Channel bar simplification */
  .channel-bar {
    height: 36px;
    font-size: 13px;
  }

  /* Input area */
  .input-area { font-size: 14px; }
  .input-field { font-size: 14px; }

  /* Status bar */
  .statusbar { font-size: 11px; padding: 0 6px; }

  /* Nick width smaller on mobile */
  .nick { max-width: 80px !important; }

  /* Larger touch targets for audio play buttons */
  .voice-play {
    min-width: 44px;
    min-height: 44px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }

  /* Favorite stars and scan icons always visible (no hover on touch) */
  .fav-star, .scan-icon { opacity: 1; }
}

/* Desktop: hide mobile-only elements */
@media (min-width: 769px) {
  .mobile-hamburger { display: none; }
  .mobile-users-btn { display: none; }
  .drawer-backdrop { display: none !important; }
}
```

**Step 2: Add mobile HTML elements**

Add a hamburger button inside `.titlebar` (as first child):

```html
<button class="mobile-hamburger" id="mobile-hamburger" aria-label="Open channels">☰</button>
```

Add a users button inside `.titlebar` (as last child, before lag span):

```html
<button class="mobile-hamburger mobile-users-btn" id="mobile-users-btn" aria-label="Open users">👥</button>
```

Add a backdrop div after the `<div class="main">` opening:

```html
<div class="drawer-backdrop" id="drawer-backdrop"></div>
```

**Step 3: Test mobile layout**

Use Playwright to resize to 375x667 (iPhone SE) and verify layout.

**Step 4: Commit**

```bash
git add web/irc-radio-live.html
git commit -m "feat(irc): add mobile responsive CSS with drawer layout and touch-sized targets"
```

---

## Task 7: Mobile Responsive — Drawer JS and Touch Gestures

**Files:**
- Modify: `web/irc-radio-live.html` (JS)

**Step 1: Add drawer toggle JS**

```javascript
// ══════════════════════════════════════════════════════════════
// MOBILE DRAWERS
// ══════════════════════════════════════════════════════════════

const channelTree = document.querySelector('.channel-tree');
const nicklist = document.querySelector('.nicklist');
const drawerBackdrop = document.getElementById('drawer-backdrop');
const hamburgerBtn = document.getElementById('mobile-hamburger');
const usersBtn = document.getElementById('mobile-users-btn');

function openDrawer(which) {
  if (which === 'channels') {
    channelTree.classList.add('open');
    nicklist.classList.remove('open');
  } else {
    nicklist.classList.add('open');
    channelTree.classList.remove('open');
  }
  drawerBackdrop.classList.add('visible');
}

function closeDrawers() {
  channelTree.classList.remove('open');
  nicklist.classList.remove('open');
  drawerBackdrop.classList.remove('visible');
}

hamburgerBtn.addEventListener('click', () => openDrawer('channels'));
usersBtn.addEventListener('click', () => openDrawer('users'));
drawerBackdrop.addEventListener('click', closeDrawers);
```

**Step 2: Auto-close drawer on channel switch (mobile)**

In `switchChannel()` (line 1447), add at the end:

```javascript
// Close mobile drawer on channel switch
if (window.innerWidth <= 768) closeDrawers();
```

**Step 3: Add swipe gesture detection**

```javascript
// Swipe gestures for mobile drawers
let touchStartX = 0;
let touchStartY = 0;
const SWIPE_THRESHOLD = 50;
const EDGE_ZONE = 30; // pixels from screen edge

document.addEventListener('touchstart', (e) => {
  touchStartX = e.touches[0].clientX;
  touchStartY = e.touches[0].clientY;
}, { passive: true });

document.addEventListener('touchend', (e) => {
  const dx = e.changedTouches[0].clientX - touchStartX;
  const dy = e.changedTouches[0].clientY - touchStartY;

  // Only trigger on horizontal swipes (not vertical scroll)
  if (Math.abs(dx) < SWIPE_THRESHOLD || Math.abs(dy) > Math.abs(dx)) return;
  if (window.innerWidth > 768) return;

  if (dx > 0 && touchStartX < EDGE_ZONE) {
    // Swipe right from left edge — open channels
    openDrawer('channels');
  } else if (dx < 0 && touchStartX > window.innerWidth - EDGE_ZONE) {
    // Swipe left from right edge — open users
    openDrawer('users');
  } else if (dx < 0 && channelTree.classList.contains('open')) {
    closeDrawers();
  } else if (dx > 0 && nicklist.classList.contains('open')) {
    closeDrawers();
  }
}, { passive: true });
```

**Step 4: Test with Playwright**

Resize browser to 375x667, verify:
- Hamburger opens channel drawer
- Users button opens nicklist drawer
- Backdrop tap closes drawers
- Channel switch closes drawer

**Step 5: Commit**

```bash
git add web/irc-radio-live.html
git commit -m "feat(irc): add mobile drawer navigation with swipe gestures"
```

---

## Task 8: Mobile Message Format

**Files:**
- Modify: `web/irc-radio-live.html` (CSS, JS — `buildMsgEl()`)

**Step 1: Add mobile message CSS**

Inside the `@media (max-width: 768px)` block, add:

```css
/* Card-style messages for mobile */
.msg {
  flex-direction: column;
  padding: 6px 10px;
  gap: 2px;
}
.msg .ts {
  position: absolute;
  right: 10px;
  top: 6px;
  font-size: 11px;
}
.msg .nick {
  font-weight: 600;
  max-width: none !important;
  width: auto !important;
  text-align: left;
  padding-right: 60px; /* space for timestamp */
}
.msg .body {
  padding-left: 0;
  font-size: 14px;
  line-height: 1.5;
}
.msg.emergency {
  border-left: 3px solid var(--danger);
  background: none;
}
```

**Step 2: Test mobile message rendering**

Resize to mobile, verify:
- Nick appears on top line, bold, colored
- Timestamp right-aligned
- Message body full width below nick
- Emergency has red left border

**Step 3: Commit**

```bash
git add web/irc-radio-live.html
git commit -m "feat(irc): add card-style mobile message layout"
```

---

## Task 9: Integration Testing with Playwright

**Files:**
- No file changes — browser testing only

**Step 1: Test desktop flow**

Navigate to live instance at full width:
1. Search for "fire" — verify filtered results
2. Star a channel — verify it appears in Favorites section
3. Clear search — verify Favorites section at top of tree
4. Verify group unread counts visible
5. Toggle scanner on a channel, enable scanner mode

**Step 2: Test mobile flow**

Resize to 375x667:
1. Verify hamburger visible, panels hidden
2. Open channel drawer, switch channel, verify auto-close
3. Verify message card layout
4. Open nicklist drawer
5. Verify scanner controls accessible

**Step 3: Test scanner audio**

On desktop:
1. Add an active channel to scan list
2. Enable scanner
3. Wait for a call to arrive
4. Verify auto-play starts
5. Verify queue indicator if multiple calls arrive
6. Test skip button

**Step 4: Commit final state**

```bash
git add web/irc-radio-live.html
git commit -m "feat(irc): IRC client overhaul — search, favorites, scanner, mobile responsive"
```

---

## Summary

| Task | What | Estimated Scope |
|------|------|-----------------|
| 1 | Channel search box | ~50 lines CSS + HTML + JS |
| 2 | Favorites system | ~80 lines JS + CSS |
| 3 | Smart collapse + group unread | ~40 lines JS + CSS (modifying existing) |
| 4 | Scanner UI (state, controls, commands) | ~100 lines JS + CSS |
| 5 | Scanner audio queue + auto-play | ~80 lines JS |
| 6 | Mobile CSS + layout | ~120 lines CSS |
| 7 | Mobile drawers + swipe gestures | ~60 lines JS |
| 8 | Mobile message format | ~30 lines CSS |
| 9 | Integration testing | No code changes |

Total: ~560 lines of additions/modifications to a 2910-line file.
