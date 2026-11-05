package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/keywordrules"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type wsKeywordStore map[string]string

func (s wsKeywordStore) GetMultiple(_ context.Context, _ []string) (map[string]string, error) {
	return s, nil
}
func (s wsKeywordStore) Set(_ context.Context, key, value string) error {
	s[key] = value
	return nil
}

func TestKeywordRulesWebSocketInterceptionAndInjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, action := range []string{"reply", "block", "inject"} {
		t.Run(action, func(t *testing.T) {
			raw, err := json.Marshal(keywordrules.Policy{Enabled: true, Rules: []keywordrules.Rule{{ID: "ws-rule", Name: "WS rule", Enabled: true, Keywords: []string{"EXACT"}, Action: action, Reply: "Local answer", Injection: "System instruction"}}})
			require.NoError(t, err)
			manager := keywordrules.NewManager(wsKeywordStore{keywordrules.SettingKey: string(raw)})
			finished := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, acceptErr := coderws.Accept(w, r, nil)
				if acceptErr != nil {
					finished <- acceptErr
					return
				}
				defer func() { _ = conn.CloseNow() }()
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = r
				c.Set(keywordrules.ContextManagerKey, manager)
				group := int64(1)
				body, applyErr := applyWSKeywordRules(c, conn, &service.APIKey{UserID: 1, GroupID: &group}, "alias", []byte(`{"type":"response.create","model":"alias","input":"EXACT"}`))
				if applyErr == nil {
					applyErr = conn.Write(r.Context(), coderws.MessageText, body)
				}
				finished <- applyErr
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			defer func() { _ = conn.CloseNow() }()
			var frames []byte
			for {
				_, frame, readErr := conn.Read(ctx)
				if readErr != nil {
					break
				}
				frames = append(frames, frame...)
			}
			switch action {
			case "reply":
				require.Contains(t, string(frames), "Local answer")
				require.Contains(t, string(frames), "response.completed")
				require.True(t, errors.Is(<-finished, keywordrules.ErrIntercepted))
			case "block":
				require.Contains(t, string(frames), "content_policy_violation")
				require.True(t, errors.Is(<-finished, keywordrules.ErrIntercepted))
			case "inject":
				require.NoError(t, <-finished)
				require.Equal(t, "System instruction", gjson.GetBytes(frames, "instructions").String())
				require.Equal(t, "EXACT", gjson.GetBytes(frames, "input").String())
			}
		})
	}
}
