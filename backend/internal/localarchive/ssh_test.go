package localarchive

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/archivewire"
	"golang.org/x/crypto/ssh"
)

func TestSSHUsesPinnedEd25519WithMultipleServerHostKeys(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	ecdsaPrivate, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	other, err := ssh.NewSignerFromKey(ecdsaPrivate)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	config := &ssh.ServerConfig{PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) { return nil, nil }}
	config.AddHostKey(other)
	config.AddHostKey(pinned)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = connection.Close() }()
		_ = connection.SetDeadline(time.Now().Add(10 * time.Second))
		server, channels, requests, err := ssh.NewServerConn(connection, config)
		if err != nil {
			return
		}
		defer func() { _ = server.Close() }()
		go ssh.DiscardRequests(requests)
		for next := range channels {
			channel, requests, err := next.Accept()
			if err != nil {
				return
			}
			for request := range requests {
				if request.Type == "exec" {
					_ = request.Reply(true, nil)
					var frame archivewire.Request
					if archivewire.Read(channel, &frame, archivewire.MaxRequest) == nil {
						_ = archivewire.Write(channel, archivewire.Response{Version: archivewire.Version}, archivewire.MaxResponse)
					}
					_ = channel.Close()
					return
				}
				_ = request.Reply(false, nil)
			}
		}
	}()
	configDir := t.TempDir()
	c, err := Initialize(configDir, Config{Host: listener.Addr().String(), HostKey: string(ssh.MarshalAuthorizedKey(pinned.PublicKey())), WebURL: "https://example.invalid", Directory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	remote, err := Connect(ctx, configDir, c)
	if err != nil {
		t.Fatal("pinned key was not selected among multiple server algorithms", err)
	}
	response, err := remote.Call(ctx, archivewire.Request{Operation: "hello"})
	_ = remote.Close()
	wg.Wait()
	if err != nil || response.Version != archivewire.Version {
		t.Fatal("encrypted protocol exchange failed", err)
	}
}
