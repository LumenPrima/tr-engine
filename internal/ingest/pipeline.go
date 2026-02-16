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
)

// Pipeline processes incoming MQTT messages from trunk-recorder.
type Pipeline struct {
	db       *database.DB
	identity *IdentityResolver
	log      zerolog.Logger
	audioDir string

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

	// Recorder cache: recorder_id → latest state
	recorderCache sync.Map

	// TR instance status cache: instance_id → trInstanceStatusEntry
	trInstanceStatus sync.Map

	ctx    context.Context
	cancel context.CancelFunc

	msgCount     atomic.Int64
	handlerCount sync.Map // handler name → *atomic.Int64
}

type PipelineOptions struct {
	DB               *database.DB
	AudioDir         string
	RawStore         bool
	RawIncludeTopics string
	RawExcludeTopics string
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

	p := &Pipeline{
		db:          opts.DB,
		identity:    NewIdentityResolver(opts.DB, log),
		log:         log,
		audioDir:    opts.AudioDir,
		rawStore:    rawStore,
		rawInclude:  rawInclude,
		rawExclude:  rawExclude,
		activeCalls:  newActiveCallMap(),
		affiliations: newAffiliationMap(),
		eventBus:    NewEventBus(4096), // ~60s of events at high rate
		ctx:         ctx,
		cancel:      cancel,
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
	if err := p.backfillAffiliations(ctx); err != nil {
		p.log.Warn().Err(err).Msg("affiliation backfill failed, continuing with empty map")
	}
	go p.statsLoop()
	go p.maintenanceLoop()
	p.log.Info().Msg("ingest pipeline started")
	return nil
}

// Stop flushes batchers and cancels the context.
func (p *Pipeline) Stop() {
	p.log.Info().Int64("total_messages", p.msgCount.Load()).Msg("ingest pipeline stopping")
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

	log.Info().Dur("elapsed_ms", time.Since(start)).Msg("partition maintenance complete")
}

// beginningOfMonth returns the first day of the month for the given time.
func beginningOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// HandleMessage is the entry point called by the MQTT client for each message.
func (p *Pipeline) HandleMessage(topic string, payload []byte) {
	p.msgCount.Add(1)

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

	// Dispatch to handler
	p.dispatch(route, topic, payload, &env)
}

func (p *Pipeline) incHandler(name string) {
	v, _ := p.handlerCount.LoadOrStore(name, &atomic.Int64{})
	v.(*atomic.Int64).Add(1)
}

func (p *Pipeline) dispatch(route *Route, topic string, payload []byte, env *Envelope) {
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
func (m *activeCallMap) FindByTgidAndTime(tgid int, startTime time.Time, tolerance time.Duration) (string, activeCallEntry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var bestKey string
	var bestEntry activeCallEntry
	bestDiff := tolerance + 1

	for key, entry := range m.calls {
		if entry.Tgid != tgid {
			continue
		}
		diff := entry.StartTime.Sub(startTime)
		if diff < 0 {
			diff = -diff
		}
		if diff <= tolerance && diff < bestDiff {
			bestKey = key
			bestEntry = entry
			bestDiff = diff
		}
	}

	return bestKey, bestEntry, bestDiff <= tolerance
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
	for _, r := range rows {
		key := affiliationKey{SystemID: r.SystemID, UnitID: r.UnitRID}
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
			Status:          "affiliated",
		})
		talkgroups[r.Tgid] = struct{}{}
	}

	p.log.Info().
		Int("units", len(rows)).
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

// UpdateRecorderCache stores the latest recorder state in the in-memory cache.
func (p *Pipeline) UpdateRecorderCache(instanceID string, rec database.RecorderSnapshotRow) {
	key := fmt.Sprintf("%s_%d_%d", instanceID, rec.SrcNum, rec.RecNum)
	p.recorderCache.Store(key, api.RecorderStateData{
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
	})
}
