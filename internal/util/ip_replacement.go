package util

import (
	"fmt"
	"mlb-controller/internal/domain/config"
	"net"
)

// ApplyIPReplacement replaces nodeIP based on replacement list. Priority: IPs > Nets > Global.
func ApplyIPReplacement(nodeIP string, lbConfig config.LoadBalancerConfig, globalList config.GlobalIPReplacementList) string {
	replacementList := lbConfig.GetIPReplacementList()

	// First, check individual IPs
	for _, ip := range replacementList.IPs {
		if ip.Source == nodeIP {
			return ip.Target
		}
	}

	// Global IPs
	for _, globalIPName := range replacementList.GlobalIPs {
		for _, gIP := range globalList.IP {
			if gIP.Name == globalIPName {
				for _, ip := range gIP.IPs {
					if ip.Source == nodeIP {
						return ip.Target
					}
				}
			}
		}
	}

	// Then, check nets
	for _, net := range replacementList.Nets {
		if isIPInNet(nodeIP, net.Source, net.Mask) {
			return replaceInNet(nodeIP, net.Source, net.Target, net.Mask)
		}
	}

	// Global Nets
	for _, globalNetName := range replacementList.GlobalNets {
		for _, gNet := range globalList.Net {
			if gNet.Name == globalNetName {
				for _, net := range gNet.Nets {
					if isIPInNet(nodeIP, net.Source, net.Mask) {
						return replaceInNet(nodeIP, net.Source, net.Target, net.Mask)
					}
				}
			}
		}
	}

	return nodeIP // No replacement
}

func isIPInNet(ip, netIP string, mask int) bool {
	_, cidrNet, _ := net.ParseCIDR(netIP + "/" + fmt.Sprint(mask))
	return cidrNet.Contains(net.ParseIP(ip))
}

func replaceInNet(ipStr, sourceNetStr, targetNetStr string, mask int) string {
	ip := net.ParseIP(ipStr)
	src := net.ParseIP(sourceNetStr)
	dst := net.ParseIP(targetNetStr)

	// Detect IPv4 vs IPv6 and normalize to 4 or 16 bytes accordingly
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
		src = src.To4()
		dst = dst.To4()
	} else {
		ip = ip.To16()
		src = src.To16()
		dst = dst.To16()
	}

	totalBits := 32
	if len(ip) != 4 {
		totalBits = 128
	}

	maskBytes := net.CIDRMask(mask, totalBits)

	host := make([]byte, len(ip))
	for i := range ip {
		host[i] = ip[i] & ^maskBytes[i]
	}
	result := make(net.IP, len(ip))
	for i := range ip {
		result[i] = (dst[i] & maskBytes[i]) | host[i]
	}

	return result.String()
}
