# node-taint-controller

[![CI](https://github.com/jterceiro/node-taint-controller/actions/workflows/ci.yaml/badge.svg)](https://github.com/jterceiro/node-taint-controller/actions/workflows/ci.yaml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go](https://img.shields.io/github/go-mod/go-version/jterceiro/node-taint-controller)](https://go.dev/)

A Kubernetes controller that handles **Non-Graceful Node Shutdowns** by managing the
`node.kubernetes.io/out-of-service:NoExecute` taint. It is built with
[controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) and follows
Kubebuilder conventions.

## How it works

1. The controller watches every Node in the cluster (or a filtered subset).
2. When a node becomes `NotReady` the controller sets the annotation
   `taint-controller.io/unhealthy-since` to record the timestamp.
3. Once the node has been `NotReady` for longer than `unhealthyDelayMinutes`, the
   `node.kubernetes.io/out-of-service:NoExecute` taint is applied, causing the
   node's pods to be force-deleted.
4. When the node returns to `Ready`, the controller waits `recoveryDelayMinutes`
   before removing the taint and the annotation.

### Safety features

| Feature | Description |
|---|---|
| **Circuit breaker** | If the percentage of `NotReady` nodes exceeds `maxUnhealthyPercentage` (default 33 %) the controller stops applying new taints to prevent cascading failures during network partitions. |
| **Node label selector** | Set `node-label-selector` in the ConfigMap to restrict which nodes are managed (e.g. `taint-controller=enabled`). |
| **Leader election** | Multiple replicas can run safely; only the leader reconciles. |
| **State persistence** | The `unhealthy-since` annotation is the only persistent state so the controller is fully stateless and restart-safe. |

## Observability

Prometheus metrics are exposed on `:8080/metrics`:

| Metric | Type | Description |
|---|---|---|
| `nodes_tainted_total` | Counter | Total times the out-of-service taint has been applied |
| `current_unhealthy_nodes` | Gauge | Current number of `NotReady` watched nodes |

## Configuration

Configuration is loaded from a `ConfigMap` at startup and on every reconcile loop so
changes take effect dynamically without restarting the controller.

```yaml
# config/configmap.yaml
data:
  unhealthy-delay-minutes: "5"    # default: 5
  recovery-delay-minutes: "10"    # default: 10
  max-unhealthy-percentage: "33"  # default: 33  (0-100; use 100 to disable)
  node-label-selector: ""         # default: ""  (watch all nodes)
```

Apply the ConfigMap before deploying the controller:

```sh
kubectl apply -f config/configmap.yaml
```

## Deployment

### Helm (recommended)

```sh
# Install into the kube-system namespace (recommended)
helm install node-taint-controller ./helm/node-taint-controller \
  --namespace kube-system --create-namespace

# Customise configuration at install time
helm install node-taint-controller ./helm/node-taint-controller \
  --namespace kube-system --create-namespace \
  --set config.unhealthyDelayMinutes=10 \
  --set config.recoveryDelayMinutes=15 \
  --set image.tag=v1.0.0
```

Key `values.yaml` options:

| Key | Default | Description |
|---|---|---|
| `replicaCount` | `1` | Number of controller replicas |
| `image.repository` | `ghcr.io/jterceiro/node-taint-controller` | Container image |
| `image.tag` | `""` (chart appVersion) | Image tag |
| `namespaceOverride` | `""` (release namespace) | Namespace for namespaced resources |
| `config.unhealthyDelayMinutes` | `"5"` | Minutes before taint is applied |
| `config.recoveryDelayMinutes` | `"10"` | Minutes before taint is removed |
| `config.maxUnhealthyPercentage` | `"33"` | Circuit-breaker threshold |
| `config.nodeLabelSelector` | `""` | Optional node label filter |
| `controller.leaderElect` | `true` | Enable leader election |

### Kustomize

```sh
# Deploy all resources (RBAC, ConfigMap, and Deployment) into kube-system
kubectl apply -k config/
```

To customise settings (e.g. change the image tag or configuration values) create
an overlay that patches the base:

```yaml
# overlays/production/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../config

images:
  - name: ghcr.io/jterceiro/node-taint-controller
    newTag: v1.0.0

patches:
  - patch: |-
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: node-taint-controller-config
        namespace: kube-system
      data:
        unhealthy-delay-minutes: "10"
        recovery-delay-minutes: "15"
```

```sh
kubectl apply -k overlays/production/
```

### Manual prerequisites

```sh
kubectl apply -f config/rbac/serviceaccount.yaml
kubectl apply -f config/rbac/clusterrole.yaml
kubectl apply -f config/rbac/clusterrolebinding.yaml
kubectl apply -f config/configmap.yaml
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--metrics-bind-address` | `:8080` | Metrics endpoint address |
| `--health-probe-bind-address` | `:8081` | Health probe address |
| `--leader-elect` | `true` | Enable leader election |
| `--config-namespace` | `kube-system` | Namespace of the ConfigMap |
| `--config-name` | `node-taint-controller-config` | Name of the ConfigMap |

## Building

```sh
# Run tests
go test -race ./...

# Build binary
go build -o manager ./cmd/

# Build container image
docker build -t node-taint-controller:latest .
```

## CI / CD

CI runs on every PR and push to `main`:

| Job | Description |
|-----|-------------|
| **test** | `go vet ./...` and `go test -race -count=1 ./...` |
| **helm-lint** | `helm lint ./helm/node-taint-controller` |
| **kustomize-build** | `kubectl kustomize ./config` (validates manifests) |

| Workflow | Trigger | Description |
|----------|---------|-------------|
| `ci.yaml` | PR / push to `main` | Tests, Helm lint, Kustomize build |
| `lint.yml` | PR / push to `main` | golangci-lint |
| `security.yml` | PR / push to `main` | Trivy (filesystem + image scan) |
| `publish.yaml` | Push of a `v*.*.*` tag | Builds and pushes the image to GHCR with semantic version tags |
| `release-charts.yml` | Push to `main` | Releases Helm chart when version changes |
