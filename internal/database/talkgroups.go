package database

import "context"

// UpsertTalkgroup inserts or updates a talkgroup, never overwriting good data with empty strings.
func (db *DB) UpsertTalkgroup(ctx context.Context, systemID, tgid int, alphaTag, tag, group, description string) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO talkgroups (system_id, tgid, alpha_tag, tag, "group", description, first_seen, last_seen)
		VALUES ($1, $2, $3, $4, $5, $6, now(), now())
		ON CONFLICT (system_id, tgid) DO UPDATE SET
			alpha_tag   = COALESCE(NULLIF($3, ''), talkgroups.alpha_tag),
			tag         = COALESCE(NULLIF($4, ''), talkgroups.tag),
			"group"     = COALESCE(NULLIF($5, ''), talkgroups."group"),
			description = COALESCE(NULLIF($6, ''), talkgroups.description),
			last_seen   = now()
	`, systemID, tgid, alphaTag, tag, group, description)
	return err
}
