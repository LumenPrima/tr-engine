package models

import (
	"encoding/json"
	"fmt"
	"time"
)

// Instance represents a trunk-recorder instance
// @Description A trunk-recorder instance that sends data to tr-engine
type Instance struct {
	ID          int             `json:"id" example:"1"`
	InstanceID  string          `json:"instance_id" example:"tr-main"`
	InstanceKey string          `json:"instance_key,omitempty" example:"secret-key"`
	FirstSeen   time.Time       `json:"first_seen" example:"2024-01-15T10:30:00Z"`
	LastSeen    time.Time       `json:"last_seen" example:"2024-01-15T12:45:00Z"`
	ConfigJSON  json.RawMessage `json:"config_json,omitempty" swaggertype:"object"`
}

// Source represents an SDR source
// @Description An SDR hardware source used for receiving radio signals
type Source struct {
	ID         int             `json:"id" example:"1"`
	InstanceID int             `json:"instance_id" example:"1"`
	SourceNum  int             `json:"source_num" example:"0"`
	CenterFreq int64           `json:"center_freq,omitempty" example:"851000000"`
	Rate       int             `json:"rate,omitempty" example:"8000000"`
	Driver     string          `json:"driver,omitempty" example:"osmosdr"`
	Device     string          `json:"device,omitempty" example:"rtl=0"`
	Antenna    string          `json:"antenna,omitempty" example:"RX"`
	Gain       int             `json:"gain,omitempty" example:"40"`
	ConfigJSON json.RawMessage `json:"config_json,omitempty" swaggertype:"object"`
}

// System represents a radio system
// @Description A radio system (P25, SmartNet, conventional, etc.)
type System struct {
	ID         int             `json:"id" example:"1"`
	InstanceID int             `json:"instance_id" example:"1"`
	SysNum     int             `json:"sys_num" example:"0"`
	ShortName  string          `json:"short_name" example:"butco"`
	SystemType string          `json:"system_type,omitempty" example:"p25"`
	SysID      string          `json:"sysid,omitempty" example:"1234"`
	WACN       string          `json:"wacn,omitempty" example:"BEE00"`
	NAC        string          `json:"nac,omitempty" example:"293"`
	RFSS       int             `json:"rfss,omitempty" example:"1"`
	SiteID     int             `json:"site_id,omitempty" example:"1"`
	ConfigJSON json.RawMessage `json:"config_json,omitempty" swaggertype:"object"`
}

// Talkgroup represents a talkgroup
// @Description A talkgroup on a radio system, unique within a SYSID
type Talkgroup struct {
	SYSID       string    `json:"sysid" example:"348"`
	TGID        int       `json:"tgid" example:"1001"`
	AlphaTag    string    `json:"alpha_tag,omitempty" example:"Fire Dispatch"`
	Description string    `json:"description,omitempty" example:"County Fire Dispatch Channel"`
	Group       string    `json:"group,omitempty" example:"Fire"`
	Tag         string    `json:"tag,omitempty" example:"Fire Dispatch"`
	Priority    int       `json:"priority,omitempty" example:"1"`
	Mode        string    `json:"mode,omitempty" example:"D"`
	FirstSeen   time.Time `json:"first_seen" example:"2024-01-01T00:00:00Z"`
	LastSeen    time.Time `json:"last_seen" example:"2024-01-15T12:30:00Z"`

	// Stats (populated by list/detail queries)
	CallCount  int `json:"call_count" example:"1547"`
	Calls1h    int `json:"calls_1h" example:"12"`
	Calls24h   int `json:"calls_24h" example:"234"`
	UnitCount  int `json:"unit_count" example:"45"`
}

// TalkgroupSite tracks which sites have seen a talkgroup
// @Description Association between a talkgroup and the sites where it has been observed
type TalkgroupSite struct {
	SYSID     string    `json:"sysid" example:"348"`
	TGID      int       `json:"tgid" example:"1001"`
	SystemID  int       `json:"system_id" example:"1"`
	FirstSeen time.Time `json:"first_seen" example:"2024-01-01T00:00:00Z"`
	LastSeen  time.Time `json:"last_seen" example:"2024-01-15T12:30:00Z"`
}

// Unit represents a radio unit
// @Description A radio unit (mobile or portable radio), unique within a SYSID
type Unit struct {
	SYSID          string    `json:"sysid" example:"348"`
	UnitID         int64     `json:"unit_id" example:"1234567"`
	AlphaTag       string    `json:"alpha_tag,omitempty" example:"Engine 1"`
	AlphaTagSource string    `json:"alpha_tag_source,omitempty" example:"radioreference"`
	FirstSeen      time.Time `json:"first_seen" example:"2024-01-01T00:00:00Z"`
	LastSeen       time.Time `json:"last_seen" example:"2024-01-15T12:30:00Z"`

	// Joined fields (not stored in units table)
	LastEventType  *string    `json:"last_event_type,omitempty" example:"call"`
	LastEventTGID  *int64     `json:"last_event_tgid,omitempty" example:"9178"`
	LastEventTGTag *string    `json:"last_event_tg_tag,omitempty" example:"09-8L Main"`
	LastEventTime  *time.Time `json:"last_event_time,omitempty"`
}

// UnitSite tracks which sites have seen a unit
// @Description Association between a unit and the sites where it has been observed
type UnitSite struct {
	SYSID     string    `json:"sysid" example:"348"`
	UnitID    int64     `json:"unit_id" example:"1234567"`
	SystemID  int       `json:"system_id" example:"1"`
	FirstSeen time.Time `json:"first_seen" example:"2024-01-01T00:00:00Z"`
	LastSeen  time.Time `json:"last_seen" example:"2024-01-15T12:30:00Z"`
}

// Recorder represents an SDR recorder
// @Description A virtual recorder that records audio from an SDR source
type Recorder struct {
	ID         int    `json:"id" example:"1"`
	InstanceID int    `json:"instance_id" example:"1"`
	SourceID   *int   `json:"source_id,omitempty" example:"1"`
	RecNum     int    `json:"rec_num" example:"0"`
	RecType    string `json:"rec_type,omitempty" example:"p25"`
}

// CallGroup represents a deduplicated group of calls
// @Description A group of duplicate call recordings from multiple recorders
type CallGroup struct {
	ID            int64      `json:"id" example:"1"`
	SystemID      int        `json:"system_id" example:"1"`
	TgSysid       *string    `json:"tg_sysid,omitempty" example:"348"`
	TGID          int        `json:"tgid" example:"1001"`
	StartTime     time.Time  `json:"start_time" example:"2024-01-15T10:30:00Z"`
	EndTime       *time.Time `json:"end_time,omitempty" example:"2024-01-15T10:30:45Z"`
	PrimaryCallID *int64     `json:"primary_call_id,omitempty" example:"100"`
	CallCount     int        `json:"call_count" example:"2"`
	Encrypted     bool       `json:"encrypted" example:"false"`
	Emergency     bool       `json:"emergency" example:"false"`
}

// Call represents an individual call recording
// @Description A recorded radio call/transmission
type Call struct {
	ID          int64   `json:"-"` // Internal database ID, not exposed
	CallGroupID *int64  `json:"call_group_id,omitempty" example:"1"`
	InstanceID  int     `json:"-"` // Internal, not exposed
	SystemID    int     `json:"-"` // Internal, not exposed
	TgSysid     *string `json:"tg_sysid,omitempty" example:"348"`
	RecorderID  *int    `json:"-"` // Internal, not exposed

	// Identifiers
	CallID   string `json:"call_id,omitempty" example:"348:9173:1769997011"` // Deterministic ID: {sysid}:{tgid}:{start_unix}
	TRCallID string `json:"-"`                                               // Internal trunk-recorder ID
	CallNum  int64  `json:"-"`                                               // Internal

	// Timing
	StartTime time.Time  `json:"start_time" example:"2024-01-15T10:30:00Z"`
	StopTime  *time.Time `json:"stop_time,omitempty" example:"2024-01-15T10:30:45Z"`
	Duration  float32    `json:"duration,omitempty" example:"45.5"`

	// Status
	CallState    int16  `json:"call_state,omitempty" example:"3"`
	MonState     int16  `json:"mon_state,omitempty" example:"0"`
	Encrypted    bool   `json:"encrypted" example:"false"`
	Emergency    bool   `json:"emergency" example:"false"`
	Phase2TDMA   bool   `json:"phase2_tdma" example:"false"`
	TDMASlot     int16  `json:"tdma_slot,omitempty" example:"0"`
	Conventional bool   `json:"conventional" example:"false"`
	Analog       bool   `json:"analog" example:"false"`
	AudioType    string `json:"audio_type,omitempty" example:"digital"`

	// Quality
	Freq       int64   `json:"freq,omitempty" example:"851012500"`
	FreqError  int     `json:"freq_error,omitempty" example:"0"`
	ErrorCount int     `json:"error_count,omitempty" example:"2"`
	SpikeCount int     `json:"spike_count,omitempty" example:"0"`
	SignalDB   float32 `json:"signal_db,omitempty" example:"-45.5"`
	NoiseDB    float32 `json:"noise_db,omitempty" example:"-80.0"`

	// Audio
	AudioPath string `json:"audio_path,omitempty" example:"butco/2024/01/15/1001-1705319400.m4a"`
	AudioURL  string `json:"audio_url,omitempty" example:"/api/v1/calls/123/audio"`
	AudioSize int    `json:"audio_size,omitempty" example:"45000"`

	// Patches
	PatchedTGIDs []int `json:"patched_tgids,omitempty"`

	// Metadata
	MetadataJSON json.RawMessage `json:"metadata_json,omitempty" swaggertype:"object"`

	// Joined fields (not stored in calls table)
	TGID       *int64     `json:"tgid,omitempty" example:"9178"`
	TGAlphaTag *string    `json:"tg_alpha_tag,omitempty" example:"09-8L Main"`
	Units      []CallUnit `json:"units,omitempty"`
}

// CallUnit represents a unit involved in a call
type CallUnit struct {
	UnitRID  int64  `json:"unit_rid" example:"924003"`
	AlphaTag string `json:"alpha_tag,omitempty" example:"09 7COM3"`
}

// GenerateCallID creates a deterministic call ID from sysid, tgid, and start time
func (c *Call) GenerateCallID() string {
	sysid := ""
	if c.TgSysid != nil {
		sysid = *c.TgSysid
	}
	tgid := int64(0)
	if c.TGID != nil {
		tgid = *c.TGID
	}
	return fmt.Sprintf("%s:%d:%d", sysid, tgid, c.StartTime.Unix())
}

// PopulateCallID sets the CallID field if not already set
func (c *Call) PopulateCallID() {
	if c.CallID == "" {
		c.CallID = c.GenerateCallID()
	}
}

// Transmission represents a unit transmission within a call
// @Description A single unit's transmission within a call
type Transmission struct {
	ID         int64      `json:"id" example:"1"`
	CallID     int64      `json:"call_id" example:"1"`
	UnitSysid  *string    `json:"unit_sysid,omitempty" example:"348"`
	UnitRID    int64      `json:"unit_rid" example:"1234567"`
	StartTime  time.Time  `json:"start_time" example:"2024-01-15T10:30:00Z"`
	StopTime   *time.Time `json:"stop_time,omitempty" example:"2024-01-15T10:30:15Z"`
	Duration   float32    `json:"duration,omitempty" example:"15.0"`
	Position   float32    `json:"position,omitempty" example:"0.0"`
	Emergency  bool       `json:"emergency" example:"false"`
	ErrorCount int        `json:"error_count,omitempty" example:"0"`
	SpikeCount int        `json:"spike_count,omitempty" example:"0"`
}

// CallFrequency represents frequency usage within a call
// @Description Frequency usage information within a call
type CallFrequency struct {
	ID         int64     `json:"id" example:"1"`
	CallID     int64     `json:"call_id" example:"1"`
	Freq       int64     `json:"freq" example:"851012500"`
	Time       time.Time `json:"time" example:"2024-01-15T10:30:00Z"`
	Position   float32   `json:"position,omitempty" example:"0.0"`
	Duration   float32   `json:"duration,omitempty" example:"15.0"`
	ErrorCount int       `json:"error_count,omitempty" example:"0"`
	SpikeCount int       `json:"spike_count,omitempty" example:"0"`
}

// UnitEvent represents a unit event (registration, affiliation, etc.)
// @Description A unit event such as registration, affiliation, or call activity
type UnitEvent struct {
	ID           int64           `json:"id" example:"1"`
	InstanceID   int             `json:"instance_id" example:"1"`
	SystemID     int             `json:"system_id" example:"1"`
	UnitSysid    *string         `json:"unit_sysid,omitempty" example:"348"`
	UnitRID      int64           `json:"unit_rid" example:"1234567"`
	EventType    string          `json:"event_type" example:"join"`
	TgSysid      *string         `json:"tg_sysid,omitempty" example:"348"`
	TGID         int             `json:"tgid,omitempty" example:"1001"`
	Time         time.Time       `json:"time" example:"2024-01-15T10:30:00Z"`
	MetadataJSON json.RawMessage `json:"metadata_json,omitempty" swaggertype:"object"`
}

// SystemRate represents a system decode rate snapshot
// @Description A decode rate measurement for a system
type SystemRate struct {
	ID             int64     `json:"id" example:"1"`
	SystemID       int       `json:"system_id" example:"1"`
	Time           time.Time `json:"time" example:"2024-01-15T10:30:00Z"`
	DecodeRate     float32   `json:"decode_rate" example:"98.5"`
	ControlChannel int64     `json:"control_channel,omitempty" example:"851012500"`
}

// RecorderStatus represents a recorder status snapshot
// @Description Status information for a recorder
type RecorderStatus struct {
	ID         int64     `json:"id" example:"1"`
	RecorderID int       `json:"recorder_id" example:"1"`
	Time       time.Time `json:"time" example:"2024-01-15T10:30:00Z"`
	State      int16     `json:"state" example:"1"`
	Freq       int64     `json:"freq,omitempty" example:"851012500"`
	CallCount  int       `json:"call_count,omitempty" example:"100"`
	Duration   float32   `json:"duration,omitempty" example:"3600.0"`
	Squelched  bool      `json:"squelched" example:"false"`
}

// TrunkMessage represents a trunking message
// @Description A trunking control channel message
type TrunkMessage struct {
	ID          int64     `json:"id" example:"1"`
	SystemID    int       `json:"system_id" example:"1"`
	Time        time.Time `json:"time" example:"2024-01-15T10:30:00Z"`
	MsgType     int16     `json:"msg_type" example:"1"`
	MsgTypeName string    `json:"msg_type_name,omitempty" example:"GRP_V_CH_GRANT"`
	Opcode      string    `json:"opcode,omitempty" example:"00"`
	OpcodeType  string    `json:"opcode_type,omitempty" example:"OSP"`
	OpcodeDesc  string    `json:"opcode_desc,omitempty" example:"Group Voice Channel Grant"`
	Meta        string    `json:"meta,omitempty" example:"tg:1001"`
}

// TranscriptionWord represents a single word with timing information
// @Description A word from the transcription with start and end times
type TranscriptionWord struct {
	Word  string  `json:"word" example:"Engine"`
	Start float32 `json:"start" example:"0.5"`  // seconds from start of audio
	End   float32 `json:"end" example:"0.85"`   // seconds from start of audio
}

// Transcription represents a speech-to-text transcription result
// @Description A transcription of a radio call's audio content
type Transcription struct {
	ID           int64               `json:"id" example:"1"`
	CallID       int64               `json:"call_id" example:"123"`
	Provider     string              `json:"provider" example:"openai"`
	Model        string              `json:"model,omitempty" example:"whisper-1"`
	Language     string              `json:"language,omitempty" example:"en"`
	Text         string              `json:"text" example:"Engine 5 responding to the scene"`
	Confidence   *float32            `json:"confidence,omitempty" example:"0.95"`
	WordCount    int                 `json:"word_count,omitempty" example:"6"`
	DurationMs   int                 `json:"duration_ms,omitempty" example:"1500"`
	Words        []TranscriptionWord `json:"words,omitempty"`
	CreatedAt    time.Time           `json:"created_at" example:"2024-01-15T12:30:00Z"`
	CallDuration float32             `json:"call_duration,omitempty" example:"15.5"` // seconds, for word timeline rendering
}

// TranscriptionQueueItem represents a pending transcription job
// @Description A queued transcription job awaiting processing
type TranscriptionQueueItem struct {
	ID        int64     `json:"id" example:"1"`
	CallID    int64     `json:"call_id" example:"123"`
	Status    string    `json:"status" example:"pending"`
	Priority  int       `json:"priority" example:"0"`
	Attempts  int       `json:"attempts" example:"0"`
	LastError string    `json:"last_error,omitempty"`
	CreatedAt time.Time `json:"created_at" example:"2024-01-15T12:30:00Z"`
	UpdatedAt time.Time `json:"updated_at" example:"2024-01-15T12:30:00Z"`
}

// APIKey represents a database-managed API key
// @Description An API key for authenticating REST API requests
type APIKey struct {
	ID         int        `json:"id" example:"1"`
	KeyPrefix  string     `json:"key_prefix" example:"tr_api_a1b2"` // First 12 chars for identification
	Name       string     `json:"name" example:"frontend"`
	Scopes     []string   `json:"scopes,omitempty"`
	ReadOnly   bool       `json:"read_only" example:"false"`
	CreatedAt  time.Time  `json:"created_at" example:"2024-01-15T12:30:00Z"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}
