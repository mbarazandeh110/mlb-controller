package validator

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"mlb-controller/internal/domain/config"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mockLoadBalancerConfig is a mock implementation of LoadBalancerConfig for testing.
type mockLoadBalancerConfig struct {
	name              string
	addresses         []config.AddressConfig
	ipReplacement     bool
	ipReplacementList config.IPReplacementList
	lbType            string
	hostname          string
	certFile          string
	keyFile           string
	caFile            string
	requestPoolSize   int
}

func (m *mockLoadBalancerConfig) GetName() string                      { return m.name }
func (m *mockLoadBalancerConfig) GetAddresses() []config.AddressConfig { return m.addresses }
func (m *mockLoadBalancerConfig) GetIPReplacement() bool               { return m.ipReplacement }
func (m *mockLoadBalancerConfig) GetIPReplacementList() config.IPReplacementList {
	return m.ipReplacementList
}
func (m *mockLoadBalancerConfig) GetType() string         { return m.lbType }
func (m *mockLoadBalancerConfig) GetHostName() string     { return m.hostname }
func (m *mockLoadBalancerConfig) GetProtocol() string     { return "https" }
func (m *mockLoadBalancerConfig) GetCertFile() string     { return m.certFile }
func (m *mockLoadBalancerConfig) GetKeyFile() string      { return m.keyFile }
func (m *mockLoadBalancerConfig) GetCAFile() string       { return m.caFile }
func (m *mockLoadBalancerConfig) GetRequestPoolSize() int { return m.requestPoolSize }
func (m *mockLoadBalancerConfig) SetDefaults(time.Duration, int) config.LoadBalancerConfig {
	return m
}

// generateTestCert generates a test certificate, private key, and CA for testing.
func generateTestCert(t *testing.T, hostname string, notAfter time.Time, rsaBits int) (certPath, keyPath, caPath string) {
	// Generate CA
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Test CA"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate CA key: %v", err)
	}
	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("Failed to create CA certificate: %v", err)
	}
	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		t.Fatalf("Failed to parse CA certificate: %v", err)
	}

	// Generate server certificate
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: hostname},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     notAfter,
		DNSNames:     []string{hostname},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverKey, err := rsa.GenerateKey(rand.Reader, rsaBits)
	if err != nil {
		t.Fatalf("Failed to generate server key: %v", err)
	}
	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("Failed to create server certificate: %v", err)
	}

	// Write CA to file
	caFile, err := os.CreateTemp("", "ca-*.pem")
	if err != nil {
		t.Fatalf("Failed to create temp CA file: %v", err)
	}
	defer caFile.Close()
	caPath = caFile.Name()
	if err := pem.Encode(caFile, &pem.Block{Type: "CERTIFICATE", Bytes: caCertDER}); err != nil {
		t.Fatalf("Failed to write CA certificate: %v", err)
	}

	// Write server certificate to file
	certFile, err := os.CreateTemp("", "cert-*.pem")
	if err != nil {
		t.Fatalf("Failed to create temp cert file: %v", err)
	}
	defer certFile.Close()
	certPath = certFile.Name()
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER}); err != nil {
		t.Fatalf("Failed to write server certificate: %v", err)
	}

	// Write server key to file
	keyFile, err := os.CreateTemp("", "key-*.pem")
	if err != nil {
		t.Fatalf("Failed to create temp key file: %v", err)
	}
	defer keyFile.Close()
	keyPath = keyFile.Name()
	keyBytes, err := x509.MarshalPKCS8PrivateKey(serverKey)
	if err != nil {
		t.Fatalf("Failed to marshal server key: %v", err)
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}); err != nil {
		t.Fatalf("Failed to write server key: %v", err)
	}

	return certPath, keyPath, caPath
}

func TestLoadBalancerValidator_Validate(t *testing.T) {
	validator := &LoadBalancerValidator{}

	t.Run("ValidConfig", func(t *testing.T) {
		cfg := &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					&mockLoadBalancerConfig{
						name:      "test-lb",
						lbType:    "nginx",
						addresses: []config.AddressConfig{{IP: "192.168.1.1", Port: 8080}},
					},
				},
			},
		}
		err := validator.Validate(cfg)
		assert.NoError(t, err)
	})

	t.Run("DuplicateName", func(t *testing.T) {
		cfg := &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					&mockLoadBalancerConfig{name: "test-lb", lbType: "nginx"},
					&mockLoadBalancerConfig{name: "test-lb", lbType: "envoy"},
				},
			},
		}
		err := validator.Validate(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "name 'test-lb' must be unique")
	})

	t.Run("EmptyName", func(t *testing.T) {
		cfg := &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					&mockLoadBalancerConfig{name: "", lbType: "nginx"},
				},
			},
		}
		err := validator.Validate(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("InvalidIP", func(t *testing.T) {
		cfg := &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					&mockLoadBalancerConfig{
						name:      "test-lb",
						lbType:    "nginx",
						addresses: []config.AddressConfig{{IP: "invalid", Port: 8080}},
					},
				},
			},
		}
		err := validator.Validate(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid IP")
	})

	t.Run("InvalidPort", func(t *testing.T) {
		cfg := &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					&mockLoadBalancerConfig{
						name:      "test-lb",
						lbType:    "nginx",
						addresses: []config.AddressConfig{{IP: "192.168.1.1", Port: 65536}},
					},
				},
			},
		}
		err := validator.Validate(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "port must be between 0 and 65535")
	})

	t.Run("EmptyIPReplacementList", func(t *testing.T) {
		cfg := &config.Config{
			LoadBalancers: config.LoadBalancersConfig{
				LoadBalancers: []config.LoadBalancerConfig{
					&mockLoadBalancerConfig{
						name:              "test-lb",
						lbType:            "nginx",
						ipReplacement:     true,
						ipReplacementList: config.IPReplacementList{},
					},
				},
			},
		}
		err := validator.Validate(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "ip_replacement_list is empty")
	})
}

func TestLoadBalancerValidator_validateCert(t *testing.T) {
	validator := &LoadBalancerValidator{}

	// Generate valid test certificate, key, and CA
	validCertPath, validKeyPath, validCAPath := generateTestCert(t, "test.example.com", time.Now().Add(24*time.Hour), 2048)
	defer os.Remove(validCertPath)
	defer os.Remove(validKeyPath)
	defer os.Remove(validCAPath)

	// Generate expired certificate
	expiredCertPath, expiredKeyPath, _ := generateTestCert(t, "test.example.com", time.Now().Add(-1*time.Hour), 2048)
	defer os.Remove(expiredCertPath)
	defer os.Remove(expiredKeyPath)

	// Generate certificate with weak key
	weakCertPath, weakKeyPath, _ := generateTestCert(t, "test.example.com", time.Now().Add(24*time.Hour), 1024)
	defer os.Remove(weakCertPath)
	defer os.Remove(weakKeyPath)

	// Generate certificate with wrong hostname
	wrongHostCertPath, wrongHostKeyPath, _ := generateTestCert(t, "wrong.example.com", time.Now().Add(24*time.Hour), 2048)
	defer os.Remove(wrongHostCertPath)
	defer os.Remove(wrongHostKeyPath)

	tests := []struct {
		name        string
		config      config.LoadBalancerConfig
		wantErr     bool
		errContains string
	}{
		// {
		// 	name: "ValidCertAndKey",
		// 	config: &mockLoadBalancerConfig{
		// 		name:     "test-lb",
		// 		lbType:   "nginx",
		// 		hostname: "test.example.com",
		// 		certFile: validCertPath,
		// 		keyFile:  validKeyPath,
		// 		caFile:   validCAPath,
		// 	},
		// 	wantErr: true,
		// },
		{
			name: "MissingKeyFile",
			config: &mockLoadBalancerConfig{
				name:     "test-lb",
				lbType:   "nginx",
				certFile: validCertPath,
			},
			wantErr:     true,
			errContains: "both cert_file and key_file must be provided together",
		},
		{
			name: "MissingCertFile",
			config: &mockLoadBalancerConfig{
				name:    "test-lb",
				lbType:  "nginx",
				keyFile: validKeyPath,
			},
			wantErr:     true,
			errContains: "both cert_file and key_file must be provided together",
		},
		{
			name: "NonExistentCertFile",
			config: &mockLoadBalancerConfig{
				name:     "test-lb",
				lbType:   "nginx",
				certFile: "/nonexistent/cert.pem",
				keyFile:  validKeyPath,
			},
			wantErr:     true,
			errContains: "cert_file /nonexistent/cert.pem does not exist",
		},
		{
			name: "NonExistentKeyFile",
			config: &mockLoadBalancerConfig{
				name:     "test-lb",
				lbType:   "nginx",
				certFile: validCertPath,
				keyFile:  "/nonexistent/key.pem",
			},
			wantErr:     true,
			errContains: "key_file /nonexistent/key.pem does not exist",
		},
		{
			name: "NonExistentCAFile",
			config: &mockLoadBalancerConfig{
				name:     "test-lb",
				lbType:   "nginx",
				certFile: validCertPath,
				keyFile:  validKeyPath,
				caFile:   "/nonexistent/ca.pem",
			},
			wantErr:     true,
			errContains: "ca_file /nonexistent/ca.pem does not exist",
		},
		{
			name: "ExpiredCertificate",
			config: &mockLoadBalancerConfig{
				name:     "test-lb",
				lbType:   "nginx",
				hostname: "test.example.com",
				certFile: expiredCertPath,
				keyFile:  expiredKeyPath,
			},
			wantErr:     true,
			errContains: "certificate has expired",
		},
		{
			name: "WeakKey",
			config: &mockLoadBalancerConfig{
				name:     "test-lb",
				lbType:   "nginx",
				hostname: "test.example.com",
				certFile: weakCertPath,
				keyFile:  weakKeyPath,
			},
			wantErr:     true,
			errContains: "insecure RSA key length: 1024 bits",
		},
		{
			name: "WrongHostname",
			config: &mockLoadBalancerConfig{
				name:     "test-lb",
				lbType:   "nginx",
				hostname: "test.example.com",
				certFile: wrongHostCertPath,
				keyFile:  wrongHostKeyPath,
			},
			wantErr:     true,
			errContains: "certificate does not match hostname test.example.com",
		},
		{
			name: "InvalidCA",
			config: &mockLoadBalancerConfig{
				name:     "test-lb",
				lbType:   "nginx",
				hostname: "test.example.com",
				certFile: validCertPath,
				keyFile:  validKeyPath,
				caFile:   writeTempFile(t, []byte("invalid CA content")),
			},
			wantErr:     true,
			errContains: "invalid CA certificate",
		},
		// {
		// 	name: "InvalidCertChain",
		// 	config: &mockLoadBalancerConfig{
		// 		name:     "test-lb",
		// 		lbType:   "nginx",
		// 		hostname: "test.example.com",
		// 		certFile: validCertPath,
		// 		keyFile:  validKeyPath,
		// 		caFile:   writeTempFile(t, generateInvalidCACert(t)),
		// 	},
		// 	wantErr:     true,
		// 	errContains: "certificate verification failed",
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateCert(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoadBalancerValidator_validateIPReplacementList(t *testing.T) {
	validator := &LoadBalancerValidator{}

	t.Run("ValidIPReplacementList", func(t *testing.T) {
		list := config.IPReplacementList{
			Nets:       []config.NetConfig{{Source: "192.168.1.0", Target: "10.0.0.0", Mask: 24}},
			IPs:        []config.IPConfig{{Source: "192.168.1.1", Target: "10.0.0.1"}},
			GlobalNets: []string{"global-net"},
			GlobalIPs:  []string{"global-ip"},
		}
		globalList := config.GlobalIPReplacementList{
			Net: []config.GlobalNetReplacement{{Name: "global-net", Nets: []config.NetConfig{{Source: "10.0.1.0", Target: "172.16.0.0", Mask: 24}}}},
			IP:  []config.GlobalIPReplacement{{Name: "global-ip", IPs: []config.IPConfig{{Source: "10.0.1.1", Target: "172.16.0.1"}}}},
		}
		err := validator.validateIPReplacementList(list, globalList, "test-lb", "nginx")
		assert.NoError(t, err)
	})

	t.Run("NetworkOverlap", func(t *testing.T) {
		list := config.IPReplacementList{
			Nets:       []config.NetConfig{{Source: "192.168.1.0", Target: "10.0.0.0", Mask: 24}},
			GlobalNets: []string{"global-net"},
		}
		globalList := config.GlobalIPReplacementList{
			Net: []config.GlobalNetReplacement{{Name: "global-net", Nets: []config.NetConfig{{Source: "192.168.1.0", Target: "172.16.0.0", Mask: 24}}}},
		}
		err := validator.validateIPReplacementList(list, globalList, "test-lb", "nginx")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "network overlap detected")
	})

	t.Run("IPOverlap", func(t *testing.T) {
		list := config.IPReplacementList{
			IPs:       []config.IPConfig{{Source: "192.168.1.1", Target: "10.0.0.1"}},
			GlobalIPs: []string{"global-ip"},
		}
		globalList := config.GlobalIPReplacementList{
			IP: []config.GlobalIPReplacement{{Name: "global-ip", IPs: []config.IPConfig{{Source: "192.168.1.1", Target: "172.16.0.1"}}}},
		}
		err := validator.validateIPReplacementList(list, globalList, "test-lb", "nginx")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "IP overlap detected")
	})
}

// writeTempFile creates a temporary file with the given content and returns its path.
func writeTempFile(t *testing.T, content []byte) string {
	file, err := os.CreateTemp("", "test-*.pem")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer file.Close()
	if _, err := file.Write(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	return file.Name()
}

// generateInvalidCACert generates an invalid CA certificate for testing.
func generateInvalidCACert(t *testing.T) []byte {
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Invalid CA"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		IsCA:         false, // Invalid because it's not a CA
	}
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate CA key: %v", err)
	}
	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("Failed to create invalid CA certificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})
}
