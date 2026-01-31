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
	// Check if sox is available
	if _, err := exec.LookPath("sox"); err != nil {
		return nil, fmt.Errorf("sox not found in PATH: %w", err)
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

	// Generate output path
	baseName := filepath.Base(inputPath)
	ext := filepath.Ext(baseName)
	nameWithoutExt := strings.TrimSuffix(baseName, ext)
	outputPath := filepath.Join(p.tmpDir, nameWithoutExt+"_processed.wav")

	// Build sox command
	args := p.buildSoxArgs(inputPath, outputPath)

	p.logger.Debug("Preprocessing audio",
		zap.String("input", inputPath),
		zap.String("output", outputPath),
		zap.Strings("sox_args", args),
	)

	// Run sox
	cmd := exec.CommandContext(ctx, "sox", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		p.logger.Error("Sox preprocessing failed",
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

// buildSoxArgs constructs the sox command arguments
func (p *Preprocessor) buildSoxArgs(inputPath, outputPath string) []string {
	// If custom filter is specified, use it directly
	if p.cfg.CustomFilter != "" {
		// Parse custom filter as space-separated args
		customArgs := strings.Fields(p.cfg.CustomFilter)
		args := []string{inputPath, outputPath}
		return append(args, customArgs...)
	}

	// Build standard voice bandpass filter
	args := []string{inputPath, outputPath}

	// Resample to target rate
	if p.cfg.SampleRate > 0 {
		args = append(args, "rate", fmt.Sprintf("%d", p.cfg.SampleRate))
	}

	// Apply bandpass filter using sinc (combines highpass and lowpass)
	if p.cfg.HighpassHz > 0 && p.cfg.LowpassHz > 0 {
		args = append(args, "sinc", fmt.Sprintf("%d-%d", p.cfg.HighpassHz, p.cfg.LowpassHz))
	} else if p.cfg.HighpassHz > 0 {
		args = append(args, "highpass", fmt.Sprintf("%d", p.cfg.HighpassHz))
	} else if p.cfg.LowpassHz > 0 {
		args = append(args, "lowpass", fmt.Sprintf("%d", p.cfg.LowpassHz))
	}

	// Normalize audio levels
	if p.cfg.Normalize {
		args = append(args, "norm")
	}

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
