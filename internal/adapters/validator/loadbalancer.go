package validator

import (
	"fmt"
	"mlb-controller/internal/domain/config"
	"mlb-controller/internal/util"
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

		// Validate addresses
		if err := v.validateAddresses(lb.GetAddresses(), name, lb.GetType()); err != nil {
			return err
		}
		// Validate Certs
		if err := v.validateCert(lb); err != nil {
			return err
		}

		// Validate IP replacement if enabled
		if lb.GetIPReplacement() {
			if (len(lb.GetIPReplacementList().GlobalIPs) == 0) && (len(lb.GetIPReplacementList().GlobalNets) == 0) &&
				(len(lb.GetIPReplacementList().Nets) == 0) && (len(lb.GetIPReplacementList().IPs) == 0) {
				return fmt.Errorf("loadbalancers.%s '%s': ip_replacement_list is empty; at least one of nets, ips, global_nets, or global_ips must be provided when ip_replacement is enabled", lb.GetType(), lb.GetName())
			}
			if err := v.validateIPReplacementList(lb.GetIPReplacementList(), cfg.GlobalIPReplacementList, name, lb.GetType()); err != nil {
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
		// if addr.Protocol == "https" && addr.Hostname == "" {
		// 	return fmt.Errorf("loadbalancers.%s '%s': hostname is only allowed for https protocol (index %d)", lbType, name, i)
		// }
	}
	return nil
}

func (v *LoadBalancerValidator) validateCert(lb config.LoadBalancerConfig) error {

	if (lb.GetCertFile() != "" && lb.GetKeyFile() == "") || (lb.GetCertFile() == "" && lb.GetKeyFile() != "") {
		return fmt.Errorf("loadbalancers.%s '%s': buth of CertFile and KeyFile are required: %s", lb.GetType(), lb.GetName(), lb.GetCertFile())
	}
	if lb.GetCertFile() != "" && !util.IsFileExists(lb.GetCertFile()) {
		return fmt.Errorf("loadbalancers.%s '%s': the CertFile is not exist: %s", lb.GetType(), lb.GetName(), lb.GetCertFile())
	}

	if lb.GetKeyFile() != "" && !util.IsFileExists(lb.GetKeyFile()) {
		return fmt.Errorf("loadbalancers.%s '%s': the KeyFile is not exist: %s", lb.GetType(), lb.GetName(), lb.GetKeyFile())
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
