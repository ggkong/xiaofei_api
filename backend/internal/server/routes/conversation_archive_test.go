package routes

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

func TestConversationArchiveRequiresAdministratorBrowserJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		role, method string
		status       int
		scoped       bool
	}{
		{"user", "jwt", 403, false}, {"admin", "admin_api_key", 403, false}, {"", "jwt", 403, false}, {"admin", "jwt", 503, false},
		{"admin", "jwt", 403, true}, {"missing", "", 401, false},
	} {
		t.Run(tc.role+"_"+tc.method, func(t *testing.T) {
			for _, endpoint := range []struct{ method, path string }{{"GET", "/api/v1/admin/conversation-archives"}, {"GET", "/api/v1/admin/conversation-archives/1"}, {"DELETE", "/api/v1/admin/conversation-archives/1"}} {
				r := gin.New()
				g := r.Group("/api/v1/admin", func(c *gin.Context) {
					if tc.role != "missing" {
						c.Set(string(middleware.ContextKeyUserRole), tc.role)
					}
					c.Set("auth_method", tc.method)
					if tc.scoped {
						c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{KeyID: "test-key"}))
					}
					c.Next()
				})
				registerConversationArchiveRoutes(g, nil)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(endpoint.method, endpoint.path, nil))
				if w.Code != tc.status {
					t.Fatalf("%s %s: got %d want %d", endpoint.method, endpoint.path, w.Code, tc.status)
				}
			}
		})
	}
}

func TestConversationArchiveValidatesTimeRangesBeforeDatabaseAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, values := range []url.Values{
		{"created_from": {"yesterday"}},
		{"created_until": {"2026-11-04T00:00:00"}},
		{"created_from": {"0000-01-01T00:00:00Z"}},
		{"created_from": {"2026-11-05T00:00:00Z"}, "created_until": {"2026-11-04T00:00:00Z"}},
		{"created_from": {"2026-11-04T00:00:00Z"}, "created_until": {"2026-11-04T00:00:00Z"}},
		{"session_id": {"bad\x00value"}},
	} {
		r := gin.New()
		g := r.Group("/api/v1/admin", func(c *gin.Context) {
			c.Set(string(middleware.ContextKeyUserRole), "admin")
			c.Set("auth_method", "jwt")
			c.Next()
		})
		registerConversationArchiveRoutes(g, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/admin/conversation-archives?"+values.Encode(), nil))
		if w.Code != 400 {
			t.Fatalf("filter %v: got %d", values, w.Code)
		}
	}
}
