//go:build !windows

package state

import (
	"os"
	"path/filepath"
	"testing"
)

func checkPrivate(t *testing.T, dir string) {
	t.Helper()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("directory mode %v, want 0700", info.Mode().Perm())
	}
	key, err := os.Stat(filepath.Join(dir, keyFile))
	if err != nil {
		t.Fatal(err)
	}
	if key.Mode().Perm() != 0o600 {
		t.Errorf("key mode %v, want 0600", key.Mode().Perm())
	}
}
