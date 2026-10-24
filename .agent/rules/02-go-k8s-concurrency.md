
---
trigger: always_on
description: Enforces Go 1.24 best practices, Kubernetes client-go controller patterns, and concurrency safety.
---
# Go & Kubernetes Controller Engineering Standards

## 1. Context Propagation & Lifecycle

- Never use `context.Background()` inside business logic, loops, or adapter methods (such as `buildUpstream` calling `GetNodeByName` or `GetLeaderAddr` querying `Leases`). Always pass the caller's `ctx context.Context` down the call stack.
- Handle OS signals (`os.Interrupt`, `syscall.SIGTERM`) in ONE place only (`cmd/mlb-controller/main.go` via `signal.NotifyContext`). Do not duplicate signal channels inside `internal/application/app.go`.

## 2. Concurrency & Race Prevention

- Protect all reads and writes to `ControllerState` (`Services`, `Pods`, `Nodes`, `Upstreams`) with `stateMutex` (`RLock` for reads, `Lock` for writes).
- When returning state via `GetState()`, return a deep copy of maps/slices to prevent external data races.
- Bound all external network calls in LoadBalancer adapters (`SyncUpstream` AND `SyncAllUpstreams`) using `a.semaphore.Acquire(ctx, 1)`.
- Ensure every spawned goroutine has a clear exit condition tied to `<-ctx.Done()` or channel closure to prevent goroutine leaks.

## 3. Kubernetes Informer & Workqueue Patterns

- Always read from local Informer caches (`Lister()`) rather than hitting the Kubernetes API Server directly during reconciliation.
- Ensure `cache.WaitForCacheSync` succeeds before processing workqueue items or running initial bulk sync.
- Handle deletion Tombstones (`cache.DeletedFinalStateUnknown`) in Informer `DeleteFunc` handlers so missed delete events during watch reconnects are safely recovered.
- In `workqueue` processing, always `defer c.queue.Done(obj)`, call `c.queue.Forget(obj)` on success or permanent validation error, and `c.queue.AddRateLimited(obj)` on transient errors.

## 4. Error Handling & Observability

- Wrap errors with context using `fmt.Errorf("context: %w", err)`.
- Aggregate parallel worker errors using `github.com/hashicorp/go-multierror`.
- Update Prometheus metrics (`UpdateBackendsTotal`, `IncrementSyncErrors`, `ObserveSyncDuration`) consistently across both single-service syncs and bulk reconciliations.
