// Package proto defines version 1 of the WebSocket protocol between agent and
// control center (ARCHITECTURE.md section 8.2).
//
// Both sides are Go and import this package, so these types are the contract.
// Frames are JSON text messages wrapped in an Envelope.
package proto

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	// Version is the protocol version sent in hello and hello.ok.
	Version = 1
	// Subprotocol is the WebSocket subprotocol name.
	Subprotocol = "smartcluster.agent.v1"
	// MaxMessageBytes is the largest frame either side accepts.
	MaxMessageBytes = 1 << 20
)

// Message types (Envelope.Type).
const (
	TypeHello     = "hello"
	TypeHelloOK   = "hello.ok"
	TypeStats     = "stats"
	TypeCmd       = "cmd"
	TypeCmdAck    = "cmd.ack"
	TypeCmdReject = "cmd.reject"
	TypeCmdResult = "cmd.result"
)

// WebSocket close codes.
const (
	CloseAuthFailed          = 4001
	CloseSuperseded          = 4409
	CloseProtocolUnsupported = 4426
)

var (
	ErrTooLarge    = errors.New("proto: frame exceeds MaxMessageBytes")
	ErrMissingType = errors.New("proto: frame has no type")
)

// Envelope wraps every frame.
type Envelope struct {
	Type string          `json:"t"`
	ID   string          `json:"id,omitempty"`
	TS   time.Time       `json:"ts"`
	Data json.RawMessage `json:"d"`
}

// Encode builds a frame of message type t carrying data.
func Encode(t, id string, ts time.Time, data any) ([]byte, error) {
	if t == "" {
		return nil, ErrMissingType
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("proto: encode %s: %w", t, err)
	}
	frame, err := json.Marshal(Envelope{Type: t, ID: id, TS: ts.UTC(), Data: raw})
	if err != nil {
		return nil, fmt.Errorf("proto: encode %s: %w", t, err)
	}
	if len(frame) > MaxMessageBytes {
		return nil, ErrTooLarge
	}
	return frame, nil
}

// Decode parses a frame. The payload stays raw until Unmarshal is called.
func Decode(frame []byte) (Envelope, error) {
	if len(frame) > MaxMessageBytes {
		return Envelope{}, ErrTooLarge
	}
	var env Envelope
	if err := json.Unmarshal(frame, &env); err != nil {
		return Envelope{}, fmt.Errorf("proto: decode frame: %w", err)
	}
	if env.Type == "" {
		return Envelope{}, ErrMissingType
	}
	return env, nil
}

// Unmarshal decodes the payload into v. Unknown fields are ignored so that a
// newer peer can add fields without breaking an older one.
func (e Envelope) Unmarshal(v any) error {
	if len(e.Data) == 0 {
		return fmt.Errorf("proto: %s frame has no payload", e.Type)
	}
	if err := json.Unmarshal(e.Data, v); err != nil {
		return fmt.Errorf("proto: decode %s payload: %w", e.Type, err)
	}
	return nil
}
