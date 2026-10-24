---
name: test-fixing
description: "Systematically identify, categorize, and fix failing Go tests, Kubernetes controller race conditions, Informer timeouts, or HTTP mock mismatches."
risk: low
source: community
---

# Test Fixing for Go & Kubernetes Controllers

## Overview

Systematically identify and fix failing tests in `mlb-controller` using root-cause categorization and incremental verification.

---

## Systematic Workflow

### 1. Run & Classify Failures
Execute `go test -race -v ./...` and group failures into one of these 5 categories:

1. **Compilation / Signature Mismatch**:
   - Cause: Function signatures changed (e.g., added `ctx context.Context`, changed constructor return types).
   - Solution: Update test call sites or fake implementations to match new interfaces in `internal/ports/`.

2. **Data Race Detected (`WARNING: DATA RACE`)**:
   - Cause: Unsynchronized read/write to shared struct (e.g., `ControllerState`, `isLeader`, test callback booleans).
   - Solution: Protect fields with `sync.RWMutex`, replace bare shared primitives with `atomic.Bool`/`sync.Mutex`, or use channels for cross-goroutine assertions.

3. **Informer Cache Timeout / SIGSEGV**:
   - Cause: Informer `Lister()` called before `informerFactory` was initialized or before `cache.WaitForCacheSync` completed.
   - Solution: In tests using `fake.NewClientset`, construct `informers.NewSharedInformerFactory`, start it with `informerFactory.Start(ctx.Done())`, and wait for `cache.WaitForCacheSync`.

4. **HTTP Mock Server / Load Balancer Mismatch**:
   - Cause: URL path, HTTP method, query param (`/dynamic?upstream=...`), or response body format does not match `NginxClient` expectations.
   - Solution: Inspect `httptest.Server` handler in test, ensure exact response formats (e.g., `server 10.0.0.1:80 weight=1;`), and verify status code returns.

5. **Assertion Failure / Logic Bug**:
   - Cause: Expected vs actual values differ (e.g., upstream weight calculation, IP replacement priority, empty backend slice).
   - Solution: Check domain logic and test table inputs. Never relax the assertion to make a bug pass—fix the underlying code.

---

## Fixing Strategy & Order

Fix in this strict dependency order:
1. **Compilation errors**: Ensure everything builds cleanly.
2. **Infrastructure / Informer setup**: Fix uninitialized informers, contexts, and fake clients.
3. **Data races**: Run `go test -race -run <TestName> <pkg>` until race warning disappears.
4. **Behavioral assertions**: Correct diffs and business logic.

---

## Best Practices

- **Never use `time.Sleep` to "fix" a race or timing test**: Always replace with `require.Eventually`, condition channels, or `sync.WaitGroup`.
- **One test at a time**: Run `go test -race -v -run TestSpecific ./internal/...` during debugging before running the full suite.
- **Deep Copy State in Tests**: When verifying state mutations, ensure you are testing against returned deep copies to reflect production behavior.
- **Always verify with zero cache**: `go test -race -count=1 ./...` prevents false positives from Go test cache.
