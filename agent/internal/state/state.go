// Package state persists the agent's identity and configuration.
//
// Everything lives in one directory that only SYSTEM/root and administrators
// can read: agent.json (control address, node id, command keys), node.key,
// node.crt and ca.crt.
package state

import (
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/eronbubi/smartcluster/pkg/enroll"
	"github.com/eronbubi/smartcluster/pkg/proto"
)

const (
	configFile = "agent.json"
	keyFile    = "node.key"
	certFile   = "node.crt"
	caFile     = "ca.crt"
)

// ErrNotEnrolled means the directory holds no identity yet.
var ErrNotEnrolled = errors.New("state: agent is not enrolled; run 'smartcluster-agent join'")

// Config is the agent's persistent configuration.
type Config struct {
	Control string         `json:"control"` // host:port of the agent listener
	NodeID  string         `json:"node_id"`
	CmdKeys []proto.PubKey `json:"cmd_keys"`
}

// State is a loaded identity, ready for mTLS.
type State struct {
	Config
	Cert   tls.Certificate
	CA     *x509.Certificate
	CAPool *x509.CertPool
}

// Save writes the identity from enrollment. Existing files are replaced.
func Save(dir, control string, id *enroll.Identity) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("state: create %s: %w", dir, err)
	}
	if err := secureDir(dir); err != nil {
		return fmt.Errorf("state: restrict access to %s: %w", dir, err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(id.Key)
	if err != nil {
		return err
	}
	cfg, err := json.MarshalIndent(Config{Control: control, NodeID: id.NodeID, CmdKeys: id.CmdKeys}, "", "  ")
	if err != nil {
		return err
	}
	files := []struct {
		name string
		data []byte
	}{
		{keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})},
		{certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: id.Cert.Raw})},
		{caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: id.CA.Raw})},
		{configFile, append(cfg, '\n')}, // last: its presence marks a complete enrollment
	}
	for _, f := range files {
		if err := writeFileAtomic(filepath.Join(dir, f.name), f.data); err != nil {
			return err
		}
	}
	return nil
}

// Load reads the identity written by Save.
func Load(dir string) (*State, error) {
	raw, err := os.ReadFile(filepath.Join(dir, configFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotEnrolled
	}
	if err != nil {
		return nil, fmt.Errorf("state: %w", err)
	}
	var s State
	if err := json.Unmarshal(raw, &s.Config); err != nil {
		return nil, fmt.Errorf("state: parse %s: %w", configFile, err)
	}
	certPEM, err1 := os.ReadFile(filepath.Join(dir, certFile))
	keyPEM, err2 := os.ReadFile(filepath.Join(dir, keyFile))
	caPEM, err3 := os.ReadFile(filepath.Join(dir, caFile))
	if err := errors.Join(err1, err2, err3); err != nil {
		return nil, fmt.Errorf("state: %w", err)
	}
	s.Cert, err = tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("state: node certificate: %w", err)
	}
	if _, ok := s.Cert.PrivateKey.(*ecdsa.PrivateKey); !ok {
		return nil, errors.New("state: node key is not ECDSA")
	}
	block, _ := pem.Decode(caPEM)
	if block == nil {
		return nil, errors.New("state: ca.crt is not PEM")
	}
	s.CA, err = x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("state: CA certificate: %w", err)
	}
	s.CAPool = x509.NewCertPool()
	s.CAPool.AddCert(s.CA)
	return &s, nil
}

func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
