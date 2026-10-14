package state

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/eronbubi/smartcluster/pkg/enroll"
	"github.com/eronbubi/smartcluster/pkg/proto"
)

func selfSigned(t *testing.T, cn string, isCA bool) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: isCA, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	return cert, key
}

func TestSaveAndLoad(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agent")
	cert, key := selfSigned(t, "0192f1c4-7e2a-7b3c-9d4e-5f6a7b8c9d0e", false)
	ca, _ := selfSigned(t, "SmartCluster CA test", true)
	id := &enroll.Identity{
		NodeID: "0192f1c4-7e2a-7b3c-9d4e-5f6a7b8c9d0e", Key: key, Cert: cert, CA: ca,
		CmdKeys: []proto.PubKey{{KeyID: "cmd-1", Key: "abc"}},
	}
	if err := Save(dir, "192.168.1.20:8444", id); err != nil {
		t.Fatal(err)
	}
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.NodeID != id.NodeID || s.Control != "192.168.1.20:8444" || len(s.CmdKeys) != 1 {
		t.Errorf("config %+v", s.Config)
	}
	if !s.CA.Equal(ca) || len(s.Cert.Certificate) != 1 {
		t.Error("certificates differ")
	}
	loadedKey := s.Cert.PrivateKey.(*ecdsa.PrivateKey)
	if !loadedKey.Equal(key) {
		t.Error("key differs")
	}
	checkPrivate(t, dir)
}

func TestLoadNotEnrolled(t *testing.T) {
	if _, err := Load(t.TempDir()); !errors.Is(err, ErrNotEnrolled) {
		t.Errorf("got %v, want ErrNotEnrolled", err)
	}
}

func TestSaveCertAndCmdKeys(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agent")
	cert, key := selfSigned(t, "0192f1c4-7e2a-7b3c-9d4e-5f6a7b8c9d0e", false)
	ca, _ := selfSigned(t, "SmartCluster CA test", true)
	id := &enroll.Identity{NodeID: "0192f1c4-7e2a-7b3c-9d4e-5f6a7b8c9d0e", Key: key, Cert: cert, CA: ca}
	if err := Save(dir, "10.0.0.1:8444", id); err != nil {
		t.Fatal(err)
	}

	newCert, newKey := selfSigned(t, "0192f1c4-7e2a-7b3c-9d4e-5f6a7b8c9d0e", false)
	if err := SaveCert(dir, newKey, newCert); err != nil {
		t.Fatal(err)
	}
	keys := []proto.PubKey{{KeyID: "cmd-2", Key: "new"}}
	if err := SaveCmdKeys(dir, keys); err != nil {
		t.Fatal(err)
	}
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Cert.PrivateKey.(*ecdsa.PrivateKey).Equal(newKey) || string(s.Cert.Certificate[0]) != string(newCert.Raw) {
		t.Error("certificate was not replaced")
	}
	if len(s.CmdKeys) != 1 || s.CmdKeys[0].KeyID != "cmd-2" || s.Control != "10.0.0.1:8444" || s.Dir != dir {
		t.Errorf("config %+v", s.Config)
	}
}
