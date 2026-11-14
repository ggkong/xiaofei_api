package localarchive

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/archivewire"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestViewerRequiresLocalHostCookieAndRejectsCrossSiteSync(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	entry, block := fixture(t, 1)
	ctx := context.Background()
	if err = db.SaveBlocks(ctx, []archivewire.Block{block}); err != nil {
		t.Fatal(err)
	}
	digest, _ := entry.Digest()
	if err = db.SaveEntry(ctx, entry, digest); err != nil {
		t.Fatal(err)
	}
	v := &Viewer{Database: db, ConfigDir: t.TempDir(), Host: "127.0.0.1:12345", Token: "secret"}
	handler := v.Handler()
	request := func(method, path, host, origin, cookie string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://"+host+path, nil)
		r.Host = host
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: "archive_session", Value: cookie})
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, test := range []struct{ method, path, host, origin, cookie string }{{"GET", "/api/records", v.Host, "", ""}, {"GET", "/api/records", "evil.example", "", "secret"}, {"POST", "/api/sync", v.Host, "https://evil.example", "secret"}, {"GET", "/open/incorrect", v.Host, "", ""}} {
		if got := request(test.method, test.path, test.host, test.origin, test.cookie); got.Code != 403 {
			t.Fatal("viewer boundary failed", got.Code)
		}
	}
	opened := request("GET", "/open/secret", v.Host, "", "")
	if opened.Code != 303 || len(opened.Result().Cookies()) != 1 || !opened.Result().Cookies()[0].HttpOnly {
		t.Fatal("session bootstrap failed")
	}
	got := request("GET", "/api/records/1", v.Host, "", "secret")
	if got.Code != 200 || !strings.Contains(got.Body.String(), "9007199254740993") {
		t.Fatal("lossless local detail unavailable", got.Code)
	}
	if got = request("GET", "/../../identity.key", v.Host, "", "secret"); got.Code != 404 {
		t.Fatal("unexpected filesystem path exposed")
	}
	if got = request("DELETE", "/api/records/1", v.Host, "", "secret"); got.Code != 405 {
		t.Fatal("viewer allowed mutation")
	}
	if got = request("GET", "/", v.Host, "", "secret"); got.Code != 200 || !strings.Contains(got.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatal("viewer content security missing")
	}
}
