-- Rollback: SYSID-based scoping for talkgroups and units
--
-- WARNING: This rollback is LOSSY. The merge operation in the up migration
-- combined duplicate talkgroups/units. This rollback cannot recreate those
-- duplicates - it will assign all talkgroups/units to a single arbitrary site.
--
-- This is primarily useful for development/testing rollbacks, not production.

BEGIN;

-- Drop new indexes and constraints
DROP INDEX IF EXISTS idx_talkgroups_sysid;
DROP INDEX IF EXISTS idx_units_sysid;

ALTER TABLE talkgroups DROP CONSTRAINT IF EXISTS talkgroups_sysid_tgid_key;
ALTER TABLE units DROP CONSTRAINT IF EXISTS units_sysid_unit_id_key;

-- Add system_id column back
ALTER TABLE talkgroups ADD COLUMN system_id INTEGER REFERENCES systems(id) ON DELETE CASCADE;
ALTER TABLE units ADD COLUMN system_id INTEGER REFERENCES systems(id) ON DELETE CASCADE;

-- Populate system_id from junction tables (picks arbitrary site if multiple)
UPDATE talkgroups t
SET system_id = (
    SELECT ts.system_id
    FROM talkgroup_sites ts
    WHERE ts.talkgroup_id = t.id
    ORDER BY ts.first_seen ASC
    LIMIT 1
);

UPDATE units u
SET system_id = (
    SELECT us.system_id
    FROM unit_sites us
    WHERE us.unit_id = u.id
    ORDER BY us.first_seen ASC
    LIMIT 1
);

-- For any orphans (no site association), try to find a system with matching sysid
UPDATE talkgroups t
SET system_id = (
    SELECT s.id
    FROM systems s
    WHERE COALESCE(s.sysid, s.short_name) = t.sysid
    LIMIT 1
)
WHERE t.system_id IS NULL;

UPDATE units u
SET system_id = (
    SELECT s.id
    FROM systems s
    WHERE COALESCE(s.sysid, s.short_name) = u.sysid
    LIMIT 1
)
WHERE u.system_id IS NULL;

-- Drop junction tables
DROP TABLE IF EXISTS talkgroup_sites;
DROP TABLE IF EXISTS unit_sites;

-- Drop sysid column
ALTER TABLE talkgroups DROP COLUMN sysid;
ALTER TABLE units DROP COLUMN sysid;

-- Restore original unique constraints
ALTER TABLE talkgroups ADD CONSTRAINT talkgroups_system_id_tgid_key UNIQUE(system_id, tgid);
ALTER TABLE units ADD CONSTRAINT units_system_id_unit_id_key UNIQUE(system_id, unit_id);

COMMIT;
