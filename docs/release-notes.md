# Release Notes — v0.10.0

**Release Date:** 2026-05-27

This release brings major new capabilities to tr-engine, including a new transcription engine for P25 systems, administrative storage controls, expanded web dashboards, and a simplified authentication model. It also includes reliability improvements for high-volume MQTT ingest and real-time event streaming.

## New Features

### IMBE ASR Transcription for P25 Calls
tr-engine now supports direct transcription from P25 IMBE codec frames through the new IMBE ASR provider. When paired with the `tr-plugin-dvcf` plugin, SymbolStream v2 `.dvcf` files are ingested over MQTT and transcribed without requiring traditional audio file processing. This provides lower-latency transcription for P25 calls. To enable, set `STT_PROVIDER=imbe`. For mixed P25+analog systems, also set `STT_FALLBACK_PROVIDER=whisper` (or another audio STT) so non-DVCF calls still get transcribed.

### Storage Management
New administrative endpoints provide visibility into database storage usage and allow configuring retention policies. Administrators can view per-table statistics and trigger manual data purges directly from the API. Retention intervals for raw messages, console logs, and plugin statuses remain configurable via environment variables.

### Eight New Web Dashboards
The web frontend now includes eight new visualization dashboards for exploring system data, in addition to the existing set of built-in pages.

### P25 Call Alert (TSBK 0x1F) Support
The ingest pipeline now recognizes and displays P25 Call Alert events, including the target unit identifier, for improved situational awareness.

### Talkgroup and Unit CSV Export
New export pages allow downloading talkgroup and unit data in trunk-recorder-compatible CSV format, simplifying migration, backup, and sharing workflows.

## Authentication and Authorization

### Simplified Three-Mode Auth
Authentication has been redesigned into three clear modes:
- **Open**: No authentication required. Suitable for private networks.
- **Token**: A shared API token is required for all access.
- **Full**: JWT-based login for write operations, with optional public read tokens for guest access.

### API Key Support for Uploads
Upload requests now support `tre_`-prefixed API keys for programmatic access, making it easier to integrate third-party upload tools.

### Role-Based Write Access
In full authentication mode, JWT tokens carry editor and admin roles that gate write operations. The legacy `WRITE_TOKEN` is deprecated and will be removed in a future release.

## Improvements

- **Preserved Manual Edits**: Edits made to talkgroup or unit alpha tags through the UI are now protected from being overwritten by subsequent MQTT or CSV re-imports.
- **Wider Call Matching Tolerance**: The ingest pipeline now matches active calls within a 10-second window, up from 5 seconds, improving reliability when trunk-recorder shifts call timing slightly between start and end messages.
- **IRC View Enhancements**: The IRC-style live view now displays talkgroup IDs in the titlebar and correctly prioritizes short alpha tags for unit nicks.
- **SSE Reliability**: The real-time event stream now reports dropped-event metrics and uses a buffered dispatch loop to absorb MQTT ingress spikes, improving stability under high load.

## Bug Fixes

- Fixed API response shapes and field names across multiple dashboards.
- Fixed a potential panic caused by double-closing MQTT handlers during rapid reconnections.
- Fixed a data race in MQTT event dispatch.
- Fixed pagination to cap the limit parameter at 1000 results.
- Fixed an issue where auth tokens could leak into playground prompt history.
- Fixed unit event deduplication to include the target unit for call alert events.

## Upgrade Notes

- **Authentication**: The `AUTH_TOKEN` environment variable is no longer auto-generated. If you previously relied on automatic token generation, set `AUTH_TOKEN` explicitly before upgrading. `WRITE_TOKEN` is deprecated; migrate to `ADMIN_PASSWORD` with JWT roles for write access control.
- **IMBE ASR**: To use the new IMBE provider, install the `tr-plugin-dvcf` plugin on your trunk-recorder instance and set `STT_PROVIDER=imbe`. No database migration is required.
- **Database**: Schema migrations are applied automatically on startup. No manual migration steps are needed for this release.
- **Configuration**: All new features are opt-in via existing environment variables or runtime API calls.

## Full Changelog

[Compare v0.9.11 to v0.10.0](https://github.com/trunk-reporter/tr-engine/compare/v0.9.11...v0.10.0)
