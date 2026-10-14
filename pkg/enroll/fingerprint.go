package enroll

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"strings"
)

// DefaultAgentPort is the control center's port for agents (enrollment and
// the agent WebSocket).
const DefaultAgentPort = "8444"

// Fingerprint returns the SHA-256 fingerprint of a certificate in upper-case
// hex groups of four, for comparison by people (e.g. the cluster CA).
func Fingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	h := strings.ToUpper(hex.EncodeToString(sum[:]))
	groups := make([]string, 0, len(h)/4)
	for i := 0; i < len(h); i += 4 {
		groups = append(groups, h[i:i+4])
	}
	return strings.Join(groups, " ")
}

// ServiceType is the mDNS service under which control centers announce their
// agent listener.
const ServiceType = "_smartcluster._tcp"
