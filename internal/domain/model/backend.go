// internal/domain/model/backend.go
package model

// Backend represents a single server in a load balancer upstream.
type Backend struct {
	// IP is the IP address of the backend (from node or pod, based on config).
	IP string
	// Port is the port of the backend (from service NodePort or pod port).
	Port int32
	// Weight is the weight of the backend (default 1, increases with duplicates).
	Weight int
}
