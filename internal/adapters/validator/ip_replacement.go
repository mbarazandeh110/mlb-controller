package validator

import (
	"fmt"
	"net"

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
	for _, netConf := range cfg.GlobalIPReplacementList.Net {
		for i, n := range netConf.Nets {
			if !util.IsValidIP(n.Source) || !util.IsValidIP(n.Target) {
				return fmt.Errorf("invalid IP in global_ip_replacement_list.net: source=%s, target=%s", n.Source, n.Target)
			}
			maxMask := 32
			if parsed := net.ParseIP(n.Source); parsed != nil && parsed.To4() == nil {
				maxMask = 128
			}
			if n.Mask < 0 || n.Mask > maxMask {
				return fmt.Errorf("global_ip_replacement_list.net.mask must be between 0 and %d", maxMask)
			}
			for j, n2 := range netConf.Nets {
				if i == j {
					continue
				}
				if util.IsNetworkOverlap(n.Source, n.Mask, n2.Source, n2.Mask) {
					return fmt.Errorf("global_ip_replacement_list.net source '%s/%d' and '%s/%d' must not overlap", n.Source, n.Mask, n2.Source, n2.Mask)
				}
			}
		}
	}

	// Validate IPs (uniqueness scoped per group)
	for _, ip := range cfg.GlobalIPReplacementList.IP {
		sourceIPs := make(map[string]struct{})
		for _, i := range ip.IPs {
			if !util.IsValidIP(i.Source) || !util.IsValidIP(i.Target) {
				return fmt.Errorf("invalid IP in global_ip_replacement_list.ip: source=%s, target=%s", i.Source, i.Target)
			}
			if _, exists := sourceIPs[i.Source]; exists {
				return fmt.Errorf("global_ip_replacement_list.ip source '%s' must be unique in group '%s'", i.Source, ip.Name)
			}
			sourceIPs[i.Source] = struct{}{}
		}
	}

	return nil
}
