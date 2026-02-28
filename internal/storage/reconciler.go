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
	window   time.Duration
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
