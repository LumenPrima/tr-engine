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
	"time"

	"go.uber.org/zap"
)

// HTTPConfig holds configuration for self-hosted HTTP Whisper servers
type HTTPConfig struct {
	URL     string // API endpoint URL (e.g., "http://localhost:9000/asr")
	APIKey  string // Optional API key for authentication
	Timeout int    // Request timeout in seconds
}

// HTTPProvider implements transcription using self-hosted HTTP Whisper servers
// Compatible with faster-whisper, whisper-asr-webservice, and similar servers
type HTTPProvider struct {
	config HTTPConfig
	client *http.Client
	logger *zap.Logger
}

// HTTPResponse represents the common response format from Whisper HTTP servers
type HTTPResponse struct {
	Text     string  `json:"text"`
	Language string  `json:"language,omitempty"`
	Duration float64 `json:"duration,omitempty"`
}

// NewHTTP creates a new HTTP transcription provider
func NewHTTP(cfg HTTPConfig, logger *zap.Logger) (*HTTPProvider, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("HTTP transcription URL is required")
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 // Default 60 seconds
	}

	return &HTTPProvider{
		config: cfg,
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
		logger: logger,
	}, nil
}

// Name returns the provider name
func (p *HTTPProvider) Name() string {
	return "http"
}

// SupportedFormats returns supported audio formats
func (p *HTTPProvider) SupportedFormats() []string {
	// Most Whisper servers support these formats
	return []string{"wav", "mp3", "m4a", "mp4", "webm", "flac", "ogg"}
}

// Transcribe sends the audio file to the self-hosted Whisper server
func (p *HTTPProvider) Transcribe(ctx context.Context, audioPath, language string) (*ProviderResult, error) {
	startTime := time.Now()

	// Open the audio file
	file, err := os.Open(audioPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer file.Close()

	// Create multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add the audio file
	// Try different field names as different servers use different conventions
	part, err := writer.CreateFormFile("audio_file", filepath.Base(audioPath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("failed to copy file content: %w", err)
	}

	// Add language if specified
	if language != "" {
		// Try common parameter names
		writer.WriteField("language", language)
		writer.WriteField("lang", language)
	}

	// Request plain text output (most compatible)
	writer.WriteField("output", "json")
	writer.WriteField("response_format", "json")

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", p.config.URL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Add API key if configured
	if p.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
		req.Header.Set("X-API-Key", p.config.APIKey) // Some servers use this header
	}

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
		p.logger.Error("HTTP Whisper server error",
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(respBody)),
		)
		return nil, fmt.Errorf("server error %d: %s", resp.StatusCode, string(respBody))
	}

	duration := int(time.Since(startTime).Milliseconds())

	// Try to parse as JSON first
	var result HTTPResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		// If JSON parsing fails, treat the response as plain text
		result.Text = string(respBody)
	}

	// Handle responses that return text in different formats
	if result.Text == "" {
		// Some servers return {"transcription": "text"} or similar
		var altResult map[string]interface{}
		if err := json.Unmarshal(respBody, &altResult); err == nil {
			for _, key := range []string{"transcription", "transcript", "result", "text"} {
				if val, ok := altResult[key].(string); ok && val != "" {
					result.Text = val
					break
				}
			}
		}
	}

	// If still empty, use raw response
	if result.Text == "" {
		result.Text = string(respBody)
	}

	p.logger.Debug("HTTP transcription completed",
		zap.String("path", audioPath),
		zap.Int("duration_ms", duration),
		zap.Int("text_length", len(result.Text)),
	)

	return &ProviderResult{
		Text:     result.Text,
		Language: result.Language,
		Duration: duration,
		Model:    "http-whisper",
	}, nil
}

// Close releases resources
func (p *HTTPProvider) Close() error {
	return nil
}
