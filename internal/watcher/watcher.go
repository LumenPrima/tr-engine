package watcher

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/trunk-recorder/tr-engine/internal/api/ws"
	"github.com/trunk-recorder/tr-engine/internal/database"
	"github.com/trunk-recorder/tr-engine/internal/database/models"
	"github.com/trunk-recorder/tr-engine/internal/storage"
	"go.uber.org/zap"
)

// Config holds watcher configuration
type Config struct {
	AudioPath string // Path to TR's audio directory
	LogPath   string // Optional: Path to TR's log directory for real-time events
	Backfill  bool   // If true, scan and import historical files on startup
}

// Watcher watches a directory for new audio files
type Watcher struct {
	db        *database.DB
	audioPath string
	logPath   string
	backfill  bool
	logger    *zap.Logger
	watcher   *fsnotify.Watcher
	logTailer *LogTailer
	hub       *ws.Hub

	// Caches (protected by mutex)
	mu             sync.RWMutex
	systemCache    map[string]*models.System
	talkgroupCache map[string]int

	// Active calls tracking (for log events)
	activeCalls map[string]*activeCall // key: "system:callID"

	// Recorder state tracking (for log events)
	recorders map[int]*RecorderState // key: recorderNum

	// Backfill state - audio files
	backfillDone     bool
	backfillTotal    int64
	backfillImported int64

	// Backfill state - log files
	logBackfillDone     bool
	logBackfillTotal    int64
	logBackfillImported int64

	// Stats
	callsProcessed int64
	callsSkipped   int64
	errors         int64
	logEvents      int64
}

// activeCall tracks an in-progress call from log events
type activeCall struct {
	system      string
	callID      string
	talkgroup   int
	freq        float64
	startTime   time.Time
	recorderNum int
}

// RecorderState tracks a recorder's current state
type RecorderState struct {
	ID         int     `json:"id"`
	SourceNum  int     `json:"src_num"`
	SourceFreq float64 `json:"source_freq"`
	Type       string  `json:"type"`
	State      string  `json:"state"`
	StateInt   int     `json:"state_int"` // 0=available, 1=recording, 2=idle
}

// New creates a new Watcher
func New(db *database.DB, cfg Config, logger *zap.Logger) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}

	w := &Watcher{
		db:             db,
		audioPath:      cfg.AudioPath,
		logPath:        cfg.LogPath,
		backfill:       cfg.Backfill,
		logger:         logger,
		watcher:        fsw,
		systemCache:    make(map[string]*models.System),
		talkgroupCache: make(map[string]int),
		activeCalls:    make(map[string]*activeCall),
		recorders:      make(map[int]*RecorderState),
	}

	// Create log tailer if log path is configured
	if cfg.LogPath != "" {
		tailer, err := NewLogTailer(cfg.LogPath, logger)
		if err != nil {
			fsw.Close()
			return nil, fmt.Errorf("create log tailer: %w", err)
		}
		w.logTailer = tailer
	}

	return w, nil
}

// Start begins watching for new files
func (w *Watcher) Start(ctx context.Context) error {
	// Ensure we have a default instance record
	if err := w.ensureDefaultInstance(ctx); err != nil {
		return fmt.Errorf("create default instance: %w", err)
	}

	// Add the base path and all existing subdirectories
	if err := w.addWatchRecursive(w.audioPath); err != nil {
		return fmt.Errorf("add watch paths: %w", err)
	}

	w.logger.Info("File watcher started",
		zap.String("audio_path", w.audioPath),
		zap.String("log_path", w.logPath),
		zap.Bool("backfill", w.backfill),
	)

	// Start real-time log tailing immediately (don't wait for backfill)
	if w.logTailer != nil {
		if err := w.logTailer.Start(ctx); err != nil {
			w.logger.Warn("Failed to start log tailer", zap.Error(err))
		} else {
			w.logger.Info("Real-time log tailing started")
			go w.processLogEvents(ctx)
		}
	}

	// Start backfill scanner if configured (runs in background)
	if w.backfill {
		go w.runBackfill(ctx)
	}

	go w.run(ctx)
	return nil
}

// Stop stops the watcher
func (w *Watcher) Stop() error {
	if w.logTailer != nil {
		w.logTailer.Stop()
	}
	return w.watcher.Close()
}

// SetHub sets the WebSocket hub for broadcasting events
func (w *Watcher) SetHub(hub *ws.Hub) {
	w.hub = hub
}

// broadcast sends an event to WebSocket clients
func (w *Watcher) broadcast(eventType string, data map[string]interface{}) {
	if w.hub == nil {
		return
	}
	event := ws.Event{
		Type:      eventType,
		Timestamp: time.Now().Unix(),
		Data:      data,
	}
	w.hub.Broadcast(event)
}

// Stats returns current watcher statistics
func (w *Watcher) Stats() (processed, skipped, errors int64) {
	return w.callsProcessed, w.callsSkipped, w.errors
}

// addWatchRecursive adds a directory and all subdirectories to the watch list
func (w *Watcher) addWatchRecursive(path string) error {
	return filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			// Skip directories we can't access (permission denied, etc.)
			w.logger.Debug("Skipping inaccessible path",
				zap.String("path", p),
				zap.Error(err),
			)
			return filepath.SkipDir
		}
		if d.IsDir() {
			if err := w.watcher.Add(p); err != nil {
				w.logger.Debug("Failed to watch directory",
					zap.String("path", p),
					zap.Error(err),
				)
			}
		}
		return nil
	})
}

// run is the main event loop
func (w *Watcher) run(ctx context.Context) {
	// Debounce map to avoid processing files multiple times
	pending := make(map[string]time.Time)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("File watcher stopped",
				zap.Int64("calls_processed", w.callsProcessed),
				zap.Int64("calls_skipped", w.callsSkipped),
				zap.Int64("errors", w.errors),
			)
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Handle new directories (add to watch)
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					w.watcher.Add(event.Name)
					w.logger.Debug("Watching new directory", zap.String("path", event.Name))
					continue
				}
			}

			// Only care about JSON files being created or written
			if !strings.HasSuffix(event.Name, ".json") {
				continue
			}

			if event.Op&(fsnotify.Create|fsnotify.Write) != 0 {
				// Add to pending with current time
				pending[event.Name] = time.Now()
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Error("Watcher error", zap.Error(err))

		case <-ticker.C:
			// Process files that have been stable for 1 second
			now := time.Now()
			for path, t := range pending {
				if now.Sub(t) > time.Second {
					delete(pending, path)
					go w.processFile(ctx, path)
				}
			}
		}
	}
}

// processFile processes a single JSON sidecar file
func (w *Watcher) processFile(ctx context.Context, jsonPath string) {
	// Read and parse JSON
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		w.logger.Debug("Failed to read file", zap.String("path", jsonPath), zap.Error(err))
		w.errors++
		return
	}

	var sidecar storage.AudioSidecar
	if err := json.Unmarshal(data, &sidecar); err != nil {
		w.logger.Debug("Failed to parse JSON", zap.String("path", jsonPath), zap.Error(err))
		w.errors++
		return
	}

	// Extract system name from path
	system := w.extractSystemName(jsonPath)
	if system == "" {
		w.logger.Debug("Could not determine system", zap.String("path", jsonPath))
		w.errors++
		return
	}

	// Get or create system
	sys, err := w.getOrCreateSystem(ctx, system)
	if err != nil {
		w.logger.Debug("Failed to get system", zap.String("system", system), zap.Error(err))
		w.errors++
		return
	}
	sysid := database.EffectiveSYSID(sys)

	// Get or create talkgroup
	tgID, err := w.getOrCreateTalkgroup(ctx, sysid, sys.ID, sidecar.Talkgroup, sidecar.TGTag, sidecar.TGDesc, sidecar.TGGroup, sidecar.TGGroupTag)
	if err != nil {
		w.logger.Debug("Failed to get talkgroup", zap.Error(err))
		w.errors++
		return
	}

	// Find the audio file
	audioFile := strings.TrimSuffix(jsonPath, ".json")
	var audioPath string
	for _, ext := range []string{".m4a", ".wav", ".mp3"} {
		if _, err := os.Stat(audioFile + ext); err == nil {
			audioPath = w.getRelativePath(audioFile + ext)
			break
		}
	}

	if audioPath == "" {
		w.logger.Debug("No audio file found", zap.String("json", jsonPath))
		w.callsSkipped++
		return
	}

	// Get audio file size
	var audioSize int
	if fi, err := os.Stat(filepath.Join(w.audioPath, audioPath)); err == nil {
		audioSize = int(fi.Size())
	}

	// Check if call already exists
	startTime := time.Unix(sidecar.StartTime, 0)
	existing, _ := w.db.GetCallBySystemTGIDAndTime(ctx, sys.ID, sidecar.Talkgroup, startTime)
	if existing != nil {
		w.callsSkipped++
		return
	}

	// Create call record
	stopTime := time.Unix(sidecar.StopTime, 0)
	call := &models.Call{
		InstanceID:  1,
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
		SignalDB:    sidecar.SignalDB,
		NoiseDB:     sidecar.NoiseDB,
		AudioPath:   audioPath,
		AudioSize:   audioSize,
	}

	if err := w.db.InsertCall(ctx, call); err != nil {
		w.logger.Debug("Failed to insert call", zap.Error(err))
		w.errors++
		return
	}

	// Process transmissions
	for idx, src := range sidecar.SrcList {
		unit, err := w.db.UpsertUnit(ctx, sysid, src.Src, src.Tag, "watcher")
		if err != nil {
			continue
		}

		// Record site association
		if unit != nil {
			w.db.UpsertUnitSite(ctx, unit.ID, sys.ID)
		}

		var unitID *int
		if unit != nil {
			unitID = &unit.ID
		}

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
		w.db.InsertTransmission(ctx, tx)
	}

	// Process frequencies
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
		w.db.InsertCallFrequency(ctx, cf)
	}

	w.callsProcessed++

	// Broadcast audio available event
	w.broadcast("audio_available", map[string]interface{}{
		"call_id":             call.ID,
		"system":              system,
		"talkgroup":           sidecar.Talkgroup,
		"talkgroup_alpha_tag": sidecar.TGTag,
		"freq":                sidecar.Freq,
		"duration":            sidecar.CallLength,
		"audio_path":          audioPath,
		"encrypted":           sidecar.Encrypted != 0,
		"emergency":           sidecar.Emergency != 0,
	})

	w.logger.Debug("Processed call",
		zap.String("system", system),
		zap.Int("talkgroup", sidecar.Talkgroup),
		zap.Float32("duration", sidecar.CallLength),
	)
}

// extractSystemName extracts the system short_name from the file path
// Expected path: {audioPath}/{system}/{year}/{month}/{day}/file.json
func (w *Watcher) extractSystemName(filePath string) string {
	rel, err := filepath.Rel(w.audioPath, filePath)
	if err != nil {
		return ""
	}

	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 1 {
		return ""
	}
	return parts[0]
}

// getRelativePath returns the path relative to audioPath
func (w *Watcher) getRelativePath(fullPath string) string {
	rel, err := filepath.Rel(w.audioPath, fullPath)
	if err != nil {
		return filepath.Base(fullPath)
	}
	return rel
}

// ensureDefaultInstance creates a default instance record if needed
func (w *Watcher) ensureDefaultInstance(ctx context.Context) error {
	_, err := w.db.UpsertInstance(ctx, "watcher", "", nil)
	return err
}

// getOrCreateSystem gets or creates a system record (cached)
func (w *Watcher) getOrCreateSystem(ctx context.Context, shortName string) (*models.System, error) {
	w.mu.RLock()
	if sys, ok := w.systemCache[shortName]; ok {
		w.mu.RUnlock()
		return sys, nil
	}
	w.mu.RUnlock()

	sys, err := w.db.UpsertSystem(ctx, 1, 0, shortName, "", "", "", "", 0, 0, nil)
	if err != nil {
		return nil, err
	}

	w.mu.Lock()
	w.systemCache[shortName] = sys
	w.mu.Unlock()
	return sys, nil
}

// getOrCreateTalkgroup gets or creates a talkgroup record (cached)
func (w *Watcher) getOrCreateTalkgroup(ctx context.Context, sysid string, systemID, tgid int, tag, desc, group, groupTag string) (int, error) {
	key := fmt.Sprintf("%s:%d", sysid, tgid)

	w.mu.RLock()
	if id, ok := w.talkgroupCache[key]; ok {
		w.mu.RUnlock()
		return id, nil
	}
	w.mu.RUnlock()

	tg, err := w.db.UpsertTalkgroup(ctx, sysid, tgid, tag, desc, group, groupTag, 0, "")
	if err != nil {
		return 0, err
	}

	// Record site association
	if err := w.db.UpsertTalkgroupSite(ctx, tg.ID, systemID); err != nil {
		w.logger.Debug("Failed to upsert talkgroup site", zap.Error(err))
	}

	w.mu.Lock()
	w.talkgroupCache[key] = tg.ID
	w.mu.Unlock()
	return tg.ID, nil
}

// processLogEvents processes events from the log tailer
func (w *Watcher) processLogEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.logTailer.Events():
			if !ok {
				return
			}
			w.handleLogEvent(ctx, event)
		}
	}
}

// handleLogEvent processes a single log event
func (w *Watcher) handleLogEvent(ctx context.Context, event LogEvent) {
	w.logEvents++

	switch event.Type {
	case EventCallStart:
		w.handleCallStart(ctx, event)
	case EventCallStop:
		w.handleCallStop(ctx, event)
	case EventCallConcluding:
		// Concluding events give us duration info but we get full data from JSON sidecar
		w.handleCallConcluding(ctx, event)
	case EventUnitOnCall:
		w.handleUnitOnCall(ctx, event)
	case EventUnitAlias:
		w.handleUnitAlias(ctx, event)
	case EventDecodeRate:
		w.handleDecodeRate(ctx, event)
	case EventRecorder:
		w.handleRecorder(ctx, event)
	// EventActiveCall, EventPatch - informational, not stored
	default:
		// Ignore other events
	}
}

// handleCallStart tracks a new active call
func (w *Watcher) handleCallStart(ctx context.Context, event LogEvent) {
	data := event.Data.(CallStartEvent)
	key := fmt.Sprintf("%s:%s", event.System, data.CallID)

	w.mu.Lock()
	w.activeCalls[key] = &activeCall{
		system:      event.System,
		callID:      data.CallID,
		talkgroup:   data.Talkgroup,
		freq:        data.Freq,
		startTime:   event.Timestamp,
		recorderNum: data.RecorderNum,
	}
	w.mu.Unlock()

	// Broadcast call start
	w.broadcast("call_start", map[string]interface{}{
		"tr_call_id": data.CallID,
		"talkgroup":  data.Talkgroup,
		"system":     event.System,
		"freq":       int64(data.Freq * 1e6),
		"recorder":   data.RecorderNum,
	})

	w.logger.Debug("Call started (from log)",
		zap.String("system", event.System),
		zap.String("call_id", data.CallID),
		zap.Int("talkgroup", data.Talkgroup),
	)
}

// handleCallStop removes a call from active tracking
func (w *Watcher) handleCallStop(ctx context.Context, event LogEvent) {
	data := event.Data.(CallStopEvent)
	key := fmt.Sprintf("%s:%s", event.System, data.CallID)

	w.mu.Lock()
	call, exists := w.activeCalls[key]
	delete(w.activeCalls, key)
	w.mu.Unlock()

	// Broadcast call end
	broadcastData := map[string]interface{}{
		"tr_call_id": data.CallID,
		"talkgroup":  data.Talkgroup,
		"system":     event.System,
		"freq":       int64(data.Freq * 1e6),
	}
	if exists {
		broadcastData["duration"] = int(event.Timestamp.Sub(call.startTime).Seconds())
	}
	w.broadcast("call_end", broadcastData)

	w.logger.Debug("Call stopped (from log)",
		zap.String("system", event.System),
		zap.String("call_id", data.CallID),
		zap.Int("hz_error", data.HzError),
	)
}

// handleCallConcluding logs call conclusion info
func (w *Watcher) handleCallConcluding(ctx context.Context, event LogEvent) {
	data := event.Data.(CallConcludingEvent)

	w.logger.Debug("Call concluding (from log)",
		zap.String("system", event.System),
		zap.String("call_id", data.CallID),
		zap.Int("elapsed", data.CallElapsed),
	)
}

// handleUnitOnCall records unit activity on a call
func (w *Watcher) handleUnitOnCall(ctx context.Context, event LogEvent) {
	data := event.Data.(UnitOnCallEvent)

	// Get or create system
	sys, err := w.getOrCreateSystem(ctx, event.System)
	if err != nil {
		return
	}
	sysid := database.EffectiveSYSID(sys)

	// Upsert unit (updates last_seen)
	unit, err := w.db.UpsertUnit(ctx, sysid, data.UnitID, "", "log")
	if err != nil {
		w.logger.Debug("Failed to upsert unit from log", zap.Error(err))
		return
	}

	// Record site association
	if unit != nil {
		w.db.UpsertUnitSite(ctx, unit.ID, sys.ID)
	}
}

// handleUnitAlias updates unit alpha tag
func (w *Watcher) handleUnitAlias(ctx context.Context, event LogEvent) {
	data := event.Data.(UnitAliasEvent)

	// We need a system context - use all systems in cache
	w.mu.RLock()
	systems := make([]*models.System, 0, len(w.systemCache))
	for _, sys := range w.systemCache {
		systems = append(systems, sys)
	}
	w.mu.RUnlock()

	// Update unit in all known systems (the alias applies to the radio itself)
	for _, sys := range systems {
		sysid := database.EffectiveSYSID(sys)
		unit, err := w.db.UpsertUnit(ctx, sysid, data.UnitID, data.AlphaTag, "log")
		if err == nil && unit != nil {
			w.db.UpsertUnitSite(ctx, unit.ID, sys.ID)
		}
	}

	w.logger.Debug("Unit alias discovered",
		zap.Int64("unit_id", data.UnitID),
		zap.String("alpha_tag", data.AlphaTag),
	)
}

// handleDecodeRate stores decode rate metrics
func (w *Watcher) handleDecodeRate(ctx context.Context, event LogEvent) {
	data := event.Data.(DecodeRateEvent)

	// Get or create system
	sys, err := w.getOrCreateSystem(ctx, data.System)
	if err != nil {
		return
	}

	// Broadcast rate update
	w.broadcast("rate_update", map[string]interface{}{
		"system":    data.System,
		"system_id": sys.ID,
		"rate":      data.MsgPerSec,
		"freq":      int64(data.Freq * 1e6),
	})

	w.logger.Debug("Decode rate",
		zap.String("system", data.System),
		zap.Int("system_id", sys.ID),
		zap.Int("msg_per_sec", data.MsgPerSec),
	)
}

// handleRecorder updates recorder status
func (w *Watcher) handleRecorder(ctx context.Context, event LogEvent) {
	data := event.Data.(RecorderEvent)

	// Convert state string to int (case-insensitive)
	stateInt := 0 // available
	switch strings.ToLower(data.State) {
	case "recording":
		stateInt = 1
	case "idle":
		stateInt = 2
	}

	// Update recorder state
	w.mu.Lock()
	w.recorders[data.RecorderNum] = &RecorderState{
		ID:         data.RecorderNum,
		SourceNum:  data.SourceNum,
		SourceFreq: data.SourceFreq,
		Type:       data.Type,
		State:      data.State,
		StateInt:   stateInt,
	}
	w.mu.Unlock()

	// Broadcast recorder update
	w.broadcast("recorder_update", map[string]interface{}{
		"id":        data.RecorderNum,
		"src_num":   data.SourceNum,
		"rec_num":   data.RecorderNum,
		"type":      data.Type,
		"state":     data.State,
		"state_int": stateInt,
	})
}

// GetRecorders returns current recorder states (implements rest.RecorderProvider)
func (w *Watcher) GetRecorders() interface{} {
	w.mu.RLock()
	defer w.mu.RUnlock()

	recorders := make([]*RecorderState, 0, len(w.recorders))
	for _, r := range w.recorders {
		recorders = append(recorders, r)
	}
	return recorders
}

// GetActiveCalls returns currently active calls (from log tracking)
func (w *Watcher) GetActiveCalls() []activeCall {
	w.mu.RLock()
	defer w.mu.RUnlock()

	calls := make([]activeCall, 0, len(w.activeCalls))
	for _, c := range w.activeCalls {
		calls = append(calls, *c)
	}
	return calls
}

// BackfillStatus returns the current backfill progress for audio and log files
func (w *Watcher) BackfillStatus() (audioDone bool, audioTotal, audioImported int64, logDone bool, logTotal, logImported int64) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.backfillDone, w.backfillTotal, w.backfillImported,
		w.logBackfillDone, w.logBackfillTotal, w.logBackfillImported
}

// runBackfill scans for historical files and imports them slowly
func (w *Watcher) runBackfill(ctx context.Context) {
	w.logger.Info("Starting backfill scan")

	// Phase 1: Audio files (JSON sidecars)
	w.runAudioBackfill(ctx)

	// Check for cancellation
	select {
	case <-ctx.Done():
		return
	default:
	}

	// Phase 2: Log files (if log path is configured)
	if w.logPath != "" {
		w.runLogBackfill(ctx)
	} else {
		w.mu.Lock()
		w.logBackfillDone = true
		w.mu.Unlock()
	}

	// Log tailing is already started in Start() - no Phase 3 needed
}

// runAudioBackfill imports historical audio JSON sidecar files
func (w *Watcher) runAudioBackfill(ctx context.Context) {
	// Collect all JSON files
	var jsonFiles []string
	err := filepath.WalkDir(w.audioPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors, continue walking
		}
		if !d.IsDir() && strings.HasSuffix(path, ".json") {
			jsonFiles = append(jsonFiles, path)
		}
		return nil
	})
	if err != nil {
		w.logger.Error("Audio backfill scan failed", zap.Error(err))
		return
	}

	w.mu.Lock()
	w.backfillTotal = int64(len(jsonFiles))
	w.mu.Unlock()

	if len(jsonFiles) == 0 {
		w.logger.Info("No historical audio files to backfill")
		w.mu.Lock()
		w.backfillDone = true
		w.mu.Unlock()
		return
	}

	w.logger.Info("Audio backfill found files",
		zap.Int("total", len(jsonFiles)),
	)

	// Process files slowly (10 per second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for _, jsonPath := range jsonFiles {
		select {
		case <-ctx.Done():
			w.logger.Info("Audio backfill interrupted",
				zap.Int64("imported", w.backfillImported),
				zap.Int64("total", w.backfillTotal),
			)
			return
		case <-ticker.C:
			w.processBackfillFile(ctx, jsonPath)
		}
	}

	w.mu.Lock()
	w.backfillDone = true
	w.mu.Unlock()

	w.logger.Info("Audio backfill complete",
		zap.Int64("imported", w.backfillImported),
		zap.Int64("skipped", w.backfillTotal-w.backfillImported),
	)
}

// runLogBackfill imports historical log files
func (w *Watcher) runLogBackfill(ctx context.Context) {
	// Log file pattern: MM-DD-YYYY_HHMM_NN.log
	reLogFile := regexp.MustCompile(`^\d{2}-\d{2}-\d{4}_\d{4}_\d{2}\.log$`)

	// Collect all log files
	entries, err := os.ReadDir(w.logPath)
	if err != nil {
		w.logger.Error("Log backfill scan failed", zap.Error(err))
		w.mu.Lock()
		w.logBackfillDone = true
		w.mu.Unlock()
		return
	}

	var logFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && reLogFile.MatchString(entry.Name()) {
			logFiles = append(logFiles, filepath.Join(w.logPath, entry.Name()))
		}
	}

	if len(logFiles) == 0 {
		w.logger.Info("No historical log files to backfill")
		w.mu.Lock()
		w.logBackfillDone = true
		w.mu.Unlock()
		return
	}

	// Sort by filename (oldest first - the date format sorts correctly)
	sort.Strings(logFiles)

	w.mu.Lock()
	w.logBackfillTotal = int64(len(logFiles))
	w.mu.Unlock()

	w.logger.Info("Log backfill found files",
		zap.Int("total", len(logFiles)),
		zap.String("oldest", filepath.Base(logFiles[0])),
		zap.String("newest", filepath.Base(logFiles[len(logFiles)-1])),
	)

	// Create a parser for log events
	parser := NewLogParser()

	// Process each log file
	for i, logPath := range logFiles {
		select {
		case <-ctx.Done():
			w.logger.Info("Log backfill interrupted",
				zap.Int64("imported", w.logBackfillImported),
				zap.Int64("total", w.logBackfillTotal),
			)
			return
		default:
		}

		eventsProcessed := w.processLogFile(ctx, logPath, parser)

		w.mu.Lock()
		w.logBackfillImported++
		w.mu.Unlock()

		w.logger.Debug("Processed historical log file",
			zap.String("file", filepath.Base(logPath)),
			zap.Int("events", eventsProcessed),
			zap.Int("progress", i+1),
			zap.Int("total", len(logFiles)),
		)

		// Reset parser state between files
		parser.Reset()
	}

	w.mu.Lock()
	w.logBackfillDone = true
	w.mu.Unlock()

	w.logger.Info("Log backfill complete",
		zap.Int64("files_processed", w.logBackfillImported),
	)
}

// processLogFile reads and processes a single historical log file
func (w *Watcher) processLogFile(ctx context.Context, logPath string, parser *LogParser) int {
	file, err := os.Open(logPath)
	if err != nil {
		w.logger.Debug("Failed to open log file", zap.String("path", logPath), zap.Error(err))
		return 0
	}
	defer file.Close()

	eventsProcessed := 0
	scanner := bufio.NewScanner(file)

	// Use a larger buffer for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return eventsProcessed
		default:
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		events := parser.ParseLine(line)
		for _, event := range events {
			w.handleLogEvent(ctx, event)
			eventsProcessed++
		}
	}

	if err := scanner.Err(); err != nil {
		w.logger.Debug("Error reading log file", zap.String("path", logPath), zap.Error(err))
	}

	return eventsProcessed
}

// processBackfillFile imports a single historical file
func (w *Watcher) processBackfillFile(ctx context.Context, jsonPath string) {
	// Read and parse JSON
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return
	}

	var sidecar storage.AudioSidecar
	if err := json.Unmarshal(data, &sidecar); err != nil {
		return
	}

	// Extract system name from path
	system := w.extractSystemName(jsonPath)
	if system == "" {
		return
	}

	// Get or create system
	sys, err := w.getOrCreateSystem(ctx, system)
	if err != nil {
		return
	}
	sysid := database.EffectiveSYSID(sys)

	// Check if already imported (by system, tgid, start_time)
	startTime := time.Unix(sidecar.StartTime, 0)
	existing, _ := w.db.GetCallBySystemTGIDAndTime(ctx, sys.ID, sidecar.Talkgroup, startTime)
	if existing != nil {
		// Already imported, skip
		return
	}

	// Get or create talkgroup
	tgID, err := w.getOrCreateTalkgroup(ctx, sysid, sys.ID, sidecar.Talkgroup, sidecar.TGTag, sidecar.TGDesc, sidecar.TGGroup, sidecar.TGGroupTag)
	if err != nil {
		return
	}

	// Find the audio file
	audioFile := strings.TrimSuffix(jsonPath, ".json")
	var audioPath string
	for _, ext := range []string{".m4a", ".wav", ".mp3"} {
		if _, err := os.Stat(audioFile + ext); err == nil {
			audioPath = w.getRelativePath(audioFile + ext)
			break
		}
	}

	if audioPath == "" {
		return // No audio file
	}

	// Get audio file size
	var audioSize int
	if fi, err := os.Stat(filepath.Join(w.audioPath, audioPath)); err == nil {
		audioSize = int(fi.Size())
	}

	// Create call record
	stopTime := time.Unix(sidecar.StopTime, 0)
	call := &models.Call{
		InstanceID:  1,
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
		SignalDB:    sidecar.SignalDB,
		NoiseDB:     sidecar.NoiseDB,
		AudioPath:   audioPath,
		AudioSize:   audioSize,
	}

	if err := w.db.InsertCall(ctx, call); err != nil {
		return
	}

	// Process transmissions
	for idx, src := range sidecar.SrcList {
		unit, err := w.db.UpsertUnit(ctx, sysid, src.Src, src.Tag, "backfill")
		if err != nil {
			continue
		}

		// Record site association
		if unit != nil {
			w.db.UpsertUnitSite(ctx, unit.ID, sys.ID)
		}

		var unitID *int
		if unit != nil {
			unitID = &unit.ID
		}

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
		w.db.InsertTransmission(ctx, tx)
	}

	// Process frequencies
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
		w.db.InsertCallFrequency(ctx, cf)
	}

	w.mu.Lock()
	w.backfillImported++
	w.mu.Unlock()
}
