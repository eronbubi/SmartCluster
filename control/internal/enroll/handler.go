// Package enroll serves the enrollment endpoint of the control center
// (ARCHITECTURE.md 4.3). The protocol and the proofs live in pkg/enroll.
package enroll

import (
	"context"
	"crypto/hmac"
	"crypto/x509"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/eronbubi/smartcluster/pkg/enroll"
	"github.com/eronbubi/smartcluster/pkg/ids"
	"github.com/eronbubi/smartcluster/pkg/proto"
)

const (
	maxRequestBytes = 64 << 10
	maxNameLength   = 63
)

// ErrCodeUsed is returned by Store.CompleteEnrollment when the code was
// consumed in the meantime.
var ErrCodeUsed = errors.New("enroll: join code already used")

// Code is an active join code as the store keeps it: only the derived key.
type Code struct {
	ID        string
	Key       []byte
	ExpiresAt time.Time
}

// NewNode is the node row created by a successful enrollment.
type NewNode struct {
	ID           string
	Name         string
	CertSerial   string
	CertNotAfter time.Time
}

// Store persists codes and nodes.
type Store interface {
	// ActiveCodes returns unused join codes that have not expired at now.
	ActiveCodes(ctx context.Context, now time.Time) ([]Code, error)
	// CompleteEnrollment marks the code used and creates the node in one
	// transaction. It returns ErrCodeUsed if the code was used meanwhile.
	CompleteEnrollment(ctx context.Context, codeID string, node NewNode) error
}

// Issuer signs agent certificates.
type Issuer interface {
	SignClientCSR(csrDER []byte, nodeID string, now time.Time) (*x509.Certificate, error)
}

// Handler serves POST /agent/v1/enroll.
type Handler struct {
	Store  Store
	Issuer Issuer
	CACert *x509.Certificate
	// ServerCert returns the leaf certificate the agent listener presents.
	// The proofs are bound to it.
	ServerCert func() *x509.Certificate
	CmdKeys    func() []proto.PubKey
	Now        func() time.Time
	Log        *slog.Logger

	limiter limiter
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
		return
	}
	now := h.Now()
	ip := remoteIP(r)
	if !h.limiter.allow(ip, now) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many enrollment attempts, wait a minute")
		return
	}

	var req enroll.Request
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is not valid JSON")
		return
	}
	if len(req.Nonce) != enroll.NonceSize || len(req.CSR) == 0 || len(req.Proof) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "csr, nonce and proof are required")
		return
	}

	serverCert := h.ServerCert()
	codes, err := h.Store.ActiveCodes(r.Context(), now)
	if err != nil {
		h.Log.Error("enroll: load join codes", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	var code *Code
	for i := range codes {
		if hmac.Equal(req.Proof, enroll.AgentProof(codes[i].Key, serverCert.Raw, req.CSR, req.Nonce)) {
			code = &codes[i]
			break
		}
	}
	if code == nil {
		// Wrong, expired or used code, or a man in the middle: the agent's
		// proof is bound to the certificate it saw. Deliberately one answer.
		h.Log.Warn("enroll: refused", "remote", ip)
		writeError(w, http.StatusForbidden, "enrollment_refused", "join code is wrong, expired or already used")
		return
	}

	nodeID := ids.NewV7()
	cert, err := h.Issuer.SignClientCSR(req.CSR, nodeID, now)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_csr", err.Error())
		return
	}
	err = h.Store.CompleteEnrollment(r.Context(), code.ID, NewNode{
		ID:           nodeID,
		Name:         nodeName(req.Hostname, nodeID),
		CertSerial:   cert.SerialNumber.Text(16),
		CertNotAfter: cert.NotAfter,
	})
	if errors.Is(err, ErrCodeUsed) {
		writeError(w, http.StatusForbidden, "enrollment_refused", "join code is wrong, expired or already used")
		return
	}
	if err != nil {
		h.Log.Error("enroll: store node", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	h.Log.Info("enroll: node joined", "node", nodeID, "name", req.Hostname, "remote", ip)
	writeJSON(w, http.StatusOK, enroll.Response{
		NodeID:     nodeID,
		ClientCert: cert.Raw,
		CACert:     h.CACert.Raw,
		CmdPubKeys: h.CmdKeys(),
		Proof:      enroll.ServerProof(code.Key, serverCert.Raw, cert.Raw, req.Nonce),
	})
}

// nodeName turns the reported hostname into a display name. It is only a
// label, so anything odd is dropped rather than rejected.
func nodeName(hostname, nodeID string) string {
	name := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			return r
		}
		return -1
	}, hostname)
	if len(name) > maxNameLength {
		name = name[:maxNameLength]
	}
	if name == "" {
		return "node-" + nodeID[len(nodeID)-6:]
	}
	return name
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	var e enroll.ErrorResponse
	e.Error.Code, e.Error.Message = code, message
	writeJSON(w, status, e)
}

// limiter allows a few enrollment attempts per remote address and minute.
// Join codes have 100 bits, so this is about noise and load, not guessing.
type limiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
}

const (
	limitWindow   = time.Minute
	limitAttempts = 10
)

func (l *limiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.attempts == nil {
		l.attempts = map[string][]time.Time{}
	}
	recent := l.attempts[ip][:0]
	for _, t := range l.attempts[ip] {
		if now.Sub(t) < limitWindow {
			recent = append(recent, t)
		}
	}
	if len(recent) >= limitAttempts {
		l.attempts[ip] = recent
		return false
	}
	l.attempts[ip] = append(recent, now)
	return true
}
