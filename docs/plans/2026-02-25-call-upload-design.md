# HTTP Call Upload Ingest — Design

## Summary

A new ingest path: `POST /api/v1/call-upload` accepts multipart call uploads compatible with both rdio-scanner and OpenMHz trunk-recorder plugins. This lets users who don't run MQTT or have local audio capture (e.g., uploading directly to Broadcastify) point their TR instance at tr-engine as an additional destination.

## Endpoint

`POST /api/v1/call-upload`

Single endpoint, auto-detects format from form field names.

## Format Detection

The audio file field name determines the format:
- **`audio`** → rdio-scanner format
- **`call`** → OpenMHz format

No ambiguity — field names don't overlap between the two formats.

### rdio-scanner Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `audio` | file | yes | Audio file (WAV or M4A) |
| `audioName` | string | no | Original filename |
| `audioType` | string | no | MIME type (`audio/wav` or `audio/mp4`) |
| `dateTime` | string | yes | Call start timestamp (Unix) |
| `frequency` | string | yes | Primary frequency |
| `talkgroup` | string | yes | Talkgroup ID |
| `system` | string | yes | System ID (numeric) |
| `systemLabel` | string | yes | System short name |
| `talkgroupGroup` | string | no | Talkgroup group |
| `talkgroupLabel` | string | no | Alpha tag |
| `talkgroupTag` | string | no | Tag |
| `talkgroupName` | string | no | Description |
| `sources` | JSON | no | `[{"pos":0.00,"src":12345,"tag":"Unit1"}]` |
| `frequencies` | JSON | no | `[{"freq":460000000,"time":0,"pos":0.00,"len":0,"errorCount":0,"spikeCount":0}]` |
| `patches` | JSON | no | Patched talkgroup IDs |
| `key` | string | yes | API key |

### OpenMHz Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `call` | file | yes | Audio file |
| `freq` | string | yes | Frequency |
| `start_time` | string | yes | Unix timestamp |
| `stop_time` | string | yes | Unix timestamp |
| `call_length` | string | no | Duration in seconds |
| `talkgroup_num` | string | yes | Talkgroup ID |
| `emergency` | string | no | Emergency flag |
| `error_count` | string | no | Error count |
| `spike_count` | string | no | Spike count |
| `source_list` | JSON | no | `[{"pos":0.00,"src":12345,"tag":"Unit1"}]` |
| `api_key` | string | yes | API key |
| `patch_list` | JSON | no | Patched talkgroup IDs |

## Auth

Three methods, checked in order:
1. `Authorization: Bearer {token}` header (existing middleware)
2. `key` form field (rdio-scanner) against `AUTH_TOKEN`
3. `api_key` form field (OpenMHz) against `AUTH_TOKEN`

The upload endpoint uses a relaxed middleware that skips the standard Bearer check and falls through to form field auth if no header is present.

## Data Flow

```
POST multipart → detect format → parse to AudioMetadata → ProcessUploadedCall()
                                                                ↓
                                            identity.Resolve → InsertCall → SSE publish
                                            saveAudioFile → UpdateCallFilename
                                            processSrcFreqData → upsert units
```

`ProcessUploadedCall()` is a thin public wrapper on the pipeline's existing `processWatchedFile` logic. Same code path the file watcher uses — no new DB queries needed.

## Responses

- `201 Created` — `{"call_id": 123, "system_id": 1, "tgid": 9044, "start_time": "...", "audio_file_path": "..."}`
- `409 Conflict` — duplicate call (same tgid + start_time within ±5s window)
- `400 Bad Request` — missing required fields or unparseable data
- `401 Unauthorized` — no valid Bearer header or form field key

## Config

| Variable | Default | Description |
|----------|---------|-------------|
| `UPLOAD_INSTANCE_ID` | `http-upload` | Instance ID for uploaded calls (like `WATCH_INSTANCE_ID` for file-watch) |

No other new config. Uses existing `AUTH_TOKEN` and `AUDIO_DIR`.

## TR Plugin Configuration

### rdio-scanner plugin
```json
{
  "name": "rdioscanner_uploader",
  "library": "librdioscanner_uploader.so",
  "server": "https://tr-engine.example.com",
  "systems": [{
    "shortName": "butco",
    "apiKey": "<AUTH_TOKEN value>",
    "systemId": 1
  }]
}
```

### OpenMHz plugin
```json
{
  "name": "openmhz_uploader",
  "library": "libopenmhz_uploader.so",
  "systems": [{
    "shortName": "butco",
    "apiKey": "<AUTH_TOKEN value>",
    "uploadServer": "https://tr-engine.example.com/api/v1"
  }]
}
```

## Files

- Create: `internal/api/upload.go` — endpoint handler, format detection, parsing
- Create: `internal/api/upload_test.go` — tests for format detection, field parsing, auth
- Modify: `internal/api/server.go` — register route
- Modify: `internal/ingest/pipeline.go` — expose `ProcessUploadedCall()` public method
- Modify: `openapi.yaml` — document new endpoint
- Modify: `sample.env` — add `UPLOAD_INSTANCE_ID`
- Modify: `internal/config/config.go` — add `UploadInstanceID` field
