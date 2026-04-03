# Warpgate Operator

A Kubernetes operator that manages [Warpgate](https://github.com/warp-tech/warpgate) bastion host resources declaratively through CRDs.

## Table of Contents

- [Features](#features)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [CRD Reference](#crd-reference)
- [Development](#development)
- [Architecture](#architecture)
- [Roadmap](#roadmap)
- [License](#license)

## Features

- 9 CRDs covering all Warpgate resource types (connections, roles, users, targets, bindings, credentials, tickets)
- Multi-instance support via `WarpgateConnection` CRDs pointing to different Warpgate instances
- Continuous drift reconciliation that enforces desired state every 5 minutes
- Secret references for sensitive fields -- no inline tokens or passwords in CRD specs
- Finalizer-based cleanup that removes Warpgate resources when CRs are deleted
- Auto-generated passwords for users, stored in Kubernetes Secrets
- Auto-created Secrets for ticket values

## Prerequisites

- Kubernetes 1.30+
- [just](https://github.com/casey/just) (task runner)
- [Podman](https://podman.io/) (container runtime)
- Go 1.26+ (for development)

## Quick Start

Create a Secret with your Warpgate admin API token, then define a connection, a role, and a user:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: warpgate-token
stringData:
  token: YOUR_WARPGATE_ADMIN_TOKEN
```

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateConnection
metadata:
  name: my-warpgate
spec:
  host: https://warpgate.example.com
  tokenSecretRef:
    name: warpgate-token
```

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateRole
metadata:
  name: developers
spec:
  connectionRef: my-warpgate
  name: developers
---
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateUser
metadata:
  name: john-doe
spec:
  connectionRef: my-warpgate
  username: john.doe
```

See the [CRD reference docs](#crd-reference) for full field details and more examples.

## CRD Reference

All resources belong to the API group `warpgate.warpgate.warp.tech/v1alpha1`.

| Kind | Description | Docs |
|------|-------------|------|
| `WarpgateConnection` | Connection to a Warpgate instance | [docs/crds/warpgate-connection.md](docs/crds/warpgate-connection.md) |
| `WarpgateRole` | Role definition | [docs/crds/warpgate-role.md](docs/crds/warpgate-role.md) |
| `WarpgateUser` | User account with credential policy and auto-generated password | [docs/crds/warpgate-user.md](docs/crds/warpgate-user.md) |
| `WarpgateTarget` | Target host (SSH, HTTP, MySQL, PostgreSQL) | [docs/crds/warpgate-target.md](docs/crds/warpgate-target.md) |
| `WarpgateUserRole` | User-to-role binding | [docs/crds/warpgate-user-role.md](docs/crds/warpgate-user-role.md) |
| `WarpgateTargetRole` | Target-to-role binding | [docs/crds/warpgate-target-role.md](docs/crds/warpgate-target-role.md) |
| `WarpgatePasswordCredential` | Password credential for a user | [docs/crds/warpgate-password-credential.md](docs/crds/warpgate-password-credential.md) |
| `WarpgatePublicKeyCredential` | SSH public key credential for a user | [docs/crds/warpgate-public-key-credential.md](docs/crds/warpgate-public-key-credential.md) |
| `WarpgateTicket` | One-time access ticket (auto-creates Secret) | [docs/crds/warpgate-ticket.md](docs/crds/warpgate-ticket.md) |

## Development

### Commands

| Recipe | Description |
|--------|-------------|
| `just build` | Build the operator binary |
| `just test` | Run unit tests |
| `just lint` | Run golangci-lint |
| `just manifests` | Generate CRD manifests and RBAC |
| `just generate` | Generate DeepCopy boilerplate |
| `just image-build` | Build the container image (podman) |
| `just install` | Install CRDs into the current cluster |
| `just deploy` | Deploy the operator to the current cluster |
| `just run` | Run the operator locally against current kubeconfig |

### Minikube

```bash
just minikube-up          # Start cluster with podman driver
just minikube-deploy      # Full dev cycle: cluster + build + install + deploy
just minikube-logs        # Tail operator logs
just minikube-teardown    # Full teardown: undeploy + uninstall + destroy
```

| Variable | Default | Description |
|----------|---------|-------------|
| `MINIKUBE_PROFILE` | `warpgate-operator` | Minikube profile name |
| `MINIKUBE_CPUS` | `2` | CPU allocation |
| `MINIKUBE_MEMORY` | `4096` | Memory allocation (MB) |
| `MINIKUBE_K8S_VERSION` | `stable` | Kubernetes version |

### Building and Deploying

```bash
just image-build IMG=ghcr.io/thereisnotime/warpgate-operator:latest
just image-push IMG=ghcr.io/thereisnotime/warpgate-operator:latest
just deploy IMG=ghcr.io/thereisnotime/warpgate-operator:latest
just build-installer IMG=ghcr.io/thereisnotime/warpgate-operator:latest  # outputs dist/install.yaml
```

## Architecture

```text
WarpgateConnection CR --> Operator reads host + token secret
        |                         |
        |                         v
        |                  Warpgate REST API
        |                         |
        v                         v
  Resource CRs ----------> Create / Update / Delete
  (Role, User,              Warpgate resources
   Target, etc.)
        |
        +-- On CR deletion --> Finalizer removes
                                Warpgate resource
```

Each resource CR references a `WarpgateConnection` by name (same namespace) via `connectionRef`. The operator resolves the connection,
reads the API token from the referenced Secret, and communicates with the Warpgate REST API.

## Roadmap

- Webhook validation for CRD specs
- Kubernetes target type support
- SSO credential management
- Helm chart published to artifact hub
- Prometheus metrics and alerts
- Multi-architecture container images
- Comprehensive E2E test suite

## License

Apache License 2.0. See individual source files for the full header.
