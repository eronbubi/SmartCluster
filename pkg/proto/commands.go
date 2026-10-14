package proto

import "encoding/json"

// CommandVersion is the version of the SignedCommand format.
const CommandVersion = 1

// SignedCommand is the payload of a "cmd" frame (ARCHITECTURE.md 5.3).
//
// Sig is an Ed25519 signature over the RFC 8785 canonical form of the received
// JSON object without the "sig" member, so every field, including ones this
// version does not know, is covered. Times are RFC 3339 strings in UTC with
// second precision; keeping them as strings means re-encoding can never change
// the signed bytes.
type SignedCommand struct {
	V         int             `json:"v"`
	KeyID     string          `json:"kid"`
	CmdID     string          `json:"cmd_id"`
	NodeID    string          `json:"node_id"`
	Type      string          `json:"type"`
	IssuedAt  string          `json:"issued_at"`
	ExpiresAt string          `json:"expires_at"`
	Payload   json.RawMessage `json:"payload"`
	Sig       string          `json:"sig"`
}

// Command types. Only milestone a commands exist so far.
const (
	CmdCertRenew = "cert.renew"
)
