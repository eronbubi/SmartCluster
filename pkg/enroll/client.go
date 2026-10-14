package enroll

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/eronbubi/smartcluster/pkg/ids"
	"github.com/eronbubi/smartcluster/pkg/proto"
)

var (
	// ErrRefused means the control center rejected the code (wrong, expired or used).
	ErrRefused = errors.New("enroll: the control center refused the join code")
	// ErrServerProof means the answer did not prove knowledge of the code for the
	// certificate we saw: wrong server or a man in the middle.
	ErrServerProof = errors.New("enroll: the control center could not prove it knows the join code")
)

const maxResponseBytes = 1 << 20

// Identity is what an agent keeps after enrolling.
type Identity struct {
	NodeID  string
	Key     *ecdsa.PrivateKey
	Cert    *x509.Certificate
	CA      *x509.Certificate
	CmdKeys []proto.PubKey
}

// Enroll joins the cluster whose control center listens for agents at
// address (host:port).
func Enroll(ctx context.Context, address string, code Code, hostname string) (*Identity, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("enroll: address %q: %w", address, err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "smartcluster-agent"},
	}, key)
	if err != nil {
		return nil, fmt.Errorf("enroll: create CSR: %w", err)
	}
	nonce := make([]byte, NonceSize)
	_, _ = rand.Read(nonce)

	// The server certificate cannot be verified yet: we do not know the
	// cluster CA. Trust comes from the channel-bound proofs instead.
	dialer := &tls.Dialer{Config: &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // authenticated by AgentProof/ServerProof below
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{"http/1.1"},
	}}
	rawConn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("enroll: connect to %s: %w", address, err)
	}
	conn := rawConn.(*tls.Conn)
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	}
	peers := conn.ConnectionState().PeerCertificates
	if len(peers) == 0 {
		return nil, errors.New("enroll: server presented no certificate")
	}
	serverCert := peers[0]

	codeKey := code.Key()
	body, err := json.Marshal(Request{
		CSR:      csrDER,
		Nonce:    nonce,
		Proof:    AgentProof(codeKey, serverCert.Raw, csrDER, nonce),
		Hostname: hostname,
	})
	if err != nil {
		return nil, err
	}

	// Send the request over exactly the connection whose certificate the
	// proof is bound to.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+address+Path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := req.Write(conn); err != nil {
		return nil, fmt.Errorf("enroll: send request: %w", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		return nil, fmt.Errorf("enroll: read response: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("enroll: read response: %w", err)
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, ErrRefused
	}
	if resp.StatusCode != http.StatusOK {
		var e ErrorResponse
		_ = json.Unmarshal(data, &e)
		return nil, fmt.Errorf("enroll: control center answered %s: %s", resp.Status, e.Error.Message)
	}

	var res Response
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("enroll: decode response: %w", err)
	}
	if !hmac.Equal(res.Proof, ServerProof(codeKey, serverCert.Raw, res.ClientCert, nonce)) {
		return nil, ErrServerProof
	}
	return checkResponse(res, key, serverCert, host)
}

// checkResponse makes sure the issued identity is usable: the certificate is
// ours, both certificates chain to the returned CA, and later normal TLS
// connections to host will verify.
func checkResponse(res Response, key *ecdsa.PrivateKey, serverCert *x509.Certificate, host string) (*Identity, error) {
	if !ids.IsUUID(res.NodeID) {
		return nil, fmt.Errorf("enroll: invalid node id %q", res.NodeID)
	}
	cert, err := x509.ParseCertificate(res.ClientCert)
	if err != nil {
		return nil, fmt.Errorf("enroll: parse client certificate: %w", err)
	}
	ca, err := x509.ParseCertificate(res.CACert)
	if err != nil {
		return nil, fmt.Errorf("enroll: parse CA certificate: %w", err)
	}
	pub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok || !pub.Equal(&key.PublicKey) {
		return nil, errors.New("enroll: client certificate is not for our key")
	}
	if cert.Subject.CommonName != res.NodeID {
		return nil, fmt.Errorf("enroll: client certificate is for %q, not node %s", cert.Subject.CommonName, res.NodeID)
	}

	roots := x509.NewCertPool()
	roots.AddCert(ca)
	if _, err := cert.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		return nil, fmt.Errorf("enroll: client certificate does not chain to the cluster CA: %w", err)
	}
	if _, err := serverCert.Verify(x509.VerifyOptions{Roots: roots, DNSName: host}); err != nil {
		return nil, fmt.Errorf("enroll: control center certificate is not valid for %s under the cluster CA: %w", host, err)
	}
	return &Identity{NodeID: res.NodeID, Key: key, Cert: cert, CA: ca, CmdKeys: res.CmdPubKeys}, nil
}
