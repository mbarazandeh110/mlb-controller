
---
trigger: always_on
description: Enforces Hexagonal Architecture, SOLID Principles, and 12-Factor App methodology across mlb-controller.
---
# Architectural, SOLID & 12-Factor Guardrails

## 1. Hexagonal Architecture (Ports & Adapters)

- **Dependency Direction**: Dependencies MUST point strictly inward (`adapters -> ports -> domain`).
- **Domain Purity**: Packages under `internal/domain/` (`model` and `config`) MUST NOT import any package from `internal/ports`, `internal/adapters`, `internal/controller`, or external infrastructure SDKs (`k8s.io/client-go`, `go.uber.org/zap`, `prometheus`).
- **Port Contracts**: All interactions between `application` / `controller` and external systems (Kubernetes API, NGINX, Envoy, Prometheus, Filesystem) MUST go through interfaces defined in `internal/ports/`.
- **No Leaky Abstractions**: Adapter-specific types (e.g., `corev1.Pod`, `http.Response`, `zap.Field`, `prometheus.GaugeVec`) must never leak into `internal/ports` or `internal/controller`. Always map them to domain models or port types inside the adapter.

## 2. SOLID Principles in Go

- **Single Responsibility (SRP)**:
  - `ControllerImpl` only orchestrates state transitions and reconciliation queues.
  - `NginxAdapter` only handles state diffing, caching, and upstream synchronization over `NginxClient`.
  - `NginxClient` only handles HTTP transport, TLS configuration, and retry/backoff logic.
  - Validators in `internal/adapters/validator/` must remain modular per domain concern.
- **Open/Closed Principle (OCP)**:
  - Adding a new load balancer type (e.g., Envoy, HAProxy) or a new config validator MUST be done by implementing `LoadBalancerAdapter` or `Validator` and registering it in the factory/composite—without modifying core controller reconciliation logic.
- **Liskov Substitution (LSP)**:
  - Every `LoadBalancerAdapter` implementation (NGINX, Envoy, Fake/Mock) must honor the exact idempotency contract of `SyncUpstream` and `SyncAllUpstreams`, including handling empty backend slices (`len(upstream.Backends) == 0`) by draining/removing existing backends.
- **Interface Segregation (ISP)**:
  - Keep port interfaces focused and minimal. If a consumer only needs read access or specific metrics, do not force bloated interface implementations.
- **Dependency Inversion (DIP)**:
  - High-level modules (`application.App`, `controller.ControllerImpl`) must receive all dependencies via constructor injection (`NewApp`, `NewController`) wired exclusively in `internal/di/wire.go`.

## 3. 12-Factor App Compliance

1. **Codebase**: Single Go module (`mlb-controller`) tracked in Git.
2. **Dependencies**: Explicitly declared in `go.mod` and isolated via static compilation (`CGO_ENABLED=0`, `-tags netgo`).
3. **Config**: Loaded from external YAML (`--config` / `CONFIG_PATH`) and environment overrides via Viper. Never hardcode IPs, URLs, timeouts, or credentials. Load and validate configuration **once** at startup (Fail-Fast).
4. **Backing Services**: Treat Kubernetes API and external Load Balancers (NGINX/Envoy) as attached network resources interchangeable via configuration.
5. **Build, Release, Run**: Strict separation enforced via multi-stage `Dockerfile` (`golang:1.24.7-alpine` builder -> `gcr.io/distroless/static:nonroot` runtime).
6. **Processes**: The controller must remain stateless and share-nothing; all runtime state (`ControllerState`, NGINX 2s cache) is reconstructed from Kubernetes Informers and LB APIs on startup.
7. **Port Binding**: Self-contained HTTP server for Prometheus metrics and health probes (`/healthz`, `/readyz`).
8. **Concurrency**: Scale out via Leader Election (`LeaseLock`) for HA and bounded internal worker pools (`RequestPoolSize` workers + `semaphore.Weighted`).
9. **Disposability**: Fast startup and deterministic graceful shutdown on `SIGTERM`/`SIGINT` using a single root `context.Context` propagation chain, draining `workqueue` and HTTP servers within the timeout window.
10. **Logs**: Write structured JSON logs exclusively to `stdout` via the `logging.Logger` port. Never write log files to disk.
