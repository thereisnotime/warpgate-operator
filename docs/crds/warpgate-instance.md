# WarpgateInstance

A `WarpgateInstance` deploys and manages a full Warpgate bastion host directly on Kubernetes. The operator creates
a StatefulSet, Services, ConfigMap, and (optionally) a TLS Certificate, then keeps them in sync with the CR spec.
When `createConnection` is enabled (the default), it also auto-creates a `WarpgateConnection` CR so that other
CRDs like `WarpgateRole` and `WarpgateUser` can reference the deployed instance immediately.

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `version` | `string` | No | `0.30.6` | Warpgate image tag to deploy (e.g. `0.26.0`, `0.30.6`, `latest`) |

> Warpgate's ghcr tags carry **no** leading `v` (e.g.
> `ghcr.io/warp-tech/warpgate:0.30.6`). When `version` and `image` are both
> omitted the operator deploys its current default (`0.30.6`). The operator
> supports both the shell-bearing images (<= 0.26) and the distroless images
> (>= 0.27, including 0.30.x): config seeding adapts automatically, so you can
> pin any published tag (k8s targets since 0.21, `--skip-securing-files` since
> ~0.22).
| `image` | `string` | No | `ghcr.io/warp-tech/warpgate:<version>` | Full image reference override |
| `seedImage` | `string` | No | pinned `busybox` | Utility image for the config-seed init container on distroless (>= 0.27) images; needs `sh`/`cp`/`sed`/`awk`. Ignored for <= 0.26 |
| `replicas` | `int32` | No | `1` | Number of Warpgate pods (minimum 1) |
| `adminPasswordSecretRef.name` | `string` | Yes | - | Name of the Secret containing the initial admin password |
| `adminPasswordSecretRef.key` | `string` | Yes | - | Key in the Secret that holds the admin password |
| `http.enabled` | `bool` | No | `true` | Enable the HTTP/HTTPS listener |
| `http.port` | `int32` | No | `8888` | Container port for HTTP |
| `http.externalPort` | `int32` | No | `443` when `externalHost` set | Public port in external URLs and the SSO `redirect_uri`; independent of the in-cluster listener `port` |
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
| `tls.secretName` | `string` | No | - | Existing TLS Secret (`tls.crt`, `tls.key`, optional `ca.crt`) served by Warpgate instead of the cert-manager one |
| `tls.verifyConnection` | `bool` | No | unset | TLS verification for the auto-created `WarpgateConnection`. See notes |
| `resources` | `ResourceRequirements` | No | - | CPU/memory requests and limits for the Warpgate container |
| `nodeSelector` | `map[string]string` | No | - | Node selector constraints |
| `tolerations` | `[]Toleration` | No | - | Scheduling tolerations |
| `createConnection` | `bool` | No | `true` | Auto-create a `WarpgateConnection` CR pointing to this instance |
| `externalHost` | `string` | No | - | External hostname for cookie domain and URL generation |
| `databaseURLSecretRef.name` | `string` | No | - | Secret holding the external (PostgreSQL) database URL. Takes precedence over `databaseURL` |
| `databaseURLSecretRef.key` | `string` | No | `url` | Key in that Secret |
| `databaseURL` | `string` | No | - | **Deprecated.** Inline database URL, stored in plain text in the CR and Deployment. Use `databaseURLSecretRef` |
| `sso.providers` | `[]SSOProvider` | No | - | OpenID Connect SSO providers (see [SSO](#sso)). Ignored when `configOverride` is set |

### SSO

`spec.sso` configures Warpgate's OpenID Connect single sign-on providers as structured fields instead of
hand-writing `sso_providers` into `configOverride`. Each provider's client secret is read from a referenced
`Secret`, injected into the init container as an environment variable, and written into `warpgate.yaml` at
runtime, so it never lands in the generated ConfigMap.

`sso` is ignored when `configOverride` is set, because the override replaces the whole config.

Each entry in `sso.providers`:

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `name` | `string` | Yes | - | Provider id, used in the login URL and to name its injected secret env var |
| `label` | `string` | No | `name` | Human-friendly name shown on the login page |
| `type` | `string` | No | `custom` | Warpgate provider type. Only `custom` (generic OIDC) is supported |
| `autoCreateUsers` | `bool` | No | `false` | Create a Warpgate user on first successful login |
| `clientID` | `string` | Yes | - | OIDC client id |
| `clientSecretSecretRef.name` | `string` | Yes | - | Secret holding the OIDC client secret |
| `clientSecretSecretRef.key` | `string` | No | `client-secret` | Key within that Secret |
| `issuerURL` | `string` | Yes | - | OIDC issuer URL (its `/.well-known/openid-configuration` must be reachable from the pod) |
| `scopes` | `[]string` | Yes | - | OAuth scopes to request; include the scope carrying the `warpgate_roles` claim for role sync |
| `roleMappings` | `map[string]string` | No | - | Map a `warpgate_roles` claim value to a Warpgate role name. When set, only the mapped roles are synced |
| `additionalTrustedAudiences` | `[]string` | No | - | Accept ID tokens whose audience is one of these, in addition to the client id |
| `trustUnknownAudiences` | `bool` | No | `false` | Disable audience validation on the ID token |

**Role sync.** Warpgate maps a user to roles from a `warpgate_roles` claim (an array of role names) that the
provider must emit, so the identity provider is the single source of access. Put the scope that carries it in
`scopes` (named `warpgate_roles` in the example below). With `roleMappings` you translate the provider's values
into Warpgate role names; without it the claim values are used as-is.

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
  version: "0.26.0"
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
  version: "0.26.0"
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
  version: "0.26.0"
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

### SSO with OpenID Connect

A deployment that authenticates users against an OIDC provider (Authentik here), with the client secret sourced from a Secret and roles synced from the `warpgate_roles` claim:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: warpgate-sso-authentik
type: Opaque
stringData:
  client-secret: "oidc-client-secret-value"
---
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateInstance
metadata:
  name: warpgate-hz
spec:
  version: "0.26.0"
  adminPasswordSecretRef:
    name: warpgate-admin
    key: password
  externalHost: gate.hz.example.com
  sso:
    providers:
      - name: authentik
        label: Authentik
        clientID: warpgate-hz
        clientSecretSecretRef:
          name: warpgate-sso-authentik
          key: client-secret
        issuerURL: https://auth.example.com/application/o/warpgate-hz/
        scopes:
          - openid
          - email
          - profile
          - warpgate_roles
        autoCreateUsers: true
        roleMappings:
          hz-k8s-admin: k8s-hz-admin
          hz-k8s-view: k8s-hz-view
```

The client secret never appears in the generated ConfigMap: the operator injects it into the init container as
`WARPGATE_SSO_<NAME>_CLIENT_SECRET` and writes it into `warpgate.yaml` at startup. The provider's redirect URL
is `https://<externalHost>/@warpgate/api/sso/return`.

## Notes

- **Config seeding and image compatibility:** Warpgate's config lives in the read-only ConfigMap the operator
  generates and is reapplied to the writable `/data/warpgate.yaml` on every pod boot, so a ConfigMap change takes
  effect on the next reconcile. How that copy runs depends on the image. Shell-bearing images (<= 0.26, Debian
  based) seed the config with a single in-image init container. Distroless images (>= 0.27, including 0.30.x) ship
  no `/bin/sh` or `cp`, so seeding is split into two init containers: a shell-free `init-setup` that runs the
  Warpgate binary directly to bootstrap the admin user, keys and self-signed certificate once (idempotent via
  Warpgate's own `--if-config-exists skip`), and an `init-config` container built from a small utility image
  (`seedImage`, a pinned `busybox` by default) that performs the file operations. The operator picks the path
  automatically from the resolved version; set `seedImage` to override the utility image (for example to use an
  internal mirror).
- **cert-manager integration:** When `tls.certManager` is true (the default), the operator creates a `Certificate`
  resource for TLS. If no `tls.issuerRef` is provided, a self-signed `Issuer` is created automatically. The
  Certificate's Secret (`<name>-tls`) is mounted into the pod and copied into `/data` by the init container, so
  Warpgate serves the cert-manager certificate. The Deployment is held back until cert-manager has issued that
  Secret; meanwhile the instance reports `Ready=False` with reason `CertificatePending`. When the certificate is
  renewed the pods are restarted to pick it up. Make sure cert-manager is installed in the cluster before deploying a
  `WarpgateInstance` with cert-manager TLS enabled, or set `tls.certManager: false`.
- **Auto-created WarpgateConnection:** When `createConnection` is true, the operator creates a `WarpgateConnection`
  CR in the same namespace, configured to talk to the deployed instance's internal Service URL. The connection name
  is stored in `status.connectionRef`. Other CRDs can reference it via `connectionRef` to manage resources on this
  instance.
- **Connection TLS verification:** The auto-created connection verifies Warpgate's certificate whenever the operator
  knows it: with `tls.secretName`, or with `tls.certManager` (which serves the `<name>-tls` Secret). It verifies
  against that Secret's `ca.crt` (or `tls.crt` when there is no `ca.crt`) through `caSecretRef`, so the certificate
  must be valid for `<name>-http.<namespace>.svc`; the cert-manager Certificate already includes that name.
  `tls.secretName` takes precedence over the cert-manager Secret when both are set. With neither, the pod serves a
  self-signed certificate it generates itself, which the operator never sees, so the connection skips verification.
  Set `tls.verifyConnection: true` to refuse that (reconciliation fails until a TLS Secret or cert-manager is
  configured), or `false` to skip verification even with a TLS Secret, for example when its certificate doesn't
  cover the in-cluster Service name.
- **Storage:** Warpgate stores its database and configuration in `/data`. The operator provisions a PVC via the
  StatefulSet's `volumeClaimTemplates`. Scaling down to zero replicas does not delete the PVC -- data persists
  across restarts.
- **Scale subresource:** The CRD exposes a scale subresource (`spec.replicas` / `status.readyReplicas`), so you can use `kubectl scale` or HPA with it.
- **External database:** Put the connection string in a Secret and reference it with `databaseURLSecretRef`
  (for example `kubectl create secret generic warpgate-db --from-literal=url='postgres://user:pass@host:5432/warpgate'`).
  The URL never lands in the generated ConfigMap: the init container receives it as `DATABASE_URL` from the Secret
  and writes it into `/data/warpgate.yaml` on the data volume. Rotating the Secret takes effect on the next pod
  restart (`kubectl rollout restart deployment/<name>`). The older `databaseURL` field still works but is deprecated,
  since its value is visible to anyone who can read the `WarpgateInstance` or its Deployment.
- **Admin password Secret:** The Secret must exist in the same namespace as the `WarpgateInstance` CR. The operator reads it at reconciliation time and injects it into the Warpgate configuration.
