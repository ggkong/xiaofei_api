package conversationarchive

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strconv"
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
	blocksMigration, err := os.ReadFile("../../migrations/250_conversation_archive_blocks.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(blocksMigration)); err != nil {
		t.Fatal(err)
	}
	devicesMigration, err := os.ReadFile("../../migrations/251_conversation_archive_devices.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(devicesMigration)); err != nil {
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
	testPermanentGroupedStorage(t, db)
}

func testPermanentGroupedStorage(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.Exec(`CREATE TABLE settings(key TEXT PRIMARY KEY,value TEXT NOT NULL,updated_at TIMESTAMPTZ DEFAULT NOW())`); err != nil {
		t.Fatal(err)
	}
	store := New(db, Options{Enabled: true, Configurable: true, Deduplicate: true, RetentionDays: 30, MaxBytes: 4 << 20})
	u, k := int64(1), int64(11)
	body, _ := json.Marshal(map[string]any{"model": "alias", "large_integer": json.Number("9007199254740993"), "_refs": []string{"client-marker"}, "messages": []map[string]string{{"role": "system", "content": strings.Repeat("common context ", 1000) + "hidden-context-search"}, {"role": "user", "content": "完全相同的问题"}}})
	response := AnnotateResponse(json.RawMessage(`{"model":"client-alias","choices":[{"message":{"content":"第一次回复"}}]}`), ModelMetadata{UpstreamModel: "forwarded-model", ResponseModel: "provider-build", Source: "upstream"}, false)
	base := Record{UserID: &u, APIKeyID: &k, Model: "alias", SessionID: "session-one", Endpoint: "/v1/chat/completions", HTTPStatus: 200, Status: "complete", RequestBody: body, ResponseBody: response, UserName: "保存时姓名", UserEmail: "snapshot@example.invalid", CreatedAt: time.Now().UTC()}
	for _, id := range []string{"compressed-one", "compressed-two"} {
		r := base
		r.RequestID = id
		if err := store.Save(ctx, &r); err != nil {
			t.Fatal(err)
		}
	}
	var blocks int
	if err := db.QueryRow(`SELECT count(*) FROM conversation_archive_blocks`).Scan(&blocks); err != nil || blocks != 1 {
		t.Fatalf("shared message not deduplicated: blocks=%d err=%v", blocks, err)
	}
	if _, err := db.Exec(`UPDATE conversation_archives SET expires_at=NOW()-INTERVAL '1 day' WHERE request_id='compressed-one'`); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdatePolicy(ctx, Policy{RetentionDays: 0, MaxContextBytes: 8 << 20}); err != nil {
		t.Fatal(err)
	}
	list, err := store.List(ctx, Filter{Query: "hidden-context-search"})
	if err != nil || len(list) != 2 {
		t.Fatalf("full-context search: count=%d err=%v", len(list), err)
	}
	if store.Policy(ctx).RetentionDays != 0 {
		t.Fatal("permanent policy not persisted")
	}
	firstID := list[1].ID
	detail, err := store.Get(ctx, firstID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.ExpiresAt.Year() != 9999 || detail.UserName != "保存时姓名" || detail.UpstreamModel != "forwarded-model" || detail.ResponseModel != "provider-build" {
		t.Fatalf("identity, provenance or permanent expiry lost: %v", detail.ExpiresAt)
	}
	var want, got any
	if !decodeJSON(body, &want) || !decodeJSON(detail.RequestBody, &got) {
		t.Fatal("invalid decoded body")
	}
	normalizedWant, _ := json.Marshal(want)
	normalizedGot, _ := json.Marshal(got)
	if string(normalizedWant) != string(normalizedGot) {
		t.Fatal("roundtrip changed captured context or a large integer")
	}
	for _, model := range []string{"alias", "forwarded-model", "provider-build"} {
		matches, err := store.List(ctx, Filter{Model: model})
		if err != nil || len(matches) != 2 {
			t.Fatalf("model provenance filter %s failed: %v", model, err)
		}
	}
	groups, err := store.ListGroups(ctx, Filter{Query: "hidden-context-search"})
	if err != nil || len(groups) != 1 || groups[0].CallCount != 2 {
		t.Fatalf("session grouping failed: %v", err)
	}
	groupKey := groups[0].ConversationKey
	// Same session text from another member must remain a different group, even
	// when the compressed context block is safely reused across records.
	other := base
	other.RequestID = "other-member"
	otherUser, otherKey := int64(2), int64(22)
	other.UserID, other.APIKeyID = &otherUser, &otherKey
	if err := store.Save(ctx, &other); err != nil {
		t.Fatal(err)
	}
	groups, err = store.ListGroups(ctx, Filter{Limit: 1})
	if err != nil || len(groups) != 1 || groups[0].CallCount != 1 {
		t.Fatalf("member separation failed: %v", err)
	}
	older, err := store.ListGroups(ctx, Filter{Limit: 1, BeforeID: groups[0].ID})
	if err != nil || len(older) != 1 || older[0].CallCount != 2 {
		t.Fatalf("whole-group pagination lost calls: %v", err)
	}
	calls, err := store.List(ctx, Filter{ConversationKey: groupKey})
	if err != nil || len(calls) != 2 {
		t.Fatalf("group call inspection failed: %v", err)
	}
	// Deleting one archive cannot make another member's shared context unreadable.
	if _, err = store.Delete(ctx, firstID); err != nil {
		t.Fatal(err)
	}
	if err = store.PurgeExpired(ctx); err != nil {
		t.Fatal(err)
	}
	others, err := store.List(ctx, Filter{UserID: 2})
	if err != nil || len(others) != 1 {
		t.Fatal(err)
	}
	if _, err = store.Get(ctx, others[0].ID); err != nil {
		t.Fatalf("GC removed a shared block: %v", err)
	}
	// Expand and compact are reversible, including client-owned marker fields.
	if count, err := store.ExpandExisting(ctx, 25); err != nil || count != 2 {
		t.Fatalf("rollback expansion: count=%d err=%v", count, err)
	}
	if err = store.PurgeExpired(ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM conversation_archive_blocks`).Scan(&blocks); err != nil || blocks != 0 {
		t.Fatal("unreferenced blocks were not collected")
	}
	if count, err := store.CompactExisting(ctx, 25); err != nil || count != 2 {
		t.Fatalf("historical compaction: count=%d err=%v", count, err)
	}
	if _, err = store.Get(ctx, others[0].ID); err != nil {
		t.Fatalf("recompacted context unreadable: %v", err)
	}
	// Permanent applies to requests saved after changing the policy as well.
	for i, raw := range []string{`null`, `[1,"text",9007199254740993]`, `"scalar"`, `{"_format":1,"_refs":["not-a-storage-reference"]}`} {
		r := base
		r.RequestID = "scalar-" + strconv.Itoa(i)
		r.SessionID = ""
		r.RequestBody = json.RawMessage(raw)
		if err = store.Save(ctx, &r); err != nil {
			t.Fatal(err)
		}
		records, err := store.List(ctx, Filter{Limit: 1})
		if err != nil || len(records) != 1 {
			t.Fatal(err)
		}
		saved, err := store.Get(ctx, records[0].ID)
		if err != nil || saved.ExpiresAt.Year() != 9999 {
			t.Fatalf("new permanent record invalid: %v", err)
		}
		if !decodeJSON(saved.RequestBody, &got) || !decodeJSON([]byte(raw), &want) {
			t.Fatal("scalar decoding failed")
		}
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(want)
		if string(a) != string(b) {
			t.Fatal("client marker or scalar changed")
		}
	}
}
