package conversationarchive

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// Uses an ephemeral dedicated database only. Never point ARCHIVE_TEST_DSN at production.
func TestStorePostgresLifecycle(t *testing.T) {
	dsn := os.Getenv("ARCHIVE_TEST_DSN")
	if dsn == "" {
		t.Skip("ARCHIVE_TEST_DSN is not configured")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var database string
	if err = db.QueryRow(`SELECT current_database()`).Scan(&database); err != nil {
		t.Fatal(err)
	}
	if database != "archive_test" {
		t.Fatal("refusing to run outside archive_test")
	}
	ctx := context.Background()
	_, err = db.Exec(`CREATE TABLE users(id BIGINT PRIMARY KEY,username TEXT NOT NULL DEFAULT '',email TEXT NOT NULL DEFAULT ''); CREATE TABLE api_keys(id BIGINT PRIMARY KEY); INSERT INTO users(id,username,email) VALUES(1,'张三','zhang@example.invalid'),(2,'李四','li@example.invalid'); INSERT INTO api_keys VALUES(11),(22);`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../migrations/249_conversation_archives.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	// Applying this additive migration twice is safe.
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	store := New(db, Options{Enabled: true, RetentionDays: 30})
	baseTime := time.Now().UTC().Truncate(time.Second)
	u, k := int64(1), int64(11)
	r := Record{RequestID: "test-1", UserID: &u, APIKeyID: &k, SessionID: "shared-session", Endpoint: "/v1/chat/completions", Model: "test-model", HTTPStatus: 200, Status: "complete", RequestBody: json.RawMessage(`{"messages":[{"role":"user","content":"hello"}]}`), ResponseBody: json.RawMessage(`{"choices":[{"message":{"content":"world"}}]}`), CreatedAt: baseTime}
	if err = store.Save(ctx, &r); err != nil {
		t.Fatal(err)
	}
	if err = store.Save(ctx, &r); err != nil {
		t.Fatal(err)
	}
	r.RequestID = "test-2"
	r.CreatedAt = baseTime.Add(time.Hour)
	u, k = 2, 22
	if err = store.Save(ctx, &r); err != nil {
		t.Fatal(err)
	}
	items, err := store.List(ctx, Filter{UserID: 1, APIKeyID: 11, SessionID: "shared-session", Limit: 30})
	if err != nil || len(items) != 1 {
		t.Fatalf("identity-scoped list: items=%d err=%v", len(items), err)
	}
	id := items[0].ID
	from := baseTime.In(time.FixedZone("UTC+8", 8*3600))
	until := baseTime.Add(time.Hour).In(time.FixedZone("UTC+8", 8*3600))
	ranged, err := store.List(ctx, Filter{CreatedFrom: &from, CreatedUntil: &until})
	if err != nil || len(ranged) != 1 || ranged[0].ID != id {
		t.Fatalf("timezone-aware inclusive/exclusive time range failed: %v", err)
	}
	page, err := store.List(ctx, Filter{Limit: 1})
	if err != nil || len(page) != 1 {
		t.Fatal("first cursor page failed")
	}
	next, err := store.List(ctx, Filter{Limit: 1, BeforeID: page[0].ID})
	if err != nil || len(next) != 1 || next[0].ID != id {
		t.Fatal("cursor pagination duplicated or lost a record")
	}
	if len(items[0].RequestBody) > 0 || len(items[0].ResponseBody) > 0 {
		t.Fatal("list leaked message bodies")
	}
	if items[0].UserName != "张三" || items[0].PromptPreview != "hello" || items[0].ResponsePreview != "world" {
		t.Fatal("list is missing the member name or conversation previews")
	}
	searched, err := store.List(ctx, Filter{Query: "张三"})
	if err != nil || len(searched) != 1 || searched[0].ID != id {
		t.Fatal("member name search failed")
	}
	detail, err := store.Get(ctx, id)
	if err != nil || !strings.Contains(string(detail.ResponseBody), "world") {
		t.Fatalf("detail failed: %v", err)
	}
	injection, err := store.List(ctx, Filter{Model: "' OR true --"})
	if err != nil || len(injection) != 0 {
		t.Fatal("model filter is not parameterized")
	}
	if _, err = db.Exec(`UPDATE conversation_archives SET expires_at=NOW()-INTERVAL '1 second' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Get(ctx, id); err != sql.ErrNoRows {
		t.Fatalf("expired record remained readable: %v", err)
	}
	if err = store.PurgeExpired(ctx); err != nil {
		t.Fatal(err)
	}
	if found, err := store.Delete(ctx, id); err != nil || found {
		t.Fatalf("expired row not purged: %v", err)
	}
	items, err = store.List(ctx, Filter{})
	if err != nil || len(items) != 1 {
		t.Fatal("other user's record was lost")
	}
	if found, err := store.Delete(ctx, items[0].ID); err != nil || !found {
		t.Fatal("delete failed")
	}
}
