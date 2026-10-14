package secrets

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSealOpen(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	b, err := NewBox(key)
	if err != nil {
		t.Fatal(err)
	}
	nonce, ct := b.Seal([]byte("ca private key"), []byte("cluster/ca_key"))
	got, err := b.Open(nonce, ct, []byte("cluster/ca_key"))
	if err != nil || string(got) != "ca private key" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := b.Open(nonce, ct, []byte("cluster/cmd_key")); err == nil {
		t.Error("opened with a different aad")
	}
	ct[0] ^= 1
	if _, err := b.Open(nonce, ct, []byte("cluster/ca_key")); err == nil {
		t.Error("opened tampered ciphertext")
	}
	other, _ := NewBox(bytes.Repeat([]byte{8}, 32))
	nonce, ct = b.Seal([]byte("x"), nil)
	if _, err := other.Open(nonce, ct, nil); err == nil {
		t.Error("opened with a different key")
	}
}

func TestSealUsesFreshNonces(t *testing.T) {
	b, _ := NewBox(make([]byte, 32))
	n1, c1 := b.Seal([]byte("same"), nil)
	n2, c2 := b.Seal([]byte("same"), nil)
	if bytes.Equal(n1, n2) || bytes.Equal(c1, c2) {
		t.Error("nonce reused")
	}
}

func TestNewBoxRejectsShortKey(t *testing.T) {
	if _, err := NewBox(make([]byte, 16)); err == nil {
		t.Error("accepted a 16-byte key")
	}
}

func TestLoadOrCreateKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "master.key")
	k1, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	k2, err := LoadOrCreateKey(path)
	if err != nil || !bytes.Equal(k1, k2) {
		t.Fatalf("second load differs: %v", err)
	}
	if err := os.WriteFile(path, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateKey(path); err == nil {
		t.Error("accepted a corrupt key file")
	}
}
