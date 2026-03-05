package audio

import "github.com/rs/zerolog"

// AudioEncoder encodes PCM audio into a compressed format.
// Implementations must be safe for use by a single goroutine (the router loop).
type AudioEncoder interface {
	// Encode takes raw PCM data and returns encoded data with its format.
	Encode(pcmData []byte) ([]byte, AudioFormat, error)
	// Close releases any resources held by the encoder.
	Close()
	// SampleRate returns the sample rate this encoder was configured for.
	SampleRate() int
}

// PCMPassthroughEncoder returns PCM data unchanged.
// Used when no Opus encoder is available or when encoding is disabled (opusBitrate == 0).
type PCMPassthroughEncoder struct {
	sampleRate int
}

// NewPCMPassthroughEncoder creates a passthrough encoder that returns PCM data unchanged.
func NewPCMPassthroughEncoder(sampleRate int) *PCMPassthroughEncoder {
	return &PCMPassthroughEncoder{sampleRate: sampleRate}
}

func (e *PCMPassthroughEncoder) Encode(pcmData []byte) ([]byte, AudioFormat, error) {
	return pcmData, AudioFormatPCM, nil
}

func (e *PCMPassthroughEncoder) Close() {}

func (e *PCMPassthroughEncoder) SampleRate() int {
	return e.sampleRate
}

// NewEncoder creates an appropriate AudioEncoder based on the requested bitrate.
// When opusBitrate is 0, encoding is disabled and a PCM passthrough is returned.
// When opusBitrate > 0, Opus encoding is requested but not yet available, so a
// PCM passthrough is returned with a log warning.
func NewEncoder(sampleRate, opusBitrate int, log zerolog.Logger) AudioEncoder {
	if opusBitrate == 0 {
		return NewPCMPassthroughEncoder(sampleRate)
	}

	// Opus encoding requested but not available in this build.
	// A future build-tagged encoder_opus.go can replace this path.
	log.Warn().
		Int("opus_bitrate", opusBitrate).
		Msg("Opus encoding requested but not available; falling back to PCM passthrough")
	return NewPCMPassthroughEncoder(sampleRate)
}
