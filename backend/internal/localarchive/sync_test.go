package localarchive

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/archivewire"
)

type fakeRemote struct {
	entries map[int64]archivewire.Entry
	block   archivewire.Block
	acks    int
	bad     bool
	failAck bool
	offload bool
	moved   map[int64]string
}

func fixture(t *testing.T, id int64) (archivewire.Entry, archivewire.Block) {
	t.Helper()
	raw := json.RawMessage(`{"role":"user","content":"<img src=x onerror=alert(1)> hello","n":9007199254740993}`)
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	block := archivewire.Block{Hash: archivewire.Hash(raw), Payload: b.Bytes(), RawBytes: len(raw)}
	meta, _ := json.Marshal(map[string]any{"id": id, "request_id": fmt.Sprint("request-", id), "created_at": time.Now().Add(-10 * 24 * time.Hour), "user_name": "张三", "model": "model-a", "upstream_model": "model-real", "prompt_preview": "hello", "conversation_key": "same"})
	entry := archivewire.Entry{ID: id, RequestID: fmt.Sprint("request-", id), StorageVersion: 1, Metadata: meta, RequestSnapshot: json.RawMessage(`{"_format":1,"_refs":["` + block.Hash + `"],"fields":{"messages":{"kind":"array","items":[{"kind":"blob","hash":"` + block.Hash + `"}]}}}`), ReplySnapshot: json.RawMessage(`{"_format":1,"_refs":[],"fields":{"choices":{"kind":"inline","raw":[{"message":{"role":"assistant","content":"world"}}]}}}`)}
	return entry, block
}
func (f *fakeRemote) Call(_ context.Context, r archivewire.Request) (archivewire.Response, error) {
	response := archivewire.Response{Version: 1}
	switch r.Operation {
	case "hello":
		response.OffloadEnabled = f.offload
		response.KeepDays = 7
	case "list":
		for id := int64(1); id <= 10; id++ {
			entry, ok := f.entries[id]
			if ok {
				response.Items = append(response.Items, archivewire.Item{ID: id, RequestID: entry.RequestID, DeviceID: func() string {
					if f.moved[id] != "" {
						return "device"
					}
					return ""
				}(), Digest: f.moved[id]})
				response.UpperID = id
			}
		}
	case "get":
		entry := f.entries[r.ID]
		response.Entry = &entry
		response.Digest, _ = entry.Digest()
	case "blocks":
		block := f.block
		if f.bad {
			block.Payload = []byte("broken")
		}
		response.Blocks = []archivewire.Block{block}
	case "ack":
		f.acks++
		if f.failAck {
			f.failAck = false
			return response, errors.New("interrupted connection")
		}
		if f.offload {
			if f.moved == nil {
				f.moved = map[int64]string{}
			}
			f.moved[r.ID] = r.Digest
			response.Moved = true
		}
	}
	return response, nil
}
func TestSyncResumesLateCommitsAndMigratesOnlyVerifiedBytes(t *testing.T) {
	ctx := context.Background()
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	e2, block := fixture(t, 2)
	f := &fakeRemote{entries: map[int64]archivewire.Entry{2: e2}, block: block, failAck: true}
	if _, err = Sync(ctx, db, f, nil); err == nil {
		t.Fatal("expected interrupted receipt")
	}
	if count, _ := db.Count(ctx); count != 1 {
		t.Fatal("durable copy lost before ack")
	}
	e1, _ := fixture(t, 1)
	f.entries[1] = e1
	if p, err := Sync(ctx, db, f, nil); err != nil || p.Total != 2 || p.Downloaded != 1 {
		t.Fatalf("late lower-ID commit skipped: %+v %v", p, err)
	}
	f.offload = true
	if p, err := Sync(ctx, db, f, nil); err != nil || p.Moved != 2 {
		t.Fatalf("verified migration: %+v %v", p, err)
	}
	raw, err := db.Detail(ctx, 1)
	if err != nil || !strings.Contains(string(raw), "9007199254740993") || !strings.Contains(string(raw), "model-real") {
		t.Fatal("context or model changed", err)
	}
	if err = db.VerifyAll(ctx); err != nil {
		t.Fatal(err)
	}
	var blocks int
	if err = db.db.QueryRow(`SELECT count(*) FROM blocks`).Scan(&blocks); err != nil || blocks != 1 {
		t.Fatal("shared blocks duplicated", err)
	}
}
func TestCorruptionAndMissingLocalArchiveNeverAcknowledge(t *testing.T) {
	ctx := context.Background()
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	entry, block := fixture(t, 1)
	f := &fakeRemote{entries: map[int64]archivewire.Entry{1: entry}, block: block, bad: true, offload: true}
	if _, err = Sync(ctx, db, f, nil); err == nil || f.acks != 0 {
		t.Fatal("corrupt payload acknowledged")
	}
	if count, _ := db.Count(ctx); count != 0 {
		t.Fatal("partial record became visible")
	}
	f.bad = false
	if _, err = Sync(ctx, db, f, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = db.db.Exec(`UPDATE blocks SET payload=x'00'`); err != nil {
		t.Fatal(err)
	}
	if err = db.VerifyAll(ctx); err == nil {
		t.Fatal("corrupt durable archive passed verification")
	}
	if _, err = db.Detail(ctx, 1); err == nil {
		t.Fatal("corrupt detail returned")
	}
	if _, err = db.db.Exec(`DELETE FROM current_entries`); err != nil {
		t.Fatal(err)
	}
	acks := f.acks
	if _, err = Sync(ctx, db, f, nil); err == nil || f.acks != acks {
		t.Fatal("missing moved archive acknowledged")
	}
}
