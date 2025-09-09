package validator

import (
	"net"
	"regexp"
)

// Helper functions
func isValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

func isValidProtocol(protocol string) bool {
	return protocol == "http" || protocol == "https" || protocol == "grpc"
}

func isValidDomain(hostname string) bool {
	domainRegex := regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
	return domainRegex.MatchString(hostname)
}
