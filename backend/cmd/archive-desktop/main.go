// archive-desktop is a standalone Windows archive sync and offline viewer.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/localarchive"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Archive:", err)
		os.Exit(1)
	}
}
func run() error {
	command := "view"
	args := os.Args[1:]
	if len(args) > 0 {
		command = args[0]
		args = args[1:]
	}
	if command == "version" {
		fmt.Println(version)
		return nil
	}
	defaultDir, err := localarchive.ConfigDirectory()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	configDir := flags.String("config", defaultDir, "configuration directory")
	directory := flags.String("directory", "", "absolute local archive directory")
	host := flags.String("host", "", "SSH host:port")
	hostKey := flags.String("host-key", "", "pinned SSH host public key")
	web := flags.String("web", "", "admin web URL")
	if err = flags.Parse(args); err != nil {
		return err
	}
	if command == "init" {
		c, err := localarchive.Initialize(*configDir, localarchive.Config{Host: *host, HostKey: *hostKey, WebURL: *web, Directory: *directory})
		if err != nil {
			return err
		}
		fmt.Println("Public key:", c.PublicKey)
		fmt.Println("Pair in your administrator browser:", c.PairURL())
		return localarchive.OpenBrowser(c.PairURL())
	}
	c, err := localarchive.LoadConfig(*configDir)
	if err != nil {
		return fmt.Errorf("run Install.cmd first: %w", err)
	}
	if command == "pair" {
		fmt.Println(c.PairURL())
		return localarchive.OpenBrowser(c.PairURL())
	}
	if command == "status" {
		return json.NewEncoder(os.Stdout).Encode(localarchive.ReadStatus(*configDir))
	}
	db, err := localarchive.Open(c.Directory)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	switch command {
	case "sync":
		ctx, timeout := context.WithTimeout(ctx, 4*time.Hour)
		defer timeout()
		if err = localarchive.RunSync(ctx, *configDir, c, db); err != nil {
			return err
		}
		fmt.Println("Sync verified and complete.")
		return nil
	case "verify":
		if err = db.VerifyAll(ctx); err != nil {
			return err
		}
		fmt.Println("Every local record passed integrity checks.")
		return nil
	case "view":
		return localarchive.View(ctx, db, *configDir, c)
	default:
		return fmt.Errorf("unknown command %q; use init, pair, sync, view, verify, status, version", command)
	}
}
