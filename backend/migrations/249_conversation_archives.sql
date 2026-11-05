-- Separate from billing logs: one immutable client-visible request/response snapshot.
CREATE TABLE IF NOT EXISTS conversation_archives (
    id BIGSERIAL PRIMARY KEY,
    request_id VARCHAR(64) NOT NULL UNIQUE,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    api_key_id BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,
    session_id VARCHAR(255) NOT NULL DEFAULT '',
    endpoint VARCHAR(255) NOT NULL,
    model VARCHAR(255) NOT NULL DEFAULT '',
    stream BOOLEAN NOT NULL DEFAULT FALSE,
    http_status INTEGER NOT NULL,
    status VARCHAR(32) NOT NULL CHECK (status IN ('complete', 'interrupted', 'error')),
    request_body JSONB NOT NULL,
    response_body JSONB NOT NULL,
    truncated BOOLEAN NOT NULL DEFAULT FALSE,
    duration_ms BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_conversation_archives_created ON conversation_archives(created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_conversation_archives_user ON conversation_archives(user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_conversation_archives_session ON conversation_archives(user_id, api_key_id, session_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_conversation_archives_expiry ON conversation_archives(expires_at);
