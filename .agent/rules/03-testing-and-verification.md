---
trigger: always_on
description: Unit testing standards, table-driven patterns, race detection, Informer mock lifecycle, and CI verification rules.
---
# Unit Testing & Verification Rules

## 1. Unit Testing Standards & Table-Driven Tests
- **No Broken Tests**: Every code modification in `internal/` MUST be accompanied by updated unit tests in the corresponding `*_test.go` file.
- **Table-Driven Tests**: Write deterministic, table-driven tests using `github.com/stretchr/testify/assert` and `require`. Structure test cases with descriptive `name` fields and run with `t.Run(tt.name, func(t *testing.T) { ... })`.
- **Assertion Granularity**: Use `require` for preconditions (e.g. `require.NoError`, `require.NotNil`), and `assert` for value comparisons (e.g. `assert.Equal`, `assert.ElementsMatch`).

## 2. In-Memory Fakes & Mocking Patterns
- **Port Fakes**: Use in-memory fakes implementing Port interfaces (`KubernetesPort`, `LoadBalancerAdapter`, `Logger`, `Metrics`). Never mock external networks directly in unit tests.
- **HTTP Adapters**: Test HTTP clients (`NginxClient`) using `httptest.NewServer` with custom `http.HandlerFunc` verifying paths, query params, headers, and responses.
- **Informer Mock Lifecycle**: When testing with `k8s.io/client-go/kubernetes/fake.NewClientset`, always initialize `informerFactory := informers.NewSharedInformerFactory(client, 0)`, start it with `informerFactory.Start(ctx.Done())`, and wait for `cache.WaitForCacheSync` before calling `Lister()` methods.

## 3. Concurrency & Flakiness Prevention
- **Prohibition of `time.Sleep`**: Never use arbitrary `time.Sleep` to wait for asynchronous events or goroutine termination. Use Go channels, `sync.WaitGroup`, or `require.Eventually(t, condition, timeout, tick)` with meaningful intervals.
- **Race Detection**: Always run unit tests with the race detector enabled (`go test -race -v -count=1 ./...`). Zero data races are permitted.
- **Independent State**: Each test case must initialize its own isolated state (`ControllerState`, channels, fakes). Do not share mutable state across parallel subtests.

## 4. Verification Gate Before Task Completion
Before concluding any task or reporting success to the user, execute:
1. `make fmt && make vet`
2. `go test -race -v -count=1 ./...`
3. `make build`
