package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/conversationarchive"
	"github.com/Wei-Shaw/sub2api/internal/keywordrules"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type archiveBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *archiveBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.limit - b.Len()
	if n > remaining {
		b.truncated = true
		p = p[:remaining]
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

type archiveBody struct {
	io.Reader
	original io.ReadCloser
}

func (b archiveBody) Close() error { return b.original.Close() }

type archiveWriter struct {
	gin.ResponseWriter
	mu          sync.Mutex
	captured    *archiveBuffer
	writeError  bool
	streamState conversationarchive.StreamState
}

func (w *archiveWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.ResponseWriter.Write(p)
	_, _ = w.captured.Write(p[:n])
	if strings.Contains(strings.ToLower(w.Header().Get("Content-Type")), "text/event-stream") {
		_, _ = w.streamState.Write(p[:n])
	}
	if err != nil {
		w.writeError = true
	}
	return n, err
}

func (w *archiveWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ResponseWriter.Flush()
}

func (w *archiveWriter) snapshot() ([]byte, bool, bool, bool, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return bytes.Clone(w.captured.Bytes()), w.captured.truncated, w.writeError, w.streamState.Complete(), w.streamState.Failed()
}
func (w *archiveWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// Prevent io.Copy from bypassing capture through an embedded ReaderFrom method.
func (w *archiveWriter) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(struct{ io.Writer }{w}, r)
}

func archiveEndpoint(path string) bool {
	return strings.HasSuffix(path, "/chat/completions") || strings.HasSuffix(path, "/responses") || strings.HasSuffix(path, "/messages")
}

// ConversationArchive must run after API-key authentication and before forwarding.
// It observes the client-visible protocol, so upstream retries/conversions create
// one snapshot, rather than duplicate messages. GET/WebSocket/media routes bypass it.
func ConversationArchive(store *conversationarchive.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !store.Enabled() || c.Request.Method != http.MethodPost || !archiveEndpoint(c.Request.URL.Path) || !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
			c.Next()
			return
		}
		key, ok := GetAPIKeyFromContext(c)
		if !ok || key == nil || key.UserID <= 0 || key.ID <= 0 || c.Request.Body == nil {
			c.Next()
			return
		}
		userID, keyID := key.UserID, key.ID
		started := time.Now()
		limit := store.MaxBytes()
		request := &archiveBuffer{limit: limit}
		response := &archiveBuffer{limit: limit}
		originalRequest := c.Request
		var requestSnapshot json.RawMessage
		var promptHash string
		c.Request = c.Request.WithContext(conversationarchive.WithRequestCapture(c.Request.Context(), func(body []byte) {
			if requestSnapshot == nil {
				promptHash = conversationarchive.PromptDigest(body)
				requestSnapshot = conversationarchive.RequestSnapshot(body, limit)
			}
		}))
		originalBody := c.Request.Body
		c.Request.Body = archiveBody{Reader: io.TeeReader(originalBody, request), original: originalBody}
		originalWriter := c.Writer
		writer := &archiveWriter{ResponseWriter: originalWriter, captured: response}
		c.Writer = writer
		returned := false
		defer func() {
			defer func() { c.Request = originalRequest }()
			c.Writer = originalWriter
			c.Request.Body = originalBody
			responseBytes, responseTruncated, writeError, streamComplete, streamFailed := writer.snapshot()
			req := conversationarchive.Snapshot(request.Bytes(), request.truncated)
			if requestSnapshot != nil {
				req = requestSnapshot
			}
			requestTruncated := request.truncated
			if effective, ok := c.Get(keywordrules.ContextEffectiveBodyKey); ok {
				if data, valid := effective.([]byte); valid {
					req = conversationarchive.RequestSnapshot(data, limit)
					requestTruncated = requestTruncated || len(data) > limit
				}
			}
			var fields struct {
				Model  string `json:"model"`
				Stream bool   `json:"stream"`
			}
			_ = json.Unmarshal(req, &fields)
			if public, ok := service.RequestedPublicModelFromContext(c.Request.Context()); ok {
				fields.Model = public
			}
			if !utf8.ValidString(fields.Model) {
				fields.Model = ""
			}
			fields.Model = strings.ReplaceAll(fields.Model, "\x00", "")
			if len([]rune(fields.Model)) > 255 {
				fields.Model = string([]rune(fields.Model)[:255])
			}
			stream := strings.Contains(strings.ToLower(writer.Header().Get("Content-Type")), "text/event-stream")
			output := conversationarchive.Snapshot(responseBytes, responseTruncated)
			status := "complete"
			if stream {
				output = conversationarchive.StreamSnapshotWithCompletion(responseBytes, responseTruncated, streamComplete)
				if streamFailed {
					status = "error"
				} else if !streamComplete {
					status = "interrupted"
				}
			}
			if !returned || writeError || c.Request.Context().Err() != nil {
				status = "interrupted"
			}
			if writer.Header().Get("X-Sub2API-Keyword-Reply") == "true" {
				output = conversationarchive.MarkKeywordReply(output)
			}
			httpStatus := writer.Status()
			if !returned && httpStatus < 400 {
				httpStatus = 500
			}
			if httpStatus >= 400 || (stream && streamFailed) || (!stream && conversationarchive.ResponseHasError(responseBytes)) {
				status = "error"
			}
			upstream, reported, conflict := service.ArchiveModelObservation(c)
			source := "upstream"
			if status == "error" && upstream == "" {
				source = "error"
			}
			if conversationarchive.BlockedResponse(output) {
				source = "blocked"
			}
			var hits []keywordrules.Hit
			if value, ok := c.Get(keywordrules.ContextHitsKey); ok {
				hits, _ = value.([]keywordrules.Hit)
			}
			output = conversationarchive.AnnotateResponse(output, conversationarchive.ModelMetadata{KeywordRules: hits, UpstreamModel: upstream, ResponseModel: reported, Conflict: conflict, Source: source}, writer.Header().Get("X-Sub2API-Keyword-Reply") == "true")
			r := conversationarchive.Record{RequestID: uuid.NewString(), UserID: &userID, APIKeyID: &keyID,
				SessionID: service.ExtractClientSessionID(c), Endpoint: c.Request.URL.Path, Model: fields.Model,
				Stream: stream || fields.Stream, HTTPStatus: httpStatus, Status: status, RequestBody: req, ResponseBody: output,
				Truncated: requestTruncated || responseTruncated, DurationMS: time.Since(started).Milliseconds(), CreatedAt: started}
			r.PromptHash = promptHash
			if key.User != nil {
				r.UserName, r.UserEmail = key.User.Username, key.User.Email
			}
			if err := store.Save(context.Background(), &r); err != nil {
				// No payload, DSN, or driver error text in operational logs.
				slog.Error("conversation archive save failed", "request_id", r.RequestID)
			}
		}()
		c.Next()
		returned = true
	}
}
