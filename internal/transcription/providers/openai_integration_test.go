package providers

import (
	"context"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
)

// Integration tests for OpenAI provider
// Run with: go test -v -run Integration ./internal/transcription/providers/...
//
// Environment variables:
//   OPENAI_API_KEY       - OpenAI API key (or any key for speaches-ai)
//   OPENAI_BASE_URL      - Base URL (empty for OpenAI, or http://localhost:8000 for speaches-ai)
//   OPENAI_MODEL         - Model to use (default: whisper-1)
//   TEST_AUDIO_FILE      - Path to test audio file

func TestOpenAI_Integration(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping integration test")
	}

	audioFile := os.Getenv("TEST_AUDIO_FILE")
	if audioFile == "" {
		t.Skip("TEST_AUDIO_FILE not set, skipping integration test")
	}

	// Check audio file exists
	if _, err := os.Stat(audioFile); os.IsNotExist(err) {
		t.Skipf("Audio file not found: %s", audioFile)
	}

	logger := zap.NewNop()

	baseURL := os.Getenv("OPENAI_BASE_URL")
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "whisper-1"
	}

	provider, err := NewOpenAI(OpenAIConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
	}, logger)
	if err != nil {
		t.Fatalf("NewOpenAI() error = %v", err)
	}
	defer provider.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	t.Logf("Testing with base_url=%q model=%q", baseURL, model)

	result, err := provider.Transcribe(ctx, audioFile, "en")
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}

	t.Logf("Result: duration=%dms text=%q", result.Duration, result.Text)

	if result.Text == "" {
		t.Error("Transcribe() returned empty text")
	}
}

// Example test commands:
//
// Test with speaches-ai:
//   OPENAI_API_KEY=dummy \
//   OPENAI_BASE_URL=http://localhost:8000 \
//   OPENAI_MODEL=deepdml/faster-whisper-large-v3-turbo-ct2 \
//   TEST_AUDIO_FILE=/path/to/audio.wav \
//   go test -v -run Integration ./internal/transcription/providers/...
//
// Test with OpenAI:
//   OPENAI_API_KEY=sk-xxx \
//   OPENAI_MODEL=whisper-1 \
//   TEST_AUDIO_FILE=/path/to/audio.wav \
//   go test -v -run Integration ./internal/transcription/providers/...
