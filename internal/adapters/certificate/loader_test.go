package certificate

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateTestCert(t *testing.T, dir string, rsaBits int, dnsName string, notBefore, notAfter time.Time) (string, string) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, rsaBits)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: dnsName,
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{dnsName},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)

	certPath := filepath.Join(dir, "cert.pem")
	certFile, err := os.Create(certPath)
	require.NoError(t, err)
	defer certFile.Close()
	require.NoError(t, pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}))

	keyPath := filepath.Join(dir, "key.pem")
	keyFile, err := os.Create(keyPath)
	require.NoError(t, err)
	defer keyFile.Close()
	require.NoError(t, pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}))

	return certPath, keyPath
}

func generateCA(t *testing.T, dir string) string {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName: "Test Root CA",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)

	caPath := filepath.Join(dir, "ca.pem")
	caFile, err := os.Create(caPath)
	require.NoError(t, err)
	defer caFile.Close()
	require.NoError(t, pem.Encode(caFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}))

	return caPath
}

func TestFileLoader_Load(t *testing.T) {
	loader := NewFileLoader()
	ctx := context.Background()

	t.Run("Valid Certificate, Key, and CA with matching hostname", func(t *testing.T) {
		tempDir := t.TempDir()
		certPath, keyPath := generateTestCert(t, tempDir, 2048, "example.com", time.Now().Add(-1*time.Hour), time.Now().Add(24*time.Hour))
		caPath := generateCA(t, tempDir)

		content, err := loader.Load(ctx, certPath, keyPath, caPath, "example.com")
		require.NoError(t, err)
		assert.NotEmpty(t, content.Cert)
		assert.NotEmpty(t, content.Key)
		assert.NotEmpty(t, content.CA)
	})

	t.Run("Empty paths return empty content without error", func(t *testing.T) {
		content, err := loader.Load(ctx, "", "", "", "")
		require.NoError(t, err)
		assert.Empty(t, content.Cert)
		assert.Empty(t, content.Key)
		assert.Empty(t, content.CA)
	})

	t.Run("Cert without Key fails validation", func(t *testing.T) {
		tempDir := t.TempDir()
		certPath, _ := generateTestCert(t, tempDir, 2048, "example.com", time.Now().Add(-1*time.Hour), time.Now().Add(24*time.Hour))

		_, err := loader.Load(ctx, certPath, "", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "both cert_file and key_file must be provided together")
	})

	t.Run("Non-existent files fail validation", func(t *testing.T) {
		_, err := loader.Load(ctx, "/nonexistent/cert.pem", "/nonexistent/key.pem", "", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})

	t.Run("Insecure RSA key length (< 2048) rejected", func(t *testing.T) {
		tempDir := t.TempDir()
		certPath, keyPath := generateTestCert(t, tempDir, 1024, "example.com", time.Now().Add(-1*time.Hour), time.Now().Add(24*time.Hour))

		_, err := loader.Load(ctx, certPath, keyPath, "", "example.com")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "insecure RSA key length")
	})

	t.Run("Hostname mismatch rejected", func(t *testing.T) {
		tempDir := t.TempDir()
		certPath, keyPath := generateTestCert(t, tempDir, 2048, "example.com", time.Now().Add(-1*time.Hour), time.Now().Add(24*time.Hour))

		_, err := loader.Load(ctx, certPath, keyPath, "", "wronghost.com")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "SAN does not match hostname")
	})

	t.Run("Expired certificate rejected", func(t *testing.T) {
		tempDir := t.TempDir()
		certPath, keyPath := generateTestCert(t, tempDir, 2048, "example.com", time.Now().Add(-48*time.Hour), time.Now().Add(-24*time.Hour))

		_, err := loader.Load(ctx, certPath, keyPath, "", "example.com")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "has expired")
	})

	t.Run("Invalid CA file rejected", func(t *testing.T) {
		tempDir := t.TempDir()
		invalidCA := filepath.Join(tempDir, "bad_ca.pem")
		require.NoError(t, os.WriteFile(invalidCA, []byte("NOT A PEM CERTIFICATE"), 0644))

		_, err := loader.Load(ctx, "", "", invalidCA, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid CA certificate")
	})
}
