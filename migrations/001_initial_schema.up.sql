-- Enable TimescaleDB extension
CREATE EXTENSION IF NOT EXISTS timescaledb;

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

-- Talkgroups (loaded from talkgroup files or discovered)
CREATE TABLE talkgroups (
    id              SERIAL PRIMARY KEY,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    tgid            INTEGER NOT NULL,
    alpha_tag       VARCHAR(255),
    description     TEXT,
    tg_group        VARCHAR(255),
    tag             VARCHAR(64),
    priority        INTEGER,
    mode            VARCHAR(16),
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(system_id, tgid)
);

CREATE INDEX idx_talkgroups_last_seen ON talkgroups(last_seen DESC);

-- Radio units
CREATE TABLE units (
    id              SERIAL PRIMARY KEY,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    unit_id         BIGINT NOT NULL,
    alpha_tag       VARCHAR(255),
    alpha_tag_source VARCHAR(32),
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(system_id, unit_id)
);

CREATE INDEX idx_units_last_seen ON units(last_seen DESC);

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
    id              BIGSERIAL PRIMARY KEY,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    talkgroup_id    INTEGER REFERENCES talkgroups(id) ON DELETE SET NULL,
    tgid            INTEGER NOT NULL,
    start_time      TIMESTAMPTZ NOT NULL,
    end_time        TIMESTAMPTZ,
    primary_call_id BIGINT,
    call_count      INTEGER DEFAULT 1,
    encrypted       BOOLEAN DEFAULT FALSE,
    emergency       BOOLEAN DEFAULT FALSE
);

SELECT create_hypertable('call_groups', 'start_time');

CREATE INDEX idx_call_groups_talkgroup ON call_groups(talkgroup_id, start_time DESC);
CREATE INDEX idx_call_groups_system ON call_groups(system_id, start_time DESC);

-- Individual call recordings (one per site/instance)
CREATE TABLE calls (
    id              BIGSERIAL,
    call_group_id   BIGINT,
    instance_id     INTEGER REFERENCES instances(id) ON DELETE CASCADE,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    talkgroup_id    INTEGER REFERENCES talkgroups(id) ON DELETE SET NULL,
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

    PRIMARY KEY (id, start_time)
);

SELECT create_hypertable('calls', 'start_time');

CREATE INDEX idx_calls_talkgroup ON calls(talkgroup_id, start_time DESC);
CREATE INDEX idx_calls_instance ON calls(instance_id, start_time DESC);
CREATE INDEX idx_calls_group ON calls(call_group_id);
CREATE INDEX idx_calls_tr_call_id ON calls(tr_call_id);

-- Transmissions within calls (for unit tracking)
CREATE TABLE transmissions (
    id              BIGSERIAL,
    call_id         BIGINT NOT NULL,
    unit_id         INTEGER REFERENCES units(id) ON DELETE SET NULL,
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

SELECT create_hypertable('transmissions', 'start_time');

CREATE INDEX idx_transmissions_unit ON transmissions(unit_id, start_time DESC);
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

SELECT create_hypertable('call_frequencies', 'time');

-- Unit events (registration, affiliation, etc.)
CREATE TABLE unit_events (
    id              BIGSERIAL,
    instance_id     INTEGER REFERENCES instances(id) ON DELETE CASCADE,
    system_id       INTEGER REFERENCES systems(id) ON DELETE CASCADE,
    unit_id         INTEGER REFERENCES units(id) ON DELETE SET NULL,
    unit_rid        BIGINT NOT NULL,
    event_type      VARCHAR(16) NOT NULL,
    talkgroup_id    INTEGER REFERENCES talkgroups(id) ON DELETE SET NULL,
    tgid            INTEGER,
    time            TIMESTAMPTZ NOT NULL,
    metadata_json   JSONB,

    PRIMARY KEY (id, time)
);

SELECT create_hypertable('unit_events', 'time');

CREATE INDEX idx_unit_events_unit ON unit_events(unit_id, time DESC);
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

SELECT create_hypertable('system_rates', 'time');

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

SELECT create_hypertable('recorder_status', 'time');

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

SELECT create_hypertable('trunk_messages', 'time');
