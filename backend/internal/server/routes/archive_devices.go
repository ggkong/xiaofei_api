package routes

import (
	"context"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/conversationarchive"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

func registerArchiveDeviceRoutes(archive *gin.RouterGroup, store *conversationarchive.Store) {
	archive.GET("/devices", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		devices, err := store.LocalDevices(ctx)
		if err != nil {
			response.Error(c, 503, "Local archive devices unavailable")
			return
		}
		response.Success(c, devices)
	})
	archive.POST("/devices", func(c *gin.Context) {
		subject, ok := middleware.GetAuthSubjectFromContext(c)
		if !ok || subject.UserID < 1 {
			response.Forbidden(c, "Administrator browser login required")
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
		var body struct {
			Name      string `json:"name"`
			PublicKey string `json:"public_key"`
		}
		if c.ShouldBindJSON(&body) != nil {
			response.BadRequest(c, "Invalid device")
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		device, err := store.PairLocalDevice(ctx, subject.UserID, body.Name, body.PublicKey)
		if err != nil {
			response.BadRequest(c, "Use a name and the ed25519 public key generated on your computer")
			return
		}
		response.Success(c, device)
	})
	archive.PUT("/devices/:device", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
		var body struct {
			Enabled  *bool `json:"enabled"`
			Offload  *bool `json:"offload_enabled"`
			KeepDays int   `json:"keep_days"`
		}
		if c.ShouldBindJSON(&body) != nil || body.Enabled == nil || body.Offload == nil {
			response.BadRequest(c, "Invalid device settings")
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		device, err := store.UpdateLocalDevice(ctx, c.Param("device"), *body.Enabled, *body.Offload, body.KeepDays)
		if err != nil {
			response.BadRequest(c, "Device unavailable or invalid retention; use 1–3650 days")
			return
		}
		response.Success(c, device)
	})
}
