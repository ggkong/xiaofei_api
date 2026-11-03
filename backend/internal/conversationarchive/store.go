// Package conversationarchive stores bounded request snapshots for administrators.
// It never receives API credentials or request headers.
package conversationarchive

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"time"
)

type Options struct {
	Enabled       bool
	MaxBytes      int
	RetentionDays int
}

func OptionsFromEnv() Options {
	o := Options{Enabled: os.Getenv("CONVERSATION_ARCHIVE_ENABLED") == "true", MaxBytes: 1 << 20, RetentionDays: 30}
	if n, err := strconv.Atoi(os.Getenv("CONVERSATION_ARCHIVE_MAX_BYTES")); err == nil && n >= 4096 && n <= 4<<20 {
		o.MaxBytes = n
	}
	if n, err := strconv.Atoi(os.Getenv("CONVERSATION_ARCHIVE_RETENTION_DAYS")); err == nil && n >= 1 && n <= 365 {
		o.RetentionDays = n
	}
	return o
}

type Record struct {
	ID           int64           `json:"id"`
	RequestID    string          `json:"request_id"`
	UserID       *int64          `json:"user_id"`
	APIKeyID     *int64          `json:"api_key_id"`
	SessionID    string          `json:"session_id"`
	Endpoint     string          `json:"endpoint"`
	Model        string          `json:"model"`
	Stream       bool            `json:"stream"`
	HTTPStatus   int             `json:"http_status"`
	Status       string          `json:"status"`
	RequestBody  json.RawMessage `json:"request_body,omitempty"`
	ResponseBody json.RawMessage `json:"response_body,omitempty"`
	Truncated    bool            `json:"truncated"`
	DurationMS   int64           `json:"duration_ms"`
	CreatedAt    time.Time       `json:"created_at"`
	ExpiresAt    time.Time       `json:"expires_at"`
}

type Filter struct {
	UserID       int64
	APIKeyID     int64
	Model        string
	SessionID    string
	Status       string
	BeforeID     int64
	Limit        int
	CreatedFrom  *time.Time
	CreatedUntil *time.Time
}

type Store struct {
	db      *sql.DB
	options Options
}

func New(db *sql.DB, options Options) *Store {
	if options.MaxBytes < 4096 {
		options.MaxBytes = 1 << 20
	}
	if options.RetentionDays < 1 || options.RetentionDays > 365 {
		options.RetentionDays = 30
	}
	return &Store{db: db, options: options}
}

func (s *Store) Enabled() bool { return s != nil && s.options.Enabled && s.db != nil }
func (s *Store) MaxBytes() int { return s.options.MaxBytes }

func (s *Store) Save(ctx context.Context, r *Record) error {
	if !s.Enabled() {
		return nil
	}
	// Independent timeout: a disconnected client must not cancel persistence.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := s.db.ExecContext(ctx, `INSERT INTO conversation_archives
 (request_id,user_id,api_key_id,session_id,endpoint,model,stream,http_status,status,request_body,response_body,truncated,duration_ms,created_at,expires_at)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
 ON CONFLICT (request_id) DO NOTHING`, r.RequestID, r.UserID, r.APIKeyID, r.SessionID, r.Endpoint, r.Model, r.Stream, r.HTTPStatus, r.Status, string(r.RequestBody), string(r.ResponseBody), r.Truncated, r.DurationMS, r.CreatedAt, r.CreatedAt.AddDate(0, 0, s.options.RetentionDays))
	return err
}

const metadataColumns = `id,request_id,user_id,api_key_id,session_id,endpoint,model,stream,http_status,status,truncated,duration_ms,created_at,expires_at`

type scanner interface{ Scan(...any) error }

func scanRecord(row scanner, detail bool) (Record, error) {
	var r Record
	args := []any{&r.ID, &r.RequestID, &r.UserID, &r.APIKeyID, &r.SessionID, &r.Endpoint, &r.Model, &r.Stream, &r.HTTPStatus, &r.Status, &r.Truncated, &r.DurationMS, &r.CreatedAt, &r.ExpiresAt}
	if detail {
		args = append(args, &r.RequestBody, &r.ResponseBody)
	}
	err := row.Scan(args...)
	return r, err
}

func (s *Store) List(ctx context.Context, f Filter) ([]Record, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("archive unavailable")
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 30
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+metadataColumns+` FROM conversation_archives
 WHERE expires_at > NOW() AND ($1::bigint=0 OR user_id=$1) AND ($2::bigint=0 OR api_key_id=$2)
 AND ($3::text='' OR model=$3) AND ($4::text='' OR session_id=$4) AND ($5::text='' OR status=$5)
 AND ($6::bigint=0 OR id<$6)
 AND ($8::timestamptz IS NULL OR created_at >= $8) AND ($9::timestamptz IS NULL OR created_at < $9)
 ORDER BY id DESC LIMIT $7`, f.UserID, f.APIKeyID, f.Model, f.SessionID, f.Status, f.BeforeID, f.Limit, f.CreatedFrom, f.CreatedUntil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	records := make([]Record, 0)
	for rows.Next() {
		r, err := scanRecord(rows, false)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (s *Store) Get(ctx context.Context, id int64) (Record, error) {
	if s == nil || s.db == nil {
		return Record{}, errors.New("archive unavailable")
	}
	return scanRecord(s.db.QueryRowContext(ctx, `SELECT `+metadataColumns+`,request_body,response_body FROM conversation_archives WHERE id=$1 AND expires_at>NOW()`, id), true)
}

func (s *Store) Delete(ctx context.Context, id int64) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("archive unavailable")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM conversation_archives WHERE id=$1`, id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n > 0, err
}

// PurgeExpired performs bounded physical cleanup. Expired rows are always hidden
// immediately; deployments should also schedule cleanup during idle periods.
func (s *Store) PurgeExpired(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM conversation_archives WHERE id IN (SELECT id FROM conversation_archives WHERE expires_at<=NOW() ORDER BY expires_at LIMIT 500)`)
	return err
}
