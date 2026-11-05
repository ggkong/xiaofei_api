package routes

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/conversationarchive"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

// An isolated mock upstream + real PostgreSQL exercise the HTTP capture/read
// boundary. Test credentials are accepted only by this in-process test router.
func TestConversationArchiveHTTPPostgresLifecycle(t *testing.T) {
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
	if err = base.QueryRow("SELECT current_database()").Scan(&database); err != nil || database != "archive_test" {
		t.Fatal("refusing to run outside archive_test")
	}
	schema := "archive_http_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err = base.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Scheme != "postgres" {
		t.Fatal("test requires a postgres:// URL")
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := sql.Open("postgres", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close(); _, _ = base.Exec("DROP SCHEMA " + schema + " CASCADE") }()
	if _, err = db.Exec("CREATE TABLE users(id BIGINT PRIMARY KEY,username TEXT NOT NULL DEFAULT '',email TEXT NOT NULL DEFAULT ''); CREATE TABLE api_keys(id BIGINT PRIMARY KEY); INSERT INTO users(id,username) VALUES(42,'Archive member'); INSERT INTO api_keys VALUES(7)"); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../../migrations/249_conversation_archives.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	blocksMigration, err := os.ReadFile("../../../migrations/250_conversation_archive_blocks.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(blocksMigration)); err != nil {
		t.Fatal(err)
	}
	store := conversationarchive.New(db, conversationarchive.Options{Enabled: true, MaxBytes: 4096, RetentionDays: 30})
	chunk := "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}]}\n\n"
	reply := strings.Repeat(chunk, 100) + "data: [DONE]\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("upstream does not support streaming")
			return
		}
		for _, event := range strings.SplitAfter(reply, "\n\n") {
			if _, err := io.WriteString(w, event); err != nil {
				t.Error(err)
				return
			}
			flusher.Flush()
		}
	}))
	defer upstream.Close()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 7, UserID: 42})
		c.Next()
	}, middleware.ConversationArchive(store), func(c *gin.Context) {
		req, err := http.NewRequestWithContext(c.Request.Context(), "POST", upstream.URL, c.Request.Body)
		if err != nil {
			t.Error(err)
			c.Status(500)
			return
		}
		result, err := upstream.Client().Do(req)
		if err != nil {
			t.Error(err)
			c.Status(502)
			return
		}
		defer func() { _ = result.Body.Close() }()
		c.Header("Content-Type", result.Header.Get("Content-Type"))
		buf := make([]byte, 73)
		for {
			n, readErr := result.Body.Read(buf)
			if n > 0 {
				if _, err := c.Writer.Write(buf[:n]); err != nil {
					t.Error(err)
					return
				}
				c.Writer.Flush()
			}
			if readErr != nil {
				if readErr != io.EOF {
					t.Error(readErr)
				}
				break
			}
		}
	})
	admin := router.Group("/api/v1/admin", func(c *gin.Context) {
		switch c.GetHeader("Authorization") {
		case "Bearer test-admin":
			c.Set(string(middleware.ContextKeyUserRole), "admin")
			c.Set("auth_method", "jwt")
		case "Bearer test-member":
			c.Set(string(middleware.ContextKeyUserRole), "user")
			c.Set("auth_method", "jwt")
		}
		c.Next()
	})
	registerConversationArchiveRoutes(admin, store)
	server := httptest.NewServer(router)
	defer server.Close()
	req, err := http.NewRequest("POST", server.URL+"/v1/chat/completions", strings.NewReader(`{"model":"mock-model","messages":[{"role":"user","content":"question"}],"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("session-id", "http-session")
	client := &http.Client{Timeout: 15 * time.Second}
	result, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(result.Body)
	_ = result.Body.Close()
	if err != nil || result.StatusCode != 200 || string(body) != reply {
		t.Fatal("HTTP relay changed the streamed reply")
	}
	// The final archive write follows the last stream flush. Synchronize without
	// assuming that reading the terminal frame means the database commit is done.
	var records []conversationarchive.Record
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		records, err = store.List(context.Background(), conversationarchive.Filter{UserID: 42, APIKeyID: 7, SessionID: "http-session"})
		if err == nil && len(records) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil || len(records) != 1 || records[0].Status != "complete" || !records[0].Truncated {
		t.Fatalf("archive state incorrect: %v", err)
	}
	id := records[0].ID
	for _, tc := range []struct {
		token  string
		status int
	}{{"", 401}, {"test-member", 403}, {"test-admin", 200}} {
		request, _ := http.NewRequest("GET", server.URL+"/api/v1/admin/conversation-archives", nil)
		if tc.token != "" {
			request.Header.Set("Authorization", "Bearer "+tc.token)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != tc.status {
			t.Fatalf("%s: got %d want %d", tc.token, response.StatusCode, tc.status)
		}
	}
	detailPath := server.URL + "/api/v1/admin/conversation-archives/" + strconv.FormatInt(id, 10)
	request, _ := http.NewRequest("GET", detailPath, nil)
	request.Header.Set("Authorization", "Bearer test-admin")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data conversationarchive.Record `json:"data"`
	}
	if err = json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 200 || response.Header.Get("Cache-Control") != "no-store" || !strings.Contains(string(envelope.Data.RequestBody), "question") {
		t.Fatal("administrator detail read failed")
	}
	request, _ = http.NewRequest("DELETE", detailPath, nil)
	request.Header.Set("Authorization", "Bearer test-member")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("ordinary user could delete records")
	}
	request, _ = http.NewRequest("DELETE", detailPath, nil)
	request.Header.Set("Authorization", "Bearer test-admin")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("administrator delete failed")
	}
	if _, err = store.Get(context.Background(), id); err != sql.ErrNoRows {
		t.Fatal("deleted record remained readable")
	}
}
