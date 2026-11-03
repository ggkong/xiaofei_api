package middleware

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/conversationarchive"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type archivePayloadContains string

func (m archivePayloadContains) Match(value driver.Value) bool {
	text, ok := value.(string)
	return ok && strings.Contains(text, string(m))
}

type archiveJSONMatches func(map[string]any) bool

func (m archiveJSONMatches) Match(value driver.Value) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	var payload map[string]any
	return json.Unmarshal([]byte(text), &payload) == nil && m(payload)
}

func TestConversationCaptureTracksTerminalsPastLimitAndClientCancellation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	chunk := "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"
	for _, tc := range []struct {
		name, contentType, body, status string
		truncated, cancelled            bool
	}{
		{"long-complete", "text/event-stream", strings.Repeat(chunk, 100) + "data: [DONE]\n\n", "complete", true, false},
		{"long-error", "text/event-stream", strings.Repeat(chunk, 100) + "data: {\"type\":\"error\",\"error\":{\"message\":\"failed\"}}\n\ndata: [DONE]\n\n", "error", true, false},
		{"interrupted", "text/event-stream", chunk, "interrupted", false, false},
		{"cancelled", "text/event-stream", chunk, "interrupted", false, true},
		{"json-error", "application/json", `{"error":{"message":"failed"}}`, "error", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store := conversationarchive.New(db, conversationarchive.Options{Enabled: true, MaxBytes: 4096, RetentionDays: 30})
			isStream := tc.contentType == "text/event-stream"
			mock.ExpectExec("INSERT INTO conversation_archives").WithArgs(sqlmock.AnyArg(), int64(42), int64(7), "", "/v1/chat/completions", "test", isStream, 200, tc.status,
				archivePayloadContains("question"), archiveJSONMatches(func(payload map[string]any) bool {
					if isStream {
						return payload["complete"] == (tc.status == "complete") && payload["truncated"] == tc.truncated
					}
					return payload["error"] != nil
				}), tc.truncated, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
			r := gin.New()
			r.Use(func(c *gin.Context) { c.Set(string(ContextKeyAPIKey), &service.APIKey{ID: 7, UserID: 42}); c.Next() }, ConversationArchive(store))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r.POST("/v1/chat/completions", func(c *gin.Context) {
				io.ReadAll(c.Request.Body)
				c.Header("Content-Type", tc.contentType)
				for i := 0; i < len(tc.body); i += 7 {
					end := i + 7
					if end > len(tc.body) {
						end = len(tc.body)
					}
					c.Writer.WriteString(tc.body[i:end])
				}
				if tc.cancelled {
					cancel()
				}
			})
			req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"test","messages":[{"role":"user","content":"question"}]}`)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != 200 || w.Body.String() != tc.body {
				t.Fatal("archive changed client-visible bytes")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestConversationCapturePreservesStreamingAndSurvivesStorageFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, failure := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		store := conversationarchive.New(db, conversationarchive.Options{Enabled: true, MaxBytes: 4096, RetentionDays: 30})
		expectation := mock.ExpectExec("INSERT INTO conversation_archives").WithArgs(
			sqlmock.AnyArg(), int64(42), int64(7), "session-test", "/v1/chat/completions", "test", true, 200, "complete",
			archivePayloadContains("question"), archivePayloadContains("hello"), false, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		)
		if failure {
			expectation.WillReturnError(errors.New("storage unavailable"))
		} else {
			expectation.WillReturnResult(sqlmock.NewResult(1, 1))
		}
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set(string(ContextKeyAPIKey), &service.APIKey{ID: 7, UserID: 42}); c.Next() })
		r.Use(ConversationArchive(store))
		w := httptest.NewRecorder()
		const first = "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"
		const last = "data: [DONE]\n\n"
		r.POST("/v1/chat/completions", func(c *gin.Context) {
			body, err := io.ReadAll(c.Request.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(body, []byte("question")) {
				t.Fatal("request body changed")
			}
			c.Header("Content-Type", "text/event-stream")
			_, _ = c.Writer.WriteString(first[:15])
			_, _ = io.Copy(c.Writer, strings.NewReader(first[15:]))
			c.Writer.Flush()
			if w.Body.String() != first || !w.Flushed {
				t.Fatal("stream was buffered instead of immediately forwarded")
			}
			_, _ = c.Writer.WriteString(last)
		})
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"test","messages":[{"role":"user","content":"question"}],"stream":true}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("session-id", "session-test")
		r.ServeHTTP(w, req)
		if w.Code != 200 || w.Body.String() != first+last {
			t.Fatal("client response changed")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
}

func TestCaptureBoundsMemoryWithoutChangingForwardedBytes(t *testing.T) {
	b := &archiveBuffer{limit: 4}
	n, err := b.Write([]byte("abcdefgh"))
	if n != 8 || err != nil || b.String() != "abcd" || !b.truncated {
		t.Fatal("capture must bound retained bytes without short-writing")
	}
	_, _ = b.Write([]byte("more"))
	if b.Len() != 4 {
		t.Fatal("buffer grew past limit")
	}
	if archiveEndpoint("/v1/messages/count_tokens") || archiveEndpoint("/v1/images/generations") {
		t.Fatal("non-dialogue endpoint captured")
	}
}
