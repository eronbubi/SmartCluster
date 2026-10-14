// Command certfixtures writes the certificates the Electron shell's tests use
// (app/electron/testdata). Only public certificates are written; the keys are
// thrown away. Fixed validity periods keep the tests deterministic.
//
//	go run ./tools/certfixtures app/electron/testdata
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

var (
	validFrom = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	validTo   = time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC)
	serial    int64
)

type issuer struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

func newKey() *ecdsa.PrivateKey {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatal(err)
	}
	return k
}

func sign(tmpl *x509.Certificate, parent *issuer, key *ecdsa.PrivateKey) *x509.Certificate {
	serial++
	tmpl.SerialNumber = big.NewInt(serial)
	parentCert, parentKey := tmpl, key
	if parent != nil {
		parentCert, parentKey = parent.cert, parent.key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parentCert, &key.PublicKey, parentKey)
	if err != nil {
		log.Fatal(err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		log.Fatal(err)
	}
	return c
}

func newCA(name string) *issuer {
	key := newKey()
	return &issuer{key: key, cert: sign(&x509.Certificate{
		Subject: pkix.Name{CommonName: name}, NotBefore: validFrom, NotAfter: validTo,
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}, nil, key)}
}

func leaf(ca *issuer, usage x509.ExtKeyUsage, from, to time.Time) *x509.Certificate {
	return sign(&x509.Certificate{
		Subject: pkix.Name{CommonName: "fixture"}, NotBefore: from, NotAfter: to,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage},
		IPAddresses: []net.IP{net.ParseIP("192.168.1.20")}, DNSNames: []string{"control.local"},
	}, ca, newKey())
}

func write(dir, name string, c *x509.Certificate) {
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		log.Fatal(err)
	}
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: certfixtures <output directory>")
	}
	dir := os.Args[1]
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
	ca := newCA("SmartCluster CA fixture")
	foreign := newCA("Foreign CA fixture")
	write(dir, "ca.pem", ca.cert)
	write(dir, "server.pem", leaf(ca, x509.ExtKeyUsageServerAuth, validFrom, validTo))
	write(dir, "client.pem", leaf(ca, x509.ExtKeyUsageClientAuth, validFrom, validTo))
	write(dir, "expired.pem", leaf(ca, x509.ExtKeyUsageServerAuth, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)))
	write(dir, "foreign-server.pem", leaf(foreign, x509.ExtKeyUsageServerAuth, validFrom, validTo))
}
