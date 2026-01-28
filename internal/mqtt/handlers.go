package mqtt

import (
	"context"
	"encoding/json"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/trunk-recorder/tr-engine/internal/ingest"
	"github.com/trunk-recorder/tr-engine/internal/metrics"
	"go.uber.org/zap"
)

// Handlers processes MQTT messages
type Handlers struct {
	processor *ingest.Processor
	logger    *zap.Logger
}

// NewHandlers creates a new Handlers instance
func NewHandlers(processor *ingest.Processor, logger *zap.Logger) *Handlers {
	return &Handlers{
		processor: processor,
		logger:    logger,
	}
}

// HandleStatusMessage handles messages from the status topic
func (h *Handlers) HandleStatusMessage(client mqtt.Client, msg mqtt.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Log raw message for debugging
	payload := msg.Payload()
	preview := string(payload)
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	h.logger.Debug("Raw MQTT message",
		zap.String("topic", msg.Topic()),
		zap.String("preview", preview),
	)

	// Parse the base message to determine type
	var base StatusMessage
	if err := json.Unmarshal(payload, &base); err != nil {
		h.logger.Error("Failed to parse status message",
			zap.Error(err),
			zap.String("topic", msg.Topic()),
		)
		metrics.MQTTParseErrors.WithLabelValues("status").Inc()
		return
	}
	base.RawJSON = payload

	h.logger.Debug("Received status message",
		zap.String("type", base.Type),
		zap.String("instance", base.InstanceID),
		zap.String("topic", msg.Topic()),
	)

	// Track message processing duration
	timer := prometheus.NewTimer(metrics.MessageProcessingDuration.WithLabelValues(base.Type))
	defer timer.ObserveDuration()

	// Track message count (use "unknown" for system when not available yet)
	shortName := "unknown"

	switch base.Type {
	case "config":
		h.handleConfig(ctx, msg.Payload())
	case "systems":
		h.handleSystems(ctx, msg.Payload())
	case "system":
		h.handleSystem(ctx, msg.Payload())
	case "rates":
		h.handleRates(ctx, msg.Payload())
	case "recorders":
		h.handleRecorders(ctx, msg.Payload())
	case "recorder":
		h.handleRecorder(ctx, msg.Payload())
	case "calls_active":
		h.handleCallsActive(ctx, msg.Payload())
	case "call_start":
		h.handleCallStart(ctx, msg.Payload())
	case "call_end":
		h.handleCallEnd(ctx, msg.Payload())
	case "audio":
		h.handleAudio(ctx, msg.Payload())
	default:
		h.logger.Debug("Unknown status message type", zap.String("type", base.Type))
	}

	metrics.MQTTMessagesReceived.WithLabelValues(base.Type, shortName).Inc()
}

// HandleUnitMessage handles messages from the unit topic
func (h *Handlers) HandleUnitMessage(client mqtt.Client, msg mqtt.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	payload := msg.Payload()

	// First parse just the type and base fields
	var base struct {
		Type       string `json:"type"`
		InstanceID string `json:"instance_id"`
		Timestamp  int64  `json:"timestamp"`
	}
	if err := json.Unmarshal(payload, &base); err != nil {
		h.logger.Error("Failed to parse unit message",
			zap.Error(err),
			zap.String("topic", msg.Topic()),
		)
		metrics.MQTTParseErrors.WithLabelValues("unit").Inc()
		return
	}

	// Track message processing duration
	timer := prometheus.NewTimer(metrics.MessageProcessingDuration.WithLabelValues("unit_" + base.Type))
	defer timer.ObserveDuration()

	eventType := base.Type

	// Extract unit data based on message type
	// "call" type has data under "call" key, "end" has data under "end" key
	var unitID int64
	var unitTag string
	var shortName string
	var sysNum int
	var tgid int
	var tgAlphaTag, tgDesc, tgGroup, tgTag string

	switch eventType {
	case "call":
		var callMsg UnitCallMessage
		if err := json.Unmarshal(payload, &callMsg); err == nil {
			unitID = callMsg.Call.Unit
			unitTag = callMsg.Call.UnitAlphaTag
			shortName = callMsg.Call.ShortName
			sysNum = callMsg.Call.SysNum
			tgid = callMsg.Call.TGID
			tgAlphaTag = callMsg.Call.TGAlphaTag
			tgDesc = callMsg.Call.TGDesc
			tgGroup = callMsg.Call.TGGroup
			tgTag = callMsg.Call.TGTag
			// Add unit to active call's unit list (creates call entry if it doesn't exist yet)
			if callMsg.Call.CallNum > 0 {
				h.processor.AddUnitToActiveCall(&ingest.UnitCallInfo{
					System:     shortName,
					CallNum:    callMsg.Call.CallNum,
					TGID:       callMsg.Call.TGID,
					TGAlphaTag: callMsg.Call.TGAlphaTag,
					Freq:       int64(callMsg.Call.Freq),
					Encrypted:  callMsg.Call.Encrypted,
					StartTime:  ParseTimestamp(callMsg.Call.StartTime),
					UnitID:     unitID,
					UnitTag:    unitTag,
				})
			}
		}
	case "end":
		var endMsg UnitEndMessage
		if err := json.Unmarshal(payload, &endMsg); err == nil {
			unitID = endMsg.End.Unit
			unitTag = endMsg.End.UnitAlphaTag
			shortName = endMsg.End.ShortName
			sysNum = endMsg.End.SysNum
			tgid = endMsg.End.TGID
			tgAlphaTag = endMsg.End.TGAlphaTag
			tgDesc = endMsg.End.TGDesc
			tgGroup = endMsg.End.TGGroup
			tgTag = endMsg.End.TGTag
		}
	default:
		// For other types (on, off, join, location, ackresp, data, ans_req), try generic parsing
		var generic UnitMessage
		if err := json.Unmarshal(payload, &generic); err == nil {
			unitID = generic.Unit
			unitTag = generic.UnitTag
			shortName = generic.ShortName
			sysNum = generic.SysNum
			tgid = generic.TGID
			tgAlphaTag = generic.TGAlphaTag
			tgDesc = generic.TGDesc
			tgGroup = generic.TGGroup
			tgTag = generic.TGTag
		}
	}

	// Skip invalid unit IDs
	if unitID <= 0 {
		return
	}

	h.logger.Debug("Received unit message",
		zap.String("type", eventType),
		zap.String("instance", base.InstanceID),
		zap.Int64("unit", unitID),
		zap.String("unit_tag", unitTag),
		zap.Int("tgid", tgid),
	)

	if err := h.processor.ProcessUnitEvent(ctx, &ingest.UnitEventData{
		InstanceID: base.InstanceID,
		ShortName:  shortName,
		SysNum:     sysNum,
		EventType:  eventType,
		UnitID:     unitID,
		UnitTag:    unitTag,
		TGID:       tgid,
		TGAlphaTag: tgAlphaTag,
		TGDesc:     tgDesc,
		TGGroup:    tgGroup,
		TGTag:      tgTag,
		Timestamp:  ParseTimestamp(base.Timestamp),
		RawJSON:    payload,
	}); err != nil {
		h.logger.Error("Failed to process unit event",
			zap.Error(err),
			zap.String("type", eventType),
			zap.Int64("unit", unitID),
		)
	}

	metrics.MQTTMessagesReceived.WithLabelValues("unit_"+eventType, shortName).Inc()
}

// HandleTrunkMessage handles trunking messages
func (h *Handlers) HandleTrunkMessage(client mqtt.Client, msg mqtt.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Track message processing duration
	timer := prometheus.NewTimer(metrics.MessageProcessingDuration.WithLabelValues("trunk"))
	defer timer.ObserveDuration()

	var trunkMsg TrunkingMessage
	if err := json.Unmarshal(msg.Payload(), &trunkMsg); err != nil {
		h.logger.Error("Failed to parse trunk message",
			zap.Error(err),
			zap.String("topic", msg.Topic()),
		)
		metrics.MQTTParseErrors.WithLabelValues("trunk").Inc()
		return
	}

	if err := h.processor.ProcessTrunkMessage(ctx, &ingest.TrunkMessageData{
		InstanceID:  trunkMsg.InstanceID,
		ShortName:   trunkMsg.ShortName,
		SysNum:      trunkMsg.SysNum,
		MsgType:     trunkMsg.MsgType,
		MsgTypeName: trunkMsg.MsgTypeName,
		Opcode:      trunkMsg.Opcode,
		OpcodeType:  trunkMsg.OpcodeType,
		OpcodeDesc:  trunkMsg.OpcodeDesc,
		Meta:        trunkMsg.Meta,
		Timestamp:   ParseTimestamp(trunkMsg.Timestamp),
	}); err != nil {
		h.logger.Error("Failed to process trunk message", zap.Error(err))
	}

	metrics.MQTTMessagesReceived.WithLabelValues("trunk", trunkMsg.ShortName).Inc()
}

func (h *Handlers) handleConfig(ctx context.Context, payload []byte) {
	var cfg ConfigMessage
	if err := json.Unmarshal(payload, &cfg); err != nil {
		h.logger.Error("Failed to parse config message", zap.Error(err))
		return
	}

	// Convert to ingest types
	sources := make([]ingest.SourceData, len(cfg.Config.Sources))
	for i, s := range cfg.Config.Sources {
		sources[i] = ingest.SourceData{
			SourceNum:  s.SourceNum,
			CenterFreq: int64(s.CenterFreq),
			Rate:       int(s.Rate),
			Driver:     s.Driver,
			Device:     s.Device,
			Antenna:    s.Antenna,
			Gain:       int(s.Gain),
		}
	}

	systems := make([]ingest.SystemData, len(cfg.Config.Systems))
	for i, s := range cfg.Config.Systems {
		systems[i] = ingest.SystemData{
			SysNum:     s.SysNum,
			ShortName:  s.ShortName,
			SystemType: s.SystemType,
		}
	}

	if err := h.processor.ProcessConfig(ctx, &ingest.ConfigData{
		InstanceID:  cfg.InstanceID,
		InstanceKey: cfg.Config.InstanceKey,
		Sources:     sources,
		Systems:     systems,
		ConfigJSON:  payload, // Store the raw payload as config JSON
	}); err != nil {
		h.logger.Error("Failed to process config", zap.Error(err))
	}
}

func (h *Handlers) handleSystems(ctx context.Context, payload []byte) {
	var msg SystemsMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		h.logger.Error("Failed to parse systems message", zap.Error(err))
		return
	}

	for _, sys := range msg.Systems {
		if err := h.processor.ProcessSystemStatus(ctx, &ingest.SystemStatusData{
			InstanceID: msg.InstanceID,
			SysNum:     sys.SysNum,
			ShortName:  sys.ShortName,
			SystemType: sys.Type,
			SysID:      sys.SysID,
			WACN:       sys.WACN,
			NAC:        sys.NAC,
			RFSS:       sys.RFSS,
			SiteID:     sys.SiteID,
			Timestamp:  ParseTimestamp(msg.Timestamp),
		}); err != nil {
			h.logger.Error("Failed to process system status",
				zap.Error(err),
				zap.String("short_name", sys.ShortName),
			)
		}
	}
}

func (h *Handlers) handleSystem(ctx context.Context, payload []byte) {
	// Single system status, similar to systems but just one
	var msg struct {
		Type       string       `json:"type"`
		InstanceID string       `json:"instance_id"`
		Timestamp  int64        `json:"timestamp"`
		System     SystemStatus `json:"system"`
	}
	if err := json.Unmarshal(payload, &msg); err != nil {
		h.logger.Error("Failed to parse system message", zap.Error(err))
		return
	}

	if err := h.processor.ProcessSystemStatus(ctx, &ingest.SystemStatusData{
		InstanceID: msg.InstanceID,
		SysNum:     msg.System.SysNum,
		ShortName:  msg.System.ShortName,
		SystemType: msg.System.Type,
		SysID:      msg.System.SysID,
		WACN:       msg.System.WACN,
		NAC:        msg.System.NAC,
		RFSS:       msg.System.RFSS,
		SiteID:     msg.System.SiteID,
		Timestamp:  ParseTimestamp(msg.Timestamp),
	}); err != nil {
		h.logger.Error("Failed to process system status", zap.Error(err))
	}
}

func (h *Handlers) handleRates(ctx context.Context, payload []byte) {
	var msg RatesMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		h.logger.Error("Failed to parse rates message", zap.Error(err))
		return
	}

	for _, rate := range msg.Rates {
		if err := h.processor.ProcessRate(ctx, &ingest.RateData{
			InstanceID:     msg.InstanceID,
			SysNum:         rate.SysNum,
			ShortName:      rate.ShortName,
			DecodeRate:     rate.DecodeRate,
			ControlChannel: int64(rate.ControlChannel),
			Timestamp:      ParseTimestamp(msg.Timestamp),
		}); err != nil {
			h.logger.Error("Failed to process rate",
				zap.Error(err),
				zap.String("short_name", rate.ShortName),
			)
		}
	}
}

func (h *Handlers) handleRecorders(ctx context.Context, payload []byte) {
	var msg RecordersMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		h.logger.Error("Failed to parse recorders message", zap.Error(err))
		return
	}

	for _, rec := range msg.Recorders {
		if err := h.processor.ProcessRecorderStatus(ctx, &ingest.RecorderData{
			InstanceID: msg.InstanceID,
			RecNum:     rec.RecNum,
			RecType:    rec.Type,
			SourceNum:  rec.SourceNum,
			State:      rec.State,
			StateType:  rec.StateType,
			Freq:       int64(rec.Freq),
			Count:      rec.Count,
			Duration:   rec.Duration,
			Squelched:  rec.Squelched,
			Timestamp:  ParseTimestamp(msg.Timestamp),
			IsSnapshot: true,
		}); err != nil {
			h.logger.Error("Failed to process recorder status",
				zap.Error(err),
				zap.String("rec_id", rec.ID),
			)
		}
	}
}

func (h *Handlers) handleRecorder(ctx context.Context, payload []byte) {
	var msg struct {
		Type       string         `json:"type"`
		InstanceID string         `json:"instance_id"`
		Timestamp  int64          `json:"timestamp"`
		Recorder   RecorderStatus `json:"recorder"`
	}
	if err := json.Unmarshal(payload, &msg); err != nil {
		h.logger.Error("Failed to parse recorder message", zap.Error(err))
		return
	}

	if err := h.processor.ProcessRecorderStatus(ctx, &ingest.RecorderData{
		InstanceID: msg.InstanceID,
		RecNum:     msg.Recorder.RecNum,
		RecType:    msg.Recorder.Type,
		SourceNum:  msg.Recorder.SourceNum,
		State:      msg.Recorder.State,
		StateType:  msg.Recorder.StateType,
		Freq:       int64(msg.Recorder.Freq),
		Count:      msg.Recorder.Count,
		Duration:   msg.Recorder.Duration,
		Squelched:  msg.Recorder.Squelched,
		Timestamp:  ParseTimestamp(msg.Timestamp),
		IsSnapshot: false,
	}); err != nil {
		h.logger.Error("Failed to process recorder status", zap.Error(err))
	}
}

func (h *Handlers) handleCallsActive(ctx context.Context, payload []byte) {
	var msg struct {
		Type       string     `json:"type"`
		InstanceID string     `json:"instance_id"`
		Timestamp  int64      `json:"timestamp"`
		Calls      []CallData `json:"calls"`
	}
	if err := json.Unmarshal(payload, &msg); err != nil {
		h.logger.Error("Failed to parse calls_active message", zap.Error(err))
		return
	}

	// Build parallel slices of ActiveCallInfo and CallEventData
	activeCalls := make([]*ingest.ActiveCallInfo, 0, len(msg.Calls))
	callDataSlice := make([]*ingest.CallEventData, 0, len(msg.Calls))
	for i := range msg.Calls {
		call := &msg.Calls[i]
		activeCalls = append(activeCalls, &ingest.ActiveCallInfo{
			CallID:     call.ID,
			CallNum:    call.CallNum,
			StartTime:  ParseTimestamp(call.StartTime),
			System:     call.ShortName,
			TGID:       call.TGID,
			TGAlphaTag: call.TGAlphaTag,
			Freq:       int64(call.Freq),
			Encrypted:  call.Encrypted,
			Emergency:  call.Emergency,
			Units: []ingest.UnitInfo{{
				UnitID:  call.Unit,
				UnitTag: call.UnitAlphaTag,
			}},
		})
		callDataSlice = append(callDataSlice, h.convertCallData(msg.InstanceID, msg.Timestamp, call))
	}

	// Diff against current in-memory state
	diff := h.processor.DiffActiveCalls(activeCalls, callDataSlice)

	// Handle new calls we missed call_start for
	for _, data := range diff.NewCalls {
		if err := h.processor.ProcessNewCallFromSnapshot(ctx, data); err != nil {
			h.logger.Error("Failed to create missed call from snapshot",
				zap.Error(err),
				zap.String("call_id", data.CallID),
			)
		}
	}

	// Handle calls that disappeared (missed call_end)
	for _, ended := range diff.EndedCalls {
		h.processor.ProcessMissedCallEnd(ctx, ended)
	}

	// Handle still-active calls (lightweight metric tracking)
	for _, data := range diff.UpdatedCalls {
		h.processor.ProcessCallActiveUpdate(ctx, data)
	}
}

func (h *Handlers) handleCallStart(ctx context.Context, payload []byte) {
	var msg CallMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		h.logger.Error("Failed to parse call_start message", zap.Error(err))
		return
	}

	callData := h.convertCallData(msg.InstanceID, msg.Timestamp, &msg.Call)
	callData.RawJSON = payload

	if err := h.processor.ProcessCallStart(ctx, callData); err != nil {
		h.logger.Error("Failed to process call start",
			zap.Error(err),
			zap.String("call_id", msg.Call.ID),
		)
	}
}

func (h *Handlers) handleCallEnd(ctx context.Context, payload []byte) {
	var msg CallMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		h.logger.Error("Failed to parse call_end message", zap.Error(err))
		return
	}

	callData := h.convertCallData(msg.InstanceID, msg.Timestamp, &msg.Call)
	callData.RawJSON = payload

	if err := h.processor.ProcessCallEnd(ctx, callData); err != nil {
		h.logger.Error("Failed to process call end",
			zap.Error(err),
			zap.String("call_id", msg.Call.ID),
		)
	}
}

func (h *Handlers) handleAudio(ctx context.Context, payload []byte) {
	var msg AudioMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		h.logger.Error("Failed to parse audio message", zap.Error(err))
		return
	}

	// Audio metadata is nested under call.metadata
	meta := msg.Call.Metadata

	h.logger.Debug("Processing audio",
		zap.Int("tgid", meta.TGID),
		zap.Int64("start_time", meta.StartTime),
		zap.String("short_name", meta.ShortName),
		zap.String("filename", meta.Filename),
		zap.Int("src_count", len(meta.SrcList)),
		zap.Int("freq_count", len(meta.FreqList)),
	)

	// Prefer M4A audio if available, otherwise use WAV
	audioData := msg.Call.AudioM4aB64
	if audioData == "" {
		audioData = msg.Call.AudioWavB64
	}

	// Convert srcList
	srcList := make([]ingest.SourceUnitData, len(meta.SrcList))
	for i, src := range meta.SrcList {
		srcList[i] = ingest.SourceUnitData{
			Src:       src.Src,
			Time:      ParseTimestamp(src.Time),
			Pos:       src.Pos,
			Emergency: src.Emergency != 0,
			Tag:       src.Tag,
		}
	}

	// Convert freqList
	freqList := make([]ingest.FreqEntryData, len(meta.FreqList))
	for i, f := range meta.FreqList {
		freqList[i] = ingest.FreqEntryData{
			Freq:       f.Freq,
			Time:       ParseTimestamp(f.Time),
			Pos:        f.Pos,
			Len:        f.Len,
			ErrorCount: f.ErrorCount,
			SpikeCount: f.SpikeCount,
		}
	}

	if err := h.processor.ProcessAudio(ctx, &ingest.AudioData{
		InstanceID: msg.InstanceID,
		ShortName:  meta.ShortName,
		TGID:       meta.TGID,
		TGAlphaTag: meta.TGTag,
		TGDesc:     meta.TGDesc,
		TGGroup:    meta.TGGroup,
		TGTag:      meta.TGGroupTag,
		StartTime:  ParseTimestamp(meta.StartTime),
		StopTime:   ParseTimestamp(meta.StopTime),
		Freq:       float64(meta.Freq),
		FreqError:  meta.FreqError,
		SignalDB:   meta.SignalDB,
		NoiseDB:    meta.NoiseDB,
		Encrypted:  meta.Encrypted != 0,
		Emergency:  meta.Emergency != 0,
		Phase2TDMA: meta.Phase2TDMA != 0,
		TDMASlot:   meta.TDMASlot,
		AudioType:  meta.AudioType,
		AudioData:  audioData,
		Filename:   meta.Filename,
		SrcList:    srcList,
		FreqList:   freqList,
	}); err != nil {
		h.logger.Error("Failed to process audio",
			zap.Error(err),
			zap.String("filename", meta.Filename),
		)
	}
}

func (h *Handlers) convertCallData(instanceID string, timestamp int64, call *CallData) *ingest.CallEventData {
	return &ingest.CallEventData{
		InstanceID:    instanceID,
		CallID:        call.ID,
		CallNum:       call.CallNum,
		Freq:          int64(call.Freq),
		FreqError:     call.FreqError,
		SysNum:        call.SysNum,
		ShortName:     call.ShortName,
		TGID:          call.TGID,
		TGAlphaTag:    call.TGAlphaTag,
		TGTag:         call.TGTag,
		TGGroup:       call.TGGroup,
		TGDesc:        call.TGDesc,
		StartTime:     ParseTimestamp(call.StartTime),
		StopTime:      ParseTimestamp(call.StopTime),
		Duration:      call.Length,
		Encrypted:     call.Encrypted,
		Emergency:     call.Emergency,
		Phase2TDMA:    call.Phase2TDMA,
		TDMASlot:      call.TDMASlot,
		Conventional:  call.Conventional,
		Analog:        call.Analog,
		AudioType:     call.AudioType,
		ErrorCount:    call.ErrorCount,
		SpikeCount:    call.SpikeCount,
		Unit:          call.Unit,
		UnitAlphaTag:  call.UnitAlphaTag,
		RecNum:        call.RecNum,
		SrcNum:        call.SrcNum,
		RecState:      call.RecState,
		RecStateType:  call.RecStateType,
		MonState:      call.MonState,
		MonStateType:  call.MonStateType,
		CallState:     call.CallState,
		CallStateType: call.CallStateType,
		SignalDB:      call.SignalDB,
		NoiseDB:       call.NoiseDB,
		Timestamp:     ParseTimestamp(timestamp),
		CallFilename:  call.CallFilename,
	}
}
