package routes

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/conversationarchive"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

func firstArchive(stores []*conversationarchive.Store) *conversationarchive.Store {
	if len(stores) > 0 {
		return stores[0]
	}
	return nil
}

func registerConversationArchiveRoutes(admin *gin.RouterGroup, store *conversationarchive.Store) {
	archive := admin.Group("/conversation-archives")
	// Defense in depth: only an administrator's interactive JWT, never member or scoped API keys.
	archive.Use(middleware.AdminOnly(), func(c *gin.Context) {
		if c.GetString("auth_method") != "jwt" || adminaccess.FromContext(c.Request.Context()) != nil {
			c.Abort()
			response.Forbidden(c, "Administrator browser login required")
			return
		}
		c.Next()
	})
	archive.GET("/settings", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		info, err := store.StorageInfo(ctx)
		if err != nil {
			response.Error(c, 503, "Archive settings unavailable")
			return
		}
		c.Header("Cache-Control", "no-store")
		response.Success(c, info)
	})
	archive.PUT("/settings", func(c *gin.Context) {
		var policy conversationarchive.Policy
		if c.ShouldBindJSON(&policy) != nil {
			response.BadRequest(c, "Invalid archive settings")
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		if err := store.UpdatePolicy(ctx, policy); err != nil {
			response.BadRequest(c, "Use 0–3650 retention days and a context limit between 4 KiB and 16 MiB")
			return
		}
		response.Success(c, policy)
	})
	archive.POST("/compact", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		count, err := store.CompactExisting(ctx, 10)
		if err != nil {
			response.Error(c, 503, "Archive optimization unavailable")
			return
		}
		response.Success(c, gin.H{"compacted": count})
	})
	listArchive := func(c *gin.Context) {
		var f conversationarchive.Filter
		for key, target := range map[string]*int64{"user_id": &f.UserID, "api_key_id": &f.APIKeyID, "before_id": &f.BeforeID} {
			if value := c.Query(key); value != "" {
				n, err := strconv.ParseInt(value, 10, 64)
				if err != nil || n < 1 {
					response.BadRequest(c, "Invalid "+key)
					return
				}
				*target = n
			}
		}
		f.Model = c.Query("model")
		f.Query = strings.TrimSpace(c.Query("q"))
		if len([]rune(f.Query)) > 200 || !utf8.ValidString(f.Query) || strings.ContainsRune(f.Query, 0) {
			response.BadRequest(c, "Invalid search query")
			return
		}
		for name, target := range map[string]**time.Time{"created_from": &f.CreatedFrom, "created_until": &f.CreatedUntil} {
			if value := c.Query(name); value != "" {
				timestamp, err := time.Parse(time.RFC3339Nano, value)
				if err != nil || timestamp.Year() < 1 {
					response.BadRequest(c, "Invalid "+name+"; use RFC3339 with a timezone")
					return
				}
				*target = &timestamp
			}
		}
		if f.CreatedFrom != nil && f.CreatedUntil != nil && !f.CreatedFrom.Before(*f.CreatedUntil) {
			response.BadRequest(c, "created_until must be after created_from")
			return
		}
		f.ConversationKey = c.Query("conversation_key")
		if key := f.ConversationKey; key != "" {
			valid := len(key) == 64 && strings.Trim(key, "0123456789abcdef") == ""
			if strings.HasPrefix(key, "record:") {
				_, err := strconv.ParseInt(strings.TrimPrefix(key, "record:"), 10, 64)
				valid = err == nil
			}
			if !valid {
				response.BadRequest(c, "Invalid conversation group")
				return
			}
		}
		f.SessionID = c.Query("session_id")
		f.Status = c.Query("status")
		if len([]rune(f.Model)) > 255 || len([]rune(f.SessionID)) > 255 || !utf8.ValidString(f.Model+f.SessionID) || strings.ContainsRune(f.Model+f.SessionID, 0) {
			response.BadRequest(c, "Invalid model or session filter")
			return
		}
		if f.Status != "" && f.Status != "complete" && f.Status != "error" && f.Status != "interrupted" && f.Status != "blocked" && f.Status != "auto_reply" {
			response.BadRequest(c, "Invalid status")
			return
		}
		f.Limit = 30
		if value := c.Query("limit"); value != "" {
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 || n > 100 {
				response.BadRequest(c, "Invalid limit")
				return
			}
			f.Limit = n
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		var items []conversationarchive.Record
		var err error
		if strings.HasSuffix(c.FullPath(), "/groups") {
			items, err = store.ListGroups(ctx, f)
		} else {
			items, err = store.List(ctx, f)
		}
		if err != nil {
			response.Error(c, 503, "Conversation archive unavailable")
			return
		}
		var next int64
		if len(items) == f.Limit {
			next = items[len(items)-1].ID
		}
		c.Header("Cache-Control", "no-store")
		response.Success(c, gin.H{"items": items, "next_before_id": next, "capture_enabled": store.Enabled(), "capture_paused": store.CapturePaused(), "retention_days": store.Policy(ctx).RetentionDays})
	}
	archive.GET("", listArchive)
	archive.GET("/groups", listArchive)
	archive.GET("/:id", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id < 1 {
			response.BadRequest(c, "Invalid record ID")
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		item, err := store.Get(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			response.NotFound(c, "Record not found")
			return
		}
		if err != nil {
			response.Error(c, 503, "Conversation archive unavailable")
			return
		}
		c.Header("Cache-Control", "no-store")
		response.Success(c, item)
	})
	archive.DELETE("/:id", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id < 1 {
			response.BadRequest(c, "Invalid record ID")
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		found, err := store.Delete(ctx, id)
		if err != nil {
			response.Error(c, 503, "Conversation archive unavailable")
			return
		}
		if !found {
			response.NotFound(c, "Record not found")
			return
		}
		response.Success(c, gin.H{"deleted": true})
	})
}
