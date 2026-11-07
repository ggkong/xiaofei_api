// Package localarchive stores verified conversation snapshots on a personal disk.
package localarchive

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/archivewire"
	_ "modernc.org/sqlite"
)

type Database struct{ db *sql.DB }
type Metadata struct {
	ID              int64     `json:"id"`
	RequestID       string    `json:"request_id"`
	UserName        string    `json:"user_name"`
	UserEmail       string    `json:"user_email"`
	Model           string    `json:"model"`
	UpstreamModel   string    `json:"upstream_model"`
	ResponseModel   string    `json:"response_model"`
	PromptPreview   string    `json:"prompt_preview"`
	ResponsePreview string    `json:"response_preview"`
	ConversationKey string    `json:"conversation_key"`
	CreatedAt       time.Time `json:"created_at"`
}

func Open(directory string) (*Database, error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("choose an absolute local archive directory")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(directory, "archive.db"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA busy_timeout=10000;
CREATE TABLE IF NOT EXISTS blocks(hash TEXT PRIMARY KEY,payload BLOB NOT NULL,raw_bytes INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS entries(digest TEXT PRIMARY KEY,id INTEGER NOT NULL,request_id TEXT NOT NULL,entry BLOB NOT NULL,metadata BLOB NOT NULL,created_at TEXT NOT NULL,group_key TEXT NOT NULL,search_text TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS current_entries(id INTEGER PRIMARY KEY,request_id TEXT NOT NULL,digest TEXT NOT NULL REFERENCES entries(digest));
CREATE INDEX IF NOT EXISTS entries_group ON entries(group_key,id);`)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Database{db: db}, nil
}
func (d *Database) Close() error { return d.db.Close() }

type querier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func lookupBlock(ctx context.Context, q querier, hash string) (archivewire.Block, error) {
	var block archivewire.Block
	block.Hash = hash
	err := q.QueryRowContext(ctx, `SELECT payload,raw_bytes FROM blocks WHERE hash=?`, hash).Scan(&block.Payload, &block.RawBytes)
	return block, err
}
func (d *Database) Missing(ctx context.Context, refs []string) ([]string, error) {
	missing := []string{}
	for _, hash := range refs {
		block, err := lookupBlock(ctx, d.db, hash)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if err == nil {
			_, err = block.Decode()
		}
		if err != nil {
			missing = append(missing, hash)
		}
	}
	return missing, nil
}
func (d *Database) SaveBlocks(ctx context.Context, blocks []archivewire.Block) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, block := range blocks {
		if _, err = block.Decode(); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO blocks(hash,payload,raw_bytes) VALUES(?,?,?) ON CONFLICT(hash) DO UPDATE SET payload=excluded.payload,raw_bytes=excluded.raw_bytes`, block.Hash, block.Payload, block.RawBytes); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func validateEntry(ctx context.Context, q querier, entry archivewire.Entry) (json.RawMessage, json.RawMessage, error) {
	refs, err := entry.Refs()
	if err != nil {
		return nil, nil, err
	}
	for _, hash := range refs {
		block, err := lookupBlock(ctx, q, hash)
		if err != nil {
			return nil, nil, err
		}
		if _, err = block.Decode(); err != nil {
			return nil, nil, err
		}
	}
	lookup := func(hash string) (json.RawMessage, error) {
		block, err := lookupBlock(ctx, q, hash)
		if err != nil {
			return nil, err
		}
		return block.Decode()
	}
	request, err := archivewire.Restore(entry.RequestSnapshot, entry.StorageVersion, lookup)
	if err != nil {
		return nil, nil, err
	}
	reply, err := archivewire.Restore(entry.ReplySnapshot, entry.StorageVersion, lookup)
	return request, reply, err
}
func (d *Database) SaveEntry(ctx context.Context, entry archivewire.Entry, digest string) error {
	actual, err := entry.Digest()
	if err != nil || actual != digest {
		return errors.New("archive entry checksum failed")
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, _, err = validateEntry(ctx, tx, entry); err != nil {
		return err
	}
	var meta Metadata
	if err = json.Unmarshal(entry.Metadata, &meta); err != nil {
		return err
	}
	raw, err := entry.Bytes()
	if err != nil {
		return err
	}
	search := strings.Join([]string{meta.UserName, meta.UserEmail, meta.Model, meta.UpstreamModel, meta.ResponseModel, meta.PromptPreview, meta.ResponsePreview}, " ")
	if _, err = tx.ExecContext(ctx, `INSERT INTO entries(digest,id,request_id,entry,metadata,created_at,group_key,search_text) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(digest) DO NOTHING`, digest, entry.ID, entry.RequestID, raw, []byte(entry.Metadata), meta.CreatedAt.Format(time.RFC3339Nano), meta.ConversationKey, search); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO current_entries(id,request_id,digest) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET request_id=excluded.request_id,digest=excluded.digest`, entry.ID, entry.RequestID, digest); err != nil {
		return err
	}
	return tx.Commit()
}
func (d *Database) Entry(ctx context.Context, id int64) (archivewire.Entry, string, error) {
	var raw []byte
	var digest string
	err := d.db.QueryRowContext(ctx, `SELECT e.entry,e.digest FROM current_entries c JOIN entries e ON e.digest=c.digest WHERE c.id=?`, id).Scan(&raw, &digest)
	if err != nil {
		return archivewire.Entry{}, "", err
	}
	if archivewire.Hash(raw) != digest {
		return archivewire.Entry{}, "", errors.New("local entry checksum failed")
	}
	var entry archivewire.Entry
	err = json.Unmarshal(raw, &entry)
	return entry, digest, err
}
func (d *Database) Verify(ctx context.Context, id int64, digest string) error {
	entry, stored, err := d.Entry(ctx, id)
	if err != nil {
		return err
	}
	if stored != digest {
		return errors.New("local receipt mismatch")
	}
	_, _, err = validateEntry(ctx, d.db, entry)
	return err
}
func (d *Database) Detail(ctx context.Context, id int64) (json.RawMessage, error) {
	entry, _, err := d.Entry(ctx, id)
	if err != nil {
		return nil, err
	}
	request, reply, err := validateEntry(ctx, d.db, entry)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(entry.Metadata, &fields); err != nil {
		return nil, err
	}
	fields["request_body"], fields["response_body"] = request, reply
	return json.Marshal(fields)
}
func (d *Database) Count(ctx context.Context) (int64, error) {
	var n int64
	err := d.db.QueryRowContext(ctx, `SELECT count(*) FROM current_entries`).Scan(&n)
	return n, err
}

type Page struct {
	Items  []json.RawMessage `json:"items"`
	NextID int64             `json:"next_id"`
	Count  int64             `json:"count"`
}

func (d *Database) List(ctx context.Context, before int64, query, group string) (Page, error) {
	page := Page{Items: []json.RawMessage{}}
	rows, err := d.db.QueryContext(ctx, `SELECT e.metadata,e.id FROM current_entries c JOIN entries e ON e.digest=c.digest WHERE (?=0 OR e.id<?) AND (?='' OR instr(lower(e.search_text),lower(?))>0) AND (?='' OR e.group_key=?) ORDER BY e.id DESC LIMIT 50`, before, before, query, query, group, group)
	if err != nil {
		return page, err
	}
	for rows.Next() {
		var raw []byte
		var id int64
		if err = rows.Scan(&raw, &id); err != nil {
			_ = rows.Close()
			return page, err
		}
		page.Items = append(page.Items, json.RawMessage(raw))
		page.NextID = id
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return page, err
	}
	if len(page.Items) < 50 {
		page.NextID = 0
	}
	page.Count, err = d.Count(ctx)
	return page, err
}
func (d *Database) VerifyAll(ctx context.Context) error {
	var before int64
	for {
		page, err := d.List(ctx, before, "", "")
		if err != nil {
			return err
		}
		for _, raw := range page.Items {
			var meta Metadata
			if err = json.Unmarshal(raw, &meta); err != nil {
				return err
			}
			_, digest, err := d.Entry(ctx, meta.ID)
			if err != nil {
				return err
			}
			if err = d.Verify(ctx, meta.ID, digest); err != nil {
				return fmt.Errorf("record %d: %w", meta.ID, err)
			}
		}
		if page.NextID == 0 {
			return nil
		}
		before = page.NextID
	}
}
