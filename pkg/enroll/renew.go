package enroll

// RenewPath is the endpoint for certificate renewal. It requires the current
// (still valid) client certificate.
const RenewPath = "/agent/v1/renew"

// RenewRequest carries a CSR for a fresh key.
type RenewRequest struct {
	CSR []byte `json:"csr"` // PKCS#10, DER
}

// RenewResponse carries the new client certificate.
type RenewResponse struct {
	ClientCert []byte `json:"client_cert"` // DER
}
