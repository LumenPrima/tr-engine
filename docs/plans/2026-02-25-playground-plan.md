# Page Builder Playground — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build an interactive prompt builder page and companion docs that demonstrate how easy it is to create custom frontends on tr-engine's API.

**Architecture:** Two deliverables — `web/playground.html` (interactive prompt builder with live preview) and `docs/building-pages.md` (static prompt template). The playground generates a minimal AI prompt containing the openapi.yaml URL, auth pattern, and user's description. No backend changes needed.

**Tech Stack:** HTML/CSS/JS (vanilla), tr-engine REST API, theme system (auth.js, theme-config.js, theme-engine.js)

**Reference files for patterns:**
- `web/call-history.html` — toolbar, glass styling, URL state
- `web/talkgroup-research.html` — two-panel layout
- `web/auth.js` — transparent auth (patches fetch, exposes `window.trAuth.getToken()`)
- `web/theme-config.js` — CSS variable names for theming
- `docs/plans/2026-02-25-playground-design.md` — approved design

**Security note:** Use `textContent` for all user-supplied data. Use `createElement` + `textContent` for DOM building. The preview iframe must be sandboxed.

---

### Task 1: Page Skeleton + Two-Panel Layout

**Files:**
- Create: `web/playground.html`

Build the complete page skeleton with two-panel layout and the prompt builder form (left panel). No prompt generation logic yet — just the UI.

**Step 1: Create the HTML file with standard boilerplate**

Follow the exact pattern from `call-history.html`:
- `<!DOCTYPE html>`, charset, viewport meta
- `<script src="auth.js?v=1"></script>` (must be first script)
- `<meta name="card-title" content="Page Builder">`
- `<meta name="card-description" content="Generate custom dashboard pages with AI">`
- `<meta name="card-order" content="11">`
- `<title>Page Builder — tr-engine</title>`
- Google Fonts preconnect + link (copy from call-history.html)
- `<script src="theme-config.js"></script>`
- Inline `<style>` block
- Body ends with `<script src="theme-engine.js?v=2"></script>`

**Step 2: Implement the inline CSS**

Base styles matching existing pages:
- Reset (`* { margin:0; padding:0; box-sizing:border-box; }`)
- Body with `var(--font-mono)`, `var(--bg)`, `var(--text)`, `font-size:13px`, flex column
- `body::before` grid overlay, `body::after` scanlines, `.vignette-overlay`, `.theme-label`

Playground-specific styles:
- `.playground` — flex row, gap:16px, padding:16px, flex:1, overflow:hidden
- `.panel` — flex:1, display:flex, flex-direction:column, gap:12px, min-width:0
- `.panel-header` — uppercase, small font, `var(--text-muted)`, letter-spacing:0.1em
- `.mode-toggle` — flex row with two buttons, glass background, border-radius
- `.mode-toggle button` — padding 8px 16px, cursor pointer, transition
- `.mode-toggle button.active` — accent background, white text
- `textarea` — glass background (`var(--glass-bg)`), glass border, `var(--font-mono)`, `var(--text)`, resize:vertical, border-radius
- `.description-input` — min-height:200px, flex:none
- `.prompt-output` — flex:1, min-height:200px, readonly styling
- `.preview-input` — min-height:150px
- `.btn` — glass background, accent border, padding 10px 20px, cursor pointer, uppercase, small font, letter-spacing
- `.btn:hover` — accent background, elevated shadow
- `.btn-copy` — smaller, inline with panel header
- `.preview-frame` — border:1px solid var(--border), border-radius, background:white, min-height:300px, flex:1
- `@media (max-width: 768px)` — stack panels vertically

**Step 3: Implement the HTML structure**

Body contains:
- `.vignette-overlay` div
- `#themeLabel` div
- `.playground` container with two `.panel` children

Left panel:
```html
<div class="panel">
  <div class="panel-header">Describe Your Page</div>
  <textarea class="description-input" id="description"
    placeholder="A live dashboard showing active calls with auto-refresh, audio playback buttons, and a chart of calls per hour by talkgroup..."></textarea>
  <div class="panel-header">Mode</div>
  <div class="mode-toggle">
    <button class="active" data-mode="integrated">Integrated</button>
    <button data-mode="standalone">Standalone</button>
  </div>
  <div class="mode-hint" id="modeHint">
    Page lives in tr-engine's web/ directory. Uses theme system and auto-auth.
  </div>
  <button class="btn" id="buildBtn">Build Prompt</button>
</div>
```

Right panel:
```html
<div class="panel">
  <div class="panel-header">
    Generated Prompt
    <button class="btn-copy" id="copyBtn" style="display:none">Copy</button>
  </div>
  <textarea class="prompt-output" id="promptOutput" readonly
    placeholder="Click 'Build Prompt' to generate..."></textarea>
  <div class="panel-header">Preview (paste AI response here)</div>
  <textarea class="preview-input" id="previewInput"
    placeholder="Paste the HTML that Claude generates..."></textarea>
  <button class="btn" id="renderBtn">Render Preview</button>
  <iframe class="preview-frame" id="previewFrame" sandbox="allow-scripts allow-same-origin"></iframe>
</div>
```

**Step 4: Implement basic interactivity**

Inside a `<script>` block (IIFE pattern):

- Mode toggle: click handlers swap `.active` class, update `#modeHint` text
  - Integrated: "Page lives in tr-engine's web/ directory. Uses theme system and auto-auth."
  - Standalone: "Self-contained HTML file. Works from anywhere — just needs the API URL."
- Build button: placeholder that just puts "TODO: prompt generation" in the output textarea
- Copy button: `navigator.clipboard.writeText()`, briefly change text to "Copied!"
- Render button: write `#previewInput` value into `#previewFrame` via `iframe.srcdoc`

**Step 5: Test in browser**

Open via the deployed instance and verify:
- Two-panel layout renders correctly
- Mode toggle switches
- Copy button works
- Render button displays pasted HTML in iframe
- Theme switching works (all 11 themes)
- Mobile layout stacks panels

**Step 6: Deploy and commit**

```bash
scp web/playground.html root@tr-dashboard:/data/tr-engine/v1/web/
git add web/playground.html
git commit -m "feat(web): page builder playground — skeleton with two-panel layout

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Prompt Generation Logic

**Files:**
- Modify: `web/playground.html`

Implement the core prompt builder that assembles the AI prompt based on mode selection and user description.

**Step 1: Define the prompt templates**

Add two template constants in the script block:

`INTEGRATED_TEMPLATE` — the prompt skeleton for integrated mode:
```
Build a single-file HTML page that will be served from tr-engine's web/ directory.

## API Specification
Read the full API spec here: https://raw.githubusercontent.com/LumenPrima/tr-engine/master/openapi.yaml

Use any endpoints you need. All endpoints are under /api/v1. Responses use {items, total, limit, offset} pagination.

## Authentication
Include this as the FIRST script in <head>:
<script src="auth.js?v=1"></script>

This automatically patches fetch() and EventSource to include auth headers. No manual auth code needed — just use fetch('/api/v1/...') normally.

## Theme System
Include these scripts:
<script src="theme-config.js"></script>  (in <head>, after auth.js)
<script src="theme-engine.js?v=2"></script>  (before </body>)

The theme engine injects a sticky header with nav and theme switcher. Use these CSS variables for styling:

Background: --bg, --bg-surface, --bg-elevated, --bg-tile
Text: --text, --text-mid, --text-muted, --text-faint
Accent: --accent, --accent-light, --accent-dim, --accent-glow
Status: --success, --warning, --danger, --info
Glass: --glass-bg, --glass-border, --glass-shine, --glass-blur
Typography: --font-display, --font-body, --font-mono
Borders: --border, --border-hover, --radius, --radius-sm
Shadows: --shadow-panel, --shadow-panel-hover

## Page Registration
Add these meta tags so the page appears in tr-engine's nav:
<meta name="card-title" content="YOUR PAGE TITLE">
<meta name="card-description" content="Short description">

## SSE Real-Time Events
For live updates, connect to /api/v1/events/stream with filter params:
const es = new EventSource('/api/v1/events/stream?types=call_start,call_end');
es.onmessage = (e) => { const data = JSON.parse(e.data); /* handle event */ };

Filter options: systems, sites, tgids, units, types, emergency_only (all optional, AND-ed).
Event types: call_start, call_end, unit_event, recorder_update, rate_update

## What to Build
{DESCRIPTION}
```

`STANDALONE_TEMPLATE` — the prompt skeleton for standalone mode:
```
Build a self-contained single-file HTML page that connects to a tr-engine REST API instance.

## API Specification
Read the full API spec here: https://raw.githubusercontent.com/LumenPrima/tr-engine/master/openapi.yaml

Use any endpoints you need. All endpoints are under /api/v1. Responses use {items, total, limit, offset} pagination.

## Authentication
The page needs to connect to a tr-engine instance. Include this auth bootstrap at the top of the page:

1. Show a config bar with an API URL input (default: window.location.origin) and a "Connect" button
2. On connect, fetch {apiUrl}/api/v1/auth-init to get the Bearer token (this endpoint requires no auth)
3. If successful, store the token and show a green connected indicator
4. If it fails (CORS or network), show a manual "Auth Token" input field as fallback
5. Create a helper function that wraps fetch to include the Authorization header:

function apiFetch(path, opts = {}) {
  return fetch(API_URL + path, {
    ...opts,
    headers: { ...opts.headers, 'Authorization': 'Bearer ' + TOKEN }
  });
}

Use apiFetch('/api/v1/...') for all API calls.

## SSE Real-Time Events
For live updates:
const url = API_URL + '/api/v1/events/stream?types=call_start,call_end&token=' + encodeURIComponent(TOKEN);
const es = new EventSource(url);
es.onmessage = (e) => { const data = JSON.parse(e.data); /* handle event */ };

## Styling
Use a clean, modern dark theme. No external CSS frameworks needed — inline styles are fine.
Suggested palette: #0a0a0f background, #e0e0e8 text, #00d4ff accent.

## What to Build
{DESCRIPTION}
```

**Step 2: Implement buildPrompt()**

Replace the placeholder Build button handler:
1. Read `#description` value, trim it
2. If empty, show brief error style on textarea (red border, shake animation), return
3. Get current mode from `.mode-toggle button.active`
4. Pick template (`INTEGRATED_TEMPLATE` or `STANDALONE_TEMPLATE`)
5. Replace `{DESCRIPTION}` with the user's text
6. Set `#promptOutput` value to the assembled prompt
7. Show the Copy button
8. Auto-select the prompt text

**Step 3: Test prompt generation**

Test both modes with a sample description like "A dashboard showing active calls with auto-refresh." Verify:
- Integrated mode includes theme variables and auth.js script tag
- Standalone mode includes the auth bootstrap and apiFetch pattern
- Description appears at the bottom
- Copy button copies the full prompt
- Empty description shows error feedback

**Step 4: Deploy and commit**

```bash
scp web/playground.html root@tr-dashboard:/data/tr-engine/v1/web/
git add web/playground.html
git commit -m "feat(web): page builder — prompt generation for integrated and standalone modes

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 3: Preview Iframe + Polish

**Files:**
- Modify: `web/playground.html`

Make the preview functional and add UX polish.

**Step 1: Implement the preview renderer**

When "Render Preview" is clicked:
1. Read `#previewInput` value
2. If empty, show error feedback, return
3. Set `#previewFrame.srcdoc` to the pasted HTML
4. For integrated mode previews: the iframe is same-origin (served from tr-engine), so auth.js and theme scripts resolve correctly
5. Scroll the iframe into view

**Step 2: Add URL state persistence**

- Save description and mode to URL params: `?mode=standalone&desc=...`
- Restore on page load via `URLSearchParams`
- Use `replaceState` on changes (not `pushState` — no need for back/forward here)
- This lets users bookmark/share their prompt setup

**Step 3: Add responsive layout**

Mobile breakpoint (`@media (max-width: 768px)`):
- Stack panels vertically (flex-direction: column)
- Playground container: padding reduce, overflow-y auto
- Textareas: min-height reduce slightly
- Preview frame: min-height 250px

**Step 4: Add a "try it" example**

Below the description textarea, add a small link row with 2-3 clickable example descriptions:
- "Live call feed with audio" — populates description with: "A live dashboard showing incoming calls via SSE. Each call shows talkgroup name, duration, unit count, and a play button for audio. Auto-scrolls as new calls arrive. Include a pause button to stop auto-scroll."
- "Talkgroup leaderboard" — populates with: "A leaderboard showing the busiest talkgroups in the last hour. Horizontal bar chart with talkgroup names on the y-axis and call count on the x-axis. Auto-refreshes every 60 seconds. Use Chart.js via CDN."
- "Unit activity tracker" — populates with: "A grid of unit cards showing all active units. Each card shows unit ID, alpha tag, last event type with a colored badge, and the talkgroup they're on. Updates live via SSE. Cards pulse briefly when their unit has new activity."

These examples double as documentation — they show users what kind of descriptions work well.

**Step 5: Test end-to-end**

1. Type a description, click Build Prompt, copy the prompt
2. Paste the prompt into Claude (external), get HTML back
3. Paste Claude's HTML into the preview textarea, click Render
4. Verify the page renders in the iframe
5. Test example links populate the description
6. Test both modes generate correct prompts
7. Test across 3-4 themes

**Step 6: Deploy and commit**

```bash
scp web/playground.html root@tr-dashboard:/data/tr-engine/v1/web/
git add web/playground.html
git commit -m "feat(web): page builder — live preview, examples, and polish

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 4: Building Pages Documentation

**Files:**
- Create: `docs/building-pages.md`
- Modify: `README.md`

Write the static prompt template doc and link it from the README.

**Step 1: Create docs/building-pages.md**

Structure:
```markdown
# Building Custom Pages

tr-engine's REST API and SSE event stream make it easy to build custom
dashboards and visualizations. You can generate pages with an AI assistant
like Claude — just describe what you want and provide the API spec.

## Quick Start: Page Builder

The fastest way: open the [Page Builder](/playground.html) on your tr-engine
instance, describe what you want, and copy the generated prompt into Claude.

Live demo: [tr-engine.luxprimatech.com/playground.html](https://tr-engine.luxprimatech.com/playground.html)

## Two Modes

### Integrated (recommended for tr-engine users)

Pages live in tr-engine's `web/` directory and use the built-in theme system
and auto-auth. They appear in the nav dropdown automatically.

### Standalone

Self-contained HTML files that work from anywhere. The page connects to your
tr-engine instance by URL and bootstraps auth via the `/auth-init` endpoint.
Good for sharing, embedding, or running locally.

## Manual Prompt Template

If you prefer to copy-paste directly, here's the prompt skeleton:

### Integrated Mode

[fenced code block with INTEGRATED_TEMPLATE, {DESCRIPTION} placeholder noted]

### Standalone Mode

[fenced code block with STANDALONE_TEMPLATE, {DESCRIPTION} placeholder noted]

## Example Descriptions

These work well as-is or as starting points:

- **Live call feed with audio**: "A live dashboard showing incoming calls
  via SSE. Each call shows talkgroup name, duration, unit count, and a play
  button for audio. Auto-scrolls as new calls arrive."

- **Talkgroup leaderboard**: "A leaderboard showing the busiest talkgroups
  in the last hour. Horizontal bar chart with call count. Auto-refreshes
  every 60 seconds. Use Chart.js via CDN."

- **Unit activity tracker**: "A grid of unit cards showing all active units.
  Each card shows unit ID, alpha tag, last event type with a colored badge.
  Updates live via SSE."

## API Reference

Full API specification: [openapi.yaml](https://raw.githubusercontent.com/LumenPrima/tr-engine/master/openapi.yaml)

Interactive docs: [API Docs](/docs.html) on your tr-engine instance

Key endpoints:
- `GET /api/v1/calls` — recorded calls (paginated, filterable)
- `GET /api/v1/calls/active` — in-progress calls
- `GET /api/v1/talkgroups` — all talkgroups with activity stats
- `GET /api/v1/units` — radio units
- `GET /api/v1/events/stream` — real-time SSE event stream
- `GET /api/v1/stats` — system statistics

See the [README](../README.md) for the full endpoint list.
```

**Step 2: Update README.md**

Add a "Building Custom Pages" entry to the Web UI table:
```
| **Page Builder** | Generate custom dashboard pages with AI — describe what you want, get a working page |
```

Add a one-liner after the Web UI section or in the Quick Start area:
```markdown
Want to build your own dashboard? See **[Building Custom Pages](docs/building-pages.md)** or open the built-in [Page Builder](/playground.html).
```

**Step 3: Deploy and commit**

```bash
scp web/playground.html root@tr-dashboard:/data/tr-engine/v1/web/
git add docs/building-pages.md README.md
git commit -m "docs: building custom pages guide with prompt templates

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
git push
```
