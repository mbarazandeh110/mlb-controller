# mlb-controller (Multi Load Balancer Controller)

**mlb-controller** is a Kubernetes controller that dynamically synchronizes upstream backends in external CDN load balancers (outside the cluster) based on the state of `NodePort` and `LoadBalancer` Services inside the cluster.

It monitors Kubernetes Services labeled with:
- `mlb-loadbalancer-name`
- `mlb-upstream-name`
- `mlb-port`

For each matching Service, it identifies the backing Pods, determines which Nodes they run on, and updates the corresponding **upstream** configuration in one or more external load balancers (currently **NGINX** via `ngx_dynamic_upstream` module).

---

## Features

- **Real-time upstream synchronization** on Pod add/remove or readiness changes
- **Periodic sync** to ensure consistency even in case of missed events
- **Leader election** support for high availability (optional)
- **IP address and network rewriting** (NAT-like mapping) before sending to CDN
- **Global and per-loadbalancer IP replacement rules**
- **Prometheus metrics** endpoint
- **Structured JSON logging**
- **Pluggable load balancer support**  
  Currently: **NGINX** (via [ngx_dynamic_upstream](https://github.com/cubicdaiya/ngx_dynamic_upstream))  
  Future: **Envoy**, **HAProxy**

---

## How It Works

1. **Watch Services** of type `NodePort` or `LoadBalancer` with required labels.
2. **Discover Pods** behind the Service.
3. **Determine Node IP + Pod Port** → backend address.
4. **Apply IP replacement rules** (global + loadbalancer-specific).
5. **Compare desired upstream state** with current state on CDN (via `list_api`).
6. **Add/remove backends** using `add_api` / `remove_api` if needed.

### Sync Triggers

| Trigger | Description |
|-------|-----------|
| **Service Events** | Add, delete, or label modifications (e.g. changing upstream name or target load balancer triggers automatic drain of old upstream) |
| **Pod Events** | Add, delete, or readiness state changes (`Ready: true/false`), as well as Pod migrations across Nodes |
| **Node Events** | Node IP address changes |
| **Periodic Sync** | Two-way reconciliation running per load balancer to eliminate state drift and prune orphaned upstreams |
| **Leader Election** | Sync runs only on the elected leader replica in HA mode |

---

## Prerequisites

- Kubernetes cluster (v1.19+ recommended)
- Access to external CDN load balancers
- **NGINX** with [`ngx_dynamic_upstream`](https://github.com/cubicdaiya/ngx_dynamic_upstream) module enabled
- RBAC permissions for:
  - `services` (get, list, watch)
  - `pods` (get, list, watch)
  - `nodes` (get, list, watch)
  - `leases` in `coordination.k8s.io` (for leader election, if enabled)

---

## Configuration

Sample configuration is located at [`configs/config.yaml`](./configs/config.yaml).

### Key Settings

#### `global_upstream_sync_period`
Default sync interval for all load balancers (e.g. `30s`), overridable per load balancer via `upstream_sync_period`.

#### `allow_empty_upstream`
Safety net flag (global or per-LB, default: `false`).
- If `false`: when a managed Service has 0 ready pods, `mlb-controller` logs a warning and preserves the existing backends on the external load balancer to avoid catastrophic outages.
- If `true`: allows draining all backends from the upstream when all Pods are not ready.
- *Note:* Deleting a Service from Kubernetes always drains and removes its backends from the load balancer.

#### `default_backend`
Fallback backend used when 0 Pods are ready. Can be configured per load balancer (under `loadbalancers[].default_backend`) or globally:
```yaml
loadbalancers:
  - type: nginx
    name: CDN-West
    default_backend:
      ip: "10.1.0.99"
      port: 8080
      weight: 1
  - type: nginx
    name: CDN-East
    default_backend:
      ip: "10.2.0.99"
      port: 9090
      weight: 2
```

#### `leader_election`
Enable high availability with Kubernetes LeaseLocks:
```yaml
leader_election:
  enabled: true
  lease_name: mlb-controller-leader
  lease_namespace: kube-system
  lease_duration: 15s
  renew_deadline: 10s
  retry_period: 2s
```

#### `global_ip_replacement_list`
Define reusable network (`net`) and IP (`ip`) rewrite rules (supports both IPv4 and IPv6).

#### `loadbalancers`
List of external load balancers to manage.

```yaml
loadbalancers:
  - type: nginx
    name: CDN
    protocol: http
    hostname: example.test
    addresses:
      - ip: 1.2.3.4
        port: 80
    list_api: /api/upstream
    add_api: /dynamic
    remove_api: /dynamic
    upstream_sync_period: 30s
    prune_orphans: true
    allow_empty_upstream: false
    default_backend:
      ip: 10.0.0.99
      port: 8080
      weight: 1
    ip_replacement: true
    ip_replacement_list:
      global_nets: [cdn1]
      global_ips: [lb2]
```

> See full example in [`configs/config.yaml`](./configs/config.yaml)

---

## Installation

### 1. Build & Deploy

```bash
# Clone the repository
git clone https://github.com/mbarazandeh110/mlb-controller.git
cd mlb-controller

# Build binary
make build

# Build container image
docker build -t mlb-controller:latest .
```

### 2. Deploy to Kubernetes

Deploy the controller using Helm:

```bash
helm install mlb-controller ./charts/mlb-controller -n kube-system
```

Or render and apply manifests using `helm template`:

```bash
helm template mlb-controller ./charts/mlb-controller | kubectl apply -f -
```

### 3. Mount Config

Ensure `config.yaml` is mounted into the pod at `/etc/mlb-controller/config.yaml`:

```yaml
volumeMounts:
  - name: config
    mountPath: /etc/mlb-controller
```

---

## Usage Example

Label a Service of type `NodePort` or `LoadBalancer` to be managed:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-app
  labels:
    mlb-loadbalancer-name: CDN
    mlb-upstream-name: backend-api
    mlb-port: "30080"
spec:
  type: NodePort
  selector:
    app: my-app
  ports:
    - port: 80
      targetPort: 8080
      nodePort: 30080
```

→ The controller will:
- Detect ready Pods backing `my-app`
- Resolve hosting Node IPs (`InternalIP` fallback to `ExternalIP`)
- Apply NAT-like IP/CIDR replacement rules
- Aggregate weights for multiple Pods scheduled on the same Node
- Synchronize backends to `backend-api` upstream in the `CDN` NGINX load balancer

---

## Health Checks & Metrics

### Probes

Exposed on port `:9090` (or configured `metrics.port`):
- **`/healthz`**: Liveness probe (HTTP 200 OK)
- **`/readyz`**: Readiness probe (HTTP 200 OK when Informers are synchronized, HTTP 503 otherwise)

### Prometheus Metrics

Exposed on `:9090/metrics`:

| Metric | Type | Description |
|--------|------|-------------|
| `mlb_is_leader` | Gauge | `1` if this controller instance is the active leader, `0` if standby |
| `mlb_backends_total` | Gauge | Total number of active backends configured per upstream |
| `mlb_sync_errors_total` | Counter | Total count of synchronization and drift errors per upstream or LB |
| `mlb_sync_duration_seconds` | Histogram | Latency in seconds of upstream synchronization operations |
| `mlb_sync_upstreams_total` | Counter | Total count of drift reconciliation passes executed |

---

## Development

```bash
# Run locally (outside cluster)
go run ./cmd/mlb-controller --config configs/config.yaml
```

Run verification targets:
```bash
make fmt && make vet
make test
make build
```

Use `make test`, `make lint`, `make build`.

---

## Roadmap

- [x] NGINX + `ngx_dynamic_upstream` support
- [ ] Envoy (gRPC/REST dynamic updates)
- [ ] HAProxy (via runtime API)
- [ ] Webhook validation for Service labels
- [ ] Dry-run mode
- [ ] Helm chart

---

## Contributing

Contributions are welcome! Please:
- Open an issue first for major changes
- Follow Go coding standards
- Write tests
- Update documentation

---

**Keep your CDN in sync with your cluster — automatically.**
