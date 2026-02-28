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
