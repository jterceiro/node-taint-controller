# Contributing to node-taint-controller

Thank you for your interest in contributing! We welcome all contributions, whether they are bug reports, feature requests, documentation improvements, or code changes. This guide will help you get started.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Setting Up Your Local Environment](#setting-up-your-local-environment)
- [Development Loop](#development-loop)
- [Running Tests](#running-tests)
- [Commit Message Guidelines](#commit-message-guidelines)
- [Submitting a Pull Request](#submitting-a-pull-request)

---

## Code of Conduct

This project follows the [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/main/code-of-conduct.md). Please read it before participating.

---

## Setting Up Your Local Environment

### Prerequisites

Ensure you have the following tools installed:

| Tool | Version | Purpose |
|---|---|---|
| [Go](https://go.dev/dl/) | ≥ 1.22 | Primary language |
| [Kustomize](https://kubectl.docs.kubernetes.io/installation/kustomize/) | ≥ 5.4 | Kubernetes manifest management |
| [KIND](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) | ≥ 0.23 | Local Kubernetes clusters |
| [kubectl](https://kubernetes.io/docs/tasks/tools/) | compatible with your cluster | Cluster interaction |
| [Docker](https://docs.docker.com/get-docker/) | ≥ 24 | Container image builds |

### Clone the Repository

```sh
git clone https://github.com/jterceiro/node-taint-controller.git
cd node-taint-controller
```

### Install Go Dependencies

```sh
go mod download
```

### Install Build-time Tools

The Makefile manages all tool binaries under the local `bin/` directory so they do not pollute your `$GOPATH`:

```sh
# Install controller-gen, setup-envtest, kustomize, and golangci-lint
make controller-gen envtest kustomize golangci-lint
```

### Create a Local KIND Cluster

```sh
kind create cluster --name node-taint-dev
```

Verify the cluster is running:

```sh
kubectl cluster-info --context kind-node-taint-dev
```

---

## Development Loop

Use the following workflow to iterate on changes locally **without** building or pushing container images.

### 1. Regenerate Manifests

Whenever you add or modify RBAC markers, CRD types, or webhook configurations, regenerate the manifests:

```sh
make manifests
```

### 2. Apply Resources to the Cluster

Deploy the RBAC, ConfigMap, and CRD resources into your local KIND cluster:

```sh
kubectl apply -k config/
```

### 3. Run the Controller Locally

Run the controller binary directly against your KIND cluster (uses `~/.kube/config` by default):

```sh
go run ./cmd/ \
  --leader-elect=false \
  --config-namespace=kube-system \
  --config-name=node-taint-controller-config
```

> **Tip:** Set `--leader-elect=false` when running a single local instance to avoid needing a lease.

### 4. Lint and Format

```sh
make fmt      # auto-format code with go fmt
make vet      # run go vet
make lint     # run golangci-lint (set FIX=1 to apply auto-fixes)
```

### 5. Build the Binary

```sh
make build
```

---

## Running Tests

### Unit and Integration Tests

The test suite uses [controller-runtime's envtest](https://book.kubebuilder.io/reference/envtest) to spin up a lightweight API server. Run all tests with:

```sh
make test
```

This target automatically downloads the correct Kubernetes binaries via `setup-envtest` and sets `KUBEBUILDER_ASSETS` for you.

### Running Tests Manually

```sh
KUBEBUILDER_ASSETS="$(bin/setup-envtest use 1.31.0 --bin-dir bin -p path)" \
  go test ./... -v -race
```

---

## Commit Message Guidelines

This project follows the [Conventional Commits](https://www.conventionalcommits.org/) specification. Each commit message should have a **type**, an optional **scope**, and a short **description**:

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

### Common Types

| Type | When to use |
|---|---|
| `feat` | A new feature |
| `fix` | A bug fix |
| `docs` | Documentation-only changes |
| `refactor` | Code change that is neither a fix nor a feature |
| `test` | Adding or updating tests |
| `chore` | Maintenance tasks (dependency bumps, CI changes, etc.) |
| `ci` | Changes to CI/CD workflows |

### Examples

```
feat: add circuit-breaker bypass flag
fix: prevent double-taint on rapid state transitions
docs: update Helm values table in README
chore: bump golangci-lint to v1.64.8
```

---

## Submitting a Pull Request

1. **Fork** the repository and create your branch from `main`:
   ```sh
   git checkout -b feat/my-new-feature
   ```
2. Make your changes and ensure `make fmt`, `make vet`, `make lint`, and `make test` all pass.
3. Commit your changes using the [Conventional Commits](#commit-message-guidelines) format.
4. Push to your fork and open a Pull Request against `main`.
5. Fill in the PR template and link any related issues.
6. A maintainer will review your PR. Please address feedback promptly.

Thank you for contributing! 🎉
