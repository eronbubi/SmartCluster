package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

func newCA(t *testing.T) *CA {
	t.Helper()
	ca, err := NewCA("test", now)
	if err != nil {
		t.Fatal(err)
	}
	return ca
}

func TestServerCertificateVerifies(t *testing.T) {
	ca := newCA(t)
	cert, err := ca.IssueServer([]string{"192.168.1.20", "eron.local", "localhost", "::1"}, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"192.168.1.20", "eron.local", "localhost", "::1"} {
		_, err := cert.Leaf.Verify(x509.VerifyOptions{Roots: ca.Pool(), DNSName: host, CurrentTime: now})
		if err != nil {
			t.Errorf("%s: %v", host, err)
		}
	}
	if len(cert.Certificate) != 2 {
		t.Error("chain should include the CA certificate")
	}
}

// ADR-0015: a browser that imports the cluster CA must not trust it for
// public names or addresses.
func TestNameConstraintsRejectPublicNames(t *testing.T) {
	ca := newCA(t)
	for _, host := range []string{"example.com", "8.8.8.8", "bank.example.local.evil.com", "2001:db8::1"} {
		if _, err := ca.IssueServer([]string{host}, now); err == nil {
			t.Errorf("issued a certificate for %s", host)
		}
	}
}

func TestSignClientCSR(t *testing.T) {
	ca := newCA(t)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: "pretend-to-be-admin"},
		DNSNames: []string{"control.local"},
	}, key)

	cert, err := ca.SignClientCSR(csr, "0192f1c4-7e2a-7b3c-9d4e-5f6a7b8c9d0e", now)
	if err != nil {
		t.Fatal(err)
	}
	if cert.Subject.CommonName != "0192f1c4-7e2a-7b3c-9d4e-5f6a7b8c9d0e" || len(cert.DNSNames) != 0 {
		t.Errorf("subject/SANs taken from CSR: %v %v", cert.Subject, cert.DNSNames)
	}
	if _, err := cert.Verify(x509.VerifyOptions{Roots: ca.Pool(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, CurrentTime: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := cert.Verify(x509.VerifyOptions{Roots: ca.Pool(), CurrentTime: now}); err == nil {
		t.Error("client certificate is usable as server certificate")
	}
}

func TestSignClientCSRRejects(t *testing.T) {
	ca := newCA(t)
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	rsaCSR, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, rsaKey)
	if _, err := ca.SignClientCSR(rsaCSR, "node", now); !errors.Is(err, ErrUnsupportedKey) {
		t.Errorf("RSA key: %v", err)
	}

	p384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	p384CSR, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, p384)
	if _, err := ca.SignClientCSR(p384CSR, "node", now); !errors.Is(err, ErrUnsupportedKey) {
		t.Errorf("P-384 key: %v", err)
	}

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	csr[len(csr)-1] ^= 0xff // break the signature
	if _, err := ca.SignClientCSR(csr, "node", now); err == nil {
		t.Error("CSR with broken signature accepted")
	}
}

func TestPEMRoundTrip(t *testing.T) {
	ca := newCA(t)
	certPEM, keyPEM, err := ca.MarshalPEM()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadCA(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Cert.Equal(ca.Cert) || !loaded.Key.Equal(ca.Key) {
		t.Error("round trip changed the CA")
	}
	if _, err := loaded.IssueServer([]string{"10.0.0.5"}, now); err != nil {
		t.Errorf("loaded CA cannot issue: %v", err)
	}
}

func TestFingerprint(t *testing.T) {
	fp := newCA(t).Fingerprint()
	if len(strings.Fields(fp)) != 16 || strings.ToUpper(fp) != fp {
		t.Errorf("fingerprint %q", fp)
	}
}
