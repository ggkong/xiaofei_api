package middleware

import (
	"bytes"
	"database/sql/driver"
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
