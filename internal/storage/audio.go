package storage

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// AudioStorage handles audio file storage
type AudioStorage struct {
	basePath string
	logger   *zap.Logger
}

// NewAudioStorage creates a new AudioStorage and ensures the base directory exists
func NewAudioStorage(basePath string, logger *zap.Logger) *AudioStorage {
	// Create base directory if it doesn't exist
	if err := os.MkdirAll(basePath, 0755); err != nil {
		logger.Error("Failed to create audio storage directory",
			zap.String("path", basePath),
			zap.Error(err),
		)
	} else {
		logger.Info("Audio storage directory ready", zap.String("path", basePath))
	}

	return &AudioStorage{
		basePath: basePath,
		logger:   logger,
	}
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
