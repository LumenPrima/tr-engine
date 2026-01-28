package mqtt

import (
	"encoding/json"
	"time"
)

// StatusMessage is the base structure for status topic messages
type StatusMessage struct {
	Type       string          `json:"type"`
	InstanceID string          `json:"instance_id"`
	Timestamp  int64           `json:"timestamp"`
	RawJSON    json.RawMessage `json:"-"`
}

// ConfigMessage contains full trunk-recorder configuration
// Structure: {"type": "config", "config": {"sources": [...], "systems": [...]}, ...}
type ConfigMessage struct {
	Type       string       `json:"type"`
	InstanceID string       `json:"instance_id"`
	Timestamp  int64        `json:"timestamp"`
	Config     ConfigData   `json:"config"`
}

// ConfigData contains the nested configuration
type ConfigData struct {
	Sources    []SourceConfig       `json:"sources"`
	Systems    []SystemConfigDetail `json:"systems"`
	CaptureDir string               `json:"capture_dir"`
	InstanceID string               `json:"instance_id"`
	InstanceKey string              `json:"instance_key"`
}

// SourceConfig represents an SDR source configuration
type SourceConfig struct {
	SourceNum        int       `json:"source_num"`
	CenterFreq       float64   `json:"center"`
	Rate             float64   `json:"rate"`
	MinHz            float64   `json:"min_hz"`
	MaxHz            float64   `json:"max_hz"`
	Error            float64   `json:"error"`
	Driver           string    `json:"driver"`
	Device           string    `json:"device"`
	Antenna          string    `json:"antenna"`
	Gain             float64   `json:"gain"`
	AnalogRecorders  int       `json:"analog_recorders"`
	DigitalRecorders int       `json:"digital_recorders"`
}

// SystemConfigDetail represents a system in the config message
type SystemConfigDetail struct {
	SysNum         int       `json:"sys_num"`
	ShortName      string    `json:"sys_name"`
	SystemType     string    `json:"system_type"`
	TalkgroupsFile string    `json:"talkgroups_file"`
	QPSK           bool      `json:"qpsk"`
	SquelchDB      float64   `json:"squelch_db"`
	AnalogLevels   float64   `json:"analog_levels"`
	DigitalLevels  float64   `json:"digital_levels"`
	AudioArchive   bool      `json:"audio_archive"`
	RecordUnknown  bool      `json:"record_unkown"` // Note: typo in trunk-recorder
	CallLog        bool      `json:"call_log"`
	ControlChannel float64   `json:"control_channel"`
	Channels       []float64 `json:"channels"`
}

// SystemsMessage contains system status information
// Structure: {"type": "systems", "systems": [...], ...}
type SystemsMessage struct {
	Type       string         `json:"type"`
	InstanceID string         `json:"instance_id"`
	Timestamp  int64          `json:"timestamp"`
	Systems    []SystemStatus `json:"systems"`
}

// SystemStatus represents the status of a system
type SystemStatus struct {
	SysNum    int    `json:"sys_num"`
	ShortName string `json:"sys_name"`
	Type      string `json:"type"`
	SysID     string `json:"sysid"`
	WACN      string `json:"wacn"`
	NAC       string `json:"nac"`
	RFSS      int    `json:"rfss"`
	SiteID    int    `json:"site_id"`
}

// RatesMessage contains system decode rates
type RatesMessage struct {
	Type       string       `json:"type"`
	InstanceID string       `json:"instance_id"`
	Timestamp  int64        `json:"timestamp"`
	Rates      []RateStatus `json:"rates"`
}

// RateStatus represents decode rates for a system
type RateStatus struct {
	SysNum           int     `json:"sys_num"`
	ShortName        string  `json:"sys_name"`
	DecodeRate       float32 `json:"decoderate"`
	DecodeRateIntv   float32 `json:"decoderate_interval"`
	ControlChannel   float64 `json:"control_channel"`
}

// RecordersMessage contains multiple recorder statuses
type RecordersMessage struct {
	Type       string           `json:"type"`
	InstanceID string           `json:"instance_id"`
	Timestamp  int64            `json:"timestamp"`
	Recorders  []RecorderStatus `json:"recorders"`
}

// RecorderMessage contains a single recorder status update
type RecorderMessage struct {
	Type       string         `json:"type"`
	InstanceID string         `json:"instance_id"`
	Timestamp  int64          `json:"timestamp"`
	Recorder   RecorderStatus `json:"recorder"`
}

// RecorderStatus represents the status of a recorder
type RecorderStatus struct {
	ID        string  `json:"id"`
	Type      string  `json:"type"`
	SourceNum int     `json:"src_num"`
	RecNum    int     `json:"rec_num"`
	State     int     `json:"rec_state"`
	StateType string  `json:"rec_state_type"`
	Freq      float64 `json:"freq"`
	Count     int     `json:"count"`
	Duration  float32 `json:"duration"`
	Squelched bool    `json:"squelched"`
}

// CallMessage represents a call event (start, active, end)
type CallMessage struct {
	Type       string          `json:"type"`
	InstanceID string          `json:"instance_id"`
	Timestamp  int64           `json:"timestamp"`
	Call       CallData        `json:"call"`
	RawJSON    json.RawMessage `json:"-"`
}

// CallsActiveMessage contains multiple active calls
type CallsActiveMessage struct {
	Type       string     `json:"type"`
	InstanceID string     `json:"instance_id"`
	Timestamp  int64      `json:"timestamp"`
	Calls      []CallData `json:"calls"`
}

// CallData contains call information for call_start, call_end, calls_active
type CallData struct {
	ID               string  `json:"id"`
	CallNum          int64   `json:"call_num"`
	SysNum           int     `json:"sys_num"`
	ShortName        string  `json:"sys_name"`
	Freq             float64 `json:"freq"`
	FreqError        int     `json:"freq_error"`
	Unit             int64   `json:"unit"`
	UnitAlphaTag     string  `json:"unit_alpha_tag"`
	TGID             int     `json:"talkgroup"`
	TGAlphaTag       string  `json:"talkgroup_alpha_tag"`
	TGDesc           string  `json:"talkgroup_description"`
	TGGroup          string  `json:"talkgroup_group"`
	TGTag            string  `json:"talkgroup_tag"`
	TGPatches        string  `json:"talkgroup_patches"`
	Elapsed          int     `json:"elapsed"`
	Length           float32 `json:"length"`
	CallState        int     `json:"call_state"`
	CallStateType    string  `json:"call_state_type"`
	MonState         int     `json:"mon_state"`
	MonStateType     string  `json:"mon_state_type"`
	AudioType        string  `json:"audio_type"`
	Phase2TDMA       bool    `json:"phase2_tdma"`
	TDMASlot         int     `json:"tdma_slot"`
	Analog           bool    `json:"analog"`
	RecNum           int     `json:"rec_num"`
	SrcNum           int     `json:"src_num"`
	RecState         int     `json:"rec_state"`
	RecStateType     string  `json:"rec_state_type"`
	Conventional     bool    `json:"conventional"`
	Encrypted        bool    `json:"encrypted"`
	Emergency        bool    `json:"emergency"`
	StartTime        int64   `json:"start_time"`
	StopTime         int64   `json:"stop_time"`
	ProcessCallTime  int64   `json:"process_call_time"`
	ErrorCount       int     `json:"error_count"`
	SpikeCount       int     `json:"spike_count"`
	RetryAttempt     int     `json:"retry_attempt"`
	SignalDB         float32 `json:"signal"`
	NoiseDB          float32 `json:"noise"`
	CallFilename     string  `json:"call_filename"`
}

// AudioMessage contains base64-encoded audio data
// Structure: {"type": "audio", "call": {"audio_wav_base64": "...", "metadata": {...}}}
type AudioMessage struct {
	Type       string        `json:"type"`
	InstanceID string        `json:"instance_id"`
	Timestamp  int64         `json:"timestamp"`
	Call       AudioCallData `json:"call"`
}

// AudioCallData contains the audio and metadata
type AudioCallData struct {
	AudioWavB64 string        `json:"audio_wav_base64"`
	AudioM4aB64 string        `json:"audio_m4a_base64"`
	Metadata    AudioMetadata `json:"metadata"`
}

// AudioMetadata contains call metadata for audio messages
type AudioMetadata struct {
	Freq        int64   `json:"freq"`
	FreqError   int     `json:"freq_error"`
	SignalDB    float32 `json:"signal"`
	NoiseDB     float32 `json:"noise"`
	SourceNum   int     `json:"source_num"`
	RecorderNum int     `json:"recorder_num"`
	TDMASlot    int     `json:"tdma_slot"`
	Phase2TDMA  int     `json:"phase2_tdma"`
	StartTime   int64   `json:"start_time"`
	StopTime    int64   `json:"stop_time"`
	Emergency   int     `json:"emergency"`
	Priority    int     `json:"priority"`
	Mode        int     `json:"mode"`
	Duplex      int     `json:"duplex"`
	Encrypted   int     `json:"encrypted"`
	CallLength  int     `json:"call_length"`
	TGID        int     `json:"talkgroup"`
	TGTag       string  `json:"talkgroup_tag"`
	TGDesc      string  `json:"talkgroup_description"`
	TGGroupTag  string  `json:"talkgroup_group_tag"`
	TGGroup     string  `json:"talkgroup_group"`
	ColorCode   int     `json:"color_code"`
	AudioType   string  `json:"audio_type"`
	ShortName   string  `json:"short_name"`
	Filename    string  `json:"filename"`
	FreqList    []FreqEntry  `json:"freqList"`
	SrcList     []SourceUnit `json:"srcList"`
}

// SourceUnit represents a unit that transmitted during a call
type SourceUnit struct {
	Src          int64   `json:"src"`
	Time         int64   `json:"time"`
	Pos          float32 `json:"pos"`
	Emergency    int     `json:"emergency"`
	SignalSystem string  `json:"signal_system"`
	Tag          string  `json:"tag"`
}

// FreqEntry represents a frequency used during a call
type FreqEntry struct {
	Freq       int64   `json:"freq"`
	Time       int64   `json:"time"`
	Pos        float32 `json:"pos"`
	Len        float32 `json:"len"`
	ErrorCount int     `json:"error_count"`
	SpikeCount int     `json:"spike_count"`
}

// UnitMessage is a generic unit event for initial parsing
type UnitMessage struct {
	Type       string          `json:"type"`
	InstanceID string          `json:"instance_id"`
	Timestamp  int64           `json:"timestamp"`
	SysNum     int             `json:"sys_num"`
	ShortName  string          `json:"short_name"`
	Unit       int64           `json:"unit"`
	UnitTag    string          `json:"unit_alpha_tag"`
	TGID       int             `json:"talkgroup"`
	TGAlphaTag string          `json:"talkgroup_alpha_tag"`
	TGDesc     string          `json:"talkgroup_description"`
	TGGroup    string          `json:"talkgroup_group"`
	TGTag      string          `json:"talkgroup_tag"`
	RawJSON    json.RawMessage `json:"-"`
}

// UnitCallMessage represents a unit joining/on a call (type: "call")
// Structure: {"type": "call", "call": {...}}
type UnitCallMessage struct {
	Type       string       `json:"type"`
	InstanceID string       `json:"instance_id"`
	Timestamp  int64        `json:"timestamp"`
	Call       UnitCallData `json:"call"`
}

// UnitCallData contains unit call event data
type UnitCallData struct {
	SysNum      int     `json:"sys_num"`
	ShortName   string  `json:"sys_name"`
	Unit        int64   `json:"unit"`
	UnitAlphaTag string `json:"unit_alpha_tag"`
	TGID        int     `json:"talkgroup"`
	TGAlphaTag  string  `json:"talkgroup_alpha_tag"`
	TGDesc      string  `json:"talkgroup_description"`
	TGGroup     string  `json:"talkgroup_group"`
	TGTag       string  `json:"talkgroup_tag"`
	TGPatches   string  `json:"talkgroup_patches"`
	CallNum     int64   `json:"call_num"`
	Freq        float64 `json:"freq"`
	Encrypted   bool    `json:"encrypted"`
	StartTime   int64   `json:"start_time"`
}

// UnitEndMessage represents a transmission end (type: "end")
// Structure: {"type": "end", "end": {...}}
type UnitEndMessage struct {
	Type       string      `json:"type"`
	InstanceID string      `json:"instance_id"`
	Timestamp  int64       `json:"timestamp"`
	End        UnitEndData `json:"end"`
}

// UnitEndData contains transmission end data
type UnitEndData struct {
	SysNum               int     `json:"sys_num"`
	ShortName            string  `json:"sys_name"`
	Unit                 int64   `json:"unit"`
	UnitAlphaTag         string  `json:"unit_alpha_tag"`
	TGID                 int     `json:"talkgroup"`
	TGAlphaTag           string  `json:"talkgroup_alpha_tag"`
	TGDesc               string  `json:"talkgroup_description"`
	TGGroup              string  `json:"talkgroup_group"`
	TGTag                string  `json:"talkgroup_tag"`
	TGPatches            string  `json:"talkgroup_patches"`
	CallNum              int64   `json:"call_num"`
	Freq                 float64 `json:"freq"`
	Position             float32 `json:"position"`
	Length               float32 `json:"length"`
	Emergency            bool    `json:"emergency"`
	Encrypted            bool    `json:"encrypted"`
	StartTime            int64   `json:"start_time"`
	StopTime             int64   `json:"stop_time"`
	ErrorCount           int     `json:"error_count"`
	SpikeCount           int     `json:"spike_count"`
	SampleCount          int     `json:"sample_count"`
	TransmissionFilename string  `json:"transmission_filename"`
}

// TrunkingMessage represents a trunking message
type TrunkingMessage struct {
	Type        string `json:"type"`
	InstanceID  string `json:"instance_id"`
	Timestamp   int64  `json:"timestamp"`
	SysNum      int    `json:"sys_num"`
	ShortName   string `json:"short_name"`
	MsgType     int    `json:"msg_type"`
	MsgTypeName string `json:"msg_type_name"`
	Opcode      string `json:"opcode"`
	OpcodeType  string `json:"opcode_type"`
	OpcodeDesc  string `json:"opcode_desc"`
	Meta        string `json:"meta"`
}

// ParseTimestamp converts a Unix timestamp to time.Time
func ParseTimestamp(ts int64) time.Time {
	if ts > 1e12 {
		// Milliseconds
		return time.UnixMilli(ts)
	}
	return time.Unix(ts, 0)
}
