package util

import (
	"fmt"
	"mlb-controller/internal/domain/config"
	"net"
	"regexp"
)

// IsValidIP checks if the given string is a valid IP address.
func IsValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

// IsValidProtocol checks if the given protocol is one of http, https, or grpc.
func IsValidProtocol(protocol string) bool {
	return protocol == "http" || protocol == "https" || protocol == "grpc"
}

// IsValidDomain checks if the given hostname is a valid domain name.
func IsValidDomain(hostname string) bool {
	domainRegex := regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
	return domainRegex.MatchString(hostname)
}

// IsValidPort checks if the given port is within the valid range (0-65535).
func IsValidPort(port int) bool {
	return port >= 0 && port <= 65535
}

// IsNetworkOverlap checks if two networks overlap based on their IP and mask.
func IsNetworkOverlap(ip1 string, mask1 int, ip2 string, mask2 int) bool {
	_, net1, err := net.ParseCIDR(fmt.Sprintf("%s/%d", ip1, mask1))
	if err != nil {
		return false
	}
	_, net2, err := net.ParseCIDR(fmt.Sprintf("%s/%d", ip2, mask2))
	if err != nil {
		return false
	}
	return net1.Contains(net2.IP) || net2.Contains(net1.IP)
}

// ValidateIPReplacementList validates IP and network replacement lists.
func ValidateIPReplacementList(list config.IPReplacementList, globalList config.GlobalIPReplacementList) error {
	sourceNets := make(map[string]struct{})
	for _, net := range list.Nets {
		if !IsValidIP(net.Source) || !IsValidIP(net.Target) {
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
		if !IsValidIP(ip.Source) || !IsValidIP(ip.Target) {
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
