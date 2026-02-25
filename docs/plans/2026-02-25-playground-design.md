# Page Builder Playground — Design

## Summary

Two deliverables that demonstrate how easy it is to build custom frontends on tr-engine's API:

1. **`web/playground.html`** — Interactive prompt builder served by tr-engine. User describes what they want, picks a mode (integrated vs standalone), clicks "Build Prompt", copies the result into Claude (or any AI), pastes back the response to preview it live.
2. **`docs/building-pages.md`** — Static prompt template and guide. Copy-pasteable prompt scaffold, example descriptions, and explanation of both modes. Linked from README.

## Playground Page (`web/playground.html`)

Single HTML file following existing patterns (auth.js, theme-config.js, theme-engine.js).

### Layout

Two-panel layout:

**Left panel — Prompt Builder:**
- Textarea: "Describe what you want to build"
- Mode toggle: **Integrated** vs **Standalone**
  - Integrated: uses auth.js + theme-engine.js, lives in tr-engine's `web/` directory
  - Standalone: self-contained single file, works from anywhere with just an API URL
- "Build Prompt" button

**Right panel — Output:**
- Generated prompt in a readonly textarea with Copy button
- "Preview" section: textarea to paste Claude's HTML response + "Render" button
- Sandboxed iframe renders the pasted HTML

### Generated Prompt Contents

The prompt is intentionally minimal — the AI reads the full API spec and decides what endpoints to use:

1. Role instruction: "Build a single-file HTML page for tr-engine's REST API"
2. Link to the openapi.yaml: `https://raw.githubusercontent.com/LumenPrima/tr-engine/master/openapi.yaml`
3. Auth pattern (varies by mode):
   - **Integrated**: `<script src="auth.js?v=1"></script>` — all fetch/EventSource auto-authenticated
   - **Standalone**: Bootstrap snippet that fetches `{apiUrl}/auth-init` for the token, falls back to manual token input
4. Theme integration (integrated mode only): `<script src="theme-config.js"></script>` + `<script src="theme-engine.js?v=2"></script>`, list of key CSS variables (`--bg`, `--text`, `--accent`, `--glass-bg`, `--font-body`, `--font-mono`, etc.)
5. The user's description verbatim

### Auth Bootstrap (Standalone Mode)

Standalone pages need to connect to a tr-engine instance without `auth.js`. The generated prompt includes this pattern:

1. Config bar at top of page: text input for API URL + "Connect" button
2. On connect: fetch `{apiUrl}/auth-init` (unauthenticated, CORS default `*`)
3. If successful: store token, show green status, proceed
4. If CORS/network error: show a manual token input field as fallback
5. All subsequent fetches include `Authorization: Bearer {token}` header

~10 lines of JS in the generated template.

### Preview Iframe

- Sandboxed with `sandbox="allow-scripts allow-same-origin"` so the pasted HTML can make API calls
- For standalone mode: works directly since the page fetches from the configured API URL
- For integrated mode: the iframe content references auth.js/theme-engine.js relative to the parent — works when served from tr-engine

## Prompt Template Doc (`docs/building-pages.md`)

Markdown file containing:

- Brief intro: "tr-engine's REST API and SSE make it easy to build custom dashboards"
- Link to playground page and demo site
- The prompt skeleton (same as what the playground generates) in a fenced code block
- Explanation of integrated vs standalone modes
- 2-3 example descriptions:
  - "A live call feed that auto-updates via SSE with audio playback buttons"
  - "A unit activity heatmap showing which talkgroups each unit uses most"
  - "A talkgroup leaderboard showing busiest channels in the last hour"
- Link to the openapi.yaml

## Files

- Create: `web/playground.html`
- Create: `docs/building-pages.md`
- Modify: `README.md` (add link to building-pages doc and playground)
