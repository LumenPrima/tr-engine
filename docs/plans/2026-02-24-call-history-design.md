# Call History Page — Design

## Summary

Searchable, filterable call log with inline audio playback and expandable detail rows. Backed entirely by `GET /api/v1/calls` with embedded data — minimal extra API calls.

## Layout

### Top Filter Bar
- **Date range**: Preset buttons (1h, 6h, 24h, 7d) + custom date picker
- **Filters**: System dropdown, talkgroup text input, unit ID input, emergency toggle, encrypted toggle
- **Transcription search**: Text input that filters by transcription content (server-side via query param if supported, else client-side)
- **Sort**: Dropdown (newest, oldest, longest duration)
- URL query params mirror all filters for shareable links

### Results Table
Compact rows, each showing:
- Timestamp (relative + absolute on hover)
- Talkgroup alpha_tag + group badge (color-coded like index.html)
- Duration
- Unit count
- Transcription snippet (first ~80 chars, truncated)
- Play button (inline audio)
- Emergency indicator (red accent)
- Encrypted indicator (lock icon)

### Expandable Row Detail
Clicking a row expands it to show:
- **Transmission timeline**: Visual bar showing who talked when (src_list), colored by unit
- **Frequency hops**: freq_list visualization
- **Signal quality**: signal_db, noise_db, error_count
- **Full transcription**: Complete text with word-level timestamps (fetched from `/calls/{id}/transcription` on expand)
- **Metadata**: system_name, site_short_name, audio_type, phase2/tdma, patched_tgids

### Inline Audio Player
- Play/pause button on each row
- Progress bar showing position
- Transmission segments overlaid on progress bar (from src_list pos/duration)
- Only one audio plays at a time (clicking another stops the current)

## API Usage

- **Main list**: `GET /calls?sort=-start_time&deduplicate=true&limit=50&offset=N` + filter params
- **Audio**: `GET /calls/{id}/audio` — streamed into `<audio>` element
- **Transcription detail**: `GET /calls/{id}/transcription` — fetched on row expand only
- No need for `/calls/{id}/frequencies` or `/calls/{id}/transmissions` — both embedded as `freq_list` and `src_list` in the call object

## Behavior

- Default view: last 24h, newest first, deduplicated
- Pagination: 50 per page with prev/next + total count display
- Clicking talkgroup name filters to that talkgroup
- Clicking unit name filters to that unit
- Emergency calls: red left border or background tint
- Encrypted calls: lock icon, dimmed audio button if no audio available
- Empty state: "No calls found" with suggestion to adjust filters
- Loading state: Skeleton rows while fetching

## Theme Integration

- Uses theme-config.js + theme-engine.js (shared header, theme picker)
- Uses auth.js for API authentication
- Follows existing card-title/card-description/card-order meta tag pattern
- Suggested card-order: 6 (after Talkgroup Directory at 5, before API Docs at 10)

## Files

- `web/call-history.html` — Single self-contained page (HTML + CSS + JS)
- No server changes needed — all endpoints already exist
