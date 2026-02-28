# S3 Audio Storage Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add S3-compatible object storage as an audio backend with local NVMe caching, background uploads, and automatic cache pruning.

**Architecture:** New `internal/storage` package with an `AudioStore` interface and three backends: local (current behavior), S3, and tiered (S3 primary + local cache). The tiered store writes to local cache synchronously for immediate availability, then uploads to S3 asynchronously. A cache pruner evicts old local files (only after verifying S3 presence), and a reconciler re-uploads any files that failed to reach S3.

**Tech Stack:** Go, AWS SDK v2 (`aws-sdk-go-v2`), S3-compatible APIs (OVH, Minio, Backblaze, R2)

**Design Spec:** `D:\Downloads\s3-audio-spec.md` (full design with code samples)

---

## Task 1: Add S3Config to configuration

**Files:**
- Modify: `internal/config/config.go`

**Step 1: Add S3Config struct and embed it in Config**

Add after the existing `Config` struct fields, before `Validate()`:

```go
// S3 audio storage (optional — local disk used when S3_BUCKET is empty)
S3 S3Config
```

Add the `S3Config` struct definition after the `Config` struct:

```go
// S3Config holds S3-compatible object storage settings for audio files.
// All fields are optional — S3 is disabled when Bucket is empty.
type S3Config struct {
	Bucket         string        `env:"S3_BUCKET"`
	Endpoint       string        `env:"S3_ENDPOINT"`
	Region         string        `env:"S3_REGION" envDefault:"us-east-1"`
	AccessKey      string        `env:"S3_ACCESS_KEY"`
	SecretKey      string        `env:"S3_SECRET_KEY"`
	Prefix         string        `env:"S3_PREFIX"`
	PresignExpiry  time.Duration `env:"S3_PRESIGN_EXPIRY" envDefault:"1h"`
	LocalCache     bool          `env:"S3_LOCAL_CACHE" envDefault:"true"`
	CacheRetention time.Duration `env:"S3_CACHE_RETENTION" envDefault:"720h"` // 30d
	CacheMaxGB     int           `env:"S3_CACHE_MAX_GB" envDefault:"0"`
	UploadMode     string        `env:"S3_UPLOAD_MODE" envDefault:"async"`
}

// Enabled reports whether S3 audio storage is configured.
func (c S3Config) Enabled() bool { return c.Bucket != "" }
```

**Step 2: Verify build**

Run: `cd /c/Users/drewm/tr-engine && go build ./...`
Expected: Success (no usage of the new fields yet, but struct parses)

**Step 3: Commit**

```
feat(config): add S3Config for optional S3 audio storage
```

---

## Task 2: Add AWS SDK v2 dependency

**Files:**
- Modify: `go.mod`, `go.sum`

**Step 1: Add AWS SDK modules**

```bash
cd /c/Users/drewm/tr-engine
go get github.com/aws/aws-sdk-go-v2
go get github.com/aws/aws-sdk-go-v2/config
go get github.com/aws/aws-sdk-go-v2/credentials
go get github.com/aws/aws-sdk-go-v2/service/s3
```

**Step 2: Tidy**

```bash
go mod tidy
```

**Step 3: Verify build**

```bash
go build ./...
```

**Step 4: Commit**

```
chore: add aws-sdk-go-v2 dependency for S3 audio storage
```

---

## Task 3: Create `internal/storage` package — interface + local backend

**Files:**
- Create: `internal/storage/storage.go`
- Create: `internal/storage/local.go`

**Step 1: Create `storage.go` — interface and factory**

```go
package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/rs/zerolog"
	"github.com/snarg/tr-engine/internal/config"
)

// AudioStore abstracts audio file storage backends.
type AudioStore interface {
	// Save stores audio data. key format: {sys_name}/{YYYY-MM-DD}/{filename}
	Save(ctx context.Context, key string, data []byte, contentType string) error

	// LocalPath returns the local filesystem path if the file exists on disk.
	// Returns "" if not available locally.
	LocalPath(key string) string

	// URL returns a presigned URL for the audio file.
	// Returns "" for local-only backends.
	URL(ctx context.Context, key string) (string, error)

	// Open returns a reader for the audio file.
	Open(ctx context.Context, key string) (io.ReadCloser, error)

	// Exists checks if an audio file exists in any backend.
	Exists(ctx context.Context, key string) bool

	// Type returns "local", "s3", or "tiered".
	Type() string
}

// New creates an AudioStore based on config. Returns the store and optional
// background services (pruner, reconciler) that the caller must Start/Stop.
// Returns an error if S3 is configured but unreachable.
func New(cfg config.S3Config, audioDir string, log zerolog.Logger) (AudioStore, []BackgroundService, error) {
	if !cfg.Enabled() {
		return NewLocalStore(audioDir), nil, nil
	}

	s3store, err := NewS3Store(cfg, log)
	if err != nil {
		return nil, nil, fmt.Errorf("S3 init failed: %w", err)
	}

	// Startup validation: verify credentials and bucket access
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s3store.HeadBucket(ctx); err != nil {
		return nil, nil, fmt.Errorf("S3 startup check failed (bucket=%q endpoint=%q): %w",
			cfg.Bucket, cfg.Endpoint, err)
	}
	log.Info().Str("bucket", cfg.Bucket).Str("endpoint", cfg.Endpoint).Msg("S3 connection verified")

	if !cfg.LocalCache {
		return s3store, nil, nil
	}

	// Tiered mode: S3 primary + local NVMe cache
	cache := NewLocalStore(audioDir)
	tiered := NewTieredStore(s3store, cache, log)

	var services []BackgroundService

	// Cache pruner
	if cfg.CacheRetention > 0 || cfg.CacheMaxGB > 0 {
		pruner := NewCachePruner(audioDir, cfg.CacheRetention, cfg.CacheMaxGB, s3store, log)
		services = append(services, pruner)
	}

	// Upload reconciler
	reconciler := NewUploadReconciler(audioDir, s3store, log)
	services = append(services, reconciler)

	return tiered, services, nil
}

// BackgroundService is a stoppable background goroutine.
type BackgroundService interface {
	Start()
	Stop()
}
```

**Step 2: Create `local.go` — wraps current disk behavior**

```go
package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalStore stores audio files on the local filesystem.
// This wraps the current behavior — no change for existing deployments.
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
	return "", nil // local-only: caller uses LocalPath + http.ServeFile
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
```

**Step 3: Verify build**

```bash
go build ./...
```

**Step 4: Commit**

```
feat(storage): add AudioStore interface and LocalStore backend
```

---

## Task 4: Create S3 backend

**Files:**
- Create: `internal/storage/s3.go`

**Step 1: Create `s3.go`**

```go
package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/rs/zerolog"
	"github.com/snarg/tr-engine/internal/config"
)

// S3Store stores audio files in an S3-compatible object store.
type S3Store struct {
	client        *s3.Client
	presignClient *s3.PresignClient
	bucket        string
	prefix        string
	presignExpiry config.S3Config // keep full config for presign expiry
	log           zerolog.Logger
}

// NewS3Store creates an S3 audio store from config.
func NewS3Store(cfg config.S3Config, log zerolog.Logger) (*S3Store, error) {
	// Build AWS config with static credentials and custom endpoint
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		),
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}

	// S3 client options (custom endpoint for non-AWS providers)
	var s3Opts []func(*s3.Options)
	if cfg.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true // required for most S3-compatible providers
		})
	}

	client := s3.NewFromConfig(awsCfg, s3Opts...)
	presignClient := s3.NewPresignClient(client)

	return &S3Store{
		client:        client,
		presignClient: presignClient,
		bucket:        cfg.Bucket,
		prefix:        cfg.Prefix,
		presignExpiry: cfg,
		log:           log.With().Str("component", "s3-store").Logger(),
	}, nil
}

// HeadBucket checks that the bucket exists and credentials are valid.
func (s *S3Store) HeadBucket(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: &s.bucket,
	})
	return err
}

func (s *S3Store) Save(ctx context.Context, key string, data []byte, contentType string) error {
	objKey := s.objectKey(key)
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &s.bucket,
		Key:         &objKey,
		Body:        bytes.NewReader(data),
		ContentType: &contentType,
	})
	return err
}

func (s *S3Store) LocalPath(key string) string {
	return "" // no local cache in S3-only mode
}

func (s *S3Store) URL(ctx context.Context, key string) (string, error) {
	objKey := s.objectKey(key)
	req, err := s.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &objKey,
	}, func(opts *s3.PresignOptions) {
		opts.Expires = s.presignExpiry.PresignExpiry
	})
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func (s *S3Store) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	objKey := s.objectKey(key)
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &objKey,
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (s *S3Store) Exists(ctx context.Context, key string) bool {
	objKey := s.objectKey(key)
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &s.bucket,
		Key:    &objKey,
	})
	return err == nil
}

func (s *S3Store) Type() string { return "s3" }

func (s *S3Store) objectKey(key string) string {
	if s.prefix != "" {
		return s.prefix + "/audio/" + key
	}
	return "audio/" + key
}
```

**Step 2: Verify build**

```bash
go build ./...
```

**Step 3: Commit**

```
feat(storage): add S3Store backend with presigned URLs
```

---

## Task 5: Create tiered backend

**Files:**
- Create: `internal/storage/tiered.go`

**Step 1: Create `tiered.go`**

```go
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
// For async mode, use SaveToCache + SaveToS3 separately.
func (s *TieredStore) Save(ctx context.Context, key string, data []byte, ct string) error {
	if err := s.primary.Save(ctx, key, data, ct); err != nil {
		return err
	}
	if err := s.cache.Save(ctx, key, data, ct); err != nil {
		s.log.Warn().Err(err).Str("key", key).Msg("cache write failed")
	}
	return nil
}

// SaveToCache writes only to the local NVMe cache. Used by the async upload
// path to make audio available immediately.
func (s *TieredStore) SaveToCache(ctx context.Context, key string, data []byte, ct string) error {
	return s.cache.Save(ctx, key, data, ct)
}

// SaveToS3 writes only to S3. Used by the async upload worker after the file
// has already been cached locally.
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
```

**Step 2: Verify build**

```bash
go build ./...
```

**Step 3: Commit**

```
feat(storage): add TieredStore — S3 primary + local NVMe cache
```

---

## Task 6: Create cache pruner

**Files:**
- Create: `internal/storage/pruner.go`

**Step 1: Create `pruner.go`**

```go
package storage

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/rs/zerolog"
)

// CachePruner evicts old files from the local NVMe cache.
// S3 retains everything permanently — the pruner only touches local disk.
// Before deleting, it verifies the file exists in S3 to prevent data loss.
type CachePruner struct {
	cacheDir  string
	retention time.Duration
	maxBytes  int64
	interval  time.Duration
	s3        *S3Store
	log       zerolog.Logger
	stop      chan struct{}
}

// NewCachePruner creates a cache pruner that evicts files by age and/or size.
func NewCachePruner(cacheDir string, retention time.Duration, maxGB int, s3 *S3Store, log zerolog.Logger) *CachePruner {
	return &CachePruner{
		cacheDir:  cacheDir,
		retention: retention,
		maxBytes:  int64(maxGB) * 1024 * 1024 * 1024,
		interval:  1 * time.Hour,
		s3:        s3,
		log:       log.With().Str("component", "cache-pruner").Logger(),
		stop:      make(chan struct{}),
	}
}

func (p *CachePruner) Start() {
	go p.loop()
}

func (p *CachePruner) Stop() {
	close(p.stop)
}

func (p *CachePruner) loop() {
	// Run once on startup to clear any backlog from downtime
	p.prune()

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.prune()
		case <-p.stop:
			return
		}
	}
}

func (p *CachePruner) prune() {
	if p.retention == 0 && p.maxBytes == 0 {
		return
	}

	cutoff := time.Now().Add(-p.retention)
	var totalSize int64
	var prunedCount int
	var prunedBytes int64
	var skippedNotInS3 int

	type fileEntry struct {
		path    string
		key     string // relative key for S3 lookup
		modTime time.Time
		size    int64
	}
	var files []fileEntry

	filepath.WalkDir(p.cacheDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		// Compute relative key by stripping cacheDir prefix
		rel, relErr := filepath.Rel(p.cacheDir, path)
		if relErr != nil {
			return nil
		}
		files = append(files, fileEntry{
			path:    path,
			key:     filepath.ToSlash(rel),
			modTime: info.ModTime(),
			size:    info.Size(),
		})
		totalSize += info.Size()
		return nil
	})

	// Sort oldest first
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	for _, f := range files {
		shouldPrune := false

		if p.retention > 0 && f.modTime.Before(cutoff) {
			shouldPrune = true
		}
		if p.maxBytes > 0 && totalSize > p.maxBytes {
			shouldPrune = true
		}

		if shouldPrune {
			// Safety: verify S3 has the file before deleting local copy
			if p.s3 != nil && !p.s3.Exists(context.Background(), f.key) {
				skippedNotInS3++
				p.log.Warn().Str("key", f.key).Msg("skipping prune: file not in S3")
				continue
			}
			if err := os.Remove(f.path); err == nil {
				prunedCount++
				prunedBytes += f.size
				totalSize -= f.size
			}
		}
	}

	p.removeEmptyDirs()

	if prunedCount > 0 || skippedNotInS3 > 0 {
		p.log.Info().
			Int("pruned", prunedCount).
			Str("freed", humanizeBytes(prunedBytes)).
			Str("remaining", humanizeBytes(totalSize)).
			Int("skipped_not_in_s3", skippedNotInS3).
			Msg("cache prune complete")
	}
}

func (p *CachePruner) removeEmptyDirs() {
	entries, _ := os.ReadDir(p.cacheDir)
	for _, sysDir := range entries {
		if !sysDir.IsDir() {
			continue
		}
		sysPath := filepath.Join(p.cacheDir, sysDir.Name())
		dateDirs, _ := os.ReadDir(sysPath)
		for _, dateDir := range dateDirs {
			if !dateDir.IsDir() {
				continue
			}
			datePath := filepath.Join(sysPath, dateDir.Name())
			remaining, _ := os.ReadDir(datePath)
			if len(remaining) == 0 {
				os.Remove(datePath)
			}
		}
		remaining, _ := os.ReadDir(sysPath)
		if len(remaining) == 0 {
			os.Remove(sysPath)
		}
	}
}

func humanizeBytes(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
```

**Step 2: Verify build**

```bash
go build ./...
```

**Step 3: Commit**

```
feat(storage): add CachePruner with S3 existence safety check
```

---

## Task 7: Create upload reconciler

**Files:**
- Create: `internal/storage/reconciler.go`

**Step 1: Create `reconciler.go`**

```go
package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// UploadReconciler scans the local cache for files missing from S3 and
// re-uploads them. Handles failed/dropped async uploads and crash recovery.
type UploadReconciler struct {
	cacheDir string
	s3       *S3Store
	interval time.Duration
	window   time.Duration // only reconcile files from date dirs within this window
	log      zerolog.Logger
	stop     chan struct{}
}

// NewUploadReconciler creates a reconciler that checks for missing S3 uploads.
func NewUploadReconciler(cacheDir string, s3 *S3Store, log zerolog.Logger) *UploadReconciler {
	return &UploadReconciler{
		cacheDir: cacheDir,
		s3:       s3,
		interval: 5 * time.Minute,
		window:   24 * time.Hour,
		log:      log.With().Str("component", "upload-reconciler").Logger(),
		stop:     make(chan struct{}),
	}
}

func (r *UploadReconciler) Start() { go r.loop() }
func (r *UploadReconciler) Stop()  { close(r.stop) }

func (r *UploadReconciler) loop() {
	// Delay first run to let startup uploads settle
	select {
	case <-time.After(2 * time.Minute):
	case <-r.stop:
		return
	}

	r.reconcile()
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.reconcile()
		case <-r.stop:
			return
		}
	}
}

func (r *UploadReconciler) reconcile() {
	var uploaded, failed, checked int

	cutoff := time.Now().Add(-r.window)

	sysDirs, _ := os.ReadDir(r.cacheDir)
	for _, sysDir := range sysDirs {
		if !sysDir.IsDir() {
			continue
		}
		sysPath := filepath.Join(r.cacheDir, sysDir.Name())
		dateDirs, _ := os.ReadDir(sysPath)
		for _, dateDir := range dateDirs {
			if !dateDir.IsDir() {
				continue
			}
			// Parse date directory name to skip old directories
			dirDate, err := time.Parse("2006-01-02", dateDir.Name())
			if err == nil && dirDate.Before(cutoff) {
				continue
			}

			datePath := filepath.Join(sysPath, dateDir.Name())
			files, _ := os.ReadDir(datePath)
			for _, f := range files {
				if f.IsDir() {
					continue
				}
				// Skip temp files from atomic writes
				if strings.HasPrefix(f.Name(), ".audio-") && strings.HasSuffix(f.Name(), ".tmp") {
					continue
				}
				checked++
				key := filepath.ToSlash(
					sysDir.Name() + "/" + dateDir.Name() + "/" + f.Name(),
				)

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				exists := r.s3.Exists(ctx, key)
				cancel()
				if exists {
					continue
				}

				data, readErr := os.ReadFile(filepath.Join(datePath, f.Name()))
				if readErr != nil {
					continue
				}

				ct := audioContentTypeFromExt(filepath.Ext(f.Name()))
				ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
				if saveErr := r.s3.Save(ctx, key, data, ct); saveErr != nil {
					r.log.Warn().Err(saveErr).Str("key", key).Msg("reconcile upload failed")
					failed++
				} else {
					uploaded++
				}
				cancel()
			}
		}
	}

	if uploaded > 0 || failed > 0 {
		r.log.Info().
			Int("uploaded", uploaded).
			Int("failed", failed).
			Int("checked", checked).
			Msg("reconcile complete")
	}
}

// audioContentTypeFromExt returns the MIME type for an audio file extension.
func audioContentTypeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".m4a":
		return "audio/mp4"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".ogg":
		return "audio/ogg"
	default:
		return "application/octet-stream"
	}
}
```

**Step 2: Verify build**

```bash
go build ./...
```

**Step 3: Commit**

```
feat(storage): add UploadReconciler for failed async upload recovery
```

---

## Task 8: Create async uploader

**Files:**
- Create: `internal/storage/uploader.go`

**Step 1: Create `uploader.go`**

```go
package storage

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

// AsyncUploader handles background S3 uploads without blocking the ingest pipeline.
// Files are already cached locally before being enqueued here.
type AsyncUploader struct {
	s3   *S3Store
	ch   chan uploadJob
	log  zerolog.Logger
	stop chan struct{}
}

type uploadJob struct {
	key         string
	data        []byte
	contentType string
}

// NewAsyncUploader creates an async S3 uploader with the given buffer size.
func NewAsyncUploader(s3 *S3Store, bufferSize int, log zerolog.Logger) *AsyncUploader {
	return &AsyncUploader{
		s3:   s3,
		ch:   make(chan uploadJob, bufferSize),
		log:  log.With().Str("component", "async-uploader").Logger(),
		stop: make(chan struct{}),
	}
}

// Enqueue adds an S3 upload job. Non-blocking — drops with warning if full.
// Safe because the file is already in the local NVMe cache.
func (u *AsyncUploader) Enqueue(key string, data []byte, contentType string) {
	job := uploadJob{key: key, data: data, contentType: contentType}
	select {
	case u.ch <- job:
	default:
		u.log.Warn().Str("key", key).Msg("async upload queue full, skipping (file safe in cache)")
	}
}

// Start launches worker goroutines.
func (u *AsyncUploader) Start(workers int) {
	for i := 0; i < workers; i++ {
		go u.worker()
	}
	u.log.Info().Int("workers", workers).Int("buffer", cap(u.ch)).Msg("async uploader started")
}

// Stop signals workers to drain. Call after closing the ingest pipeline.
func (u *AsyncUploader) Stop() {
	close(u.ch)
}

func (u *AsyncUploader) worker() {
	for job := range u.ch {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := u.s3.Save(ctx, job.key, job.data, job.contentType); err != nil {
			u.log.Error().Err(err).Str("key", job.key).Msg("async S3 upload failed (file safe in cache)")
		}
		cancel()
	}
}
```

**Step 2: Verify build**

```bash
go build ./...
```

**Step 3: Commit**

```
feat(storage): add AsyncUploader with backpressure handling
```

---

## Task 9: Wire storage into the ingest pipeline

**Files:**
- Modify: `internal/ingest/pipeline.go`
- Modify: `internal/ingest/handler_audio.go`
- Modify: `internal/ingest/handler_upload.go`

**Step 1: Add AudioStore to Pipeline**

In `pipeline.go`, add to the `Pipeline` struct:

```go
store      storage.AudioStore
uploader   *storage.AsyncUploader // nil if not async mode
```

Add to `PipelineOptions`:

```go
Store      storage.AudioStore
S3Uploader *storage.AsyncUploader // nil if not async mode or no S3
```

In `NewPipeline`, set the fields:

```go
store:    opts.Store,
uploader: opts.S3Uploader,
```

In `Stop()`, add before `p.cancel()`:

```go
if p.uploader != nil {
	p.uploader.Stop()
}
```

Add the import for `"github.com/snarg/tr-engine/internal/storage"`.

**Step 2: Replace `saveAudioFile` in `handler_audio.go`**

Replace the `saveAudioFile` method and update `handleAudio` to use the store.

In `handleAudio`, replace the audio save block (lines 52-85):

```go
if p.trAudioDir == "" {
	audioData := msg.Call.AudioM4ABase64
	inferredType := "m4a"
	if audioData == "" {
		audioData = msg.Call.AudioWavBase64
		inferredType = "wav"
	}

	audioType := meta.AudioType
	if audioType == "" {
		audioType = inferredType
	}

	if audioData != "" {
		decoded, decErr := base64.StdEncoding.DecodeString(audioData)
		if decErr != nil {
			p.log.Warn().Err(decErr).Msg("failed to decode audio base64")
		} else {
			audioSize = len(decoded)
			filename := buildAudioFilename(meta.Filename, audioType, startTime)
			audioKey := buildAudioRelPath(meta.ShortName, startTime, filename)
			contentType := audioContentType(audioType)

			if err := p.saveAudio(ctx, audioKey, decoded, contentType); err != nil {
				p.log.Error().Err(err).Msg("failed to save audio file")
			} else {
				audioPath = audioKey
			}
		}
	}

	if callID > 0 && audioPath != "" {
		if err := p.db.UpdateCallAudio(ctx, callID, callStartTime, audioPath, audioSize); err != nil {
			p.log.Warn().Err(err).Int64("call_id", callID).Msg("failed to update call audio")
		}
	}
}
```

Add the `saveAudio` helper and `audioContentType` below `saveAudioFile`:

```go
// saveAudio writes audio data through the storage abstraction.
// For tiered stores in async mode: writes to local cache synchronously,
// then enqueues S3 upload in the background.
func (p *Pipeline) saveAudio(ctx context.Context, key string, data []byte, contentType string) error {
	if p.uploader != nil {
		// Async mode: cache locally first, then background S3 upload
		if tiered, ok := p.store.(*storage.TieredStore); ok {
			if err := tiered.SaveToCache(ctx, key, data, contentType); err != nil {
				return err
			}
			p.uploader.Enqueue(key, data, contentType)
			return nil
		}
	}
	// Sync mode or non-tiered store
	return p.store.Save(ctx, key, data, contentType)
}

// audioContentType returns the MIME type for an audio type string.
func audioContentType(audioType string) string {
	switch audioType {
	case "m4a":
		return "audio/mp4"
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "ogg":
		return "audio/ogg"
	default:
		return "application/octet-stream"
	}
}
```

**Step 3: Update `ProcessUploadedCall` in `handler_upload.go`**

Replace the audio save block (lines 91-113):

```go
var audioPath string
if len(audioData) > 0 {
	audioType := meta.AudioType
	if audioType == "" {
		if idx := strings.LastIndex(audioFilename, "."); idx >= 0 {
			audioType = audioFilename[idx+1:]
		}
	}
	if audioType == "" {
		audioType = "m4a"
	}

	filename := buildAudioFilename(audioFilename, audioType, startTime)
	audioKey := buildAudioRelPath(meta.ShortName, startTime, filename)
	contentType := audioContentType(audioType)

	if err := p.saveAudio(ctx, audioKey, audioData, contentType); err != nil {
		p.log.Error().Err(err).Int64("call_id", callID).Msg("failed to save uploaded audio file")
	} else {
		audioPath = audioKey
		if updateErr := p.db.UpdateCallAudio(ctx, callID, callStartTime, audioPath, len(audioData)); updateErr != nil {
			p.log.Warn().Err(updateErr).Int64("call_id", callID).Msg("failed to update call audio path")
		}
	}
}
```

**Step 4: Keep `saveAudioFile` for now (file watcher still uses it directly)**

The `saveAudioFile` method is still called by... actually, checking the code, `saveAudioFile` is only called from `handleAudio` and `ProcessUploadedCall`. Both are now using `saveAudio`. The `saveAudioFile` method can be removed entirely. Also remove the `"os"` import if it becomes unused.

**Step 5: Verify build**

```bash
go build ./...
```

**Step 6: Commit**

```
feat(ingest): wire AudioStore into pipeline with async upload support
```

---

## Task 10: Wire storage into audio serving

**Files:**
- Modify: `internal/api/calls.go`

**Step 1: Add store to CallsHandler**

Add to the `CallsHandler` struct:

```go
store storage.AudioStore
```

Update `NewCallsHandler`:

```go
func NewCallsHandler(db *database.DB, audioDir, trAudioDir string, store storage.AudioStore, live LiveDataSource) *CallsHandler {
	return &CallsHandler{db: db, audioDir: audioDir, trAudioDir: trAudioDir, store: store, live: live}
}
```

Add import for `"github.com/snarg/tr-engine/internal/storage"`.

**Step 2: Update `GetCallAudio` for the 3-step resolution chain**

Replace the `GetCallAudio` method:

```go
func (h *CallsHandler) GetCallAudio(w http.ResponseWriter, r *http.Request) {
	id, err := PathInt64(r, "id")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid call ID")
		return
	}

	audioPath, callFilename, err := h.db.GetCallAudioPath(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "audio not found")
		return
	}

	// 1. Try storage layer (local cache for tiered, local disk for local-only)
	if audioPath != "" && h.store != nil {
		if localFile := h.store.LocalPath(audioPath); localFile != "" {
			h.serveLocalFile(w, r, localFile, id)
			return
		}
	}

	// 2. Try S3 presigned URL redirect (tiered/S3 stores only)
	if audioPath != "" && h.store != nil {
		if url, urlErr := h.store.URL(r.Context(), audioPath); urlErr == nil && url != "" {
			http.Redirect(w, r, url, http.StatusTemporaryRedirect)
			return
		}
	}

	// 3. Fall back to TR_AUDIO_DIR resolution (file watch mode)
	fullPath := audio.ResolveFile(h.audioDir, h.trAudioDir, audioPath, callFilename)
	if fullPath != "" {
		h.serveLocalFile(w, r, fullPath, id)
		return
	}

	WriteError(w, http.StatusNotFound, "audio file not found on disk")
}

func (h *CallsHandler) serveLocalFile(w http.ResponseWriter, r *http.Request, path string, callID int64) {
	ext := strings.ToLower(filepath.Ext(path))
	contentTypes := map[string]string{
		".m4a": "audio/mp4",
		".mp3": "audio/mpeg",
		".wav": "audio/wav",
		".ogg": "audio/ogg",
	}
	if ct, ok := contentTypes[ext]; ok {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%d%s"`, callID, ext))
	http.ServeFile(w, r, path)
}
```

Remove the old `resolveAudioFile` method (it's been inlined).

**Step 3: Update `server.go` to pass store through**

In `server.go`, update the `NewCallsHandler` call (around line 142):

```go
NewCallsHandler(opts.DB, opts.Config.AudioDir, opts.Config.TRAudioDir, opts.Store, opts.Live).Routes(r)
```

Add to `ServerOptions`:

```go
Store storage.AudioStore
```

Add import for `"github.com/snarg/tr-engine/internal/storage"`.

**Step 4: Verify build**

```bash
go build ./...
```

**Step 5: Commit**

```
feat(api): add S3 presigned URL fallback to audio serving
```

---

## Task 11: Wire storage into transcription workers

**Files:**
- Modify: `internal/transcribe/worker.go`

**Step 1: Add store to WorkerPoolOptions**

Add to `WorkerPoolOptions`:

```go
Store storage.AudioStore // if set, used instead of AudioDir for file resolution
```

Add import for `"github.com/snarg/tr-engine/internal/storage"`.

**Step 2: Update `processJob` to use store.Open() when available**

In `processJob`, replace the audio resolution block (line 191):

```go
// 1. Resolve audio file
var audioReader io.ReadCloser
var audioPath string

if wp.opts.Store != nil && job.AudioFilePath != "" {
	// Use storage abstraction — tries local cache first, then S3
	reader, openErr := wp.opts.Store.Open(ctx, job.AudioFilePath)
	if openErr == nil {
		audioReader = reader
		// For preprocessing, we need a file path. Check local first.
		audioPath = wp.opts.Store.LocalPath(job.AudioFilePath)
		if audioPath == "" {
			// Not in local cache — write to temp file for preprocessing/STT
			tmpFile, tmpErr := os.CreateTemp("", "tr-audio-*.tmp")
			if tmpErr != nil {
				reader.Close()
				return errorf("create temp for STT: %w", tmpErr)
			}
			if _, cpErr := io.Copy(tmpFile, reader); cpErr != nil {
				reader.Close()
				tmpFile.Close()
				os.Remove(tmpFile.Name())
				return errorf("copy audio to temp: %w", cpErr)
			}
			reader.Close()
			tmpFile.Close()
			audioPath = tmpFile.Name()
			defer os.Remove(audioPath)
		} else {
			reader.Close()
		}
	}
}

// Fallback to direct file resolution
if audioPath == "" {
	audioPath = audio.ResolveFile(wp.opts.AudioDir, wp.opts.TRAudioDir, job.AudioFilePath, job.CallFilename)
}
if audioPath == "" {
	return errorf("audio file not found: path=%q filename=%q", job.AudioFilePath, job.CallFilename)
}
```

Add imports for `"io"` and `"os"` if not already present.

**Step 3: Verify build**

```bash
go build ./...
```

**Step 4: Commit**

```
feat(transcribe): use AudioStore for audio resolution with S3 fallback
```

---

## Task 12: Wire everything together in main.go

**Files:**
- Modify: `cmd/tr-engine/main.go`

**Step 1: Initialize storage after config, before pipeline**

Add after the database connection block and before transcription setup (~line 112):

```go
// Audio storage (local disk default, optional S3)
store, bgServices, err := storage.New(cfg.S3, cfg.AudioDir, log)
if err != nil {
	log.Fatal().Err(err).Msg("failed to initialize audio storage")
}
for _, svc := range bgServices {
	svc.Start()
	defer svc.Stop()
}
log.Info().Str("type", store.Type()).Msg("audio storage initialized")

// Async uploader (only for tiered stores in async mode)
var s3Uploader *storage.AsyncUploader
if tiered, ok := store.(*storage.TieredStore); ok && cfg.S3.UploadMode == "async" {
	s3Uploader = storage.NewAsyncUploader(tiered.S3Store(), 500, log)
	s3Uploader.Start(2)
	// Stopped by pipeline.Stop()
}
```

Add import for `"github.com/snarg/tr-engine/internal/storage"`.

**Step 2: Pass store to pipeline**

Update the `PipelineOptions` construction:

```go
pipeline := ingest.NewPipeline(ingest.PipelineOptions{
	DB:               db,
	AudioDir:         cfg.AudioDir,
	TRAudioDir:       cfg.TRAudioDir,
	RawStore:         cfg.RawStore,
	RawIncludeTopics: cfg.RawIncludeTopics,
	RawExcludeTopics: cfg.RawExcludeTopics,
	MergeP25Systems:  cfg.MergeP25Systems,
	TranscribeOpts:   transcribeOpts,
	Store:            store,
	S3Uploader:       s3Uploader,
	Log:              log,
})
```

**Step 3: Pass store to transcription opts**

If transcription is configured, add the store:

```go
if sttProvider != nil {
	transcribeOpts = &transcribe.WorkerPoolOptions{
		// ... existing fields ...
		Store: store,
	}
}
```

**Step 4: Pass store to server options**

```go
srv := api.NewServer(api.ServerOptions{
	// ... existing fields ...
	Store: store,
})
```

**Step 5: Verify build**

```bash
go build ./...
```

**Step 6: Commit**

```
feat: wire S3 audio storage into main startup

Supports three modes:
- Local-only (default, no config change)
- S3-only (S3_BUCKET + S3_LOCAL_CACHE=false)
- Tiered (S3_BUCKET, default) with local NVMe cache,
  async upload, cache pruner, and upload reconciler
```

---

## Task 13: Final cleanup — remove dead code

**Files:**
- Modify: `internal/ingest/handler_audio.go`

**Step 1: Remove the old `saveAudioFile` method**

Delete the `saveAudioFile` method (the one that writes directly to disk with `os.CreateTemp`/`os.Rename`). The `buildAudioFilename` and `buildAudioRelPath` helpers are still used — keep them.

If the `"os"` import is no longer needed in `handler_audio.go`, remove it. Keep `"os"` only if `processWatchedFile` still uses `os.Stat`.

**Step 2: Verify build**

```bash
go build ./...
```

**Step 3: Verify lint (TypeScript strict mode equivalent)**

```bash
go vet ./...
```

**Step 4: Commit**

```
refactor: remove saveAudioFile — replaced by AudioStore abstraction
```

---

## Summary of Changes

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `S3Config` struct |
| `internal/storage/storage.go` | `AudioStore` interface + `New()` factory |
| `internal/storage/local.go` | `LocalStore` — wraps current disk behavior |
| `internal/storage/s3.go` | `S3Store` — presigned URLs, upload, download |
| `internal/storage/tiered.go` | `TieredStore` — S3 primary + local cache |
| `internal/storage/pruner.go` | `CachePruner` — age/size eviction with S3 safety |
| `internal/storage/reconciler.go` | `UploadReconciler` — re-uploads missing files |
| `internal/storage/uploader.go` | `AsyncUploader` — background S3 uploads |
| `internal/ingest/pipeline.go` | Add `store`/`uploader` fields |
| `internal/ingest/handler_audio.go` | Use `saveAudio()` via store, remove `saveAudioFile` |
| `internal/ingest/handler_upload.go` | Use `saveAudio()` via store |
| `internal/api/calls.go` | 3-step resolution: cache → S3 redirect → TR_AUDIO_DIR |
| `internal/api/server.go` | Pass store to `CallsHandler` |
| `internal/transcribe/worker.go` | Use `store.Open()` for audio with S3 fallback |
| `cmd/tr-engine/main.go` | Initialize storage, wire into pipeline/server |
| `go.mod` | Add `aws-sdk-go-v2` |

**No changes to:** Database schema, API contracts, dashboard, file watcher behavior.
