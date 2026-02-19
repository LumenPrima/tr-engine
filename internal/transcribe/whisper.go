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
// Zero-value fields are omitted from the request, preserving backward
// compatibility with servers that ignore unknown form fields (e.g. speaches).
type TranscribeOpts struct {
	// Standard OpenAI params
	Temperature float64
	Language    string
	Prompt      string // initial_prompt / domain vocabulary
	Hotwords    string // vocabulary boost terms

	// Decoding
	BeamSize int // 0 = server default (typically 5)

	// Anti-hallucination
	RepetitionPenalty           float64 // >1.0 penalizes repetition (0 = omit)
	NoRepeatNgramSize           int     // block n-gram repetition (0 = disabled)
	ConditionOnPreviousText     *bool   // nil = omit (server default); false = prevent cascading
	NoSpeechThreshold           float64 // 0 = omit (server default ~0.6)
	HallucinationSilenceThreshold float64 // 0 = omit/disabled
	MaxNewTokens                int     // 0 = omit/unlimited

	// VAD
	VadFilter bool
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
// Uses multipart/form-data. Only non-default parameters are sent, so this
// works with speaches, the custom whisper-server, or any OpenAI-compatible endpoint.
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

	// --- Extended parameters (only sent when non-default) ---

	if opts.Prompt != "" {
		w.WriteField("prompt", opts.Prompt)
	}

	if opts.Hotwords != "" {
		w.WriteField("hotwords", opts.Hotwords)
	}

	if opts.BeamSize > 0 {
		w.WriteField("beam_size", fmt.Sprintf("%d", opts.BeamSize))
	}

	if opts.RepetitionPenalty > 0 && opts.RepetitionPenalty != 1.0 {
		w.WriteField("repetition_penalty", fmt.Sprintf("%.2f", opts.RepetitionPenalty))
	}

	if opts.NoRepeatNgramSize > 0 {
		w.WriteField("no_repeat_ngram_size", fmt.Sprintf("%d", opts.NoRepeatNgramSize))
	}

	if opts.ConditionOnPreviousText != nil {
		if *opts.ConditionOnPreviousText {
			w.WriteField("condition_on_previous_text", "true")
		} else {
			w.WriteField("condition_on_previous_text", "false")
		}
	}

	if opts.NoSpeechThreshold > 0 {
		w.WriteField("no_speech_threshold", fmt.Sprintf("%.2f", opts.NoSpeechThreshold))
	}

	if opts.HallucinationSilenceThreshold > 0 {
		w.WriteField("hallucination_silence_threshold", fmt.Sprintf("%.2f", opts.HallucinationSilenceThreshold))
	}

	if opts.MaxNewTokens > 0 {
		w.WriteField("max_new_tokens", fmt.Sprintf("%d", opts.MaxNewTokens))
	}

	if opts.VadFilter {
		w.WriteField("vad_filter", "true")
	}

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
