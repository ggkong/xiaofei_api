package middleware

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/keywordrules"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type keywordTestStore struct{ values map[string]string }

func (s *keywordTestStore) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := map[string]string{}
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}
func (s *keywordTestStore) Set(_ context.Context, key, value string) error {
	s.values[key] = value
	return nil
}
func keywordManager(t *testing.T, action string, enabled bool) *keywordrules.Manager {
	t.Helper()
	policy := keywordrules.Policy{Enabled: enabled, Rules: []keywordrules.Rule{{ID: "rule", Name: "Test rule", Enabled: true, Keywords: []string{"EXACT"}, MatchMode: "exact", Action: action, Reply: "Local reply", Injection: "Injected system instruction", InjectionPosition: "append", UserIDs: []int64{42}}}}
	raw, err := json.Marshal(policy)
	require.NoError(t, err)
	return keywordrules.NewManager(&keywordTestStore{map[string]string{keywordrules.SettingKey: string(raw)}})
}
func keywordRouter(manager *keywordrules.Manager, next gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := int64(3)
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyAPIKey), &service.APIKey{ID: 7, UserID: 42, GroupID: &group})
		c.Next()
	}, KeywordRules(manager, 2<<20))
	router.POST("/v1/chat/completions", next)
	return router
}
func keywordRequest(body string) *http.Request {
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}
func TestKeywordRulesReplyAndBlockDoNotReachUpstream(t *testing.T) {
	for _, action := range []string{"reply", "block"} {
		t.Run(action, func(t *testing.T) {
			forwarded := 0
			router := keywordRouter(keywordManager(t, action, true), func(c *gin.Context) { forwarded++; c.Status(599) })
			response := httptest.NewRecorder()
			router.ServeHTTP(response, keywordRequest(`{"model":"alias","messages":[{"role":"user","content":"EXACT"}]}`))
			require.Zero(t, forwarded)
			if action == "reply" {
				require.Equal(t, 200, response.Code)
				require.Equal(t, "Local reply", gjson.Get(response.Body.String(), "choices.0.message.content").String())
				require.Equal(t, "true", response.Header().Get("X-Sub2API-Keyword-Reply"))
			} else {
				require.Equal(t, 403, response.Code)
				require.Equal(t, "content_policy_violation", gjson.Get(response.Body.String(), "error.code").String())
			}
		})
	}
}
func TestKeywordRulesInjectionReachesUpstreamAndPreservesOriginalForAudit(t *testing.T) {
	var received []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		received, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write([]byte(`{"choices":[{"message":{"content":"Provider reply"}}]}`))
		require.NoError(t, err)
	}))
	defer upstream.Close()
	router := keywordRouter(keywordManager(t, "inject", true), func(c *gin.Context) {
		require.True(t, keywordrules.Managed(c.Request.Context()))
		original, ok := c.Get(keywordrules.ContextOriginalBodyKey)
		require.True(t, ok)
		originalBody, valid := original.([]byte)
		require.True(t, valid)
		require.NotContains(t, string(originalBody), "Injected system instruction")
		body, err := httputil.ReadLenientJSONRequestBodyWithPrealloc(c.Request, 2<<20)
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(c.Request.Context(), "POST", upstream.URL, strings.NewReader(string(body)))
		require.NoError(t, err)
		response, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = response.Body.Close() }()
		out, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		c.Data(response.StatusCode, "application/json", out)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, keywordRequest(`{"model":"alias","number":9007199254740993,"messages":[{"role":"system","content":"Original"},{"role":"user","content":"EXACT"}]}`))
	require.Equal(t, 200, response.Code)
	require.Contains(t, string(received), "9007199254740993")
	require.Equal(t, "Original", gjson.GetBytes(received, "messages.0.content").String())
	require.Equal(t, "Injected system instruction", gjson.GetBytes(received, "messages.1.content").String())
	require.Equal(t, "EXACT", gjson.GetBytes(received, "messages.2.content").String())
	require.Contains(t, response.Body.String(), "Provider reply")
}
func TestKeywordRulesOffAndNearMissesForwardUnchanged(t *testing.T) {
	for _, tc := range []struct {
		enabled bool
		text    string
	}{{false, "EXACT"}, {true, "exact"}, {true, "EXACT!"}} {
		forwarded := false
		router := keywordRouter(keywordManager(t, "reply", tc.enabled), func(c *gin.Context) {
			forwarded = true
			require.True(t, keywordrules.Managed(c.Request.Context()))
			body, err := io.ReadAll(c.Request.Body)
			require.NoError(t, err)
			require.NotContains(t, string(body), "Injected")
			c.Status(204)
		})
		raw, _ := json.Marshal(map[string]any{"model": "alias", "messages": []any{map[string]string{"role": "user", "content": tc.text}}})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, keywordRequest(string(raw)))
		require.True(t, forwarded)
		require.Equal(t, 204, response.Code)
	}
}

func TestKeywordRulesHandleJSONRegardlessOfContentType(t *testing.T) {
	for _, contentType := range []string{"", "text/plain", "application/json; charset=utf-8"} {
		t.Run(contentType, func(t *testing.T) {
			router := keywordRouter(keywordManager(t, "reply", true), func(c *gin.Context) { c.Status(599) })
			request := keywordRequest(`{"model":"alias","messages":[{"role":"user","content":"EXACT"}]}`)
			request.Header.Set("Content-Type", contentType)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, 200, response.Code)
			require.Equal(t, "true", response.Header().Get("X-Sub2API-Keyword-Reply"))
		})
	}
	// Leave invalid model types to normal request validation.
	router := keywordRouter(keywordManager(t, "reply", true), func(c *gin.Context) { c.Status(204) })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, keywordRequest(`{"model":123,"messages":[{"role":"user","content":"EXACT"}]}`))
	require.Equal(t, 204, response.Code)
}
