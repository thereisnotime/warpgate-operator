# WarpgateInstance

A `WarpgateInstance` deploys and manages a full Warpgate bastion host directly on Kubernetes. The operator creates
a StatefulSet, Services, ConfigMap, and (optionally) a TLS Certificate, then keeps them in sync with the CR spec.
When `createConnection` is enabled (the default), it also auto-creates a `WarpgateConnection` CR so that other
CRDs like `WarpgateRole` and `WarpgateUser` can reference the deployed instance immediately.

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `version` | `string` | Yes | - | Warpgate image tag to deploy (e.g. `0.21.1`, `latest`) |
| `image` | `string` | No | `ghcr.io/warp-tech/warpgate:<version>` | Full image reference override |
| `replicas` | `int32` | No | `1` | Number of Warpgate pods (minimum 1) |
| `adminPasswordSecretRef.name` | `string` | Yes | - | Name of the Secret containing the initial admin password |
| `adminPasswordSecretRef.key` | `string` | Yes | - | Key in the Secret that holds the admin password |
| `http.enabled` | `bool` | No | `true` | Enable the HTTP/HTTPS listener |
| `http.port` | `int32` | No | `8888` | Container port for HTTP |
| `http.serviceType` | `string` | No | `ClusterIP` | Kubernetes Service type (`ClusterIP`, `NodePort`, `LoadBalancer`) |
| `ssh.enabled` | `bool` | No | - | Enable the SSH listener |
| `ssh.port` | `int32` | No | `2222` | Container port for SSH |
| `ssh.serviceType` | `string` | No | `ClusterIP` | Kubernetes Service type for SSH |
| `mysql.enabled` | `bool` | No | - | Enable the MySQL protocol proxy listener |
| `mysql.port` | `int32` | No | - | Container port for MySQL proxy |
| `postgresql.enabled` | `bool` | No | - | Enable the PostgreSQL protocol proxy listener |
| `postgresql.port` | `int32` | No | - | Container port for PostgreSQL proxy |
| `storage.size` | `string` | No | `1Gi` | PVC size for Warpgate data |
| `storage.storageClassName` | `string` | No | cluster default | StorageClass override |
| `tls.certManager` | `bool` | No | `true` | Enable automatic TLS via cert-manager |
| `tls.issuerRef.name` | `string` | No | - | Name of a cert-manager Issuer or ClusterIssuer |
| `tls.issuerRef.kind` | `string` | No | - | `Issuer` or `ClusterIssuer` |
| `resources` | `ResourceRequirements` | No | - | CPU/memory requests and limits for the Warpgate container |
| `nodeSelector` | `map[string]string` | No | - | Node selector constraints |
| `tolerations` | `[]Toleration` | No | - | Scheduling tolerations |
| `createConnection` | `bool` | No | `true` | Auto-create a `WarpgateConnection` CR pointing to this instance |
| `externalHost` | `string` | No | - | External hostname for cookie domain and URL generation |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `readyReplicas` | `int32` | Number of ready Warpgate pods |
| `version` | `string` | Currently deployed Warpgate version |
| `connectionRef` | `string` | Name of the auto-created `WarpgateConnection` CR (if `createConnection` is true) |
| `endpoint` | `string` | Internal service URL for the Warpgate API |
| `conditions` | `[]Condition` | Standard Kubernetes conditions reflecting the instance state |

## Print Columns

| Column | Source | Type |
|--------|--------|------|
| Version | `.spec.version` | string |
| Replicas | `.spec.replicas` | integer |
| Ready | `.status.readyReplicas` | integer |
| Endpoint | `.status.endpoint` | string |
| Age | `.metadata.creationTimestamp` | date |

## Examples

### Minimal Deployment

The simplest possible deployment -- just a version and an admin password Secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: warpgate-admin
stringData:
  password: my-secure-admin-password
---
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateInstance
metadata:
  name: warpgate
spec:
  version: "0.21.1"
  adminPasswordSecretRef:
    name: warpgate-admin
    key: password
```

This creates a single-replica Warpgate deployment with HTTP on port 8888, a 1Gi PVC, self-signed TLS via cert-manager, and an auto-created `WarpgateConnection` CR.

### Full Deployment with All Options

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateInstance
metadata:
  name: warpgate-production
spec:
  version: "0.21.1"
  replicas: 2
  externalHost: warpgate.example.com
  adminPasswordSecretRef:
    name: warpgate-admin
    key: password
  http:
    enabled: true
    port: 8888
    serviceType: LoadBalancer
  ssh:
    enabled: true
    port: 2222
    serviceType: LoadBalancer
  mysql:
    enabled: true
    port: 33306
  postgresql:
    enabled: true
    port: 55432
  storage:
    size: 10Gi
    storageClassName: fast-ssd
  tls:
    certManager: true
    issuerRef:
      name: letsencrypt-prod
      kind: ClusterIssuer
  resources:
    requests:
      cpu: 250m
      memory: 256Mi
    limits:
      cpu: "1"
      memory: 1Gi
  nodeSelector:
    kubernetes.io/os: linux
  tolerations:
    - key: dedicated
      operator: Equal
      value: bastion
      effect: NoSchedule
  createConnection: true
```

### SSH Enabled

A deployment focused on SSH access with a dedicated LoadBalancer:

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateInstance
metadata:
  name: warpgate-ssh
spec:
  version: "0.21.1"
  adminPasswordSecretRef:
    name: warpgate-admin
    key: password
  http:
    enabled: true
    port: 8888
    serviceType: ClusterIP
  ssh:
    enabled: true
    port: 2222
    serviceType: LoadBalancer
```

## Notes

- **cert-manager integration:** When `tls.certManager` is true (the default), the operator creates a `Certificate`
  resource for TLS. If no `tls.issuerRef` is provided, a self-signed `Issuer` is created automatically. Make sure
  cert-manager is installed in the cluster before deploying a `WarpgateInstance` with cert-manager TLS enabled.
- **Auto-created WarpgateConnection:** When `createConnection` is true, the operator creates a `WarpgateConnection`
  CR in the same namespace, configured to talk to the deployed instance's internal Service URL. The connection name
  is stored in `status.connectionRef`. Other CRDs can reference it via `connectionRef` to manage resources on this
  instance.
- **Storage:** Warpgate stores its database and configuration in `/data`. The operator provisions a PVC via the
  StatefulSet's `volumeClaimTemplates`. Scaling down to zero replicas does not delete the PVC -- data persists
  across restarts.
- **Scale subresource:** The CRD exposes a scale subresource (`spec.replicas` / `status.readyReplicas`), so you can use `kubectl scale` or HPA with it.
- **Admin password Secret:** The Secret must exist in the same namespace as the `WarpgateInstance` CR. The operator reads it at reconciliation time and injects it into the Warpgate configuration.
