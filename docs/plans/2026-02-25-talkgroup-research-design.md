# Talkgroup Research Page — Design

## Summary

A two-view frontend page for investigating talkgroups: a sortable browse table of all heard talkgroups, and a full-page detail view with stats, charts, unit network visualization, call history with audio playback, live affiliations, and event timeline.

## Technology

- Single HTML file: `web/talkgroup-research.html`
- Vanilla JS, inline CSS with theme variables
- Chart.js via CDN for bar/line/doughnut charts
- Hand-drawn SVG for unit network graph
- Standard includes: auth.js, theme-config.js, theme-engine.js
- card-title: "Talkgroup Research", card-order: 7

## View 1: Browse

Sortable table of all heard talkgroups. Entry point for the page.

**Data source:** `GET /talkgroups?sort=-calls_1h&limit=50` with pagination.

**Columns:**

| Column | Source Field | Sortable |
|--------|------------|----------|
| TGID | `tgid` | Yes |
| Name | `alpha_tag` | Yes |
| Group | `group` | Yes |
| Calls (1h) | `calls_1h` | Yes |
| Calls (24h) | `calls_24h` | Yes |
| Units | `unit_count` | Yes |
| Encrypted % | computed from `encrypted_count/call_count` | No |
| Last Seen | `last_seen` | Yes |

**Features:**
- Search bar filters by alpha_tag or tgid (server-side `search` param)
- System filter dropdown (populated from `GET /systems`)
- Click any row to enter detail view
- URL state: `?search=X&system=Y&sort=Z` preserved across navigation

## View 2: Detail (Full-Page Takeover)

Triggered by clicking a TG row or navigating to `?tg=3:48686` (or `3-48686`). Back button returns to browse with scroll/filter state preserved.

### Header Section

**Data source:** `GET /talkgroups/{id}` (single TG with stats)

Displays:
- TG name (alpha_tag), TGID, system name, group, tag, description
- Stat badges: total calls, unique units, encryption %, first seen, last seen
- Encryption doughnut (small, inline with stats) — clear vs encrypted

### Charts Row

Two charts side by side below the header.

**Activity Timeline** (Chart.js line/area chart):
- Data source: `GET /calls?tgid=X&sort=start_time&limit=500` (client buckets into hourly slots)
- X-axis: hours (last 24h)
- Y-axis: call count per hour
- Filled area with theme accent color, semi-transparent
- Shows when the TG is hot vs quiet

**Site Distribution** (Chart.js doughnut):
- Data source: same calls response, client groups by `site_short_name`
- Shows geographic spread of where calls are heard
- Theme-aware color palette

### Tabbed Content Area

Four tabs: **Units**, **Calls**, **Affiliations**, **Events**

#### Units Tab (default)

Two panels side by side:

**Top Talkers** (Chart.js horizontal bar chart):
- Data source: `GET /talkgroups/{id}/units?window=1440&limit=20`
- Uses new `call_count` field
- Unit alpha_tag on Y-axis, call count on X-axis
- Top 10 shown in chart, full list in table below
- Theme accent gradient on bars

**Unit Network** (SVG, hand-drawn):
- Central node = this talkgroup
- Connected nodes = units (sized by call_count)
- Click a unit node to expand: fetches `GET /units/{id}/calls?limit=20`, shows other talkgroups that unit talks on as secondary nodes
- Simple force-directed layout (custom JS, no library)
- Nodes colored by: has alpha_tag (accent) vs no tag (muted)
- Lines thickness proportional to call_count

**Units Table** (below charts):
- Full unit list from `/talkgroups/{id}/units`
- Columns: unit_id, alpha_tag, call_count, last_seen, last_event_type
- Click row to highlight in network graph and show cross-TG activity

#### Calls Tab

**Data source:** `GET /calls?tgid=X&sort=-start_time&deduplicate=true&limit=25`

Compact call table:
- Columns: time, duration, encrypted, site, audio
- Play button per row (same audio pattern as call-history: append `?token=` to audio URL)
- Expandable rows for transmission detail (src_list, frequencies)
- Pagination (offset-based, 25 per page)
- Hidden `<audio>` element shared across all rows

#### Affiliations Tab

**Data source:** `GET /unit-affiliations?tgid=X`

Live view of who's currently on the channel:
- Columns: unit_id, alpha_tag, status (affiliated/off), affiliated_since, last_event_time
- Color-coded: green for affiliated, gray for off
- Auto-refreshes every 30 seconds
- Badge showing count of currently affiliated units

#### Events Tab

**Data source:** `GET /unit-events?system_id=X&tgid=Y&limit=50`

Timeline of unit activity on this talkgroup:
- Columns: time, event_type, unit_id (using new `unit_id` alias), unit_alpha_tag
- Filter buttons for event type (all, call, join, location, on, off)
- Pagination
- Color-coded event types (call=blue, join=green, location=orange, off=gray)

## URL State

The page uses URL query params for all navigable state:

- Browse: `talkgroup-research.html?search=SWAT&system=3&sort=-calls_1h`
- Detail: `talkgroup-research.html?tg=3:48686` (or `3-48686`)
- Detail with tab: `talkgroup-research.html?tg=3:48686&tab=calls`

Browser back/forward works naturally. Direct links to detail views work.

## API Endpoints Used

| Endpoint | View | Purpose |
|----------|------|---------|
| `GET /systems` | Browse | Populate system filter dropdown |
| `GET /talkgroups` | Browse | Main table data with sort/search/pagination |
| `GET /talkgroups/{id}` | Detail | Header stats and metadata |
| `GET /talkgroups/{id}/units` | Detail/Units | Units list with call_count |
| `GET /calls` | Detail/Calls + Charts | Call history + activity timeline data |
| `GET /unit-affiliations` | Detail/Affiliations | Live channel membership |
| `GET /unit-events` | Detail/Events | Unit activity timeline |
| `GET /units/{id}/calls` | Detail/Network | Cross-TG activity for network expansion |
| `GET /calls/{id}/audio` | Detail/Calls | Audio playback |

## Files

- Create: `web/talkgroup-research.html` (single file, all JS/CSS inline)
- No backend changes needed — all API endpoints already exist
