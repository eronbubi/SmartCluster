// Package state persists the agent's identity and configuration.
//
// Everything lives in one directory that only SYSTEM/root and administrators
// can read: agent.json (control address, node id, command keys), node.pem
// (private key and certificate together, so a renewal replaces both with one
// atomic rename) and ca.crt.
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
	configFile   = "agent.json"
	identityFile = "node.pem"
	caFile       = "ca.crt"
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
	Dir    string
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
	if err := SaveCert(dir, id.Key, id.Cert); err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(dir, caFile), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: id.CA.Raw})); err != nil {
		return err
	}
	// Last: the config file's presence marks a complete enrollment.
	return writeConfig(dir, Config{Control: control, NodeID: id.NodeID, CmdKeys: id.CmdKeys})
}

// SaveCert replaces key and certificate in one step (after renewal).
func SaveCert(dir string, key *ecdsa.PrivateKey, cert *x509.Certificate) error {
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	data := append(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})...)
	return writeFileAtomic(filepath.Join(dir, identityFile), data)
}

// SaveCmdKeys replaces the trusted command keys.
func SaveCmdKeys(dir string, keys []proto.PubKey) error {
	s, err := Load(dir)
	if err != nil {
		return err
	}
	cfg := s.Config
	cfg.CmdKeys = keys
	return writeConfig(dir, cfg)
}

func writeConfig(dir string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, configFile), append(data, '\n'))
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
	s := State{Dir: dir}
	if err := json.Unmarshal(raw, &s.Config); err != nil {
		return nil, fmt.Errorf("state: parse %s: %w", configFile, err)
	}
	identity, err1 := os.ReadFile(filepath.Join(dir, identityFile))
	caPEM, err2 := os.ReadFile(filepath.Join(dir, caFile))
	if err := errors.Join(err1, err2); err != nil {
		return nil, fmt.Errorf("state: %w", err)
	}
	s.Cert, err = tls.X509KeyPair(identity, identity)
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
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
