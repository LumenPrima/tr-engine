package providers

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

	"go.uber.org/zap"
)

const (
	defaultOpenAIURL = "https://api.openai.com/v1/audio/transcriptions"
)

// OpenAIConfig holds configuration for the OpenAI Whisper API provider
type OpenAIConfig struct {
	APIKey  string
	BaseURL string // Empty = OpenAI default, or custom for compatible APIs (Groq, Together, etc.)
	Model   string
	Prompt  string // Optional prompt to guide transcription (terminology, context)
}

// OpenAIProvider implements transcription using OpenAI's Whisper API
// Also compatible with Groq, Together.ai, and other OpenAI-compatible APIs
type OpenAIProvider struct {
	config OpenAIConfig
	client *http.Client
	logger *zap.Logger
}

// OpenAIResult represents the response from OpenAI's transcription API
type OpenAIResult struct {
	Text string `json:"text"`
}

// OpenAIWord represents a word with timing in the API response
type OpenAIWord struct {
	Word  string  `json:"word"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// OpenAIVerboseResult represents verbose_json response format
type OpenAIVerboseResult struct {
	Text     string       `json:"text"`
	Language string       `json:"language"`
	Duration float64      `json:"duration"`
	Words    []OpenAIWord `json:"words"`
}

// NewOpenAI creates a new OpenAI transcription provider
func NewOpenAI(cfg OpenAIConfig, logger *zap.Logger) (*OpenAIProvider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required")
	}

	if cfg.Model == "" {
		cfg.Model = "whisper-1"
	}

	return &OpenAIProvider{
		config: cfg,
		client: &http.Client{
			Timeout: 120 * time.Second, // Transcription can take a while for longer files
		},
		logger: logger,
	}, nil
}

// Name returns the provider name
func (p *OpenAIProvider) Name() string {
	if p.config.BaseURL != "" {
		return "openai-compatible"
	}
	return "openai"
}

// SupportedFormats returns supported audio formats
func (p *OpenAIProvider) SupportedFormats() []string {
	// OpenAI Whisper supports these formats
	return []string{"mp3", "mp4", "mpeg", "mpga", "m4a", "wav", "webm"}
}

// Transcribe sends the audio file to the OpenAI Whisper API
func (p *OpenAIProvider) Transcribe(ctx context.Context, audioPath, language string) (*ProviderResult, error) {
	startTime := time.Now()

	// Open the audio file
	file, err := os.Open(audioPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer file.Close()

	// Check file size
	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat audio file: %w", err)
	}

	// OpenAI has a 25MB limit
	if stat.Size() > 25*1024*1024 {
		return nil, fmt.Errorf("audio file too large: %d bytes (max 25MB)", stat.Size())
	}

	// Create multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add the audio file
	part, err := writer.CreateFormFile("file", filepath.Base(audioPath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("failed to copy file content: %w", err)
	}

	// Add model
	if err := writer.WriteField("model", p.config.Model); err != nil {
		return nil, fmt.Errorf("failed to write model field: %w", err)
	}

	// Add language if specified
	if language != "" {
		if err := writer.WriteField("language", language); err != nil {
			return nil, fmt.Errorf("failed to write language field: %w", err)
		}
	}

	// Request verbose output with word-level timestamps
	if err := writer.WriteField("response_format", "verbose_json"); err != nil {
		return nil, fmt.Errorf("failed to write response_format field: %w", err)
	}

	// Request word-level timestamps
	if err := writer.WriteField("timestamp_granularities[]", "word"); err != nil {
		return nil, fmt.Errorf("failed to write timestamp_granularities field: %w", err)
	}

	// Add prompt if configured (helps with domain-specific terminology)
	if p.config.Prompt != "" {
		if err := writer.WriteField("prompt", p.config.Prompt); err != nil {
			return nil, fmt.Errorf("failed to write prompt field: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Determine API URL
	apiURL := defaultOpenAIURL
	if p.config.BaseURL != "" {
		apiURL = p.config.BaseURL
		// Remove trailing slash for consistent handling
		apiURL = strings.TrimSuffix(apiURL, "/")
		// Append the transcriptions endpoint if not already present
		if !strings.HasSuffix(apiURL, "/audio/transcriptions") {
			if strings.HasSuffix(apiURL, "/v1") {
				apiURL += "/audio/transcriptions"
			} else {
				apiURL += "/v1/audio/transcriptions"
			}
		}
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send request
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for errors
	if resp.StatusCode != http.StatusOK {
		p.logger.Error("OpenAI API error",
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(respBody)),
		)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response - try verbose format first
	var verboseResult OpenAIVerboseResult
	if err := json.Unmarshal(respBody, &verboseResult); err != nil {
		// Fall back to simple format
		var simpleResult OpenAIResult
		if err := json.Unmarshal(respBody, &simpleResult); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		verboseResult.Text = simpleResult.Text
	}

	duration := int(time.Since(startTime).Milliseconds())

	// Convert OpenAI words to our Word type
	var words []Word
	for _, w := range verboseResult.Words {
		words = append(words, Word{
			Word:  w.Word,
			Start: float32(w.Start),
			End:   float32(w.End),
		})
	}

	p.logger.Debug("Transcription completed",
		zap.String("path", audioPath),
		zap.Int("duration_ms", duration),
		zap.Int("text_length", len(verboseResult.Text)),
		zap.Int("word_count", len(words)),
	)

	return &ProviderResult{
		Text:     verboseResult.Text,
		Language: verboseResult.Language,
		Duration: duration,
		Model:    p.config.Model,
		Words:    words,
	}, nil
}

// Close releases resources
func (p *OpenAIProvider) Close() error {
	return nil
}
