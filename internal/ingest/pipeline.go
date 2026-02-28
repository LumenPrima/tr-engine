package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/snarg/tr-engine/internal/api"
	"github.com/snarg/tr-engine/internal/database"
	"github.com/snarg/tr-engine/internal/metrics"
	"github.com/snarg/tr-engine/internal/storage"
	"github.com/snarg/tr-engine/internal/transcribe"
)

// Pipeline processes incoming MQTT messages from trunk-recorder.
type Pipeline struct {
	db       *database.DB
	identity *IdentityResolver
	log      zerolog.Logger
	audioDir   string
	trAudioDir string // when set, skip saving audio files (served from TR's filesystem)
	store      storage.AudioStore
	uploader   *storage.AsyncUploader // nil if not async mode

	rawBatcher      *Batcher[database.RawMessageRow]
	recorderBatcher *Batcher[database.RecorderSnapshotRow]
	trunkingBatcher *Batcher[database.TrunkingMessageRow]

	// Active call tracking: tr_call_id → db call_id
	activeCalls *activeCallMap

	// Unit affiliation tracking: (system_id, unit_id) → current talkgroup
	affiliations *affiliationMap

	// Event bus for SSE subscribers
	eventBus *EventBus

	// Raw archival config
	rawStore   bool            // false = disable all raw archival
	rawInclude map[string]bool // if non-empty, allowlist mode (only these handlers)
	rawExclude map[string]bool // if non-empty, denylist mode (skip these handlers)

	// P25 system merging
	mergeP25Systems bool // when false, systems with same sysid/wacn stay separate

	// Transcription worker pool (optional, nil if WHISPER_URL not set)
	transcriber *transcribe.WorkerPool

	// File watcher (optional, nil if WATCH_DIR not set)
	watcher *FileWatcher

	// Recorder cache: recorder_id → latest state
	recorderCache sync.Map

	// TR instance status cache: instance_id → trInstanceStatusEntry
	trInstanceStatus sync.Map

	// Unit event dedup buffer: unitDedupKey → time.Time (first seen)
	unitEventDedup sync.Map

	// Warmup gate: buffer non-identity messages until system registration
	// establishes real sysid/wacn, preventing duplicate system creation
	// when calls arrive before system info on fresh start.
	warmupDone  atomic.Bool
	warmupMu    sync.Mutex
	warmupBuf   []bufferedMsg
	warmupTimer *time.Timer

	ctx    context.Context
	cancel context.CancelFunc

	msgCount     atomic.Int64
	handlerCount sync.Map // handler name → *atomic.Int64
}

// bufferedMsg holds a message deferred during warmup.
type bufferedMsg struct {
	route   *Route
	topic   string
	payload []byte
}

type PipelineOptions struct {
	DB               *database.DB
	AudioDir         string
	TRAudioDir       string
	Store            storage.AudioStore
	S3Uploader       *storage.AsyncUploader // nil if not async mode or no S3
	RawStore         bool
	RawIncludeTopics string
	RawExcludeTopics string
	MergeP25Systems  bool // auto-merge systems with same sysid/wacn (default true)
	TranscribeOpts   *transcribe.WorkerPoolOptions // nil = transcription disabled
	Log              zerolog.Logger
}

func NewPipeline(opts PipelineOptions) *Pipeline {
	ctx, cancel := context.WithCancel(context.Background())
	log := opts.Log.With().Str("component", "ingest").Logger()

	// Parse raw archival config
	rawStore := opts.RawStore
	rawInclude := parseHandlerSet(opts.RawIncludeTopics)
	rawExclude := parseHandlerSet(opts.RawExcludeTopics)

	if !rawStore {
		log.Info().Msg("raw message archival disabled (RAW_STORE=false)")
	} else if len(rawInclude) > 0 {
		names := make([]string, 0, len(rawInclude))
		for h := range rawInclude {
			names = append(names, h)
		}
		log.Info().Strs("handlers", names).Msg("raw message archival allowlist active")
	} else if len(rawExclude) > 0 {
		names := make([]string, 0, len(rawExclude))
		for h := range rawExclude {
			names = append(names, h)
		}
		log.Info().Strs("handlers", names).Msg("raw message archival excluded for handlers")
	}

	if !opts.MergeP25Systems {
		log.Info().Msg("P25 system auto-merge disabled (MERGE_P25_SYSTEMS=false)")
	}

	p := &Pipeline{
		db:              opts.DB,
		identity:        NewIdentityResolver(opts.DB, log),
		log:             log,
		audioDir:        opts.AudioDir,
		trAudioDir:      opts.TRAudioDir,
		store:           opts.Store,
		uploader:        opts.S3Uploader,
		rawStore:        rawStore,
		rawInclude:      rawInclude,
		rawExclude:      rawExclude,
		mergeP25Systems: opts.MergeP25Systems,
		activeCalls:  newActiveCallMap(),
		affiliations: newAffiliationMap(),
		eventBus:    NewEventBus(4096), // ~60s of events at high rate
		ctx:         ctx,
		cancel:      cancel,
	}

	// Transcription worker pool (optional)
	if opts.TranscribeOpts != nil {
		tOpts := opts.TranscribeOpts
		tOpts.PublishEvent = func(eventType string, systemID, tgid int, payload map[string]any) {
			p.PublishEvent(EventData{
				Type:     eventType,
				SystemID: systemID,
				Tgid:     tgid,
				Payload:  payload,
			})
		}
		p.transcriber = transcribe.NewWorkerPool(*tOpts)
	}

	p.rawBatcher = NewBatcher[database.RawMessageRow](100, 2*time.Second, p.flushRawMessages)
	p.recorderBatcher = NewBatcher[database.RecorderSnapshotRow](100, 2*time.Second, p.flushRecorderSnapshots)
	p.trunkingBatcher = NewBatcher[database.TrunkingMessageRow](100, 2*time.Second, p.flushTrunkingMessages)

	return p
}

// Start loads the identity cache and begins periodic stats logging and maintenance.
func (p *Pipeline) Start(ctx context.Context) error {
	if err := p.identity.LoadCache(ctx); err != nil {
		return err
	}

	// Skip warmup if identity cache already has entries (not a fresh DB).
	if p.identity.CacheLen() > 0 {
		p.warmupDone.Store(true)
		p.log.Info().Msg("identity cache populated, skipping warmup gate")
	} else {
		p.warmupTimer = time.AfterFunc(5*time.Second, func() {
			p.log.Warn().Msg("warmup timeout — processing buffered messages without full system identity")
			p.completeWarmup()
		})
		p.log.Info().Msg("warmup gate active — buffering calls until system registration arrives")
	}

	if err := p.backfillAffiliations(ctx); err != nil {
		p.log.Warn().Err(err).Msg("affiliation backfill failed, continuing with empty map")
	}
	go p.statsLoop()
	go p.maintenanceLoop()
	go p.talkgroupStatsLoop()
	go p.dedupCleanupLoop()
	go p.affiliationEvictionLoop()
	if p.transcriber != nil {
		p.transcriber.Start()
	}
	p.log.Info().Msg("ingest pipeline started")
	return nil
}

// StartWatcher creates and starts a file watcher on the given directory.
func (p *Pipeline) StartWatcher(watchDir, instanceID string, backfillDays int) error {
	fw := newFileWatcher(p, watchDir, instanceID, backfillDays)
	if err := fw.Start(); err != nil {
		return err
	}
	p.watcher = fw
	return nil
}

// ResolveIdentity resolves (or auto-creates) the system/site for a given
// instance ID and system name. Used by TR auto-discovery to resolve system IDs
// for talkgroup directory import.
func (p *Pipeline) ResolveIdentity(ctx context.Context, instanceID, sysName string) (*ResolvedIdentity, error) {
	return p.identity.Resolve(ctx, instanceID, sysName)
}

// WatcherStatus returns the file watcher status, or nil if not active.
func (p *Pipeline) WatcherStatus() *api.WatcherStatusData {
	if p.watcher == nil {
		return nil
	}
	return p.watcher.Status()
}

// TranscriptionStatus returns the transcription service status.
func (p *Pipeline) TranscriptionStatus() *api.TranscriptionStatusData {
	if p.transcriber == nil {
		return nil
	}
	return &api.TranscriptionStatusData{
		Status:  "ok",
		Model:   p.transcriber.Model(),
		Workers: p.transcriber.Workers(),
	}
}

// EnqueueTranscription enqueues a call for transcription by looking it up in the DB.
func (p *Pipeline) EnqueueTranscription(callID int64) bool {
	if p.transcriber == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
	defer cancel()

	c, err := p.db.GetCallForTranscription(ctx, callID)
	if err != nil {
		p.log.Warn().Err(err).Int64("call_id", callID).Msg("failed to load call for transcription")
		return false
	}
	return p.transcriber.Enqueue(transcribe.Job{
		CallID:        c.CallID,
		CallStartTime: c.StartTime,
		SystemID:      c.SystemID,
		Tgid:          c.Tgid,
		Duration:      derefFloat32(c.Duration),
		AudioFilePath: c.AudioFilePath,
		CallFilename:  c.CallFilename,
		SrcList:       c.SrcList,
		TgAlphaTag:    c.TgAlphaTag,
		TgDescription: c.TgDescription,
		TgTag:         c.TgTag,
		TgGroup:       c.TgGroup,
	})
}

// TranscriptionQueueStats returns transcription queue statistics.
func (p *Pipeline) TranscriptionQueueStats() *api.TranscriptionQueueStatsData {
	if p.transcriber == nil {
		return nil
	}
	stats := p.transcriber.Stats()
	return &api.TranscriptionQueueStatsData{
		Pending:   stats.Pending,
		Completed: stats.Completed,
		Failed:    stats.Failed,
	}
}

// enqueueTranscription is called by ingest handlers when a call has audio ready.
func (p *Pipeline) enqueueTranscription(callID int64, startTime time.Time, systemID int, audioFilePath string, meta *AudioMetadata) {
	if p.transcriber == nil {
		return
	}
	dur := float32(meta.CallLength)
	if dur < float32(p.transcriber.MinDuration()) || dur > float32(p.transcriber.MaxDuration()) {
		return
	}
	job := transcribe.Job{
		CallID:        callID,
		CallStartTime: startTime,
		SystemID:      systemID,
		Tgid:          meta.Talkgroup,
		Duration:      dur,
		AudioFilePath: audioFilePath,
		CallFilename:  meta.Filename,
		TgAlphaTag:    meta.TalkgroupTag,
		TgDescription: meta.TalkgroupDesc,
		TgTag:         meta.TalkgroupGroupTag,
		TgGroup:       meta.TalkgroupGroup,
	}
	// Try to get src_list from metadata
	if len(meta.SrcList) > 0 {
		if raw, err := json.Marshal(meta.SrcList); err == nil {
			job.SrcList = raw
		}
	}
	if !p.transcriber.Enqueue(job) {
		p.log.Warn().Int64("call_id", callID).Msg("transcription queue full, skipping")
	}
}

// insertSourceTranscription inserts a pre-generated transcript directly, bypassing the STT queue.
func (p *Pipeline) insertSourceTranscription(callID int64, startTime time.Time, systemID, tgid int, meta *AudioMetadata) {
	ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
	defer cancel()

	text := strings.TrimSpace(meta.Transcript)
	if text == "" {
		return
	}

	wordCount := len(strings.Fields(text))

	row := &database.TranscriptionRow{
		CallID:        callID,
		CallStartTime: startTime,
		Text:          text,
		Source:        "auto",
		IsPrimary:     true,
		Provider:      "source",
		WordCount:     wordCount,
		Words:         meta.TranscriptWords, // nil if not provided
	}

	id, err := p.db.InsertTranscription(ctx, row)
	if err != nil {
		p.log.Warn().Err(err).Int64("call_id", callID).Msg("failed to insert source transcript")
		return
	}

	p.log.Debug().Int64("call_id", callID).Int("transcription_id", id).Msg("source transcript inserted")

	// Publish SSE event
	p.PublishEvent(EventData{
		Type:     "transcription",
		SystemID: systemID,
		Tgid:     tgid,
		Payload: map[string]any{
			"call_id":    callID,
			"system_id":  systemID,
			"tgid":       tgid,
			"text":       text,
			"word_count": wordCount,
			"source":     "source",
		},
	})
}

func derefFloat32(p *float32) float32 {
	if p == nil {
		return 0
	}
	return *p
}

// Stop flushes batchers and cancels the context.
func (p *Pipeline) Stop() {
	p.log.Info().Int64("total_messages", p.msgCount.Load()).Msg("ingest pipeline stopping")
	if p.warmupTimer != nil {
		p.warmupTimer.Stop()
	}
	if p.watcher != nil {
		p.watcher.Stop()
	}
	if p.transcriber != nil {
		p.transcriber.Stop()
	}
	if p.uploader != nil {
		p.uploader.Stop()
	}
	p.rawBatcher.Stop()
	p.recorderBatcher.Stop()
	p.trunkingBatcher.Stop()
	p.cancel()
}

// statsLoop logs message counts every 60 seconds.
func (p *Pipeline) statsLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	var lastTotal int64
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			total := p.msgCount.Load()
			delta := total - lastTotal
			lastTotal = total

			evt := p.log.Info().
				Int64("total", total).
				Int64("last_60s", delta).
				Int("active_calls", p.activeCalls.Len())

			// Collect per-handler counts
			p.handlerCount.Range(func(key, value any) bool {
				evt = evt.Int64(key.(string), value.(*atomic.Int64).Load())
				return true
			})

			evt.Msg("stats")
		}
	}
}

// maintenanceLoop runs partition creation, decimation, and purging on a daily schedule.
// It runs once immediately on startup to ensure partitions exist, then every 24 hours.
func (p *Pipeline) maintenanceLoop() {
	p.runMaintenance()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.runMaintenance()
		}
	}
}

func (p *Pipeline) runMaintenance() {
	log := p.log.With().Str("task", "maintenance").Logger()
	start := time.Now()
	log.Info().Msg("partition maintenance starting")

	ctx, cancel := context.WithTimeout(p.ctx, 5*time.Minute)
	defer cancel()

	// 1. Create monthly partitions 3 months ahead
	monthlyTables := []string{"calls", "call_frequencies", "call_transmissions", "unit_events", "trunking_messages"}
	for _, table := range monthlyTables {
		partDate := beginningOfMonth(time.Now()).AddDate(0, 3, 0)
		result, err := p.db.CreateMonthlyPartition(ctx, table, partDate)
		if err != nil {
			log.Warn().Err(err).Str("table", table).Msg("failed to create monthly partition")
		} else {
			log.Debug().Str("result", result).Str("table", table).Msg("monthly partition")
		}
	}

	// 2. Create weekly partitions 3 weeks ahead
	for weekOffset := 0; weekOffset <= 3; weekOffset++ {
		weekDate := time.Now().AddDate(0, 0, weekOffset*7)
		result, err := p.db.CreateWeeklyPartition(ctx, "mqtt_raw_messages", weekDate)
		if err != nil {
			log.Warn().Err(err).Int("week_offset", weekOffset).Msg("failed to create weekly partition")
		} else {
			log.Debug().Str("result", result).Msg("weekly partition")
		}
	}

	// 3. Decimate state tables
	for _, spec := range []struct{ table, col string }{
		{"recorder_snapshots", "time"},
		{"decode_rates", "time"},
	} {
		result, err := p.db.DecimateStateTable(ctx, spec.table, spec.col)
		if err != nil {
			log.Warn().Err(err).Str("table", spec.table).Msg("decimation failed")
		} else if result.Deleted1w > 0 || result.Deleted1m > 0 {
			log.Info().
				Str("table", spec.table).
				Int64("deleted_1w", result.Deleted1w).
				Int64("deleted_1m", result.Deleted1m).
				Msg("decimation complete")
		}
	}

	// 4. Purge expired data
	for _, spec := range []struct {
		table     string
		col       string
		retention time.Duration
	}{
		{"console_messages", "log_time", 30 * 24 * time.Hour},
		{"plugin_statuses", "time", 30 * 24 * time.Hour},
		{"call_active_checkpoints", "snapshot_time", 7 * 24 * time.Hour},
	} {
		n, err := p.db.PurgeOlderThan(ctx, spec.table, spec.col, spec.retention)
		if err != nil {
			log.Warn().Err(err).Str("table", spec.table).Msg("purge failed")
		} else if n > 0 {
			log.Info().Str("table", spec.table).Int64("deleted", n).Msg("purged old rows")
		}
	}

	// 5. Drop old weekly partitions (raw MQTT, 7-day retention)
	dropped, err := p.db.DropOldWeeklyPartitions(ctx, "mqtt_raw_messages", 7*24*time.Hour)
	if err != nil {
		log.Warn().Err(err).Msg("failed to drop old weekly partitions")
	}
	for _, name := range dropped {
		log.Info().Str("partition", name).Msg("dropped old weekly partition")
	}

	// 6. Purge stale RECORDING calls (call_start with no call_end or audio after 1 hour)
	stalePurged, err := p.db.PurgeStaleCalls(ctx, 1*time.Hour)
	if err != nil {
		log.Warn().Err(err).Msg("failed to purge stale calls")
	} else if stalePurged > 0 {
		log.Info().Int64("deleted", stalePurged).Msg("purged stale RECORDING calls")
	}

	// 7. Clean up orphaned call_groups (no calls reference them)
	orphansPurged, err := p.db.PurgeOrphanCallGroups(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to purge orphan call_groups")
	} else if orphansPurged > 0 {
		log.Info().Int64("deleted", orphansPurged).Msg("purged orphan call_groups")
	}

	// 8. Expire stale entries from in-memory active calls map (calls older than 1 hour)
	staleMapEntries := 0
	for trCallID, entry := range p.activeCalls.All() {
		if time.Since(entry.StartTime) > 1*time.Hour {
			p.activeCalls.Delete(trCallID)
			staleMapEntries++
		}
	}
	if staleMapEntries > 0 {
		log.Info().Int("expired", staleMapEntries).Msg("expired stale active calls from memory")
	}

	log.Info().Dur("elapsed_ms", time.Since(start)).Msg("partition maintenance complete")
}

// talkgroupStatsLoop refreshes cached talkgroup stats on two cadences:
// - Hot (calls_1h, calls_24h): every 5 minutes, scans only 24h of calls
// - Cold (call_count_30d, unit_count_30d): every hour, scans 30 days
func (p *Pipeline) talkgroupStatsLoop() {
	log := p.log.With().Str("task", "tg-stats").Logger()

	// Initial refresh: both hot and cold on startup
	p.refreshTalkgroupStatsHot(log)
	p.refreshTalkgroupStatsCold(log)

	hotTicker := time.NewTicker(5 * time.Minute)
	coldTicker := time.NewTicker(1 * time.Hour)
	defer hotTicker.Stop()
	defer coldTicker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-coldTicker.C:
			p.refreshTalkgroupStatsCold(log)
		case <-hotTicker.C:
			p.refreshTalkgroupStatsHot(log)
		}
	}
}

func (p *Pipeline) refreshTalkgroupStatsHot(log zerolog.Logger) {
	ctx, cancel := context.WithTimeout(p.ctx, 2*time.Minute)
	defer cancel()

	updated, err := p.db.RefreshTalkgroupStatsHot(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("talkgroup stats hot refresh failed")
		return
	}
	if updated > 0 {
		log.Info().Int64("updated", updated).Msg("talkgroup stats hot refreshed")
	}
}

func (p *Pipeline) refreshTalkgroupStatsCold(log zerolog.Logger) {
	ctx, cancel := context.WithTimeout(p.ctx, 5*time.Minute)
	defer cancel()

	updated, err := p.db.RefreshTalkgroupStatsCold(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("talkgroup stats cold refresh failed")
		return
	}
	if updated > 0 {
		log.Info().Int64("updated", updated).Msg("talkgroup stats cold refreshed")
	}
}

// unitDedupKey identifies a unique unit event for deduplication across sites.
// No time bucket — the dedup window is controlled by the 10-second cleanup loop.
// This avoids boundary artifacts where events 1-2s apart straddle a fixed bucket edge.
type unitDedupKey struct {
	SystemID  int
	UnitID    int
	EventType string
	Tgid      int
}

// dedupCleanupLoop sweeps expired entries from the unit event dedup buffer every 10 seconds.
func (p *Pipeline) dedupCleanupLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.unitEventDedup.Range(func(key, value any) bool {
				if time.Since(value.(time.Time)) > 10*time.Second {
					p.unitEventDedup.Delete(key)
				}
				return true
			})
		}
	}
}

func (p *Pipeline) affiliationEvictionLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			if n := p.affiliations.EvictStale(24 * time.Hour); n > 0 {
				p.log.Debug().Int("evicted", n).Msg("affiliation map eviction")
			}
		}
	}
}

// beginningOfMonth returns the first day of the month for the given time.
func beginningOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// ensurePartitionsFor creates monthly partitions for all partitioned tables for
// the month containing the given timestamp. Called on-demand when an insert
// fails with "no partition found".
func (p *Pipeline) ensurePartitionsFor(t time.Time) {
	ctx, cancel := context.WithTimeout(p.ctx, 30*time.Second)
	defer cancel()

	month := beginningOfMonth(t)
	tables := []string{"calls", "call_frequencies", "call_transmissions", "unit_events", "trunking_messages"}
	for _, table := range tables {
		result, err := p.db.CreateMonthlyPartition(ctx, table, month)
		if err != nil {
			p.log.Warn().Err(err).Str("table", table).Time("month", month).Msg("failed to create on-demand partition")
		} else if !strings.Contains(result, "already exists") {
			p.log.Info().Str("result", result).Str("table", table).Msg("created on-demand partition")
		}
	}
}

// HandleMessage is the entry point called by the MQTT client for each message.
func (p *Pipeline) HandleMessage(topic string, payload []byte) {
	p.msgCount.Add(1)
	metrics.MQTTMessagesTotal.Inc()

	route := ParseTopic(topic)

	// Best-effort extract instance_id for archival
	var env Envelope
	_ = json.Unmarshal(payload, &env)

	// Archive raw message
	if route == nil {
		p.archiveRaw("_unknown", topic, payload, env.InstanceID)
		p.log.Warn().Str("topic", topic).Msg("unknown topic, skipping")
		return
	}

	p.archiveRaw(route.Handler, topic, payload, env.InstanceID)

	// Track instance as connected on any message (not just trunk_recorder/status)
	if env.InstanceID != "" {
		p.UpdateTRInstanceStatus(env.InstanceID, "connected", time.Now())
	}

	// Dispatch to handler
	p.dispatch(route, topic, payload, &env)
}

func (p *Pipeline) incHandler(name string) {
	v, _ := p.handlerCount.LoadOrStore(name, &atomic.Int64{})
	v.(*atomic.Int64).Add(1)
	metrics.MQTTHandlerMessagesTotal.WithLabelValues(name).Inc()
}

func (p *Pipeline) dispatch(route *Route, topic string, payload []byte, env *Envelope) {
	// Warmup gate: buffer non-identity messages until system registration arrives
	if !p.warmupDone.Load() {
		switch route.Handler {
		case "systems", "system", "config", "status":
			// Identity-establishing handlers pass through during warmup
		default:
			p.warmupMu.Lock()
			if !p.warmupDone.Load() {
				p.warmupBuf = append(p.warmupBuf, bufferedMsg{
					route:   route,
					topic:   topic,
					payload: append([]byte(nil), payload...), // copy — may be reused
				})
				p.warmupMu.Unlock()
				return
			}
			p.warmupMu.Unlock()
		}
	}

	p.incHandler(route.Handler)
	var err error

	switch route.Handler {
	case "status":
		err = p.handleStatus(payload)
	case "systems":
		err = p.handleSystems(payload)
	case "system":
		err = p.handleSystem(payload)
	case "call_start":
		err = p.handleCallStart(payload)
	case "call_end":
		err = p.handleCallEnd(payload)
	case "calls_active":
		err = p.handleCallsActive(payload)
	case "audio":
		err = p.handleAudio(payload)
	case "recorders":
		err = p.handleRecorders(payload)
	case "recorder":
		err = p.handleRecorder(payload)
	case "rates":
		err = p.handleRates(payload)
	case "config":
		err = p.handleConfig(payload)
	case "trunking_message":
		err = p.handleTrunkingMessage(topic, payload)
	case "console":
		err = p.handleConsoleLog(payload)
	case "unit_event":
		err = p.handleUnitEvent(topic, payload)
	default:
		p.log.Warn().Str("handler", route.Handler).Msg("no handler for route")
		return
	}

	if err != nil {
		p.log.Error().Err(err).
			Str("handler", route.Handler).
			Str("topic", topic).
			Msg("handler error")
	}
}

// completeWarmup ends the warmup gate and replays buffered messages.
// Safe to call multiple times — only the first call has effect.
func (p *Pipeline) completeWarmup() {
	p.warmupMu.Lock()
	if p.warmupDone.Load() {
		p.warmupMu.Unlock()
		return
	}
	p.warmupDone.Store(true)
	if p.warmupTimer != nil {
		p.warmupTimer.Stop()
	}
	buf := p.warmupBuf
	p.warmupBuf = nil
	p.warmupMu.Unlock()

	p.log.Info().Int("buffered_messages", len(buf)).Msg("warmup complete, replaying buffered messages")
	for _, msg := range buf {
		var env Envelope
		_ = json.Unmarshal(msg.payload, &env)
		p.dispatch(msg.route, msg.topic, msg.payload, &env)
	}
}

func (p *Pipeline) flushRawMessages(rows []database.RawMessageRow) {
	ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
	defer cancel()

	n, err := p.db.InsertRawMessages(ctx, rows)
	if err != nil {
		p.log.Error().Err(err).Int("count", len(rows)).Msg("failed to flush raw messages")
		return
	}
	p.log.Debug().Int64("inserted", n).Msg("flushed raw messages")
}

func (p *Pipeline) flushTrunkingMessages(rows []database.TrunkingMessageRow) {
	ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
	defer cancel()

	n, err := p.db.InsertTrunkingMessages(ctx, rows)
	if err != nil {
		p.log.Error().Err(err).Int("count", len(rows)).Msg("failed to flush trunking messages")
		return
	}
	p.log.Debug().Int64("inserted", n).Msg("flushed trunking messages")
}

func (p *Pipeline) flushRecorderSnapshots(rows []database.RecorderSnapshotRow) {
	ctx, cancel := context.WithTimeout(p.ctx, 10*time.Second)
	defer cancel()

	n, err := p.db.InsertRecorderSnapshots(ctx, rows)
	if err != nil {
		p.log.Error().Err(err).Int("count", len(rows)).Msg("failed to flush recorder snapshots")
		return
	}
	p.log.Debug().Int64("inserted", n).Msg("flushed recorder snapshots")
}

// archiveRaw conditionally stores a message in mqtt_raw_messages based on the
// raw archival config: RAW_STORE, RAW_INCLUDE_TOPICS, RAW_EXCLUDE_TOPICS.
// Use handler="_unknown" for unrecognized topics.
func (p *Pipeline) archiveRaw(handler, topic string, payload []byte, instanceID string) {
	if !p.rawStore {
		return
	}
	if len(p.rawInclude) > 0 {
		if !p.rawInclude[handler] {
			return
		}
	} else if p.rawExclude[handler] {
		return
	}

	rawPayload := payload
	if handler == "audio" {
		rawPayload = stripAudioBase64(payload)
	}
	p.rawBatcher.Add(database.RawMessageRow{
		Topic:      topic,
		Payload:    rawPayload,
		ReceivedAt: time.Now(),
		InstanceID: instanceID,
	})
}

// parseHandlerSet splits a comma-separated string into a set of handler names.
func parseHandlerSet(s string) map[string]bool {
	m := make(map[string]bool)
	if s == "" {
		return m
	}
	for _, h := range strings.Split(s, ",") {
		h = strings.TrimSpace(h)
		if h != "" {
			m[h] = true
		}
	}
	return m
}

// stripAudioBase64 removes the base64 audio data from audio message payloads
// before storing in mqtt_raw_messages. The audio is already saved to disk by
// the audio handler, so keeping it in the DB is pure waste (~60KB per message).
func stripAudioBase64(payload []byte) []byte {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(payload, &obj); err != nil {
		return payload
	}
	callRaw, ok := obj["call"]
	if !ok {
		return payload
	}
	var call map[string]json.RawMessage
	if err := json.Unmarshal(callRaw, &call); err != nil {
		return payload
	}
	delete(call, "audio_m4a_base64")
	delete(call, "audio_wav_base64")
	callBytes, err := json.Marshal(call)
	if err != nil {
		return payload
	}
	obj["call"] = callBytes
	out, err := json.Marshal(obj)
	if err != nil {
		return payload
	}
	return out
}

// activeCallMap tracks in-flight calls: tr_call_id → call metadata for API display.
type activeCallEntry struct {
	CallID        int64
	StartTime     time.Time
	SystemID      int
	SystemName    string
	Sysid         string
	SiteID        *int
	SiteShortName string
	Tgid          int
	TgAlphaTag    string
	TgDescription string
	TgTag         string
	TgGroup       string
	Unit          int
	UnitAlphaTag  string
	Freq          int64
	Emergency     bool
	Encrypted     bool
	Analog        bool
	Conventional  bool
	Phase2TDMA    bool
	AudioType     string
}

type activeCallMap struct {
	mu    sync.Mutex
	calls map[string]activeCallEntry
}

func newActiveCallMap() *activeCallMap {
	return &activeCallMap{calls: make(map[string]activeCallEntry)}
}

func (m *activeCallMap) Set(trCallID string, entry activeCallEntry) {
	m.mu.Lock()
	m.calls[trCallID] = entry
	m.mu.Unlock()
}

func (m *activeCallMap) Get(trCallID string) (activeCallEntry, bool) {
	m.mu.Lock()
	e, ok := m.calls[trCallID]
	m.mu.Unlock()
	return e, ok
}

func (m *activeCallMap) Delete(trCallID string) {
	m.mu.Lock()
	delete(m.calls, trCallID)
	m.mu.Unlock()
}

// FindByTgidAndTime finds an active call matching the given tgid with a start
// time within tolerance. Returns the map key, entry, and whether found. This
// handles trunk-recorder shifting start_time by 1-2s between call_start and
// call_end, which changes the ID since it embeds start_time.
//
// When multiple calls match, prefers the one whose start_time is at or before
// the reported startTime (the original call), breaking ties by closest time
// difference. This prevents matching a newer back-to-back call on the same
// talkgroup when the original call's start_time shifted forward.
func (m *activeCallMap) FindByTgidAndTime(tgid int, startTime time.Time, tolerance time.Duration) (string, activeCallEntry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var bestKey string
	var bestEntry activeCallEntry
	bestDiff := tolerance + 1
	bestIsBeforeOrAt := false

	for key, entry := range m.calls {
		if entry.Tgid != tgid {
			continue
		}
		diff := entry.StartTime.Sub(startTime)
		absDiff := diff
		if absDiff < 0 {
			absDiff = -absDiff
		}
		if absDiff > tolerance {
			continue
		}

		// Prefer calls that started at or before the reported time (the
		// original call shifted forward), over calls that started after
		// (a newer back-to-back call on the same talkgroup).
		isBeforeOrAt := diff <= 0
		better := false
		if isBeforeOrAt && !bestIsBeforeOrAt {
			better = true // before-or-at always beats after
		} else if isBeforeOrAt == bestIsBeforeOrAt {
			better = absDiff < bestDiff // same category: pick closest
		}

		if better {
			bestKey = key
			bestEntry = entry
			bestDiff = absDiff
			bestIsBeforeOrAt = isBeforeOrAt
		}
	}

	return bestKey, bestEntry, bestDiff <= tolerance
}

// FindByFreq returns the first active call on the given frequency, if any.
func (m *activeCallMap) FindByFreq(freq int64) (activeCallEntry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.calls {
		if e.Freq == freq {
			return e, true
		}
	}
	return activeCallEntry{}, false
}

func (m *activeCallMap) Len() int {
	m.mu.Lock()
	n := len(m.calls)
	m.mu.Unlock()
	return n
}

// All returns a snapshot of all active call entries.
func (m *activeCallMap) All() map[string]activeCallEntry {
	m.mu.Lock()
	result := make(map[string]activeCallEntry, len(m.calls))
	for k, v := range m.calls {
		result[k] = v
	}
	m.mu.Unlock()
	return result
}

// affiliationEntry tracks a unit's current talkgroup affiliation.
type affiliationEntry struct {
	SystemID        int
	SystemName      string
	Sysid           string
	UnitID          int
	UnitAlphaTag    string
	Tgid            int
	TgAlphaTag      string
	TgDescription   string
	TgTag           string
	TgGroup         string
	PreviousTgid    *int
	AffiliatedSince time.Time
	LastEventTime   time.Time
	Status          string // "affiliated" or "off"
}

type affiliationKey struct {
	SystemID int
	UnitID   int
}

type affiliationMap struct {
	mu    sync.Mutex
	items map[affiliationKey]*affiliationEntry
}

func newAffiliationMap() *affiliationMap {
	return &affiliationMap{items: make(map[affiliationKey]*affiliationEntry)}
}

// Update sets or overwrites an affiliation entry (used for "join" events).
func (m *affiliationMap) Update(key affiliationKey, entry *affiliationEntry) {
	m.mu.Lock()
	m.items[key] = entry
	m.mu.Unlock()
}

// MarkOff marks a unit as disconnected without removing it from the map.
func (m *affiliationMap) MarkOff(key affiliationKey, t time.Time) {
	m.mu.Lock()
	if e, ok := m.items[key]; ok {
		e.Status = "off"
		e.LastEventTime = t
	}
	m.mu.Unlock()
}

// UpdateActivity updates the LastEventTime for an existing entry.
func (m *affiliationMap) UpdateActivity(key affiliationKey, t time.Time) {
	m.mu.Lock()
	if e, ok := m.items[key]; ok {
		e.LastEventTime = t
	}
	m.mu.Unlock()
}

// Get returns a copy of the entry if it exists.
func (m *affiliationMap) Get(key affiliationKey) (*affiliationEntry, bool) {
	m.mu.Lock()
	e, ok := m.items[key]
	if ok {
		copy := *e
		m.mu.Unlock()
		return &copy, true
	}
	m.mu.Unlock()
	return nil, false
}

// All returns a snapshot of all affiliation entries.
func (m *affiliationMap) All() []affiliationEntry {
	m.mu.Lock()
	result := make([]affiliationEntry, 0, len(m.items))
	for _, e := range m.items {
		result = append(result, *e)
	}
	m.mu.Unlock()
	return result
}

// EvictStale removes entries whose LastEventTime is older than maxAge.
// Returns the number of entries evicted.
func (m *affiliationMap) EvictStale(maxAge time.Duration) int {
	cutoff := time.Now().Add(-maxAge)
	m.mu.Lock()
	evicted := 0
	for k, e := range m.items {
		if e.LastEventTime.Before(cutoff) {
			delete(m.items, k)
			evicted++
		}
	}
	m.mu.Unlock()
	return evicted
}

// ----- LiveDataSource interface implementation -----

// ActiveCalls returns currently in-progress calls.
func (p *Pipeline) ActiveCalls() []api.ActiveCallData {
	entries := p.activeCalls.All()
	calls := make([]api.ActiveCallData, 0, len(entries))
	for _, e := range entries {
		calls = append(calls, api.ActiveCallData{
			CallID:        e.CallID,
			SystemID:      e.SystemID,
			SystemName:    e.SystemName,
			Sysid:         e.Sysid,
			SiteID:        e.SiteID,
			SiteShortName: e.SiteShortName,
			Tgid:          e.Tgid,
			TgAlphaTag:    e.TgAlphaTag,
			TgDescription: e.TgDescription,
			TgTag:         e.TgTag,
			TgGroup:       e.TgGroup,
			StartTime:     e.StartTime,
			Duration:      float32(time.Since(e.StartTime).Seconds()),
			Freq:          e.Freq,
			Emergency:     e.Emergency,
			Encrypted:     e.Encrypted,
			Analog:        e.Analog,
			Conventional:  e.Conventional,
			Phase2TDMA:    e.Phase2TDMA,
			AudioType:     e.AudioType,
		})
	}
	return calls
}

// LatestRecorders returns the most recent recorder state snapshot.
func (p *Pipeline) LatestRecorders() []api.RecorderStateData {
	var recorders []api.RecorderStateData
	p.recorderCache.Range(func(key, value any) bool {
		if r, ok := value.(api.RecorderStateData); ok {
			recorders = append(recorders, r)
		}
		return true
	})
	return recorders
}

// Subscribe registers a new SSE subscriber with the given filter.
func (p *Pipeline) Subscribe(filter api.EventFilter) (<-chan api.SSEEvent, func()) {
	return p.eventBus.Subscribe(filter)
}

// ReplaySince returns buffered events since the given event ID.
func (p *Pipeline) ReplaySince(lastEventID string, filter api.EventFilter) []api.SSEEvent {
	return p.eventBus.ReplaySince(lastEventID, filter)
}

// RewriteSystemID updates the identity cache after a system merge,
// rewriting all entries that point at oldSystemID to use newSystemID.
func (p *Pipeline) RewriteSystemID(oldSystemID, newSystemID int) {
	p.identity.RewriteSystemID(oldSystemID, newSystemID)
}

// MsgCount returns the total number of MQTT messages processed.
func (p *Pipeline) MsgCount() int64 {
	return p.msgCount.Load()
}

// HandlerCounts returns a snapshot of per-handler message counts.
func (p *Pipeline) HandlerCounts() map[string]int64 {
	counts := make(map[string]int64)
	p.handlerCount.Range(func(key, value any) bool {
		counts[key.(string)] = value.(*atomic.Int64).Load()
		return true
	})
	return counts
}

// ActiveCallCount returns the number of currently in-progress calls.
func (p *Pipeline) ActiveCallCount() int {
	return p.activeCalls.Len()
}

// SSESubscriberCount returns the number of active SSE subscribers.
func (p *Pipeline) SSESubscriberCount() int {
	return p.eventBus.SubscriberCount()
}

// IngestMetrics returns pipeline state for Prometheus metrics.
func (p *Pipeline) IngestMetrics() *api.IngestMetricsData {
	return &api.IngestMetricsData{
		MsgCount:       p.MsgCount(),
		ActiveCalls:    p.ActiveCallCount(),
		HandlerCounts:  p.HandlerCounts(),
		SSESubscribers: p.SSESubscriberCount(),
	}
}

// PublishEvent is a convenience method to publish an event through the event bus.
func (p *Pipeline) PublishEvent(e EventData) {
	if p.eventBus != nil {
		p.eventBus.Publish(e)
	}
}

// trInstanceStatusEntry caches the last-seen status for a TR instance.
type trInstanceStatusEntry struct {
	Status   string
	LastSeen time.Time
}

// UpdateTRInstanceStatus caches the latest status for a TR instance.
func (p *Pipeline) UpdateTRInstanceStatus(instanceID, status string, t time.Time) {
	p.trInstanceStatus.Store(instanceID, trInstanceStatusEntry{
		Status:   status,
		LastSeen: t,
	})
}

// TRInstanceStatus returns the cached status of all known TR instances.
func (p *Pipeline) TRInstanceStatus() []api.TRInstanceStatusData {
	var result []api.TRInstanceStatusData
	p.trInstanceStatus.Range(func(key, value any) bool {
		entry := value.(trInstanceStatusEntry)
		result = append(result, api.TRInstanceStatusData{
			InstanceID: key.(string),
			Status:     entry.Status,
			LastSeen:   entry.LastSeen,
		})
		return true
	})
	return result
}

// backfillAffiliations loads recent join events from the DB to populate the affiliation map on startup.
func (p *Pipeline) backfillAffiliations(ctx context.Context) error {
	start := time.Now()

	rows, err := p.db.LoadRecentAffiliations(ctx)
	if err != nil {
		return fmt.Errorf("load recent affiliations: %w", err)
	}

	talkgroups := make(map[int]struct{})
	var offCount int
	for _, r := range rows {
		key := affiliationKey{SystemID: r.SystemID, UnitID: r.UnitRID}
		status := "affiliated"
		if r.WentOff {
			status = "off"
			offCount++
		}
		p.affiliations.Update(key, &affiliationEntry{
			SystemID:        r.SystemID,
			SystemName:      r.SystemName,
			Sysid:           r.Sysid,
			UnitID:          r.UnitRID,
			UnitAlphaTag:    r.UnitAlphaTag,
			Tgid:            r.Tgid,
			TgAlphaTag:      r.TgAlphaTag,
			TgDescription:   r.TgDescription,
			TgTag:           r.TgTag,
			TgGroup:         r.TgGroup,
			AffiliatedSince: r.Time,
			LastEventTime:   r.Time,
			Status:          status,
		})
		talkgroups[r.Tgid] = struct{}{}
	}

	p.log.Info().
		Int("units", len(rows)).
		Int("affiliated", len(rows)-offCount).
		Int("off", offCount).
		Int("talkgroups", len(talkgroups)).
		Dur("elapsed_ms", time.Since(start)).
		Msg("affiliation map backfilled from DB")
	return nil
}

// UnitAffiliations returns the current talkgroup affiliation state for all tracked units.
func (p *Pipeline) UnitAffiliations() []api.UnitAffiliationData {
	entries := p.affiliations.All()
	result := make([]api.UnitAffiliationData, 0, len(entries))
	for _, e := range entries {
		result = append(result, api.UnitAffiliationData{
			SystemID:        e.SystemID,
			SystemName:      e.SystemName,
			Sysid:           e.Sysid,
			UnitID:          e.UnitID,
			UnitAlphaTag:    e.UnitAlphaTag,
			Tgid:            e.Tgid,
			TgAlphaTag:      e.TgAlphaTag,
			TgDescription:   e.TgDescription,
			TgTag:           e.TgTag,
			TgGroup:         e.TgGroup,
			PreviousTgid:    e.PreviousTgid,
			AffiliatedSince: e.AffiliatedSince,
			LastEventTime:   e.LastEventTime,
			Status:          e.Status,
		})
	}
	return result
}

// UpdateRecorderCache stores the latest recorder state in the in-memory cache,
// enriched with active call data (talkgroup, unit) by matching frequency.
func (p *Pipeline) UpdateRecorderCache(instanceID string, rec database.RecorderSnapshotRow) {
	key := fmt.Sprintf("%s_%d_%d", instanceID, rec.SrcNum, rec.RecNum)
	data := api.RecorderStateData{
		ID:         rec.RecorderID,
		InstanceID: instanceID,
		SrcNum:     rec.SrcNum,
		RecNum:     rec.RecNum,
		Type:       rec.Type,
		RecState:   rec.RecStateType,
		Freq:       rec.Freq,
		Duration:   rec.Duration,
		Count:      rec.Count,
		Squelched:  rec.Squelched,
	}
	if rec.Freq > 0 {
		if call, ok := p.activeCalls.FindByFreq(rec.Freq); ok {
			data.Tgid = &call.Tgid
			data.TgAlphaTag = &call.TgAlphaTag
			data.UnitID = &call.Unit
			data.UnitAlphaTag = &call.UnitAlphaTag
		}
	}
	p.recorderCache.Store(key, data)
}
