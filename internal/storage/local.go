package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalStore stores audio files on the local filesystem.
type LocalStore struct {
	audioDir string
}

// NewLocalStore creates a local filesystem audio store.
func NewLocalStore(audioDir string) *LocalStore {
	return &LocalStore{audioDir: audioDir}
}

// safePath resolves key to an absolute path under audioDir, rejecting path traversal.
func (s *LocalStore) safePath(key string) (string, error) {
	full := filepath.Join(s.audioDir, filepath.FromSlash(key))
	abs, err := filepath.Abs(full)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	base, err := filepath.Abs(s.audioDir)
	if err != nil {
		return "", fmt.Errorf("invalid base: %w", err)
	}
	if !strings.HasPrefix(abs, base+string(filepath.Separator)) && abs != base {
		return "", fmt.Errorf("path traversal rejected: %q", key)
	}
	return abs, nil
}

func (s *LocalStore) Save(ctx context.Context, key string, data []byte, contentType string) error {
	path, err := s.safePath(key)
	if err != nil {
		return err
	}
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
	full, err := s.safePath(key)
	if err != nil {
		return ""
	}
	if _, err := os.Stat(full); err == nil {
		return full
	}
	return ""
}

func (s *LocalStore) URL(ctx context.Context, key string) (string, error) {
	return "", nil
}

func (s *LocalStore) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	path, err := s.safePath(key)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

func (s *LocalStore) Exists(ctx context.Context, key string) bool {
	path, err := s.safePath(key)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func (s *LocalStore) Type() string { return "local" }

// Dir returns the audio directory path.
func (s *LocalStore) Dir() string { return s.audioDir }
