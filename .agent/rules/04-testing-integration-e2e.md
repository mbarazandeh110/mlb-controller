---
trigger: always_on
description: Standards for Kubernetes integration testing (envtest), containerized load balancer tests (testcontainers-go), build tags, and deterministic test execution.
---
# Integration & End-to-End Testing Rules

## 1. Build Tag Isolation
- **Mandatory Build Tag**: All integration test files must reside in dedicated files or directories with the build tag:
  ```go
  //go:build integration
  // +build integration
  ```
  Integration tests must NEVER be executed as part of standard `go test ./...` so that unit test feedback loops remain sub-second.
- **Dedicated Makefile Target**: Integration tests must be invoked via `make test-integration` or `go test -tags=integration -race -v ./...`.

## 2. Kubernetes API Integration: `envtest`
- **Real Control Plane**: Use `sigs.k8s.io/controller-runtime/pkg/envtest` to run real `kube-apiserver` and `etcd` binaries for testing Informer cache synchronization, label selectors, and optimistic concurrency.
- **Binary Provisioning**: Use `setup-envtest` (`setup-envtest use 1.30.0 -p path`) to supply `KUBEBUILDER_ASSETS` automatically in CI and local development.
- **Lifecycle Teardown**: Always call `testEnv.Stop()` inside a deferred function or `t.Cleanup` to prevent orphan etcd processes.

## 3. External Services: `testcontainers-go`
- **Ephemeral Containers**: When testing external load balancers (e.g. NGINX with `ngx_dynamic_upstream` or Envoy), spin up containerized instances using `github.com/testcontainers/testcontainers-go`.
- **Dynamic Port Mapping**: Never bind to fixed host ports (e.g. `8080`, `80`). Always map to dynamic host ports (`nat.Port`) provided by testcontainers to allow safe parallel execution.
- **Deterministic Health Checks**: Container definitions must include readiness wait strategies (`wait.ForHTTP`, `wait.ForLog`) before test execution begins.

## 4. Flakiness & Timing Rules
- **Zero `time.Sleep`**: Arbitrary sleep calls are strictly prohibited.
- **Polling Assertions**: Use `require.Eventually` or `assert.Eventually` with:
  - Polling interval: 50ms – 100ms.
  - Timeout budget: 5s – 15s.
  - Explicit error messages explaining what condition failed to converge.
- **Namespace Isolation**: Each integration test must generate and use a unique Kubernetes Namespace (e.g., `fmt.Sprintf("test-%d", time.Now().UnixNano())`) to ensure absolute test isolation.

## 5. Graceful Teardown & Goroutine Leak Prevention
- Every background goroutine started during an integration test (e.g., `controller.Start(ctx)`) must be bound to a test-scoped context with cancellation (`ctx, cancel := context.WithCancel(t.Context())`).
- Teardown functions must wait for worker pools to terminate cleanly before the test exits.
