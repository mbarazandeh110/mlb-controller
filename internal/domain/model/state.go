// internal/domain/model/state.go
package model

// ControllerState holds the current state of monitored services, pods, nodes, and upstreams.
type ControllerState struct {
	// Services is a map of services, keyed by namespace/name.
	Services map[string]Service
	// Pods is a map of pods, keyed by namespace/name.
	Pods map[string]Pod
	// Nodes is a map of nodes, keyed by node name.
	Nodes map[string]Node
	// Upstreams is a map of upstreams, keyed by loadbalancer_name/upstream_name.
	Upstreams map[string]Upstream
}
