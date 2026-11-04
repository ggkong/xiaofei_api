-- Keep repeated context blocks once instead of repeating entire coding histories.
ALTER TABLE conversation_archives ADD COLUMN IF NOT EXISTS storage_version SMALLINT NOT NULL DEFAULT 0;
ALTER TABLE conversation_archives ADD COLUMN IF NOT EXISTS conversation_key VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE conversation_archives ADD COLUMN IF NOT EXISTS prompt_hash VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE conversation_archives ADD COLUMN IF NOT EXISTS group_source VARCHAR(24) NOT NULL DEFAULT '';
ALTER TABLE conversation_archives ADD COLUMN IF NOT EXISTS user_name_snapshot TEXT NOT NULL DEFAULT '';
ALTER TABLE conversation_archives ADD COLUMN IF NOT EXISTS user_email_snapshot TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_archive_conversation_group ON conversation_archives(conversation_key,id DESC);
CREATE TABLE IF NOT EXISTS conversation_archive_blocks (
    hash VARCHAR(64) PRIMARY KEY,
    payload BYTEA NOT NULL,
    raw_bytes INTEGER NOT NULL,
    search_text TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_archive_blocks_gc ON conversation_archive_blocks(last_checked_at,hash);
CREATE INDEX IF NOT EXISTS idx_archive_request_block_refs ON conversation_archives USING GIN ((request_body->'_refs')) WHERE storage_version=1;
CREATE INDEX IF NOT EXISTS idx_archive_response_block_refs ON conversation_archives USING GIN ((response_body->'_refs')) WHERE storage_version=1;
