package testutil

// TestSchema is a TimescaleDB-free version of the schema for unit testing
// This mirrors the production schema but without hypertable commands
const TestSchema = `
-- Trunk-recorder instances connecting to this engine
CREATE TABLE IF NOT EXISTS instances (
    id              SERIAL PRIMARY KEY,
    instance_id     VARCHAR(255) UNIQUE NOT NULL,
    instance_key    VARCHAR(255),
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    config_json     JSONB
);

-- SDR sources from each instance
CREATE TABLE IF NOT EXISTS sources (
    id              SERIAL PRIMARY KEY,
    instance_id     INTEGER REFERENCES instances(id) ON DELETE CASCADE,
    source_num      INTEGER NOT NULL,
    center_freq     BIGINT,
    rate            INTEGER,
    driver          VARCHAR(64),
    device          VARCHAR(255),
    antenna         VARCHAR(64),
    gain            INTEGER,
    config_json     JSONB,
    UNIQUE(instance_id, source_num)
);

-- Radio systems (trunked or conventional)
CREATE TABLE IF NOT EXISTS systems (
    id              SERIAL PRIMARY KEY,
    instance_id     INTEGER REFERENCES instances(id) ON DELETE CASCADE,
    sys_num         INTEGER NOT NULL,
    short_name      VARCHAR(64) NOT NULL,
    system_type     VARCHAR(32),
    sysid           VARCHAR(16),
    wacn            VARCHAR(16),
    nac             VARCHAR(16),
    rfss            INTEGER,
    site_id         INTEGER,
    config_json     JSONB,
    UNIQUE(instance_id, sys_num)
);

CREATE INDEX IF NOT EXISTS idx_systems_short_name ON systems(short_name);

-- Talkgroups (loaded from talkgroup files or discovered)
-- Unique within SYSID, keyed by natural composite (sysid, tgid)
CREATE TABLE IF NOT EXISTS talkgroups (
    sysid           VARCHAR(16) NOT NULL,
    tgid            INTEGER NOT NULL,
    alpha_tag       VARCHAR(255),
    description     TEXT,
    tg_group        VARCHAR(255),
    tag             VARCHAR(64),
    priority        INTEGER,
    mode            VARCHAR(16),
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (sysid, tgid)
);

CREATE INDEX IF NOT EXISTS idx_talkgroups_last_seen ON talkgroups(last_seen DESC);

-- Junction table: which sites have seen each talkgroup
CREATE TABLE IF NOT EXISTS talkgroup_sites (
    sysid           VARCHAR(16) NOT NULL,
    tgid            INTEGER NOT NULL,
    system_id       INTEGER NOT NULL REFERENCES systems(id) ON DELETE CASCADE,
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (sysid, tgid, system_id)
);

-- Radio units
-- Unique within SYSID, keyed by natural composite (sysid, unit_id)
CREATE TABLE IF NOT EXISTS units (
    sysid           VARCHAR(16) NOT NULL,
    unit_id         BIGINT NOT NULL,
    alpha_tag       VARCHAR(255),
    alpha_tag_source VARCHAR(32),
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (sysid, unit_id)
);

CREATE INDEX IF NOT EXISTS idx_units_last_seen ON units(last_seen DESC);

-- Junction table: which sites have seen each unit
CREATE TABLE IF NOT EXISTS unit_sites (
    sysid           VARCHAR(16) NOT NULL,
    rid             BIGINT NOT NULL,
    system_id       INTEGER NOT NULL REFERENCES systems(id) ON DELETE CASCADE,
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (sysid, rid, system_id)
);

-- Recorders from each instance
CREATE TABLE IF NOT EXISTS recorders (
    id              SERIAL PRIMARY KEY,
    instance_id     INTEGER REFERENCES instances(id) ON DELETE CASCADE,
    source_id       INTEGER REFERENCES sources(id) ON DELETE SET NULL,
    rec_num         INTEGER NOT NULL,
    rec_type        VARCHAR(16),
    UNIQUE(instance_id, source_id, rec_num)
);

-- Call groups (deduplicated logical calls)
-- Note: In production this is a TimescaleDB hypertable
CREATE TABLE IF NOT EXISTS call_groups (
    id              BIGSERIAL PRIMARY KEY,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    tg_sysid        VARCHAR(16),
    tgid            INTEGER NOT NULL,
    start_time      TIMESTAMPTZ NOT NULL,
    end_time        TIMESTAMPTZ,
    primary_call_id BIGINT,
    call_count      INTEGER DEFAULT 1,
    encrypted       BOOLEAN DEFAULT FALSE,
    emergency       BOOLEAN DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_call_groups_talkgroup ON call_groups(tg_sysid, tgid, start_time DESC);
CREATE INDEX IF NOT EXISTS idx_call_groups_system ON call_groups(system_id, start_time DESC);

-- Individual call recordings (one per site/instance)
-- Note: In production this is a TimescaleDB hypertable
CREATE TABLE IF NOT EXISTS calls (
    id              BIGSERIAL PRIMARY KEY,
    call_group_id   BIGINT,
    instance_id     INTEGER REFERENCES instances(id) ON DELETE CASCADE,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    tg_sysid        VARCHAR(16),
    tgid            INTEGER,
    recorder_id     INTEGER REFERENCES recorders(id) ON DELETE SET NULL,

    -- Call identifiers
    tr_call_id      VARCHAR(64),
    call_num        BIGINT,

    -- Timing
    start_time      TIMESTAMPTZ NOT NULL,
    stop_time       TIMESTAMPTZ,
    duration        REAL,

    -- Status
    call_state      SMALLINT,
    mon_state       SMALLINT,
    encrypted       BOOLEAN DEFAULT FALSE,
    emergency       BOOLEAN DEFAULT FALSE,
    phase2_tdma     BOOLEAN DEFAULT FALSE,
    tdma_slot       SMALLINT,
    conventional    BOOLEAN DEFAULT FALSE,
    analog          BOOLEAN DEFAULT FALSE,
    audio_type      VARCHAR(16),

    -- Quality metrics
    freq            BIGINT,
    freq_error      INTEGER,
    error_count     INTEGER,
    spike_count     INTEGER,
    signal_db       REAL,
    noise_db        REAL,

    -- Audio file reference
    audio_path      VARCHAR(512),
    audio_size      INTEGER,

    -- Patches
    patched_tgids   INTEGER[],

    -- Full JSON for any fields we don't explicitly model
    metadata_json   JSONB
);

CREATE INDEX IF NOT EXISTS idx_calls_talkgroup ON calls(tg_sysid, tgid, start_time DESC) WHERE tg_sysid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_calls_instance ON calls(instance_id, start_time DESC);
CREATE INDEX IF NOT EXISTS idx_calls_group ON calls(call_group_id);
CREATE INDEX IF NOT EXISTS idx_calls_tr_call_id ON calls(tr_call_id);

-- Transmissions within calls (for unit tracking)
-- Note: In production this is a TimescaleDB hypertable
CREATE TABLE IF NOT EXISTS transmissions (
    id              BIGSERIAL PRIMARY KEY,
    call_id         BIGINT NOT NULL,
    unit_sysid      VARCHAR(16),
    unit_rid        BIGINT NOT NULL,
    start_time      TIMESTAMPTZ NOT NULL,
    stop_time       TIMESTAMPTZ,
    duration        REAL,
    position        REAL,
    emergency       BOOLEAN DEFAULT FALSE,
    error_count     INTEGER,
    spike_count     INTEGER
);

CREATE INDEX IF NOT EXISTS idx_transmissions_unit ON transmissions(unit_sysid, unit_rid, start_time DESC) WHERE unit_sysid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_transmissions_call ON transmissions(call_id);

-- Frequency usage within calls
-- Note: In production this is a TimescaleDB hypertable
CREATE TABLE IF NOT EXISTS call_frequencies (
    id              BIGSERIAL PRIMARY KEY,
    call_id         BIGINT NOT NULL,
    freq            BIGINT NOT NULL,
    time            TIMESTAMPTZ NOT NULL,
    position        REAL,
    duration        REAL,
    error_count     INTEGER,
    spike_count     INTEGER
);

-- Unit events (registration, affiliation, etc.)
-- Note: In production this is a TimescaleDB hypertable
CREATE TABLE IF NOT EXISTS unit_events (
    id              BIGSERIAL PRIMARY KEY,
    instance_id     INTEGER REFERENCES instances(id) ON DELETE CASCADE,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    unit_sysid      VARCHAR(16),
    unit_rid        BIGINT NOT NULL,
    event_type      VARCHAR(16) NOT NULL,
    tg_sysid        VARCHAR(16),
    tgid            INTEGER,
    time            TIMESTAMPTZ NOT NULL,
    metadata_json   JSONB
);

CREATE INDEX IF NOT EXISTS idx_unit_events_unit ON unit_events(unit_sysid, unit_rid, time DESC) WHERE unit_sysid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_unit_events_talkgroup ON unit_events(tg_sysid, tgid, time DESC) WHERE tg_sysid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_unit_events_system ON unit_events(system_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_unit_events_type ON unit_events(event_type, time DESC);

-- System decode rates
-- Note: In production this is a TimescaleDB hypertable
CREATE TABLE IF NOT EXISTS system_rates (
    id              BIGSERIAL PRIMARY KEY,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    time            TIMESTAMPTZ NOT NULL,
    decode_rate     REAL,
    control_channel BIGINT
);

-- Recorder status snapshots
-- Note: In production this is a TimescaleDB hypertable
CREATE TABLE IF NOT EXISTS recorder_status (
    id              BIGSERIAL PRIMARY KEY,
    recorder_id     INTEGER REFERENCES recorders(id) ON DELETE CASCADE,
    time            TIMESTAMPTZ NOT NULL,
    state           SMALLINT,
    freq            BIGINT,
    call_count      INTEGER,
    duration        REAL,
    squelched       BOOLEAN
);

-- Trunking messages (optional, high volume)
-- Note: In production this is a TimescaleDB hypertable
CREATE TABLE IF NOT EXISTS trunk_messages (
    id              BIGSERIAL PRIMARY KEY,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    time            TIMESTAMPTZ NOT NULL,
    msg_type        SMALLINT,
    msg_type_name   VARCHAR(32),
    opcode          VARCHAR(4),
    opcode_type     VARCHAR(32),
    opcode_desc     VARCHAR(128),
    meta            TEXT
);

-- Transcriptions table (for speech-to-text)
CREATE TABLE IF NOT EXISTS transcriptions (
    id              BIGSERIAL PRIMARY KEY,
    call_id         BIGINT NOT NULL UNIQUE,
    provider        VARCHAR(32) NOT NULL,
    model           VARCHAR(64),
    language        VARCHAR(8),
    text            TEXT NOT NULL,
    confidence      REAL,
    word_count      INTEGER,
    duration_ms     INTEGER,
    words_json      JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_transcriptions_call ON transcriptions(call_id);

-- Transcription queue table
CREATE TABLE IF NOT EXISTS transcription_queue (
    id              BIGSERIAL PRIMARY KEY,
    call_id         BIGINT NOT NULL UNIQUE,
    status          VARCHAR(16) NOT NULL DEFAULT 'pending',
    priority        INTEGER NOT NULL DEFAULT 0,
    attempts        INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_transcription_queue_status ON transcription_queue(status, priority DESC, created_at);

-- Migration tracking table (for compatibility)
CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT PRIMARY KEY,
    dirty BOOLEAN NOT NULL DEFAULT FALSE
);

-- Mark as migrated to version 5 (removed surrogate IDs)
INSERT INTO schema_migrations (version, dirty) VALUES (5, FALSE) ON CONFLICT DO NOTHING;
`
