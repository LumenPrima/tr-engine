package export

import "time"

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
	SystemIDs     []int `json:"system_ids,omitempty"`
	IncludeAudio  bool  `json:"include_audio"`
	IncludeEvents bool  `json:"include_events"`
}

// ManifestCounts records entity counts in the archive.
type ManifestCounts struct {
	Systems            int `json:"systems"`
	Sites              int `json:"sites"`
	Talkgroups         int `json:"talkgroups"`
	TalkgroupDirectory int `json:"talkgroup_directory"`
	Units              int `json:"units"`
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
