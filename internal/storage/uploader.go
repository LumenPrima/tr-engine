package storage

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

// AsyncUploader handles background S3 uploads without blocking the ingest pipeline.
// Files are already cached locally before being enqueued here.
type AsyncUploader struct {
	s3       *S3Store
	local    *LocalStore
	ch       chan uploadJob
	log      zerolog.Logger
	stopped  atomic.Bool
	stopOnce sync.Once
}

type uploadJob struct {
	key         string
	contentType string
}

// NewAsyncUploader creates an async S3 uploader with the given buffer size.
func NewAsyncUploader(tiered *TieredStore, bufferSize int, log zerolog.Logger) *AsyncUploader {
	if tiered == nil {
		return nil
	}
	return &AsyncUploader{
		s3:    tiered.s3,
		local: tiered.local,
		ch:    make(chan uploadJob, bufferSize),
		log:   log.With().Str("component", "async-uploader").Logger(),
	}
}

// Enqueue adds an S3 upload job. Non-blocking — drops with warning if full or stopped.
// Safe because the file is already in the local cache.
func (u *AsyncUploader) Enqueue(key string, contentType string) {
	if u == nil || u.stopped.Load() {
		return
	}
	job := uploadJob{key: key, contentType: contentType}
	select {
	case u.ch <- job:
	default:
		u.log.Warn().Str("key", key).Msg("async upload queue full, skipping (file safe in cache)")
	}
}

// Start launches worker goroutines.
func (u *AsyncUploader) Start(workers int) {
	if u == nil {
		return
	}
	for i := 0; i < workers; i++ {
		go u.worker()
	}
	u.log.Info().Int("workers", workers).Int("buffer", cap(u.ch)).Msg("async uploader started")
}

// Stop signals workers to drain. Call after closing the ingest pipeline.
func (u *AsyncUploader) Stop() {
	if u == nil {
		return
	}
	u.stopped.Store(true)
	u.stopOnce.Do(func() { close(u.ch) })
}

func (u *AsyncUploader) worker() {
	for job := range u.ch {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		var data []byte
		if u.local != nil {
			path, err := u.local.safePath(job.key)
			if err != nil {
				u.log.Error().Err(err).Str("key", job.key).Msg("async upload: invalid local path")
				cancel()
				continue
			}
			fileData, readErr := os.ReadFile(path)
			if readErr != nil {
				u.log.Error().Err(readErr).Str("key", job.key).Msg("async upload: failed to read cached file")
				cancel()
				continue
			}
			data = fileData
		}

		if len(data) == 0 {
			cancel()
			u.log.Error().Str("key", job.key).Msg("async upload: no data available for S3 save")
			continue
		}

		if err := u.s3.Save(ctx, job.key, data, job.contentType); err != nil {
			u.log.Error().Err(err).Str("key", job.key).Msg("async S3 upload failed (file safe in cache)")
		}
		cancel()
	}
}
