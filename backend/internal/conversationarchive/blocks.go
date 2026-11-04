package conversationarchive

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"

	"github.com/lib/pq"
	"github.com/tidwall/gjson"
)

const blockGCAdvisoryLock int64 = 24925001

type contextBlock struct {
	Hash       string
	Payload    []byte
	RawBytes   int
	SearchText string
}
type packedValue struct {
	Kind  string          `json:"kind"`
	Raw   json.RawMessage `json:"raw,omitempty"`
	Hash  string          `json:"hash,omitempty"`
	Items []packedValue   `json:"items,omitempty"`
}
type packedSnapshot struct {
	Version   int                    `json:"_format"`
	Refs      []string               `json:"_refs"`
	Preview   string                 `json:"_preview"`
	Fields    map[string]packedValue `json:"fields"`
	Root      *packedValue           `json:"root,omitempty"`
	AutoReply bool                   `json:"keyword_auto_reply,omitempty"`
	Blocked   bool                   `json:"_blocked,omitempty"`
	Omitted   bool                   `json:"_omitted,omitempty"`
	Meta      json.RawMessage        `json:"_meta,omitempty"`
	RawBytes  int                    `json:"_raw_bytes"`
}

func packSnapshot(raw json.RawMessage, request bool, blocks map[string]contextBlock) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	isObject := json.Unmarshal(raw, &fields) == nil && fields != nil
	if !json.Valid(raw) {
		return nil, errors.New("invalid archive snapshot")
	}
	packed := packedSnapshot{Version: 1, Fields: make(map[string]packedValue), Refs: []string{}, RawBytes: len(raw), AutoReply: gjson.GetBytes(raw, "keyword_auto_reply").Bool(), Blocked: BlockedResponse(raw), Omitted: gjson.GetBytes(raw, "omitted").Bool()}
	if request {
		packed.Preview = PromptText(raw, 600)
	} else {
		packed.Preview = ReplyText(raw, 800)
		packed.Meta = json.RawMessage(gjson.GetBytes(raw, "_sub2api_archive").Raw)
		if len(packed.Meta) == 0 {
			packed.Meta, _ = json.Marshal(ModelMetadata{WireModel: ReportedWireModel(raw), Source: "legacy"})
		}
	}
	refs := make(map[string]bool)
	pack := func(value json.RawMessage) (packedValue, error) {
		if len(value) < 256 {
			return packedValue{Kind: "inline", Raw: value}, nil
		}
		var decoded any
		if !decodeJSON(value, &decoded) {
			return packedValue{}, errors.New("invalid context block")
		}
		canonical, err := json.Marshal(decoded)
		if err != nil {
			return packedValue{}, err
		}
		digest := sha256.Sum256(canonical)
		hash := hex.EncodeToString(digest[:])
		if _, ok := blocks[hash]; !ok {
			var compressed bytes.Buffer
			writer, _ := gzip.NewWriterLevel(&compressed, gzip.BestSpeed)
			if _, err = writer.Write(canonical); err != nil {
				return packedValue{}, err
			}
			if err = writer.Close(); err != nil {
				return packedValue{}, err
			}
			blocks[hash] = contextBlock{Hash: hash, Payload: compressed.Bytes(), RawBytes: len(canonical), SearchText: string(canonical)}
		}
		refs[hash] = true
		return packedValue{Kind: "blob", Hash: hash}, nil
	}
	for name, value := range fields {
		var items []json.RawMessage
		if request && (name == "messages" || name == "input") && len(value) > 0 && value[0] == '[' && json.Unmarshal(value, &items) == nil {
			array := packedValue{Kind: "array", Items: make([]packedValue, 0, len(items))}
			for _, item := range items {
				part, err := pack(item)
				if err != nil {
					return nil, err
				}
				array.Items = append(array.Items, part)
			}
			packed.Fields[name] = array
		} else {
			part, err := pack(value)
			if err != nil {
				return nil, err
			}
			packed.Fields[name] = part
		}
	}
	if !isObject {
		part, err := pack(raw)
		if err != nil {
			return nil, err
		}
		packed.Root = &part
	}
	for ref := range refs {
		packed.Refs = append(packed.Refs, ref)
	}
	sort.Strings(packed.Refs)
	return json.Marshal(packed)
}

func persistBlocks(ctx context.Context, tx *sql.Tx, blocks map[string]contextBlock) error {
	if len(blocks) == 0 {
		return nil
	}
	hashes := make([]string, 0, len(blocks))
	payloads := make(pq.ByteaArray, 0, len(blocks))
	sizes := make([]int64, 0, len(blocks))
	search := make([]string, 0, len(blocks))
	for hash, block := range blocks {
		hashes = append(hashes, hash)
		payloads = append(payloads, block.Payload)
		sizes = append(sizes, int64(block.RawBytes))
		search = append(search, block.SearchText)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO conversation_archive_blocks(hash,payload,raw_bytes,search_text) SELECT * FROM unnest($1::text[],$2::bytea[],$3::bigint[],$4::text[]) ON CONFLICT(hash) DO NOTHING`, pq.Array(hashes), payloads, pq.Array(sizes), pq.Array(search))
	return err
}

func (s *Store) unpackSnapshot(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	var packed packedSnapshot
	if json.Unmarshal(raw, &packed) != nil || packed.Version != 1 {
		return nil, errors.New("invalid stored context")
	}
	blocks := make(map[string]json.RawMessage)
	if len(packed.Refs) > 0 {
		rows, err := s.db.QueryContext(ctx, `SELECT hash,payload,raw_bytes FROM conversation_archive_blocks WHERE hash=ANY($1::text[])`, pq.Array(packed.Refs))
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var hash string
			var payload []byte
			var size int
			if err := rows.Scan(&hash, &payload, &size); err != nil {
				return nil, err
			}
			if size < 0 || size > 32<<20 {
				return nil, errors.New("invalid context block size")
			}
			reader, err := gzip.NewReader(bytes.NewReader(payload))
			if err != nil {
				return nil, err
			}
			value, err := io.ReadAll(io.LimitReader(reader, int64(size)+1))
			_ = reader.Close()
			if err != nil || len(value) != size || !json.Valid(value) {
				return nil, errors.New("corrupt context block")
			}
			digest := sha256.Sum256(value)
			if hex.EncodeToString(digest[:]) != hash {
				return nil, errors.New("context checksum mismatch")
			}
			blocks[hash] = value
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	var unpack func(packedValue) (json.RawMessage, error)
	unpack = func(value packedValue) (json.RawMessage, error) {
		switch value.Kind {
		case "inline":
			if !json.Valid(value.Raw) {
				return nil, errors.New("invalid inline context")
			}
			return value.Raw, nil
		case "blob":
			data, ok := blocks[value.Hash]
			if !ok {
				return nil, errors.New("context block missing")
			}
			return data, nil
		case "array":
			items := make([]json.RawMessage, 0, len(value.Items))
			for _, part := range value.Items {
				data, err := unpack(part)
				if err != nil {
					return nil, err
				}
				items = append(items, data)
			}
			return json.Marshal(items)
		}
		return nil, errors.New("invalid context encoding")
	}
	fields := make(map[string]json.RawMessage)
	if packed.Root != nil {
		return unpack(*packed.Root)
	}
	for name, value := range packed.Fields {
		data, err := unpack(value)
		if err != nil {
			return nil, err
		}
		fields[name] = data
	}
	return json.Marshal(fields)
}

func (s *Store) compactRecord(ctx context.Context, tx *sql.Tx, request, response json.RawMessage) (json.RawMessage, json.RawMessage, error) {
	blocks := make(map[string]contextBlock)
	req, err := packSnapshot(request, true, blocks)
	if err != nil {
		return nil, nil, err
	}
	resp, err := packSnapshot(response, false, blocks)
	if err != nil {
		return nil, nil, err
	}
	if err = persistBlocks(ctx, tx, blocks); err != nil {
		return nil, nil, err
	}
	return req, resp, nil
}

func (s *Store) CompactExisting(ctx context.Context, limit int) (int, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("archive unavailable")
	}
	if limit < 1 || limit > 25 {
		limit = 10
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock_shared($1)`, blockGCAdvisoryLock); err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT c.id,c.request_body,c.response_body,c.request_id,c.user_id,c.api_key_id,c.session_id,c.created_at,COALESCE(NULLIF(c.user_name_snapshot,''),u.username,''),COALESCE(NULLIF(c.user_email_snapshot,''),u.email,''),c.conversation_key,c.prompt_hash,c.group_source FROM conversation_archives c LEFT JOIN users u ON u.id=c.user_id WHERE c.storage_version=0 AND c.expires_at>NOW() ORDER BY c.id LIMIT $1 FOR UPDATE OF c SKIP LOCKED`, limit)
	if err != nil {
		return 0, err
	}
	records := []Record{}
	for rows.Next() {
		var r Record
		if err = rows.Scan(&r.ID, &r.RequestBody, &r.ResponseBody, &r.RequestID, &r.UserID, &r.APIKeyID, &r.SessionID, &r.CreatedAt, &r.UserName, &r.UserEmail, &r.ConversationKey, &r.PromptHash, &r.GroupSource); err != nil {
			_ = rows.Close()
			return 0, err
		}
		records = append(records, r)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return 0, err
	}
	for _, r := range records {
		if r.ConversationKey == "" {
			prepareGrouping(&r)
		}
		req, resp, err := s.compactRecord(ctx, tx, r.RequestBody, r.ResponseBody)
		if err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE conversation_archives SET request_body=$2,response_body=$3,storage_version=1,conversation_key=$4,prompt_hash=$5,group_source=$6,user_name_snapshot=$7,user_email_snapshot=$8 WHERE id=$1`, r.ID, string(req), string(resp), r.ConversationKey, r.PromptHash, r.GroupSource, r.UserName, r.UserEmail); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return len(records), nil
}

// ExpandExisting makes the storage format readable by pre-deduplication
// application versions. Operators must pause new capture and check free space.
func (s *Store) ExpandExisting(ctx context.Context, limit int) (int, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("archive unavailable")
	}
	if limit < 1 || limit > 25 {
		limit = 10
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock_shared($1)`, blockGCAdvisoryLock); err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,request_body,response_body FROM conversation_archives WHERE storage_version=1 ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return 0, err
	}
	type row struct {
		id                int64
		request, response json.RawMessage
	}
	records := []row{}
	for rows.Next() {
		var r row
		if err = rows.Scan(&r.id, &r.request, &r.response); err != nil {
			_ = rows.Close()
			return 0, err
		}
		records = append(records, r)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return 0, err
	}
	for _, r := range records {
		request, err := s.unpackSnapshot(ctx, r.request)
		if err != nil {
			return 0, err
		}
		response, err := s.unpackSnapshot(ctx, r.response)
		if err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE conversation_archives SET request_body=$2,response_body=$3,storage_version=0 WHERE id=$1`, r.id, string(request), string(response)); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return len(records), nil
}
