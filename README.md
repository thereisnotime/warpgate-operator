# Warpgate Operator

A Kubernetes operator that manages [Warpgate](https://github.com/warp-tech/warpgate) resources declaratively through Custom Resource Definitions. It mirrors the full surface of the [Warpgate Terraform provider](https://registry.terraform.io/providers/warp-tech/warpgate/latest/docs), letting you manage bastion host access as native Kubernetes resources.

## Features

- **9 CRDs** covering all Warpgate resource types: connections, roles, users, targets, bindings, credentials, and tickets
- **Multi-instance support** via `WarpgateConnection` CRDs pointing to different Warpgate instances
- **Drift reconciliation** that continuously enforces the desired state defined in your CRs
- **Secret references** for sensitive fields (API tokens, passwords, SSH keys) instead of inline values
- **Finalizer-based cleanup** that removes Warpgate resources when the corresponding CR is deleted
- **Namespace-scoped CRs** managed by a cluster-scoped operator

## Custom Resources

| CRD | Description |
|-----|-------------|
| `WarpgateConnection` | Connection details for a Warpgate instance |
| `WarpgateRole` | Role definitions |
| `WarpgateUser` | User accounts with credential policies |
| `WarpgateTarget` | Target hosts (SSH, HTTP, MySQL, PostgreSQL) |
| `WarpgateUserRole` | User-to-role bindings |
| `WarpgateTargetRole` | Target-to-role bindings |
| `WarpgatePasswordCredential` | Password credentials for users |
| `WarpgatePublicKeyCredential` | SSH public key credentials for users |
| `WarpgateTicket` | One-time access tickets (auto-creates a Secret with the ticket secret) |

All resources belong to the API group `warpgate.warpgate.warp.tech/v1alpha1`.

## Prerequisites

- Kubernetes 1.30+
- [just](https://github.com/casey/just) (task runner)
- [Podman](https://podman.io/) (container runtime)
- Go 1.26+ (for development)

## Quick Start

### 1. Create the API token Secret

```bash
kubectl create secret generic warpgate-token \
  --from-literal=token=YOUR_WARPGATE_ADMIN_TOKEN
```

### 2. Define a connection

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateConnection
metadata:
  name: my-warpgate
spec:
  host: https://warpgate.example.com
  tokenSecretRef:
    name: warpgate-token
    key: token
```

### 3. Manage resources

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateRole
metadata:
  name: developers
spec:
  connectionRef: my-warpgate
  name: developers
  description: Role for developer access
---
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateUser
metadata:
  name: john-doe
spec:
  connectionRef: my-warpgate
  username: john.doe
  description: Developer user
  credentialPolicy:
    ssh:
      - PublicKey
    http:
      - Password
---
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateTarget
metadata:
  name: production-ssh
spec:
  connectionRef: my-warpgate
  name: production-ssh
  description: Production SSH bastion target
  ssh:
    host: 10.0.1.100
    port: 22
    username: admin
    authKind: PublicKey
---
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateUserRole
metadata:
  name: john-doe-developers
spec:
  connectionRef: my-warpgate
  username: john.doe
  roleName: developers
---
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateTargetRole
metadata:
  name: production-ssh-developers
spec:
  connectionRef: my-warpgate
  targetName: production-ssh
  roleName: developers
```

### 4. Manage credentials

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgatePublicKeyCredential
metadata:
  name: john-doe-laptop
spec:
  connectionRef: my-warpgate
  username: john.doe
  label: laptop-key
  opensshPublicKey: ssh-ed25519 AAAAC3NzaC1lZDI1NTE5... john@laptop
---
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgatePasswordCredential
metadata:
  name: john-doe-password
spec:
  connectionRef: my-warpgate
  username: john.doe
  passwordSecretRef:
    name: john-doe-password-secret
    key: password
```

### 5. Issue a ticket

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateTicket
metadata:
  name: onetime-access
spec:
  connectionRef: my-warpgate
  username: john.doe
  targetName: production-ssh
  numberOfUses: 1
  description: One-time access for maintenance window
```

The operator automatically creates a Kubernetes Secret named `<ticket-name>-secret` containing the ticket secret value.

## Target Types

The `WarpgateTarget` CRD supports four target types via mutually exclusive spec fields:

**SSH**
```yaml
spec:
  ssh:
    host: 10.0.1.100
    port: 22
    username: admin
    authKind: PublicKey  # or Password
    passwordSecretRef:   # required if authKind: Password
      name: ssh-password
      key: password
```

**HTTP**
```yaml
spec:
  http:
    url: https://internal-app.example.com
    tls:
      mode: Required  # Disabled, Preferred, or Required
      verify: true
    headers:
      X-Custom-Header: value
    externalHost: app.example.com
```

**MySQL**
```yaml
spec:
  mysql:
    host: db.internal
    port: 3306
    username: app_user
    passwordSecretRef:
      name: mysql-password
      key: password
    tls:
      mode: Preferred
      verify: false
```

**PostgreSQL**
```yaml
spec:
  postgresql:
    host: pg.internal
    port: 5432
    username: app_user
    passwordSecretRef:
      name: pg-password
      key: password
    tls:
      mode: Required
      verify: true
```

## Development

### Available Commands

Run `just` to see all recipes, or `just help` for details.

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

### Local Development with Minikube

The justfile includes recipes for managing a local minikube cluster using the podman driver:

```bash
# Start a minikube cluster with podman
just minikube-up

# Full dev cycle: start cluster, build image, load, install CRDs, deploy
just minikube-deploy

# Tail operator logs
just minikube-logs

# Show cluster status
just minikube-status

# Stop cluster (preserves state)
just minikube-stop

# Destroy cluster completely
just minikube-destroy

# Full teardown: undeploy, uninstall CRDs, destroy cluster
just minikube-teardown
```

Minikube settings are configurable via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `MINIKUBE_PROFILE` | `warpgate-operator` | Minikube profile name |
| `MINIKUBE_CPUS` | `2` | CPU allocation |
| `MINIKUBE_MEMORY` | `4096` | Memory allocation (MB) |
| `MINIKUBE_K8S_VERSION` | `stable` | Kubernetes version |

### Running Tests

```bash
# Unit tests
just test

# E2E tests (starts minikube automatically)
just test-e2e
```

### Building and Deploying

```bash
# Build and push container image
just image-build IMG=ghcr.io/thereisnotime/warpgate-operator:latest
just image-push IMG=ghcr.io/thereisnotime/warpgate-operator:latest

# Deploy to a cluster
just deploy IMG=ghcr.io/thereisnotime/warpgate-operator:latest

# Generate a standalone install manifest
just build-installer IMG=ghcr.io/thereisnotime/warpgate-operator:latest
# Output: dist/install.yaml
```

## Architecture

```
WarpgateConnection CR ──► Operator reads host + token secret
        │                         │
        │                         ▼
        │                  Warpgate REST API
        │                         │
        ▼                         ▼
  Resource CRs ──────────► Create / Update / Delete
  (Role, User,              Warpgate resources
   Target, etc.)
        │
        └── On CR deletion ──► Finalizer removes
                                Warpgate resource
```

Each resource CR references a `WarpgateConnection` by name (in the same namespace) via the `connectionRef` field. The operator resolves the connection, reads the API token from the referenced Kubernetes Secret, and communicates with the Warpgate REST API.

Reconciliation runs every 5 minutes to detect and correct drift.

## License

Apache License 2.0. See individual source files for the full header.
