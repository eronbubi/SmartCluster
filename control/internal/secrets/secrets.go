// Package secrets encrypts values stored in the database with the master key
// (ARCHITECTURE.md 5.1, 5.6).
//
// The master key lives in a file next to the database. It protects dumps and
// backups, not a machine on which an attacker already has root.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	keySize = 32
	// KeyVersion is stored with every secret so the key can be rotated later.
	KeyVersion = 1
)

// Box seals and opens secrets with AES-256-GCM.
type Box struct {
	aead cipher.AEAD
}

// NewBox returns a Box for a 32-byte master key.
func NewBox(key []byte) (*Box, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("secrets: master key has %d bytes, want %d", len(key), keySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext. aad binds the ciphertext to where it is stored
// (scope and name), so a ciphertext copied to another row does not open.
func (b *Box) Seal(plaintext, aad []byte) (nonce, ciphertext []byte) {
	nonce = make([]byte, b.aead.NonceSize())
	_, _ = rand.Read(nonce)
	return nonce, b.aead.Seal(nil, nonce, plaintext, aad)
}

// Open decrypts a value sealed with the same aad.
func (b *Box) Open(nonce, ciphertext, aad []byte) ([]byte, error) {
	if len(nonce) != b.aead.NonceSize() {
		return nil, errors.New("secrets: bad nonce length")
	}
	plain, err := b.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, errors.New("secrets: cannot decrypt (wrong master key or tampered data)")
	}
	return plain, nil
}

// LoadOrCreateKey reads the master key file or creates it with mode 0600.
func LoadOrCreateKey(path string) ([]byte, error) {
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != keySize {
			return nil, fmt.Errorf("secrets: %s has %d bytes, want %d", path, len(key), keySize)
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	key = make([]byte, keySize)
	_, _ = rand.Read(key)
	// O_EXCL: never overwrite a key another process created in the meantime.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("secrets: create master key: %w", err)
	}
	if _, err := f.Write(key); err != nil {
		f.Close()
		return nil, err
	}
	return key, f.Close()
}
