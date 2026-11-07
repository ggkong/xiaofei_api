package conversationarchive

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/archivewire"
	"github.com/lib/pq"
)

func entryFromRecord(record Record) (archivewire.Entry, error) {
	if record.LocalArchive != nil {
		return archivewire.Entry{}, errLocalCopyOnly
	}
	entry := archivewire.Entry{ID: record.ID, RequestID: record.RequestID, StorageVersion: record.storageVersion, RequestSnapshot: record.RequestBody, ReplySnapshot: record.ResponseBody}
	record.RequestBody, record.ResponseBody = nil, nil
	var err error
	entry.Metadata, err = json.Marshal(record)
	return entry, err
}
func (s *Store) localEntry(ctx context.Context, id int64) (archivewire.Entry, error) {
	// The shared GC lock and row lock keep the snapshot and its referenced blocks
	// stable while collecting an immutable copy for transport.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return archivewire.Entry{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock_shared($1)`, blockGCAdvisoryLock); err != nil {
		return archivewire.Entry{}, err
	}
	r, err := scanRecord(tx.QueryRowContext(ctx, `SELECT `+recordColumns+` FROM conversation_archives c LEFT JOIN users u ON u.id=c.user_id WHERE c.id=$1 AND c.expires_at>NOW() FOR SHARE OF c`, id), true)
	if err != nil {
		return archivewire.Entry{}, err
	}
	entry, err := entryFromRecord(r)
	if err != nil {
		return entry, err
	}
	// Transfer holds are refreshed before releasing the lock. A normal record
	// deletion may remove references before the client downloads the blocks.
	refs, err := entry.Refs()
	if err != nil {
		return entry, err
	}
	if len(refs) > 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE conversation_archive_blocks SET last_checked_at=NOW()+interval '2 hours' WHERE hash=ANY($1::text[])`, pq.Array(refs)); err != nil {
			return entry, err
		}
	}
	return entry, tx.Commit()
}
func (s *Store) localList(ctx context.Context, request archivewire.Request) (archivewire.Response, error) {
	response := archivewire.Response{Version: archivewire.Version, UpperID: request.UpperID, Items: []archivewire.Item{}}
	if request.AfterID < 0 || request.UpperID < 0 {
		return response, errors.New("invalid archive cursor")
	}
	if response.UpperID == 0 {
		if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(id),0) FROM conversation_archives`).Scan(&response.UpperID); err != nil {
			return response, err
		}
	}
	limit := request.Limit
	if limit < 1 || limit > 500 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,request_id,local_archive_device,local_archive_hash FROM conversation_archives WHERE expires_at>NOW() AND id>$1 AND id<=$2 ORDER BY id LIMIT $3`, request.AfterID, response.UpperID, limit)
	if err != nil {
		return response, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var item archivewire.Item
		if err = rows.Scan(&item.ID, &item.RequestID, &item.DeviceID, &item.Digest); err != nil {
			return response, err
		}
		response.Items = append(response.Items, item)
	}
	if len(response.Items) == limit {
		response.NextID = response.Items[len(response.Items)-1].ID
	}
	return response, rows.Err()
}
func (s *Store) localBlocks(ctx context.Context, hashes []string) (archivewire.Response, error) {
	response := archivewire.Response{Version: archivewire.Version, Blocks: []archivewire.Block{}}
	if len(hashes) == 0 || len(hashes) > 64 {
		return response, errors.New("request 1–64 archive blocks")
	}
	for _, hash := range hashes {
		if !archivewire.ValidHash(hash) {
			return response, errors.New("invalid archive hash")
		}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT hash,payload,raw_bytes FROM conversation_archive_blocks WHERE hash=ANY($1::text[]) ORDER BY hash`, pq.Array(hashes))
	if err != nil {
		return response, err
	}
	defer func() { _ = rows.Close() }()
	seen := map[string]bool{}
	size := 0
	for rows.Next() {
		var block archivewire.Block
		if err = rows.Scan(&block.Hash, &block.Payload, &block.RawBytes); err != nil {
			return response, err
		}
		if size > 0 && size+len(block.Payload) > 48<<20 {
			continue
		}
		if block.RawBytes < 0 || block.RawBytes > archivewire.MaxBlock || len(block.Payload) > archivewire.MaxBlock+65536 {
			return response, errors.New("invalid archive block size")
		}
		response.Blocks = append(response.Blocks, block)
		size += len(block.Payload)
		seen[block.Hash] = true
	}
	if err = rows.Err(); err != nil {
		return response, err
	}
	for _, hash := range hashes {
		if !seen[hash] {
			response.Remaining = append(response.Remaining, hash)
		}
	}
	if len(response.Blocks) == 0 {
		return response, errors.New("archive block unavailable")
	}
	return response, nil
}
func (s *Store) localAck(ctx context.Context, device LocalDevice, request archivewire.Request) (archivewire.Response, error) {
	response := archivewire.Response{Version: archivewire.Version}
	if request.ID < 1 || !archivewire.ValidHash(request.Digest) || request.Confirmed < 0 || request.Confirmed > 1000000000 {
		return response, errors.New("invalid archive receipt")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return response, err
	}
	defer func() { _ = tx.Rollback() }()
	// Exclude GC while replacing references; capture can keep using shared blocks.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock_shared($1)`, blockGCAdvisoryLock); err != nil {
		return response, err
	}
	current, err := scanDevice(tx.QueryRowContext(ctx, `SELECT `+deviceColumns+` FROM conversation_archive_devices d JOIN users u ON u.id=d.owner_id WHERE d.id=$1 AND d.enabled AND u.role='admin' AND u.status='active' AND u.deleted_at IS NULL FOR SHARE OF d,u`, device.ID))
	if err != nil {
		return response, err
	}
	r, err := scanRecord(tx.QueryRowContext(ctx, `SELECT `+recordColumns+` FROM conversation_archives c LEFT JOIN users u ON u.id=c.user_id WHERE c.id=$1 AND c.expires_at>NOW() FOR UPDATE OF c`, request.ID), true)
	if err != nil {
		return response, err
	}
	if r.LocalArchive != nil {
		if r.LocalArchive.DeviceID != device.ID || r.LocalArchive.Digest != request.Digest {
			return response, errors.New("archive receipt belongs to a different copy")
		}
		response.Moved = true
	} else {
		entry, entryErr := entryFromRecord(r)
		if entryErr != nil {
			return response, entryErr
		}
		digest, digestErr := entry.Digest()
		if digestErr != nil {
			return response, digestErr
		}
		if digest != request.Digest {
			return response, errors.New("archive receipt is stale; fetch the record again")
		}
		if current.OffloadEnabled && r.CreatedAt.Before(time.Now().AddDate(0, 0, -current.KeepDays)) {
			var requestSnapshot, replySnapshot packedSnapshot
			// Only packed records can be migrated without changing model provenance.
			if r.storageVersion != 1 || json.Unmarshal(r.RequestBody, &requestSnapshot) != nil || json.Unmarshal(r.ResponseBody, &replySnapshot) != nil {
				return response, errors.New("compact this record before migration")
			}
			originalBytes := int64(requestSnapshot.RawBytes) + int64(replySnapshot.RawBytes)
			requestSnapshot.Fields = map[string]packedValue{"model": {Kind: "inline", Raw: mustJSON(r.Model)}}
			requestSnapshot.Root = nil
			requestSnapshot.Refs = []string{}
			requestSnapshot.RawBytes = 0
			replySnapshot.Fields = map[string]packedValue{"_sub2api_archive": {Kind: "inline", Raw: replySnapshot.Meta}}
			replySnapshot.Root = nil
			replySnapshot.Refs = []string{}
			replySnapshot.RawBytes = 0
			if len(replySnapshot.Meta) == 0 {
				replySnapshot.Fields = map[string]packedValue{}
			}
			requestStub, marshalErr := json.Marshal(requestSnapshot)
			if marshalErr != nil {
				return response, marshalErr
			}
			replyStub, marshalErr := json.Marshal(replySnapshot)
			if marshalErr != nil {
				return response, marshalErr
			}
			_, err = tx.ExecContext(ctx, `UPDATE conversation_archives SET request_body=$2,response_body=$3,local_archive_device=$4,local_archive_hash=$5,local_archived_at=NOW(),local_archive_bytes=$6 WHERE id=$1`, r.ID, string(requestStub), string(replyStub), device.ID, request.Digest, originalBytes)
			if err != nil {
				return response, err
			}
			response.Moved = true
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE conversation_archive_devices SET last_seen_at=NOW(),confirmed_records=GREATEST(confirmed_records,$2) WHERE id=$1`, device.ID, request.Confirmed); err != nil {
		return response, err
	}
	return response, tx.Commit()
}
func mustJSON(value string) json.RawMessage { raw, _ := json.Marshal(value); return raw }

// ServeLocalTransfer has no shell, file paths, raw SQL or general administration
// operations. The forced SSH command fixes deviceID before reading the protocol.
func (s *Store) ServeLocalTransfer(ctx context.Context, deviceID string, input io.Reader, output io.Writer) error {
	for {
		var request archivewire.Request
		if err := archivewire.Read(input, &request, archivewire.MaxRequest); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		requestCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		device, err := s.AuthorizedLocalDevice(requestCtx, deviceID)
		response := archivewire.Response{Version: archivewire.Version}
		if err == nil {
			switch request.Operation {
			case "hello":
				response.OffloadEnabled = device.OffloadEnabled
				response.KeepDays = device.KeepDays
			case "list":
				response, err = s.localList(requestCtx, request)
			case "get":
				if request.ID < 1 {
					err = errors.New("invalid archive ID")
					break
				}
				var entry archivewire.Entry
				entry, err = s.localEntry(requestCtx, request.ID)
				response.Entry = &entry
				if err == nil {
					response.Digest, err = response.Entry.Digest()
				}
			case "blocks":
				response, err = s.localBlocks(requestCtx, request.Hashes)
			case "ack":
				response, err = s.localAck(requestCtx, device, request)
			default:
				err = errors.New("unsupported archive operation")
			}
		}
		if err != nil {
			response = archivewire.Response{Version: archivewire.Version, Error: "archive request unavailable"}
			if errors.Is(err, errLocalCopyOnly) {
				response.Error = "record is on the paired computer; restore its local archive file"
			}
			if errors.Is(err, sql.ErrNoRows) {
				response.Error = "record or authorized device is unavailable"
			}
		}
		cancel()
		if err = archivewire.Write(output, response, archivewire.MaxResponse); err != nil {
			return err
		}
	}
}
