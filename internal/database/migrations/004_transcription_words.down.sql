-- Rollback: Remove word-level timestamps from transcriptions

BEGIN;

ALTER TABLE transcriptions DROP COLUMN IF EXISTS words_json;

COMMIT;
