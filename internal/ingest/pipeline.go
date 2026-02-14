package ingest

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
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

	// Active call tracking: tr_call_id → db call_id
	activeCalls *activeCallMap

	ctx    context.Context
	cancel context.CancelFunc

	msgCount atomic.Int64
}

type PipelineOptions struct {
	DB       *database.DB
	AudioDir string
	Log      zerolog.Logger
}

func NewPipeline(opts PipelineOptions) *Pipeline {
	ctx, cancel := context.WithCancel(context.Background())
	log := opts.Log.With().Str("component", "ingest").Logger()

	p := &Pipeline{
		db:          opts.DB,
		identity:    NewIdentityResolver(opts.DB, log),
		log:         log,
		audioDir:    opts.AudioDir,
		activeCalls: newActiveCallMap(),
		ctx:         ctx,
		cancel:      cancel,
	}

	p.rawBatcher = NewBatcher[database.RawMessageRow](100, 2*time.Second, p.flushRawMessages)
	p.recorderBatcher = NewBatcher[database.RecorderSnapshotRow](100, 2*time.Second, p.flushRecorderSnapshots)

	return p
}

// Start loads the identity cache.
func (p *Pipeline) Start(ctx context.Context) error {
	if err := p.identity.LoadCache(ctx); err != nil {
		return err
	}
	p.log.Info().Msg("ingest pipeline started")
	return nil
}

// Stop flushes batchers and cancels the context.
func (p *Pipeline) Stop() {
	p.log.Info().Int64("total_messages", p.msgCount.Load()).Msg("ingest pipeline stopping")
	p.rawBatcher.Stop()
	p.recorderBatcher.Stop()
	p.cancel()
}

// HandleMessage is the entry point called by the MQTT client for each message.
func (p *Pipeline) HandleMessage(topic string, payload []byte) {
	p.msgCount.Add(1)

	route := ParseTopic(topic)
	if route == nil {
		p.log.Warn().Str("topic", topic).Msg("unknown topic, skipping")
		return
	}

	// Archive raw message (best-effort extract instance_id)
	var env Envelope
	_ = json.Unmarshal(payload, &env)

	p.rawBatcher.Add(database.RawMessageRow{
		Topic:      topic,
		Payload:    payload,
		ReceivedAt: time.Now(),
		InstanceID: env.InstanceID,
	})

	// Dispatch to handler
	p.dispatch(route, topic, payload, &env)
}

func (p *Pipeline) dispatch(route *Route, topic string, payload []byte, env *Envelope) {
	var err error

	switch route.Handler {
	case "status":
		err = p.handleStatus(payload)
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

// activeCallMap tracks in-flight calls: tr_call_id → (db call_id, start_time).
type activeCallEntry struct {
	CallID    int64
	StartTime time.Time
}

type activeCallMap struct {
	mu    sync.Mutex
	calls map[string]activeCallEntry
}

func newActiveCallMap() *activeCallMap {
	return &activeCallMap{calls: make(map[string]activeCallEntry)}
}

func (m *activeCallMap) Set(trCallID string, callID int64, startTime time.Time) {
	m.mu.Lock()
	m.calls[trCallID] = activeCallEntry{CallID: callID, StartTime: startTime}
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

func (m *activeCallMap) Len() int {
	m.mu.Lock()
	n := len(m.calls)
	m.mu.Unlock()
	return n
}
