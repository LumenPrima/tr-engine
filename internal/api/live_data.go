package api

import "time"

// LiveDataSource provides real-time data from the ingest pipeline to the API layer.
// The pipeline implements this interface — no circular imports since api owns the interface.
type LiveDataSource interface {
	// ActiveCalls returns currently in-progress calls from the MQTT tracker.
	ActiveCalls() []ActiveCallData

	// LatestRecorders returns the most recent recorder state snapshot.
	LatestRecorders() []RecorderStateData

	// TRInstanceStatus returns the cached status of all known TR instances.
	TRInstanceStatus() []TRInstanceStatusData

	// UnitAffiliations returns current talkgroup affiliation state for all tracked units.
	UnitAffiliations() []UnitAffiliationData

	// Subscribe returns a channel that receives SSE events matching the filter,
	// and a cancel function to unsubscribe.
	Subscribe(filter EventFilter) (<-chan SSEEvent, func())

	// ReplaySince returns buffered events since the given event ID (for Last-Event-ID recovery).
	ReplaySince(lastEventID string, filter EventFilter) []SSEEvent

	// WatcherStatus returns the file watcher status, or nil if not active.
	WatcherStatus() *WatcherStatusData
}

// WatcherStatusData represents the status of the file watcher ingest mode.
type WatcherStatusData struct {
	Status         string `json:"status"`           // "watching", "backfilling", "stopped"
	WatchDir       string `json:"watch_dir"`
	FilesProcessed int64  `json:"files_processed"`
	FilesSkipped   int64  `json:"files_skipped"`
}

// ActiveCallData represents an in-progress call from the pipeline.
type ActiveCallData struct {
	CallID        int64     `json:"call_id"`
	SystemID      int       `json:"system_id"`
	SystemName    string    `json:"system_name"`
	Sysid         string    `json:"sysid"`
	SiteID        *int      `json:"site_id,omitempty"`
	SiteShortName string    `json:"site_short_name,omitempty"`
	Tgid          int       `json:"tgid"`
	TgAlphaTag    string    `json:"tg_alpha_tag,omitempty"`
	TgDescription string    `json:"tg_description,omitempty"`
	TgTag         string    `json:"tg_tag,omitempty"`
	TgGroup       string    `json:"tg_group,omitempty"`
	StartTime     time.Time `json:"start_time"`
	Duration      float32   `json:"duration,omitempty"`
	Freq          int64     `json:"freq,omitempty"`
	Emergency     bool      `json:"emergency"`
	Encrypted     bool      `json:"encrypted"`
	Analog        bool      `json:"analog"`
	Conventional  bool      `json:"conventional"`
	Phase2TDMA    bool      `json:"phase2_tdma"`
	AudioType     string    `json:"audio_type,omitempty"`
}

// RecorderStateData represents a recorder's current state.
type RecorderStateData struct {
	ID           string  `json:"id"`
	InstanceID   string  `json:"instance_id"`
	SrcNum       int16   `json:"src_num"`
	RecNum       int16   `json:"rec_num"`
	Type         string  `json:"type"`
	RecState     string  `json:"rec_state"`
	Freq         int64   `json:"freq"`
	Duration     float32 `json:"duration"`
	Count        int     `json:"count"`
	Squelched    bool    `json:"squelched"`
	Tgid         *int    `json:"tgid,omitempty"`
	TgAlphaTag   *string `json:"tg_alpha_tag,omitempty"`
	UnitID       *int    `json:"unit_id,omitempty"`
	UnitAlphaTag *string `json:"unit_alpha_tag,omitempty"`
}

// TRInstanceStatusData represents the cached status of a trunk-recorder instance.
type TRInstanceStatusData struct {
	InstanceID string    `json:"instance_id"`
	Status     string    `json:"status"`
	LastSeen   time.Time `json:"last_seen"`
}

// UnitAffiliationData represents a unit's current talkgroup affiliation.
type UnitAffiliationData struct {
	SystemID        int       `json:"system_id"`
	SystemName      string    `json:"system_name,omitempty"`
	Sysid           string    `json:"sysid,omitempty"`
	UnitID          int       `json:"unit_id"`
	UnitAlphaTag    string    `json:"unit_alpha_tag,omitempty"`
	Tgid            int       `json:"tgid"`
	TgAlphaTag      string    `json:"tg_alpha_tag,omitempty"`
	TgDescription   string    `json:"tg_description,omitempty"`
	TgTag           string    `json:"tg_tag,omitempty"`
	TgGroup         string    `json:"tg_group,omitempty"`
	PreviousTgid    *int      `json:"previous_tgid,omitempty"`
	AffiliatedSince time.Time `json:"affiliated_since"`
	LastEventTime   time.Time `json:"last_event_time"`
	Status          string    `json:"status"` // "affiliated" or "off"
}

// EventFilter specifies which events an SSE subscriber wants to receive.
type EventFilter struct {
	Systems       []int
	Sites         []int
	Tgids         []int
	Units         []int
	Types         []string
	EmergencyOnly bool
}

// SSEEvent represents a server-sent event ready for transmission.
type SSEEvent struct {
	ID        string `json:"event_id"`
	Type      string `json:"event_type"`
	SubType   string `json:"sub_type,omitempty"`
	Timestamp string `json:"timestamp"`
	SystemID  int    `json:"system_id,omitempty"`
	SiteID    int    `json:"site_id,omitempty"`
	Tgid      int    `json:"tgid,omitempty"`
	UnitID    int    `json:"unit_id,omitempty"`
	Data      []byte `json:"-"` // pre-serialized JSON payload
}
