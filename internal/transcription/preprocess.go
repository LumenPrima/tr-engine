package transcription

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/trunk-recorder/tr-engine/internal/config"
	"go.uber.org/zap"
)

// Preprocessor handles audio preprocessing before transcription
type Preprocessor struct {
	cfg    config.AudioPreprocessConfig
	logger *zap.Logger
	tmpDir string
}

// NewPreprocessor creates a new audio preprocessor
func NewPreprocessor(cfg config.AudioPreprocessConfig, logger *zap.Logger) (*Preprocessor, error) {
	// Check if ffmpeg is available
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("ffmpeg not found in PATH: %w", err)
	}

	// Create temp directory for processed files
	tmpDir, err := os.MkdirTemp("", "tr-engine-audio-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	return &Preprocessor{
		cfg:    cfg,
		logger: logger,
		tmpDir: tmpDir,
	}, nil
}

// Process preprocesses an audio file for transcription
// Returns the path to the processed file (caller should clean up)
func (p *Preprocessor) Process(ctx context.Context, inputPath string) (string, error) {
	if !p.cfg.Enabled {
		return inputPath, nil
	}

	// Generate output path (always WAV for consistency)
	baseName := filepath.Base(inputPath)
	ext := filepath.Ext(baseName)
	nameWithoutExt := strings.TrimSuffix(baseName, ext)
	outputPath := filepath.Join(p.tmpDir, nameWithoutExt+"_processed.wav")

	// Build ffmpeg command
	args := p.buildFFmpegArgs(inputPath, outputPath)

	p.logger.Debug("Preprocessing audio",
		zap.String("input", inputPath),
		zap.String("output", outputPath),
		zap.Strings("ffmpeg_args", args),
	)

	// Run ffmpeg
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		p.logger.Warn("FFmpeg preprocessing failed, using original file",
			zap.String("input", inputPath),
			zap.Error(err),
			zap.String("output", string(output)),
		)
		// Return original file on failure - don't block transcription
		return inputPath, nil
	}

	// Verify output file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		p.logger.Warn("Preprocessed file not created, using original",
			zap.String("input", inputPath),
		)
		return inputPath, nil
	}

	return outputPath, nil
}

// buildFFmpegArgs constructs the ffmpeg command arguments
func (p *Preprocessor) buildFFmpegArgs(inputPath, outputPath string) []string {
	// Base args: overwrite output, hide banner, set log level
	args := []string{"-y", "-hide_banner", "-loglevel", "error", "-i", inputPath}

	// If custom filter is specified, use it directly
	if p.cfg.CustomFilter != "" {
		customArgs := strings.Fields(p.cfg.CustomFilter)
		args = append(args, customArgs...)
		args = append(args, outputPath)
		return args
	}

	// Build audio filter chain
	var filters []string

	// Apply highpass filter
	if p.cfg.HighpassHz > 0 {
		filters = append(filters, fmt.Sprintf("highpass=f=%d", p.cfg.HighpassHz))
	}

	// Apply lowpass filter
	if p.cfg.LowpassHz > 0 {
		filters = append(filters, fmt.Sprintf("lowpass=f=%d", p.cfg.LowpassHz))
	}

	// Normalize audio levels (use loudnorm for broadcast-style normalization)
	if p.cfg.Normalize {
		filters = append(filters, "loudnorm=I=-16:TP=-1.5:LRA=11")
	}

	// Add filter chain if we have filters
	if len(filters) > 0 {
		args = append(args, "-af", strings.Join(filters, ","))
	}

	// Set sample rate
	if p.cfg.SampleRate > 0 {
		args = append(args, "-ar", fmt.Sprintf("%d", p.cfg.SampleRate))
	}

	// Convert to mono (single channel) for speech recognition
	args = append(args, "-ac", "1")

	// Output path
	args = append(args, outputPath)

	return args
}

// Cleanup removes a processed file if it's in the temp directory
func (p *Preprocessor) Cleanup(path string) {
	// Only clean up files we created
	if strings.HasPrefix(path, p.tmpDir) {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			p.logger.Debug("Failed to cleanup temp file", zap.String("path", path), zap.Error(err))
		}
	}
}

// Close cleans up the preprocessor's temp directory
func (p *Preprocessor) Close() error {
	if p.tmpDir != "" {
		return os.RemoveAll(p.tmpDir)
	}
	return nil
}
