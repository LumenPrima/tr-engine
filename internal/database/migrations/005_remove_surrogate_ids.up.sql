-- Migration: Remove surrogate IDs from talkgroups and units
--
-- This migration removes the serial `id` columns from talkgroups and units,
-- using the natural keys (sysid, tgid) and (sysid, unit_id) as primary keys.
--
-- WARNING: This is a breaking change that requires a fresh database.

BEGIN;

-- ============================================================================
-- STEP 1: Add sysid columns to referencing tables
-- ============================================================================

-- calls: add tg_sysid for talkgroup lookup
ALTER TABLE calls ADD COLUMN tg_sysid VARCHAR(16);

-- call_groups: add tg_sysid for talkgroup lookup
ALTER TABLE call_groups ADD COLUMN tg_sysid VARCHAR(16);

-- unit_events: add tg_sysid for talkgroup lookup, unit_sysid for unit lookup
ALTER TABLE unit_events ADD COLUMN tg_sysid VARCHAR(16);
ALTER TABLE unit_events ADD COLUMN unit_sysid VARCHAR(16);

-- transmissions: add unit_sysid for unit lookup
ALTER TABLE transmissions ADD COLUMN unit_sysid VARCHAR(16);

-- ============================================================================
-- STEP 2: Populate sysid columns from existing FK relationships
-- ============================================================================

-- Populate tg_sysid in calls
UPDATE calls c
SET tg_sysid = t.sysid
FROM talkgroups t
WHERE c.talkgroup_id = t.id;

-- Populate tg_sysid in call_groups
UPDATE call_groups cg
SET tg_sysid = t.sysid
FROM talkgroups t
WHERE cg.talkgroup_id = t.id;

-- Populate tg_sysid and unit_sysid in unit_events
UPDATE unit_events ue
SET tg_sysid = t.sysid
FROM talkgroups t
WHERE ue.talkgroup_id = t.id;

UPDATE unit_events ue
SET unit_sysid = u.sysid
FROM units u
WHERE ue.unit_id = u.id;

-- Populate unit_sysid in transmissions
UPDATE transmissions tx
SET unit_sysid = u.sysid
FROM units u
WHERE tx.unit_id = u.id;

-- ============================================================================
-- STEP 3: Update junction tables to use composite keys
-- ============================================================================

-- talkgroup_sites: add sysid, tgid columns
ALTER TABLE talkgroup_sites ADD COLUMN sysid VARCHAR(16);
ALTER TABLE talkgroup_sites ADD COLUMN tgid INTEGER;

UPDATE talkgroup_sites ts
SET sysid = t.sysid, tgid = t.tgid
FROM talkgroups t
WHERE ts.talkgroup_id = t.id;

-- unit_sites: add sysid, unit_id columns
ALTER TABLE unit_sites ADD COLUMN sysid VARCHAR(16);
ALTER TABLE unit_sites ADD COLUMN rid BIGINT;

UPDATE unit_sites us
SET sysid = u.sysid, rid = u.unit_id
FROM units u
WHERE us.unit_id = u.id;

-- ============================================================================
-- STEP 4: Drop old FK columns and constraints
-- ============================================================================

-- Drop FK constraints first
ALTER TABLE calls DROP CONSTRAINT IF EXISTS calls_talkgroup_id_fkey;
ALTER TABLE call_groups DROP CONSTRAINT IF EXISTS call_groups_talkgroup_id_fkey;
ALTER TABLE unit_events DROP CONSTRAINT IF EXISTS unit_events_talkgroup_id_fkey;
ALTER TABLE unit_events DROP CONSTRAINT IF EXISTS unit_events_unit_id_fkey;
ALTER TABLE transmissions DROP CONSTRAINT IF EXISTS transmissions_unit_id_fkey;
ALTER TABLE talkgroup_sites DROP CONSTRAINT IF EXISTS talkgroup_sites_talkgroup_id_fkey;
ALTER TABLE unit_sites DROP CONSTRAINT IF EXISTS unit_sites_unit_id_fkey;

-- Drop indexes on old FK columns
DROP INDEX IF EXISTS idx_calls_talkgroup;
DROP INDEX IF EXISTS idx_transmissions_unit;
DROP INDEX IF EXISTS idx_unit_events_unit;
DROP INDEX IF EXISTS idx_call_groups_talkgroup;

-- Drop old FK columns
ALTER TABLE calls DROP COLUMN talkgroup_id;
ALTER TABLE call_groups DROP COLUMN talkgroup_id;
ALTER TABLE unit_events DROP COLUMN talkgroup_id;
ALTER TABLE unit_events DROP COLUMN unit_id;
ALTER TABLE transmissions DROP COLUMN unit_id;

-- ============================================================================
-- STEP 5: Update talkgroup_sites and unit_sites primary keys
-- ============================================================================

-- Drop old PK and FK columns from junction tables
ALTER TABLE talkgroup_sites DROP CONSTRAINT talkgroup_sites_pkey;
ALTER TABLE talkgroup_sites DROP COLUMN talkgroup_id;
ALTER TABLE talkgroup_sites ALTER COLUMN sysid SET NOT NULL;
ALTER TABLE talkgroup_sites ALTER COLUMN tgid SET NOT NULL;
ALTER TABLE talkgroup_sites ADD PRIMARY KEY (sysid, tgid, system_id);

ALTER TABLE unit_sites DROP CONSTRAINT unit_sites_pkey;
ALTER TABLE unit_sites DROP COLUMN unit_id;
ALTER TABLE unit_sites ALTER COLUMN sysid SET NOT NULL;
ALTER TABLE unit_sites ALTER COLUMN rid SET NOT NULL;
ALTER TABLE unit_sites ADD PRIMARY KEY (sysid, rid, system_id);

-- ============================================================================
-- STEP 6: Change talkgroups primary key to (sysid, tgid)
-- ============================================================================

-- Drop the old PK and unique constraint
ALTER TABLE talkgroups DROP CONSTRAINT talkgroups_pkey;
ALTER TABLE talkgroups DROP CONSTRAINT talkgroups_sysid_tgid_key;

-- Drop the id column
ALTER TABLE talkgroups DROP COLUMN id;

-- Add new composite primary key
ALTER TABLE talkgroups ADD PRIMARY KEY (sysid, tgid);

-- ============================================================================
-- STEP 7: Change units primary key to (sysid, unit_id)
-- ============================================================================

-- Drop the old PK and unique constraint
ALTER TABLE units DROP CONSTRAINT units_pkey;
ALTER TABLE units DROP CONSTRAINT units_sysid_unit_id_key;

-- Drop the id column
ALTER TABLE units DROP COLUMN id;

-- Add new composite primary key
ALTER TABLE units ADD PRIMARY KEY (sysid, unit_id);

-- ============================================================================
-- STEP 8: Create indexes for the new schema
-- ============================================================================

-- Calls: index on talkgroup lookup
CREATE INDEX idx_calls_talkgroup ON calls(tg_sysid, tgid, start_time DESC) WHERE tg_sysid IS NOT NULL;

-- Transmissions: index on unit lookup
CREATE INDEX idx_transmissions_unit ON transmissions(unit_sysid, unit_rid, start_time DESC) WHERE unit_sysid IS NOT NULL;

-- Unit events: indexes on talkgroup and unit lookups
CREATE INDEX idx_unit_events_talkgroup ON unit_events(tg_sysid, tgid, time DESC) WHERE tg_sysid IS NOT NULL;
CREATE INDEX idx_unit_events_unit ON unit_events(unit_sysid, unit_rid, time DESC) WHERE unit_sysid IS NOT NULL;

-- Talkgroups: keep last_seen index
CREATE INDEX idx_talkgroups_last_seen ON talkgroups(last_seen DESC);

-- Units: keep last_seen index
CREATE INDEX idx_units_last_seen ON units(last_seen DESC);

COMMIT;
