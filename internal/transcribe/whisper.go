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

// WhisperClient calls an OpenAI-compatible /v1/audio/transcriptions endpoint.
type WhisperClient struct {
	url     string
	model   string
	timeout time.Duration
	client  *http.Client
}

// TranscribeOpts are per-request options for the Whisper API.
type TranscribeOpts struct {
	Temperature float64
	Language    string
}

// WhisperResponse is the parsed response from the Whisper API (verbose_json format).
type WhisperResponse struct {
	Text     string        `json:"text"`
	Language string        `json:"language"`
	Duration float64       `json:"duration"`
	Words    []WhisperWord `json:"words"`
}

// WhisperWord is a word with start/end timestamps from Whisper.
type WhisperWord struct {
	Word  string  `json:"word"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// NewWhisperClient creates a new Whisper HTTP client.
func NewWhisperClient(url, model string, timeout time.Duration) *WhisperClient {
	return &WhisperClient{
		url:     url,
		model:   model,
		timeout: timeout,
		client:  &http.Client{Timeout: timeout},
	}
}

// Transcribe sends an audio file to the Whisper API and returns the result.
// Uses multipart/form-data with: file, model, language, temperature,
// response_format=verbose_json, timestamp_granularities[]=word.
func (wc *WhisperClient) Transcribe(ctx context.Context, audioPath string, opts TranscribeOpts) (*WhisperResponse, error) {
	f, err := os.Open(audioPath)
	if err != nil {
		return nil, fmt.Errorf("open audio file: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Audio file field
	part, err := w.CreateFormFile("file", filepath.Base(audioPath))
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, fmt.Errorf("copy audio data: %w", err)
	}

	// Model
	if wc.model != "" {
		w.WriteField("model", wc.model)
	}

	// Language
	lang := opts.Language
	if lang == "" {
		lang = "en"
	}
	w.WriteField("language", lang)

	// Temperature
	w.WriteField("temperature", fmt.Sprintf("%.2f", opts.Temperature))

	// Response format: verbose_json for word-level timestamps
	w.WriteField("response_format", "verbose_json")

	// Request word-level timestamps
	w.WriteField("timestamp_granularities[]", "word")

	w.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wc.url, &buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := wc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("whisper request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("whisper API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result WhisperResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}
