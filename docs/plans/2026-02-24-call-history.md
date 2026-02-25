# Call History Page — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a searchable, filterable call log page with inline audio playback and expandable detail rows.

**Architecture:** Single self-contained HTML page (`web/call-history.html`) using the existing theme system and auth layer. All data comes from `GET /api/v1/calls` with embedded fields — no server changes needed. Expandable rows lazy-fetch transcription detail.

**Tech Stack:** Vanilla HTML/CSS/JS, theme-config.js, theme-engine.js, auth.js, `<audio>` element API.

**Design doc:** `docs/plans/2026-02-24-call-history-design.md`

---

### Task 1: Create page file with full HTML/CSS structure

**Files:**
- Create: `web/call-history.html`

**Step 1: Create the complete page with HTML skeleton + all CSS**

Create `web/call-history.html` with:
- Standard boilerplate: `auth.js?v=1` in head, `theme-config.js`, Google Fonts link, `theme-engine.js?v=2` at end of body
- Meta tags: `card-title="Call History"`, `card-description="Searchable call log with audio playback"`, `card-order="6"`
- Vignette overlay + theme label divs
- Filter toolbar with: range buttons (1h, 6h, 24h, 7d), system dropdown, talkgroup input, unit input, emergency toggle, encrypted toggle, transcription search box, sort dropdown
- Results table with columns: Time, Talkgroup, Dur, Units, Transcription, Play, Flags
- Loading spinner state, empty state
- Pagination bar (prev/next + page info)
- Hidden `<audio>` element

**CSS must include all of these (using theme variables throughout):**
- Page base (body, grid bg, scanlines, vignette)
- Toolbar (sticky at top:60px, glass-bg, backdrop-filter)
- Range buttons (active state with accent bg)
- Toggle buttons (emergency=danger, encrypted=warning)
- Search box
- Table (sticky thead, column widths via classes, hover rows)
- Emergency row accent (red left border)
- Talkgroup badge + clickable name
- Play button (circle, playing state fills with accent)
- Detail row (accent left border, grid layout)
- Transmission timeline bar (colored segments positioned absolutely)
- Signal quality badges
- Full transcription with hoverable words
- Metadata grid (key-value pairs)
- Audio player (progress bar with fill + head + seek)
- Loading/empty states with spinner animation
- Responsive breakpoint at 768px (hide transcript + flags columns)
- Scrollbar styling

Column width classes: `.col-time` 130px, `.col-tg` 22%, `.col-dur` 60px right-aligned, `.col-units` 50px centered, `.col-transcript` auto, `.col-play` 44px centered, `.col-flags` 50px centered.

**Step 2: Verify page appears in nav and CSS renders**

Open the page, confirm it shows in the nav dropdown, toolbar renders, table header displays, and CSS variables work with theme switching.

**Step 3: Commit**

```bash
git add web/call-history.html
git commit -m "feat(call-history): page skeleton with HTML/CSS structure"
```

---

### Task 2: Core JavaScript — State, Helpers, API Fetch, Table Rendering

**Files:**
- Modify: `web/call-history.html` (the `<script>` block)

**Step 1: Write the core JavaScript**

All code goes inside a single IIFE `(function() { 'use strict'; ... })();`

**Constants and state:**
- `API = '/api/v1'`, `PAGE_SIZE = 50`
- `calls = []`, `total = 0`, `offset = 0`, `activeRange = '24h'`
- `expandedCallId = null`, `currentAudioId = null`

**DOM refs:** getElementById for all toolbar inputs, tbody, loading, empty, pagination elements, audioEl.

**Helper functions:**
- `esc(s)` — creates a temporary div, sets textContent, returns the div's textContent (for safe attribute values)
- `fmtDuration(sec)` — returns "Xm Ys" or "Xs" or "—"
- `fmtFreq(hz)` — returns "XXX.XXXX MHz" or "—"
- `relativeTime(iso)` — returns "Xs ago", "Xm ago", "Xh ago", "Xd ago"
- `absTime(iso)` — returns `new Date(iso).toLocaleString()`
- `rangeStart(range)` — returns ISO string for now minus the range duration
- `fmtMMSS(sec)` — returns "M:SS" format
- `UNIT_COLORS` array of 8 theme variable colors, `unitColor(unitId)` picks by modulo

**buildParams():** Constructs URLSearchParams from current filter state:
- sort, deduplicate=true, limit, offset, start_time from range
- system_id, tgid, unit_id from inputs (if non-empty)
- emergency=true, encrypted=true from toggle active state

**fetchCalls():** Async function that:
1. Shows loading state, clears tbody, resets expandedCallId
2. Fetches `${API}/calls?${buildParams()}`
3. Parses response, stores calls and total
4. If transcription search has text, filters calls client-side (`.transcription_text.toLowerCase().includes(q)`)
5. Calls renderTable() and renderPagination()
6. On error: hides loading, shows empty state with error message

**renderTable():** Uses DOM API (createElement, appendChild) to build table rows. For each call:
- Create `<tr class="call-row">` with `data-id` attribute, add `.emergency` class if emergency
- Create 7 `<td>` cells:
  - Time: textContent = relativeTime, title = absTime
  - Talkgroup: create `.tg-name` span (textContent=alpha_tag, data-tgid), optionally append `.tg-tag` span for group
  - Duration: textContent = fmtDuration
  - Units: textContent = unit count from unit_ids or src_list length
  - Transcription: `.transcript-snippet` span, textContent = first 80 chars + "..." if truncated
  - Play: `.play-btn` button with SVG play icon (createElementNS for svg/polygon), add `.no-audio` class if no audio_url, store data-id and data-url attributes
  - Flags: append emoji text nodes for emergency/encrypted indicators
- Append row to tbody

**renderPagination():** Shows/hides pagination div. Calculates page/pages, updates pageInfo textContent, enables/disables prev/next buttons.

**Step 2: Verify calls load and display**

Open the page, confirm calls load from API, table renders with correct data, pagination works.

**Step 3: Commit**

```bash
git add web/call-history.html
git commit -m "feat(call-history): core fetch and table rendering"
```

---

### Task 3: Filter Event Listeners + URL Sync

**Files:**
- Modify: `web/call-history.html` (add to `<script>` block)

**Step 1: Wire up all filter controls**

**Range buttons:** querySelectorAll('.range-btn') click listener — toggle active class, set activeRange, reset offset, fetchCalls, syncURL.

**Toggle buttons:** emergencyToggle and encryptedToggle click listeners — toggle active class, reset offset, fetchCalls, syncURL.

**Dropdowns:** systemFilter and sortSelect change listeners — reset offset, fetchCalls, syncURL.

**Text inputs (debounced):** tgFilter, unitFilter, transcriptSearch input listeners — clearTimeout/setTimeout at 400ms, reset offset, fetchCalls, syncURL.

**Clickable talkgroup names:** tbody click listener — if target is `.tg-name`, set tgFilter.value from data-tgid, fetch and sync.

**Prev/next buttons:** click listeners adjusting offset by PAGE_SIZE.

**syncURL():** Builds URLSearchParams from current filter state (range, system, tg, unit, emergency, encrypted, q, sort), calls `history.replaceState`. Only includes non-default values.

**restoreFromURL():** Reads URLSearchParams from window.location.search, applies values to all filter controls. Call before initial fetchCalls.

**Populate systems dropdown:** loadSystems() fetches `${API}/systems`, creates `<option>` elements for each system, appends to systemFilter select.

**Init sequence:** restoreFromURL() → loadSystems() → fetchCalls()

**Step 2: Verify all filters work**

Test each filter control, confirm URL updates, reload page and verify filters restore from URL.

**Step 3: Commit**

```bash
git add web/call-history.html
git commit -m "feat(call-history): filter controls and URL sync"
```

---

### Task 4: Row Expansion + Detail Panel

**Files:**
- Modify: `web/call-history.html` (add to `<script>` block)

**Step 1: Add row click handler and detail panel rendering**

**tbody click handler:** On click (excluding .tg-name and .play-btn clicks), find closest `.call-row`, get callId from data-id, call toggleDetail.

**toggleDetail(callId, row):**
1. If already expanded (expandedCallId === callId), remove detail-row and reset expandedCallId
2. Otherwise: remove any existing detail-row, set expandedCallId
3. Create a detail-row `<tr>` with a single `<td colspan="7">` containing a loading spinner
4. Insert after the clicked row
5. Fetch transcription from `${API}/calls/${callId}/transcription` (catch 404 silently)
6. Replace loading spinner with full detail panel using DOM methods

**renderDetail(call, transcription):** Returns a DOM element (not HTML string). Uses createElement throughout:

- **Detail panel** (div.detail-panel, CSS grid 2-col):

- **Transmission timeline** (if src_list exists): h4 "Transmission Timeline" + div.timeline-bar containing absolutely-positioned segments. Each segment: div.timeline-seg styled with left%, width%, background color from unitColor(). Title attribute shows unit tag + duration.

- **Signal quality**: h4 + div.signal-badges containing signal-badge spans for signal_db, noise_db, error_count, spike_count, freq.

- **Frequency hops** (if freq_list.length > 1): Same timeline-bar pattern with info-colored segments.

- **Transcription**: If transcription has words array, render each word as a span.word with data-src and title. If only transcription_text available, render as plain text. If no transcription, skip section.

- **Audio player** (if audio_url): h4 + div.audio-player containing play button, progress bar (with transmission overlay segments and fill/head elements), time display. Store data-call-id on the player wrapper and progress-wrap for seeking.

- **Metadata**: h4 + div.meta-grid with key/value spans for system_name, site_short_name, audio_type, start/stop times, phase2_tdma, patched_tgids.

All text content set via textContent. Attribute values set via setAttribute or element properties. No string concatenation into markup.

**Step 2: Verify row expansion works**

Click rows, confirm detail panels expand/collapse, transcription loads, timeline renders.

**Step 3: Commit**

```bash
git add web/call-history.html
git commit -m "feat(call-history): expandable detail panel with transcription"
```

---

### Task 5: Inline Audio Player

**Files:**
- Modify: `web/call-history.html` (add to `<script>` block)

**Step 1: Add audio playback logic**

**audioEl event listeners:**
- `timeupdate`: Find the active .audio-player element by data-call-id, update progress fill width%, head left%, and time text.
- `ended`: Call stopAudio().

**playAudio(callId, url):**
1. If currentAudioId exists and is different, stop previous (remove .playing class, reset icon)
2. If same callId: toggle pause/resume
3. If new: set currentAudioId, set audioEl.src, play, set playing state

**stopAudio():** Reset playing state, clear currentAudioId, pause audioEl, clear src.

**setPlayingState(callId, playing):** Find all `.play-btn[data-id="${callId}"]` (both table row and detail panel), toggle .playing class, swap SVG between play triangle and pause bars (using createElementNS).

**Document-level click handlers:**
- Play button click: Find closest .play-btn (skip .no-audio), stopPropagation, call playAudio with id and url from data attributes.
- Progress bar seek: Find closest .audio-progress-wrap, calculate click position as percentage of width, set audioEl.currentTime.

**Step 2: Verify audio playback**

Test play/pause, seeking, switching between calls, progress bar updates.

**Step 3: Commit**

```bash
git add web/call-history.html
git commit -m "feat(call-history): inline audio player with seek"
```

---

### Task 6: Polish + Deploy

**Files:**
- Modify: `web/call-history.html`

**Step 1: Add relative time auto-refresh**

setInterval every 30s: iterate .call-row elements, find matching call by data-id, update time cell textContent with fresh relativeTime value.

**Step 2: Full end-to-end test**

Verify all 20 features:
1. Page loads with last 24h of calls, newest first, deduplicated
2. Date range buttons (1h, 6h, 24h, 7d) switch time window
3. System dropdown populates from API and filters
4. Talkgroup text filter works with debounce
5. Unit ID text filter works with debounce
6. Emergency toggle shows only emergency calls
7. Encrypted toggle shows only encrypted calls
8. Transcription search filters by text content
9. Sort dropdown (newest, oldest, longest) changes order
10. Pagination prev/next navigates pages
11. Row expansion shows detail panel
12. Transmission timeline renders colored unit segments
13. Signal quality badges display
14. Full transcription with word-level detail (if available)
15. Audio plays/pauses on button click
16. Progress bar seeks on click
17. Only one audio plays at a time
18. URL updates with filter state (shareable)
19. URL params restore on page load
20. Clicking talkgroup name sets filter
21. Emergency rows have red left border
22. Empty state shows when no results
23. Theme switching applies correctly across all elements
24. Responsive layout hides columns on mobile

**Step 3: Deploy to server**

```bash
scp web/call-history.html root@tr-dashboard:/data/tr-engine/v1/web/
```

**Step 4: Final commit + push**

```bash
git add web/call-history.html docs/plans/2026-02-24-call-history-design.md docs/plans/2026-02-24-call-history.md
git commit -m "feat: add Call History page with search, filters, and audio playback"
git push
```
