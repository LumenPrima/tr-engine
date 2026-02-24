-- name: ClearPrimaryTranscription :exec
UPDATE transcriptions SET is_primary = false
WHERE call_id = $1 AND call_start_time = $2 AND is_primary = true;

-- name: InsertTranscriptionRow :one
INSERT INTO transcriptions (
    call_id, call_start_time, text, source, is_primary,
    confidence, language, model, provider,
    word_count, duration_ms, words
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id;

-- name: UpdateCallTranscriptionDenorm :exec
UPDATE calls SET
    has_transcription = true,
    transcription_status = $3,
    transcription_text = $4,
    transcription_word_count = $5
WHERE call_id = $1 AND start_time = $2;

-- name: UpdateCallGroupTranscriptionDenorm :exec
UPDATE call_groups SET
    transcription_text = $3,
    transcription_status = $4
WHERE id = (
    SELECT c.call_group_id FROM calls c
    WHERE c.call_id = $1 AND c.start_time = $2 AND c.call_group_id IS NOT NULL
);

-- name: GetPrimaryTranscription :one
SELECT id, call_id, text, source, is_primary,
    confidence, language, model, provider,
    word_count, duration_ms, words, created_at
FROM transcriptions
WHERE call_id = $1 AND is_primary = true
ORDER BY created_at DESC
LIMIT 1;

-- name: ListTranscriptionsByCall :many
SELECT id, call_id, text, source, is_primary,
    confidence, language, model, provider,
    word_count, duration_ms, words, created_at
FROM transcriptions
WHERE call_id = $1
ORDER BY created_at DESC;

-- name: GetCallForTranscription :one
SELECT call_id, start_time, system_id, tgid, duration,
    COALESCE(audio_file_path, '') AS audio_file_path,
    COALESCE(call_filename, '') AS call_filename,
    src_list, encrypted, has_transcription,
    COALESCE(tg_alpha_tag, '') AS tg_alpha_tag,
    COALESCE(tg_description, '') AS tg_description,
    COALESCE(tg_tag, '') AS tg_tag,
    COALESCE(tg_group, '') AS tg_group
FROM calls
WHERE call_id = $1
ORDER BY start_time DESC
LIMIT 1;

-- name: UpdateCallTranscriptionStatus :exec
UPDATE calls SET transcription_status = $3
WHERE call_id = $1 AND start_time = $2;

-- name: UpdateCallGroupTranscriptionStatus :exec
UPDATE call_groups SET transcription_status = $3
WHERE id = (
    SELECT c.call_group_id FROM calls c
    WHERE c.call_id = $1 AND c.start_time = $2 AND c.call_group_id IS NOT NULL
);
