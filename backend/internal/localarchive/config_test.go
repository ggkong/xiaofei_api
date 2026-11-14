package localarchive

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestIdentityIsStableAndProtectedOnWindows(t *testing.T) {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	configDir, archiveDir := t.TempDir(), t.TempDir()
	c := Config{Host: "127.0.0.1:22", HostKey: string(ssh.MarshalAuthorizedKey(key)), WebURL: "https://example.invalid", Directory: archiveDir}
	first, err := Initialize(configDir, c)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Initialize(configDir, c)
	if err != nil || first.PublicKey != second.PublicKey {
		t.Fatal("identity rotated on reinstall", err)
	}
	protected, err := os.ReadFile(filepath.Join(configDir, "identity.key"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := unprotect(protected)
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		t.Fatal("identity protection round trip failed", err)
	}
	if runtime.GOOS == "windows" && bytes.Equal(raw, protected) {
		t.Fatal("Windows private key stored unencrypted")
	}
	lock, err := AcquireSyncLock(configDir)
	if err != nil {
		t.Fatal(err)
	}
	if secondLock, err := AcquireSyncLock(configDir); err == nil {
		_ = secondLock.Close()
		t.Fatal("overlapping sync allowed")
	}
	_ = lock.Close()
	again, err := AcquireSyncLock(configDir)
	if err != nil {
		t.Fatal("sync lock did not release", err)
	}
	_ = again.Close()
	c.Directory = t.TempDir()
	if _, err = Initialize(configDir, c); err == nil {
		t.Fatal("silently changed primary archive directory")
	}
}
