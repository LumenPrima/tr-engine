package transcription

import (
	"fmt"

	"github.com/trunk-recorder/tr-engine/internal/config"
	"github.com/trunk-recorder/tr-engine/internal/transcription/providers"
	"go.uber.org/zap"
)

// Provider is an alias for the providers.Provider interface
type Provider = providers.Provider

// Result is an alias for the providers.ProviderResult type
type Result = providers.ProviderResult

// NewProvider creates a transcription provider based on configuration
func NewProvider(cfg config.TranscriptionConfig, audioBasePath string, logger *zap.Logger) (Provider, error) {
	switch cfg.Provider {
	case "openai":
		return providers.NewOpenAI(providers.OpenAIConfig{
			APIKey:  cfg.OpenAI.APIKey,
			BaseURL: cfg.OpenAI.BaseURL,
			Model:   cfg.OpenAI.Model,
			Prompt:  cfg.OpenAI.Prompt,
		}, logger)

	case "http":
		return providers.NewHTTP(providers.HTTPConfig{
			URL:     cfg.HTTP.URL,
			APIKey:  cfg.HTTP.APIKey,
			Timeout: cfg.HTTP.Timeout,
		}, logger)

	case "embedded":
		return nil, fmt.Errorf("embedded provider requires CGO and whisper.cpp bindings (not yet implemented)")

	default:
		return nil, fmt.Errorf("unknown transcription provider: %s", cfg.Provider)
	}
}
