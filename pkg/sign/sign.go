// Package sign signs commands in the control center and verifies them in the
// agent (ARCHITECTURE.md 5.3).
package sign

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/eronbubi/smartcluster/pkg/ids"
	"github.com/eronbubi/smartcluster/pkg/jcs"
	"github.com/eronbubi/smartcluster/pkg/proto"
)

const (
	// DefaultValidity is how long a freshly signed command stays valid.
	DefaultValidity = 5 * time.Minute
	// MaxValidity is the longest validity window an agent accepts. It also
	// bounds how long command ids must be remembered for replay protection.
	MaxValidity = 15 * time.Minute
	// ClockSkew is the tolerated difference between the clocks of control
	// center and agent, after the agent applied the offset from hello.ok.
	ClockSkew = 30 * time.Second

	replayGrace = time.Minute
	sigMember   = "sig"
)

// EncodeKey returns the wire form of an Ed25519 public key.
func EncodeKey(key ed25519.PublicKey) string {
	return base64.RawURLEncoding.EncodeToString(key)
}

// DecodeKey parses the wire form of an Ed25519 public key.
func DecodeKey(s string) (ed25519.PublicKey, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("sign: decode key: %w", err)
	}
	if len(b) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("sign: key has %d bytes, want %d", len(b), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(b), nil
}

// Signer creates signed commands. It is safe for concurrent use.
type Signer struct {
	keyID    string
	key      ed25519.PrivateKey
	validity time.Duration
	now      func() time.Time
}

// NewSigner returns a Signer that signs with key under the id keyID.
func NewSigner(keyID string, key ed25519.PrivateKey) *Signer {
	return &Signer{keyID: keyID, key: key, validity: DefaultValidity, now: time.Now}
}

// Sign creates a command of type typ for nodeID with a fresh command id.
// payload must marshal to a JSON object without fractional numbers.
func (s *Signer) Sign(nodeID, typ string, payload any) (proto.SignedCommand, error) {
	raw := json.RawMessage(`{}`)
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return proto.SignedCommand{}, fmt.Errorf("sign: marshal payload: %w", err)
		}
		raw = b
	}
	issued := s.now().UTC().Truncate(time.Second)
	cmd := proto.SignedCommand{
		V:         proto.CommandVersion,
		KeyID:     s.keyID,
		CmdID:     ids.NewV7(),
		NodeID:    nodeID,
		Type:      typ,
		IssuedAt:  issued.Format(time.RFC3339),
		ExpiresAt: issued.Add(s.validity).Format(time.RFC3339),
		Payload:   raw,
	}
	doc, err := json.Marshal(cmd)
	if err != nil {
		return proto.SignedCommand{}, fmt.Errorf("sign: marshal command: %w", err)
	}
	msg, err := jcs.CanonicalizeWithout(doc, sigMember)
	if err != nil {
		return proto.SignedCommand{}, fmt.Errorf("sign: canonicalize: %w", err)
	}
	cmd.Sig = base64.RawURLEncoding.EncodeToString(ed25519.Sign(s.key, msg))
	return cmd, nil
}

// RejectError is a verification failure. Code is one of the proto.Reject*
// constants and goes back to the control center in a cmd.reject frame.
type RejectError struct {
	Code string
	Err  error
}

func (e *RejectError) Error() string { return fmt.Sprintf("command rejected (%s): %v", e.Code, e.Err) }
func (e *RejectError) Unwrap() error { return e.Err }

func reject(code string, format string, args ...any) error {
	return &RejectError{Code: code, Err: fmt.Errorf(format, args...)}
}

// Verifier checks commands addressed to one node. It is safe for concurrent use.
//
// The replay cache lives in memory. After an agent restart a command could be
// replayed within its validity window; the mTLS channel is the primary
// protection against that, the cache is defense in depth.
type Verifier struct {
	nodeID string
	now    func() time.Time

	mu   sync.Mutex
	keys map[string]ed25519.PublicKey
	seen map[string]time.Time // cmd_id -> forget after
}

// NewVerifier returns a Verifier for nodeID that trusts keys (by key id).
// now is the agent's clock corrected by the offset from hello.ok.
func NewVerifier(nodeID string, keys map[string]ed25519.PublicKey, now func() time.Time) *Verifier {
	v := &Verifier{nodeID: nodeID, now: now, seen: make(map[string]time.Time)}
	v.SetKeys(keys)
	return v
}

// SetKeys replaces the trusted command keys.
func (v *Verifier) SetKeys(keys map[string]ed25519.PublicKey) {
	copied := make(map[string]ed25519.PublicKey, len(keys))
	for id, k := range keys {
		copied[id] = k
	}
	v.mu.Lock()
	v.keys = copied
	v.mu.Unlock()
}

// Verify checks the raw payload of a "cmd" frame and returns the decoded
// command. Errors are *RejectError. Whether the command type is known and the
// payload valid for it is checked by the caller.
func (v *Verifier) Verify(raw []byte) (proto.SignedCommand, error) {
	var cmd proto.SignedCommand
	if err := json.Unmarshal(raw, &cmd); err != nil {
		return cmd, reject(proto.RejectInvalidPayload, "decode command: %w", err)
	}
	if cmd.V != proto.CommandVersion {
		return cmd, reject(proto.RejectInvalidPayload, "command version %d not supported", cmd.V)
	}

	v.mu.Lock()
	key, ok := v.keys[cmd.KeyID]
	v.mu.Unlock()
	if !ok {
		return cmd, reject(proto.RejectBadSignature, "unknown key id %q", cmd.KeyID)
	}
	sig, err := base64.RawURLEncoding.DecodeString(cmd.Sig)
	if err != nil {
		return cmd, reject(proto.RejectBadSignature, "decode signature: %w", err)
	}
	msg, err := jcs.CanonicalizeWithout(raw, sigMember)
	if err != nil {
		return cmd, reject(proto.RejectInvalidPayload, "canonicalize: %w", err)
	}
	if !ed25519.Verify(key, msg, sig) {
		return cmd, reject(proto.RejectBadSignature, "signature does not match")
	}

	if cmd.NodeID != v.nodeID {
		return cmd, reject(proto.RejectWrongNode, "command is for node %q", cmd.NodeID)
	}

	issued, err1 := time.Parse(time.RFC3339, cmd.IssuedAt)
	expires, err2 := time.Parse(time.RFC3339, cmd.ExpiresAt)
	if err := errors.Join(err1, err2); err != nil {
		return cmd, reject(proto.RejectInvalidPayload, "parse validity window: %w", err)
	}
	if !expires.After(issued) || expires.Sub(issued) > MaxValidity {
		return cmd, reject(proto.RejectExpired, "validity window %s to %s not allowed", cmd.IssuedAt, cmd.ExpiresAt)
	}
	now := v.now()
	if now.Add(ClockSkew).Before(issued) || now.After(expires) {
		return cmd, reject(proto.RejectExpired, "valid from %s to %s, now %s", cmd.IssuedAt, cmd.ExpiresAt, now.UTC().Format(time.RFC3339))
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	for id, forget := range v.seen {
		if now.After(forget) {
			delete(v.seen, id)
		}
	}
	if _, dup := v.seen[cmd.CmdID]; dup {
		return cmd, reject(proto.RejectReplay, "command %s already received", cmd.CmdID)
	}
	v.seen[cmd.CmdID] = expires.Add(replayGrace)
	return cmd, nil
}
