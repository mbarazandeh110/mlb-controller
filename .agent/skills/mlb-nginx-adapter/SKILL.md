
---
name: mlb-nginx-adapter
description: Specialized skill for developing, debugging, and testing the NGINX LoadBalancerAdapter (ngx_dynamic_upstream) in mlb-controller. Use whenever working on internal/adapters/loadbalancer/nginx, internal/domain/config/nginx.go, or internal/adapters/validator/nginx.go.
---
# MLB NGINX Adapter Specialist (`ngx_dynamic_upstream`)

## When to Use

- Modifying or fixing `internal/adapters/loadbalancer/nginx/client.go` or `nginx.go`
- Updating NGINX bulk response parsing (`parseBulkNginxBackends`)
- Fixing TLS/mTLS/RootCA certificate loading in `NginxClient`
- Writing or fixing unit tests in `nginx_test.go` and `client_test.go`

## Core Mechanics of the NGINX Adapter

1. **Protocol & Endpoints**: Communicates with external NGINX servers running `ngx_dynamic_upstream` via HTTP/HTTPS GET requests:
   - `list_api`: Fetches all upstreams and their servers in bulk text format.
   - `add_api`: `?upstream=<name>&server=<ip>:<port>&weight=<w>`
   - `remove_api`: `?upstream=<name>&server=<ip>:<port>`
2. **Bulk Caching (`refreshCache`)**:
   - Caches parsed state per `client.baseURL` (`map[string]map[string][]model.Backend`) with a 2-second TTL (`cacheExp`) protected by `cacheMutex` (`sync.RWMutex`).
   - Any mutation (`add_api` or `remove_api`) MUST invalidate the cache for that specific `client.baseURL` via `invalidateCache(c.baseURL)`.
3. **Weight Aggregation**:
   - Multiple Pods of the same Service scheduled on the same Kubernetes Node produce identical `NodeIP:NodePort` backends.
   - `SyncUpstream` aggregates duplicate `IP:Port` entries by summing/incrementing `Weight` (minimum weight = 1).

## Mandatory Engineering Checklist for NGINX Adapter

1. **TLS & Custom CA Certificates (`client.go`)**:
   - When `ngx.Protocol == "https"` and `len(ngx.CA) > 0`, initialize `certPool := x509.NewCertPool()` AND call `if !certPool.AppendCertsFromPEM(ngx.CA) { return nil, fmt.Errorf(...) }` before assigning `transport.TLSClientConfig.RootCAs = certPool`.
   - Always check `tls.X509KeyPair(ngx.Cert, ngx.Key)` and `x509.ParseCertificate` errors explicitly instead of ignoring them with `_`.
2. **Concurrency Limiting (`nginx.go`)**:
   - Both `SyncUpstream` AND `SyncAllUpstreams` MUST acquire `a.semaphore.Acquire(ctx, 1)` inside each client goroutine (and `defer a.semaphore.Release(1)`) to enforce `RequestPoolSize` limits.
3. **Empty Upstream Handling**:
   - If `SyncUpstream` is called with `len(upstream.Backends) == 0`, it MUST remove all currently active backends for `upstream.Name` on all NGINX clients.
4. **Robust Bulk Parsing (`parseBulkNginxBackends`)**:
   - Support both bulk multi-upstream format (upstream headers followed by `server IP:Port weight=X;`) and single-upstream flat responses cleanly, handling IPv4 and bracketed IPv6 (`[::1]:8080`) addresses.
