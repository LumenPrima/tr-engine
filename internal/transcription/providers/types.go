package providers

import "context"

// Word represents a single word with timing information
type Word struct {
	Word  string  `json:"word"`
	Start float32 `json:"start"` // seconds from start of audio
	End   float32 `json:"end"`   // seconds from start of audio
}

// ProviderResult contains the transcription output from any provider
type ProviderResult struct {
	Text       string  // The transcribed text
	Language   string  // Detected or specified language
	Confidence float32 // Confidence score (0-1), if available
	Duration   int     // Processing time in milliseconds
	Model      string  // Model used for transcription
	Words      []Word  // Word-level timestamps (if available)
}

// Provider is the interface for speech-to-text transcription providers
type Provider interface {
	// Name returns the provider name for logging and storage
	Name() string

	// Transcribe transcribes the audio file at the given path
	Transcribe(ctx context.Context, audioPath, language string) (*ProviderResult, error)

	// SupportedFormats returns the list of supported audio formats
	SupportedFormats() []string

	// Close releases any resources held by the provider
	Close() error
}
