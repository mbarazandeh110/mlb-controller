package validator

import (
	"fmt"
	"mlb-controller/internal/domain/config"
	"mlb-controller/internal/util"
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
			if !util.IsValidIP(n.Source) || !util.IsValidIP(n.Target) {
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
			if !util.IsValidIP(i.Source) || !util.IsValidIP(i.Target) {
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
