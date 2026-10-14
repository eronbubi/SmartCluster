package enroll

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"

	"github.com/eronbubi/smartcluster/pkg/proto"
)

// Path is the enrollment endpoint on the agent port of the control center.
const Path = "/agent/v1/enroll"

// NonceSize is the length of the agent's nonce in bytes.
const NonceSize = 32

// Request is sent by the agent. Byte fields are base64 in JSON.
type Request struct {
	CSR      []byte `json:"csr"` // PKCS#10, DER
	Nonce    []byte `json:"nonce"`
	Proof    []byte `json:"proof"`
	Hostname string `json:"hostname"`
}

// Response is returned by the control center on success.
type Response struct {
	NodeID     string         `json:"node_id"`
	ClientCert []byte         `json:"client_cert"` // DER
	CACert     []byte         `json:"ca_cert"`     // DER
	CmdPubKeys []proto.PubKey `json:"cmd_pubkeys"`
	Proof      []byte         `json:"proof"`
}

// ErrorResponse is returned with any non-2xx status.
type ErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// AgentProof proves knowledge of the join code, bound to the server
// certificate the agent saw, its CSR and its nonce.
func AgentProof(key, serverCertDER, csrDER, nonce []byte) []byte {
	return proof(key, "agent", serverCertDER, csrDER, nonce)
}

// ServerProof proves knowledge of the join code, bound to the same server
// certificate, the issued client certificate and the agent's nonce.
func ServerProof(key, serverCertDER, clientCertDER, nonce []byte) []byte {
	return proof(key, "server", serverCertDER, clientCertDER, nonce)
}

func proof(key []byte, label string, serverCertDER, object, nonce []byte) []byte {
	certHash := sha256.Sum256(serverCertDER)
	objectHash := sha256.Sum256(object)
	m := hmac.New(sha256.New, key)
	// Every field is length-prefixed so that no two inputs share an encoding.
	for _, field := range [][]byte{[]byte(label), certHash[:], objectHash[:], nonce} {
		var n [4]byte
		binary.BigEndian.PutUint32(n[:], uint32(len(field))) //nolint:gosec // G115: fields are hashes, a label and a 32-byte nonce
		m.Write(n[:])
		m.Write(field)
	}
	return m.Sum(nil)
}
