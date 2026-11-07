package localarchive

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web/*
var viewerFiles embed.FS

type SyncStatus struct {
	Running   bool       `json:"running"`
	AttemptAt time.Time  `json:"attempt_at"`
	SuccessAt *time.Time `json:"success_at"`
	Progress  Progress   `json:"progress"`
	Error     string     `json:"error,omitempty"`
}

func ReadStatus(configDir string) SyncStatus {
	var s SyncStatus
	raw, err := os.ReadFile(filepath.Join(configDir, "status.json"))
	if err == nil {
		_ = json.Unmarshal(raw, &s)
	}
	if s.Running {
		if lock, lockErr := AcquireSyncLock(configDir); lockErr == nil {
			_ = lock.Close()
			s.Running = false
			s.Error = "Previous sync stopped; you can resume it"
		}
	}
	return s
}
func RunSync(ctx context.Context, configDir string, c Config, db *Database) error {
	lock, err := AcquireSyncLock(configDir)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	status := ReadStatus(configDir)
	status.Running = true
	status.Error = ""
	status.AttemptAt = time.Now()
	status.Progress = Progress{}
	save := func() {
		raw, marshalErr := json.Marshal(status)
		if marshalErr == nil {
			_ = atomicWrite(filepath.Join(configDir, "status.json"), raw)
		}
	}
	save()
	remote, err := Connect(ctx, configDir, c)
	if err == nil {
		var last time.Time
		status.Progress, err = Sync(ctx, db, remote, func(p Progress) {
			if time.Since(last) > 2*time.Second {
				status.Progress = p
				save()
				last = time.Now()
			}
		})
		_ = remote.Close()
	}
	status.Running = false
	if err == nil {
		now := time.Now()
		status.SuccessAt = &now
	} else {
		status.Error = err.Error()
	}
	save()
	return err
}

type Viewer struct {
	Database  *Database
	ConfigDir string
	Config    Config
	Host      string
	Token     string
	mu        sync.Mutex
	syncing   bool
}

func (v *Viewer) Handler() http.Handler {
	files, _ := fs.Sub(viewerFiles, "web")
	static := http.FileServer(http.FS(files))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		if r.Host != v.Host || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+v.Host) || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "Forbidden", 403)
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/open/") {
			token := strings.TrimPrefix(r.URL.Path, "/open/")
			if v.Token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(v.Token)) != 1 {
				http.Error(w, "Forbidden", 403)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "archive_session", Value: v.Token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		cookie, err := r.Cookie("archive_session")
		if err != nil || v.Token == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(v.Token)) != 1 {
			http.Error(w, "Open this page from the archive tool", 403)
			return
		}
		if r.URL.Path == "/api/sync" && r.Method == http.MethodPost {
			v.mu.Lock()
			if !v.syncing {
				v.syncing = true
				go func() {
					defer func() { v.mu.Lock(); v.syncing = false; v.mu.Unlock() }()
					ctx, cancel := context.WithTimeout(context.Background(), 4*time.Hour)
					defer cancel()
					_ = RunSync(ctx, v.ConfigDir, v.Config, v.Database)
				}()
			}
			v.mu.Unlock()
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", 405)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if r.URL.Path == "/api/status" {
			v.writeJSON(w, map[string]any{"directory": v.Config.Directory, "sync": ReadStatus(v.ConfigDir)})
			return
		}
		if r.URL.Path == "/api/records" {
			before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
			query := r.URL.Query().Get("q")
			group := r.URL.Query().Get("group")
			if before < 0 || len(query) > 800 || len(group) > 100 {
				http.Error(w, "Invalid query", 400)
				return
			}
			page, err := v.Database.List(ctx, before, query, group)
			if err != nil {
				http.Error(w, "Could not read local archive", 500)
				return
			}
			v.writeJSON(w, page)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/records/") {
			id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/records/"), 10, 64)
			if err != nil || id < 1 {
				http.Error(w, "Invalid record", 400)
				return
			}
			raw, err := v.Database.Detail(ctx, id)
			if err != nil {
				http.Error(w, "Record unavailable or integrity check failed; retain the archive disk and check sync status", 404)
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			if r.URL.Query().Get("download") == "1" {
				w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=conversation-%d.json", id))
			}
			_, _ = w.Write(raw)
			return
		}
		if r.URL.Path != "/" && r.URL.Path != "/app.js" && r.URL.Path != "/context.js" && r.URL.Path != "/style.css" {
			http.NotFound(w, r)
			return
		}
		static.ServeHTTP(w, r)
	})
}
func (v *Viewer) writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}
func View(ctx context.Context, db *Database, configDir string, c Config) error {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	var token [32]byte
	if _, err = rand.Read(token[:]); err != nil {
		_ = listener.Close()
		return err
	}
	v := &Viewer{Database: db, ConfigDir: configDir, Config: c, Host: listener.Addr().String(), Token: hex.EncodeToString(token[:])}
	server := &http.Server{Handler: v.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() { <-ctx.Done(); _ = server.Close() }()
	target := "http://" + v.Host + "/open/" + v.Token
	fmt.Println("Local archive viewer:", target)
	if err = OpenBrowser(target); err != nil {
		fmt.Println("Open the local address above in your browser.")
	}
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
