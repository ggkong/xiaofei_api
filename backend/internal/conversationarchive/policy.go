package conversationarchive

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"
)

const policySettingKey = "conversation_archive_policy"

type Policy struct {
	RetentionDays   int `json:"retention_days"`
	MaxContextBytes int `json:"max_context_bytes"`
}

type StorageInfo struct {
	Policy
	Records       int64 `json:"records"`
	StorageBytes  int64 `json:"storage_bytes"`
	BlockCount    int64 `json:"unique_blocks"`
	LogicalBytes  int64 `json:"logical_bytes"`
	FreeBytes     int64 `json:"free_bytes"`
	CapturePaused bool  `json:"capture_paused"`
}

func (s *Store) defaultPolicy() Policy {
	days := s.options.RetentionDays
	if s.options.Permanent {
		days = 0
	}
	return Policy{RetentionDays: days, MaxContextBytes: s.options.MaxBytes}
}

func (s *Store) Policy(ctx context.Context) Policy {
	if !s.options.Configurable || s.db == nil {
		return s.defaultPolicy()
	}
	s.policyMu.Lock()
	defer s.policyMu.Unlock()
	if time.Since(s.policyLoadedAt) < time.Second {
		return s.policyValue
	}
	value := s.defaultPolicy()
	if !s.policyLoadedAt.IsZero() {
		value = s.policyValue
	}
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1`, policySettingKey).Scan(&raw); err == nil {
		var stored Policy
		if json.Unmarshal([]byte(raw), &stored) == nil && validatePolicy(stored) == nil {
			value = stored
		}
	}
	s.policyValue, s.policyLoadedAt = value, time.Now()
	return value
}

func validatePolicy(policy Policy) error {
	if policy.RetentionDays < 0 || policy.RetentionDays > 3650 {
		return errors.New("retention must be 0 (permanent) or 1–3650 days")
	}
	if policy.MaxContextBytes < 4096 || policy.MaxContextBytes > 16<<20 {
		return errors.New("context limit must be between 4 KiB and 16 MiB")
	}
	return nil
}

func (s *Store) UpdatePolicy(ctx context.Context, policy Policy) error {
	if s == nil || s.db == nil {
		return errors.New("archive unavailable")
	}
	if err := validatePolicy(policy); err != nil {
		return err
	}
	raw, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, blockGCAdvisoryLock); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES($1,$2,NOW()) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=NOW()`, policySettingKey, string(raw)); err != nil {
		return err
	}
	if policy.RetentionDays == 0 {
		_, err = tx.ExecContext(ctx, `UPDATE conversation_archives SET expires_at='9999-12-31T00:00:00Z'`)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE conversation_archives SET expires_at=created_at+make_interval(days=>$1) WHERE expires_at>NOW()`, policy.RetentionDays)
	}
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.policyMu.Lock()
	s.policyValue, s.policyLoadedAt = policy, time.Now()
	s.policyMu.Unlock()
	return nil
}

func archiveDataPath() string {
	if info, err := os.Stat("/app/data"); err == nil && info.IsDir() {
		return "/app/data"
	}
	return "."
}

func (s *Store) StorageInfo(ctx context.Context) (StorageInfo, error) {
	if s == nil || s.db == nil {
		return StorageInfo{}, errors.New("archive unavailable")
	}
	info := StorageInfo{Policy: s.Policy(ctx), FreeBytes: filesystemFreeBytes(archiveDataPath())}
	info.CapturePaused = info.FreeBytes > 0 && info.FreeBytes < 512<<20
	err := s.db.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(CASE WHEN storage_version=1 THEN (request_body->>'_raw_bytes')::bigint+(response_body->>'_raw_bytes')::bigint ELSE octet_length(request_body::text)+octet_length(response_body::text) END),0) FROM conversation_archives WHERE expires_at>NOW()`).Scan(&info.Records, &info.LogicalBytes)
	if err != nil {
		return info, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT pg_total_relation_size('conversation_archives')+pg_total_relation_size('conversation_archive_blocks'),(SELECT count(*) FROM conversation_archive_blocks)`).Scan(&info.StorageBytes, &info.BlockCount)
	return info, err
}
