# HTTP Call Upload Ingest — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `POST /api/v1/call-upload` endpoint that accepts multipart call uploads compatible with both rdio-scanner and OpenMHz trunk-recorder plugins, providing a third ingest path alongside MQTT and file-watch.

**Architecture:** Single endpoint with auto-detection of upload format (rdio-scanner vs OpenMHz) based on form field names. Parses to the existing `AudioMetadata` struct, then calls a new public `ProcessUploadedCall()` method on Pipeline that reuses the same identity resolution, call creation, audio saving, and SSE publishing logic as the file watcher.

**Tech Stack:** Go, Chi router, multipart/form-data parsing, existing Pipeline ingest infrastructure

**Reference files:**
- `internal/ingest/handler_audio.go` — `processWatchedFile` (lines 339-447), `createCallFromAudio`, `saveAudioFile`
- `internal/ingest/messages.go` — `AudioMetadata`, `SrcItem`, `FreqItem` structs
- `internal/api/calls.go` — handler pattern (struct, constructor, Routes method)
- `internal/api/server.go` — route registration pattern
- `internal/api/middleware.go` — `BearerAuth` middleware (lines 211-237)
- `internal/config/config.go` — config struct with env tags
- `docs/plans/2026-02-25-call-upload-design.md` — approved design

---

### Task 1: Config + Upload Instance ID

**Files:**
- Modify: `internal/config/config.go`
- Modify: `sample.env`

Add the `UPLOAD_INSTANCE_ID` config field.

**Step 1: Add config field**

In `internal/config/config.go`, find the `WatchInstanceID` field (around line 27) and add below it:

```go
UploadInstanceID  string `env:"UPLOAD_INSTANCE_ID" envDefault:"http-upload"`
```

**Step 2: Add sample.env documentation**

In `sample.env`, find the `WATCH_INSTANCE_ID` comment block (around line 96) and add after the `WATCH_BACKFILL_DAYS` line:

```
# Instance ID for HTTP-uploaded calls (used for identity resolution)
# UPLOAD_INSTANCE_ID=http-upload
```

**Step 3: Verify build**

```bash
go build ./...
```

**Step 4: Commit**

```bash
git add internal/config/config.go sample.env
git commit -m "feat(config): add UPLOAD_INSTANCE_ID for HTTP call upload ingest

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Pipeline Public Method — ProcessUploadedCall

**Files:**
- Modify: `internal/ingest/pipeline.go` or create `internal/ingest/handler_upload.go`
- Create: `internal/ingest/handler_upload_test.go`

Expose a public method on Pipeline that the API handler can call. This reuses the existing `createCallFromAudio`, `saveAudioFile`, `processSrcFreqData`, and SSE publishing logic.

**Step 1: Write the test**

Create `internal/ingest/handler_upload_test.go`:

```go
package ingest

import (
	"testing"
)

func TestParseRdioScannerFields(t *testing.T) {
	// Test that rdio-scanner form fields are correctly parsed to AudioMetadata
	tests := []struct {
		name      string
		fields    map[string]string
		wantTgid  int
		wantFreq  float64
		wantShort string
		wantErr   bool
	}{
		{
			name: "valid rdio-scanner fields",
			fields: map[string]string{
				"talkgroup":   "9044",
				"frequency":   "859262500",
				"dateTime":    "1708881234",
				"system":      "1",
				"systemLabel": "butco",
			},
			wantTgid:  9044,
			wantFreq:  859262500,
			wantShort: "butco",
		},
		{
			name: "missing talkgroup",
			fields: map[string]string{
				"frequency":   "859262500",
				"dateTime":    "1708881234",
				"systemLabel": "butco",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, err := parseRdioScannerFields(tt.fields)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if meta.Talkgroup != tt.wantTgid {
				t.Errorf("Talkgroup = %d, want %d", meta.Talkgroup, tt.wantTgid)
			}
			if meta.Freq != tt.wantFreq {
				t.Errorf("Freq = %f, want %f", meta.Freq, tt.wantFreq)
			}
			if meta.ShortName != tt.wantShort {
				t.Errorf("ShortName = %q, want %q", meta.ShortName, tt.wantShort)
			}
		})
	}
}

func TestParseOpenMHzFields(t *testing.T) {
	tests := []struct {
		name      string
		fields    map[string]string
		wantTgid  int
		wantStart int64
		wantStop  int64
		wantErr   bool
	}{
		{
			name: "valid openmhz fields",
			fields: map[string]string{
				"talkgroup_num": "9044",
				"freq":          "859262500",
				"start_time":    "1708881234",
				"stop_time":     "1708881276",
			},
			wantTgid:  9044,
			wantStart: 1708881234,
			wantStop:  1708881276,
		},
		{
			name: "missing talkgroup_num",
			fields: map[string]string{
				"freq":       "859262500",
				"start_time": "1708881234",
				"stop_time":  "1708881276",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, err := parseOpenMHzFields(tt.fields)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if meta.Talkgroup != tt.wantTgid {
				t.Errorf("Talkgroup = %d, want %d", meta.Talkgroup, tt.wantTgid)
			}
			if meta.StartTime != tt.wantStart {
				t.Errorf("StartTime = %d, want %d", meta.StartTime, tt.wantStart)
			}
			if meta.StopTime != tt.wantStop {
				t.Errorf("StopTime = %d, want %d", meta.StopTime, tt.wantStop)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/ingest/ -run TestParseRdioScanner -v
go test ./internal/ingest/ -run TestParseOpenMHz -v
```

Expected: FAIL — functions don't exist yet.

**Step 3: Implement the parsing functions and ProcessUploadedCall**

Create `internal/ingest/handler_upload.go`:

```go
package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog"
)

// UploadResult is returned by ProcessUploadedCall on success.
type UploadResult struct {
	CallID        int64     `json:"call_id"`
	SystemID      int       `json:"system_id"`
	Tgid          int       `json:"tgid"`
	StartTime     time.Time `json:"start_time"`
	AudioFilePath string    `json:"audio_file_path,omitempty"`
}

// ProcessUploadedCall handles an HTTP-uploaded call. It reuses the same pipeline
// as processWatchedFile: identity resolution, dedup, call creation, audio saving,
// src/freq processing, unit upserts, and SSE event publishing.
func (p *Pipeline) ProcessUploadedCall(ctx context.Context, instanceID string, meta *AudioMetadata, audioData []byte, audioFilename string) (*UploadResult, error) {
	log := p.log.With().
		Str("handler", "upload").
		Str("instance_id", instanceID).
		Int("talkgroup", meta.Talkgroup).
		Str("short_name", meta.ShortName).
		Logger()

	startTime := time.Unix(meta.StartTime, 0)

	// Resolve system/site identity
	identity, err := p.identity.Resolve(ctx, instanceID, meta.ShortName)
	if err != nil {
		return nil, fmt.Errorf("identity resolution: %w", err)
	}

	// Dedup check
	existingID, _, findErr := p.db.FindCallForAudio(ctx, identity.SystemID, meta.Talkgroup, startTime)
	if findErr == nil {
		log.Debug().Int64("existing_call_id", existingID).Msg("duplicate call, skipping")
		return nil, fmt.Errorf("duplicate call: existing call_id=%d", existingID)
	}

	// Create the call record
	callID, callStartTime, err := p.createCallFromAudio(ctx, identity, meta, startTime)
	if err != nil {
		return nil, fmt.Errorf("create call: %w", err)
	}

	result := &UploadResult{
		CallID:    callID,
		SystemID:  identity.SystemID,
		Tgid:      meta.Talkgroup,
		StartTime: callStartTime,
	}

	// Save audio file if provided
	if len(audioData) > 0 {
		audioType := meta.AudioType
		if audioType == "" {
			audioType = "m4a"
		}
		if audioFilename == "" {
			audioFilename = fmt.Sprintf("%d.%s", meta.StartTime, audioType)
		}

		audioPath, err := p.saveAudioFile(meta.ShortName, startTime, audioFilename, audioType, audioData)
		if err != nil {
			log.Error().Err(err).Msg("failed to save audio file")
			// Don't fail the whole upload — call record is already created
		} else {
			p.db.UpdateCallFilename(ctx, callID, callStartTime, audioPath)
			result.AudioFilePath = audioPath
		}
	}

	// Process src/freq data (transmissions, frequencies, unit IDs)
	p.processSrcFreqData(ctx, callID, callStartTime, meta)

	// Upsert units from srcList
	for _, s := range meta.SrcList {
		if s.Src > 0 {
			p.db.UpsertUnit(ctx, identity.SystemID, s.Src, s.Tag, "upload", startTime, meta.Talkgroup)
		}
	}

	// Compute duration for SSE event
	duration := float64(meta.CallLength)
	if duration == 0 && meta.StopTime > 0 {
		duration = float64(meta.StopTime - meta.StartTime)
	}

	// Publish SSE event
	p.PublishEvent(EventData{
		Type:      "call_end",
		SystemID:  identity.SystemID,
		SiteID:    identity.SiteID,
		Tgid:      meta.Talkgroup,
		Emergency: meta.Emergency > 0,
		Payload: map[string]any{
			"call_id":       callID,
			"tgid":          meta.Talkgroup,
			"tg_alpha_tag":  meta.TalkgroupTag,
			"freq":          meta.Freq,
			"start_time":    callStartTime.Format(time.RFC3339),
			"duration":      duration,
			"source":        "upload",
		},
	})

	// Enqueue transcription if applicable
	if meta.Encrypted == 0 && result.AudioFilePath != "" {
		p.enqueueTranscription(callID, callStartTime, identity.SystemID, result.AudioFilePath, meta)
	}

	log.Info().
		Int64("call_id", callID).
		Int("system_id", identity.SystemID).
		Str("audio_path", result.AudioFilePath).
		Msg("uploaded call processed")

	return result, nil
}

// parseRdioScannerFields parses rdio-scanner upload form fields into AudioMetadata.
func parseRdioScannerFields(fields map[string]string) (*AudioMetadata, error) {
	tgStr := fields["talkgroup"]
	if tgStr == "" {
		return nil, fmt.Errorf("missing required field: talkgroup")
	}
	tg, err := strconv.Atoi(tgStr)
	if err != nil {
		return nil, fmt.Errorf("invalid talkgroup: %s", tgStr)
	}

	dateTime := fields["dateTime"]
	if dateTime == "" {
		return nil, fmt.Errorf("missing required field: dateTime")
	}
	startTime, err := strconv.ParseInt(dateTime, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid dateTime: %s", dateTime)
	}

	shortName := fields["systemLabel"]
	if shortName == "" {
		return nil, fmt.Errorf("missing required field: systemLabel")
	}

	meta := &AudioMetadata{
		Talkgroup:         tg,
		StartTime:         startTime,
		ShortName:         shortName,
		TalkgroupTag:      fields["talkgroupLabel"],
		TalkgroupDesc:     fields["talkgroupName"],
		TalkgroupGroupTag: fields["talkgroupTag"],
		TalkgroupGroup:    fields["talkgroupGroup"],
	}

	if freqStr := fields["frequency"]; freqStr != "" {
		if f, err := strconv.ParseFloat(freqStr, 64); err == nil {
			meta.Freq = f
		}
	}

	// Parse sources JSON
	if sourcesStr := fields["sources"]; sourcesStr != "" {
		var sources []SrcItem
		if err := json.Unmarshal([]byte(sourcesStr), &sources); err == nil {
			meta.SrcList = sources
		}
	}

	// Parse frequencies JSON
	if freqsStr := fields["frequencies"]; freqsStr != "" {
		var freqs []FreqItem
		if err := json.Unmarshal([]byte(freqsStr), &freqs); err == nil {
			meta.FreqList = freqs
		}
	}

	// Infer audioType from audioType field or audioName extension
	if at := fields["audioType"]; at != "" {
		switch at {
		case "audio/mp4", "audio/m4a":
			meta.AudioType = "m4a"
		case "audio/wav", "audio/x-wav":
			meta.AudioType = "wav"
		case "audio/mpeg":
			meta.AudioType = "mp3"
		default:
			meta.AudioType = "m4a"
		}
	}

	return meta, nil
}

// parseOpenMHzFields parses OpenMHz upload form fields into AudioMetadata.
func parseOpenMHzFields(fields map[string]string) (*AudioMetadata, error) {
	tgStr := fields["talkgroup_num"]
	if tgStr == "" {
		return nil, fmt.Errorf("missing required field: talkgroup_num")
	}
	tg, err := strconv.Atoi(tgStr)
	if err != nil {
		return nil, fmt.Errorf("invalid talkgroup_num: %s", tgStr)
	}

	startStr := fields["start_time"]
	if startStr == "" {
		return nil, fmt.Errorf("missing required field: start_time")
	}
	startTime, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid start_time: %s", startStr)
	}

	meta := &AudioMetadata{
		Talkgroup: tg,
		StartTime: startTime,
	}

	if stopStr := fields["stop_time"]; stopStr != "" {
		if st, err := strconv.ParseInt(stopStr, 10, 64); err == nil {
			meta.StopTime = st
			meta.CallLength = int(st - startTime)
		}
	}

	if freqStr := fields["freq"]; freqStr != "" {
		if f, err := strconv.ParseFloat(freqStr, 64); err == nil {
			meta.Freq = f
		}
	}

	if ec := fields["error_count"]; ec != "" {
		if v, err := strconv.Atoi(ec); err == nil {
			meta.FreqError = v
		}
	}

	if sc := fields["spike_count"]; sc != "" {
		if v, err := strconv.Atoi(sc); err == nil {
			// Store in first FreqItem if present, else note for later
			_ = v
		}
	}

	if emStr := fields["emergency"]; emStr != "" {
		if v, err := strconv.Atoi(emStr); err == nil {
			meta.Emergency = v
		}
	}

	// Parse source_list JSON
	if srcStr := fields["source_list"]; srcStr != "" {
		var sources []SrcItem
		if err := json.Unmarshal([]byte(srcStr), &sources); err == nil {
			meta.SrcList = sources
		}
	}

	// Parse patch_list JSON (ignore for now, but parse to avoid losing data)
	// OpenMHz doesn't send short_name in the form — it's in the URL path.
	// The API handler will set ShortName from context or config.

	return meta, nil
}

// DetectUploadFormat returns "rdio-scanner" or "openmhz" based on form field names.
// Returns empty string if format cannot be determined.
func DetectUploadFormat(fieldNames []string) string {
	for _, name := range fieldNames {
		if name == "audio" || name == "audioName" || name == "systemLabel" {
			return "rdio-scanner"
		}
		if name == "call" || name == "talkgroup_num" || name == "start_time" {
			return "openmhz"
		}
	}
	return ""
}
```

**Step 4: Run tests**

```bash
go test ./internal/ingest/ -run TestParse -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/ingest/handler_upload.go internal/ingest/handler_upload_test.go
git commit -m "feat(ingest): add ProcessUploadedCall and format parsers for HTTP upload

Supports rdio-scanner and OpenMHz multipart form formats.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 3: Upload Auth Middleware

**Files:**
- Modify: `internal/api/middleware.go`
- Create: `internal/api/middleware_test.go` (or add to existing)

The standard `BearerAuth` middleware checks `Authorization` header and `?token=` query param. The upload endpoint also needs to accept `key` or `api_key` as multipart form fields. Create a wrapper middleware.

**Step 1: Write the test**

Add to `internal/api/middleware_test.go` (create if needed):

```go
func TestUploadAuth(t *testing.T) {
	token := "test-secret-token"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name       string
		setup      func(r *http.Request)
		wantStatus int
	}{
		{
			name: "bearer header",
			setup: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+token)
			},
			wantStatus: 200,
		},
		{
			name: "query param token",
			setup: func(r *http.Request) {
				r.URL.RawQuery = "token=" + token
			},
			wantStatus: 200,
		},
		{
			name: "no auth",
			setup: func(r *http.Request) {},
			wantStatus: 401,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/call-upload", nil)
			tt.setup(req)
			rec := httptest.NewRecorder()
			UploadAuth(token)(handler).ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
```

Note: Testing form field auth (`key`/`api_key` fields in multipart body) is complex because it requires constructing a real multipart body. The form field auth check will be integration-tested in Task 5. The unit test covers the header and query param paths that the middleware delegates to.

**Step 2: Run test to verify it fails**

```bash
go test ./internal/api/ -run TestUploadAuth -v
```

Expected: FAIL — `UploadAuth` doesn't exist.

**Step 3: Implement UploadAuth**

Add to `internal/api/middleware.go`:

```go
// UploadAuth is like BearerAuth but also accepts auth via form field "key" or "api_key"
// in multipart uploads. This avoids requiring TR upload plugins to set custom headers.
// Check order: Authorization header → ?token= query param → form field "key" → form field "api_key"
func UploadAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			// 1. Check Authorization header
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				if subtle.ConstantTimeCompare([]byte(auth[7:]), []byte(token)) == 1 {
					next.ServeHTTP(w, r)
					return
				}
			}

			// 2. Check ?token= query param
			if qt := r.URL.Query().Get("token"); qt != "" {
				if subtle.ConstantTimeCompare([]byte(qt), []byte(token)) == 1 {
					next.ServeHTTP(w, r)
					return
				}
			}

			// 3. Check form field "key" (rdio-scanner) or "api_key" (OpenMHz)
			// ParseMultipartForm is idempotent and safe to call before the handler reads the body.
			if err := r.ParseMultipartForm(32 << 20); err == nil {
				for _, fieldName := range []string{"key", "api_key"} {
					if val := r.FormValue(fieldName); val != "" {
						if subtle.ConstantTimeCompare([]byte(val), []byte(token)) == 1 {
							next.ServeHTTP(w, r)
							return
						}
					}
				}
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":"unauthorized"}`)
		})
	}
}
```

**Step 4: Run tests**

```bash
go test ./internal/api/ -run TestUploadAuth -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/middleware.go internal/api/middleware_test.go
git commit -m "feat(api): add UploadAuth middleware for form field auth

Accepts Bearer header, ?token= query param, or multipart form field
key/api_key. Used by the call upload endpoint.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 4: Upload API Handler

**Files:**
- Create: `internal/api/upload.go`
- Modify: `internal/api/server.go`

The HTTP handler that receives multipart uploads, detects the format, parses to AudioMetadata, and calls `ProcessUploadedCall`.

**Step 1: Create the handler**

Create `internal/api/upload.go`:

```go
package api

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/snarg/tr-engine/internal/ingest"
)

// UploadHandler handles HTTP call uploads compatible with rdio-scanner and OpenMHz.
type UploadHandler struct {
	pipeline   *ingest.Pipeline
	instanceID string
}

// NewUploadHandler creates a new upload handler.
func NewUploadHandler(pipeline *ingest.Pipeline, instanceID string) *UploadHandler {
	return &UploadHandler{
		pipeline:   pipeline,
		instanceID: instanceID,
	}
}

// Upload handles POST /api/v1/call-upload.
func (h *UploadHandler) Upload(w http.ResponseWriter, r *http.Request) {
	// Limit upload size (50 MB should cover any call audio)
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form: " + err.Error()})
		return
	}
	defer r.MultipartForm.RemoveAll()

	// Detect format from form field names
	var fieldNames []string
	for k := range r.MultipartForm.Value {
		fieldNames = append(fieldNames, k)
	}
	for k := range r.MultipartForm.File {
		fieldNames = append(fieldNames, k)
	}

	format := ingest.DetectUploadFormat(fieldNames)
	if format == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]string{
			"error": "unrecognized upload format: expected rdio-scanner or OpenMHz fields",
		})
		return
	}

	// Extract flat form values into a map
	fields := make(map[string]string)
	for k, v := range r.MultipartForm.Value {
		if len(v) > 0 {
			fields[k] = v[0]
		}
	}

	// Parse metadata based on format
	var meta *ingest.AudioMetadata
	var audioFieldName string
	var err error

	switch format {
	case "rdio-scanner":
		meta, err = ingest.ParseRdioScannerFields(fields)
		audioFieldName = "audio"
	case "openmhz":
		meta, err = ingest.ParseOpenMHzFields(fields)
		audioFieldName = "call"
		// OpenMHz doesn't include short_name in form fields.
		// If still empty, use the instanceID as fallback.
		if meta != nil && meta.ShortName == "" {
			meta.ShortName = h.instanceID
		}
	}

	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// Read audio file
	var audioData []byte
	var audioFilename string
	if file, header, fileErr := r.FormFile(audioFieldName); fileErr == nil {
		defer file.Close()
		audioData, err = io.ReadAll(file)
		if err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read audio file"})
			return
		}
		audioFilename = header.Filename

		// Infer audio type from filename if not set in metadata
		if meta.AudioType == "" {
			ext := strings.TrimPrefix(filepath.Ext(audioFilename), ".")
			if ext != "" {
				meta.AudioType = ext
			} else {
				meta.AudioType = "m4a"
			}
		}
	}

	// Process the uploaded call
	result, err := h.pipeline.ProcessUploadedCall(r.Context(), h.instanceID, meta, audioData, audioFilename)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate call") {
			WriteJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	WriteJSON(w, http.StatusCreated, result)
}
```

**Step 2: Register the route in server.go**

In `internal/api/server.go`, find the `r.Route("/api/v1", ...)` block. Add the upload handler in its own group with `UploadAuth` middleware, before the group that applies standard `BearerAuth`:

Find the existing pattern (around line 81):
```go
r.Route("/api/v1", func(r chi.Router) {
```

The upload route needs to be in a separate `r.Group` with `UploadAuth` instead of `BearerAuth`. Restructure the `/api/v1` block:

```go
r.Route("/api/v1", func(r chi.Router) {
	// Upload endpoint with custom auth (accepts form field key/api_key)
	if opts.Live != nil {
		uploadHandler := NewUploadHandler(opts.Live.(*ingest.Pipeline), opts.Config.UploadInstanceID)
		r.Group(func(r chi.Router) {
			r.Use(UploadAuth(opts.Config.AuthToken))
			r.Post("/call-upload", uploadHandler.Upload)
		})
	}

	// All other routes with standard Bearer auth
	r.Group(func(r chi.Router) {
		r.Use(BearerAuth(opts.Config.AuthToken))
		// ... existing handler registrations unchanged ...
	})
})
```

Note: `opts.Live` is a `LiveDataSource` interface. The upload handler needs the concrete `*Pipeline` type for `ProcessUploadedCall`. Use a type assertion. If `opts.Live` is nil (no ingest pipeline configured), the upload endpoint is not registered.

**Step 3: Verify build**

```bash
go build ./...
```

**Step 4: Commit**

```bash
git add internal/api/upload.go internal/api/server.go
git commit -m "feat(api): add POST /call-upload endpoint for rdio-scanner and OpenMHz

Auto-detects format from form field names. Parses to AudioMetadata
and calls ProcessUploadedCall for identity resolution, call creation,
audio saving, and SSE event publishing.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 5: Integration Test

**Files:**
- Create: `internal/api/upload_test.go`

Test the full upload flow with a mock pipeline. This verifies format detection, field parsing, auth, and error handling at the HTTP level.

**Step 1: Write the integration tests**

Create `internal/api/upload_test.go`:

```go
package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/snarg/tr-engine/internal/ingest"
)

func TestDetectUploadFormat(t *testing.T) {
	tests := []struct {
		name   string
		fields []string
		want   string
	}{
		{"rdio-scanner by audio field", []string{"audio", "talkgroup", "dateTime"}, "rdio-scanner"},
		{"rdio-scanner by systemLabel", []string{"systemLabel", "talkgroup"}, "rdio-scanner"},
		{"openmhz by call field", []string{"call", "talkgroup_num", "freq"}, "openmhz"},
		{"openmhz by talkgroup_num", []string{"talkgroup_num", "start_time"}, "openmhz"},
		{"unknown format", []string{"foo", "bar"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ingest.DetectUploadFormat(tt.fields)
			if got != tt.want {
				t.Errorf("DetectUploadFormat = %q, want %q", got, tt.want)
			}
		})
	}
}

func buildRdioScannerForm(t *testing.T, fields map[string]string, audioData []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	for k, v := range fields {
		writer.WriteField(k, v)
	}

	if audioData != nil {
		part, err := writer.CreateFormFile("audio", "test.m4a")
		if err != nil {
			t.Fatal(err)
		}
		part.Write(audioData)
	}

	writer.Close()
	return body, writer.FormDataContentType()
}

func buildOpenMHzForm(t *testing.T, fields map[string]string, audioData []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	for k, v := range fields {
		writer.WriteField(k, v)
	}

	if audioData != nil {
		part, err := writer.CreateFormFile("call", "test.m4a")
		if err != nil {
			t.Fatal(err)
		}
		part.Write(audioData)
	}

	writer.Close()
	return body, writer.FormDataContentType()
}

func TestUploadEndpoint_BadFormat(t *testing.T) {
	handler := NewUploadHandler(nil, "test")

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("foo", "bar")
	writer.Close()

	req := httptest.NewRequest("POST", "/api/v1/call-upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	handler.Upload(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var resp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] == "" {
		t.Error("expected error message in response")
	}
}

func TestUploadEndpoint_MissingFields(t *testing.T) {
	handler := NewUploadHandler(nil, "test")

	// rdio-scanner format missing required talkgroup
	body, ct := buildRdioScannerForm(t, map[string]string{
		"systemLabel": "butco",
		"dateTime":    "1708881234",
		// missing talkgroup
	}, []byte("fake-audio"))

	req := httptest.NewRequest("POST", "/api/v1/call-upload", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()

	handler.Upload(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUploadAuth_FormFieldKey(t *testing.T) {
	token := "test-secret"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// rdio-scanner "key" field
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("key", token)
	writer.WriteField("talkgroup", "9044")
	writer.Close()

	req := httptest.NewRequest("POST", "/api/v1/call-upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	UploadAuth(token)(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (form field key auth)", rec.Code, http.StatusOK)
	}

	// OpenMHz "api_key" field
	body2 := &bytes.Buffer{}
	writer2 := multipart.NewWriter(body2)
	writer2.WriteField("api_key", token)
	writer2.WriteField("talkgroup_num", "9044")
	writer2.Close()

	req2 := httptest.NewRequest("POST", "/api/v1/call-upload", body2)
	req2.Header.Set("Content-Type", writer2.FormDataContentType())
	rec2 := httptest.NewRecorder()

	UploadAuth(token)(handler).ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (form field api_key auth)", rec2.Code, http.StatusOK)
	}
}
```

**Step 2: Run tests**

```bash
go test ./internal/api/ -run TestUpload -v
go test ./internal/api/ -run TestDetect -v
```

Expected: PASS

**Step 3: Commit**

```bash
git add internal/api/upload_test.go
git commit -m "test(api): integration tests for call upload endpoint

Tests format detection, field parsing, auth (Bearer, query, form fields),
and error handling.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 6: OpenAPI Spec + README

**Files:**
- Modify: `openapi.yaml`
- Modify: `README.md`

Document the new endpoint.

**Step 1: Add to openapi.yaml**

Add a new path entry for `/call-upload` in the paths section. Follow the existing pattern. Key fields:
- Summary: "Upload a call recording"
- Description: "Accepts multipart uploads compatible with rdio-scanner and OpenMHz trunk-recorder plugins. Auto-detects format from form field names."
- Request body: multipart/form-data with rdio-scanner and OpenMHz field descriptions
- Responses: 201 Created, 400 Bad Request, 401 Unauthorized, 409 Conflict

**Step 2: Update README.md**

Add to the API Endpoints table:
```
| `POST /call-upload` | Upload call recording (rdio-scanner/OpenMHz compatible) |
```

Add to the Ingest Modes section a fourth bullet:
```markdown
- **HTTP Upload** (`POST /api/v1/call-upload`) — accepts multipart call uploads compatible with
  trunk-recorder's rdio-scanner and OpenMHz upload plugins. Point TR's upload plugin at your
  tr-engine instance. Produces `call_end` events with audio.
```

Add to the Data Flow diagram:
```
trunk-recorder  ──MQTT──>  broker  ──MQTT──>  tr-engine  ──REST/SSE──>  clients
      |                                            |
      +──audio files──>  fsnotify watcher ─────────+
      |                                            |
      +──HTTP upload──>  POST /call-upload ────────+
                                                   v
                                               PostgreSQL
```

**Step 3: Update Changelog**

Add to the v0.8.5 (or v0.9.0) section:
```markdown
- **HTTP call upload** — `POST /api/v1/call-upload` accepts multipart uploads compatible with
  trunk-recorder's rdio-scanner and OpenMHz upload plugins. Third ingest path alongside MQTT
  and file-watch — no local audio capture or MQTT broker required.
```

**Step 4: Commit**

```bash
git add openapi.yaml README.md
git commit -m "docs: add call upload endpoint to API spec, README, and changelog

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 7: Deploy + End-to-End Test

**Step 1: Build and deploy**

```bash
./deploy-dev.sh --binary-only
```

**Step 2: Test with curl (rdio-scanner format)**

```bash
curl -X POST https://tr-engine.luxprimatech.com/api/v1/call-upload \
  -F "key=AUTH_TOKEN_VALUE" \
  -F "audio=@test-audio.m4a" \
  -F "audioName=test.m4a" \
  -F "audioType=audio/mp4" \
  -F "talkgroup=9044" \
  -F "frequency=859262500" \
  -F "dateTime=1708881234" \
  -F "system=1" \
  -F "systemLabel=butco" \
  -F 'sources=[{"pos":0.00,"src":924003,"tag":"Unit1"}]'
```

Expected: 201 Created with call_id

**Step 3: Test with curl (OpenMHz format)**

```bash
curl -X POST https://tr-engine.luxprimatech.com/api/v1/call-upload \
  -F "api_key=AUTH_TOKEN_VALUE" \
  -F "call=@test-audio.m4a" \
  -F "talkgroup_num=9044" \
  -F "freq=859262500" \
  -F "start_time=1708881300" \
  -F "stop_time=1708881342" \
  -F 'source_list=[{"pos":0.00,"src":924003,"tag":"Unit1"}]'
```

Expected: 201 Created with call_id

**Step 4: Verify the calls appear**

```bash
curl https://tr-engine.luxprimatech.com/api/v1/calls?sort=-start_time&limit=5
```

Verify the uploaded calls appear with correct metadata.

**Step 5: Test duplicate rejection**

Re-run one of the curl commands above. Expected: 409 Conflict.

**Step 6: Test auth rejection**

```bash
curl -X POST https://tr-engine.luxprimatech.com/api/v1/call-upload \
  -F "key=wrong-token" \
  -F "talkgroup=9044" \
  -F "dateTime=1708881234" \
  -F "systemLabel=butco"
```

Expected: 401 Unauthorized.

**Step 7: Final commit + push**

```bash
git push
```
