-- Migration: SYSID-based scoping for talkgroups and units
--
-- This migration changes talkgroups and units from being scoped per-site (system_id)
-- to being scoped per-network (sysid). This reflects P25 reality where talkgroups
-- and units are unique within a SYSID, not within a site.
--
-- Changes:
-- 1. Add sysid column to talkgroups and units
-- 2. Create junction tables for site tracking (talkgroup_sites, unit_sites)
-- 3. Merge duplicate records that share the same (sysid, tgid/unit_id)
-- 4. Update foreign key references to merged records
-- 5. Drop system_id from talkgroups and units
-- 6. Add new unique constraint on (sysid, tgid/unit_id)

BEGIN;

-- ============================================================================
-- STEP 1: Add sysid column to talkgroups and units
-- ============================================================================

ALTER TABLE talkgroups ADD COLUMN sysid VARCHAR(16);
ALTER TABLE units ADD COLUMN sysid VARCHAR(16);

-- Populate sysid from the associated system
-- For systems without sysid, use short_name as fallback (conventional systems)
UPDATE talkgroups t
SET sysid = COALESCE(s.sysid, s.short_name)
FROM systems s
WHERE t.system_id = s.id;

UPDATE units u
SET sysid = COALESCE(s.sysid, s.short_name)
FROM systems s
WHERE u.system_id = s.id;

-- ============================================================================
-- STEP 2: Create junction tables for site tracking
-- ============================================================================

CREATE TABLE talkgroup_sites (
    talkgroup_id    INTEGER NOT NULL,
    system_id       INTEGER NOT NULL REFERENCES systems(id) ON DELETE CASCADE,
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (talkgroup_id, system_id)
);

CREATE TABLE unit_sites (
    unit_id         INTEGER NOT NULL,
    system_id       INTEGER NOT NULL REFERENCES systems(id) ON DELETE CASCADE,
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (unit_id, system_id)
);

CREATE INDEX idx_talkgroup_sites_system ON talkgroup_sites(system_id);
CREATE INDEX idx_unit_sites_system ON unit_sites(system_id);

-- Populate junction tables with current site associations
INSERT INTO talkgroup_sites (talkgroup_id, system_id, first_seen, last_seen)
SELECT id, system_id, first_seen, last_seen
FROM talkgroups
WHERE system_id IS NOT NULL;

INSERT INTO unit_sites (unit_id, system_id, first_seen, last_seen)
SELECT id, system_id, first_seen, last_seen
FROM units
WHERE system_id IS NOT NULL;

-- ============================================================================
-- STEP 3: Merge duplicate talkgroups (same sysid + tgid)
-- ============================================================================

-- Create temp table to map old IDs to canonical IDs
CREATE TEMP TABLE talkgroup_id_map AS
WITH ranked AS (
    SELECT
        id,
        sysid,
        tgid,
        ROW_NUMBER() OVER (
            PARTITION BY sysid, tgid
            ORDER BY first_seen ASC, id ASC
        ) as rn
    FROM talkgroups
),
canonical AS (
    SELECT id as canonical_id, sysid, tgid
    FROM ranked
    WHERE rn = 1
)
SELECT r.id as old_id, c.canonical_id
FROM ranked r
JOIN canonical c ON r.sysid = c.sysid AND r.tgid = c.tgid;

-- Update foreign key references to point to canonical talkgroup
UPDATE calls c
SET talkgroup_id = m.canonical_id
FROM talkgroup_id_map m
WHERE c.talkgroup_id = m.old_id
AND m.old_id != m.canonical_id;

UPDATE call_groups cg
SET talkgroup_id = m.canonical_id
FROM talkgroup_id_map m
WHERE cg.talkgroup_id = m.old_id
AND m.old_id != m.canonical_id;

UPDATE unit_events ue
SET talkgroup_id = m.canonical_id
FROM talkgroup_id_map m
WHERE ue.talkgroup_id = m.old_id
AND m.old_id != m.canonical_id;

-- Merge talkgroup_sites: add site associations from duplicates to canonical
INSERT INTO talkgroup_sites (talkgroup_id, system_id, first_seen, last_seen)
SELECT m.canonical_id, ts.system_id, ts.first_seen, ts.last_seen
FROM talkgroup_sites ts
JOIN talkgroup_id_map m ON ts.talkgroup_id = m.old_id
WHERE m.old_id != m.canonical_id
ON CONFLICT (talkgroup_id, system_id) DO UPDATE
SET first_seen = LEAST(talkgroup_sites.first_seen, EXCLUDED.first_seen),
    last_seen = GREATEST(talkgroup_sites.last_seen, EXCLUDED.last_seen);

-- Delete site associations for duplicates
DELETE FROM talkgroup_sites ts
USING talkgroup_id_map m
WHERE ts.talkgroup_id = m.old_id
AND m.old_id != m.canonical_id;

-- Merge talkgroup metadata: keep best alpha_tag, earliest first_seen, latest last_seen
UPDATE talkgroups t
SET
    alpha_tag = COALESCE(t.alpha_tag, dup.alpha_tag),
    description = COALESCE(t.description, dup.description),
    tg_group = COALESCE(t.tg_group, dup.tg_group),
    tag = COALESCE(t.tag, dup.tag),
    priority = COALESCE(t.priority, dup.priority),
    mode = COALESCE(t.mode, dup.mode),
    first_seen = LEAST(t.first_seen, dup.first_seen),
    last_seen = GREATEST(t.last_seen, dup.last_seen)
FROM talkgroups dup
JOIN talkgroup_id_map m ON dup.id = m.old_id
WHERE t.id = m.canonical_id
AND m.old_id != m.canonical_id;

-- Delete duplicate talkgroups
DELETE FROM talkgroups t
USING talkgroup_id_map m
WHERE t.id = m.old_id
AND m.old_id != m.canonical_id;

DROP TABLE talkgroup_id_map;

-- ============================================================================
-- STEP 4: Merge duplicate units (same sysid + unit_id)
-- ============================================================================

CREATE TEMP TABLE unit_id_map AS
WITH ranked AS (
    SELECT
        id,
        sysid,
        unit_id,
        ROW_NUMBER() OVER (
            PARTITION BY sysid, unit_id
            ORDER BY first_seen ASC, id ASC
        ) as rn
    FROM units
),
canonical AS (
    SELECT id as canonical_id, sysid, unit_id
    FROM ranked
    WHERE rn = 1
)
SELECT r.id as old_id, c.canonical_id
FROM ranked r
JOIN canonical c ON r.sysid = c.sysid AND r.unit_id = c.unit_id;

-- Update foreign key references to point to canonical unit
UPDATE transmissions tx
SET unit_id = m.canonical_id
FROM unit_id_map m
WHERE tx.unit_id = m.old_id
AND m.old_id != m.canonical_id;

UPDATE unit_events ue
SET unit_id = m.canonical_id
FROM unit_id_map m
WHERE ue.unit_id = m.old_id
AND m.old_id != m.canonical_id;

-- Merge unit_sites: add site associations from duplicates to canonical
INSERT INTO unit_sites (unit_id, system_id, first_seen, last_seen)
SELECT m.canonical_id, us.system_id, us.first_seen, us.last_seen
FROM unit_sites us
JOIN unit_id_map m ON us.unit_id = m.old_id
WHERE m.old_id != m.canonical_id
ON CONFLICT (unit_id, system_id) DO UPDATE
SET first_seen = LEAST(unit_sites.first_seen, EXCLUDED.first_seen),
    last_seen = GREATEST(unit_sites.last_seen, EXCLUDED.last_seen);

-- Delete site associations for duplicates
DELETE FROM unit_sites us
USING unit_id_map m
WHERE us.unit_id = m.old_id
AND m.old_id != m.canonical_id;

-- Merge unit metadata
UPDATE units u
SET
    alpha_tag = COALESCE(u.alpha_tag, dup.alpha_tag),
    alpha_tag_source = COALESCE(u.alpha_tag_source, dup.alpha_tag_source),
    first_seen = LEAST(u.first_seen, dup.first_seen),
    last_seen = GREATEST(u.last_seen, dup.last_seen)
FROM units dup
JOIN unit_id_map m ON dup.id = m.old_id
WHERE u.id = m.canonical_id
AND m.old_id != m.canonical_id;

-- Delete duplicate units
DELETE FROM units u
USING unit_id_map m
WHERE u.id = m.old_id
AND m.old_id != m.canonical_id;

DROP TABLE unit_id_map;

-- ============================================================================
-- STEP 5: Finalize schema changes
-- ============================================================================

-- Drop old constraints and columns
ALTER TABLE talkgroups DROP CONSTRAINT talkgroups_system_id_tgid_key;
ALTER TABLE talkgroups DROP COLUMN system_id;

ALTER TABLE units DROP CONSTRAINT units_system_id_unit_id_key;
ALTER TABLE units DROP COLUMN system_id;

-- Add NOT NULL constraint now that all rows have sysid
ALTER TABLE talkgroups ALTER COLUMN sysid SET NOT NULL;
ALTER TABLE units ALTER COLUMN sysid SET NOT NULL;

-- Add new unique constraints
ALTER TABLE talkgroups ADD CONSTRAINT talkgroups_sysid_tgid_key UNIQUE(sysid, tgid);
ALTER TABLE units ADD CONSTRAINT units_sysid_unit_id_key UNIQUE(sysid, unit_id);

-- Add indexes for sysid lookups
CREATE INDEX idx_talkgroups_sysid ON talkgroups(sysid);
CREATE INDEX idx_units_sysid ON units(sysid);

-- Add foreign key constraints to junction tables now that merging is complete
ALTER TABLE talkgroup_sites
    ADD CONSTRAINT talkgroup_sites_talkgroup_id_fkey
    FOREIGN KEY (talkgroup_id) REFERENCES talkgroups(id) ON DELETE CASCADE;

ALTER TABLE unit_sites
    ADD CONSTRAINT unit_sites_unit_id_fkey
    FOREIGN KEY (unit_id) REFERENCES units(id) ON DELETE CASCADE;

COMMIT;
