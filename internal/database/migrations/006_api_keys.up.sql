-- API keys table for database-managed keys
CREATE TABLE IF NOT EXISTS api_keys (
    id SERIAL PRIMARY KEY,
    key_hash TEXT NOT NULL UNIQUE,        -- SHA-256 hash of the key (never store plaintext)
    key_prefix VARCHAR(12) NOT NULL,      -- First 8 chars for identification (tr_api_xxx...)
    name VARCHAR(255) NOT NULL,           -- Human-readable name
    scopes TEXT[] DEFAULT '{}',           -- Future: permission scopes
    read_only BOOLEAN DEFAULT FALSE,      -- Future: read-only access
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,               -- NULL = never expires
    revoked_at TIMESTAMPTZ                -- NULL = active, set = revoked
);

CREATE INDEX idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX idx_api_keys_prefix ON api_keys(key_prefix);
