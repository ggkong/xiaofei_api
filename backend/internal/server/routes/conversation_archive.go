package routes

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

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
	archive.GET("", func(c *gin.Context) {
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
		f.SessionID = c.Query("session_id")
		f.Status = c.Query("status")
		if len([]rune(f.Model)) > 255 || len([]rune(f.SessionID)) > 255 {
			response.BadRequest(c, "Filter too long")
			return
		}
		if f.Status != "" && f.Status != "complete" && f.Status != "error" && f.Status != "interrupted" {
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
		items, err := store.List(ctx, f)
		if err != nil {
			response.Error(c, 503, "Conversation archive unavailable")
			return
		}
		var next int64
		if len(items) == f.Limit {
			next = items[len(items)-1].ID
		}
		c.Header("Cache-Control", "no-store")
		response.Success(c, gin.H{"items": items, "next_before_id": next, "capture_enabled": store.Enabled()})
	})
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
