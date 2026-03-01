# DeepInfra STT Provider

**Date:** 2026-02-28
**Status:** Approved
**Requested by:** MortalMonkey (Discord)

## Problem

Users with DeepInfra credits cannot use them for transcription. DeepInfra's Whisper API differs from the OpenAI-compatible format that the existing `whisper` provider expects:

| Aspect | OpenAI-compatible (current) | DeepInfra |
|--------|----------------------------|-----------|
| Endpoint | User-provided URL (POST as-is) | `https://api.deepinfra.com/v1/inference/{model}` |
| Audio form field | `file` | `audio` |
| Model location | `model` form field | URL path segment |
| Word timestamp key | `word` | `text` |
| Extra params | `response_format`, `timestamp_granularities[]`, `temperature` | Not needed/supported |

DeepInfra does NOT support `/v1/openai/audio/transcriptions` — their OpenAI-compatible endpoint only covers chat, completions, and embeddings.

## Solution

Add a dedicated `deepinfra` STT provider that handles DeepInfra's native inference API format.

## Implementation

### New file: `internal/transcribe/deepinfra.go`

```go
type DeepInfraClient struct {
    apiKey  string
    model   string           // e.g. "openai/whisper-large-v3-turbo"
    timeout time.Duration
    client  *http.Client
}
```

**Request:**
- POST `https://api.deepinfra.com/v1/inference/{model}`
- Header: `Authorization: bearer {apiKey}`
- Multipart form: single field `audio` with binary file stream
- No extra params (model in URL, timestamps returned by default)

**Response parsing:**
```json
{
  "text": "transcribed text",
  "language": "en",
  "duration": 45.5,
  "segments": [{"id": 0, "start": 0.0, "end": 2.5, "text": "..."}],
  "words": [{"start": 0.0, "end": 0.5, "text": "hello"}]
}
```
- Map `words[].text` to `Word.Word` (field name difference from OpenAI format)
- `segments` available but unused (word-level attribution uses `words`)

### Config additions: `internal/config/config.go`

```go
DeepInfraAPIKey string `env:"DEEPINFRA_STT_API_KEY"`
DeepInfraModel  string `env:"DEEPINFRA_STT_MODEL" envDefault:"openai/whisper-large-v3-turbo"`
```

### Provider init: `cmd/tr-engine/main.go`

Add `case "deepinfra"` to the STT provider switch, requiring `DEEPINFRA_STT_API_KEY`.
Reuse `WhisperTimeout` for the HTTP client timeout.

### User configuration

```env
STT_PROVIDER=deepinfra
DEEPINFRA_STT_API_KEY=di_xxxxxxxx
DEEPINFRA_STT_MODEL=openai/whisper-large-v3-turbo  # optional, this is the default
```

## What doesn't change

- `Provider` interface — no modifications
- Worker pool, queue, unit attribution — all unchanged
- `TranscribeOpts` passed but DeepInfra provider ignores unsupported fields
- Existing `whisper` and `elevenlabs` providers unaffected

## Files to modify

1. **Create** `internal/transcribe/deepinfra.go` — ~100 lines
2. **Edit** `internal/config/config.go` — add 2 config fields
3. **Edit** `cmd/tr-engine/main.go` — add `case "deepinfra"` in provider switch
4. **Edit** `sample.env` — add DeepInfra config examples
5. **Edit** `CLAUDE.md` — mention DeepInfra provider in implementation status

## References

- [DeepInfra Whisper tutorial](https://deepinfra.com/docs/tutorials/whisper)
- [DeepInfra whisper-large-v3-turbo API](https://deepinfra.com/openai/whisper-large-v3-turbo/api)
- [DeepInfra OpenAI-compatible API docs](https://deepinfra.com/docs/openai_api) (audio NOT supported)
