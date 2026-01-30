package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/trunk-recorder/tr-engine/internal/database"
	"github.com/trunk-recorder/tr-engine/internal/database/models"
	"github.com/trunk-recorder/tr-engine/internal/storage"
	"go.uber.org/zap"
)

// Importer handles bulk import of historical audio files
type Importer struct {
	db          *database.DB
	audioPath   string // source path (where to scan for files)
	destPath    string // destination path (where to copy files in copy mode)
	storageMode string // "copy" or "external"
	batchSize   int
	throttle    int // max calls per second, 0 = unlimited
	logger      *zap.Logger

	// Stats
	totalCalls    int64
	totalTx       int64
	totalFreqs    int64
	skippedCalls  int64
	errorCalls    int64
	bytesCopied   int64
	startTime     time.Time

	// Caches
	systemCache    map[string]*models.System
	talkgroupCache map[string]int // "sysid:tgid" -> db ID
}

// Config holds importer configuration
type Config struct {
	AudioPath   string // source path to scan for audio files
	DestPath    string // destination path for copy mode (empty = same as AudioPath)
	StorageMode string // "copy" or "external"
	BatchSize   int
	Throttle    int // calls per second, 0 = unlimited
	InstanceID  string
}

// New creates a new Importer
func New(db *database.DB, cfg Config, logger *zap.Logger) *Importer {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 1000
	}
	if cfg.InstanceID == "" {
		cfg.InstanceID = "importer"
	}
	if cfg.StorageMode == "" {
		cfg.StorageMode = "external"
	}
	// In external mode, dest = source; in copy mode, dest must be specified
	destPath := cfg.DestPath
	if cfg.StorageMode == "external" || destPath == "" {
		destPath = cfg.AudioPath
	}

	return &Importer{
		db:             db,
		audioPath:      cfg.AudioPath,
		destPath:       destPath,
		storageMode:    cfg.StorageMode,
		batchSize:      cfg.BatchSize,
		throttle:       cfg.Throttle,
		logger:         logger,
		systemCache:    make(map[string]*models.System),
		talkgroupCache: make(map[string]int),
	}
}

// Checkpoint represents import progress
type Checkpoint struct {
	LastSystem string `json:"last_system"`
	LastYear   int    `json:"last_year"`
	LastMonth  int    `json:"last_month"`
	LastDay    int    `json:"last_day"`
	TotalCalls int64  `json:"total_calls"`
	UpdatedAt  string `json:"updated_at"`
}

// Run starts the import process
func (i *Importer) Run(ctx context.Context) error {
	i.startTime = time.Now()

	// Load checkpoint
	checkpoint, err := i.loadCheckpoint(ctx)
	if err != nil {
		return fmt.Errorf("failed to load checkpoint: %w", err)
	}

	if checkpoint != nil {
		i.totalCalls = checkpoint.TotalCalls
		i.logger.Info("Resuming import from checkpoint",
			zap.String("last_system", checkpoint.LastSystem),
			zap.Int("last_year", checkpoint.LastYear),
			zap.Int("last_month", checkpoint.LastMonth),
			zap.Int("last_day", checkpoint.LastDay),
			zap.Int64("total_calls", checkpoint.TotalCalls),
		)
		fmt.Printf("Resuming from %s/%d/%02d/%02d (%d calls already imported)\n",
			checkpoint.LastSystem, checkpoint.LastYear, checkpoint.LastMonth, checkpoint.LastDay, checkpoint.TotalCalls)
	} else {
		fmt.Println("Starting fresh import...")
	}

	// Discover systems (top-level directories)
	systems, err := i.discoverSystems()
	if err != nil {
		return fmt.Errorf("failed to discover systems: %w", err)
	}

	if len(systems) == 0 {
		return fmt.Errorf("no system directories found in %s", i.audioPath)
	}

	fmt.Printf("Found %d systems: %v\n", len(systems), systems)

	// Process each system
	for _, system := range systems {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Skip systems before checkpoint
		if checkpoint != nil && system < checkpoint.LastSystem {
			continue
		}

		if err := i.processSystem(ctx, system, checkpoint); err != nil {
			return fmt.Errorf("failed to process system %s: %w", system, err)
		}

		// Clear checkpoint system filter after first system
		if checkpoint != nil && system == checkpoint.LastSystem {
			checkpoint = nil
		}
	}

	// Final stats
	elapsed := time.Since(i.startTime)
	rate := float64(i.totalCalls) / elapsed.Seconds()

	fmt.Printf("\n=== Import Complete ===\n")
	fmt.Printf("Total calls:    %d\n", i.totalCalls)
	fmt.Printf("Transmissions:  %d\n", i.totalTx)
	fmt.Printf("Frequencies:    %d\n", i.totalFreqs)
	fmt.Printf("Skipped:        %d\n", i.skippedCalls)
	fmt.Printf("Errors:         %d\n", i.errorCalls)
	if i.storageMode == "copy" {
		fmt.Printf("Bytes copied:   %s\n", formatBytes(i.bytesCopied))
	}
	fmt.Printf("Time elapsed:   %s\n", elapsed.Round(time.Second))
	fmt.Printf("Average rate:   %.1f calls/sec\n", rate)

	return nil
}

// formatBytes formats bytes into human-readable string
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// discoverSystems finds all system directories
func (i *Importer) discoverSystems() ([]string, error) {
	entries, err := os.ReadDir(i.audioPath)
	if err != nil {
		return nil, err
	}

	var systems []string
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			systems = append(systems, entry.Name())
		}
	}

	sort.Strings(systems)
	return systems, nil
}

// processSystem processes all years in a system directory
func (i *Importer) processSystem(ctx context.Context, system string, checkpoint *Checkpoint) error {
	systemPath := filepath.Join(i.audioPath, system)

	years, err := i.discoverNumericDirs(systemPath)
	if err != nil {
		return err
	}

	for _, year := range years {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Skip years before checkpoint
		if checkpoint != nil && checkpoint.LastSystem == system && year < checkpoint.LastYear {
			continue
		}

		if err := i.processYear(ctx, system, year, checkpoint); err != nil {
			return err
		}
	}

	return nil
}

// processYear processes all months in a year directory
func (i *Importer) processYear(ctx context.Context, system string, year int, checkpoint *Checkpoint) error {
	yearPath := filepath.Join(i.audioPath, system, strconv.Itoa(year))

	months, err := i.discoverNumericDirs(yearPath)
	if err != nil {
		return err
	}

	for _, month := range months {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Skip months before checkpoint
		if checkpoint != nil && checkpoint.LastSystem == system &&
		   checkpoint.LastYear == year && month < checkpoint.LastMonth {
			continue
		}

		if err := i.processMonth(ctx, system, year, month, checkpoint); err != nil {
			return err
		}
	}

	return nil
}

// processMonth processes all days in a month directory
func (i *Importer) processMonth(ctx context.Context, system string, year, month int, checkpoint *Checkpoint) error {
	monthPath := filepath.Join(i.audioPath, system, strconv.Itoa(year), strconv.Itoa(month))

	days, err := i.discoverNumericDirs(monthPath)
	if err != nil {
		return err
	}

	for _, day := range days {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Skip days before checkpoint
		if checkpoint != nil && checkpoint.LastSystem == system &&
		   checkpoint.LastYear == year && checkpoint.LastMonth == month && day <= checkpoint.LastDay {
			continue
		}

		if err := i.processDay(ctx, system, year, month, day); err != nil {
			i.logger.Error("Failed to process day",
				zap.String("system", system),
				zap.Int("year", year),
				zap.Int("month", month),
				zap.Int("day", day),
				zap.Error(err),
			)
			// Continue with next day instead of failing entirely
			continue
		}

		// Save checkpoint after each day
		if err := i.saveCheckpoint(ctx, system, year, month, day); err != nil {
			i.logger.Error("Failed to save checkpoint", zap.Error(err))
		}
	}

	return nil
}

// processDay processes all JSON files in a day directory
func (i *Importer) processDay(ctx context.Context, system string, year, month, day int) error {
	dayPath := filepath.Join(i.audioPath, system, strconv.Itoa(year), strconv.Itoa(month), strconv.Itoa(day))

	// Find all JSON files
	jsonFiles, err := filepath.Glob(filepath.Join(dayPath, "*.json"))
	if err != nil {
		return err
	}

	if len(jsonFiles) == 0 {
		return nil
	}

	dayStart := time.Now()
	dayCallCount := 0

	fmt.Printf("\rProcessing %s/%d/%02d/%02d (%d files)...    ", system, year, month, day, len(jsonFiles))

	// Process files with throttling
	for _, jsonFile := range jsonFiles {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err := i.processJSONFile(ctx, system, jsonFile); err != nil {
			i.errorCalls++
			i.logger.Debug("Failed to process file",
				zap.String("file", jsonFile),
				zap.Error(err),
			)
			continue
		}

		dayCallCount++
		i.totalCalls++

		// Throttle if configured
		if i.throttle > 0 && dayCallCount%i.throttle == 0 {
			time.Sleep(time.Second)
		}
	}

	elapsed := time.Since(dayStart)
	rate := float64(dayCallCount) / elapsed.Seconds()
	fmt.Printf("\rProcessed %s/%d/%02d/%02d: %d calls (%.1f/sec), total: %d    \n",
		system, year, month, day, dayCallCount, rate, i.totalCalls)

	return nil
}

// discoverNumericDirs finds numeric subdirectories and returns them sorted
func (i *Importer) discoverNumericDirs(path string) ([]int, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var nums []int
	for _, entry := range entries {
		if entry.IsDir() {
			if n, err := strconv.Atoi(entry.Name()); err == nil {
				nums = append(nums, n)
			}
		}
	}

	sort.Ints(nums)
	return nums, nil
}

// processJSONFile processes a single JSON sidecar file
func (i *Importer) processJSONFile(ctx context.Context, system string, jsonPath string) error {
	// Read and parse JSON
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	var sidecar storage.AudioSidecar
	if err := json.Unmarshal(data, &sidecar); err != nil {
		return fmt.Errorf("parse JSON: %w", err)
	}

	// Get or create system
	sys, err := i.getOrCreateSystem(ctx, system)
	if err != nil {
		return fmt.Errorf("get system: %w", err)
	}
	sysid := database.EffectiveSYSID(sys)

	// Get or create talkgroup
	tgID, err := i.getOrCreateTalkgroup(ctx, sysid, sys.ID, sidecar.Talkgroup, sidecar.TGTag, sidecar.TGDesc, sidecar.TGGroup, sidecar.TGGroupTag)
	if err != nil {
		return fmt.Errorf("get talkgroup: %w", err)
	}

	// Find the audio file (try common extensions)
	audioFile := strings.TrimSuffix(jsonPath, ".json")
	var srcAudioPath string
	var audioExt string
	for _, ext := range []string{".m4a", ".wav", ".mp3"} {
		if _, err := os.Stat(audioFile + ext); err == nil {
			srcAudioPath = audioFile + ext
			audioExt = ext
			break
		}
	}

	if srcAudioPath == "" {
		i.skippedCalls++
		return nil // No audio file found, skip
	}

	// Get relative path (relative to source audioPath)
	relPath := i.getRelativePath(srcAudioPath)

	// Get audio file size from source
	var audioSize int
	fi, err := os.Stat(srcAudioPath)
	if err != nil {
		return fmt.Errorf("stat audio file: %w", err)
	}
	audioSize = int(fi.Size())

	// In copy mode, copy the file to destination
	if i.storageMode == "copy" {
		dstPath := filepath.Join(i.destPath, relPath)

		// Skip if destination already exists
		if _, err := os.Stat(dstPath); err == nil {
			// File already copied, just use the relative path
		} else {
			// Copy the file
			n, err := copyFile(srcAudioPath, dstPath)
			if err != nil {
				return fmt.Errorf("copy audio file: %w", err)
			}
			i.bytesCopied += n

			// Also copy the JSON sidecar
			dstJSON := strings.TrimSuffix(dstPath, audioExt) + ".json"
			if _, err := copyFile(jsonPath, dstJSON); err != nil {
				i.logger.Debug("Failed to copy JSON sidecar", zap.Error(err))
			}
		}
	}

	// audioPath stored in DB is always the relative path
	audioPath := relPath

	// Check if call already exists (by system, tgid, start_time)
	startTime := time.Unix(sidecar.StartTime, 0)
	existing, _ := i.db.GetCallBySystemTGIDAndTime(ctx, sys.ID, sidecar.Talkgroup, startTime)
	if existing != nil {
		i.skippedCalls++
		return nil // Already imported
	}

	// Create call record
	stopTime := time.Unix(sidecar.StopTime, 0)
	call := &models.Call{
		InstanceID:  1, // Default instance for imports
		SystemID:    sys.ID,
		TalkgroupID: &tgID,
		StartTime:   startTime,
		StopTime:    &stopTime,
		Duration:    sidecar.CallLength,
		CallState:   3, // Completed
		Freq:        sidecar.Freq,
		FreqError:   sidecar.FreqError,
		Encrypted:   sidecar.Encrypted != 0,
		Emergency:   sidecar.Emergency != 0,
		Phase2TDMA:  sidecar.Phase2TDMA != 0,
		TDMASlot:    int16(sidecar.TDMASlot),
		AudioType:   sidecar.AudioType,
		SignalDB:    filterSentinelDB(sidecar.SignalDB),
		NoiseDB:     filterSentinelDB(sidecar.NoiseDB),
		AudioPath:   audioPath,
		AudioSize:   audioSize,
	}

	if err := i.db.InsertCall(ctx, call); err != nil {
		return fmt.Errorf("insert call: %w", err)
	}

	// Process transmissions from srcList
	var txs []*models.Transmission
	for idx, src := range sidecar.SrcList {
		// Get or create unit
		unit, err := i.db.UpsertUnit(ctx, sysid, src.Src, src.Tag, "import")
		if err != nil {
			i.logger.Debug("Failed to upsert unit", zap.Error(err))
			continue
		}

		var unitID *int
		if unit != nil {
			unitID = &unit.ID
			// Record site association
			if err := i.db.UpsertUnitSite(ctx, unit.ID, sys.ID); err != nil {
				i.logger.Debug("Failed to upsert unit site", zap.Error(err))
			}
		}

		// Calculate duration
		var duration float32
		var txStopTime *time.Time
		srcTime := time.Unix(src.Time, 0)

		if idx+1 < len(sidecar.SrcList) {
			duration = sidecar.SrcList[idx+1].Pos - src.Pos
		} else if sidecar.CallLength > 0 {
			duration = sidecar.CallLength - src.Pos
		}
		if duration > 0 {
			st := srcTime.Add(time.Duration(duration*1000) * time.Millisecond)
			txStopTime = &st
		}

		tx := &models.Transmission{
			CallID:    call.ID,
			UnitID:    unitID,
			UnitRID:   src.Src,
			StartTime: srcTime,
			StopTime:  txStopTime,
			Duration:  duration,
			Position:  src.Pos,
			Emergency: src.Emergency != 0,
		}

		if err := i.db.InsertTransmission(ctx, tx); err != nil {
			i.logger.Debug("Failed to insert transmission", zap.Error(err))
			continue
		}
		i.totalTx++
	}

	// Process frequencies from freqList
	for _, f := range sidecar.FreqList {
		cf := &models.CallFrequency{
			CallID:     call.ID,
			Freq:       f.Freq,
			Time:       time.Unix(f.Time, 0),
			Position:   f.Pos,
			Duration:   f.Len,
			ErrorCount: f.ErrorCount,
			SpikeCount: f.SpikeCount,
		}
		if err := i.db.InsertCallFrequency(ctx, cf); err != nil {
			i.logger.Debug("Failed to insert frequency", zap.Error(err))
			continue
		}
		i.totalFreqs++
	}

	return nil
}

// getRelativePath extracts the relative path from a full path
func (i *Importer) getRelativePath(fullPath string) string {
	rel, err := filepath.Rel(i.audioPath, fullPath)
	if err != nil {
		return filepath.Base(fullPath)
	}
	return rel
}

// getOrCreateSystem gets or creates a system record
func (i *Importer) getOrCreateSystem(ctx context.Context, shortName string) (*models.System, error) {
	if sys, ok := i.systemCache[shortName]; ok {
		return sys, nil
	}

	sys, err := i.db.UpsertSystem(ctx, 1, 0, shortName, "", "", "", "", 0, 0, nil)
	if err != nil {
		return nil, err
	}

	i.systemCache[shortName] = sys
	return sys, nil
}

// getOrCreateTalkgroup gets or creates a talkgroup record
func (i *Importer) getOrCreateTalkgroup(ctx context.Context, sysid string, systemID, tgid int, tag, desc, group, groupTag string) (int, error) {
	key := fmt.Sprintf("%s:%d", sysid, tgid)
	if id, ok := i.talkgroupCache[key]; ok {
		return id, nil
	}

	tg, err := i.db.UpsertTalkgroup(ctx, sysid, tgid, tag, desc, group, groupTag, 0, "")
	if err != nil {
		return 0, err
	}

	// Record site association
	if err := i.db.UpsertTalkgroupSite(ctx, tg.ID, systemID); err != nil {
		i.logger.Debug("Failed to upsert talkgroup site", zap.Error(err))
	}

	i.talkgroupCache[key] = tg.ID
	return tg.ID, nil
}

// loadCheckpoint loads the import checkpoint from the database
func (i *Importer) loadCheckpoint(_ context.Context) (*Checkpoint, error) {
	// Use a file-based checkpoint in the current directory
	// (audio path may be read-only)
	checkpointFile := ".tr-engine-import-checkpoint"

	data, err := os.ReadFile(checkpointFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, err
	}

	return &checkpoint, nil
}

// saveCheckpoint saves the import checkpoint
func (i *Importer) saveCheckpoint(_ context.Context, system string, year, month, day int) error {
	checkpoint := Checkpoint{
		LastSystem: system,
		LastYear:   year,
		LastMonth:  month,
		LastDay:    day,
		TotalCalls: i.totalCalls,
		UpdatedAt:  time.Now().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return err
	}

	// Save in current directory (audio path may be read-only)
	checkpointFile := ".tr-engine-import-checkpoint"
	return os.WriteFile(checkpointFile, data, 0644)
}

// copyFile copies a file from src to dst, creating directories as needed
// Returns the number of bytes copied
func copyFile(src, dst string) (int64, error) {
	// Create destination directory
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return 0, fmt.Errorf("create directory: %w", err)
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("open source: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return 0, fmt.Errorf("create destination: %w", err)
	}
	defer dstFile.Close()

	n, err := io.Copy(dstFile, srcFile)
	if err != nil {
		return 0, fmt.Errorf("copy: %w", err)
	}

	return n, nil
}

// filterSentinelDB converts sentinel values (999, -999) to 0 so omitempty excludes them.
// Trunk-recorder uses 999 to indicate unknown signal/noise levels.
func filterSentinelDB(v float32) float32 {
	if v >= 900 || v <= -900 {
		return 0
	}
	return v
}
