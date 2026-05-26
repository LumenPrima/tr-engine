package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalStore_SaveAndOpen(t *testing.T) {
	t.Parallel()

	t.Run("save and open round-trip", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := NewLocalStore(dir)

		data := []byte("hello audio world")
		ctx := context.Background()

		if err := s.Save(ctx, "test.mp3", data, "audio/mpeg"); err != nil {
			t.Fatalf("Save() = %v", err)
		}

		r, err := s.Open(ctx, "test.mp3")
		if err != nil {
			t.Fatalf("Open() = %v", err)
		}
		defer r.Close()

		read, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("ReadAll() = %v", err)
		}

		if string(read) != string(data) {
			t.Errorf("read data = %q, want %q", read, data)
		}
	})

	t.Run("save creates nested subdirectories", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := NewLocalStore(dir)

		data := []byte("nested")
		ctx := context.Background()

		if err := s.Save(ctx, "butco/2026-05-01/abc.mp3", data, "audio/mpeg"); err != nil {
			t.Fatalf("Save() = %v", err)
		}

		if !s.Exists(ctx, "butco/2026-05-01/abc.mp3") {
			t.Fatal("nested file should exist")
		}
	})

	t.Run("save rejects path traversal", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := NewLocalStore(dir)

		ctx := context.Background()
		err := s.Save(ctx, "../etc/evil", []byte("evil"), "text/plain")
		if err == nil {
			t.Error("Save() should fail for path traversal")
		}
	})

	t.Run("save with empty data succeeds", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := NewLocalStore(dir)

		ctx := context.Background()
		if err := s.Save(ctx, "empty.mp3", []byte{}, "audio/mpeg"); err != nil {
			t.Fatalf("Save() with empty data = %v", err)
		}
	})

	t.Run("saved file is complete not partial temp", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := NewLocalStore(dir)

		data := make([]byte, 1024*100)
		for i := range data {
			data[i] = byte(i % 256)
		}

		ctx := context.Background()
		if err := s.Save(ctx, "big.mp3", data, "audio/mpeg"); err != nil {
			t.Fatalf("Save() = %v", err)
		}

		path := s.LocalPath("big.mp3")
		if path == "" {
			t.Fatal("LocalPath() returned empty for saved file")
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if info.Size() != int64(len(data)) {
			t.Errorf("file size = %d, want %d", info.Size(), len(data))
		}

		entries, _ := os.ReadDir(dir)
		foundTemp := false
		for _, entry := range entries {
			if filepath.Ext(entry.Name()) == ".tmp" {
				foundTemp = true
				break
			}
		}
		if foundTemp {
			t.Error("temp file left behind after save")
		}
	})
}

func TestLocalStore_Get(t *testing.T) {
	t.Parallel()

	t.Run("exists returns true for existing file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := NewLocalStore(dir)

		ctx := context.Background()
		s.Save(ctx, "test.mp3", []byte("data"), "audio/mpeg")

		if !s.Exists(ctx, "test.mp3") {
			t.Error("Exists() = false for existing file")
		}
	})

	t.Run("exists returns false for missing file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := NewLocalStore(dir)

		ctx := context.Background()
		if s.Exists(ctx, "nonexistent.mp3") {
			t.Error("Exists() = true for missing file")
		}
	})

	t.Run("exists rejects path traversal", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := NewLocalStore(dir)

		ctx := context.Background()
		if s.Exists(ctx, "../etc/passwd") {
			t.Error("Exists() should not return true for path traversal")
		}
	})
}

func TestLocalStore_LocalPath(t *testing.T) {
	t.Parallel()

	t.Run("local path returned for existing file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := NewLocalStore(dir)

		s.Save(context.Background(), "test.mp3", []byte("data"), "audio/mpeg")

		lp := s.LocalPath("test.mp3")
		if lp == "" {
			t.Error("LocalPath() returned empty for existing file")
		}
		if !filepath.IsAbs(lp) {
			t.Errorf("LocalPath() = %q is not absolute", lp)
		}
	})

	t.Run("local path empty for missing file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := NewLocalStore(dir)

		lp := s.LocalPath("nonexistent.mp3")
		if lp != "" {
			t.Errorf("LocalPath() = %q, want empty for missing file", lp)
		}
	})

	t.Run("local path rejects path traversal", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := NewLocalStore(dir)

		lp := s.LocalPath("../etc/passwd")
		if lp != "" {
			t.Errorf("LocalPath() = %q, want empty for path traversal", lp)
		}
	})
}

func TestLocalStore_URL(t *testing.T) {
	t.Parallel()

	t.Run("local store always returns empty URL and nil error", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := NewLocalStore(dir)

		u, err := s.URL(context.Background(), "test.mp3")
		if u != "" {
			t.Errorf("URL() = %q, want empty string for local store", u)
		}
		if err != nil {
			t.Errorf("URL() error = %v, want nil", err)
		}
	})
}

func TestLocalStore_Type(t *testing.T) {
	t.Parallel()

	s := NewLocalStore("/tmp")
	if got := s.Type(); got != "local" {
		t.Errorf("Type() = %q, want %q", got, "local")
	}
}

func TestLocalStore_DirectoryCreation(t *testing.T) {
	t.Parallel()

	t.Run("multiple nested levels created automatically", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		s := NewLocalStore(dir)

		ctx := context.Background()
		if err := s.Save(ctx, "a/b/c/d/e/f/g/h.wav", []byte("deep"), "audio/pcm"); err != nil {
			t.Fatalf("Save() = %v", err)
		}

		if !s.Exists(ctx, "a/b/c/d/e/f/g/h.wav") {
			t.Error("deeply nested file should exist")
		}
	})
}

func TestLocalStore_Dir(t *testing.T) {
	t.Parallel()

	dir := "/var/audio/custom"
	s := NewLocalStore(dir)
	if got := s.Dir(); got != dir {
		t.Errorf("Dir() = %q, want %q", got, dir)
	}
}

func TestLocalStore_Open_NotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := NewLocalStore(dir)

	ctx := context.Background()
	_, err := s.Open(ctx, "missing.mp3")
	if err == nil {
		t.Error("Open() should error for missing file")
	}
}
