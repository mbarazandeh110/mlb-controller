// internal/domain/model/node.go
package model

// Node represents a Kubernetes Node with its addresses.
type Node struct {
	// Name is the name of the node.
	Name string
	// ClusterIP is the k8s-cluster IP address of the node.
	ClusterIP string
	// InternalIP is the internal IP address of the node.
	InternalIP string
	// ExternalIP is the external IP address of the node (if available).
	ExternalIP string
}
