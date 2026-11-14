package localarchive

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

type Config struct {
	Host      string `json:"host"`
	HostKey   string `json:"host_key"`
	WebURL    string `json:"web_url"`
	Directory string `json:"directory"`
	PublicKey string `json:"public_key"`
}

func ConfigDirectory() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Sub2APIArchive"), nil
}
func (c Config) validate() error {
	if _, _, err := net.SplitHostPort(c.Host); err != nil {
		return errors.New("invalid archive SSH host:port")
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(c.HostKey))
	if err != nil || key.Type() != ssh.KeyAlgoED25519 {
		return errors.New("invalid pinned SSH host key")
	}
	web, err := url.Parse(c.WebURL)
	if err != nil || web.Host == "" || (web.Scheme != "http" && web.Scheme != "https") || web.User != nil || web.RawQuery != "" || web.Fragment != "" {
		return errors.New("invalid admin web URL")
	}
	if !filepath.IsAbs(c.Directory) {
		return errors.New("choose an absolute archive folder")
	}
	if filepath.Dir(filepath.Clean(c.Directory)) == filepath.Clean(c.Directory) {
		return errors.New("choose a dedicated folder, such as D:\\Sub2API, instead of the root of a disk")
	}
	return nil
}
func Initialize(configDir string, c Config) (Config, error) {
	if err := c.validate(); err != nil {
		return c, err
	}
	if existing, err := LoadConfig(configDir); err == nil {
		if existing.Host != c.Host || existing.HostKey != c.HostKey || existing.Directory != c.Directory {
			return c, errors.New("an archive is already configured; keep its original host and folder")
		}
		return existing, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return c, err
	}
	if files, err := os.ReadDir(c.Directory); err == nil && len(files) > 0 {
		if _, err = os.Stat(filepath.Join(c.Directory, "archive.db")); err != nil {
			return c, errors.New("choose an empty dedicated archive folder")
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return c, err
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return c, err
	}
	if err := privateDirectory(configDir); err != nil {
		return c, err
	}
	if err := os.MkdirAll(c.Directory, 0700); err != nil {
		return c, err
	}
	if err := privateDirectory(c.Directory); err != nil {
		return c, err
	}
	configPath := filepath.Join(configDir, "config.json")
	keyPath := filepath.Join(configDir, "identity.key")
	var private ed25519.PrivateKey
	protected, err := os.ReadFile(keyPath)
	if err == nil {
		raw, unprotectErr := unprotect(protected)
		if unprotectErr != nil {
			return c, unprotectErr
		}
		private = ed25519.PrivateKey(raw)
	} else if errors.Is(err, os.ErrNotExist) {
		_, private, err = ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return c, err
		}
		protected, err = protect(private)
		if err != nil {
			return c, err
		}
		if err = atomicWrite(keyPath, protected); err != nil {
			return c, err
		}
	} else {
		return c, err
	}
	if len(private) != ed25519.PrivateKeySize {
		return c, errors.New("invalid local identity")
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		return c, err
	}
	c.PublicKey = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return c, err
	}
	return c, atomicWrite(configPath, raw)
}
func LoadConfig(configDir string) (Config, error) {
	var c Config
	raw, err := os.ReadFile(filepath.Join(configDir, "config.json"))
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(raw, &c); err != nil {
		return c, err
	}
	return c, c.validate()
}
func atomicWrite(path string, raw []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".archive-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer func() { _ = os.Remove(name) }()
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(raw)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}
func (c Config) PairURL() string {
	u, _ := url.Parse(strings.TrimRight(c.WebURL, "/") + "/admin/conversation-archives")
	query := u.Query()
	query.Set("archive_key", c.PublicKey)
	u.RawQuery = query.Encode()
	return u.String()
}
