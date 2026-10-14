// Package pki is the cluster certificate authority (ARCHITECTURE.md 5.1,
// ADR-0015). It issues the control center's server certificate and the
// agents' client certificates.
package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"

	"github.com/eronbubi/smartcluster/pkg/enroll"
)

const (
	caValidity     = 10 * 365 * 24 * time.Hour
	leafValidity   = 365 * 24 * time.Hour
	backdate       = time.Hour // tolerates clocks that are a little behind
	RenewBeforeEnd = 30 * 24 * time.Hour
)

// permittedNets limits what the CA can vouch for: private, loopback and
// link-local addresses. A browser that trusts the cluster CA (ADR-0015)
// therefore never trusts it for public addresses.
var permittedNets = []string{
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8", "169.254.0.0/16",
	"fc00::/7", "fe80::/10", "::1/128",
}

// permittedDomains limits DNS names to mDNS names and localhost.
var permittedDomains = []string{"local", "localhost"}

// Allowed reports whether host (an IP address or DNS name) lies within the
// CA's name constraints, i.e. whether a server certificate may contain it.
func Allowed(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		for _, s := range permittedNets {
			if _, n, err := net.ParseCIDR(s); err == nil && n.Contains(ip) {
				return true
			}
		}
		return false
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, d := range permittedDomains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// CA is the cluster certificate authority.
type CA struct {
	Cert *x509.Certificate
	Key  *ecdsa.PrivateKey
}

// NewCA creates a cluster CA with name constraints.
func NewCA(clusterName string, now time.Time) (*CA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	var nets []*net.IPNet
	for _, s := range permittedNets {
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			return nil, err
		}
		nets = append(nets, n)
	}
	tmpl := &x509.Certificate{
		SerialNumber:                serial(),
		Subject:                     pkix.Name{CommonName: "SmartCluster CA " + clusterName, Organization: []string{"SmartCluster"}},
		NotBefore:                   now.Add(-backdate),
		NotAfter:                    now.Add(caValidity),
		KeyUsage:                    x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid:       true,
		IsCA:                        true,
		MaxPathLenZero:              true,
		PermittedDNSDomainsCritical: true,
		PermittedDNSDomains:         permittedDomains,
		PermittedIPRanges:           nets,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("pki: create CA: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &CA{Cert: cert, Key: key}, nil
}

// Pool returns a pool containing only this CA.
func (ca *CA) Pool() *x509.CertPool {
	p := x509.NewCertPool()
	p.AddCert(ca.Cert)
	return p
}

// Fingerprint returns the SHA-256 fingerprint of the CA certificate for
// comparison by people (same format the agent shows).
func (ca *CA) Fingerprint() string {
	return enroll.Fingerprint(ca.Cert)
}

// IssueServer issues the control center's TLS certificate for the given IP
// addresses and DNS names. Names outside the CA's constraints are rejected.
func (ca *CA) IssueServer(hosts []string, now time.Time) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: "SmartCluster control center"},
		NotBefore:    now.Add(-backdate),
		NotAfter:     now.Add(leafValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Cert, &key.PublicKey, ca.Key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("pki: issue server certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, err
	}
	// Go does not check name constraints when signing; verify so a host the
	// constraints forbid fails here and not later in every client.
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: ca.Pool(), CurrentTime: now}); err != nil {
		return tls.Certificate{}, fmt.Errorf("pki: server certificate for %v violates the CA constraints: %w", hosts, err)
	}
	return tls.Certificate{Certificate: [][]byte{der, ca.Cert.Raw}, PrivateKey: key, Leaf: leaf}, nil
}

var ErrUnsupportedKey = errors.New("pki: CSR key must be ECDSA P-256")

// SignClientCSR issues an agent certificate for nodeID. The subject comes
// from nodeID, never from the CSR; the CSR only contributes its public key.
func (ca *CA) SignClientCSR(csrDER []byte, nodeID string, now time.Time) (*x509.Certificate, error) {
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		return nil, fmt.Errorf("pki: parse CSR: %w", err)
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("pki: CSR signature: %w", err)
	}
	pub, ok := csr.PublicKey.(*ecdsa.PublicKey)
	if !ok || pub.Curve != elliptic.P256() {
		return nil, ErrUnsupportedKey
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: nodeID},
		NotBefore:    now.Add(-backdate),
		NotAfter:     now.Add(leafValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Cert, pub, ca.Key)
	if err != nil {
		return nil, fmt.Errorf("pki: sign client certificate: %w", err)
	}
	return x509.ParseCertificate(der)
}

// MarshalPEM encodes certificate and private key.
func (ca *CA) MarshalPEM() (certPEM, keyPEM []byte, err error) {
	keyDER, err := x509.MarshalPKCS8PrivateKey(ca.Key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Cert.Raw}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), nil
}

// LoadCA decodes a CA written by MarshalPEM.
func LoadCA(certPEM, keyPEM []byte) (*CA, error) {
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("pki: load CA: %w", err)
	}
	key, ok := pair.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, ErrUnsupportedKey
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, err
	}
	if !cert.IsCA {
		return nil, errors.New("pki: certificate is not a CA")
	}
	return &CA{Cert: cert, Key: key}, nil
}

func serial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		panic(err) // crypto/rand does not fail
	}
	return n.Add(n, big.NewInt(1))
}
