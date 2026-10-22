# warpgate-operator Helm Chart

Kubernetes operator for managing [Warpgate](https://github.com/warp-tech/warpgate) bastion host
resources via CRDs. Mirrors the full surface of the
[Warpgate Terraform provider](https://registry.terraform.io/providers/warp-tech/warpgate/latest/docs).

## Prerequisites

- Kubernetes 1.25+
- Helm 3.10+
- [cert-manager](https://cert-manager.io/) (required for admission webhooks)

## Install

```bash
helm install warpgate-operator oci://ghcr.io/thereisnotime/charts/warpgate-operator \
  --namespace warpgate-operator \
  --create-namespace
```

## CRDs

The chart installs 10 CRDs under the `warpgate.warpgate.warp.tech/v1alpha1` API group:

| CRD | Purpose |
|-----|---------|
| `WarpgateInstance` | Deploy and manage Warpgate servers on Kubernetes |
| `WarpgateConnection` | Connect to external or self-hosted Warpgate instances |
| `Role` | Warpgate role |
| `User` | Warpgate user |
| `Target` | SSH, HTTP, MySQL, PostgreSQL, Kubernetes, RDP, or VNC target |
| `UserRole` | Bind a user to a role |
| `TargetRole` | Bind a target to a role |
| `PasswordCredential` | Password credential for a user |
| `PublicKeyCredential` | SSH public key credential for a user |
| `Ticket` | Short-lived access ticket |

## Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Operator image repository | `ghcr.io/thereisnotime/warpgate-operator` |
| `image.tag` | Image tag (defaults to chart appVersion) | `""` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `replicaCount` | Number of operator replicas | `1` |
| `crds.install` | Install CRDs as part of the release | `true` |
| `rbac.create` | Create the manager ClusterRole and its binding (a ClusterRoleBinding, or RoleBindings when the watched namespaces are limited) | `true` |
| `serviceAccount.create` | Create a ServiceAccount | `true` |
| `webhooks.enabled` | Enable admission webhooks | `true` |
| `webhooks.certManager.enabled` | Use cert-manager for webhook TLS | `true` |
| `webhooks.certManager.issuerRef.name` | Existing Issuer/ClusterIssuer name (empty = create self-signed) | `""` |
| `metrics.enabled` | Enable metrics endpoint | `true` |
| `metrics.secure` | Serve metrics over HTTPS | `true` |
| `leaderElection.enabled` | Enable leader election for HA | `true` |
| `operator.watchNamespaces` | Namespaces to watch (empty = all, unless `operator.watchNamespaceSelector` is set) | `[]` |
| `operator.watchNamespaceSelector` | Label selector of additional namespaces to watch | `""` |
| `resources.requests.cpu` | CPU request | `100m` |
| `resources.requests.memory` | Memory request | `128Mi` |
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `256Mi` |

## Limiting the Watched Namespaces

By default the operator watches all namespaces and its ClusterRole is bound cluster-wide. Set
`operator.watchNamespaces` and/or `operator.watchNamespaceSelector` to watch only some namespaces:
the operator then caches and reads objects, including Secrets, only there, and the chart binds the
manager ClusterRole with a RoleBinding in each listed namespace and the release namespace instead
of a ClusterRoleBinding.

```yaml
operator:
  watchNamespaces:
    - team-a
  watchNamespaceSelector: warpgate.warp.tech/watch=true
```

Namespaces selected only by label need a RoleBinding to the
manager ClusterRole (`<fullname>-manager`), created alongside the label. The operator
restarts itself when the set of matching namespaces changes.

## Links

- [GitHub](https://github.com/thereisnotime/warpgate-operator)
- [Warpgate](https://github.com/warp-tech/warpgate)
