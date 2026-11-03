package routes

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

func TestConversationArchiveRequiresAdministratorBrowserJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		role, method string
		status       int
	}{
		{"user", "jwt", 403}, {"admin", "admin_api_key", 403}, {"", "jwt", 403}, {"admin", "jwt", 503},
	} {
		t.Run(tc.role+"_"+tc.method, func(t *testing.T) {
			for _, path := range []string{"/api/v1/admin/conversation-archives", "/api/v1/admin/conversation-archives/1"} {
				r := gin.New()
				g := r.Group("/api/v1/admin", func(c *gin.Context) {
					c.Set(string(middleware.ContextKeyUserRole), tc.role)
					c.Set("auth_method", tc.method)
					c.Next()
				})
				registerConversationArchiveRoutes(g, nil)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
				if w.Code != tc.status {
					t.Fatalf("%s: got %d want %d", path, w.Code, tc.status)
				}
			}
		})
	}
}
