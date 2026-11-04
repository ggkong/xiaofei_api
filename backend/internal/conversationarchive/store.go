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
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
	"github.com/tidwall/gjson"
)

type Options struct {
	Enabled       bool
	MaxBytes      int
	RetentionDays int
	Permanent     bool
	Deduplicate   bool
	Configurable  bool
}

func OptionsFromEnv() Options {
	o := Options{Enabled: os.Getenv("CONVERSATION_ARCHIVE_ENABLED") == "true", MaxBytes: 4 << 20, RetentionDays: 0, Permanent: true, Deduplicate: os.Getenv("CONVERSATION_ARCHIVE_DEDUP_ENABLED") != "false", Configurable: true}
	if n, err := strconv.Atoi(os.Getenv("CONVERSATION_ARCHIVE_MAX_BYTES")); err == nil && n >= 4096 && n <= 16<<20 {
		o.MaxBytes = n
	}
	if n, err := strconv.Atoi(os.Getenv("CONVERSATION_ARCHIVE_RETENTION_DAYS")); err == nil && n >= 0 && n <= 3650 {
		o.RetentionDays = n
		o.Permanent = n == 0
	}
	return o
}

type Record struct {
	ID              int64           `json:"id"`
	RequestID       string          `json:"request_id"`
	UserID          *int64          `json:"user_id"`
	APIKeyID        *int64          `json:"api_key_id"`
	SessionID       string          `json:"session_id"`
	Endpoint        string          `json:"endpoint"`
	Model           string          `json:"model"`
	Stream          bool            `json:"stream"`
	HTTPStatus      int             `json:"http_status"`
	Status          string          `json:"status"`
	RequestBody     json.RawMessage `json:"request_body,omitempty"`
	ResponseBody    json.RawMessage `json:"response_body,omitempty"`
	Truncated       bool            `json:"truncated"`
	DurationMS      int64           `json:"duration_ms"`
	CreatedAt       time.Time       `json:"created_at"`
	ExpiresAt       time.Time       `json:"expires_at"`
	UserName        string          `json:"user_name"`
	UserEmail       string          `json:"user_email"`
	PromptPreview   string          `json:"prompt_preview"`
	ResponsePreview string          `json:"response_preview"`
	Blocked         bool            `json:"blocked"`
	AutoReply       bool            `json:"auto_reply"`
	PromptOmitted   bool            `json:"prompt_omitted"`
	UpstreamModel   string          `json:"upstream_model"`
	ResponseModel   string          `json:"response_model"`
	WireModel       string          `json:"wire_model"`
	ModelConflict   bool            `json:"model_conflict"`
	ReplySource     string          `json:"reply_source"`
	ConversationKey string          `json:"conversation_key"`
	PromptHash      string          `json:"-"`
	GroupSource     string          `json:"group_source"`
	CallCount       int64           `json:"call_count,omitempty"`
	Models          []string        `json:"models,omitempty"`
	FirstAt         *time.Time      `json:"first_at,omitempty"`
	BlockedCount    int64           `json:"blocked_count,omitempty"`
	AutoReplyCount  int64           `json:"auto_reply_count,omitempty"`
	storageVersion  int
}

type Filter struct {
	UserID          int64
	APIKeyID        int64
	Model           string
	SessionID       string
	Status          string
	BeforeID        int64
	Limit           int
	CreatedFrom     *time.Time
	CreatedUntil    *time.Time
	Query           string
	ConversationKey string
}

type Store struct {
	db             *sql.DB
	options        Options
	policyMu       sync.Mutex
	policyValue    Policy
	policyLoadedAt time.Time
}

func New(db *sql.DB, options Options) *Store {
	if options.MaxBytes < 4096 {
		options.MaxBytes = 1 << 20
	}
	if (!options.Permanent && options.RetentionDays < 1) || options.RetentionDays > 3650 {
		options.RetentionDays = 30
	}
	return &Store{db: db, options: options}
}

func (s *Store) Enabled() bool { return s != nil && s.options.Enabled && s.db != nil }
func (s *Store) MaxBytes() int { return s.Policy(context.Background()).MaxContextBytes }
func (s *Store) CapturePaused() bool {
	free := filesystemFreeBytes(archiveDataPath())
	return free > 0 && free < 512<<20
}

func (s *Store) Save(ctx context.Context, r *Record) error {
	if !s.Enabled() {
		return nil
	}
	if s.CapturePaused() {
		return errors.New("archive storage capacity reached")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	policy := s.defaultPolicy()
	expires := r.CreatedAt.AddDate(0, 0, policy.RetentionDays)
	if policy.RetentionDays == 0 {
		expires = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	}
	request, response := r.RequestBody, r.ResponseBody
	args := func() []any {
		return []any{r.RequestID, r.UserID, r.APIKeyID, r.SessionID, r.Endpoint, r.Model, r.Stream, r.HTTPStatus, r.Status, string(request), string(response), r.Truncated, r.DurationMS, r.CreatedAt, expires}
	}
	const insert = `INSERT INTO conversation_archives
 (request_id,user_id,api_key_id,session_id,endpoint,model,stream,http_status,status,request_body,response_body,truncated,duration_ms,created_at,expires_at)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
 ON CONFLICT (request_id) DO NOTHING`
	if !s.options.Deduplicate && !s.options.Configurable {
		_, err := s.db.ExecContext(ctx, insert, args()...)
		return err
	}
	prepareGrouping(r)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock_shared($1)`, blockGCAdvisoryLock); err != nil {
		return err
	}
	if s.options.Configurable {
		var raw string
		err = tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1`, policySettingKey).Scan(&raw)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if err == nil {
			if json.Unmarshal([]byte(raw), &policy) != nil || validatePolicy(policy) != nil {
				return errors.New("invalid archive policy")
			}
		}
		expires = r.CreatedAt.AddDate(0, 0, policy.RetentionDays)
		if policy.RetentionDays == 0 {
			expires = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
		}
	}
	version := 0
	if s.options.Deduplicate {
		request, response, err = s.compactRecord(ctx, tx, request, response)
		if err != nil {
			return err
		}
		version = 1
	}
	const compactInsert = `INSERT INTO conversation_archives
 (request_id,user_id,api_key_id,session_id,endpoint,model,stream,http_status,status,request_body,response_body,truncated,duration_ms,created_at,expires_at,storage_version,conversation_key,prompt_hash,group_source,user_name_snapshot,user_email_snapshot)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
 ON CONFLICT(request_id) DO NOTHING`
	if _, err = tx.ExecContext(ctx, compactInsert, append(args(), version, r.ConversationKey, r.PromptHash, r.GroupSource, r.UserName, r.UserEmail)...); err != nil {
		return err
	}
	return tx.Commit()
}

const metadataColumns = `id,request_id,user_id,api_key_id,session_id,endpoint,model,stream,http_status,status,truncated,duration_ms,created_at,expires_at`

var recordColumns = "c." + strings.ReplaceAll(metadataColumns, ",", ",c.") + `,COALESCE(NULLIF(c.user_name_snapshot,''),u.username,''),COALESCE(NULLIF(c.user_email_snapshot,''),u.email,''),c.request_body,c.response_body,c.storage_version,c.conversation_key,c.group_source`

type scanner interface{ Scan(...any) error }

func scanRecord(row scanner, detail bool) (Record, error) {
	var r Record
	args := []any{&r.ID, &r.RequestID, &r.UserID, &r.APIKeyID, &r.SessionID, &r.Endpoint, &r.Model, &r.Stream, &r.HTTPStatus, &r.Status, &r.Truncated, &r.DurationMS, &r.CreatedAt, &r.ExpiresAt}
	args = append(args, &r.UserName, &r.UserEmail, &r.RequestBody, &r.ResponseBody, &r.storageVersion, &r.ConversationKey, &r.GroupSource)
	err := row.Scan(args...)
	if err == nil {
		meta := gjson.GetBytes(r.ResponseBody, "_sub2api_archive")
		if r.storageVersion == 1 {
			var request, response packedSnapshot
			if json.Unmarshal(r.RequestBody, &request) != nil || json.Unmarshal(r.ResponseBody, &response) != nil {
				return r, errors.New("invalid context snapshot")
			}
			r.PromptPreview, r.ResponsePreview = request.Preview, response.Preview
			r.Blocked, r.AutoReply, r.PromptOmitted = response.Blocked, response.AutoReply, request.Omitted
			meta = gjson.ParseBytes(response.Meta)
		} else {
			r.PromptPreview = PromptText(r.RequestBody, 600)
			r.ResponsePreview = ReplyText(r.ResponseBody, 800)
			r.Blocked = BlockedResponse(r.ResponseBody)
			r.AutoReply = gjson.GetBytes(r.ResponseBody, "keyword_auto_reply").Bool()
			r.PromptOmitted = gjson.GetBytes(r.RequestBody, "omitted").Bool()
		}
		r.UpstreamModel = meta.Get("upstream_model").String()
		r.ResponseModel = meta.Get("response_model").String()
		r.WireModel = meta.Get("wire_model").String()
		if r.WireModel == "" && r.storageVersion == 0 {
			r.WireModel = ReportedWireModel(r.ResponseBody)
		}
		r.ModelConflict = meta.Get("conflict").Bool()
		r.ReplySource = meta.Get("source").String()
	}

	if !detail {
		r.RequestBody, r.ResponseBody = nil, nil
	}
	return r, err
}

func (s *Store) List(ctx context.Context, f Filter) ([]Record, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("archive unavailable")
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 30
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+recordColumns+` FROM conversation_archives c LEFT JOIN users u ON u.id=c.user_id`+archiveWhere+` ORDER BY c.id DESC LIMIT $7`, filterArgs(f)...)
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
	record, err := scanRecord(s.db.QueryRowContext(ctx, `SELECT `+recordColumns+` FROM conversation_archives c LEFT JOIN users u ON u.id=c.user_id WHERE c.id=$1 AND c.expires_at>NOW()`, id), true)
	if err != nil || record.storageVersion == 0 {
		return record, err
	}
	record.RequestBody, err = s.unpackSnapshot(ctx, record.RequestBody)
	if err != nil {
		return Record{}, err
	}
	record.ResponseBody, err = s.unpackSnapshot(ctx, record.ResponseBody)
	return record, err

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
	if !s.options.Deduplicate {
		_, err := s.db.ExecContext(ctx, `DELETE FROM conversation_archives WHERE id IN (SELECT id FROM conversation_archives WHERE expires_at<=NOW() ORDER BY expires_at LIMIT 500)`)
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var locked bool
	if err = tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock($1)`, blockGCAdvisoryLock).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return nil
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM conversation_archives WHERE id IN (SELECT id FROM conversation_archives WHERE expires_at<=NOW() ORDER BY expires_at LIMIT 500)`); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT hash FROM conversation_archive_blocks ORDER BY last_checked_at,hash LIMIT 500`)
	if err != nil {
		return err
	}
	hashes := []string{}
	for rows.Next() {
		var hash string
		if err = rows.Scan(&hash); err != nil {
			_ = rows.Close()
			return err
		}
		hashes = append(hashes, hash)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	if len(hashes) > 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE conversation_archive_blocks SET last_checked_at=NOW() WHERE hash=ANY($1::text[])`, pq.Array(hashes)); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM conversation_archive_blocks b WHERE b.hash=ANY($1::text[]) AND NOT EXISTS(SELECT 1 FROM conversation_archives c WHERE c.storage_version=1 AND (c.request_body->'_refs' ? b.hash OR c.response_body->'_refs' ? b.hash))`, pq.Array(hashes)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const archiveWhere = ` WHERE c.expires_at > NOW() AND ($1::bigint=0 OR c.user_id=$1) AND ($2::bigint=0 OR c.api_key_id=$2)
 AND ($3::text='' OR c.model=$3 OR (c.storage_version=1 AND (c.response_body#>>'{_meta,upstream_model}'=$3 OR c.response_body#>>'{_meta,response_model}'=$3)) OR c.response_body#>>'{_sub2api_archive,upstream_model}'=$3 OR c.response_body#>>'{_sub2api_archive,response_model}'=$3) AND ($4::text='' OR c.session_id=$4)
 AND ($5::text='' OR c.status=$5 OR ($5='auto_reply' AND c.response_body->>'keyword_auto_reply'='true') OR ($5='blocked' AND c.http_status=403 AND
 (c.response_body#>>'{error,code}'='content_policy_violation' OR c.response_body#>>'{error,type}'='content_policy_violation' OR c.response_body->>'_blocked'='true')))
 AND ($6::bigint=0 OR c.id<$6)
 AND ($8::timestamptz IS NULL OR c.created_at >= $8) AND ($9::timestamptz IS NULL OR c.created_at < $9)
 AND ($10::text='' OR strpos(lower(concat_ws(' ',c.user_name_snapshot,c.user_email_snapshot,u.username,u.email,c.model,c.request_body::text,c.response_body::text)),lower($10))>0 OR
 (c.storage_version=1 AND EXISTS(SELECT 1 FROM conversation_archive_blocks b WHERE b.hash IN (SELECT jsonb_array_elements_text(CASE WHEN c.storage_version=1 THEN COALESCE(c.request_body->'_refs','[]') || COALESCE(c.response_body->'_refs','[]') ELSE '[]'::jsonb END)) AND strpos(lower(b.search_text),lower($10))>0)))
 AND ($11::text='' OR CASE WHEN c.conversation_key<>'' THEN c.conversation_key ELSE 'record:'||c.id::text END=$11)`

func filterArgs(f Filter) []any {
	return []any{f.UserID, f.APIKeyID, f.Model, f.SessionID, f.Status, f.BeforeID, f.Limit, f.CreatedFrom, f.CreatedUntil, f.Query, f.ConversationKey}
}
