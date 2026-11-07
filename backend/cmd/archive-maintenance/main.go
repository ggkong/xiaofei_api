package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/archivewire"
	"github.com/Wei-Shaw/sub2api/internal/conversationarchive"
)

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		fmt.Fprintln(os.Stderr, "Usage: archive-maintenance stats|compact|expand|purge|local-keys|local-sync DEVICE_ID")
		os.Exit(2)
	}
	dsn := url.URL{Scheme: "postgres", User: url.UserPassword(env("DATABASE_USER", "sub2api"), os.Getenv("DATABASE_PASSWORD")), Host: net.JoinHostPort(env("DATABASE_HOST", "postgres"), env("DATABASE_PORT", "5432")), Path: "/" + env("DATABASE_DBNAME", "sub2api")}
	query := dsn.Query()
	query.Set("sslmode", env("DATABASE_SSLMODE", "disable"))
	dsn.RawQuery = query.Encode()
	db, err := sql.Open("postgres", dsn.String())
	if err != nil {
		fail()
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(4)
	options := conversationarchive.OptionsFromEnv()
	options.Enabled = true
	options.Deduplicate = true
	options.Configurable = true
	store := conversationarchive.New(db, options)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	switch os.Args[1] {
	case "local-keys":
		devices, err := store.AuthorizedLocalKeys(ctx)
		if err != nil || json.NewEncoder(os.Stdout).Encode(devices) != nil {
			fail()
		}
	case "local-sync":
		if len(os.Args) != 3 || !archivewire.ValidHash(os.Args[2]) {
			fail()
		}
		if store.ServeLocalTransfer(ctx, os.Args[2], os.Stdin, os.Stdout) != nil {
			fail()
		}
	case "stats":
		info, err := store.StorageInfo(ctx)
		if err != nil {
			fail()
		}
		_ = json.NewEncoder(os.Stdout).Encode(info)
	case "purge":
		if store.PurgeExpired(ctx) != nil {
			fail()
		}
		fmt.Println(`{"purged":true}`)
	case "compact", "expand":
		if os.Args[1] == "expand" {
			info, err := store.StorageInfo(ctx)
			if err != nil {
				fail()
			}
			if info.FreeBytes > 0 && info.LogicalBytes+1<<30 > info.FreeBytes {
				fmt.Fprintln(os.Stderr, "Not enough free space to expand stored contexts safely")
				os.Exit(1)
			}
		}
		total := 0
		for {
			batch, cancel := context.WithTimeout(ctx, 15*time.Second)
			var n int
			var err error
			if os.Args[1] == "compact" {
				n, err = store.CompactExisting(batch, 10)
			} else {
				n, err = store.ExpandExisting(batch, 10)
			}
			cancel()
			if err != nil {
				fail()
			}
			total += n
			if n == 0 {
				break
			}
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"operation": os.Args[1], "records": total})
	default:
		fmt.Fprintln(os.Stderr, "Unknown archive maintenance operation")
		os.Exit(2)
	}
}

func fail() {
	fmt.Fprintln(os.Stderr, "Archive maintenance failed; no credentials or conversation contents were printed")
	os.Exit(1)
}
