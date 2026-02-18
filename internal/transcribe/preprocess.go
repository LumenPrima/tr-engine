package transcribe

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// soxAvailable caches whether sox is in PATH (checked once at startup).
var soxAvailable *bool

// CheckSox checks if sox is available in PATH. Call once at startup.
func CheckSox() bool {
	if soxAvailable != nil {
		return *soxAvailable
	}
	_, err := exec.LookPath("sox")
	avail := err == nil
	soxAvailable = &avail
	return avail
}

// Preprocess applies audio cleanup for Whisper using sox:
//   - Resample to 16kHz mono
//   - Voice bandpass filter (300-3000Hz) via sinc — removes out-of-band tones,
//     DTMF, MDC1200, paging tones, and low-frequency noise
//   - Normalize volume
//
// Returns the path to a temporary WAV file and a cleanup function.
// If sox is unavailable, returns the original path with a no-op cleanup.
func Preprocess(ctx context.Context, inputPath string) (string, func(), error) {
	noop := func() {}

	if !CheckSox() {
		return inputPath, noop, nil
	}

	// Create temp file for output
	tmpDir := os.TempDir()
	outPath := filepath.Join(tmpDir, fmt.Sprintf("tr-engine-preprocess-%d.wav", os.Getpid()))

	// Sox pipeline: resample to 16kHz mono, voice bandpass 300-3000Hz, normalize
	//
	// sinc 300-3000 provides a sharper rolloff than highpass+lowpass and effectively
	// removes:
	// - Sub-300Hz rumble and hum
	// - DTMF tones (partial — some overlap)
	// - MDC1200 signaling (1200/1800Hz FSK — attenuated by narrow band)
	// - Paging tones above 3000Hz
	// - Any out-of-band noise artifacts
	//
	// The normalize pass ensures consistent volume for Whisper.
	cmd := exec.CommandContext(ctx, "sox",
		inputPath, outPath,
		"rate", "16000",
		"channels", "1",
		"sinc", "300-3000",
		"norm",
	)
	if err := cmd.Run(); err != nil {
		// Clean up partial output
		os.Remove(outPath)
		return inputPath, noop, fmt.Errorf("sox preprocess: %w", err)
	}

	cleanup := func() {
		os.Remove(outPath)
	}
	return outPath, cleanup, nil
}
