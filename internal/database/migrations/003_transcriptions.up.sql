-- Migration: Add transcription support
--
-- This migration adds:
-- 1. transcriptions table for storing transcription results
-- 2. transcription_queue table for job queue management
-- 3. transcription_id column on calls table

BEGIN;

-- ============================================================================
-- STEP 1: Create transcriptions table
-- ============================================================================

CREATE TABLE transcriptions (
    id              BIGSERIAL PRIMARY KEY,
    call_id         BIGINT NOT NULL,
    provider        VARCHAR(32) NOT NULL,
    model           VARCHAR(64),
    language        VARCHAR(8),
    text            TEXT NOT NULL,
    confidence      REAL,
    word_count      INTEGER,
    duration_ms     INTEGER,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for looking up transcription by call
CREATE INDEX idx_transcriptions_call ON transcriptions(call_id);

-- Full-text search index on transcription text
CREATE INDEX idx_transcriptions_text ON transcriptions USING gin(to_tsvector('english', text));

-- ============================================================================
-- STEP 2: Create transcription job queue table
-- ============================================================================

CREATE TABLE transcription_queue (
    id              BIGSERIAL PRIMARY KEY,
    call_id         BIGINT NOT NULL UNIQUE,
    status          VARCHAR(16) DEFAULT 'pending',
    priority        INTEGER DEFAULT 0,
    attempts        INTEGER DEFAULT 0,
    last_error      TEXT,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Index for efficient queue polling (pending jobs ordered by priority)
CREATE INDEX idx_transcription_queue_status ON transcription_queue(status, priority DESC, created_at);

-- ============================================================================
-- STEP 3: Add transcription_id to calls table
-- ============================================================================

ALTER TABLE calls ADD COLUMN transcription_id BIGINT;

-- Note: We don't add a foreign key constraint here because:
-- 1. The calls table is a hypertable (TimescaleDB) which has limitations on foreign keys
-- 2. The application layer handles the relationship integrity
-- 3. Transcription lookup is done via transcriptions.call_id anyway

COMMIT;
