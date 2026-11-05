package routes

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestKeywordRuleRoutesRequireAdminBrowserJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, method := range []string{"GET", "PUT", "POST"} {
		for _, auth := range []string{"member", "api_key", "scoped_key", "jwt"} {
			router := gin.New()
			admin := router.Group("/api/v1/admin")
			admin.Use(func(c *gin.Context) {
				if auth == "member" {
					c.Set(string(middleware.ContextKeyUserRole), "user")
				} else {
					c.Set(string(middleware.ContextKeyUserRole), "admin")
				}
				c.Set("auth_method", auth)
				if auth == "scoped_key" {
					c.Set("auth_method", "jwt")
					c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{KeyID: "scoped"}))
				}
				c.Next()
			})
			registerKeywordRuleRoutes(admin, nil)
			path := "/api/v1/admin/keyword-rules"
			if method == "POST" {
				path += "/test"
			}
			response := httptest.NewRecorder()
			request := httptest.NewRequest(method, path, strings.NewReader(`{}`))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)
			want := 403
			if auth == "jwt" {
				want = 503
				if method == "POST" {
					want = 200
				}
			}
			require.Equal(t, want, response.Code, "%s %s: %s", method, auth, response.Body.String())
			if auth == "jwt" {
				require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			}
		}
	}
}
