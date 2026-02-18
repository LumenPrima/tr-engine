package ingest

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
	"github.com/snarg/tr-engine/internal/api"
)

// FileWatcher monitors a trunk-recorder audio output directory for new JSON
// metadata files and ingests them via the Pipeline. This provides an alternative
// to MQTT-based ingestion for users who don't have the MQTT plugin configured.
type FileWatcher struct {
	pipeline   *Pipeline
	watchDir   string
	instanceID string
	backfillDays int
	log        zerolog.Logger

	watcher *fsnotify.Watcher
	cancel  func()

	// Debounce: coalesce rapid Create+Write events on the same file.
	debounceMu sync.Mutex
	debounceTimers map[string]*time.Timer

	// Stats
	filesProcessed atomic.Int64
	filesSkipped   atomic.Int64
	status         atomic.Value // string: "starting", "backfilling", "watching", "stopped"
}

func newFileWatcher(p *Pipeline, watchDir, instanceID string, backfillDays int) *FileWatcher {
	fw := &FileWatcher{
		pipeline:       p,
		watchDir:       watchDir,
		instanceID:     instanceID,
		backfillDays:   backfillDays,
		log:            p.log.With().Str("component", "watcher").Logger(),
		debounceTimers: make(map[string]*time.Timer),
	}
	fw.status.Store("starting")
	return fw
}

// Start initializes the fsnotify watcher, adds all existing directories, and
// begins watching for new files. If backfill is enabled, it processes existing
// JSON files in a background goroutine.
func (fw *FileWatcher) Start() error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	fw.watcher = w

	// Walk the directory tree and add all directories to fsnotify.
	dirCount := 0
	err = filepath.WalkDir(fw.watchDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fw.log.Warn().Err(err).Str("path", path).Msg("error walking directory")
			return nil // continue walking
		}
		if d.IsDir() {
			if addErr := w.Add(path); addErr != nil {
				fw.log.Warn().Err(addErr).Str("path", path).Msg("failed to watch directory")
			} else {
				dirCount++
			}
		}
		return nil
	})
	if err != nil {
		w.Close()
		return err
	}

	fw.log.Info().
		Int("directories", dirCount).
		Str("watch_dir", fw.watchDir).
		Msg("file watcher initialized")

	// Derive cancellation from the pipeline context.
	ctx, cancel := fw.pipeline.ctx, func() {}
	_ = ctx
	fw.cancel = cancel

	go fw.watchLoop()

	// Backfill existing files in background
	if fw.backfillDays >= 0 {
		go fw.backfill()
	} else {
		fw.status.Store("watching")
	}

	return nil
}

// Stop closes the fsnotify watcher and cancels any in-flight processing.
func (fw *FileWatcher) Stop() {
	fw.status.Store("stopped")
	if fw.watcher != nil {
		fw.watcher.Close()
	}
	if fw.cancel != nil {
		fw.cancel()
	}
	fw.log.Info().
		Int64("files_processed", fw.filesProcessed.Load()).
		Int64("files_skipped", fw.filesSkipped.Load()).
		Msg("file watcher stopped")
}

// Status returns the current watcher status for the health endpoint.
func (fw *FileWatcher) Status() *api.WatcherStatusData {
	s, _ := fw.status.Load().(string)
	return &api.WatcherStatusData{
		Status:         s,
		WatchDir:       fw.watchDir,
		FilesProcessed: fw.filesProcessed.Load(),
		FilesSkipped:   fw.filesSkipped.Load(),
	}
}

// watchLoop is the main event loop that processes fsnotify events.
func (fw *FileWatcher) watchLoop() {
	for {
		select {
		case <-fw.pipeline.ctx.Done():
			return

		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}

			if event.Op&(fsnotify.Create|fsnotify.Write) == 0 {
				continue
			}

			// New directory: add it to the watch set so we catch files in
			// newly created date directories (e.g. 2026/2/18/).
			if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
				if err := fw.watcher.Add(event.Name); err != nil {
					fw.log.Warn().Err(err).Str("path", event.Name).Msg("failed to watch new directory")
				} else {
					fw.log.Debug().Str("path", event.Name).Msg("watching new directory")
				}
				continue
			}

			// Only process .json files
			if !strings.HasSuffix(strings.ToLower(event.Name), ".json") {
				continue
			}

			fw.scheduleProcess(event.Name)

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			fw.log.Error().Err(err).Msg("fsnotify error")
		}
	}
}

// scheduleProcess debounces file processing by 500ms. This coalesces rapid
// Create+Write events and ensures the file is fully written before reading.
func (fw *FileWatcher) scheduleProcess(path string) {
	fw.debounceMu.Lock()
	defer fw.debounceMu.Unlock()

	if t, ok := fw.debounceTimers[path]; ok {
		t.Reset(500 * time.Millisecond)
		return
	}

	fw.debounceTimers[path] = time.AfterFunc(500*time.Millisecond, func() {
		fw.debounceMu.Lock()
		delete(fw.debounceTimers, path)
		fw.debounceMu.Unlock()

		fw.processJSONFile(path)
	})
}

// processJSONFile reads a JSON metadata file, parses it, and passes it to the
// pipeline for call creation.
func (fw *FileWatcher) processJSONFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		fw.log.Warn().Err(err).Str("path", path).Msg("failed to read JSON file")
		return
	}

	var meta AudioMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		fw.log.Warn().Err(err).Str("path", path).Msg("failed to parse JSON metadata")
		return
	}

	// Skip files with no talkgroup (invalid metadata)
	if meta.Talkgroup <= 0 {
		fw.filesSkipped.Add(1)
		return
	}

	if err := fw.pipeline.processWatchedFile(fw.instanceID, &meta, path); err != nil {
		fw.log.Warn().Err(err).Str("path", path).Msg("failed to process watched file")
		return
	}

	fw.filesProcessed.Add(1)
}

// backfill scans the watch directory for existing JSON files and processes any
// that aren't already in the database. Files are processed oldest-first with
// rate limiting to avoid overwhelming the database on first run.
func (fw *FileWatcher) backfill() {
	fw.status.Store("backfilling")
	start := time.Now()

	// Collect all .json files
	type fileEntry struct {
		path      string
		startTime int64
	}
	var files []fileEntry

	var cutoff int64
	if fw.backfillDays > 0 {
		cutoff = time.Now().AddDate(0, 0, -fw.backfillDays).Unix()
	}

	_ = filepath.WalkDir(fw.watchDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".json") {
			return nil
		}

		// Parse start_time from filename: {tgid}-{start_time}_{freq}-call_{id}.json
		ts := parseStartTimeFromFilename(filepath.Base(path))
		if ts == 0 {
			return nil
		}

		if cutoff > 0 && ts < cutoff {
			return nil // too old
		}

		files = append(files, fileEntry{path: path, startTime: ts})
		return nil
	})

	// Sort oldest first
	sort.Slice(files, func(i, j int) bool {
		return files[i].startTime < files[j].startTime
	})

	// Ensure partitions exist for the full date range before processing.
	// The schema only creates partitions for current month + 3 ahead, so
	// older backfill data would fail with "no partition found".
	if len(files) > 0 {
		oldest := time.Unix(files[0].startTime, 0)
		fw.ensurePartitions(oldest)
	}

	fw.log.Info().
		Int("files", len(files)).
		Int("backfill_days", fw.backfillDays).
		Msg("backfill starting")

	// Process files concurrently with a worker pool.
	const numWorkers = 16
	work := make(chan fileEntry, numWorkers*2)
	var wg sync.WaitGroup

	var processed atomic.Int64

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range work {
				fw.processJSONFile(f.path)
				n := processed.Add(1)
				if n%5000 == 0 {
					fw.log.Info().
						Int64("processed", n).
						Int("total", len(files)).
						Msg("backfill progress")
				}
			}
		}()
	}

	for _, f := range files {
		select {
		case <-fw.pipeline.ctx.Done():
			fw.log.Info().Int64("processed", processed.Load()).Msg("backfill interrupted by shutdown")
			close(work)
			wg.Wait()
			return
		case work <- f:
		}
	}
	close(work)
	wg.Wait()

	fw.status.Store("watching")
	fw.log.Info().
		Int64("processed", processed.Load()).
		Dur("elapsed", time.Since(start)).
		Msg("backfill complete")
}

// ensurePartitions creates monthly partitions for all partitioned tables from
// the given start time through the current month. This is needed for backfill
// since the schema only creates partitions for the current month + 3 ahead.
func (fw *FileWatcher) ensurePartitions(oldest time.Time) {
	ctx := fw.pipeline.ctx
	db := fw.pipeline.db

	monthlyTables := []string{"calls", "call_frequencies", "call_transmissions", "unit_events", "trunking_messages"}
	start := beginningOfMonth(oldest)
	now := beginningOfMonth(time.Now())
	created := 0

	for m := start; !m.After(now); m = m.AddDate(0, 1, 0) {
		for _, table := range monthlyTables {
			result, err := db.CreateMonthlyPartition(ctx, table, m)
			if err != nil {
				fw.log.Warn().Err(err).Str("table", table).Time("month", m).Msg("failed to create backfill partition")
			} else if !strings.Contains(result, "already exists") {
				created++
				fw.log.Info().Str("result", result).Str("table", table).Msg("created backfill partition")
			}
		}
	}

	if created > 0 {
		fw.log.Info().Int("partitions_created", created).Time("oldest_file", oldest).Msg("backfill partitions ensured")
	}
}

// parseStartTimeFromFilename extracts the Unix timestamp from a trunk-recorder
// filename. Format: {tgid}-{start_time}_{freq}-call_{id}.json
// Example: 9044-1771332008_859262500.0-call_28344.json → 1771332008
func parseStartTimeFromFilename(name string) int64 {
	// Split on first hyphen to get past the tgid
	parts := strings.SplitN(name, "-", 2)
	if len(parts) < 2 {
		return 0
	}
	// The rest starts with start_time followed by underscore
	rest := parts[1]
	idx := strings.Index(rest, "_")
	if idx <= 0 {
		return 0
	}
	ts, err := strconv.ParseInt(rest[:idx], 10, 64)
	if err != nil {
		return 0
	}
	return ts
}
