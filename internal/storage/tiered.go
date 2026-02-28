package storage

import (
	"bytes"
	"context"
	"io"

	"github.com/rs/zerolog"
)

// TieredStore combines local disk (source of truth) with S3 (backup/durability).
// Write path: save locally first (never block on S3), then push to S3.
// Read path: local first, S3 fallback with cache-on-read.
type TieredStore struct {
	s3    *S3Store
	local *LocalStore
	log   zerolog.Logger
}

// NewTieredStore creates a tiered local-primary + S3-backup store.
func NewTieredStore(s3 *S3Store, local *LocalStore, log zerolog.Logger) *TieredStore {
	return &TieredStore{
		s3:    s3,
		local: local,
		log:   log.With().Str("component", "tiered-store").Logger(),
	}
}

// Save writes to local disk first (fatal on failure), then S3 (warning on failure).
// S3 failures are non-fatal — the upload reconciler will catch them.
func (s *TieredStore) Save(ctx context.Context, key string, data []byte, ct string) error {
	if err := s.local.Save(ctx, key, data, ct); err != nil {
		return err
	}
	if err := s.s3.Save(ctx, key, data, ct); err != nil {
		s.log.Warn().Err(err).Str("key", key).Msg("S3 backup write failed, reconciler will retry")
	}
	return nil
}

// SaveLocal writes only to local disk.
func (s *TieredStore) SaveLocal(ctx context.Context, key string, data []byte, ct string) error {
	return s.local.Save(ctx, key, data, ct)
}

// SaveToS3 writes only to S3.
func (s *TieredStore) SaveToS3(ctx context.Context, key string, data []byte, ct string) error {
	return s.s3.Save(ctx, key, data, ct)
}

func (s *TieredStore) LocalPath(key string) string {
	return s.local.LocalPath(key)
}

func (s *TieredStore) URL(ctx context.Context, key string) (string, error) {
	return s.s3.URL(ctx, key)
}

// Open returns a reader for the audio file. Checks local disk first, then
// falls back to S3. On S3 hit, the file is cached locally for future reads.
func (s *TieredStore) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if r, err := s.local.Open(ctx, key); err == nil {
		return r, nil
	}
	// S3 fallback: read, cache locally, return
	r, err := s.s3.Open(ctx, key)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		return nil, err
	}
	// Best-effort local cache write
	if cacheErr := s.local.Save(ctx, key, data, ""); cacheErr != nil {
		s.log.Warn().Err(cacheErr).Str("key", key).Msg("failed to cache S3 file locally")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *TieredStore) Exists(ctx context.Context, key string) bool {
	if s.local.Exists(ctx, key) {
		return true
	}
	return s.s3.Exists(ctx, key)
}

func (s *TieredStore) Type() string { return "tiered" }

// S3Store returns the underlying S3 store (used by pruner/reconciler).
func (s *TieredStore) S3Store() *S3Store { return s.s3 }
