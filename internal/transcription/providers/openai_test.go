package providers

import (
	"testing"

	"go.uber.org/zap"
)

func TestOpenAI_URLConstruction(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		wantURL  string
	}{
		{
			name:    "empty base URL uses OpenAI default",
			baseURL: "",
			wantURL: "https://api.openai.com/v1/audio/transcriptions",
		},
		{
			name:    "speaches-ai root URL",
			baseURL: "http://localhost:8000",
			wantURL: "http://localhost:8000/v1/audio/transcriptions",
		},
		{
			name:    "speaches-ai with trailing slash",
			baseURL: "http://localhost:8000/",
			wantURL: "http://localhost:8000/v1/audio/transcriptions",
		},
		{
			name:    "speaches-ai with v1 path",
			baseURL: "http://localhost:8000/v1",
			wantURL: "http://localhost:8000/v1/audio/transcriptions",
		},
		{
			name:    "speaches-ai with v1 path and trailing slash",
			baseURL: "http://localhost:8000/v1/",
			wantURL: "http://localhost:8000/v1/audio/transcriptions",
		},
		{
			name:    "full URL already specified",
			baseURL: "http://localhost:8000/v1/audio/transcriptions",
			wantURL: "http://localhost:8000/v1/audio/transcriptions",
		},
		{
			name:    "Groq API",
			baseURL: "https://api.groq.com/openai/v1",
			wantURL: "https://api.groq.com/openai/v1/audio/transcriptions",
		},
		{
			name:    "Together.ai API",
			baseURL: "https://api.together.xyz/v1",
			wantURL: "https://api.together.xyz/v1/audio/transcriptions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL := buildAPIURL(tt.baseURL)
			if gotURL != tt.wantURL {
				t.Errorf("buildAPIURL(%q) = %q, want %q", tt.baseURL, gotURL, tt.wantURL)
			}
		})
	}
}

// buildAPIURL is extracted for testing - mirrors the logic in Transcribe
func buildAPIURL(baseURL string) string {
	if baseURL == "" {
		return defaultOpenAIURL
	}
	apiURL := baseURL
	// Remove trailing slash for consistent handling
	for len(apiURL) > 0 && apiURL[len(apiURL)-1] == '/' {
		apiURL = apiURL[:len(apiURL)-1]
	}
	// Append the transcriptions endpoint if not already present
	if len(apiURL) < 21 || apiURL[len(apiURL)-21:] != "/audio/transcriptions" {
		if len(apiURL) >= 3 && apiURL[len(apiURL)-3:] == "/v1" {
			apiURL += "/audio/transcriptions"
		} else {
			apiURL += "/v1/audio/transcriptions"
		}
	}
	return apiURL
}

func TestNewOpenAI_RequiresAPIKey(t *testing.T) {
	logger := zap.NewNop()

	_, err := NewOpenAI(OpenAIConfig{
		APIKey: "",
	}, logger)

	if err == nil {
		t.Error("NewOpenAI() should require API key")
	}
}

func TestNewOpenAI_DefaultModel(t *testing.T) {
	logger := zap.NewNop()

	provider, err := NewOpenAI(OpenAIConfig{
		APIKey: "test-key",
	}, logger)

	if err != nil {
		t.Fatalf("NewOpenAI() error = %v", err)
	}

	if provider.config.Model != "whisper-1" {
		t.Errorf("Default model = %q, want %q", provider.config.Model, "whisper-1")
	}
}
