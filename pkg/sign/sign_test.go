package sign

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eronbubi/smartcluster/pkg/proto"
)

const nodeID = "0192f1c4-7e2a-7b3c-9d4e-5f6a7b8c9d0e"

var t0 = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

type fixture struct {
	signer   *Signer
	verifier *Verifier
	clock    *time.Time
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	clock := t0
	s := NewSigner("cmd-1", priv)
	s.now = func() time.Time { return t0 }
	v := NewVerifier(nodeID, map[string]ed25519.PublicKey{"cmd-1": pub}, func() time.Time { return clock })
	return fixture{signer: s, verifier: v, clock: &clock}
}

func (f fixture) signRaw(t *testing.T, typ string, payload any) []byte {
	t.Helper()
	cmd, err := f.signer.Sign(nodeID, typ, payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func wantReject(t *testing.T, err error, code string) {
	t.Helper()
	var re *RejectError
	if !errors.As(err, &re) {
		t.Fatalf("got %v, want RejectError %s", err, code)
	}
	if re.Code != code {
		t.Fatalf("code = %s (%v), want %s", re.Code, re.Err, code)
	}
}

func TestSignAndVerify(t *testing.T) {
	f := newFixture(t)
	raw := f.signRaw(t, proto.CmdCertRenew, map[string]any{"delay_s": 3, "reason": "expiring"})

	cmd, err := f.verifier.Verify(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Type != proto.CmdCertRenew || cmd.NodeID != nodeID || cmd.KeyID != "cmd-1" {
		t.Errorf("unexpected command %+v", cmd)
	}
	if cmd.IssuedAt != "2026-10-14T12:00:00Z" || cmd.ExpiresAt != "2026-10-14T12:05:00Z" {
		t.Errorf("validity %s .. %s", cmd.IssuedAt, cmd.ExpiresAt)
	}
	if string(cmd.Payload) != `{"delay_s":3,"reason":"expiring"}` {
		t.Errorf("payload %s", cmd.Payload)
	}
}

func TestNilPayloadBecomesEmptyObject(t *testing.T) {
	f := newFixture(t)
	cmd, err := f.verifier.Verify(f.signRaw(t, proto.CmdCertRenew, nil))
	if err != nil {
		t.Fatal(err)
	}
	if string(cmd.Payload) != `{}` {
		t.Errorf("payload %s", cmd.Payload)
	}
}

// The signature covers the canonical form, so member order and whitespace of
// the received frame do not matter.
func TestVerifyIgnoresMemberOrder(t *testing.T) {
	f := newFixture(t)
	var m map[string]json.RawMessage
	if err := json.Unmarshal(f.signRaw(t, proto.CmdCertRenew, map[string]int{"b": 1, "a": 2}), &m); err != nil {
		t.Fatal(err)
	}
	reordered := `{ "sig": ` + string(m["sig"]) + `, "payload": {"a":2, "b":1}, "type": ` + string(m["type"]) +
		`, "node_id": ` + string(m["node_id"]) + `, "kid": ` + string(m["kid"]) + `, "v": 1, "cmd_id": ` + string(m["cmd_id"]) +
		`, "issued_at": ` + string(m["issued_at"]) + `, "expires_at": ` + string(m["expires_at"]) + ` }`
	if _, err := f.verifier.Verify([]byte(reordered)); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRejectsTampering(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(m map[string]any)
	}{
		{"payload changed", func(m map[string]any) { m["payload"] = map[string]any{"delay_s": 4} }},
		{"type changed", func(m map[string]any) { m["type"] = "role.apply" }},
		{"expiry extended", func(m map[string]any) { m["expires_at"] = "2026-10-14T12:10:00Z" }},
		{"unknown member added", func(m map[string]any) { m["shell"] = "rm -rf /" }},
		{"signature truncated", func(m map[string]any) { m["sig"] = m["sig"].(string)[:20] }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			var m map[string]any
			if err := json.Unmarshal(f.signRaw(t, proto.CmdCertRenew, map[string]int{"delay_s": 3}), &m); err != nil {
				t.Fatal(err)
			}
			tt.mutate(m)
			raw, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.verifier.Verify(raw)
			wantReject(t, err, proto.RejectBadSignature)
		})
	}
}

func TestVerifyRejectsUnknownKey(t *testing.T) {
	f := newFixture(t)
	_, other, _ := ed25519.GenerateKey(nil)
	foreign := NewSigner("cmd-9", other)
	foreign.now = func() time.Time { return t0 }
	cmd, err := foreign.Sign(nodeID, proto.CmdCertRenew, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(cmd)
	_, err = f.verifier.Verify(raw)
	wantReject(t, err, proto.RejectBadSignature)
}

func TestVerifyRejectsKeyIDSwap(t *testing.T) {
	// A command signed by an untrusted key must not pass by claiming a trusted key id.
	f := newFixture(t)
	_, other, _ := ed25519.GenerateKey(nil)
	foreign := NewSigner("cmd-1", other)
	foreign.now = func() time.Time { return t0 }
	cmd, _ := foreign.Sign(nodeID, proto.CmdCertRenew, nil)
	raw, _ := json.Marshal(cmd)
	_, err := f.verifier.Verify(raw)
	wantReject(t, err, proto.RejectBadSignature)
}

func TestVerifyRejectsWrongNode(t *testing.T) {
	f := newFixture(t)
	cmd, _ := f.signer.Sign("0192f1c4-0000-7000-8000-000000000000", proto.CmdCertRenew, nil)
	raw, _ := json.Marshal(cmd)
	_, err := f.verifier.Verify(raw)
	wantReject(t, err, proto.RejectWrongNode)
}

func TestVerifyTimeWindow(t *testing.T) {
	tests := []struct {
		name  string
		now   time.Time
		valid bool
	}{
		{"at issue time", t0, true},
		{"just before expiry", t0.Add(DefaultValidity), true},
		{"after expiry", t0.Add(DefaultValidity + time.Second), false},
		{"agent clock slightly behind", t0.Add(-ClockSkew), true},
		{"agent clock too far behind", t0.Add(-ClockSkew - time.Second), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			raw := f.signRaw(t, proto.CmdCertRenew, nil)
			*f.clock = tt.now
			_, err := f.verifier.Verify(raw)
			if tt.valid && err != nil {
				t.Fatalf("rejected: %v", err)
			}
			if !tt.valid {
				wantReject(t, err, proto.RejectExpired)
			}
		})
	}
}

func TestVerifyRejectsOverlongValidity(t *testing.T) {
	f := newFixture(t)
	f.signer.validity = MaxValidity + time.Minute
	_, err := f.verifier.Verify(f.signRaw(t, proto.CmdCertRenew, nil))
	wantReject(t, err, proto.RejectExpired)
}

func TestVerifyRejectsReplay(t *testing.T) {
	f := newFixture(t)
	raw := f.signRaw(t, proto.CmdCertRenew, nil)
	if _, err := f.verifier.Verify(raw); err != nil {
		t.Fatal(err)
	}
	*f.clock = t0.Add(time.Minute)
	_, err := f.verifier.Verify(raw)
	wantReject(t, err, proto.RejectReplay)
}

func TestReplayCacheIsPruned(t *testing.T) {
	f := newFixture(t)
	if _, err := f.verifier.Verify(f.signRaw(t, proto.CmdCertRenew, nil)); err != nil {
		t.Fatal(err)
	}
	*f.clock = t0.Add(DefaultValidity + replayGrace + time.Second)
	f.signer.now = func() time.Time { return *f.clock }
	if _, err := f.verifier.Verify(f.signRaw(t, proto.CmdCertRenew, nil)); err != nil {
		t.Fatal(err)
	}
	if n := len(f.verifier.seen); n != 1 {
		t.Errorf("replay cache holds %d ids, want 1", n)
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name string
		raw  string
		code string
	}{
		{"not json", `{`, proto.RejectInvalidPayload},
		{"wrong version", `{"v":2,"kid":"cmd-1","sig":""}`, proto.RejectInvalidPayload},
		{"fractional number", strings.Replace(string(f.signRaw(t, proto.CmdCertRenew, nil)), `"payload":{}`, `"payload":{"x":1.5}`, 1), proto.RejectInvalidPayload},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.verifier.Verify([]byte(tt.raw))
			wantReject(t, err, tt.code)
		})
	}
}

func TestSignRejectsFractionalPayload(t *testing.T) {
	f := newFixture(t)
	if _, err := f.signer.Sign(nodeID, proto.CmdCertRenew, map[string]float64{"x": 1.5}); err == nil {
		t.Error("fractional payload was signed")
	}
}

func TestKeyEncoding(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	got, err := DecodeKey(EncodeKey(pub))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(pub) {
		t.Error("round trip changed the key")
	}
	if _, err := DecodeKey("c2hvcnQ"); err == nil {
		t.Error("short key accepted")
	}
}
