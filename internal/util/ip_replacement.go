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
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	_, cidrNet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", netIP, mask))
	if err != nil || cidrNet == nil {
		return false
	}
	return cidrNet.Contains(parsedIP)
}

func replaceInNet(ipStr, sourceNetStr, targetNetStr string, mask int) string {
	ip := net.ParseIP(ipStr)
	src := net.ParseIP(sourceNetStr)
	dst := net.ParseIP(targetNetStr)
	if ip == nil || src == nil || dst == nil {
		return ipStr
	}

	// Detect IPv4 vs IPv6 and normalize accordingly
	isIPv4 := ip.To4() != nil && src.To4() != nil && dst.To4() != nil
	if isIPv4 {
		ip = ip.To4()
		src = src.To4()
		dst = dst.To4()
	} else {
		ip = ip.To16()
		src = src.To16()
		dst = dst.To16()
	}

	totalBits := 32
	if !isIPv4 {
		totalBits = 128
	}

	if mask < 0 || mask > totalBits {
		return ipStr
	}

	maskBytes := net.CIDRMask(mask, totalBits)
	if maskBytes == nil || len(maskBytes) != len(ip) || len(dst) != len(ip) {
		return ipStr
	}

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
