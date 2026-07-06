package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type nonSeekableReadCloser struct {
	reader *bytes.Reader
}

func newNonSeekableReadCloser(data []byte) *nonSeekableReadCloser {
	return &nonSeekableReadCloser{reader: bytes.NewReader(data)}
}

func (r *nonSeekableReadCloser) Close() error { return nil }

func (r *nonSeekableReadCloser) Read(p []byte) (int, error) { return r.reader.Read(p) }

func TestServeLocalFile_NoRangeReturns200(t *testing.T) {
	h := &CallsHandler{}
	dir := t.TempDir()
	path := filepath.Join(dir, "audio.mp3")
	data := []byte("abcdefghijklmnopqrstuvwxyz")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write audio file: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/calls/123/audio", nil)

	h.serveLocalFile(rec, req, path, 123)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "audio/mpeg" {
		t.Fatalf("content-type = %q, want %q", got, "audio/mpeg")
	}
	if got := rec.Header().Get("Content-Disposition"); got != `inline; filename="123.mp3"` {
		t.Fatalf("content-disposition = %q", got)
	}
	if body := rec.Body.Bytes(); !bytes.Equal(body, data) {
		t.Fatalf("body = %q, want %q", body, data)
	}
}

func TestServeLocalFile_ValidRangeReturns206(t *testing.T) {
	h := &CallsHandler{}
	dir := t.TempDir()
	path := filepath.Join(dir, "audio.mp3")
	data := []byte("abcdefghijklmnopqrstuvwxyz")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write audio file: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/calls/123/audio", nil)
	req.Header.Set("Range", "bytes=0-3")

	h.serveLocalFile(rec, req, path, 123)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusPartialContent)
	}
	if got := rec.Header().Get("Content-Range"); got != "bytes 0-3/26" {
		t.Fatalf("content-range = %q, want %q", got, "bytes 0-3/26")
	}
	if body := rec.Body.String(); body != "abcd" {
		t.Fatalf("body = %q, want %q", body, "abcd")
	}
}

func TestServeOpenAudio_SeekableValidRangeReturns206(t *testing.T) {
	h := &CallsHandler{}
	data := []byte("abcdefghijklmnopqrstuvwxyz")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/calls/123/audio", nil)
	req.Header.Set("Range", "bytes=5-8")

	served := h.serveOpenAudio(rec, req, ioNopCloser{Reader: bytes.NewReader(data)}, "audio.mp3", 123)
	if !served {
		t.Fatal("serveOpenAudio returned false")
	}

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusPartialContent)
	}
	if got := rec.Header().Get("Content-Range"); got != "bytes 5-8/26" {
		t.Fatalf("content-range = %q, want %q", got, "bytes 5-8/26")
	}
	if body := rec.Body.String(); body != "fghi" {
		t.Fatalf("body = %q, want %q", body, "fghi")
	}
}

func TestServeOpenAudio_NonSeekableValidRangeReturns206(t *testing.T) {
	h := &CallsHandler{}
	data := []byte("abcdefghijklmnopqrstuvwxyz")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/calls/123/audio", nil)
	req.Header.Set("Range", "bytes=10-12")

	served := h.serveOpenAudio(rec, req, newNonSeekableReadCloser(data), "audio.mp3", 123)
	if !served {
		t.Fatal("serveOpenAudio returned false")
	}

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusPartialContent)
	}
	if got := rec.Header().Get("Content-Range"); got != "bytes 10-12/26" {
		t.Fatalf("content-range = %q, want %q", got, "bytes 10-12/26")
	}
	if body := rec.Body.String(); body != "klm" {
		t.Fatalf("body = %q, want %q", body, "klm")
	}
}

func TestServeOpenAudio_InvalidRangeReturns416(t *testing.T) {
	h := &CallsHandler{}
	data := []byte("abcdefghijklmnopqrstuvwxyz")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/calls/123/audio", nil)
	req.Header.Set("Range", "bytes=999-1000")

	served := h.serveOpenAudio(rec, req, newNonSeekableReadCloser(data), "audio.mp3", 123)
	if !served {
		t.Fatal("serveOpenAudio returned false")
	}

	if rec.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestedRangeNotSatisfiable)
	}
	if got := rec.Header().Get("Content-Range"); got != "bytes */26" {
		t.Fatalf("content-range = %q, want %q", got, "bytes */26")
	}
}

type ioNopCloser struct {
	*bytes.Reader
}

func (c ioNopCloser) Close() error { return nil }
