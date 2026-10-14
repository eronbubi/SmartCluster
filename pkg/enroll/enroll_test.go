package enroll

import (
	"bytes"
	"errors"
	"testing"
)

func TestNewCode(t *testing.T) {
	c := NewCode()
	parsed, err := ParseCode(string(c))
	if err != nil || parsed != c {
		t.Fatalf("NewCode %q does not parse back: %q %v", c, parsed, err)
	}
	if NewCode() == c {
		t.Error("two codes are equal")
	}
}

func TestParseCodeNormalizes(t *testing.T) {
	want := Code("K7QF2MXAPW9DHT3RZB6N")
	for _, in := range []string{
		"K7QF2MXAPW9DHT3RZB6N",
		"k7qf2-mxapw-9dht3-rzb6n",
		" K7QF2 MXAPW 9DHT3 RZB6N ",
	} {
		got, err := ParseCode(in)
		if err != nil || got != want {
			t.Errorf("ParseCode(%q) = %q, %v", in, got, err)
		}
	}
	got, err := ParseCode("ABCDE-FGHJK-MNPQR-STO1L")
	if err != nil || got != "ABCDEFGHJKMNPQRST011" {
		t.Errorf("O/L normalization: %q %v", got, err)
	}
}

func TestParseCodeRejects(t *testing.T) {
	for _, in := range []string{"", "K7QF2", "K7QF2MXAPW9DHT3RZB6NX", "K7QF2MXAPW9DHT3RZB6U", "K7QF2MXAPW9DHT3RZB6!"} {
		if _, err := ParseCode(in); !errors.Is(err, ErrInvalidCode) {
			t.Errorf("ParseCode(%q) = %v, want ErrInvalidCode", in, err)
		}
	}
}

func TestCodeString(t *testing.T) {
	if got := Code("K7QF2MXAPW9DHT3RZB6N").String(); got != "K7QF2-MXAPW-9DHT3-RZB6N" {
		t.Errorf("got %q", got)
	}
}

func TestCodeKey(t *testing.T) {
	a, b := Code("K7QF2MXAPW9DHT3RZB6N"), Code("K7QF2MXAPW9DHT3RZB6P")
	if len(a.Key()) != 32 || bytes.Equal(a.Key(), b.Key()) || !bytes.Equal(a.Key(), a.Key()) {
		t.Error("keys must be 32 bytes, deterministic and differ per code")
	}
}

func TestProofsAreBound(t *testing.T) {
	key := Code("K7QF2MXAPW9DHT3RZB6N").Key()
	cert, obj, nonce := []byte("server cert"), []byte("csr"), []byte("nonce")
	base := AgentProof(key, cert, obj, nonce)

	variants := map[string][]byte{
		"other certificate": AgentProof(key, []byte("mitm cert"), obj, nonce),
		"other csr":         AgentProof(key, cert, []byte("other csr"), nonce),
		"other nonce":       AgentProof(key, cert, obj, []byte("nonce2")),
		"other code":        AgentProof(Code("K7QF2MXAPW9DHT3RZB6P").Key(), cert, obj, nonce),
		"server direction":  ServerProof(key, cert, obj, nonce),
	}
	for name, p := range variants {
		if bytes.Equal(p, base) {
			t.Errorf("%s gives the same proof", name)
		}
	}
	if !bytes.Equal(base, AgentProof(key, cert, obj, nonce)) {
		t.Error("proof is not deterministic")
	}
}
