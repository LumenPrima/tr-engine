# DeepInfra STT Provider Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `deepinfra` STT provider so users can transcribe calls using DeepInfra's Whisper API.

**Architecture:** New provider implementation (`deepinfra.go`) follows the same pattern as `elevenlabs.go` — implements the `Provider` interface, POSTs multipart audio to DeepInfra's native inference endpoint, maps their response format to our common `Response`/`Word` types. Config + init wiring in existing files.

**Tech Stack:** Go, net/http, multipart/form-data, DeepInfra REST API

**Design doc:** `docs/plans/2026-02-28-deepinfra-stt-provider-design.md`

---

### Task 1: Add DeepInfra config fields

**Files:**
- Modify: `internal/config/config.go:81-84` (after ElevenLabs config block)

**Step 1: Add config fields**

Add these two fields after the ElevenLabs block (after line 84) in the `Config` struct:

```go
	// DeepInfra STT (alternative to Whisper; used when STT_PROVIDER=deepinfra)
	DeepInfraAPIKey string `env:"DEEPINFRA_STT_API_KEY"`
	DeepInfraModel  string `env:"DEEPINFRA_STT_MODEL" envDefault:"openai/whisper-large-v3-turbo"`
```

**Step 2: Verify it compiles**

Run: `go build ./...`
Expected: Success (no errors)

**Step 3: Commit**

```bash
git add internal/config/config.go
git commit -m "feat: add DeepInfra STT config fields"
```

---

### Task 2: Create DeepInfra provider

**Files:**
- Create: `internal/transcribe/deepinfra.go`

**Step 1: Create the provider file**

Create `internal/transcribe/deepinfra.go` with this content:

```go
package transcribe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const deepInfraBaseURL = "https://api.deepinfra.com/v1/inference/"

// DeepInfraClient calls DeepInfra's native inference API for Whisper models.
// Implements the Provider interface.
type DeepInfraClient struct {
	apiKey  string
	model   string // e.g. "openai/whisper-large-v3-turbo"
	timeout time.Duration
	client  *http.Client
}

// deepInfraResponse is the JSON response from the DeepInfra inference API.
type deepInfraResponse struct {
	Text     string           `json:"text"`
	Language string           `json:"language"`
	Duration float64          `json:"duration"`
	Words    []deepInfraWord  `json:"words"`
}

// deepInfraWord is a word with timestamps from DeepInfra.
// Note: DeepInfra uses "text" for the word field, not "word" like OpenAI.
type deepInfraWord struct {
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// NewDeepInfraClient creates a new DeepInfra inference client.
func NewDeepInfraClient(apiKey, model string, timeout time.Duration) *DeepInfraClient {
	return &DeepInfraClient{
		apiKey:  apiKey,
		model:   model,
		timeout: timeout,
		client:  &http.Client{Timeout: timeout},
	}
}

// Name returns the provider name.
func (di *DeepInfraClient) Name() string { return "deepinfra" }

// Model returns the configured model identifier.
func (di *DeepInfraClient) Model() string { return di.model }

// Transcribe sends an audio file to DeepInfra's inference API and returns the result.
// Uses multipart/form-data with field name "audio" (DeepInfra's convention).
func (di *DeepInfraClient) Transcribe(ctx context.Context, audioPath string, opts TranscribeOpts) (*Response, error) {
	f, err := os.Open(audioPath)
	if err != nil {
		return nil, fmt.Errorf("open audio file: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Audio file field — DeepInfra uses "audio", not "file"
	part, err := w.CreateFormFile("audio", filepath.Base(audioPath))
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, fmt.Errorf("copy audio data: %w", err)
	}

	w.Close()

	// Endpoint: https://api.deepinfra.com/v1/inference/{model}
	url := deepInfraBaseURL + di.model

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "bearer "+di.apiKey)

	resp, err := di.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deepinfra request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deepinfra API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result deepInfraResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Convert DeepInfra word format (uses "text") to our common Word type (uses "Word")
	words := make([]Word, len(result.Words))
	for i, dw := range result.Words {
		words[i] = Word{
			Word:  dw.Text,
			Start: dw.Start,
			End:   dw.End,
		}
	}

	return &Response{
		Text:     result.Text,
		Language: result.Language,
		Duration: result.Duration,
		Words:    words,
	}, nil
}
```

**Step 2: Verify it compiles**

Run: `go build ./...`
Expected: Success

**Step 3: Commit**

```bash
git add internal/transcribe/deepinfra.go
git commit -m "feat: add DeepInfra STT provider"
```

---

### Task 3: Wire provider in main.go

**Files:**
- Modify: `cmd/tr-engine/main.go:162-176` (STT provider switch)

**Step 1: Add the deepinfra case**

In the STT provider switch block (after the `"elevenlabs"` case at line 171, before `"none"` at line 172), add:

```go
	case "deepinfra":
		if cfg.DeepInfraAPIKey == "" {
			log.Fatal().Msg("STT_PROVIDER=deepinfra requires DEEPINFRA_STT_API_KEY")
		}
		sttProvider = transcribe.NewDeepInfraClient(cfg.DeepInfraAPIKey, cfg.DeepInfraModel, cfg.WhisperTimeout)
```

**Step 2: Update the fatal error message to include deepinfra**

Change line 175 from:
```go
		log.Fatal().Str("provider", cfg.STTProvider).Msg("unknown STT_PROVIDER (valid: whisper, elevenlabs, none)")
```
to:
```go
		log.Fatal().Str("provider", cfg.STTProvider).Msg("unknown STT_PROVIDER (valid: whisper, elevenlabs, deepinfra, none)")
```

**Step 3: Verify it compiles**

Run: `go build ./...`
Expected: Success

**Step 4: Commit**

```bash
git add cmd/tr-engine/main.go
git commit -m "feat: wire DeepInfra provider in startup"
```

---

### Task 4: Add sample.env documentation

**Files:**
- Modify: `sample.env:324-325` (after ElevenLabs section, before LLM section)

**Step 1: Add DeepInfra section**

After the ElevenLabs section (after line 324 `# ELEVENLABS_KEYTERMS=`), add:

```env

# =============================================================================
# DeepInfra STT (alternative to Whisper — requires STT_PROVIDER=deepinfra)
# =============================================================================

# DeepInfra API key (required when STT_PROVIDER=deepinfra)
# Get yours at https://deepinfra.com/dash/api_keys
# DEEPINFRA_STT_API_KEY=

# DeepInfra Whisper model (default: openai/whisper-large-v3-turbo)
# See available models at https://deepinfra.com/models/automatic-speech-recognition
# DEEPINFRA_STT_MODEL=openai/whisper-large-v3-turbo
```

**Step 2: Update the STT provider comment at line 234**

Change:
```env
# STT provider: "whisper" (default) or "elevenlabs"
# Whisper requires WHISPER_URL. ElevenLabs requires ELEVENLABS_API_KEY.
```
to:
```env
# STT provider: "whisper" (default), "elevenlabs", or "deepinfra"
# Whisper requires WHISPER_URL. ElevenLabs requires ELEVENLABS_API_KEY. DeepInfra requires DEEPINFRA_STT_API_KEY.
```

**Step 3: Commit**

```bash
git add sample.env
git commit -m "docs: add DeepInfra STT config to sample.env"
```

---

### Task 5: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md` (configuration table and implementation status)

**Step 1: Add DeepInfra env vars to the configuration docs**

In the "Additional env-only settings" paragraph, add `DEEPINFRA_STT_API_KEY` and `DEEPINFRA_STT_MODEL` to the list of env-only settings.

**Step 2: Update the STT_PROVIDER comment in CLAUDE.md if referenced**

Search for any mention of `STT_PROVIDER` values and add `deepinfra` to the valid list.

**Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: document DeepInfra STT provider in CLAUDE.md"
```

---

### Task 6: Build and verify

**Step 1: Full build**

Run: `go build ./...`
Expected: Success with no errors or warnings.

**Step 2: Verify binary starts without DeepInfra config (no regression)**

Run: `./tr-engine.exe --version`
Expected: Prints version normally.

**Step 3: Verify fatal on missing API key**

Set `STT_PROVIDER=deepinfra` without `DEEPINFRA_STT_API_KEY` and confirm it fatals with the correct message.

**Step 4: Commit (if any fixes needed)**

Only if adjustments were needed from verification.
