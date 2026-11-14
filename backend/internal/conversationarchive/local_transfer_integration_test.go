package conversationarchive

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/archivewire"
	"golang.org/x/crypto/ssh"
)

func TestLocalArchivePostgresVerifiedMigration(t *testing.T) {
	dsn := os.Getenv("ARCHIVE_TEST_DSN")
	if dsn == "" {
		t.Skip("ARCHIVE_TEST_DSN is not configured")
	}
	base, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = base.Close() }()
	var database string
	if err = base.QueryRow(`SELECT current_database()`).Scan(&database); err != nil || database != "archive_test" {
		t.Fatal("refusing non-test database")
	}
	schema := fmt.Sprint("archive_local_", time.Now().UnixNano())
	if _, err = base.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = base.Exec(`DROP SCHEMA ` + schema + ` CASCADE`) }()
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Scheme != "postgres" {
		t.Fatal("postgres URL required")
	}
	q := parsed.Query()
	q.Set("search_path", schema)
	parsed.RawQuery = q.Encode()
	db, err := sql.Open("postgres", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err = db.Exec(`CREATE TABLE users(id BIGINT PRIMARY KEY,username TEXT,email TEXT,role TEXT,status TEXT,deleted_at TIMESTAMPTZ);CREATE TABLE api_keys(id BIGINT PRIMARY KEY);INSERT INTO users VALUES(1,'管理员','a@example.invalid','admin','active',NULL);INSERT INTO api_keys VALUES(1);`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"249_conversation_archives.sql", "250_conversation_archive_blocks.sql", "251_conversation_archive_devices.sql", "251_conversation_archive_devices.sql"} {
		raw, err := os.ReadFile("../../migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	store := New(db, Options{Enabled: true, Permanent: true, Deduplicate: true})
	ctx := context.Background()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	device, err := store.PairLocalDevice(ctx, 1, "My Windows", string(ssh.MarshalAuthorizedKey(key)))
	if err != nil || device.OffloadEnabled {
		t.Fatal("pair must default to copy only", err)
	}
	if _, err = store.UpdateLocalDevice(ctx, device.ID, true, true, 7); err == nil {
		t.Fatal("migration enabled before first receipt")
	}
	uid := int64(1)
	record := Record{RequestID: "old", UserID: &uid, UserName: "张三", Model: "requested", Endpoint: "/v1/chat/completions", HTTPStatus: 200, Status: "complete", CreatedAt: time.Now().Add(-9 * 24 * time.Hour), RequestBody: json.RawMessage(`{"model":"requested","messages":[{"role":"user","content":"` + strings.Repeat("same context ", 100) + `","n":9007199254740993}]}`), ResponseBody: json.RawMessage(`{"choices":[{"message":{"content":"older answer"}}],"_sub2api_archive":{"upstream_model":"forwarded","response_model":"provider","wire_model":"client","source":"upstream"}}`)}
	if err = store.Save(ctx, &record); err != nil {
		t.Fatal(err)
	}
	record.RequestID = "recent"
	record.CreatedAt = time.Now()
	record.ResponseBody = json.RawMessage(`{"choices":[{"message":{"content":"recent answer"}}]}`)
	if err = store.Save(ctx, &record); err != nil {
		t.Fatal(err)
	}
	items, err := store.List(ctx, Filter{Limit: 10})
	if err != nil || len(items) != 2 {
		t.Fatal(err)
	}
	oldID, recentID := items[1].ID, items[0].ID
	entry, err := store.localEntry(ctx, oldID)
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := entry.Digest()
	refs, _ := entry.Refs()
	blocks, err := store.localBlocks(ctx, refs)
	if err != nil || len(blocks.Blocks) == 0 {
		t.Fatal("export missing blocks", err)
	}
	if _, err = store.localAck(ctx, device, archivewire.Request{ID: oldID, Digest: strings.Repeat("0", 64), Confirmed: 1}); err == nil {
		t.Fatal("wrong receipt accepted")
	}
	receipt, err := store.localAck(ctx, device, archivewire.Request{ID: oldID, Digest: digest, Confirmed: 1})
	if err != nil || receipt.Moved {
		t.Fatal("copy-only receipt migrated body", err)
	}
	if _, err = store.UpdateLocalDevice(ctx, device.ID, true, true, 7); err != nil {
		t.Fatal(err)
	}
	receipt, err = store.localAck(ctx, device, archivewire.Request{ID: oldID, Digest: digest, Confirmed: 1})
	if err != nil || !receipt.Moved {
		t.Fatal("valid old record not moved", err)
	}
	saved, err := store.Get(ctx, oldID)
	if err != nil || saved.LocalArchive == nil || len(saved.RequestBody) != 0 || saved.UpstreamModel != "forwarded" || saved.ResponseModel != "provider" || saved.PromptPreview == "" {
		t.Fatal("local stub lost provenance or misrepresented context", err)
	}
	again, err := store.localAck(ctx, device, archivewire.Request{ID: oldID, Digest: digest, Confirmed: 1})
	if err != nil || !again.Moved {
		t.Fatal("idempotent receipt failed", err)
	}
	recent, err := store.localEntry(ctx, recentID)
	if err != nil {
		t.Fatal(err)
	}
	recentDigest, _ := recent.Digest()
	receipt, err = store.localAck(ctx, device, archivewire.Request{ID: recentID, Digest: recentDigest, Confirmed: 2})
	if err != nil || receipt.Moved {
		t.Fatal("recent body was migrated", err)
	}
	if err = store.PurgeExpired(ctx); err != nil {
		t.Fatal(err)
	}
	// Transfer leases survive a concurrent record deletion / offload, then expire.
	var held int
	if err = db.QueryRow(`SELECT count(*) FROM conversation_archive_blocks WHERE last_checked_at>NOW()`).Scan(&held); err != nil || held == 0 {
		t.Fatal("transfer holds missing", err)
	}
	if _, err = db.Exec(`UPDATE conversation_archive_blocks SET last_checked_at=NOW()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	if err = store.PurgeExpired(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, recentID)
	if err != nil || !strings.Contains(string(got.RequestBody), "9007199254740993") {
		t.Fatal("GC deleted shared context", err)
	}
	manifest, err := store.localList(ctx, archivewire.Request{})
	if err != nil || manifest.Items[0].Digest != digest {
		t.Fatal("local location missing from manifest", err)
	}
	// Revocation takes effect on every RPC, even with a still-open SSH session.
	if _, err = store.UpdateLocalDevice(ctx, device.ID, false, false, 7); err != nil {
		t.Fatal(err)
	}
	var input, output bytes.Buffer
	if err = archivewire.Write(&input, archivewire.Request{Operation: "list"}, archivewire.MaxRequest); err != nil {
		t.Fatal(err)
	}
	if err = store.ServeLocalTransfer(ctx, device.ID, &input, &output); err != nil {
		t.Fatal(err)
	}
	var response archivewire.Response
	if err = archivewire.Read(&output, &response, archivewire.MaxResponse); err != nil || response.Error == "" || len(response.Items) > 0 {
		t.Fatal("revoked device read data", err)
	}
	if _, err = store.UpdateLocalDevice(ctx, device.ID, true, false, 7); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE users SET role='user'`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.AuthorizedLocalDevice(ctx, device.ID); err == nil {
		t.Fatal("demoted owner retained archive access")
	}
	keys, err := store.AuthorizedLocalKeys(ctx)
	if err != nil || len(keys) != 0 {
		t.Fatal("demoted owner still published to SSH", err)
	}
}
