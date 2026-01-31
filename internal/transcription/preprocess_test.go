package transcription

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/trunk-recorder/tr-engine/internal/config"
	"go.uber.org/zap"
)

func TestPreprocessor_BuildSoxArgs(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name     string
		cfg      config.AudioPreprocessConfig
		input    string
		output   string
		wantArgs []string
	}{
		{
			name: "default voice bandpass",
			cfg: config.AudioPreprocessConfig{
				Enabled:    true,
				SampleRate: 16000,
				HighpassHz: 300,
				LowpassHz:  3000,
				Normalize:  true,
			},
			input:    "/tmp/input.wav",
			output:   "/tmp/output.wav",
			wantArgs: []string{"/tmp/input.wav", "/tmp/output.wav", "rate", "16000", "sinc", "300-3000", "norm"},
		},
		{
			name: "highpass only",
			cfg: config.AudioPreprocessConfig{
				Enabled:    true,
				SampleRate: 16000,
				HighpassHz: 300,
				LowpassHz:  0,
				Normalize:  false,
			},
			input:    "/tmp/input.wav",
			output:   "/tmp/output.wav",
			wantArgs: []string{"/tmp/input.wav", "/tmp/output.wav", "rate", "16000", "highpass", "300"},
		},
		{
			name: "custom filter",
			cfg: config.AudioPreprocessConfig{
				Enabled:      true,
				CustomFilter: "rate 8000 highpass 200 norm -3",
			},
			input:    "/tmp/input.wav",
			output:   "/tmp/output.wav",
			wantArgs: []string{"/tmp/input.wav", "/tmp/output.wav", "rate", "8000", "highpass", "200", "norm", "-3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Preprocessor{
				cfg:    tt.cfg,
				logger: logger,
			}

			args := p.buildSoxArgs(tt.input, tt.output)

			if len(args) != len(tt.wantArgs) {
				t.Errorf("buildSoxArgs() got %d args, want %d\nGot: %v\nWant: %v",
					len(args), len(tt.wantArgs), args, tt.wantArgs)
				return
			}

			for i, arg := range args {
				if arg != tt.wantArgs[i] {
					t.Errorf("buildSoxArgs() arg[%d] = %q, want %q", i, arg, tt.wantArgs[i])
				}
			}
		})
	}
}

func TestPreprocessor_Process_Disabled(t *testing.T) {
	logger := zap.NewNop()

	p := &Preprocessor{
		cfg: config.AudioPreprocessConfig{
			Enabled: false,
		},
		logger: logger,
	}

	input := "/some/path/audio.wav"
	output, err := p.Process(context.Background(), input)

	if err != nil {
		t.Errorf("Process() error = %v", err)
	}
	if output != input {
		t.Errorf("Process() when disabled should return input path, got %q", output)
	}
}

func TestPreprocessor_Integration(t *testing.T) {
	// Skip if sox is not available
	if _, err := os.Stat("/usr/bin/sox"); os.IsNotExist(err) {
		t.Skip("sox not found, skipping integration test")
	}

	// Create a simple test WAV file
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "test.wav")

	// Create a minimal WAV file (silence)
	// WAV header for 1 second of 8kHz 16-bit mono silence
	wavHeader := []byte{
		'R', 'I', 'F', 'F', // ChunkID
		0x24, 0x3e, 0x00, 0x00, // ChunkSize (16036 bytes)
		'W', 'A', 'V', 'E', // Format
		'f', 'm', 't', ' ', // Subchunk1ID
		16, 0, 0, 0, // Subchunk1Size (16 for PCM)
		1, 0, // AudioFormat (1 = PCM)
		1, 0, // NumChannels (1 = mono)
		0x40, 0x1f, 0x00, 0x00, // SampleRate (8000)
		0x80, 0x3e, 0x00, 0x00, // ByteRate (16000)
		2, 0, // BlockAlign (2)
		16, 0, // BitsPerSample (16)
		'd', 'a', 't', 'a', // Subchunk2ID
		0x00, 0x3e, 0x00, 0x00, // Subchunk2Size (16000 bytes of audio)
	}

	// Add 8000 samples of silence (1 second)
	audioData := make([]byte, 16000)
	wavData := append(wavHeader, audioData...)

	if err := os.WriteFile(inputPath, wavData, 0644); err != nil {
		t.Fatalf("Failed to create test WAV: %v", err)
	}

	logger := zap.NewNop()
	cfg := config.AudioPreprocessConfig{
		Enabled:    true,
		SampleRate: 16000,
		HighpassHz: 300,
		LowpassHz:  3000,
		Normalize:  true,
	}

	p, err := NewPreprocessor(cfg, logger)
	if err != nil {
		t.Fatalf("NewPreprocessor() error = %v", err)
	}
	defer p.Close()

	output, err := p.Process(context.Background(), inputPath)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// Verify output file was created
	if output == inputPath {
		t.Error("Process() should return different path when preprocessing is enabled")
	}

	if _, err := os.Stat(output); os.IsNotExist(err) {
		t.Errorf("Output file was not created: %s", output)
	}

	// Cleanup
	p.Cleanup(output)

	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Errorf("Cleanup() should have removed file: %s", output)
	}
}
