package storage

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

// AudioStorage handles audio file storage
type AudioStorage struct {
	basePath string
	mode     string // "copy" or "external"
	logger   *zap.Logger
}

// NewAudioStorage creates a new AudioStorage and ensures the base directory exists
func NewAudioStorage(basePath, mode string, logger *zap.Logger) *AudioStorage {
	if mode == "" {
		mode = "copy"
	}

	if mode == "copy" {
		// Create base directory if it doesn't exist (only for copy mode)
		if err := os.MkdirAll(basePath, 0755); err != nil {
			logger.Error("Failed to create audio storage directory",
				zap.String("path", basePath),
				zap.Error(err),
			)
		} else {
			logger.Info("Audio storage directory ready", zap.String("path", basePath), zap.String("mode", mode))
		}
	} else {
		logger.Info("Audio storage configured", zap.String("path", basePath), zap.String("mode", mode))
	}

	return &AudioStorage{
		basePath: basePath,
		mode:     mode,
		logger:   logger,
	}
}

// Mode returns the storage mode ("copy" or "external")
func (s *AudioStorage) Mode() string {
	return s.mode
}

// IsExternalMode returns true if using external audio files
func (s *AudioStorage) IsExternalMode() bool {
	return s.mode == "external"
}

// SaveAudio saves base64-encoded audio data to disk
// Returns the relative path and file size
func (s *AudioStorage) SaveAudio(shortName string, startTime time.Time, audioBase64, filename string) (string, int, error) {
	// Decode base64
	audioData, err := base64.StdEncoding.DecodeString(audioBase64)
	if err != nil {
		return "", 0, fmt.Errorf("failed to decode audio: %w", err)
	}

	// Build path: {basePath}/{system}/{year}/{month}/{day}/{filename}
	year := startTime.Format("2006")
	month := startTime.Format("01")
	day := startTime.Format("02")

	dirPath := filepath.Join(s.basePath, shortName, year, month, day)

	// Create directories
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", 0, fmt.Errorf("failed to create directory: %w", err)
	}

	// Generate filename if not provided
	if filename == "" {
		filename = fmt.Sprintf("%d_%s.m4a", startTime.Unix(), shortName)
	}

	fullPath := filepath.Join(dirPath, filename)
	relativePath := filepath.Join(shortName, year, month, day, filename)

	// Write file
	if err := os.WriteFile(fullPath, audioData, 0644); err != nil {
		return "", 0, fmt.Errorf("failed to write audio file: %w", err)
	}

	s.logger.Debug("Saved audio file",
		zap.String("path", relativePath),
		zap.Int("size", len(audioData)),
	)

	return relativePath, len(audioData), nil
}

// GetAudioPath returns the full path to an audio file
func (s *AudioStorage) GetAudioPath(relativePath string) string {
	return filepath.Join(s.basePath, relativePath)
}

// AudioExists checks if an audio file exists
func (s *AudioStorage) AudioExists(relativePath string) bool {
	fullPath := s.GetAudioPath(relativePath)
	_, err := os.Stat(fullPath)
	return err == nil
}

// DeleteAudio deletes an audio file
func (s *AudioStorage) DeleteAudio(relativePath string) error {
	fullPath := s.GetAudioPath(relativePath)
	return os.Remove(fullPath)
}

// GetBasePath returns the base storage path
func (s *AudioStorage) GetBasePath() string {
	return s.basePath
}

// MapExternalPath converts a trunk-recorder path to a local path
// Example: /app/tr_audio/butco/2025/1/28/file.m4a -> {basePath}/butco/2025/1/28/file.m4a
func (s *AudioStorage) MapExternalPath(trPath, shortName string) string {
	// Find the short_name in the path and extract everything from there
	idx := strings.Index(trPath, "/"+shortName+"/")
	if idx == -1 {
		// Short name not found, try just the filename
		return filepath.Join(s.basePath, filepath.Base(trPath))
	}
	relativePath := trPath[idx+1:] // Skip the leading slash
	return filepath.Join(s.basePath, relativePath)
}

// GetRelativePathFromExternal extracts the relative path from a trunk-recorder absolute path
// Example: /app/tr_audio/butco/2025/1/28/file.m4a -> butco/2025/1/28/file.m4a
func (s *AudioStorage) GetRelativePathFromExternal(trPath, shortName string) string {
	idx := strings.Index(trPath, "/"+shortName+"/")
	if idx == -1 {
		return filepath.Base(trPath)
	}
	return trPath[idx+1:]
}

// AudioSidecar contains the JSON sidecar file data from trunk-recorder
type AudioSidecar struct {
	Freq        int64   `json:"freq"`
	FreqError   int     `json:"freq_error"`
	SignalDB    float32 `json:"signal"`
	NoiseDB     float32 `json:"noise"`
	SourceNum   int     `json:"source_num"`
	RecorderNum int     `json:"recorder_num"`
	TDMASlot    int     `json:"tdma_slot"`
	Phase2TDMA  int     `json:"phase2_tdma"`
	StartTime   int64   `json:"start_time"`
	StopTime    int64   `json:"stop_time"`
	Emergency   int     `json:"emergency"`
	Priority    int     `json:"priority"`
	Mode        int     `json:"mode"`
	Duplex      int     `json:"duplex"`
	Encrypted   int     `json:"encrypted"`
	CallLength  float32 `json:"call_length"`
	Talkgroup   int     `json:"talkgroup"`
	TGTag       string  `json:"talkgroup_tag"`
	TGDesc      string  `json:"talkgroup_description"`
	TGGroupTag  string  `json:"talkgroup_group_tag"`
	TGGroup     string  `json:"talkgroup_group"`
	AudioType   string  `json:"audio_type"`
	ShortName   string  `json:"short_name"`
	FreqList    []struct {
		Freq       int64   `json:"freq"`
		Time       int64   `json:"time"`
		Pos        float32 `json:"pos"`
		Len        float32 `json:"len"`
		ErrorCount int     `json:"error_count"`
		SpikeCount int     `json:"spike_count"`
	} `json:"freqList"`
	SrcList []struct {
		Src          int64   `json:"src"`
		Time         int64   `json:"time"`
		Pos          float32 `json:"pos"`
		Emergency    int     `json:"emergency"`
		SignalSystem string  `json:"signal_system"`
		Tag          string  `json:"tag"`
	} `json:"srcList"`
}

// ReadAudioSidecar reads the JSON sidecar file for an audio file
func (s *AudioStorage) ReadAudioSidecar(audioPath string) (*AudioSidecar, error) {
	// Replace audio extension with .json
	jsonPath := strings.TrimSuffix(audioPath, filepath.Ext(audioPath)) + ".json"

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read sidecar file: %w", err)
	}

	var sidecar AudioSidecar
	if err := json.Unmarshal(data, &sidecar); err != nil {
		return nil, fmt.Errorf("failed to parse sidecar file: %w", err)
	}

	return &sidecar, nil
}
