package validator

import (
	"fmt"
	"mlb-controller/internal/domain/config"
	"net"
)

// LoadBalancerValidator validates common properties of load balancer configurations.
type LoadBalancerValidator struct{}

func (v *LoadBalancerValidator) Validate(cfg *config.Config) error {
	// Validate unique names across all load balancers
	names := make(map[string]struct{})
	for _, lb := range cfg.LoadBalancers.Nginx {
		if lb.Name == "" {
			return fmt.Errorf("loadbalancers.nginx.name is required")
		}
		if _, exists := names[lb.Name]; exists {
			return fmt.Errorf("loadbalancers.nginx.name '%s' must be unique", lb.Name)
		}
		names[lb.Name] = struct{}{}
	}
	for _, lb := range cfg.LoadBalancers.Envoy {
		if lb.Name == "" {
			return fmt.Errorf("loadbalancers.envoy.name is required")
		}
		if _, exists := names[lb.Name]; exists {
			return fmt.Errorf("loadbalancers.envoy.name '%s' must be unique", lb.Name)
		}
		names[lb.Name] = struct{}{}
	}

	// Validate Nginx load balancers
	for _, lb := range cfg.LoadBalancers.Nginx {
		if err := v.validateLoadBalancer(lb, cfg.GlobalIPReplacementList); err != nil {
			return fmt.Errorf("loadbalancers.nginx '%s': %w", lb.Name, err)
		}
	}

	// Validate Envoy load balancers
	for _, lb := range cfg.LoadBalancers.Envoy {
		if err := v.validateLoadBalancer(lb, cfg.GlobalIPReplacementList); err != nil {
			return fmt.Errorf("loadbalancers.envoy '%s': %w", lb.Name, err)
		}
	}

	return nil
}

func (v *LoadBalancerValidator) validateLoadBalancer(lb config.LoadBalancerConfig, globalList config.GlobalIPReplacementList) error {
	// Validate addresses
	for _, addr := range lb.GetAddresses() {
		if !isValidIP(addr.IP) {
			return fmt.Errorf("invalid IP in addresses: %s", addr.IP)
		}
		if addr.Port < 0 || addr.Port > 65535 {
			return fmt.Errorf("port must be between 0 and 65535: %d", addr.Port)
		}
		if !isValidProtocol(addr.Protocol) {
			return fmt.Errorf("protocol must be one of: http, https, grpc; got: %s", addr.Protocol)
		}
		if addr.Protocol == "https" && addr.Hostname == "" {
			return fmt.Errorf("hostname is required for https protocol")
		}
		if addr.Hostname != "" && !isValidDomain(addr.Hostname) {
			return fmt.Errorf("invalid hostname: %s", addr.Hostname)
		}
	}

	// Validate IP replacement
	if lb.GetIPReplacement() {
		// Validate nets
		sourceNets := make(map[string]struct{})
		for _, net := range lb.GetIPReplacementList().Nets {
			if !isValidIP(net.Source) || !isValidIP(net.Target) {
				return fmt.Errorf("invalid IP in nets: source=%s, target=%s", net.Source, net.Target)
			}
			if net.Mask < 0 || net.Mask > 32 {
				return fmt.Errorf("nets.mask must be between 0 and 32")
			}
			sourceNet := fmt.Sprintf("%s/%d", net.Source, net.Mask)
			if _, exists := sourceNets[sourceNet]; exists {
				return fmt.Errorf("nets source '%s' must not overlap within the same load balancer", sourceNet)
			}
			sourceNets[sourceNet] = struct{}{}
		}

		// Validate IPs
		sourceIPs := make(map[string]struct{})
		for _, ip := range lb.GetIPReplacementList().IPs {
			if !isValidIP(ip.Source) || !isValidIP(ip.Target) {
				return fmt.Errorf("invalid IP in ips: source=%s, target=%s", ip.Source, ip.Target)
			}
			if _, exists := sourceIPs[ip.Source]; exists {
				return fmt.Errorf("ips source '%s' must be unique within the same load balancer", ip.Source)
			}
			sourceIPs[ip.Source] = struct{}{}
		}

		// Validate global references
		globalNetNames := make(map[string][]config.NetConfig)
		for _, net := range globalList.Net {
			globalNetNames[net.Name] = net.Nets
		}
		for _, name := range lb.GetIPReplacementList().GlobalNets {
			if _, exists := globalNetNames[name]; !exists {
				return fmt.Errorf("global_nets '%s' does not exist in global_ip_replacement_list.net", name)
			}
		}

		globalIPNames := make(map[string][]config.IPConfig)
		for _, ip := range globalList.IP {
			globalIPNames[ip.Name] = ip.IPs
		}
		for _, name := range lb.GetIPReplacementList().GlobalIPs {
			if _, exists := globalIPNames[name]; !exists {
				return fmt.Errorf("global_ips '%s' does not exist in global_ip_replacement_list.ip", name)
			}
		}

		// Check for overlap between nets and global_nets
		for _, name := range lb.GetIPReplacementList().GlobalNets {
			globalNets, exists := globalNetNames[name]
			if !exists {
				continue // Already validated above
			}
			for _, globalNet := range globalNets {
				globalNetStr := fmt.Sprintf("%s/%d", globalNet.Source, globalNet.Mask)
				for _, localNet := range lb.GetIPReplacementList().Nets {
					localNetStr := fmt.Sprintf("%s/%d", localNet.Source, localNet.Mask)
					if isNetworkOverlap(globalNet.Source, globalNet.Mask, localNet.Source, localNet.Mask) {
						return fmt.Errorf("network overlap detected: global_nets '%s' overlaps with nets '%s' in load balancer", globalNetStr, localNetStr)
					}
				}
			}
		}

		// Check for overlap between ips and global_ips
		for _, name := range lb.GetIPReplacementList().GlobalIPs {
			globalIPs, exists := globalIPNames[name]
			if !exists {
				continue // Already validated above
			}
			for _, globalIP := range globalIPs {
				for _, localIP := range lb.GetIPReplacementList().IPs {
					if globalIP.Source == localIP.Source {
						return fmt.Errorf("IP overlap detected: global_ips '%s' overlaps with ips '%s' in load balancer", globalIP.Source, localIP.Source)
					}
				}
			}
		}
	}

	return nil
}

// isNetworkOverlap checks if two networks overlap based on their IP and mask.
func isNetworkOverlap(ip1 string, mask1 int, ip2 string, mask2 int) bool {
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
