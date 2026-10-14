package session

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"

	"github.com/eronbubi/smartcluster/agent/internal/state"
	"github.com/eronbubi/smartcluster/pkg/enroll"
	"github.com/eronbubi/smartcluster/pkg/proto"
	"github.com/eronbubi/smartcluster/pkg/sign"
)

const maxRenewResponse = 64 << 10

// commands executes verified commands. Only the types below exist; there is
// no way to run arbitrary programs (ARCHITECTURE.md 5.4).
type commands struct {
	st     *state.State
	client *http.Client
	log    *slog.Logger
}

func (h *commands) handle(ctx context.Context, ws *websocket.Conn, v *sign.Verifier, raw []byte) {
	cmd, err := v.Verify(raw)
	if err != nil {
		var re *sign.RejectError
		code := proto.RejectInvalidPayload
		if errors.As(err, &re) {
			code = re.Code
		}
		h.log.Warn("command rejected", "cmd", cmd.CmdID, "code", code, "err", err)
		_ = write(ctx, ws, proto.TypeCmdReject, proto.CmdReject{CmdID: cmd.CmdID, Code: code, Detail: err.Error()})
		return
	}

	var run func(context.Context, json.RawMessage) error
	switch cmd.Type {
	case proto.CmdCertRenew:
		run = h.certRenew
	default:
		h.log.Warn("command rejected", "cmd", cmd.CmdID, "type", cmd.Type, "err", errUnknownType)
		_ = write(ctx, ws, proto.TypeCmdReject, proto.CmdReject{CmdID: cmd.CmdID, Code: proto.RejectUnknownType})
		return
	}
	if err := write(ctx, ws, proto.TypeCmdAck, proto.CmdAck{CmdID: cmd.CmdID}); err != nil {
		return
	}
	result := proto.CmdResult{CmdID: cmd.CmdID, OK: true}
	if err := run(ctx, cmd.Payload); err != nil {
		h.log.Error("command failed", "cmd", cmd.CmdID, "type", cmd.Type, "err", err)
		result.OK, result.ErrorCode = false, "failed"
	} else {
		h.log.Info("command done", "cmd", cmd.CmdID, "type", cmd.Type)
	}
	_ = write(ctx, ws, proto.TypeCmdResult, result)
}

// certRenew creates a new key, asks the control center to certify it over the
// current mTLS identity and stores the result. The running connection keeps
// the old certificate; the next connection uses the new one.
func (h *commands) certRenew(ctx context.Context, _ json.RawMessage) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	if err != nil {
		return err
	}
	body, err := json.Marshal(enroll.RenewRequest{CSR: csr})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+h.st.Control+enroll.RenewPath, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("renewal refused: %s", resp.Status)
	}
	var res enroll.RenewResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxRenewResponse)).Decode(&res); err != nil {
		return err
	}
	cert, err := x509.ParseCertificate(res.ClientCert)
	if err != nil {
		return err
	}
	if pub, ok := cert.PublicKey.(*ecdsa.PublicKey); !ok || !pub.Equal(&key.PublicKey) || cert.Subject.CommonName != h.st.NodeID {
		return errors.New("renewed certificate does not match our key or node")
	}
	if _, err := cert.Verify(x509.VerifyOptions{Roots: h.st.CAPool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		return fmt.Errorf("renewed certificate does not chain to the cluster CA: %w", err)
	}
	return state.SaveCert(h.st.Dir, key, cert)
}
