//go:build !windows

package localarchive

import (
	"os"
	"os/exec"
	"runtime"

	"golang.org/x/sys/unix"
)

func protect(raw []byte) ([]byte, error)   { return raw, nil }
func unprotect(raw []byte) ([]byte, error) { return raw, nil }
func privateDirectory(path string) error   { return os.Chmod(path, 0700) }
func lockFile(file *os.File) error         { return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB) }
func OpenBrowser(target string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	return exec.Command(name, target).Start()
}
