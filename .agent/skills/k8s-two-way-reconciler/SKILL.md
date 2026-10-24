
---
name: k8s-two-way-reconciler
description: Specialized skill for the Kubernetes Informer, Workqueue, and Two-Way Reconciliation engine in mlb-controller. Use when editing internal/controller, internal/adapters/kubernetes, or internal/application.
---
# Kubernetes Two-Way Reconciliation Engine Specialist

## When to Use

- Modifying `internal/controller/controller.go` or `internal/adapters/kubernetes/client.go`
- Fixing state drift, Service deletion cleanup, or zero-ready-pod edge cases
- Writing unit and integration tests for `ControllerImpl` and `KubernetesAdapter`

## Reconciliation Architecture

`mlb-controller` uses a hybrid reconciliation model:

1. **Reactive Event-Driven Loop**:
   - `KubernetesAdapter` watches `Services`, `Pods`, and `Nodes` via `SharedInformerFactory`.
   - Events update `ControllerImpl.state` and enqueue affected `namespace/serviceName` keys into `workqueue.RateLimitingInterface`.
   - Workers (`runWorker`) pop `namespace/serviceName` and invoke `syncHandler`.
2. **Periodic Two-Way Drift Reconciliation (`SyncUpstreams`)**:
   - Triggered on leadership acquisition (`OnStartedLeading`) and periodically via `GlobalUpstreamSyncPeriod`.
   - Builds the complete desired state map across all Services and invokes `adapter.SyncAllUpstreams(ctx, desiredState[lbName])` to purge orphaned upstreams from external load balancers.

## Critical Edge Cases to Enforce

1. **Service Deletion / Label Removal**:
   - If a managed Service is deleted (or loses its `mlb-*` labels), `handleServiceEvent` removes it from `c.state.Services`.
   - You MUST track which `lbName/upstreamName` was associated with that `namespace/serviceName` (or trigger an immediate upstream drain / two-way sync for that upstream) so backends are removed from the LB immediately upon Service deletion rather than waiting for the periodic ticker.
2. **Zero Ready Pods (All Pods Unready or Deleted)**:
   - When `syncUpstreamsForService` finds 0 ready pods (`len(backends) == 0`), `buildUpstream` must NOT cause `syncUpstreamsForService` to silently return `nil` while leaving stale backends active on the LB.
   - Instead, construct an `Upstream` with an empty `Backends: []model.Backend{}` slice and call `syncLoadBalancerUpstream(ctx, upstream)` so the LB adapter immediately removes all backends for that upstream.
3. **Tombstone Handling in Informers**:
   - In `KubernetesAdapter` `DeleteFunc`, handle both `*corev1.Service` (or Pod/Node) and `cache.DeletedFinalStateUnknown` to prevent missed deletions when watches reconnect.
4. **Informer Lifecycle in Tests**:
   - When testing `KubernetesAdapter` with `k8s.io/client-go/kubernetes/fake`, always initialize `informerFactory := informers.NewSharedInformerFactory(client, 0)`, register the informers, start the factory, and call `cache.WaitForCacheSync` so `Lister()` queries work without nil-pointer panics.
