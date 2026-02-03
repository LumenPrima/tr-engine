-- tr-engine schema (consolidated, works with vanilla PostgreSQL)
-- TimescaleDB is optional - hypertables will be created if available

-- Try to enable TimescaleDB (silently skip if not available)
DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS timescaledb;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'TimescaleDB not available, using standard PostgreSQL tables';
END $$;

-- Trunk-recorder instances connecting to this engine
CREATE TABLE instances (
    id              SERIAL PRIMARY KEY,
    instance_id     VARCHAR(255) UNIQUE NOT NULL,
    instance_key    VARCHAR(255),
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    config_json     JSONB
);

-- SDR sources from each instance
CREATE TABLE sources (
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
CREATE TABLE systems (
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

CREATE INDEX idx_systems_short_name ON systems(short_name);

-- Talkgroups (keyed by sysid + tgid)
CREATE TABLE talkgroups (
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

CREATE INDEX idx_talkgroups_last_seen ON talkgroups(last_seen DESC);
CREATE INDEX idx_talkgroups_sysid ON talkgroups(sysid);

-- Radio units (keyed by sysid + unit_id)
CREATE TABLE units (
    sysid           VARCHAR(16) NOT NULL,
    unit_id         BIGINT NOT NULL,
    alpha_tag       VARCHAR(255),
    alpha_tag_source VARCHAR(32),
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (sysid, unit_id)
);

CREATE INDEX idx_units_last_seen ON units(last_seen DESC);
CREATE INDEX idx_units_sysid ON units(sysid);

-- Junction table: which sites have seen which talkgroups
CREATE TABLE talkgroup_sites (
    sysid           VARCHAR(16) NOT NULL,
    tgid            INTEGER NOT NULL,
    system_id       INTEGER NOT NULL REFERENCES systems(id) ON DELETE CASCADE,
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (sysid, tgid, system_id)
);

CREATE INDEX idx_talkgroup_sites_system ON talkgroup_sites(system_id);

-- Junction table: which sites have seen which units
CREATE TABLE unit_sites (
    sysid           VARCHAR(16) NOT NULL,
    rid             BIGINT NOT NULL,
    system_id       INTEGER NOT NULL REFERENCES systems(id) ON DELETE CASCADE,
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (sysid, rid, system_id)
);

CREATE INDEX idx_unit_sites_system ON unit_sites(system_id);

-- Recorders from each instance
CREATE TABLE recorders (
    id              SERIAL PRIMARY KEY,
    instance_id     INTEGER REFERENCES instances(id) ON DELETE CASCADE,
    source_id       INTEGER REFERENCES sources(id) ON DELETE SET NULL,
    rec_num         INTEGER NOT NULL,
    rec_type        VARCHAR(16),
    UNIQUE(instance_id, source_id, rec_num)
);

-- Call groups (deduplicated logical calls)
CREATE TABLE call_groups (
    id              BIGSERIAL,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    tg_sysid        VARCHAR(16),
    tgid            INTEGER NOT NULL,
    start_time      TIMESTAMPTZ NOT NULL,
    end_time        TIMESTAMPTZ,
    primary_call_id BIGINT,
    call_count      INTEGER DEFAULT 1,
    encrypted       BOOLEAN DEFAULT FALSE,
    emergency       BOOLEAN DEFAULT FALSE,
    PRIMARY KEY (id, start_time)
);

CREATE INDEX idx_call_groups_talkgroup ON call_groups(tg_sysid, tgid, start_time DESC);
CREATE INDEX idx_call_groups_system ON call_groups(system_id, start_time DESC);

-- Individual call recordings (one per site/instance)
CREATE TABLE calls (
    id              BIGSERIAL,
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
    metadata_json   JSONB,

    -- Transcription reference
    transcription_id BIGINT,

    PRIMARY KEY (id, start_time)
);

CREATE INDEX idx_calls_talkgroup ON calls(tg_sysid, tgid, start_time DESC) WHERE tg_sysid IS NOT NULL;
CREATE INDEX idx_calls_tg_sysid_tgid ON calls(tg_sysid, tgid, start_time DESC);
CREATE INDEX idx_calls_instance ON calls(instance_id, start_time DESC);
CREATE INDEX idx_calls_group ON calls(call_group_id);
CREATE INDEX idx_calls_tr_call_id ON calls(tr_call_id);

-- Transmissions within calls (for unit tracking)
CREATE TABLE transmissions (
    id              BIGSERIAL,
    call_id         BIGINT NOT NULL,
    unit_sysid      VARCHAR(16),
    unit_rid        BIGINT NOT NULL,
    start_time      TIMESTAMPTZ NOT NULL,
    stop_time       TIMESTAMPTZ,
    duration        REAL,
    position        REAL,
    emergency       BOOLEAN DEFAULT FALSE,
    error_count     INTEGER,
    spike_count     INTEGER,
    PRIMARY KEY (id, start_time)
);

CREATE INDEX idx_transmissions_unit ON transmissions(unit_sysid, unit_rid, start_time DESC) WHERE unit_sysid IS NOT NULL;
CREATE INDEX idx_transmissions_call ON transmissions(call_id);

-- Frequency usage within calls
CREATE TABLE call_frequencies (
    id              BIGSERIAL,
    call_id         BIGINT NOT NULL,
    freq            BIGINT NOT NULL,
    time            TIMESTAMPTZ NOT NULL,
    position        REAL,
    duration        REAL,
    error_count     INTEGER,
    spike_count     INTEGER,
    PRIMARY KEY (id, time)
);

-- Unit events (registration, affiliation, etc.)
CREATE TABLE unit_events (
    id              BIGSERIAL,
    instance_id     INTEGER REFERENCES instances(id) ON DELETE CASCADE,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    unit_sysid      VARCHAR(16),
    unit_rid        BIGINT NOT NULL,
    event_type      VARCHAR(16) NOT NULL,
    tg_sysid        VARCHAR(16),
    tgid            INTEGER,
    time            TIMESTAMPTZ NOT NULL,
    metadata_json   JSONB,
    PRIMARY KEY (id, time)
);

CREATE INDEX idx_unit_events_unit ON unit_events(unit_sysid, unit_rid, time DESC) WHERE unit_sysid IS NOT NULL;
CREATE INDEX idx_unit_events_talkgroup ON unit_events(tg_sysid, tgid, time DESC) WHERE tg_sysid IS NOT NULL;
CREATE INDEX idx_unit_events_system ON unit_events(system_id, time DESC);
CREATE INDEX idx_unit_events_type ON unit_events(event_type, time DESC);

-- System decode rates
CREATE TABLE system_rates (
    id              BIGSERIAL,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    time            TIMESTAMPTZ NOT NULL,
    decode_rate     REAL,
    control_channel BIGINT,
    PRIMARY KEY (id, time)
);

-- Recorder status snapshots
CREATE TABLE recorder_status (
    id              BIGSERIAL,
    recorder_id     INTEGER REFERENCES recorders(id) ON DELETE CASCADE,
    time            TIMESTAMPTZ NOT NULL,
    state           SMALLINT,
    freq            BIGINT,
    call_count      INTEGER,
    duration        REAL,
    squelched       BOOLEAN,
    PRIMARY KEY (id, time)
);

-- Trunking messages (optional, high volume)
CREATE TABLE trunk_messages (
    id              BIGSERIAL,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    time            TIMESTAMPTZ NOT NULL,
    msg_type        SMALLINT,
    msg_type_name   VARCHAR(32),
    opcode          VARCHAR(4),
    opcode_type     VARCHAR(32),
    opcode_desc     VARCHAR(128),
    meta            TEXT,
    PRIMARY KEY (id, time)
);

-- Transcriptions
CREATE TABLE transcriptions (
    id              BIGSERIAL PRIMARY KEY,
    call_id         BIGINT NOT NULL UNIQUE,
    provider        VARCHAR(32) NOT NULL,
    model           VARCHAR(64),
    language        VARCHAR(10),
    text            TEXT NOT NULL,
    confidence      REAL,
    word_count      INTEGER,
    duration_ms     INTEGER,
    words_json      JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_transcriptions_call ON transcriptions(call_id);
CREATE INDEX idx_transcriptions_created ON transcriptions(created_at DESC);

-- Transcription queue
CREATE TABLE transcription_queue (
    id              BIGSERIAL PRIMARY KEY,
    call_id         BIGINT NOT NULL UNIQUE,
    status          VARCHAR(16) NOT NULL DEFAULT 'pending',
    priority        INTEGER NOT NULL DEFAULT 0,
    attempts        INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_transcription_queue_status ON transcription_queue(status, priority DESC, created_at);

-- API keys
CREATE TABLE api_keys (
    id              SERIAL PRIMARY KEY,
    key_hash        VARCHAR(64) NOT NULL UNIQUE,
    key_prefix      VARCHAR(12) NOT NULL,
    name            VARCHAR(255) NOT NULL,
    scopes          TEXT[] DEFAULT '{}',
    read_only       BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at    TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ,
    revoked_at      TIMESTAMPTZ
);

CREATE INDEX idx_api_keys_hash ON api_keys(key_hash) WHERE revoked_at IS NULL;

-- Migration tracking (created by golang-migrate for external DB, manually for embedded)
CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT PRIMARY KEY,
    dirty BOOLEAN NOT NULL DEFAULT FALSE
);
INSERT INTO schema_migrations (version, dirty) VALUES (1, FALSE) ON CONFLICT DO NOTHING;

-- Try to create hypertables if TimescaleDB is available
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_proc WHERE proname = 'create_hypertable') THEN
        PERFORM create_hypertable('call_groups', 'start_time', if_not_exists => TRUE);
        PERFORM create_hypertable('calls', 'start_time', if_not_exists => TRUE);
        PERFORM create_hypertable('transmissions', 'start_time', if_not_exists => TRUE);
        PERFORM create_hypertable('call_frequencies', 'time', if_not_exists => TRUE);
        PERFORM create_hypertable('unit_events', 'time', if_not_exists => TRUE);
        PERFORM create_hypertable('system_rates', 'time', if_not_exists => TRUE);
        PERFORM create_hypertable('recorder_status', 'time', if_not_exists => TRUE);
        PERFORM create_hypertable('trunk_messages', 'time', if_not_exists => TRUE);
        RAISE NOTICE 'TimescaleDB hypertables created';
    END IF;
END $$;
