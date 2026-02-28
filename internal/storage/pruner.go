package storage

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
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
	stopOnce  sync.Once
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
	p.stopOnce.Do(func() { close(p.stop) })
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
		key     string
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
			if p.s3 != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				inS3 := p.s3.Exists(ctx, f.key)
				cancel()
				if !inS3 {
					skippedNotInS3++
					p.log.Warn().Str("key", f.key).Msg("skipping prune: file not in S3")
					continue
				}
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
