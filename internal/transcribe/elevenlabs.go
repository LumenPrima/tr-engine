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
	"strings"
	"time"
)

const elevenLabsSTTEndpoint = "https://api.elevenlabs.io/v1/speech-to-text"

// ElevenLabsClient calls the ElevenLabs Speech-to-Text API.
// Implements the Provider interface.
type ElevenLabsClient struct {
	apiKey   string
	model    string // "scribe_v1" or "scribe_v2"
	keyterms string // comma-separated boost terms
	timeout  time.Duration
	client   *http.Client
}

// elevenlabsResponse is the JSON response from the ElevenLabs STT API.
type elevenlabsResponse struct {
	LanguageCode       string             `json:"language_code"`
	LanguageProbability float64           `json:"language_probability"`
	Text               string             `json:"text"`
	Words              []elevenlabsWord   `json:"words"`
}

// elevenlabsWord is a word or spacing entry from ElevenLabs.
type elevenlabsWord struct {
	Text        string  `json:"text"`
	Type        string  `json:"type"` // "word" or "spacing"
	StartTimeMs float64 `json:"start_time_ms"`
	EndTimeMs   float64 `json:"end_time_ms"`
}

// NewElevenLabsClient creates a new ElevenLabs STT client.
func NewElevenLabsClient(apiKey, model, keyterms string, timeout time.Duration) *ElevenLabsClient {
	return &ElevenLabsClient{
		apiKey:   apiKey,
		model:    model,
		keyterms: keyterms,
		timeout:  timeout,
		client:   &http.Client{Timeout: timeout},
	}
}

// Name returns the provider name.
func (el *ElevenLabsClient) Name() string { return "elevenlabs" }

// Model returns the configured model identifier.
func (el *ElevenLabsClient) Model() string { return el.model }

// Transcribe sends an audio file to the ElevenLabs STT API and returns the result.
func (el *ElevenLabsClient) Transcribe(ctx context.Context, audioPath string, opts TranscribeOpts) (*Response, error) {
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

	// Model ID
	w.WriteField("model_id", el.model)

	// Language code (ElevenLabs uses ISO-639 codes like Whisper)
	lang := opts.Language
	if lang == "" {
		lang = "en"
	}
	w.WriteField("language_code", lang)

	// Always request word-level timestamps
	w.WriteField("timestamps_granularity", "word")

	// Keyterms: ElevenLabs accepts a JSON array of objects with {"text": "term", "weight": 1.0}.
	// Build from the comma-separated config string and/or per-request hotwords.
	keyterms := el.buildKeyterms(opts.Hotwords)
	if keyterms != "" {
		w.WriteField("keyterms", keyterms)
	}

	w.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, elevenLabsSTTEndpoint, &buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("xi-api-key", el.apiKey)

	resp, err := el.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("elevenlabs API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result elevenlabsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Convert to common Word type, filtering out spacing entries
	var words []Word
	for _, ew := range result.Words {
		if ew.Type != "word" {
			continue
		}
		words = append(words, Word{
			Word:  ew.Text,
			Start: ew.StartTimeMs / 1000.0,
			End:   ew.EndTimeMs / 1000.0,
		})
	}

	return &Response{
		Text:     result.Text,
		Language: result.LanguageCode,
		Words:    words,
	}, nil
}

// buildKeyterms merges config-level keyterms with per-request hotwords into a
// JSON array of {"text": "term"} objects for the ElevenLabs API.
func (el *ElevenLabsClient) buildKeyterms(hotwords string) string {
	var terms []string

	if el.keyterms != "" {
		for _, t := range strings.Split(el.keyterms, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				terms = append(terms, t)
			}
		}
	}

	if hotwords != "" {
		for _, t := range strings.Split(hotwords, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				terms = append(terms, t)
			}
		}
	}

	if len(terms) == 0 {
		return ""
	}

	type keyterm struct {
		Text string `json:"text"`
	}
	arr := make([]keyterm, len(terms))
	for i, t := range terms {
		arr[i] = keyterm{Text: t}
	}
	b, _ := json.Marshal(arr)
	return string(b)
}
