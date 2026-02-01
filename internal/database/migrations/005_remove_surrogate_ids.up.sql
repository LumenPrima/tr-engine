-- Migration: Remove surrogate IDs from talkgroups and units
--
-- This migration removes the serial `id` columns from talkgroups and units,
-- using the natural keys (sysid, tgid) and (sysid, unit_id) as primary keys.
--
-- WARNING: This is a breaking change. For existing databases with data,
-- the FK columns are populated first, then old columns are dropped.

BEGIN;

-- ============================================================================
-- STEP 1: Add natural key columns to referencing tables
-- ============================================================================

-- calls: add tg_sysid and tgid for talkgroup lookup
ALTER TABLE calls ADD COLUMN IF NOT EXISTS tg_sysid VARCHAR(16);
ALTER TABLE calls ADD COLUMN IF NOT EXISTS tgid INTEGER;

-- call_groups: add tg_sysid for talkgroup lookup
ALTER TABLE call_groups ADD COLUMN IF NOT EXISTS tg_sysid VARCHAR(16);

-- unit_events: add natural key columns
ALTER TABLE unit_events ADD COLUMN IF NOT EXISTS tg_sysid VARCHAR(16);
ALTER TABLE unit_events ADD COLUMN IF NOT EXISTS tgid INTEGER;
ALTER TABLE unit_events ADD COLUMN IF NOT EXISTS unit_sysid VARCHAR(16);
ALTER TABLE unit_events ADD COLUMN IF NOT EXISTS unit_rid BIGINT;

-- transmissions: add unit natural key columns
ALTER TABLE transmissions ADD COLUMN IF NOT EXISTS unit_sysid VARCHAR(16);
ALTER TABLE transmissions ADD COLUMN IF NOT EXISTS unit_rid BIGINT;

-- ============================================================================
-- STEP 2: Populate natural key columns from existing FK relationships
-- ============================================================================

-- Populate tg_sysid and tgid in calls (only if talkgroup_id column exists)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'calls' AND column_name = 'talkgroup_id') THEN
        UPDATE calls c SET tg_sysid = t.sysid, tgid = t.tgid
        FROM talkgroups t WHERE c.talkgroup_id = t.id AND c.tg_sysid IS NULL;
    END IF;
END $$;

-- Populate tg_sysid in call_groups
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'call_groups' AND column_name = 'talkgroup_id') THEN
        UPDATE call_groups cg SET tg_sysid = t.sysid
        FROM talkgroups t WHERE cg.talkgroup_id = t.id AND cg.tg_sysid IS NULL;
    END IF;
END $$;

-- Populate unit_events natural keys
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'unit_events' AND column_name = 'talkgroup_id') THEN
        UPDATE unit_events ue SET tg_sysid = t.sysid, tgid = t.tgid
        FROM talkgroups t WHERE ue.talkgroup_id = t.id AND ue.tg_sysid IS NULL;
    END IF;

    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'unit_events' AND column_name = 'unit_id') THEN
        UPDATE unit_events ue SET unit_sysid = u.sysid, unit_rid = u.unit_id
        FROM units u WHERE ue.unit_id = u.id AND ue.unit_sysid IS NULL;
    END IF;
END $$;

-- Populate transmissions natural keys
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'transmissions' AND column_name = 'unit_id') THEN
        UPDATE transmissions tx SET unit_sysid = u.sysid, unit_rid = u.unit_id
        FROM units u WHERE tx.unit_id = u.id AND tx.unit_sysid IS NULL;
    END IF;
END $$;

-- ============================================================================
-- STEP 3: Update junction tables to use composite keys
-- ============================================================================

-- talkgroup_sites: add sysid, tgid columns
ALTER TABLE talkgroup_sites ADD COLUMN IF NOT EXISTS sysid VARCHAR(16);
ALTER TABLE talkgroup_sites ADD COLUMN IF NOT EXISTS tgid INTEGER;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'talkgroup_sites' AND column_name = 'talkgroup_id') THEN
        UPDATE talkgroup_sites ts SET sysid = t.sysid, tgid = t.tgid
        FROM talkgroups t WHERE ts.talkgroup_id = t.id AND ts.sysid IS NULL;
    END IF;
END $$;

-- unit_sites: add sysid, rid columns
ALTER TABLE unit_sites ADD COLUMN IF NOT EXISTS sysid VARCHAR(16);
ALTER TABLE unit_sites ADD COLUMN IF NOT EXISTS rid BIGINT;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'unit_sites' AND column_name = 'unit_id') THEN
        UPDATE unit_sites us SET sysid = u.sysid, rid = u.unit_id
        FROM units u WHERE us.unit_id = u.id AND us.sysid IS NULL;
    END IF;
END $$;

-- ============================================================================
-- STEP 4: Drop old FK constraints (if they exist)
-- ============================================================================

ALTER TABLE calls DROP CONSTRAINT IF EXISTS calls_talkgroup_id_fkey;
ALTER TABLE call_groups DROP CONSTRAINT IF EXISTS call_groups_talkgroup_id_fkey;
ALTER TABLE unit_events DROP CONSTRAINT IF EXISTS unit_events_talkgroup_id_fkey;
ALTER TABLE unit_events DROP CONSTRAINT IF EXISTS unit_events_unit_id_fkey;
ALTER TABLE transmissions DROP CONSTRAINT IF EXISTS transmissions_unit_id_fkey;
ALTER TABLE talkgroup_sites DROP CONSTRAINT IF EXISTS talkgroup_sites_talkgroup_id_fkey;
ALTER TABLE unit_sites DROP CONSTRAINT IF EXISTS unit_sites_unit_id_fkey;

-- Drop old indexes
DROP INDEX IF EXISTS idx_calls_talkgroup;
DROP INDEX IF EXISTS idx_transmissions_unit;
DROP INDEX IF EXISTS idx_unit_events_unit;
DROP INDEX IF EXISTS idx_call_groups_talkgroup;

-- ============================================================================
-- STEP 5: Drop old FK columns (if they exist)
-- ============================================================================

ALTER TABLE calls DROP COLUMN IF EXISTS talkgroup_id;
ALTER TABLE call_groups DROP COLUMN IF EXISTS talkgroup_id;
ALTER TABLE unit_events DROP COLUMN IF EXISTS talkgroup_id;
ALTER TABLE unit_events DROP COLUMN IF EXISTS unit_id;
ALTER TABLE transmissions DROP COLUMN IF EXISTS unit_id;

-- ============================================================================
-- STEP 6: Update talkgroup_sites primary key
-- ============================================================================

ALTER TABLE talkgroup_sites DROP CONSTRAINT IF EXISTS talkgroup_sites_pkey;
ALTER TABLE talkgroup_sites DROP COLUMN IF EXISTS talkgroup_id;
ALTER TABLE talkgroup_sites ALTER COLUMN sysid SET NOT NULL;
ALTER TABLE talkgroup_sites ALTER COLUMN tgid SET NOT NULL;
ALTER TABLE talkgroup_sites ADD PRIMARY KEY (sysid, tgid, system_id);

-- ============================================================================
-- STEP 7: Update unit_sites primary key
-- ============================================================================

ALTER TABLE unit_sites DROP CONSTRAINT IF EXISTS unit_sites_pkey;
ALTER TABLE unit_sites DROP COLUMN IF EXISTS unit_id;
ALTER TABLE unit_sites ALTER COLUMN sysid SET NOT NULL;
ALTER TABLE unit_sites ALTER COLUMN rid SET NOT NULL;
ALTER TABLE unit_sites ADD PRIMARY KEY (sysid, rid, system_id);

-- ============================================================================
-- STEP 8: Change talkgroups primary key to (sysid, tgid)
-- ============================================================================

-- Only if id column exists (hasn't been migrated yet)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'talkgroups' AND column_name = 'id') THEN
        ALTER TABLE talkgroups DROP CONSTRAINT talkgroups_pkey;
        ALTER TABLE talkgroups DROP CONSTRAINT IF EXISTS talkgroups_sysid_tgid_key;
        ALTER TABLE talkgroups DROP COLUMN id;
        ALTER TABLE talkgroups ADD PRIMARY KEY (sysid, tgid);
    END IF;
END $$;

-- ============================================================================
-- STEP 9: Change units primary key to (sysid, unit_id)
-- ============================================================================

-- Only if id column exists (hasn't been migrated yet)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'units' AND column_name = 'id') THEN
        ALTER TABLE units DROP CONSTRAINT units_pkey;
        ALTER TABLE units DROP CONSTRAINT IF EXISTS units_sysid_unit_id_key;
        ALTER TABLE units DROP COLUMN id;
        ALTER TABLE units ADD PRIMARY KEY (sysid, unit_id);
    END IF;
END $$;

-- ============================================================================
-- STEP 10: Create indexes for the new schema
-- ============================================================================

-- Calls: index on talkgroup lookup
CREATE INDEX IF NOT EXISTS idx_calls_talkgroup ON calls(tg_sysid, tgid, start_time DESC) WHERE tg_sysid IS NOT NULL;

-- Transmissions: index on unit lookup
CREATE INDEX IF NOT EXISTS idx_transmissions_unit ON transmissions(unit_sysid, unit_rid, start_time DESC) WHERE unit_sysid IS NOT NULL;

-- Unit events: indexes on talkgroup and unit lookups
CREATE INDEX IF NOT EXISTS idx_unit_events_talkgroup ON unit_events(tg_sysid, tgid, time DESC) WHERE tg_sysid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_unit_events_unit ON unit_events(unit_sysid, unit_rid, time DESC) WHERE unit_sysid IS NOT NULL;

-- Talkgroups: last_seen index
CREATE INDEX IF NOT EXISTS idx_talkgroups_last_seen ON talkgroups(last_seen DESC);

-- Units: last_seen index
CREATE INDEX IF NOT EXISTS idx_units_last_seen ON units(last_seen DESC);

COMMIT;
