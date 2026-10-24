---
name: k8s-integration-testing
description: "Patterns, tools, and execution strategies for Kubernetes controller and load balancer integration testing using envtest and testcontainers-go in mlb-controller."
risk: low
source: project
---

# Kubernetes & Load Balancer Integration Testing

## Overview

Integration testing in `mlb-controller` validates the interaction between the controller logic, the real Kubernetes API server (`envtest`), and external load balancer endpoints (via `testcontainers-go` or `httptest.Server`).

Unlike unit tests—which use in-memory fakes (`k8s.io/client-go/kubernetes/fake`) and test pure business logic—integration tests verify:
1. Real Kubernetes Informer cache synchronization, label selectors, and watch event delivery.
2. Optimistic concurrency control and resource version handling.
3. Actual HTTP/TLS communication with load balancers (e.g. NGINX with `ngx_dynamic_upstream`).
4. End-to-end reconciliation flow from Service creation/deletion to external LB upstream state.

---

## 1. Testing Pyramid for mlb-controller

```
        / \
       /   \      E2E Tests (Kind cluster + Helm / Kube manifests)
      / E2E \     - Full container image deployed in local Kind
     /-------\
    /         \   Integration Tests (envtest + testcontainers-go)
   / Integrat. \  - Real kube-apiserver + etcd, real NGINX container
  /-------------\ - Tagged with //go:build integration
 /     Unit      \ Unit Tests (fake.Clientset + httptest.Server)
/      Tests      \ - Sub-second, in-memory, runs on every commit
------------------- - go test -race ./internal/...
```

| Layer | Tools | Scope | Speed | Build Tag |
|---|---|---|---|---|
| **Unit** | `fake.NewClientset`, `httptest.Server`, `testify` | Individual adapters, parsers, validators, pure controller reconciliation | Milliseconds | None (default) |
| **Integration** | `envtest.Environment`, `testcontainers-go` | Controller + real K8s API server + real NGINX container | 5-15 seconds | `//go:build integration` |
| **E2E** | Kind, `kubectl`, Helm | Complete compiled binary in container inside K8s | 1-3 minutes | `//go:build e2e` |

---

## 2. Core Integration Tools & Libraries

### A. Kubernetes API Integration: `controller-runtime/pkg/envtest`
- **What it does**: Spins up local, real `kube-apiserver` and `etcd` binaries without kubelet or container runtimes.
- **Why use it**: 
  - `fake.Clientset` lacks real API schema validation, CRD support, label/field selector indexing, and accurate watch stream timing.
  - `envtest` gives 100% authentic Kubernetes API behavior while executing in-process inside your Go test binary.
- **Tooling dependency**: `setup-envtest` CLI manages the platform binaries:
  ```bash
  go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
  setup-envtest use 1.30.0 -p path
  ```

### B. Load Balancer Integration: `testcontainers-go`
- **What it does**: Automatically starts ephemeral Docker containers (e.g. custom NGINX image with `ngx_dynamic_upstream` compiled in) for the duration of the test.
- **Why use it**:
  - Tests verify authentic HTTP parsing, query parameters (`/dynamic?upstream=...`), SSL/mTLS handshakes, and upstream reloading without manual environment setup.
  - Ephemeral containers bind to dynamic free ports on `localhost`, preventing port collisions in parallel CI runs.

---

## 3. Writing Integration Tests

### Rule 1: Build Tag Isolation
Every integration test file MUST declare the build tag on the very first line:
```go
//go:build integration
// +build integration

package controller_test
```
This guarantees `go test ./...` remains blazingly fast and never fails if Docker or `envtest` binaries are missing.

### Rule 2: envtest Setup & Teardown Lifecycle
```go
//go:build integration

package controller_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func setupTestEnv(t *testing.T) (*kubernetes.Clientset, func()) {
	t.Helper()

	testEnv := &envtest.Environment{
		ErrorIfCRDPathMissing: false,
	}

	cfg, err := testEnv.Start()
	require.NoError(t, err, "failed to start envtest environment")

	client, err := kubernetes.NewForConfig(cfg)
	require.NoError(t, err, "failed to create k8s client from envtest config")

	teardown := func() {
		err := testEnv.Stop()
		require.NoError(t, err, "failed to stop envtest")
	}

	return client, teardown
}
```

### Rule 3: Deterministic Wait Patterns (Zero `time.Sleep`)
**Never** use `time.Sleep` in integration tests. Kubernetes informers and reconciliation workqueues operate asynchronously. Always poll with `require.Eventually` or `assert.Eventually`:

```go
// BAD: Flaky and slow
time.Sleep(2 * time.Second)
assert.Equal(t, 2, len(lbAdapter.GetUpstreams()))

// GOOD: Fast, deterministic, and fails with clear message on timeout
require.Eventually(t, func() bool {
    state := controller.GetState()
    upstream, exists := state.Upstreams["nginx-main/app-upstream"]
    return exists && len(upstream.Backends) == 2
}, 5*time.Second, 50*time.Millisecond, "expected upstream with 2 backends to sync")
```

### Rule 4: Namespace & Resource Isolation
Create a unique Kubernetes Namespace per test to prevent test interference during parallel execution:
```go
ns := &corev1.Namespace{
    ObjectMeta: metav1.ObjectMeta{
        Name: fmt.Sprintf("test-mlb-%d", time.Now().UnixNano()),
    },
}
_, err := client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
require.NoError(t, err)
```

---

## 4. Execution Commands

### Run Unit Tests (Default)
```bash
go test -race -v ./...
```

### Run Integration Tests with envtest Assets
```bash
# Using Makefile
make test-integration

# Direct command
KUBEBUILDER_ASSETS="$(setup-envtest use 1.30.0 -p path)" go test -tags=integration -race -v ./...
```

### Run Specific Integration Test
```bash
KUBEBUILDER_ASSETS="$(setup-envtest use 1.30.0 -p path)" go test -tags=integration -race -v -run TestController_ReconciliationFlow ./...
```

---

## 5. Anti-Patterns & Common Pitfalls

| Anti-Pattern | Why it Fails | Correct Approach |
|---|---|---|
| `time.Sleep(N * time.Second)` | Causes test flakiness in CI and artificially slows test suites. | Use `require.Eventually` with 50-100ms polling. |
| Hardcoded Host Ports (`8080`) | Conflicts when running multiple tests or concurrent CI jobs. | Use dynamic ephemeral ports (`port 0` or testcontainers mapped port). |
| Shared Global State | Tests pass individually but fail when run in sequence or with `-race`. | Instantiate new `ControllerState`, Informer factory, and unique namespaces per test. |
| Missing Cache Sync | `Lister()` calls fail or return empty because cache is not warm. | Always call `cache.WaitForCacheSync(ctx.Done(), informers...)` before assertions. |
| Leaking Goroutines / Contexts | Background workers keep running after test finishes, causing data races. | Cancel test context `defer cancel()` and wait for worker loops to stop. |
