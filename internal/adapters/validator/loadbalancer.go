package validator

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"mlb-controller/internal/domain/config"
	"mlb-controller/internal/util"
	"os"
	"time"
)

// LoadBalancerValidator validates general load balancer configuration.
type LoadBalancerValidator struct{}

func (v *LoadBalancerValidator) Validate(cfg *config.Config) error {
	names := make(map[string]struct{})
	for _, lb := range cfg.LoadBalancers.LoadBalancers {
		name := lb.GetName()
		if name == "" {
			return fmt.Errorf("loadbalancers.%s.name is required", lb.GetType())
		}
		if _, exists := names[name]; exists {
			return fmt.Errorf("loadbalancers.%s.name '%s' must be unique across all loadbalancers", lb.GetType(), name)
		}
		names[name] = struct{}{}

		if lb.GetHostName() != "" && !util.IsValidDomain(lb.GetHostName()) {
			return fmt.Errorf("loadbalancers.%s '%s': invalid hostname: %s", lb.GetType(), lb.GetName(), lb.GetHostName())
		}

		// Validate addresses
		if err := v.validateAddresses(lb.GetAddresses(), name, lb.GetType()); err != nil {
			return err
		}
		// Validate Certs
		if err := v.validateCert(lb); err != nil {
			return err
		}

		// Validate IP replacement configuration for the load balancer.
		// Ensures that if ip_replacement is enabled, ip_replacement_list contains at least one rule.
		if lb.GetIPReplacement() {
			ipList := lb.GetIPReplacementList()
			if len(ipList.GlobalIPs) == 0 && len(ipList.GlobalNets) == 0 && len(ipList.Nets) == 0 && len(ipList.IPs) == 0 {
				return fmt.Errorf("loadbalancers.%s '%s': ip_replacement_list is empty; at least one of nets, ips, global_nets, or global_ips must be provided when ip_replacement is enabled", lb.GetType(), lb.GetName())
			}
			if err := v.validateIPReplacementList(ipList, cfg.GlobalIPReplacementList, name, lb.GetType()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (v *LoadBalancerValidator) validateAddresses(addresses []config.AddressConfig, name, lbType string) error {
	for _, addr := range addresses {
		if !util.IsValidIP(addr.IP) {
			return fmt.Errorf("loadbalancers.%s '%s': invalid IP in addresses: %s", lbType, name, addr.IP)
		}
		if !util.IsValidPort(addr.Port) {
			return fmt.Errorf("loadbalancers.%s '%s': port must be between 0 and 65535: %d", lbType, name, addr.Port)
		}
	}
	return nil
}

func (v *LoadBalancerValidator) validateCert(lb config.LoadBalancerConfig) error {
	// Ensure both cert_file and key_file are provided together
	if (lb.GetCertFile() != "" && lb.GetKeyFile() == "") || (lb.GetCertFile() == "" && lb.GetKeyFile() != "") {
		return fmt.Errorf("loadbalancers.%s '%s': both cert_file and key_file must be provided together", lb.GetType(), lb.GetName())
	}

	// Check existence of cert_file and key_file
	if lb.GetCertFile() != "" && !util.IsFileExists(lb.GetCertFile()) {
		return fmt.Errorf("loadbalancers.%s '%s': cert_file %s does not exist", lb.GetType(), lb.GetName(), lb.GetCertFile())
	}
	if lb.GetKeyFile() != "" && !util.IsFileExists(lb.GetKeyFile()) {
		return fmt.Errorf("loadbalancers.%s '%s': key_file %s does not exist", lb.GetType(), lb.GetName(), lb.GetKeyFile())
	}

	// Validate certificate and key pair
	if lb.GetCertFile() != "" {
		if err := validateCertKey(lb.GetCertFile(), lb.GetKeyFile(), lb.GetHostName()); err != nil {
			return fmt.Errorf("loadbalancers.%s '%s': invalid certificate: %w", lb.GetType(), lb.GetName(), err)
		}
	}

	// Validate CA file
	if lb.GetCAFile() != "" {
		if !util.IsFileExists(lb.GetCAFile()) {
			return fmt.Errorf("loadbalancers.%s '%s': ca_file %s does not exist", lb.GetType(), lb.GetName(), lb.GetCAFile())
		}
		if err := validateCA(lb.GetCAFile(), lb.GetCertFile(), lb.GetHostName()); err != nil {
			return fmt.Errorf("loadbalancers.%s '%s': invalid ca_file: %w", lb.GetType(), lb.GetName(), err)
		}
	}

	return nil
}

func (v *LoadBalancerValidator) validateIPReplacementList(list config.IPReplacementList, globalList config.GlobalIPReplacementList, name, lbType string) error {
	if err := util.ValidateIPReplacementList(list, globalList); err != nil {
		return fmt.Errorf("loadbalancers.%s '%s': %w", lbType, name, err)
	}

	// Check for overlaps with global lists
	for _, globalNetName := range list.GlobalNets {
		for _, globalNet := range globalList.Net {
			if globalNet.Name == globalNetName {
				for _, gNet := range globalNet.Nets {
					for _, lNet := range list.Nets {
						if util.IsNetworkOverlap(gNet.Source, gNet.Mask, lNet.Source, lNet.Mask) {
							return fmt.Errorf("network overlap detected: global_nets '%s/%d' overlaps with nets '%s/%d' in load balancer '%s'", gNet.Source, gNet.Mask, lNet.Source, lNet.Mask, name)
						}
					}
				}
			}
		}
	}

	for _, globalIPName := range list.GlobalIPs {
		for _, globalIP := range globalList.IP {
			if globalIP.Name == globalIPName {
				for _, gIP := range globalIP.IPs {
					for _, lIP := range list.IPs {
						if gIP.Source == lIP.Source {
							return fmt.Errorf("IP overlap detected: global_ips '%s' overlaps with ips '%s' in load balancer '%s'", gIP.Source, lIP.Source, name)
						}
					}
				}
			}
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
