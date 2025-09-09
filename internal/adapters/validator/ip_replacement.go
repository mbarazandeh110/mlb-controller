package validator

import (
	"fmt"
	"mlb-controller/internal/domain/config"
)

// GlobalIPReplacementValidator validates global IP and network replacement lists.
type GlobalIPReplacementValidator struct{}

func (v *GlobalIPReplacementValidator) Validate(cfg *config.Config) error {
	// Validate unique names
	names := make(map[string]struct{})
	for _, net := range cfg.GlobalIPReplacementList.Net {
		if net.Name == "" {
			return fmt.Errorf("global_ip_replacement_list.net.name is required")
		}
		if _, exists := names[net.Name]; exists {
			return fmt.Errorf("global_ip_replacement_list.net.name '%s' must be unique", net.Name)
		}
		names[net.Name] = struct{}{}
	}
	for _, ip := range cfg.GlobalIPReplacementList.IP {
		if ip.Name == "" {
			return fmt.Errorf("global_ip_replacement_list.ip.name is required")
		}
		if _, exists := names[ip.Name]; exists {
			return fmt.Errorf("global_ip_replacement_list.ip.name '%s' must be unique", ip.Name)
		}
		names[ip.Name] = struct{}{}
	}

	// Validate nets
	sourceNets := make(map[string]struct{})
	for _, net := range cfg.GlobalIPReplacementList.Net {
		for _, n := range net.Nets {
			if !isValidIP(n.Source) || !isValidIP(n.Target) {
				return fmt.Errorf("invalid IP in global_ip_replacement_list.net: source=%s, target=%s", n.Source, n.Target)
			}
			if n.Mask < 0 || n.Mask > 32 {
				return fmt.Errorf("global_ip_replacement_list.net.mask must be between 0 and 32")
			}
			sourceNet := fmt.Sprintf("%s/%d", n.Source, n.Mask)
			if _, exists := sourceNets[sourceNet]; exists {
				return fmt.Errorf("global_ip_replacement_list.net source '%s' must not overlap", sourceNet)
			}
			sourceNets[sourceNet] = struct{}{}
		}
	}

	// Validate IPs
	sourceIPs := make(map[string]struct{})
	for _, ip := range cfg.GlobalIPReplacementList.IP {
		for _, i := range ip.IPs {
			if !isValidIP(i.Source) || !isValidIP(i.Target) {
				return fmt.Errorf("invalid IP in global_ip_replacement_list.ip: source=%s, target=%s", i.Source, i.Target)
			}
			if _, exists := sourceIPs[i.Source]; exists {
				return fmt.Errorf("global_ip_replacement_list.ip source '%s' must be unique", i.Source)
			}
			sourceIPs[i.Source] = struct{}{}
		}
	}

	return nil
}

// Helper function to validate IP replacement lists
func validateIPReplacementList(list config.IPReplacementList, globalList config.GlobalIPReplacementList) error {
	sourceNets := make(map[string]struct{})
	for _, net := range list.Nets {
		if !isValidIP(net.Source) || !isValidIP(net.Target) {
			return fmt.Errorf("invalid IP in nets: source=%s, target=%s", net.Source, net.Target)
		}
		if net.Mask < 0 || net.Mask > 32 {
			return fmt.Errorf("nets.mask must be between 0 and 32")
		}
		sourceNet := fmt.Sprintf("%s/%d", net.Source, net.Mask)
		if _, exists := sourceNets[sourceNet]; exists {
			return fmt.Errorf("nets source '%s' must not overlap", sourceNet)
		}
		sourceNets[sourceNet] = struct{}{}
	}

	sourceIPs := make(map[string]struct{})
	for _, ip := range list.IPs {
		if !isValidIP(ip.Source) || !isValidIP(ip.Target) {
			return fmt.Errorf("invalid IP in ips: source=%s, target=%s", ip.Source, ip.Target)
		}
		if _, exists := sourceIPs[ip.Source]; exists {
			return fmt.Errorf("ips source '%s' must be unique", ip.Source)
		}
		sourceIPs[ip.Source] = struct{}{}
	}

	// Validate global references
	globalNetNames := make(map[string]struct{})
	for _, net := range globalList.Net {
		globalNetNames[net.Name] = struct{}{}
	}
	for _, name := range list.GlobalNets {
		if _, exists := globalNetNames[name]; !exists {
			return fmt.Errorf("global_nets '%s' does not exist in global_ip_replacement_list.net", name)
		}
	}

	globalIPNames := make(map[string]struct{})
	for _, ip := range globalList.IP {
		globalIPNames[ip.Name] = struct{}{}
	}
	for _, name := range list.GlobalIPs {
		if _, exists := globalIPNames[name]; !exists {
			return fmt.Errorf("global_ips '%s' does not exist in global_ip_replacement_list.ip", name)
		}
	}

	return nil
}
