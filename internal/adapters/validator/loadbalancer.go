package validator

import (
	"fmt"
	"mlb-controller/internal/domain/config"
	"mlb-controller/internal/util"
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
		if !util.IsValidIP(addr.IP) {
			return fmt.Errorf("invalid IP in addresses: %s", addr.IP)
		}
		if !util.IsValidPort(addr.Port) {
			return fmt.Errorf("port must be between 0 and 65535: %d", addr.Port)
		}
		if !util.IsValidProtocol(addr.Protocol) {
			return fmt.Errorf("protocol must be one of: http, https, grpc; got: %s", addr.Protocol)
		}
		if addr.Protocol == "https" && addr.Hostname == "" {
			return fmt.Errorf("hostname is required for https protocol")
		}
		if addr.Hostname != "" && !util.IsValidDomain(addr.Hostname) {
			return fmt.Errorf("invalid hostname: %s", addr.Hostname)
		}
	}

	// Validate IP replacement
	if lb.GetIPReplacement() {
		if err := util.ValidateIPReplacementList(lb.GetIPReplacementList(), globalList); err != nil {
			return err
		}

		// Check for overlap between nets and global_nets
		for _, name := range lb.GetIPReplacementList().GlobalNets {
			for _, net := range globalList.Net {
				if net.Name != name {
					continue
				}
				for _, globalNet := range net.Nets {
					globalNetStr := fmt.Sprintf("%s/%d", globalNet.Source, globalNet.Mask)
					for _, localNet := range lb.GetIPReplacementList().Nets {
						localNetStr := fmt.Sprintf("%s/%d", localNet.Source, localNet.Mask)
						if util.IsNetworkOverlap(globalNet.Source, globalNet.Mask, localNet.Source, localNet.Mask) {
							return fmt.Errorf("network overlap detected: global_nets '%s' overlaps with nets '%s' in load balancer", globalNetStr, localNetStr)
						}
					}
				}
			}
		}

		// Check for overlap between ips and global_ips
		for _, name := range lb.GetIPReplacementList().GlobalIPs {
			for _, ip := range globalList.IP {
				if ip.Name != name {
					continue
				}
				for _, globalIP := range ip.IPs {
					for _, localIP := range lb.GetIPReplacementList().IPs {
						if globalIP.Source == localIP.Source {
							return fmt.Errorf("IP overlap detected: global_ips '%s' overlaps with ips '%s' in load balancer", globalIP.Source, localIP.Source)
						}
					}
				}
			}
		}
	}

	return nil
}
