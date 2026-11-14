package localarchive

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/archivewire"
)

type Remote interface {
	Call(context.Context, archivewire.Request) (archivewire.Response, error)
}
type Progress struct {
	Checked    int64 `json:"checked"`
	Downloaded int64 `json:"downloaded"`
	Moved      int64 `json:"moved"`
	Total      int64 `json:"total"`
}

// Sync rescans the bounded manifest on every run: late commits with lower IDs
// are therefore picked up on the following run instead of being skipped forever.
func Sync(ctx context.Context, db *Database, remote Remote, progress func(Progress)) (Progress, error) {
	p := Progress{}
	hello, err := remote.Call(ctx, archivewire.Request{Operation: "hello"})
	if err != nil {
		return p, err
	}
	p.Total, err = db.Count(ctx)
	if err != nil {
		return p, err
	}
	var after, upper int64
	for {
		page, err := remote.Call(ctx, archivewire.Request{Operation: "list", AfterID: after, UpperID: upper, Limit: 200})
		if err != nil {
			return p, err
		}
		upper = page.UpperID
		for _, item := range page.Items {
			if item.ID <= after || item.ID > upper {
				return p, errors.New("invalid archive manifest order")
			}
			after = item.ID
			entry, digest, localErr := db.Entry(ctx, item.ID)
			if localErr != nil && !errors.Is(localErr, sql.ErrNoRows) {
				return p, localErr
			}
			if item.DeviceID != "" {
				if localErr != nil || entry.RequestID != item.RequestID || digest != item.Digest {
					return p, fmt.Errorf("record %d is on another archive or is missing locally; keep the original archive disk", item.ID)
				}
				p.Checked++
				if progress != nil {
					progress(p)
				}
				continue
			}
			fresh := localErr != nil || entry.RequestID != item.RequestID
			needsAck := fresh
			if !fresh && hello.OffloadEnabled {
				var meta Metadata
				if json.Unmarshal(entry.Metadata, &meta) != nil {
					return p, errors.New("invalid local metadata")
				}
				needsAck = meta.CreatedAt.Before(time.Now().AddDate(0, 0, -hello.KeepDays))
			}
			if needsAck {
				// Refresh the immutable snapshot before issuing a receipt. User-name
				// fallbacks and compaction can change its digest between runs.
				response, err := remote.Call(ctx, archivewire.Request{Operation: "get", ID: item.ID})
				if err != nil {
					return p, err
				}
				if response.Entry == nil || response.Entry.ID != item.ID || response.Entry.RequestID != item.RequestID {
					return p, errors.New("archive response identity mismatch")
				}
				entry = *response.Entry
				digest = response.Digest
				refs, err := entry.Refs()
				if err != nil {
					return p, err
				}
				missing, err := db.Missing(ctx, refs)
				if err != nil {
					return p, err
				}
				for len(missing) > 0 {
					end := min(len(missing), 64)
					batch := missing[:end]
					response, err := remote.Call(ctx, archivewire.Request{Operation: "blocks", Hashes: batch})
					if err != nil {
						return p, err
					}
					allowed := map[string]bool{}
					for _, hash := range batch {
						allowed[hash] = true
					}
					if len(response.Blocks) == 0 {
						return p, errors.New("archive block transfer made no progress")
					}
					for _, block := range response.Blocks {
						if !allowed[block.Hash] {
							return p, errors.New("unexpected archive block")
						}
					}
					if err = db.SaveBlocks(ctx, response.Blocks); err != nil {
						return p, err
					}
					next, err := db.Missing(ctx, refs)
					if err != nil {
						return p, err
					}
					if len(next) >= len(missing) {
						return p, errors.New("archive block transfer made no progress")
					}
					missing = next
				}
				if err = db.SaveEntry(ctx, entry, digest); err != nil {
					return p, err
				}
				// Reopen committed bytes and restore both bodies before acknowledging.
				// An interrupted write, full disk, or checksum error never sends ack.
				if err = db.Verify(ctx, entry.ID, digest); err != nil {
					return p, err
				}
				p.Total, err = db.Count(ctx)
				if err != nil {
					return p, err
				}
				receipt, err := remote.Call(ctx, archivewire.Request{Operation: "ack", ID: entry.ID, Digest: digest, Confirmed: p.Total})
				if err != nil {
					return p, err
				}
				if fresh {
					p.Downloaded++
				}
				if receipt.Moved {
					p.Moved++
				}
			}
			p.Checked++
			if progress != nil {
				progress(p)
			}
		}
		if page.NextID == 0 {
			return p, nil
		}
		if page.NextID != after {
			return p, errors.New("archive cursor did not advance")
		}
	}
}
