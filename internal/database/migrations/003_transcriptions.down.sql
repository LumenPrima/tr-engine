-- Rollback migration: Remove transcription support

BEGIN;

-- Remove transcription_id from calls
ALTER TABLE calls DROP COLUMN IF EXISTS transcription_id;

-- Drop transcription queue table
DROP TABLE IF EXISTS transcription_queue;

-- Drop transcriptions table
DROP TABLE IF EXISTS transcriptions;

COMMIT;
