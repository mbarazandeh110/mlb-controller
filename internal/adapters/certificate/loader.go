package certificate

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	certificate_ports "mlb-controller/internal/ports/certificate"
	"mlb-controller/internal/util"
)

// FileLoader is a concrete adapter that loads certificate content from the filesystem.
type FileLoader struct{}

// NewFileLoader creates a new instance of FileLoader.
func NewFileLoader() *FileLoader {
	return &FileLoader{}
}

// Load reads the content of certificate, key, and CA files.
func (f *FileLoader) Load(ctx context.Context, certPath, keyPath, caPath, hostname string) (*certificate_ports.CertificateContent, error) {
	var certContent, keyContent, caContent []byte

	err := validateCert(certPath, keyPath, caPath, hostname)
	if err != nil {
		return nil, err
	}
	if certPath != "" {
		certContent, _ = os.ReadFile(certPath)
		keyContent, _ = os.ReadFile(keyPath)
	}

	if caPath != "" {
		caContent, _ = os.ReadFile(caPath)
	}

	return &certificate_ports.CertificateContent{
		Cert: certContent,
		Key:  keyContent,
		CA:   caContent,
	}, nil
}

func validateCert(certPath, keyPath, caPath, hostname string) error {
	// Ensure both cert_file and key_file are provided together
	if (certPath != "" && keyPath == "") || (certPath == "" && keyPath != "") {
		return fmt.Errorf("both cert_file and key_file must be provided together")
	}

	// Check existence of cert_file and key_file
	if certPath != "" && !util.IsFileExists(certPath) {
		return fmt.Errorf("cert_file %s does not exist", certPath)
	}
	if keyPath != "" && !util.IsFileExists(keyPath) {
		return fmt.Errorf("key_file %s does not exist", keyPath)
	}

	// Validate certificate and key pair
	if certPath != "" {
		if err := validateCertKey(certPath, keyPath, hostname); err != nil {
			return fmt.Errorf("invalid certificate: %w", err)
		}
	}

	// Validate CA file
	if caPath != "" {
		if !util.IsFileExists(caPath) {
			return fmt.Errorf("ca_file %s does not exist", caPath)
		}
		if err := validateCA(caPath, certPath, hostname); err != nil {
			return fmt.Errorf("invalid ca_file: %w", err)
		}
	}

	return nil
}

// validateCA validates that the file contains a valid PEM certificate and verifies the certificate chain if a certificate is provided.
func validateCA(caFilePath, certFilePath, hostname string) error {
	caContent, err := os.ReadFile(caFilePath)
	if err != nil {
		return fmt.Errorf("failed to read ca_file '%s': %w", caFilePath, err)
	}
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caContent) {
		return fmt.Errorf("invalid CA certificate in ca_file '%s'", caFilePath)
	}

	// If a certificate is provided, verify the certificate chain
	if certFilePath != "" {
		certContent, err := os.ReadFile(certFilePath)
		if err != nil {
			return fmt.Errorf("failed to read cert_file '%s': %w", certFilePath, err)
		}
		cert, err := x509.ParseCertificate(certContent)
		if err != nil {
			return fmt.Errorf("failed to parse certificate: %w", err)
		}
		opts := x509.VerifyOptions{
			Roots:     caCertPool,
			DNSName:   hostname,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		}
		if _, err := cert.Verify(opts); err != nil {
			return fmt.Errorf("certificate verification failed: %w", err)
		}
	}

	return nil
}

// validateCertKey validates that the certificate and key files contain a valid PEM certificate and private key,
// checks certificate expiration, hostname, and key strength.
func validateCertKey(certPath, keyPath, hostname string) error {
	// Validate certificate and key content
	certContent, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("failed to read cert_file '%s': %w", certPath, err)
	}
	keyContent, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("failed to read key_file '%s': %w", keyPath, err)
	}

	// Validate certificate and key pair
	cert, err := tls.X509KeyPair(certContent, keyContent)
	if err != nil {
		return fmt.Errorf("invalid certificate or key: %w", err)
	}

	// Parse certificate to perform additional validations
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Check certificate expiration
	if time.Now().After(x509Cert.NotAfter) {
		return fmt.Errorf("certificate has expired: NotAfter=%s", x509Cert.NotAfter)
	}
	if time.Now().Before(x509Cert.NotBefore) {
		return fmt.Errorf("certificate is not yet valid: NotBefore=%s", x509Cert.NotBefore)
	}

	// Verify hostname if provided
	if hostname != "" {
		if err := x509Cert.VerifyHostname(hostname); err != nil {
			return fmt.Errorf("certificate does not match hostname %s: %w", hostname, err)
		}
	}

	// Check key strength (e.g., RSA key length >= 2048 bits)
	if x509Cert.PublicKeyAlgorithm == x509.RSA {
		if rsaKey, ok := x509Cert.PublicKey.(*rsa.PublicKey); ok && rsaKey.N.BitLen() < 2048 {
			return fmt.Errorf("certificate uses insecure RSA key length: %d bits (minimum 2048 required)", rsaKey.N.BitLen())
		}
	}

	return nil
}
