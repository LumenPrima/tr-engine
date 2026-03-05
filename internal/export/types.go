package export

import (
	"encoding/json"
	"time"
)

// Manifest describes the export archive contents.
type Manifest struct {
	Version        int             `json:"version"`
	Format         string          `json:"format"`
	CreatedAt      time.Time       `json:"created_at"`
	SourceInstance string          `json:"source_instance"`
	Filters        ManifestFilters `json:"filters"`
	Counts         ManifestCounts  `json:"counts"`
}

// ManifestFilters records which filters were applied during export.
type ManifestFilters struct {
	SystemIDs     []int      `json:"system_ids,omitempty"`
	IncludeAudio  bool       `json:"include_audio"`
	IncludeEvents bool       `json:"include_events"`
	TimeRange     *TimeRange `json:"time_range,omitempty"`
}

// TimeRange represents an optional start/end time filter.
type TimeRange struct {
	Start *time.Time `json:"start,omitempty"`
	End   *time.Time `json:"end,omitempty"`
}

// ManifestCounts records entity counts in the archive.
type ManifestCounts struct {
	Systems            int `json:"systems"`
	Sites              int `json:"sites"`
	Talkgroups         int `json:"talkgroups"`
	TalkgroupDirectory int `json:"talkgroup_directory"`
	Units              int `json:"units"`
	Calls              int `json:"calls"`
	Transcriptions     int `json:"transcriptions"`
	AudioFiles         int `json:"audio_files"`
}

// SystemRef is a natural key reference to a system.
// P25: sysid+wacn. Conventional: identified via site refs.
type SystemRef struct {
	Sysid string `json:"sysid,omitempty"`
	Wacn  string `json:"wacn,omitempty"`
}

// SiteRef is a natural key reference to a site.
type SiteRef struct {
	InstanceID string `json:"instance_id"`
	ShortName  string `json:"short_name"`
}

// SystemRecord is a JSONL record for a system.
type SystemRecord struct {
	V     int       `json:"_v"`
	Type  string    `json:"type"`
	Name  string    `json:"name"`
	Sysid string    `json:"sysid,omitempty"`
	Wacn  string    `json:"wacn,omitempty"`
	Sites []SiteRef `json:"sites,omitempty"` // for conventional systems
}

// SiteRecord is a JSONL record for a site.
type SiteRecord struct {
	V          int       `json:"_v"`
	SystemRef  SystemRef `json:"system_ref"`
	InstanceID string    `json:"instance_id"`
	ShortName  string    `json:"short_name"`
	Nac        string    `json:"nac,omitempty"`
}

// TalkgroupRecord is a JSONL record for a heard talkgroup.
type TalkgroupRecord struct {
	V              int        `json:"_v"`
	SystemRef      SystemRef  `json:"system_ref"`
	Tgid           int        `json:"tgid"`
	AlphaTag       string     `json:"alpha_tag,omitempty"`
	AlphaTagSource string     `json:"alpha_tag_source,omitempty"`
	Tag            string     `json:"tag,omitempty"`
	Group          string     `json:"group,omitempty"`
	Description    string     `json:"description,omitempty"`
	Mode           string     `json:"mode,omitempty"`
	Priority       *int       `json:"priority,omitempty"`
	FirstSeen      *time.Time `json:"first_seen,omitempty"`
	LastSeen       *time.Time `json:"last_seen,omitempty"`
}

// TalkgroupDirectoryRecord is a JSONL record for a talkgroup directory entry.
type TalkgroupDirectoryRecord struct {
	V           int       `json:"_v"`
	SystemRef   SystemRef `json:"system_ref"`
	Tgid        int       `json:"tgid"`
	AlphaTag    string    `json:"alpha_tag,omitempty"`
	Mode        string    `json:"mode,omitempty"`
	Description string    `json:"description,omitempty"`
	Tag         string    `json:"tag,omitempty"`
	Category    string    `json:"category,omitempty"`
	Priority    *int      `json:"priority,omitempty"`
}

// UnitRecord is a JSONL record for a unit.
type UnitRecord struct {
	V              int        `json:"_v"`
	SystemRef      SystemRef  `json:"system_ref"`
	UnitID         int        `json:"unit_id"`
	AlphaTag       string     `json:"alpha_tag,omitempty"`
	AlphaTagSource string     `json:"alpha_tag_source,omitempty"`
	FirstSeen      *time.Time `json:"first_seen,omitempty"`
	LastSeen       *time.Time `json:"last_seen,omitempty"`
}

// CallRecord is a JSONL record for a call.
type CallRecord struct {
	V             int             `json:"_v"`
	SystemRef     SystemRef       `json:"system_ref"`
	SiteRef       *SiteRef        `json:"site_ref,omitempty"`
	Tgid          int             `json:"tgid"`
	StartTime     time.Time       `json:"start_time"`
	StopTime      *time.Time      `json:"stop_time,omitempty"`
	Duration      *float32        `json:"duration,omitempty"`
	Freq          *int64          `json:"freq,omitempty"`
	FreqError     *int            `json:"freq_error,omitempty"`
	SignalDB      *float32        `json:"signal_db,omitempty"`
	NoiseDB       *float32        `json:"noise_db,omitempty"`
	ErrorCount    *int            `json:"error_count,omitempty"`
	SpikeCount    *int            `json:"spike_count,omitempty"`
	AudioType     string          `json:"audio_type,omitempty"`
	AudioFilePath string          `json:"audio_file_path,omitempty"`
	AudioFileSize *int            `json:"audio_file_size,omitempty"`
	Phase2TDMA    bool            `json:"phase2_tdma,omitempty"`
	TDMASlot      *int16          `json:"tdma_slot,omitempty"`
	Analog        bool            `json:"analog,omitempty"`
	Conventional  bool            `json:"conventional,omitempty"`
	Encrypted     bool            `json:"encrypted,omitempty"`
	Emergency     bool            `json:"emergency,omitempty"`
	PatchedTgids  []int           `json:"patched_tgids,omitempty"`
	SrcList       json.RawMessage `json:"src_list,omitempty"`
	FreqList      json.RawMessage `json:"freq_list,omitempty"`
	UnitIDs       []int           `json:"unit_ids,omitempty"`
	MetadataJSON  json.RawMessage `json:"metadata_json,omitempty"`
	IncidentData  json.RawMessage `json:"incident_data,omitempty"`
	InstanceID    string          `json:"instance_id,omitempty"`
}

// TranscriptionRecord is a JSONL record for a transcription.
type TranscriptionRecord struct {
	V             int             `json:"_v"`
	SystemRef     SystemRef       `json:"system_ref"`
	Tgid          int             `json:"tgid"`
	CallStartTime time.Time       `json:"call_start_time"`
	Text          string          `json:"text,omitempty"`
	Source        string          `json:"source"`
	IsPrimary     bool            `json:"is_primary,omitempty"`
	Confidence    *float32        `json:"confidence,omitempty"`
	Language      string          `json:"language,omitempty"`
	Model         string          `json:"model,omitempty"`
	Provider      string          `json:"provider,omitempty"`
	WordCount     int             `json:"word_count,omitempty"`
	DurationMs    int             `json:"duration_ms,omitempty"`
	ProviderMs    *int            `json:"provider_ms,omitempty"`
	Words         json.RawMessage `json:"words,omitempty"`
}
