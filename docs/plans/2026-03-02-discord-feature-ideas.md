# Feature Ideas from TR Discord #plugins-lobby

Source: Discord export of Trunk-Recorder #plugins-lobby channel, reviewed 2026-03-02.
Key contributors: taclane (tr-web author, MQTT plugin author), smashedbotatos (iCAD/AlertPage), lilhoser (callstream/pizzawave), dygear (UDP plugin), tadscottsmith (TG hot-reload), robotastic (TR creator).

## Already Have in tr-engine

- **Decode rate history charts** — `stream-graph.html`, persistent DB history (tr-web only has 5/15/60 min window)
- **Active/recent calls with audio + transcription** — `scanner.html`, `call-history.html`
- **Recorder status with state badges** — REST + SSE `recorder_update`, enriched with TG/unit matching
- **Console log viewer** — SSE `console` events in `events.html`
- **Trunking message display** — SSE `trunking_message`, stored in `trunking_messages` table
- **Affiliations/unit tracking** — `units.html`, affiliations API, persistent history
- **Systems overview** — `systems-overview.html`
- **Multi-instance aggregation** — MQTT-based, automatic P25 system merging
- **Call deduplication across sites** — call groups
- **Historical talkgroup analytics** — `analytics.html`, `call-history.html`, ad-hoc SQL via `/query`
- **Transcription pipeline** — Whisper/ElevenLabs/DeepInfra, built-in, with `provider_ms` performance tracking
- **DB maintenance** — configurable retention periods (`RETENTION_*` env vars), admin API (`GET/POST /admin/maintenance`)
- **SSE event streaming with filtering** — compound type syntax, system/site/TG/unit filters
- **Talkgroup directory** — `talkgroup-directory.html`, imported from TR CSV

## To Build

### RBAC / User Management
Replace token-based auth with proper user accounts, login form, and role-based access control. Requested by Austïn and MortalMonkey. Current two-tier model (AUTH_TOKEN read, WRITE_TOKEN write) works for single-operator setups but doesn't scale to shared deployments. Needs design phase — big feature.

### Decode Rate Dashboard (Frontend)
Visualize decode rate history over time per system. Backend endpoint exists (`GET /api/v1/stats/rates` with `system_id` filter, `decode_rate_interval`, `control_channel`). Just needs a frontend chart page. Requested by indecline ("sitting around 85%").

### Transcription Performance Dashboard (Frontend)
Display per-call real-time/transcribe-time ratio in the UI. Backend already tracks `provider_ms` and exposes rolling averages via queue stats API. Frontend visualization needed. Requested by gofaster for comparing tuning/models/providers.

### Network Graph Visualization
D3 force-directed unit-to-TG relationship graph, in-browser. Show clusters (Fire/EMS/PD) emerging from affiliation patterns. Time-window selector to see how the network evolves. tr-web exports to Gephi as a separate desktop app; we can do it live with historical replay. Data source: `unit_events` (affiliations) + `calls` (unit_ids). Inspired by tr-web's Gephi integration and community enthusiasm for network analysis.

### OmniTrunker-Style Live Status Page
Split view: active voice channels on top (system, channel freq, TG, alpha tag, source unit, unit alias, elapsed time, mode, encryption badge), unit activity feed on bottom with color-coded type badges (GRANT, AFFILIATION, REGISTRATION, DEREGISTRATION, ACKNOWLEDGE, DATA_GRANT, LOCATION). Buffer size control, type filter dropdown, auto-scroll toggle. Clean, dense, real-time. All data available via existing SSE events (`call_start`, `call_end`, `unit_event`, `trunking_message`). Pure frontend page.

### Colorized Console Log Viewer
Raw TR stdout with regex-based syntax coloring (ANSI codes are stripped in MQTT transit). Color patterns: call conclusions (green), encrypted calls (red), grants, affiliations, recorder assignments, frequencies, TG numbers. Search, filter by system/site, scroll through history from `console_messages` table. Different from `events.html` which shows structured events — this is the "terminal feel" that TR operators are used to seeing.

### Recorder Depth Chart
Visual timeline showing recorder utilization over time. How many recorders are idle/recording/available at any moment. Useful for capacity planning ("do I need more SDRs?"). Data source: `recorder_snapshots` table (append-only with decimation).

### Transcription Word Replacement / Custom Vocabulary
Per-system or per-TG dictionaries for consistent Whisper misrecognitions (town names, 10-codes, radio jargon). smashedbotatos has extensive production experience at 400-instance scale with iCAD. Common issues: town names, "10-4" transcribed as "so far", radio jargon mangled. Regex-based address extraction also useful.

### Historical Calls-Per-Hour / TG Heatmap
"Which TG had the most calls during X period?" Repeatedly requested in Discord, never well-served by any tool. Data is all in `calls` table — needs a visualization layer.

## Blocked on Upstream

### Denied Affiliation Tracking
GAV=2 (denied) affiliations are visible in TR console output but not published via MQTT plugin as structured events. Would reveal units attempting restricted TGs, out-of-area units, network access patterns. Needs TR or MQTT plugin to expose denied affiliations as unit events. Could theoretically be parsed from console log text as a workaround, but fragile.

### PTT Press/Release Events
Proposed by dygear as new plugin API hooks (`unit_ptt_pressed`, `unit_ptt_released`). More granular than call_start/call_end for unit activity tracking. Depends on upstream TR plugin API changes.

## Intentionally Skipping

- **Config editor / TR restart button** — TR-side concern, not a backend responsibility. tr-web can do this because it's an in-process plugin with direct access to TR internals.
- **Admin login history** — Different auth model (token-based vs username/password).

## Community Context

### tr-web (taclane) — Complementary, Not Competing
tr-web is a C++ plugin embedded in TR. No external dependencies, no database (2MB JSON file for affiliations). Live-only, single TR instance. Our advantages: persistent history, multi-instance aggregation, REST API, transcription, ad-hoc SQL. Their advantage: zero-dependency deployment, in-process access to TR internals (console output with ANSI colors, config editing, restart).

### Key Community Projects for Reference
- **iCAD** (smashedbotatos): https://github.com/TheGreatCodeholio/icad_transcript_api — Production transcription at scale
- **tr-web** (taclane): https://github.com/taclane/tr-web — Embedded web dashboard plugin
- **trunk-recorder-stack** (geometrix): https://github.com/ge0metrix/trunk-recorder-stack — MQTT-based transcription pipeline
- **tr-plugin-udp** (dygear): https://github.com/MimoCAD/tr-plugin-udp — Binary-packed UDP alternative to MQTT
- **trunk-transcribe** (CrimeIsDown): Referenced multiple times, has custom prompts per UID
