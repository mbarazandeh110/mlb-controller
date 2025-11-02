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
| **Pod Events** | Add, delete, or readiness change |
| **Periodic Sync** | Configurable per load balancer or globally |
| **Leader Election** | Sync runs only on the elected leader instance |

---

## Prerequisites

- Kubernetes cluster (v1.19+ recommended)
- Access to external CDN load balancers
- **NGINX** with [`ngx_dynamic_upstream`](https://github.com/cubicdaiya/ngx_dynamic_upstream) module enabled
- RBAC permissions for:
  - `services` (get, list, watch)
  - `pods` (get, list, watch)
  - `nodes` (get, list, watch)
  - `leases` (for leader election, if enabled)

---

## Configuration

Sample configuration is located at [`configs/config.yaml`](./configs/config.yaml).

### Key Sections

#### `global_upstream_sync_period`
Default sync interval for all load balancers (overridable per LB).

#### `leader_election`
Enable high availability with a single active controller.

#### `global_ip_replacement_list`
Define reusable network (`net`) and IP (`ip`) rewrite rules.

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
# Clone the repo
git clone https://github.com/mbarazandeh110/mlb-controller.git
cd mlb-controller

# Build the container
docker build -t mlb-controller:latest .

# Push to registry
docker push your-registry/mlb-controller:latest
```

### 2. Deploy to Kubernetes

```bash
```

> Manifests include: Deployment, ServiceAccount, ClusterRole, ConfigMap (with config.yaml)

### 3. Mount Config

Ensure `config.yaml` is mounted into the pod at `/etc/mlb-controller/config.yaml`.

```yaml
volumeMounts:
  - name: config
    mountPath: /etc/mlb
```

---

## Usage Example

Label a Service to be managed:

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
- Detect Pods behind `my-app`
- Map `NodeIP:30080` → backend
- Apply IP replacements
- Add to `backend-api` upstream in the `CDN` NGINX load balancer

---

## Metrics

Exposed on `:9090/metrics` (Prometheus format)

| Metric | Description |
|-------|-------------|
---

## Development

```bash
# Run locally (outside cluster)
go run main.go --config configs/config.yaml
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
```
