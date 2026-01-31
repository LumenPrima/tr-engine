package ingest

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/trunk-recorder/tr-engine/internal/api/ws"
	"github.com/trunk-recorder/tr-engine/internal/database"
	"github.com/trunk-recorder/tr-engine/internal/database/models"
	"github.com/trunk-recorder/tr-engine/internal/dedup"
	"github.com/trunk-recorder/tr-engine/internal/storage"
	"go.uber.org/zap"
)

// ActiveCallInfo holds detailed info about an active call
type ActiveCallInfo struct {
	CallID       string    `json:"call_id"`
	CallNum      int64     `json:"call_num"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time,omitempty"`
	System       string    `json:"system"`
	TGID         int       `json:"tgid"`
	TGAlphaTag   string    `json:"tg_alpha_tag"`
	Freq         int64     `json:"freq"`
	Encrypted    bool      `json:"encrypted"`
	Emergency    bool      `json:"emergency"`
	Units        []UnitInfo `json:"units"`
	MissedCount  int       `json:"-"` // How many consecutive snapshots this call has been missing from
}

const maxRecentCalls = 50 // Keep last 50 calls

// missedSnapshotThreshold is how many consecutive snapshots a call must be missing
// before we consider it "ended". This gives call_end messages time to arrive since
// they may come slightly after the call disappears from calls_active snapshots.
const missedSnapshotThreshold = 3

// UnitInfo holds unit details for active calls
type UnitInfo struct {
	UnitID   int64  `json:"unit_id"`
	UnitTag  string `json:"unit_tag"`
}

// recorderStateKey uniquely identifies a recorder for state tracking
type recorderStateKey struct {
	InstanceID int
	SourceNum  int
	RecNum     int
}

// recorderState tracks last known state for snapshot dedup and dashboard display
type recorderState struct {
	State     int
	Freq      int64
	Squelched bool
}

// RecorderInfo holds full recorder state for the dashboard API
type RecorderInfo struct {
	RecorderID    int     `json:"recorder_id"`
	RecNum        int     `json:"rec_num"`
	SrcNum        int     `json:"src_num"`
	RecType       string  `json:"rec_type"`
	State         int     `json:"state"`
	StateType     string  `json:"rec_state_type"`
	Freq          int64   `json:"freq"`
	Count         int     `json:"count"`
	Duration      float32 `json:"duration"`
	Squelched     bool    `json:"squelched"`
	TGID          int     `json:"tgid,omitempty"`
	TGAlphaTag    string  `json:"tg_alpha_tag,omitempty"`
	UnitID        int64   `json:"unit_id,omitempty"`
	UnitAlphaTag  string  `json:"unit_alpha_tag,omitempty"`
}

// TranscriptionQueuer is the interface for queuing calls for transcription
type TranscriptionQueuer interface {
	QueueCall(ctx context.Context, callID int64, duration float32, priority int) error
}

// Processor handles ingestion of MQTT messages
type Processor struct {
	db           *database.DB
	storage      *storage.AudioStorage
	dedup        *dedup.Engine
	logger       *zap.Logger
	hub          *ws.Hub
	transcriber  TranscriptionQueuer
	instanceLock sync.RWMutex
	instances    map[string]int // instanceID -> db ID cache
	systemLock   sync.RWMutex
	systems      map[string]*models.System // shortName -> system cache
	sourceLock   sync.RWMutex
	sources      map[string]int // "instID:sourceNum" -> db ID cache
	activeCallsLock sync.RWMutex
	activeCalls  map[string]*ActiveCallInfo // key -> call details
	recentCalls  []*ActiveCallInfo          // recently ended calls (newest first)
	recorderStateLock sync.RWMutex
	recorderStates    map[recorderStateKey]recorderState
	recorderInfos     map[recorderStateKey]*RecorderInfo // full state for dashboard
	stopCleanup       chan struct{}
}

// NewProcessor creates a new Processor
func NewProcessor(db *database.DB, storage *storage.AudioStorage, dedup *dedup.Engine, logger *zap.Logger) *Processor {
	p := &Processor{
		db:             db,
		storage:        storage,
		dedup:          dedup,
		logger:         logger,
		instances:      make(map[string]int),
		systems:        make(map[string]*models.System),
		sources:        make(map[string]int),
		activeCalls:    make(map[string]*ActiveCallInfo),
		recorderStates: make(map[recorderStateKey]recorderState),
		recorderInfos:  make(map[recorderStateKey]*RecorderInfo),
		stopCleanup:    make(chan struct{}),
	}

	// Start periodic cleanup of stale active calls
	go p.runCleanupLoop()

	return p
}

// GetActiveCallCount returns the number of currently active calls
func (p *Processor) GetActiveCallCount() int {
	p.activeCallsLock.RLock()
	defer p.activeCallsLock.RUnlock()
	return len(p.activeCalls)
}

// GetActiveCalls returns all currently active calls with details
func (p *Processor) GetActiveCalls() []*ActiveCallInfo {
	p.activeCallsLock.RLock()
	defer p.activeCallsLock.RUnlock()

	calls := make([]*ActiveCallInfo, 0, len(p.activeCalls))
	for _, call := range p.activeCalls {
		calls = append(calls, call)
	}
	return calls
}

// GetRecentCalls returns recently ended calls (newest first)
func (p *Processor) GetRecentCalls() []*ActiveCallInfo {
	p.activeCallsLock.RLock()
	defer p.activeCallsLock.RUnlock()

	// Return a copy to avoid races
	result := make([]*ActiveCallInfo, len(p.recentCalls))
	copy(result, p.recentCalls)
	return result
}


// GetRecorders returns all known recorder states for the dashboard
func (p *Processor) GetRecorders() []*RecorderInfo {
	p.recorderStateLock.RLock()
	defer p.recorderStateLock.RUnlock()

	recorders := make([]*RecorderInfo, 0, len(p.recorderInfos))
	for _, info := range p.recorderInfos {
		// Return a copy to avoid races
		cp := *info
		recorders = append(recorders, &cp)
	}
	return recorders
}

// UnitCallInfo contains data from a unit/call message needed to create an active call entry
type UnitCallInfo struct {
	System     string
	CallNum    int64
	TGID       int
	TGAlphaTag string
	Freq       int64
	Encrypted  bool
	StartTime  time.Time
	UnitID     int64
	UnitTag    string
}

// AddUnitToActiveCall adds a unit to an active call (from unit "call" events).
// If the call doesn't exist yet (unit/call arrived before call_start), creates the entry.
func (p *Processor) AddUnitToActiveCall(info *UnitCallInfo) {
	p.activeCallsLock.Lock()
	defer p.activeCallsLock.Unlock()

	key := makeCallKey(info.System, info.CallNum)
	if call, exists := p.activeCalls[key]; exists {
		p.addUnitToCallLocked(call, info.UnitID, info.UnitTag)
		return
	}

	// Call doesn't exist yet — create it from unit/call data
	call := &ActiveCallInfo{
		CallNum:    info.CallNum,
		StartTime:  info.StartTime,
		System:     info.System,
		TGID:       info.TGID,
		TGAlphaTag: info.TGAlphaTag,
		Freq:       info.Freq,
		Encrypted:  info.Encrypted,
	}
	p.addUnitToCallLocked(call, info.UnitID, info.UnitTag)
	p.activeCalls[key] = call

	p.logger.Debug("Created active call from unit/call message",
		zap.String("system", info.System),
		zap.Int64("call_num", info.CallNum),
		zap.Int("tgid", info.TGID),
		zap.Int64("unit", info.UnitID),
	)
}

// makeCallKey creates a unique key for tracking a call
func makeCallKey(system string, callNum int64) string {
	return system + ":" + strconv.FormatInt(callNum, 10)
}

func coalesceStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func coalesceInt64(a, b int64) int64 {
	if a != 0 {
		return a
	}
	return b
}

// trackActiveCall adds a call to the active calls tracker
func (p *Processor) trackActiveCall(data *CallEventData) {
	p.activeCallsLock.Lock()
	defer p.activeCallsLock.Unlock()

	key := makeCallKey(data.ShortName, data.CallNum)

	// Check if call already exists (may have been created by an earlier unit/call message)
	if existing, exists := p.activeCalls[key]; exists {
		// Backfill fields that unit/call doesn't have
		if data.CallID != "" && existing.CallID == "" {
			existing.CallID = data.CallID
		}
		existing.Emergency = existing.Emergency || data.Emergency
		existing.TGAlphaTag = coalesceStr(data.TGAlphaTag, existing.TGAlphaTag)
		existing.Freq = coalesceInt64(data.Freq, existing.Freq)
		if data.Unit > 0 {
			p.addUnitToCall(existing, data.Unit, data.UnitAlphaTag)
		}
		return
	}

	info := &ActiveCallInfo{
		CallID:     data.CallID,
		CallNum:    data.CallNum,
		StartTime:  data.StartTime,
		System:     data.ShortName,
		TGID:       data.TGID,
		TGAlphaTag: data.TGAlphaTag,
		Freq:       data.Freq,
		Encrypted:  data.Encrypted,
		Emergency:  data.Emergency,
		Units:      []UnitInfo{},
	}

	// Add initiating unit if present
	if data.Unit > 0 {
		info.Units = append(info.Units, UnitInfo{
			UnitID:  data.Unit,
			UnitTag: data.UnitAlphaTag,
		})
	}

	p.activeCalls[key] = info
	p.logger.Debug("Tracking active call",
		zap.String("key", key),
		zap.Int64("call_num", data.CallNum),
		zap.String("tg_alpha", data.TGAlphaTag),
		zap.Int("active_count", len(p.activeCalls)),
	)
}

// addUnitToCall adds a unit to an active call if not already present
func (p *Processor) addUnitToCall(call *ActiveCallInfo, unitID int64, unitTag string) {
	for i, u := range call.Units {
		if u.UnitID == unitID {
			// Update tag if provided
			if unitTag != "" && u.UnitTag == "" {
				call.Units[i].UnitTag = unitTag
			}
			return // Already exists
		}
	}
	call.Units = append(call.Units, UnitInfo{
		UnitID:  unitID,
		UnitTag: unitTag,
	})
}

// untrackActiveCall removes a call from the active calls tracker by call_id (legacy)
func (p *Processor) untrackActiveCall(callID string) {
	p.activeCallsLock.Lock()
	defer p.activeCallsLock.Unlock()

	// Try to find by call_id (for call_end messages)
	for key, info := range p.activeCalls {
		if info.CallID == callID {
			delete(p.activeCalls, key)
			p.logger.Debug("Untracked active call by call_id",
				zap.String("call_id", callID),
				zap.Int("active_count", len(p.activeCalls)),
			)
			return
		}
	}
	p.logger.Debug("Call not found in active tracker by call_id",
		zap.String("call_id", callID),
	)
}

// untrackActiveCallEnd removes a call on call_end using system+call_num (direct lookup) with call_id fallback
func (p *Processor) untrackActiveCallEnd(system string, callNum int64, callID string) {
	p.activeCallsLock.Lock()
	defer p.activeCallsLock.Unlock()

	// Direct map lookup by system+call_num (handles calls created from unit/call that have no CallID)
	if callNum > 0 {
		key := makeCallKey(system, callNum)
		if _, exists := p.activeCalls[key]; exists {
			delete(p.activeCalls, key)
			p.logger.Debug("Untracked active call by system+call_num",
				zap.String("system", system),
				zap.Int64("call_num", callNum),
				zap.Int("active_count", len(p.activeCalls)),
			)
			return
		}
	}

	// Fallback: scan by call_id (for calls tracked with a different key)
	if callID != "" {
		for key, info := range p.activeCalls {
			if info.CallID == callID {
				delete(p.activeCalls, key)
				p.logger.Debug("Untracked active call by call_id fallback",
					zap.String("call_id", callID),
					zap.Int("active_count", len(p.activeCalls)),
				)
				return
			}
		}
	}

	p.logger.Debug("Call not found in active tracker on call_end",
		zap.String("system", system),
		zap.Int64("call_num", callNum),
		zap.String("call_id", callID),
	)
}

// UntrackActiveCallByNum removes a call from the active calls tracker by system and call_num
func (p *Processor) UntrackActiveCallByNum(system string, callNum int64, unitID int64, unitTag string) {
	p.activeCallsLock.Lock()
	defer p.activeCallsLock.Unlock()

	key := makeCallKey(system, callNum)
	if info, exists := p.activeCalls[key]; exists {
		// Add the unit that ended the call if not already present
		p.addUnitToCallLocked(info, unitID, unitTag)

		delete(p.activeCalls, key)
		p.logger.Debug("Untracked active call by call_num",
			zap.String("system", system),
			zap.Int64("call_num", callNum),
			zap.Int("active_count", len(p.activeCalls)),
		)
	}
}

// addUnitToCallLocked adds a unit to a call (must hold lock)
func (p *Processor) addUnitToCallLocked(call *ActiveCallInfo, unitID int64, unitTag string) {
	for i, u := range call.Units {
		if u.UnitID == unitID {
			if unitTag != "" && u.UnitTag == "" {
				call.Units[i].UnitTag = unitTag
			}
			return
		}
	}
	call.Units = append(call.Units, UnitInfo{
		UnitID:  unitID,
		UnitTag: unitTag,
	})
}

// Stop stops background goroutines (cleanup loop)
func (p *Processor) Stop() {
	close(p.stopCleanup)
}

// runCleanupLoop periodically cleans up stale active calls
func (p *Processor) runCleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCleanup:
			return
		case <-ticker.C:
			p.cleanupStaleCalls()
		}
	}
}

// cleanupStaleCalls removes calls that have been active for too long (likely missed call_end)
func (p *Processor) cleanupStaleCalls() {
	p.activeCallsLock.Lock()
	defer p.activeCallsLock.Unlock()

	maxAge := 10 * time.Minute
	now := time.Now()
	staleCount := 0

	for callID, info := range p.activeCalls {
		if now.Sub(info.StartTime) > maxAge {
			delete(p.activeCalls, callID)
			staleCount++
		}
	}

	if staleCount > 0 {
		p.logger.Debug("Cleaned up stale active calls",
			zap.Int("removed", staleCount),
			zap.Int("remaining", len(p.activeCalls)),
		)
	}
}

// SetHub sets the WebSocket hub for broadcasting events
func (p *Processor) SetHub(hub *ws.Hub) {
	p.hub = hub
}

// SetTranscriptionService sets the transcription service for automatic queuing
func (p *Processor) SetTranscriptionService(svc TranscriptionQueuer) {
	p.transcriber = svc
}

// getOrCreateInstance gets the database ID for an instance, creating if needed
func (p *Processor) getOrCreateInstance(ctx context.Context, instanceID string) (int, error) {
	// Check cache first
	p.instanceLock.RLock()
	if id, ok := p.instances[instanceID]; ok {
		p.instanceLock.RUnlock()
		return id, nil
	}
	p.instanceLock.RUnlock()

	// Create/update instance
	inst, err := p.db.UpsertInstance(ctx, instanceID, "", nil)
	if err != nil {
		return 0, err
	}

	// Update cache
	p.instanceLock.Lock()
	p.instances[instanceID] = inst.ID
	p.instanceLock.Unlock()

	return inst.ID, nil
}

// getSystem gets the cached system record by short name
func (p *Processor) getSystem(ctx context.Context, shortName string) (*models.System, error) {
	// Check cache first
	p.systemLock.RLock()
	if sys, ok := p.systems[shortName]; ok {
		p.systemLock.RUnlock()
		return sys, nil
	}
	p.systemLock.RUnlock()

	// Query database
	sys, err := p.db.GetSystemByShortName(ctx, shortName)
	if err != nil {
		return nil, err
	}
	if sys == nil {
		return nil, nil // System not found
	}

	// Update cache
	p.systemLock.Lock()
	p.systems[shortName] = sys
	p.systemLock.Unlock()

	return sys, nil
}

// getSystemID gets the database ID for a system (convenience wrapper)
func (p *Processor) getSystemID(ctx context.Context, shortName string) (int, error) {
	sys, err := p.getSystem(ctx, shortName)
	if err != nil {
		return 0, err
	}
	if sys == nil {
		return 0, nil
	}
	return sys.ID, nil
}

// makeSourceKey creates a cache key for source lookups
func makeSourceKey(instanceID, sourceNum int) string {
	return strconv.Itoa(instanceID) + ":" + strconv.Itoa(sourceNum)
}

// getSourceID gets the database ID for a source by instance DB ID and source number
func (p *Processor) getSourceID(instanceID, sourceNum int) int {
	key := makeSourceKey(instanceID, sourceNum)
	p.sourceLock.RLock()
	if id, ok := p.sources[key]; ok {
		p.sourceLock.RUnlock()
		return id
	}
	p.sourceLock.RUnlock()
	return 0 // Not cached — source registration happens via config message
}

// broadcast sends an event to WebSocket clients
func (p *Processor) broadcast(eventType string, data interface{}) {
	if p.hub == nil {
		return
	}

	event := ws.Event{
		Type:      eventType,
		Timestamp: time.Now().Unix(),
		Data:      data,
	}

	p.hub.Broadcast(event)
}

// ConfigData holds configuration message data
type ConfigData struct {
	InstanceID  string
	InstanceKey string
	Sources     []SourceData
	Systems     []SystemData
	ConfigJSON  json.RawMessage
}

// SourceData holds source configuration
type SourceData struct {
	SourceNum  int
	CenterFreq int64
	Rate       int
	Driver     string
	Device     string
	Antenna    string
	Gain       int
}

// SystemData holds system configuration
type SystemData struct {
	SysNum     int
	ShortName  string
	SystemType string
	SysID      string
	WACN       string
	NAC        string
	RFSS       int
	SiteID     int
}

// SystemStatusData holds system status data
type SystemStatusData struct {
	InstanceID string
	SysNum     int
	ShortName  string
	SystemType string
	SysID      string
	WACN       string
	NAC        string
	RFSS       int
	SiteID     int
	Timestamp  time.Time
}

// RateData holds decode rate data
type RateData struct {
	InstanceID     string
	SysNum         int
	ShortName      string
	DecodeRate     float32
	ControlChannel int64
	Timestamp      time.Time
}

// RecorderData holds recorder status data
type RecorderData struct {
	InstanceID string
	RecNum     int
	RecType    string
	SourceNum  int
	State      int
	StateType  string
	Freq       int64
	Count      int
	Duration   float32
	Squelched  bool
	Timestamp  time.Time
	IsSnapshot bool // true when from bulk recorders message (skip if unchanged)
}

// CallEventData holds call event data
type CallEventData struct {
	InstanceID    string
	CallID        string
	CallNum       int64
	Freq          int64
	FreqError     int
	SysNum        int
	ShortName     string
	TGID          int
	TGAlphaTag    string
	TGTag         string
	TGGroup       string
	TGDesc        string
	StartTime     time.Time
	StopTime      time.Time
	Duration      float32
	Encrypted     bool
	Emergency     bool
	Phase2TDMA    bool
	TDMASlot      int
	Conventional  bool
	Analog        bool
	AudioType     string
	ErrorCount    int
	SpikeCount    int
	Unit          int64
	UnitAlphaTag  string
	RecNum        int
	SrcNum        int
	RecState      int
	RecStateType  string
	MonState      int
	MonStateType  string
	CallState     int
	CallStateType string
	SignalDB      float32
	NoiseDB       float32
	Timestamp     time.Time
	CallFilename  string // Path to audio file from trunk-recorder
	RawJSON       []byte
}

// SourceUnitData holds unit transmission data
type SourceUnitData struct {
	Src        int64
	Time       time.Time
	Pos        float32
	Emergency  bool
	SignalDB   float32
	NoiseDB    float32
	ErrorCount int
	SpikeCount int
	Tag        string
}

// FreqEntryData holds frequency entry data
type FreqEntryData struct {
	Freq       int64
	Time       time.Time
	Pos        float32
	Len        float32
	ErrorCount int
	SpikeCount int
}

// AudioData holds audio message data
type AudioData struct {
	InstanceID string
	CallID     string
	CallNum    int64
	ShortName  string
	TGID       int
	TGAlphaTag string
	TGDesc     string
	TGGroup    string
	TGTag      string
	StartTime  time.Time
	StopTime   time.Time
	Freq       float64
	FreqError  int
	SignalDB   float32
	NoiseDB    float32
	Encrypted  bool
	Emergency  bool
	Phase2TDMA bool
	TDMASlot   int
	AudioType  string
	AudioData  string // Base64 encoded
	Filename   string
	SrcList    []SourceUnitData
	FreqList   []FreqEntryData
}

// UnitEventData holds unit event data
type UnitEventData struct {
	InstanceID string
	ShortName  string
	SysNum     int
	EventType  string
	UnitID     int64
	UnitTag    string
	TGID       int
	TGAlphaTag string
	TGDesc     string
	TGGroup    string
	TGTag      string
	Timestamp  time.Time
	RawJSON    []byte
}

// TrunkMessageData holds trunk message data
type TrunkMessageData struct {
	InstanceID  string
	ShortName   string
	SysNum      int
	MsgType     int
	MsgTypeName string
	Opcode      string
	OpcodeType  string
	OpcodeDesc  string
	Meta        string
	Timestamp   time.Time
}

// GetDB returns the database connection
func (p *Processor) GetDB() *database.DB {
	return p.db
}

// ProcessConfig handles a config message
func (p *Processor) ProcessConfig(ctx context.Context, data *ConfigData) error {
	// Upsert instance
	inst, err := p.db.UpsertInstance(ctx, data.InstanceID, data.InstanceKey, data.ConfigJSON)
	if err != nil {
		return err
	}

	// Update cache
	p.instanceLock.Lock()
	p.instances[data.InstanceID] = inst.ID
	p.instanceLock.Unlock()

	// Upsert sources
	for _, src := range data.Sources {
		srcJSON, _ := json.Marshal(src)
		s, err := p.db.UpsertSource(ctx, inst.ID, src.SourceNum, src.CenterFreq, src.Rate, src.Driver, src.Device, src.Antenna, src.Gain, srcJSON)
		if err != nil {
			p.logger.Error("Failed to upsert source",
				zap.Error(err),
				zap.Int("source_num", src.SourceNum),
			)
		} else {
			// Cache source ID
			p.sourceLock.Lock()
			p.sources[makeSourceKey(inst.ID, src.SourceNum)] = s.ID
			p.sourceLock.Unlock()
		}
	}

	// Upsert systems
	for _, sys := range data.Systems {
		sysJSON, _ := json.Marshal(sys)
		s, err := p.db.UpsertSystem(ctx, inst.ID, sys.SysNum, sys.ShortName, sys.SystemType, sys.SysID, sys.WACN, sys.NAC, sys.RFSS, sys.SiteID, sysJSON)
		if err != nil {
			p.logger.Error("Failed to upsert system",
				zap.Error(err),
				zap.String("short_name", sys.ShortName),
			)
		} else {
			// Update cache
			p.systemLock.Lock()
			p.systems[sys.ShortName] = s
			p.systemLock.Unlock()
		}
	}

	p.logger.Info("Processed config",
		zap.String("instance", data.InstanceID),
		zap.Int("sources", len(data.Sources)),
		zap.Int("systems", len(data.Systems)),
	)

	return nil
}

// ProcessSystemStatus handles a system status update
func (p *Processor) ProcessSystemStatus(ctx context.Context, data *SystemStatusData) error {
	instID, err := p.getOrCreateInstance(ctx, data.InstanceID)
	if err != nil {
		return err
	}

	// Upsert system
	sys, err := p.db.UpsertSystem(ctx, instID, data.SysNum, data.ShortName, data.SystemType, data.SysID, data.WACN, data.NAC, data.RFSS, data.SiteID, nil)
	if err != nil {
		return err
	}

	// Update cache
	p.systemLock.Lock()
	p.systems[data.ShortName] = sys
	p.systemLock.Unlock()

	// Broadcast system update
	p.broadcast("system_update", map[string]interface{}{
		"system":      data.ShortName,
		"system_type": data.SystemType,
		"sysid":       data.SysID,
	})

	return nil
}

// ProcessRate handles a decode rate message
func (p *Processor) ProcessRate(ctx context.Context, data *RateData) error {
	sys, err := p.getSystem(ctx, data.ShortName)
	if err != nil {
		return err
	}
	if sys == nil {
		// System not yet registered, skip
		return nil
	}
	sysid := database.EffectiveSYSID(sys)

	rate := &models.SystemRate{
		SystemID:       sys.ID,
		Time:           data.Timestamp,
		DecodeRate:     data.DecodeRate,
		ControlChannel: data.ControlChannel,
	}
	if err := p.db.InsertSystemRate(ctx, rate); err != nil {
		return err
	}

	// Broadcast update
	// max_rate is 40 for P25 Phase 1 systems (voice slots per second)
	p.broadcast("rate_update", map[string]interface{}{
		"system":          data.ShortName,
		"sysid":           sysid,
		"decode_rate":     data.DecodeRate,
		"max_rate":        40,
		"control_channel": data.ControlChannel,
	})

	return nil
}

// ProcessRecorderStatus handles a recorder status message
func (p *Processor) ProcessRecorderStatus(ctx context.Context, data *RecorderData) error {
	instID, err := p.getOrCreateInstance(ctx, data.InstanceID)
	if err != nil {
		return err
	}

	// Resolve source_id from cache (populated by ProcessConfig)
	var sourceID *int
	if srcID := p.getSourceID(instID, data.SourceNum); srcID > 0 {
		sourceID = &srcID
	}

	// Track state for dedup: snapshots skip if unchanged, individual events always proceed
	stateKey := recorderStateKey{
		InstanceID: instID,
		SourceNum:  data.SourceNum,
		RecNum:     data.RecNum,
	}
	newState := recorderState{
		State:     data.State,
		Freq:      data.Freq,
		Squelched: data.Squelched,
	}

	p.recorderStateLock.RLock()
	prev, exists := p.recorderStates[stateKey]
	p.recorderStateLock.RUnlock()

	skipDB := data.IsSnapshot && exists && prev == newState

	// Always update cached state and full info (count/duration change even when state doesn't)
	p.recorderStateLock.Lock()
	p.recorderStates[stateKey] = newState
	info := p.recorderInfos[stateKey]
	if info == nil {
		info = &RecorderInfo{}
		p.recorderInfos[stateKey] = info
	}
	info.RecNum = data.RecNum
	info.SrcNum = data.SourceNum
	info.RecType = data.RecType
	info.State = data.State
	info.StateType = data.StateType
	info.Freq = data.Freq
	info.Count = data.Count
	info.Duration = data.Duration
	info.Squelched = data.Squelched
	// Clear call context — will be re-enriched below if recording
	info.TGID = 0
	info.TGAlphaTag = ""
	info.UnitID = 0
	info.UnitAlphaTag = ""
	p.recorderStateLock.Unlock()

	// Enrich with active call info if recorder is on a frequency
	if data.Freq > 0 {
		p.enrichRecorderFromActiveCall(info, data.Freq)
	}

	if skipDB {
		return nil // Snapshot with no state change, skip DB insert and broadcast
	}

	// Upsert recorder with resolved source_id
	rec, err := p.db.UpsertRecorder(ctx, instID, sourceID, data.RecNum, data.RecType)
	if err != nil {
		return err
	}

	// Update recorder_id in info now that we have it
	p.recorderStateLock.Lock()
	info.RecorderID = rec.ID
	p.recorderStateLock.Unlock()

	// Insert status snapshot
	status := &models.RecorderStatus{
		RecorderID: rec.ID,
		Time:       data.Timestamp,
		State:      int16(data.State),
		Freq:       data.Freq,
		CallCount:  data.Count,
		Duration:   data.Duration,
		Squelched:  data.Squelched,
	}
	if err := p.db.InsertRecorderStatus(ctx, status); err != nil {
		return err
	}

	// Broadcast update with full context (read enriched info)
	p.recorderStateLock.RLock()
	broadcastData := map[string]interface{}{
		"recorder_id":    rec.ID,
		"rec_num":        data.RecNum,
		"src_num":        data.SourceNum,
		"rec_type":       data.RecType,
		"state":          data.State,
		"rec_state_type": data.StateType,
		"freq":           data.Freq,
		"count":          data.Count,
		"duration":       data.Duration,
		"squelched":      data.Squelched,
	}
	if info.TGID > 0 {
		broadcastData["tgid"] = info.TGID
		broadcastData["tg_alpha_tag"] = info.TGAlphaTag
	}
	if info.UnitID > 0 {
		broadcastData["unit_id"] = info.UnitID
		broadcastData["unit_alpha_tag"] = info.UnitAlphaTag
	}
	p.recorderStateLock.RUnlock()
	p.broadcast("recorder_update", broadcastData)

	return nil
}

// enrichRecorderFromActiveCall looks up an active call by frequency and populates
// the recorder info with talkgroup/unit context
func (p *Processor) enrichRecorderFromActiveCall(info *RecorderInfo, freq int64) {
	p.activeCallsLock.RLock()
	defer p.activeCallsLock.RUnlock()

	for _, call := range p.activeCalls {
		if call.Freq == freq {
			p.recorderStateLock.Lock()
			info.TGID = call.TGID
			info.TGAlphaTag = call.TGAlphaTag
			if len(call.Units) > 0 {
				// Use the most recent unit (last in list)
				last := call.Units[len(call.Units)-1]
				info.UnitID = last.UnitID
				info.UnitAlphaTag = last.UnitTag
			}
			p.recorderStateLock.Unlock()
			return
		}
	}
}
