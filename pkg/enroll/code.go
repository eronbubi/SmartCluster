// Package enroll implements node enrollment with a one-time join code
// (ARCHITECTURE.md 4.3, ADR-0009).
//
// The join code authenticates both sides and binds the exchange to the TLS
// certificate the agent actually saw, so a man in the middle with another
// certificate makes both proofs fail. No fingerprint comparison by hand is
// needed.
package enroll

import (
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"strings"
)

// CodeLength is the number of Crockford base32 symbols in a join code (100 bits).
const CodeLength = 20

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

var ErrInvalidCode = errors.New("enroll: join code must be 20 characters of 0-9 and A-Z without I, L, O, U")

// Code is a join code in canonical form: 20 upper-case Crockford base32 symbols.
type Code string

// NewCode returns a random join code.
func NewCode() Code {
	var b [CodeLength]byte
	_, _ = rand.Read(b[:]) // never fails since Go 1.24
	for i := range b {
		b[i] = crockford[b[i]&0x1f] // 256 is a multiple of 32, so this is uniform
	}
	return Code(b[:])
}

// ParseCode accepts what a person types: any case, with or without hyphens
// and spaces, and the usual confusions I/L for 1 and O for 0.
func ParseCode(s string) (Code, error) {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		switch r {
		case '-', ' ':
			continue
		case 'I', 'L':
			r = '1'
		case 'O':
			r = '0'
		}
		if !strings.ContainsRune(crockford, r) {
			return "", ErrInvalidCode
		}
		b.WriteRune(r)
	}
	if b.Len() != CodeLength {
		return "", ErrInvalidCode
	}
	return Code(b.String()), nil
}

// String formats the code for display: XXXXX-XXXXX-XXXXX-XXXXX.
func (c Code) String() string {
	s := string(c)
	if len(s) != CodeLength {
		return s
	}
	return s[0:5] + "-" + s[5:10] + "-" + s[10:15] + "-" + s[15:20]
}

// Key derives the proof key from the code. The control center stores only
// this key, never the code itself.
func (c Code) Key() []byte {
	key, err := hkdf.Key(sha256.New, []byte(c), []byte("smartcluster-enroll-v1"), "proof key", 32)
	if err != nil {
		panic(err) // only possible for invalid lengths, which are constants here
	}
	return key
}
