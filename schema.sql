-- ============================================================
-- tr-engine: PostgreSQL 17+ Database Schema
-- ============================================================
--
-- Complete DDL for trunk-recorder MQTT ingestion and REST API.
-- Designed for ~1-2M MQTT messages/day with monthly partitioning
-- on high-volume tables.
--
-- Run: psql -f schema.sql
-- ============================================================

BEGIN;

-- ============================================================
-- Extensions
-- ============================================================

CREATE EXTENSION IF NOT EXISTS pg_trgm;   -- trigram indexes for ILIKE search

-- ============================================================
-- Generic Trigger: set_updated_at()
-- ============================================================

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ============================================================
-- 1. instances
-- ============================================================

CREATE TABLE instances (
    id            serial       PRIMARY KEY,
    instance_id   text         UNIQUE NOT NULL,
    instance_key  text,
    first_seen    timestamptz,
    last_seen     timestamptz,
    created_at    timestamptz  NOT NULL DEFAULT now(),
    updated_at    timestamptz  NOT NULL DEFAULT now()
);

CREATE TRIGGER trg_instances_updated_at
    BEFORE UPDATE ON instances
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ============================================================
-- 2. systems
-- ============================================================

CREATE TABLE systems (
    system_id    serial       PRIMARY KEY,
    system_type  text         NOT NULL CHECK (system_type IN ('p25', 'smartnet', 'conventional')),
    name         text,
    sysid        text         NOT NULL DEFAULT '0',
    wacn         text         NOT NULL DEFAULT '0',
    deleted_at   timestamptz,
    created_at   timestamptz  NOT NULL DEFAULT now(),
    updated_at   timestamptz  NOT NULL DEFAULT now()
);

-- P25/smartnet identity uniqueness (only for active, non-conventional systems)
CREATE UNIQUE INDEX uq_systems_sysid_wacn
    ON systems (sysid, wacn)
    WHERE system_type IN ('p25', 'smartnet')
      AND deleted_at IS NULL
      AND sysid <> '0';

CREATE TRIGGER trg_systems_updated_at
    BEFORE UPDATE ON systems
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ============================================================
-- 3. sites
-- ============================================================

CREATE TABLE sites (
    site_id          serial       PRIMARY KEY,
    system_id        int          NOT NULL REFERENCES systems (system_id),
    instance_id      text         NOT NULL REFERENCES instances (instance_id),
    short_name       text         NOT NULL,
    sys_num          smallint,
    nac              text,
    rfss             smallint,
    p25_site_id      smallint,
    system_type_raw  text,
    first_seen       timestamptz,
    last_seen        timestamptz,
    created_at       timestamptz  NOT NULL DEFAULT now(),
    updated_at       timestamptz  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_sites_instance_short_name
    ON sites (instance_id, short_name);

CREATE UNIQUE INDEX uq_sites_system_instance_short_name
    ON sites (system_id, instance_id, short_name);

CREATE TRIGGER trg_sites_updated_at
    BEFORE UPDATE ON sites
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ============================================================
-- 4. talkgroups
-- ============================================================

CREATE TABLE talkgroups (
    system_id     int          NOT NULL REFERENCES systems (system_id),
    tgid          int          NOT NULL,
    alpha_tag     text,
    tag           text,
    "group"       text,
    description   text,
    mode          text         CHECK (mode IN ('D', 'A', 'E', 'M', 'T')),
    priority      int,
    first_seen    timestamptz,
    last_seen     timestamptz,
    search_vector tsvector,
    created_at    timestamptz  NOT NULL DEFAULT now(),
    updated_at    timestamptz  NOT NULL DEFAULT now(),

    PRIMARY KEY (system_id, tgid)
);

-- Full-text search
CREATE INDEX idx_talkgroups_search_vector ON talkgroups USING gin (search_vector);

-- Trigram indexes for ILIKE pattern matching
CREATE INDEX idx_talkgroups_alpha_tag_trgm ON talkgroups USING gin (alpha_tag gin_trgm_ops);
CREATE INDEX idx_talkgroups_description_trgm ON talkgroups USING gin (description gin_trgm_ops);

-- Trigger: auto-update search_vector with weighted fields
CREATE OR REPLACE FUNCTION talkgroups_search_vector_update()
RETURNS trigger AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', coalesce(NEW.alpha_tag, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(NEW.description, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(NEW."group", '')), 'C') ||
        setweight(to_tsvector('english', coalesce(NEW.tag, '')), 'C');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_talkgroups_search_vector
    BEFORE INSERT OR UPDATE OF alpha_tag, description, "group", tag
    ON talkgroups
    FOR EACH ROW EXECUTE FUNCTION talkgroups_search_vector_update();

CREATE TRIGGER trg_talkgroups_updated_at
    BEFORE UPDATE ON talkgroups
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ============================================================
-- 5. units
-- ============================================================

CREATE TABLE units (
    system_id         int          NOT NULL REFERENCES systems (system_id),
    unit_id           int          NOT NULL,
    alpha_tag         text,
    alpha_tag_source  text,
    first_seen        timestamptz,
    last_seen         timestamptz,
    last_event_type   text,
    last_event_time   timestamptz,
    last_event_tgid   int,
    created_at        timestamptz  NOT NULL DEFAULT now(),
    updated_at        timestamptz  NOT NULL DEFAULT now(),

    PRIMARY KEY (system_id, unit_id)
);

-- Trigram index for ILIKE search on alpha_tag
CREATE INDEX idx_units_alpha_tag_trgm ON units USING gin (alpha_tag gin_trgm_ops);

CREATE TRIGGER trg_units_updated_at
    BEFORE UPDATE ON units
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ============================================================
-- 6. call_groups
-- ============================================================

CREATE TABLE call_groups (
    id                     serial       PRIMARY KEY,
    system_id              int          NOT NULL REFERENCES systems (system_id),
    tgid                   int          NOT NULL,
    start_time             timestamptz  NOT NULL,
    primary_call_id        bigint,      -- FK added after calls table exists
    tg_alpha_tag           text,
    tg_description         text,
    tg_tag                 text,
    tg_group               text,
    transcription_text     text,
    transcription_status   text         CHECK (transcription_status IN ('none', 'auto', 'reviewed', 'verified', 'excluded')),
    created_at             timestamptz  NOT NULL DEFAULT now(),
    updated_at             timestamptz  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_call_groups_system_tgid_start
    ON call_groups (system_id, tgid, start_time);

CREATE TRIGGER trg_call_groups_updated_at
    BEFORE UPDATE ON call_groups
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ============================================================
-- 7. calls (PARTITIONED BY RANGE on start_time, monthly)
-- ============================================================

CREATE TABLE calls (
    call_id               bigserial,
    call_group_id         int          REFERENCES call_groups (id),
    system_id             int          NOT NULL REFERENCES systems (system_id),
    site_id               int          REFERENCES sites (site_id),
    tgid                  int          NOT NULL,
    tr_call_id            text,
    call_num              int,
    start_time            timestamptz  NOT NULL,
    stop_time             timestamptz,
    duration              real,
    freq                  bigint,
    freq_error            int,
    signal_db             real,
    noise_db              real,
    error_count           int,
    spike_count           int,
    audio_type            text,
    audio_file_path       text,
    audio_file_size       int,
    call_filename         text,
    phase2_tdma           boolean,
    tdma_slot             smallint,
    analog                boolean,
    conventional          boolean,
    encrypted             boolean,
    emergency             boolean,
    call_state            smallint,
    call_state_type       text,
    mon_state             smallint,
    mon_state_type        text,
    rec_state             smallint,
    rec_state_type        text,
    rec_num               smallint,
    src_num               smallint,
    process_call_time     real,
    retry_attempt         smallint,
    patched_tgids         int[],
    system_name           text,
    site_short_name       text,
    tg_alpha_tag          text,
    tg_description        text,
    tg_tag                text,
    tg_group              text,
    has_transcription     boolean      NOT NULL DEFAULT false,
    transcription_status  text         NOT NULL DEFAULT 'none'
                                       CHECK (transcription_status IN ('none', 'auto', 'reviewed', 'verified', 'excluded')),
    transcription_text    text,
    transcription_word_count int,
    metadata_json         jsonb,
    instance_id           text,
    created_at            timestamptz  NOT NULL DEFAULT now(),
    updated_at            timestamptz  NOT NULL DEFAULT now(),

    PRIMARY KEY (call_id, start_time)
) PARTITION BY RANGE (start_time);

-- Indexes (inherited by all partitions)
CREATE INDEX idx_calls_system_start     ON calls (system_id, start_time DESC);
CREATE INDEX idx_calls_site_start       ON calls (site_id, start_time DESC);
CREATE INDEX idx_calls_system_tgid_start ON calls (system_id, tgid, start_time DESC);
CREATE INDEX idx_calls_call_group       ON calls (call_group_id);
CREATE INDEX idx_calls_start_time       ON calls (start_time DESC);
CREATE INDEX idx_calls_emergency        ON calls (start_time DESC) WHERE emergency;
CREATE INDEX idx_calls_encrypted        ON calls (start_time DESC) WHERE encrypted;
CREATE INDEX idx_calls_has_transcription ON calls (start_time DESC) WHERE has_transcription;
CREATE INDEX idx_calls_transcription_status ON calls (transcription_status, start_time DESC)
    WHERE transcription_status <> 'none';
CREATE INDEX idx_calls_freq             ON calls (freq);
CREATE INDEX idx_calls_duration         ON calls (duration);
CREATE INDEX idx_calls_instance         ON calls (instance_id);

CREATE TRIGGER trg_calls_updated_at
    BEFORE UPDATE ON calls
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Deferred FK: call_groups.primary_call_id → calls
-- Uses (call_id) alone since primary_call_id doesn't carry start_time.
-- The application must ensure referential integrity for this field.
-- PG 17 allows FK references to partitioned tables by the full PK,
-- but primary_call_id is a convenience pointer updated after insert,
-- so we leave it as a soft reference.
COMMENT ON COLUMN call_groups.primary_call_id IS
    'Best-quality call_id for this group. Soft FK — not enforced by constraint since it references a partitioned table without the partition key.';

-- ============================================================
-- 8. call_frequencies (PARTITIONED BY RANGE on call_start_time, monthly)
-- ============================================================

CREATE TABLE call_frequencies (
    id               bigserial,
    call_id          bigint       NOT NULL,
    call_start_time  timestamptz  NOT NULL,
    freq             bigint       NOT NULL,
    "time"           timestamptz,
    pos              real,
    len              real,
    error_count      int,
    spike_count      int,

    PRIMARY KEY (id, call_start_time),
    FOREIGN KEY (call_id, call_start_time) REFERENCES calls (call_id, start_time)
) PARTITION BY RANGE (call_start_time);

CREATE INDEX idx_call_frequencies_call ON call_frequencies (call_id, call_start_time);

-- ============================================================
-- 9. call_transmissions (PARTITIONED BY RANGE on call_start_time, monthly)
-- ============================================================

CREATE TABLE call_transmissions (
    id               bigserial,
    call_id          bigint       NOT NULL,
    call_start_time  timestamptz  NOT NULL,
    src              int          NOT NULL,
    "time"           timestamptz,
    pos              real,
    emergency        smallint     DEFAULT 0,
    signal_system    text,
    tag              text,

    PRIMARY KEY (id, call_start_time),
    FOREIGN KEY (call_id, call_start_time) REFERENCES calls (call_id, start_time)
) PARTITION BY RANGE (call_start_time);

CREATE INDEX idx_call_transmissions_call ON call_transmissions (call_id, call_start_time);
CREATE INDEX idx_call_transmissions_src  ON call_transmissions (src, call_start_time DESC);

-- ============================================================
-- 10. unit_events (PARTITIONED BY RANGE on time, monthly)
-- ============================================================

CREATE TABLE unit_events (
    id                       bigserial,
    event_type               text         NOT NULL,
    system_id                int          NOT NULL REFERENCES systems (system_id),
    unit_rid                 int          NOT NULL,
    "time"                   timestamptz  NOT NULL,
    tgid                     int,
    unit_alpha_tag           text,
    tg_alpha_tag             text,
    call_num                 int,
    freq                     bigint,
    start_time               timestamptz,
    stop_time                timestamptz,
    encrypted                boolean,
    emergency                boolean,
    "position"               real,
    length                   real,
    error_count              int,
    spike_count              int,
    sample_count             int,
    transmission_filename    text,
    talkgroup_patches        int[],
    instance_id              text,
    sys_num                  smallint,
    sys_name                 text,
    metadata_json            jsonb,

    PRIMARY KEY (id, "time")
) PARTITION BY RANGE ("time");

CREATE INDEX idx_unit_events_system_unit_time ON unit_events (system_id, unit_rid, "time" DESC);
CREATE INDEX idx_unit_events_system_tgid_time ON unit_events (system_id, tgid, "time" DESC)
    WHERE tgid IS NOT NULL;
CREATE INDEX idx_unit_events_type_time        ON unit_events (event_type, "time" DESC);
CREATE INDEX idx_unit_events_unit_time        ON unit_events (unit_rid, "time" DESC);

-- ============================================================
-- 11. transcriptions (NOT partitioned — lower volume)
-- ============================================================

CREATE TABLE transcriptions (
    id              serial       PRIMARY KEY,
    call_id         bigint       NOT NULL,
    call_start_time timestamptz  NOT NULL,
    text            text,
    source          text         NOT NULL CHECK (source IN ('auto', 'human', 'llm')),
    is_primary      boolean      NOT NULL DEFAULT false,
    confidence      real,
    language        text,
    model           text,
    provider        text,
    word_count      int,
    duration_ms     int,
    words           jsonb,
    search_vector   tsvector,
    created_at      timestamptz  NOT NULL DEFAULT now(),

    FOREIGN KEY (call_id, call_start_time) REFERENCES calls (call_id, start_time)
);

CREATE INDEX idx_transcriptions_call ON transcriptions (call_id, call_start_time);
CREATE INDEX idx_transcriptions_search_vector ON transcriptions USING gin (search_vector);
CREATE INDEX idx_transcriptions_primary ON transcriptions (call_id) WHERE is_primary;

-- Trigger: auto-update search_vector from text
CREATE OR REPLACE FUNCTION transcriptions_search_vector_update()
RETURNS trigger AS $$
BEGIN
    NEW.search_vector := to_tsvector('english', coalesce(NEW.text, ''));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_transcriptions_search_vector
    BEFORE INSERT OR UPDATE OF text
    ON transcriptions
    FOR EACH ROW EXECUTE FUNCTION transcriptions_search_vector_update();

-- ============================================================
-- 12. decode_rates (state table with decimation)
-- ============================================================

CREATE TABLE decode_rates (
    id                    bigserial    PRIMARY KEY,
    system_id             int          REFERENCES systems (system_id),
    decode_rate           real,
    decode_rate_interval  real,
    control_channel       bigint,
    sys_num               smallint,
    sys_name              text,
    "time"                timestamptz  NOT NULL,
    instance_id           text
);

CREATE INDEX idx_decode_rates_system_time ON decode_rates (system_id, "time" DESC);

-- ============================================================
-- 13. recorder_snapshots (state table with decimation)
-- ============================================================

CREATE TABLE recorder_snapshots (
    id              bigserial    PRIMARY KEY,
    instance_id     text,
    recorder_id     text,
    src_num         smallint,
    rec_num         smallint,
    type            text,
    rec_state       smallint,
    rec_state_type  text,
    freq            bigint,
    duration        real,
    count           int,
    squelched       boolean,
    "time"          timestamptz  NOT NULL
);

CREATE INDEX idx_recorder_snapshots_instance_time ON recorder_snapshots (instance_id, "time" DESC);
CREATE INDEX idx_recorder_snapshots_recorder_time ON recorder_snapshots (recorder_id, "time" DESC);

-- ============================================================
-- 14. trunking_messages (PARTITIONED BY RANGE on time, monthly)
-- ============================================================

CREATE TABLE trunking_messages (
    id            bigserial,
    system_id     int          REFERENCES systems (system_id),
    sys_num       smallint,
    sys_name      text,
    trunk_msg     int,
    trunk_msg_type text,
    opcode        text,
    opcode_type   text,
    opcode_desc   text,
    meta          jsonb,
    "time"        timestamptz  NOT NULL,
    instance_id   text,

    PRIMARY KEY (id, "time")
) PARTITION BY RANGE ("time");

CREATE INDEX idx_trunking_messages_system_time ON trunking_messages (system_id, "time" DESC);
CREATE INDEX idx_trunking_messages_opcode_time ON trunking_messages (opcode, "time" DESC);

-- ============================================================
-- 15. console_messages (30-day rolling retention)
-- ============================================================

CREATE TABLE console_messages (
    id              bigserial    PRIMARY KEY,
    instance_id     text,
    log_time        timestamptz,
    severity        text,
    log_msg         text,
    mqtt_timestamp  timestamptz,
    created_at      timestamptz  NOT NULL DEFAULT now()
);

CREATE INDEX idx_console_messages_instance_time ON console_messages (instance_id, log_time DESC);
CREATE INDEX idx_console_messages_severity_time ON console_messages (severity, log_time DESC);

-- ============================================================
-- 16. plugin_statuses (30-day rolling retention)
-- ============================================================

CREATE TABLE plugin_statuses (
    id          bigserial    PRIMARY KEY,
    client_id   text,
    instance_id text,
    status      text,
    "time"      timestamptz,
    created_at  timestamptz  NOT NULL DEFAULT now()
);

CREATE INDEX idx_plugin_statuses_instance_time ON plugin_statuses (instance_id, "time" DESC);

-- ============================================================
-- 17. instance_configs (permanent, low volume)
-- ============================================================

CREATE TABLE instance_configs (
    id            serial       PRIMARY KEY,
    instance_id   text         REFERENCES instances (instance_id),
    capture_dir   text,
    upload_server text,
    call_timeout  int,
    log_file      text,
    instance_key  text,
    config_json   jsonb        NOT NULL,
    "time"        timestamptz,
    created_at    timestamptz  NOT NULL DEFAULT now()
);

CREATE INDEX idx_instance_configs_instance_time ON instance_configs (instance_id, "time" DESC);

-- ============================================================
-- 18. mqtt_raw_messages (PARTITIONED BY RANGE on received_at, weekly)
-- ============================================================

CREATE TABLE mqtt_raw_messages (
    id            bigserial,
    topic         text         NOT NULL,
    payload       jsonb,
    received_at   timestamptz  NOT NULL DEFAULT now(),
    instance_id   text,
    processed     boolean      NOT NULL DEFAULT false,
    process_error text,

    PRIMARY KEY (id, received_at)
) PARTITION BY RANGE (received_at);

CREATE INDEX idx_mqtt_raw_topic_received ON mqtt_raw_messages (topic, received_at DESC);
CREATE INDEX idx_mqtt_raw_unprocessed    ON mqtt_raw_messages (received_at)
    WHERE NOT processed;

-- ============================================================
-- 19. call_active_checkpoints (24-hour retention, crash recovery)
-- ============================================================

CREATE TABLE call_active_checkpoints (
    id             serial       PRIMARY KEY,
    instance_id    text,
    snapshot_time  timestamptz  NOT NULL DEFAULT now(),
    active_calls   jsonb,
    call_count     int
);

CREATE INDEX idx_active_checkpoints_time ON call_active_checkpoints (snapshot_time DESC);

-- ============================================================
-- 20. system_merge_log (permanent audit trail)
-- ============================================================

CREATE TABLE system_merge_log (
    id                serial       PRIMARY KEY,
    source_id         int          NOT NULL,
    target_id         int          NOT NULL,
    calls_moved       int,
    talkgroups_moved  int,
    talkgroups_merged int,
    units_moved       int,
    units_merged      int,
    events_moved      int,
    performed_at      timestamptz  NOT NULL DEFAULT now(),
    performed_by      text
);

-- ============================================================
-- Helper: create_monthly_partition()
--
-- Creates a monthly partition for a given table if it doesn't
-- already exist. Naming convention: {table}_y{YYYY}m{MM}
--
-- Usage:
--   SELECT create_monthly_partition('calls', '2026-02-01');
--   SELECT create_monthly_partition('calls', '2026-03-01');
-- ============================================================

CREATE OR REPLACE FUNCTION create_monthly_partition(
    parent_table text,
    partition_start date
)
RETURNS text AS $$
DECLARE
    partition_name text;
    partition_end  date;
    start_str      text;
    end_str        text;
BEGIN
    partition_name := parent_table || '_y'
        || to_char(partition_start, 'YYYY')
        || 'm'
        || to_char(partition_start, 'MM');

    partition_end := partition_start + interval '1 month';
    start_str := to_char(partition_start, 'YYYY-MM-DD');
    end_str   := to_char(partition_end, 'YYYY-MM-DD');

    -- Check if partition already exists
    IF EXISTS (
        SELECT 1 FROM pg_class
        WHERE relname = partition_name
          AND relkind = 'r'
    ) THEN
        RETURN partition_name || ' (already exists)';
    END IF;

    EXECUTE format(
        'CREATE TABLE %I PARTITION OF %I FOR VALUES FROM (%L) TO (%L)',
        partition_name, parent_table, start_str, end_str
    );

    RETURN partition_name;
END;
$$ LANGUAGE plpgsql;

-- ============================================================
-- Helper: create_weekly_partition()
--
-- Creates a weekly partition for mqtt_raw_messages.
-- Naming convention: {table}_w{YYYY}_{IW}
--
-- Usage:
--   SELECT create_weekly_partition('mqtt_raw_messages', '2026-02-09');
-- ============================================================

CREATE OR REPLACE FUNCTION create_weekly_partition(
    parent_table text,
    partition_start date
)
RETURNS text AS $$
DECLARE
    partition_name text;
    partition_end  date;
    start_str      text;
    end_str        text;
BEGIN
    -- Align to Monday
    partition_start := partition_start - ((extract(isodow FROM partition_start)::int - 1) || ' days')::interval;
    partition_end   := partition_start + interval '7 days';

    partition_name := parent_table || '_w'
        || to_char(partition_start, 'IYYY')
        || '_'
        || to_char(partition_start, 'IW');

    start_str := to_char(partition_start, 'YYYY-MM-DD');
    end_str   := to_char(partition_end, 'YYYY-MM-DD');

    IF EXISTS (
        SELECT 1 FROM pg_class
        WHERE relname = partition_name
          AND relkind = 'r'
    ) THEN
        RETURN partition_name || ' (already exists)';
    END IF;

    EXECUTE format(
        'CREATE TABLE %I PARTITION OF %I FOR VALUES FROM (%L) TO (%L)',
        partition_name, parent_table, start_str, end_str
    );

    RETURN partition_name;
END;
$$ LANGUAGE plpgsql;

-- ============================================================
-- Helper: decimate_state_table()
--
-- Decimation policy for state tables (recorder_snapshots, decode_rates):
--   - Rows older than 24h: keep 1 per minute (delete others)
--   - Rows older than 7d:  keep 1 per hour (delete others)
--
-- Usage:
--   SELECT decimate_state_table('recorder_snapshots', 'time');
--   SELECT decimate_state_table('decode_rates', 'time');
-- ============================================================

CREATE OR REPLACE FUNCTION decimate_state_table(
    target_table text,
    time_column  text
)
RETURNS TABLE(deleted_24h bigint, deleted_7d bigint) AS $$
DECLARE
    cnt_24h bigint := 0;
    cnt_7d  bigint := 0;
BEGIN
    -- Phase 1: For rows 24h-7d old, keep 1 per minute
    EXECUTE format($q$
        WITH numbered AS (
            SELECT id,
                   row_number() OVER (
                       PARTITION BY date_trunc('minute', %I)
                       ORDER BY %I
                   ) AS rn
            FROM %I
            WHERE %I < now() - interval '24 hours'
              AND %I >= now() - interval '7 days'
        )
        DELETE FROM %I WHERE id IN (
            SELECT id FROM numbered WHERE rn > 1
        )
    $q$, time_column, time_column, target_table,
         time_column, time_column, target_table);

    GET DIAGNOSTICS cnt_24h = ROW_COUNT;

    -- Phase 2: For rows >7d old, keep 1 per hour
    EXECUTE format($q$
        WITH numbered AS (
            SELECT id,
                   row_number() OVER (
                       PARTITION BY date_trunc('hour', %I)
                       ORDER BY %I
                   ) AS rn
            FROM %I
            WHERE %I < now() - interval '7 days'
        )
        DELETE FROM %I WHERE id IN (
            SELECT id FROM numbered WHERE rn > 1
        )
    $q$, time_column, time_column, target_table,
         time_column, target_table);

    GET DIAGNOSTICS cnt_7d = ROW_COUNT;

    RETURN QUERY SELECT cnt_24h, cnt_7d;
END;
$$ LANGUAGE plpgsql;

-- ============================================================
-- Create initial partitions
--
-- Creates partitions for the current month and 3 months ahead
-- for all partitioned tables.
-- ============================================================

DO $$
DECLARE
    month_offset int;
    partition_date date;
    monthly_tables text[] := ARRAY[
        'calls',
        'call_frequencies',
        'call_transmissions',
        'unit_events',
        'trunking_messages'
    ];
    tbl text;
    week_offset int;
    week_date date;
BEGIN
    -- Monthly partitions: current month + 3 months ahead
    FOR month_offset IN 0..3 LOOP
        partition_date := date_trunc('month', current_date)::date + (month_offset || ' months')::interval;
        FOREACH tbl IN ARRAY monthly_tables LOOP
            PERFORM create_monthly_partition(tbl, partition_date);
        END LOOP;
    END LOOP;

    -- Weekly partitions for mqtt_raw_messages: current week + 3 weeks ahead
    FOR week_offset IN 0..3 LOOP
        week_date := current_date + (week_offset * 7);
        PERFORM create_weekly_partition('mqtt_raw_messages', week_date);
    END LOOP;
END $$;

-- ============================================================
-- Retention Job Comments
--
-- These should be scheduled via pg_cron or the application layer.
-- ============================================================

COMMENT ON TABLE mqtt_raw_messages IS
    'Raw MQTT archive. Retention: 7 days. Drop weekly partitions older than 7d.';

COMMENT ON TABLE call_active_checkpoints IS
    'Crash recovery snapshots. Retention: 24h. Purge: DELETE WHERE snapshot_time < now() - interval ''24 hours''.';

COMMENT ON TABLE console_messages IS
    'TR console log. Retention: 30 days. Purge: DELETE WHERE log_time < now() - interval ''30 days''.';

COMMENT ON TABLE plugin_statuses IS
    'Plugin status log. Retention: 30 days. Purge: DELETE WHERE time < now() - interval ''30 days''.';

COMMENT ON TABLE recorder_snapshots IS
    'Recorder state snapshots. Decimation: 1/min after 24h, 1/hour after 7d. Run: SELECT decimate_state_table(''recorder_snapshots'', ''time'').';

COMMENT ON TABLE decode_rates IS
    'Decode rate snapshots. Decimation: 1/min after 24h, 1/hour after 7d. Run: SELECT decimate_state_table(''decode_rates'', ''time'').';

/*
 * Recommended pg_cron schedule:
 *
 * -- Create partitions 3 months ahead (daily at 00:05)
 * SELECT cron.schedule('create-partitions', '5 0 * * *', $$
 *     DO $body$
 *     DECLARE
 *         tbl text;
 *         monthly_tables text[] := ARRAY['calls','call_frequencies','call_transmissions','unit_events','trunking_messages'];
 *     BEGIN
 *         FOREACH tbl IN ARRAY monthly_tables LOOP
 *             PERFORM create_monthly_partition(tbl, (date_trunc('month', current_date) + interval '3 months')::date);
 *         END LOOP;
 *         PERFORM create_weekly_partition('mqtt_raw_messages', current_date + 21);
 *     END $body$;
 * $$);
 *
 * -- Decimate state tables (daily at 01:00)
 * SELECT cron.schedule('decimate-state', '0 1 * * *', $$
 *     SELECT decimate_state_table('recorder_snapshots', 'time');
 *     SELECT decimate_state_table('decode_rates', 'time');
 * $$);
 *
 * -- Purge raw MQTT (daily at 02:00 — drop weekly partitions older than 7d)
 * -- Implementation depends on naming convention; see create_weekly_partition().
 *
 * -- Purge active checkpoints (hourly)
 * SELECT cron.schedule('purge-checkpoints', '0 * * * *', $$
 *     DELETE FROM call_active_checkpoints WHERE snapshot_time < now() - interval '24 hours';
 * $$);
 *
 * -- Purge console/plugin logs (daily at 03:00)
 * SELECT cron.schedule('purge-logs', '0 3 * * *', $$
 *     DELETE FROM console_messages WHERE log_time < now() - interval '30 days';
 *     DELETE FROM plugin_statuses WHERE time < now() - interval '30 days';
 * $$);
 */

COMMIT;
