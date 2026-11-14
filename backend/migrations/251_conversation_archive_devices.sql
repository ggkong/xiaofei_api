CREATE TABLE IF NOT EXISTS conversation_archive_devices (
 id VARCHAR(64) PRIMARY KEY,
 name VARCHAR(100) NOT NULL,
 public_key TEXT NOT NULL,
 owner_id BIGINT NOT NULL,
 enabled BOOLEAN NOT NULL DEFAULT TRUE,
 offload_enabled BOOLEAN NOT NULL DEFAULT FALSE,
 keep_days INTEGER NOT NULL DEFAULT 7 CHECK(keep_days BETWEEN 1 AND 3650),
 confirmed_records BIGINT NOT NULL DEFAULT 0,
 last_seen_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE conversation_archives ADD COLUMN IF NOT EXISTS local_archive_device VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE conversation_archives ADD COLUMN IF NOT EXISTS local_archive_hash VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE conversation_archives ADD COLUMN IF NOT EXISTS local_archived_at TIMESTAMPTZ;
ALTER TABLE conversation_archives ADD COLUMN IF NOT EXISTS local_archive_bytes BIGINT NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_archive_local_device ON conversation_archives(local_archive_device) WHERE local_archive_device<>'';
