# Talkgroup Research Page — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a two-view frontend page for investigating talkgroups — browse table + full-page detail view with charts, unit network graph, call history with audio, live affiliations, and event timeline.

**Architecture:** Single HTML file (`web/talkgroup-research.html`) following existing patterns: vanilla JS, inline CSS with theme variables, no framework. Chart.js via CDN for bar/line/doughnut charts. Hand-drawn SVG for unit network visualization. All data from existing REST API endpoints — no backend changes needed.

**Tech Stack:** HTML/CSS/JS (vanilla), Chart.js 4.x (CDN), SVG, tr-engine REST API

**Reference files for patterns:**
- `web/call-history.html` — toolbar, table rendering, audio playback, pagination, URL state
- `web/units.html` — grid layout, status colors
- `web/theme-config.js` — CSS variable names for theming
- `web/auth.js` — transparent auth (patches fetch, exposes `window.trAuth.getToken()`)

**Security note:** Use `textContent` for all user-supplied data. Use `createElement` + `textContent` for DOM building. Only use safe DOM methods — no raw HTML insertion with untrusted data.

---

### Task 1: Page Skeleton + Browse View

**Files:**
- Create: `web/talkgroup-research.html`

Build the complete page skeleton with the browse view (talkgroup table). This is the foundation everything else builds on.

**Step 1: Create the HTML file with standard boilerplate**

Follow the exact pattern from `call-history.html`:
- `<!DOCTYPE html>`, charset, viewport meta
- `<script src="auth.js?v=1"></script>` (must be first script)
- `<meta name="card-title" content="Talkgroup Research">`
- `<meta name="card-description" content="Deep-dive analysis of talkgroup activity, units, and relationships">`
- `<meta name="card-order" content="7">`
- `<title>Talkgroup Research — tr-engine</title>`
- Google Fonts preconnect + link (copy from call-history.html)
- `<script src="theme-config.js"></script>`
- Chart.js CDN: `<script src="https://cdn.jsdelivr.net/npm/chart.js@4/dist/chart.umd.min.js"></script>`
- Inline `<style>` block
- Body ends with `<script src="theme-engine.js?v=2"></script>`

**Step 2: Implement the inline CSS**

Follow the same base styles as call-history.html:
- Reset (`* { margin:0; padding:0; box-sizing:border-box; }`)
- Body with `var(--font-mono)`, `var(--bg)`, `var(--text)`, `font-size:13px`, flex column
- `body::before` grid overlay, `body::after` scanlines, `.vignette-overlay`, `.theme-label`
- `.toolbar` — sticky below header (top:60px), glass background, flex row with gap
- `.toolbar input, .toolbar select` — glass-styled inputs
- Table styles: `.tg-table` with full width, border-collapse, glass background
  - `th` — uppercase, small font, text-muted, sticky, left-aligned, cursor:pointer for sortable
  - `td` — padding 8px 12px, border-bottom with faint border
  - `tr:hover` — elevated background
  - `.sort-arrow` — indicator for active sort column
- `.content` — flex:1, padding, overflow-y:auto, position:relative, z-index:1
- `.pagination` — flex row, centered, gap, glass buttons
- `.loading-state`, `.empty-state` — centered text, muted color
- `.badge` — small pill with glass background, accent border
- `.stat-badge` — larger pill for header stats

Add styles for the detail view (will be populated in later tasks):
- `.detail-view` — hidden by default (`display:none`), full page
- `.detail-header` — glass panel with padding, flex column
- `.detail-back` — button styled like toolbar button
- `.charts-row` — flex row, gap, two equal children
- `.chart-panel` — glass background, border-radius, padding, flex:1
- `.tabs` — flex row of tab buttons, glass background
- `.tab-btn` — padding, cursor:pointer, muted text; `.tab-btn.active` accent border-bottom
- `.tab-content` — padding

**Step 3: Implement the browse view HTML structure**

Body contains:
- `.vignette-overlay` div
- `#themeLabel` div
- `#browseView` with toolbar (search input, system dropdown, result count), content area (table with thead/tbody), loading/empty states, pagination
- `#detailView` (hidden, populated dynamically in later tasks)
- Hidden `<audio>` element for playback

Table columns: TGID, Name, Group, 1h, 24h, Total, Units, Enc%, Last Seen. Headers have `data-sort` attributes for sortable columns.

**Step 4: Implement the browse JavaScript**

Inside a `<script>` block (IIFE pattern like call-history):

Core functions:
- `loadSystems()` — fetch `/systems`, populate dropdown
- `fetchTalkgroups()` — fetch `/talkgroups` with sort/search/pagination params, call `renderBrowseTable()`
- `renderBrowseTable()` — build table rows using `createElement` + `textContent` (no raw HTML with user data). Each row stores `dataset.systemId` and `dataset.tgid`. Click handler calls `showDetail(systemId, tgid)`.
- `updatePagination()` — show/hide prev/next, update page info
- `syncURL()` / `restoreFromURL()` — persist search/system/sort to URL params; detect `?tg=X:Y` for direct detail links
- `showDetail(systemId, tgid)` — hide browse, show detail, push URL state, call `loadDetail()` (placeholder for now)
- `showBrowse()` — hide detail, show browse
- `relativeTime(iso)` — human-friendly time diff
- `esc(s)` — escape HTML via createElement/textContent pattern
- Sort: click header toggles `currentSort` between `field` and `-field`, re-fetches
- Search: 300ms debounce on input
- `popstate` listener for back/forward navigation

**Step 5: Test in browser**

Open via Tailnet and verify:
- Talkgroup table loads with real data
- Search filters work (debounced)
- System dropdown filters
- Column sorting toggles
- Pagination works
- Clicking a row shows placeholder detail
- Back button returns to browse
- URL updates with filters

**Step 6: Deploy and commit**

```bash
scp web/talkgroup-research.html root@tr-dashboard:/data/tr-engine/v1/web/
git add web/talkgroup-research.html
git commit -m "feat(web): talkgroup research page — browse view with sortable table

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Detail View — Header + Stats + Charts

**Files:**
- Modify: `web/talkgroup-research.html`

Build the detail view header with talkgroup metadata, stat badges, and two Chart.js charts (activity timeline + site distribution).

**Step 1: Implement loadDetail() function**

Replace the placeholder. The function:
1. Clears detail view, builds DOM structure using createElement
2. Shows back button (calls `showBrowse()`)
3. Fetches `GET /talkgroups/{systemId}:{tgid}` for metadata
4. Populates header: TG name (alpha_tag), TGID, system name, group, tag, description
5. Renders stat badges: total calls, unique units, encryption %, first seen, last seen

**Step 2: Encryption doughnut (small, in header stats)**

- Small 80x80 canvas next to stat badges
- Chart.js doughnut: encrypted count vs clear count
- Colors: `var(--danger)` for encrypted, `var(--success)` for clear
- Center text plugin showing percentage
- Read CSS variable values via `getComputedStyle(document.body).getPropertyValue()`

**Step 3: Activity Timeline chart (Chart.js line/area)**

- Fetch `GET /calls?tgid=X&sort=start_time&limit=500` for chart data
- Client-side: bucket calls into hourly slots for last 24h
- Chart.js line chart with fill (area)
- X-axis: hour labels (e.g., "2pm", "3pm")
- Y-axis: call count per hour
- Fill color: accent at 20% opacity
- Line color: accent
- Responsive with `maintainAspectRatio: false`
- Place in left `.chart-panel`

**Step 4: Site Distribution doughnut**

- Group the same calls data by `site_short_name`
- Chart.js doughnut
- Color palette from theme: `--electric`, `--cyan`, `--lime`, `--magenta`, `--orange`
- Legend below chart with site names and counts
- Place in right `.chart-panel`

**Step 5: Test and deploy**

Navigate to a talkgroup detail, verify:
- Header shows correct metadata
- Charts render with real data
- Charts use theme colors
- Back button works
- Switch themes — charts should still be readable

```bash
scp web/talkgroup-research.html root@tr-dashboard:/data/tr-engine/v1/web/
git add web/talkgroup-research.html
git commit -m "feat(web): talkgroup research — detail header with stats and charts

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 3: Detail View — Units Tab with Top Talkers Chart

**Files:**
- Modify: `web/talkgroup-research.html`

Build the tabs system and the Units tab: horizontal bar chart of top talkers + full units table.

**Step 1: Implement tab system**

- Four tab buttons: Units, Calls, Affiliations, Events
- Click switches `.tab-btn.active` class and shows/hides `.tab-content` panels
- URL updates with `&tab=units` etc (via `replaceState`)
- Default tab: Units
- Tab state restored from URL on page load

**Step 2: Top Talkers horizontal bar chart**

- Data source: `GET /talkgroups/{id}/units?window=1440&limit=20`
- Chart.js horizontal bar chart
- Y-axis: unit alpha_tag (or "Unit " + unit_id if no tag)
- X-axis: call_count (from our new field)
- Show top 10 in chart
- Bar color: theme accent with gradient
- Click a bar to highlight corresponding row in table

**Step 3: Units table**

- Below the chart
- Columns: Unit ID, Alpha Tag, Call Count, Last Seen, Last Event Type
- Built with createElement + textContent (safe DOM methods)
- Rows clickable — stores unit data for expansion in Task 6

**Step 4: Test, deploy, commit**

```bash
scp web/talkgroup-research.html root@tr-dashboard:/data/tr-engine/v1/web/
git add web/talkgroup-research.html
git commit -m "feat(web): talkgroup research — units tab with top talkers chart

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 4: Detail View — Calls Tab with Audio Playback

**Files:**
- Modify: `web/talkgroup-research.html`

Build the Calls tab: compact call table with audio playback, expandable detail rows.

**Step 1: Calls table**

- Data source: `GET /calls?tgid=X&sort=-start_time&deduplicate=true&limit=25`
- Columns: Time (relative), Duration, Encrypted (lock icon), Site, Audio (play button)
- Pagination (offset-based, 25 per page)
- Encrypted calls show lock icon, no play button
- Build with createElement + textContent

**Step 2: Audio playback**

- Reuse the hidden `<audio id="audioEl">` element
- Play button per row, clicking loads audio URL with auth token:
  ```javascript
  var authToken = window.trAuth && window.trAuth.getToken();
  var url = API + '/calls/' + callId + '/audio';
  audioEl.src = authToken ? url + '?token=' + encodeURIComponent(authToken) : url;
  ```
- Highlight currently playing row with accent background
- Stop previous playback when clicking new row

**Step 3: Expandable detail rows**

- Click a non-audio area of a call row to expand/collapse
- Expanded row shows:
  - Transmission list (fetched from `GET /calls/{id}/transmissions`)
  - Transcription (fetched from `GET /calls/{id}/transcription`)
  - Unit IDs from the call
- Fetch on expand (lazy load)

**Step 4: Test, deploy, commit**

```bash
scp web/talkgroup-research.html root@tr-dashboard:/data/tr-engine/v1/web/
git add web/talkgroup-research.html
git commit -m "feat(web): talkgroup research — calls tab with audio playback

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 5: Detail View — Affiliations + Events Tabs

**Files:**
- Modify: `web/talkgroup-research.html`

Build the remaining two tabs.

**Step 1: Affiliations tab**

- Data source: `GET /unit-affiliations?tgid=X`
- Table columns: Unit ID, Alpha Tag, Status, Affiliated Since, Last Event
- Status indicator: green circle for "affiliated", gray circle for "off"
- Badge at top showing count of currently affiliated units
- Auto-refresh every 30 seconds (setInterval, clear on tab switch or view change)
- Relative times, refreshed on timer

**Step 2: Events tab**

- Data source: `GET /unit-events?system_id=X&tgid=Y&limit=50`
- Filter buttons row: All, Call, Join, Location, On, Off (toggle `type` query param)
- Table columns: Time (relative), Event Type, Unit ID, Unit Tag
- Event type color-coded pills: call=accent, join=success, location=warning, off=text-muted
- Pagination (offset-based, 50 per page)

**Step 3: Test, deploy, commit**

```bash
scp web/talkgroup-research.html root@tr-dashboard:/data/tr-engine/v1/web/
git add web/talkgroup-research.html
git commit -m "feat(web): talkgroup research — affiliations and events tabs

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 6: Unit Network Visualization (SVG)

**Files:**
- Modify: `web/talkgroup-research.html`

Build the interactive unit network graph on the Units tab.

**Step 1: SVG container and layout**

- Add a `.network-panel` div on the Units tab (beside top talkers chart on desktop, below on mobile)
- SVG element with viewBox for responsive sizing
- Central node = this talkgroup (larger circle, accent fill, alpha_tag label)
- Unit nodes arranged in a circle around center
- Node radius proportional to call_count (min 8px, max 24px)
- Lines from center to each unit, stroke-width proportional to call_count

**Step 2: Node rendering**

- Create SVG elements via `document.createElementNS('http://www.w3.org/2000/svg', ...)`
- Talkgroup center: `<circle>` + `<text>` with alpha_tag
- Unit nodes: `<circle>` with fill from theme (accent if tagged, text-muted if not)
- Labels: `<text>` with alpha_tag or truncated unit_id, positioned radially outside
- Hover: show tooltip div (positioned absolutely) with full details

**Step 3: Interactive expansion**

- Click a unit node:
  - Fetch `GET /units/{systemId}:{unitId}/calls?limit=20`
  - Extract unique tgids from results (excluding current TG)
  - Animate secondary nodes (other TGs) branching outward from clicked unit
  - Secondary nodes use different color (`--cyan` or `--lime`)
  - Click a secondary TG node to call `showDetail(systemId, tgid)` — navigate to that TG
- Click expanded unit again to collapse
- Only one unit expanded at a time

**Step 4: Animation and effects**

- Nodes fade in on initial render (CSS transition on opacity via class toggle)
- Expansion animates position (transition on cx/cy or transform)
- Hover glow: CSS filter `drop-shadow(0 0 6px var(--accent-glow))`
- Connecting lines animate opacity

**Step 5: Test, deploy, commit**

Test with TG 48686 (22 units). Verify:
- Nodes render in circle layout
- Sizes vary by call_count
- Click unit shows cross-TG branches
- Click secondary TG navigates
- Theme colors work
- Responsive

```bash
scp web/talkgroup-research.html root@tr-dashboard:/data/tr-engine/v1/web/
git add web/talkgroup-research.html
git commit -m "feat(web): talkgroup research — interactive unit network visualization

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 7: Polish + Final Deploy

**Files:**
- Modify: `web/talkgroup-research.html`

**Step 1: Responsive layout**

- Mobile breakpoint (`@media (max-width: 768px)`):
  - Charts stack vertically
  - Network panel full width below charts
  - Table columns reduce (hide group, 24h on small screens)
  - Toolbar wraps

**Step 2: Loading and empty states**

- Chart loading: pulsing gray placeholder rectangles
- Tab loading: centered "Loading..." with spinner animation
- Error states with message and retry button
- Empty states per tab:
  - No calls: "No calls recorded for this talkgroup"
  - No units: "No unit activity in the selected time window"
  - No affiliations: "No units currently affiliated"
  - No events: "No events in the selected time range"

**Step 3: Chart theme reactivity**

- Listen for theme changes (MutationObserver on body style or theme-engine event)
- Re-read CSS variables and update Chart.js instances with new colors
- Update SVG node fills/strokes

**Step 4: Final testing across themes**

Switch through all 11 themes and verify readability, color contrast, glass effects.

**Step 5: Final deploy and push**

```bash
scp web/talkgroup-research.html root@tr-dashboard:/data/tr-engine/v1/web/
git add web/talkgroup-research.html
git commit -m "feat(web): talkgroup research page — polish and responsive layout

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
git push
```
