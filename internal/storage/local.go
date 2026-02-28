package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalStore stores audio files on the local filesystem.
type LocalStore struct {
	audioDir string
}

// NewLocalStore creates a local filesystem audio store.
func NewLocalStore(audioDir string) *LocalStore {
	return &LocalStore{audioDir: audioDir}
}

func (s *LocalStore) Save(ctx context.Context, key string, data []byte, contentType string) error {
	path := filepath.Join(s.audioDir, key)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	// Atomic write: temp file + rename
	tmp, err := os.CreateTemp(dir, ".audio-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

func (s *LocalStore) LocalPath(key string) string {
	full := filepath.Join(s.audioDir, key)
	if _, err := os.Stat(full); err == nil {
		return full
	}
	return ""
}

func (s *LocalStore) URL(ctx context.Context, key string) (string, error) {
	return "", nil
}

func (s *LocalStore) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(s.audioDir, key))
}

func (s *LocalStore) Exists(ctx context.Context, key string) bool {
	_, err := os.Stat(filepath.Join(s.audioDir, key))
	return err == nil
}

func (s *LocalStore) Type() string { return "local" }

// Dir returns the audio directory path.
func (s *LocalStore) Dir() string { return s.audioDir }
