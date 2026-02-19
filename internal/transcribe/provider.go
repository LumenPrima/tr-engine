package transcribe

import "context"

// Provider is the interface for speech-to-text backends.
type Provider interface {
	Transcribe(ctx context.Context, audioPath string, opts TranscribeOpts) (*Response, error)
	Name() string  // "whisper", "elevenlabs"
	Model() string // model identifier for DB/logs
}

// Response is the common transcription result from any provider.
type Response struct {
	Text     string
	Language string
	Duration float64 // audio duration in seconds
	Words    []Word  // nil if provider doesn't support word timestamps
}

// Word is a timestamped word from any STT provider.
type Word struct {
	Word  string
	Start float64 // seconds
	End   float64 // seconds
}
