-- Migration: Add word-level timestamps to transcriptions
--
-- This migration adds:
-- 1. words_json JSONB column to store word-level timing data from Whisper

BEGIN;

-- Add words_json column to store array of {word, start, end} objects
ALTER TABLE transcriptions ADD COLUMN words_json JSONB;

COMMIT;
