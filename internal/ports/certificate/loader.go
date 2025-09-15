package certificate

import (
	"context"
)

// CertificateLoader defines the interface for loading certificate and key content.
// This interface acts as a Port in the Hexagonal Architecture.
type CertificateLoader interface {
	Load(ctx context.Context, certPath, keyPath, caPath, hostname string) (*CertificateContent, error)
}

// CertificateContent holds the actual content of the certificate, key, and CA files.
type CertificateContent struct {
	Cert []byte
	Key  []byte
	CA   []byte
}
