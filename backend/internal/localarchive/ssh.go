package localarchive

import (
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/archivewire"
	"golang.org/x/crypto/ssh"
)

type SSHRemote struct {
	connection net.Conn
	client     *ssh.Client
	session    *ssh.Session
	input      io.Reader
	output     io.Writer
}

func Connect(ctx context.Context, configDir string, c Config) (*SSHRemote, error) {
	protected, err := os.ReadFile(filepath.Join(configDir, "identity.key"))
	if err != nil {
		return nil, err
	}
	raw, err := unprotect(protected)
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid local identity")
	}
	signer, err := ssh.NewSignerFromKey(ed25519.PrivateKey(raw))
	if err != nil {
		return nil, err
	}
	pinned, _, _, _, err := ssh.ParseAuthorizedKey([]byte(c.HostKey))
	if err != nil {
		return nil, err
	}
	connection, err := (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, "tcp", c.Host)
	if err != nil {
		return nil, err
	}
	_ = connection.SetDeadline(time.Now().Add(30 * time.Second))
	sshConnection, channels, requests, err := ssh.NewClientConn(connection, c.Host, &ssh.ClientConfig{User: "sub2api-archive", Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, HostKeyCallback: ssh.FixedHostKey(pinned), HostKeyAlgorithms: []string{ssh.KeyAlgoED25519}, Timeout: 20 * time.Second})
	if err != nil {
		_ = connection.Close()
		return nil, errors.New("encrypted archive connection failed: check pairing and server identity")
	}
	client := ssh.NewClient(sshConnection, channels, requests)
	session, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	remote := &SSHRemote{connection: connection, client: client, session: session}
	remote.output, err = session.StdinPipe()
	if err == nil {
		remote.input, err = session.StdoutPipe()
	}
	if err == nil {
		err = session.Start("archive-sync")
	}
	if err != nil {
		_ = remote.Close()
		return nil, err
	}
	return remote, nil
}
func (r *SSHRemote) Call(ctx context.Context, request archivewire.Request) (archivewire.Response, error) {
	deadline := time.Now().Add(90 * time.Second)
	if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
		deadline = end
	}
	if err := ctx.Err(); err != nil {
		return archivewire.Response{}, err
	}
	_ = r.connection.SetDeadline(deadline)
	if err := archivewire.Write(r.output, request, archivewire.MaxRequest); err != nil {
		return archivewire.Response{}, err
	}
	var response archivewire.Response
	if err := archivewire.Read(r.input, &response, archivewire.MaxResponse); err != nil {
		return response, err
	}
	if response.Version != archivewire.Version {
		return response, errors.New("unsupported archive protocol version")
	}
	if response.Error != "" {
		return response, errors.New(response.Error)
	}
	return response, nil
}
func (r *SSHRemote) Close() error { _ = r.session.Close(); return r.client.Close() }
func AcquireSyncLock(configDir string) (*os.File, error) {
	file, err := os.OpenFile(filepath.Join(configDir, "sync.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = lockFile(file); err != nil {
		_ = file.Close()
		return nil, errors.New("a sync is already running")
	}
	return file, nil
}
