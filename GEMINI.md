# Project Context: mlb-controller (Multi Load Balancer Controller)

First of all read all rules (.agent/rules/*) and all skills (.agent/skills/*/SKILL.md) and always use golang-pro skill.

## 1. Overview & Purpose

`mlb-controller` is an enterprise Kubernetes controller written in Go (v1.24.6) that dynamically synchronizes upstream backend servers in external CDN/Edge load balancers (outside the K8s cluster) based on the state of Kubernetes `NodePort` and `LoadBalancer` Services.

### Target Service Identification

A Kubernetes Service is managed by `mlb-controller` if and only if:

- `spec.type` is `NodePort` or `LoadBalancer`.
- It contains the labels:
  - `mlb-loadbalancer-name`: Target LB name defined in `config.yaml`.
  - `mlb-upstream-name`: Target upstream group name on the external LB.
  - `mlb-port`: NodePort used for routing traffic to the nodes.

### Core Business Flow

1. Watch `Services`, `Pods`, and `Nodes` via `client-go` `SharedInformerFactory` and local cache `Lister` (`internal/adapters/kubernetes/client.go`).
2. Filter `Ready` Pods matching the Service selector and resolve their hosting Node's `InternalIP` (fallback to `ExternalIP`).
3. Apply NAT-like IP/CIDR replacement rules (`internal/util/ip_replacement.go`) with priority: `LB IPs > Global IPs > LB Nets > Global Nets` (supports IPv4 and IPv6).
4. Aggregate backends running on the same Node (`IP:Port`) by incrementing `Backend.Weight`.
5. Synchronize desired state with external load balancers via Event-Driven Workqueue (`SyncUpstream`) and periodic Two-Way Reconciliation (`SyncAllUpstreams`) to eliminate state drift and orphaned upstreams.

---

## 2. Architecture: Hexagonal (Ports & Adapters)

Strict separation of concerns must be maintained across all packages:

- `cmd/mlb-controller/main.go`: Entrypoint, OS signal handling (`SIGINT`, `SIGTERM`), DI container bootstrap, and graceful shutdown.
- `internal/domain/model`: Pure domain entities (`Service`, `Pod`, `Node`, `Upstream`, `Backend`, `ControllerState`). Zero external dependencies.
- `internal/domain/config`: Configuration structs (`Config`, `NginxConfig`, `EnvoyConfig`, `LeaderElectionConfig`, `GlobalIPReplacementList`) and `ApplyDefaultValues`.
- `internal/ports`: Interfaces defining boundaries (`KubernetesPort`, `LoadBalancerAdapter`, `Controller`, `LeaderElectionPort`, `Metrics`, `Logger`, `CertificateLoader`, `Loader`, `Validator`).
- `internal/application/app.go`: Application lifecycle orchestration (Leader Election callbacks, Metrics server startup/shutdown).
- `internal/controller/controller.go`: Core reconciliation loop, `workqueue.RateLimitingInterface` workers, event handlers, and periodic two-way drift sync.
- `internal/adapters`:
  - `config`: Viper + `mapstructure/v2` loader.
  - `validator`: Composite pattern with 10 specialized validators.
  - `certificate`: TLS/mTLS X.509 validator and loader (checks expiry, SAN hostname, RSA >= 2048 / ECDSA >= 256 bits).
  - `kubernetes`: `client-go` v0.34.1 Informers and Listers.
  - `loadbalancer`: Factory pattern + concrete implementations (`nginx` via `ngx_dynamic_upstream`).
  - `leaderelection`: Kubernetes `coordination/v1` `LeaseLock`.
  - `logging`: Uber `zap` structured logger.
  - `metrics`: Prometheus registry and HTTP exporter.
- `internal/di/wire.go`: Manual Dependency Injection container wiring.

---

## 3. Build, Test & Verification Commands

Always use the `Makefile` targets to verify changes:

- Build binary: `make build`
- Format & Vet: `make fmt && make vet`
- Lint: `make lint` (`golangci-lint run`)
- Unit Tests: `make test` (`go test -v ./...`)
- Coverage: `make test-cover`
- Integration Tests: `make test-integration`

---

## 4. Current Status & Next Roadmap Priorities

### Phase 1 Focus (Core, NGINX, Reconciliation & Safety Net) - COMPLETED
All core architectural and reconciliation bugs have been resolved and hardened:
1. **Full Test Suite Operational**: All tests passing with `-race`, deterministic waiting, and zero arbitrary `time.Sleep`.
2. **NGINX Adapter Hardened**: Full TLS/mTLS, RootCA certificate pool validation, SAN verification, concurrency semaphore enforcement in single and bulk reconciliation, robust URL query parameter handling, and IPv4/IPv6 support.
3. **Robust Safety Net & Fail-Static**: Automatic fallback to `default_backend` or preserving existing backends on zero ready pods (`allow_empty_upstream: false`) and on Kubernetes retrieval errors, preventing accidental traffic draining or orphan pruning.
4. **State Consistency & 12-Factor Lifecycle**: Pure level-triggered reconciliation, single-point OS signal management, clean DI with context propagation, Viper environment variable overrides (`v.AutomaticEnv()`), and automated cache/metric cleanup.

### Phase 2 Focus (Upcoming)
1. **Envoy Adapter**: Implement gRPC/REST Envoy Dynamic Upstream adapter matching the `LoadBalancerAdapter` port.
2. **HAProxy Adapter**: Implement HAProxy Runtime API socket / Data Plane API adapter.
3. **End-to-End & Integration Testing**: `envtest` control plane tests and `testcontainers-go` containerized NGINX tests under `//go:build integration`.
