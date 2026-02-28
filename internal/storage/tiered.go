package storage

import (
	"context"
	"io"

	"github.com/rs/zerolog"
)

// TieredStore combines S3 (source of truth) with a local NVMe cache for
// low-latency serving. Write path: cache locally, upload to S3 async.
// Read path: local cache first, S3 fallback.
type TieredStore struct {
	primary *S3Store
	cache   *LocalStore
	log     zerolog.Logger
}

// NewTieredStore creates a tiered S3 + local cache store.
func NewTieredStore(primary *S3Store, cache *LocalStore, log zerolog.Logger) *TieredStore {
	return &TieredStore{
		primary: primary,
		cache:   cache,
		log:     log.With().Str("component", "tiered-store").Logger(),
	}
}

// Save writes to both S3 (source of truth) and local cache. Used in sync mode.
func (s *TieredStore) Save(ctx context.Context, key string, data []byte, ct string) error {
	if err := s.primary.Save(ctx, key, data, ct); err != nil {
		return err
	}
	if err := s.cache.Save(ctx, key, data, ct); err != nil {
		s.log.Warn().Err(err).Str("key", key).Msg("cache write failed")
	}
	return nil
}

// SaveToCache writes only to the local NVMe cache.
func (s *TieredStore) SaveToCache(ctx context.Context, key string, data []byte, ct string) error {
	return s.cache.Save(ctx, key, data, ct)
}

// SaveToS3 writes only to S3.
func (s *TieredStore) SaveToS3(ctx context.Context, key string, data []byte, ct string) error {
	return s.primary.Save(ctx, key, data, ct)
}

func (s *TieredStore) LocalPath(key string) string {
	return s.cache.LocalPath(key)
}

func (s *TieredStore) URL(ctx context.Context, key string) (string, error) {
	return s.primary.URL(ctx, key)
}

func (s *TieredStore) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if r, err := s.cache.Open(ctx, key); err == nil {
		return r, nil
	}
	return s.primary.Open(ctx, key)
}

func (s *TieredStore) Exists(ctx context.Context, key string) bool {
	if s.cache.Exists(ctx, key) {
		return true
	}
	return s.primary.Exists(ctx, key)
}

func (s *TieredStore) Type() string { return "tiered" }

// S3Store returns the underlying S3 store (used by pruner/reconciler).
func (s *TieredStore) S3Store() *S3Store { return s.primary }
