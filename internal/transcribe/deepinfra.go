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
	Text     string          `json:"text"`
	Language string          `json:"language"`
	Duration float64         `json:"duration"`
	Words    []deepInfraWord `json:"words"`
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
