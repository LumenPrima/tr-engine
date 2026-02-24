-- name: InsertRecorderSnapshots :copyfrom
INSERT INTO recorder_snapshots (
    instance_id, recorder_id, src_num, rec_num, type,
    rec_state, rec_state_type, freq, duration, count,
    squelched, "time"
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: InsertDecodeRates :copyfrom
INSERT INTO decode_rates (
    system_id, decode_rate, decode_rate_interval,
    control_channel, sys_num, sys_name, "time", instance_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);
