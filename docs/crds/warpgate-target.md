# WarpgateTarget

A `WarpgateTarget` represents a target host in Warpgate that users can connect to through the bastion.
Seven target types are supported: SSH, HTTP, MySQL, PostgreSQL, Kubernetes, RDP, and VNC. Exactly one type must be set per CR.

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `connectionRef` | `string` | Yes | - | Name of the `WarpgateConnection` CR in the same namespace |
| `name` | `string` | Yes | - | Target name in Warpgate |
| `description` | `string` | No | `""` | Human-readable description |
| `groupRef` | `string` | No | - | Name of a `WarpgateTargetGroup` CR in the same namespace |
| `rateLimitBytesPerSecond` | `int` | No | - | Transfer speed limit for sessions to this target |
| `requireApproval` | `bool` | No | `false` | Every session to this target waits for admin approval |
| `ticketRequestsDisabled` | `bool` | No | `false` | Users cannot request access tickets for this target |
| `ticketRequireApproval` | `bool` | No | `false` | Ticket requests for this target need admin approval |
| `ticketMaxDurationSeconds` | `int` | No | - | Longest ticket validity users may request |
| `ticketMaxUses` | `int` | No | - | Most uses a requested ticket may have |
| `ssh` | `object` | No | - | SSH target configuration (mutually exclusive with other types) |
| `http` | `object` | No | - | HTTP target configuration |
| `mysql` | `object` | No | - | MySQL target configuration |
| `postgresql` | `object` | No | - | PostgreSQL target configuration |
| `kubernetes` | `object` | No | - | Kubernetes target configuration |
| `rdp` | `object` | No | - | RDP target configuration |
| `vnc` | `object` | No | - | VNC target configuration |

### SSH Target

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `ssh.host` | `string` | Yes | - | Hostname or IP of the SSH target |
| `ssh.port` | `int` | Yes | - | SSH port |
| `ssh.username` | `string` | Yes | - | SSH username |
| `ssh.authKind` | `string` | Yes | - | Authentication method: `Password`, `PublicKey`, or `IamRole` |
| `ssh.passwordSecretRef` | `object` | No | - | Secret reference for SSH password (required if `authKind: Password`) |
| `ssh.keyID` | `string` | No | - | UUID of a stored Warpgate client key to use; empty uses the default keys |
| `ssh.allowInsecureAlgos` | `bool` | No | `false` | Allow insecure SSH algorithms |
| `ssh.jumpHostRef` | `string` | No | - | Name of another synced SSH `WarpgateTarget` to jump through |

### HTTP Target

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `http.url` | `string` | Yes | - | Upstream URL of the HTTP target |
| `http.tls` | `object` | No | - | TLS configuration (see TLS Config below) |
| `http.headers` | `map[string]string` | No | - | Additional HTTP headers sent to the upstream |
| `http.externalHost` | `string` | No | - | Override the Host header sent to the upstream |

### MySQL Target

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `mysql.host` | `string` | Yes | - | Hostname or IP of the MySQL server |
| `mysql.port` | `int` | Yes | - | MySQL port |
| `mysql.username` | `string` | Yes | - | MySQL username |
| `mysql.authKind` | `string` | No | `Password` | `Password` or `IamRole` (AWS RDS IAM auth) |
| `mysql.passwordSecretRef` | `object` | No | - | Secret reference for the MySQL password |
| `mysql.tls` | `object` | No | - | TLS configuration |
| `mysql.defaultDatabaseName` | `string` | No | - | Database shown in connection instructions |

### PostgreSQL Target

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `postgresql.host` | `string` | Yes | - | Hostname or IP of the PostgreSQL server |
| `postgresql.port` | `int` | Yes | - | PostgreSQL port |
| `postgresql.username` | `string` | Yes | - | PostgreSQL username |
| `postgresql.protocolVersion` | `string` | No | `3.2` | PostgreSQL protocol version for the target connection (`3.0` or `3.2`) |
| `postgresql.authKind` | `string` | No | `Password` | `Password` or `IamRole` (AWS RDS IAM auth) |
| `postgresql.passwordSecretRef` | `object` | No | - | Secret reference for the PostgreSQL password |
| `postgresql.tls` | `object` | No | - | TLS configuration |
| `postgresql.idleTimeout` | `string` | No | - | Idle target connection timeout (e.g. `5m`) |
| `postgresql.defaultDatabaseName` | `string` | No | - | Database shown in connection instructions |

### Kubernetes Target

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `kubernetes.clusterURL` | `string` | Yes | - | Kubernetes API server URL |
| `kubernetes.authKind` | `string` | Yes | - | `Token`, `Certificate`, or `IamRole` (EKS) |
| `kubernetes.tokenSecretRef` | `object` | No | - | Secret reference for the bearer token (default key `token`). For `Token` auth, set this or `serviceAccountToken` |
| `kubernetes.serviceAccountToken` | `object` | No | - | Request a short-lived token for a ServiceAccount instead of reading one from a Secret. See [ServiceAccount tokens](#serviceaccount-tokens) |
| `kubernetes.serviceAccountToken.name` | `string` | Yes | - | ServiceAccount in the target's namespace |
| `kubernetes.serviceAccountToken.audiences` | `[]string` | Yes | - | 1-8 audiences the token is issued for. No commas or whitespace |
| `kubernetes.serviceAccountToken.expirationSeconds` | `int64` | No | `7200` | Requested lifetime, 7200-172800. Renewed after half of the lifetime actually granted |
| `kubernetes.certificateSecretRef` | `object` | No | - | Secret reference for the PEM client certificate (default key `certificate`) |
| `kubernetes.privateKeySecretRef` | `object` | No | - | Secret reference for the PEM client key (default key `privateKey`) |
| `kubernetes.tls` | `object` | No | - | TLS configuration |

### RDP Target

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `rdp.host` | `string` | Yes | - | Hostname or IP of the RDP server |
| `rdp.port` | `int` | No | `3389` | RDP port |
| `rdp.username` | `string` | Yes | - | Logon username |
| `rdp.domain` | `string` | No | - | Windows logon domain |
| `rdp.passwordSecretRef` | `object` | No | - | Secret reference for the logon password |
| `rdp.verifyTLS` | `bool` | No | `false` | Verify the server certificate against the system root store |
| `rdp.compression` | `string` | No | `remotefx` | `remotefx`, `lossless` or `graphics_pipeline` |
| `rdp.interactiveLogon` | `bool` | No | `false` | Show the target's sign-in screen instead of logging on automatically |
| `rdp.tlsSecurity` | `string` | No | `Tls12` | `Tls12`, `Tls12WithLegacyCiphers`, or `Tls10Unsafe` |

### VNC Target

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `vnc.host` | `string` | Yes | - | Hostname or IP of the VNC server |
| `vnc.port` | `int` | No | `5900` | VNC port |
| `vnc.passwordSecretRef` | `object` | No | - | Secret reference for the VNC password; omit for no authentication |

### TLS Config (shared)

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `tls.mode` | `string` | Yes | - | TLS mode: `Disabled`, `Preferred`, or `Required` |
| `tls.verify` | `bool` | No | `true` | Verify the target's TLS certificate. Set `false` explicitly to accept unverified certificates |

When the `tls` block is omitted entirely, Warpgate's own default applies: mode `Preferred` with verification on.
A `tls` block that leaves out `verify` also verifies. Before this default existed, a block such as
`tls: {mode: Required}` silently turned verification off; add `verify: false` if you really need that.

### SecretKeyRef (shared)

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | `string` | Yes | - | Name of the Kubernetes Secret |
| `key` | `string` | No | per field | Key within the Secret; defaults to `password`, or `token` / `certificate` / `privateKey` for the Kubernetes refs |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `externalID` | `string` | Warpgate-assigned target UUID |
| `serviceAccountToken.expirationTimestamp` | `Time` | Expiry of the token last sent to Warpgate (`serviceAccountToken` targets only). The token itself is never stored |
| `serviceAccountToken.renewAfter` | `Time` | When the operator requests the next token |
| `conditions` | `[]Condition` | Standard Kubernetes conditions |

## Print Columns

| Column | Source |
|--------|--------|
| Name | `.spec.name` |
| Type | `.status.conditions[?(@.type=="Ready")].reason` |
| Ready | `.status.conditions[?(@.type=="Ready")].status` |

## Examples

### SSH Target

```yaml
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
```

### SSH Target with Password Auth

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateTarget
metadata:
  name: legacy-ssh
spec:
  connectionRef: my-warpgate
  name: legacy-ssh
  ssh:
    host: 10.0.1.50
    port: 22
    username: root
    authKind: Password
    passwordSecretRef:
      name: ssh-password
      key: password
    allowInsecureAlgos: true
```

### HTTP Target

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateTarget
metadata:
  name: internal-app
spec:
  connectionRef: my-warpgate
  name: internal-app
  description: Internal web application
  http:
    url: https://internal-app.example.com
    tls:
      mode: Required
      verify: true
    headers:
      X-Custom-Header: value
    externalHost: app.example.com
```

### MySQL Target

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateTarget
metadata:
  name: staging-mysql
spec:
  connectionRef: my-warpgate
  name: staging-mysql
  description: Staging MySQL database
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

### PostgreSQL Target

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateTarget
metadata:
  name: production-postgres
spec:
  connectionRef: my-warpgate
  name: production-postgres
  description: Production PostgreSQL database
  postgresql:
    host: pg.internal
    port: 5432
    username: app_user
    protocolVersion: "3.0"
    passwordSecretRef:
      name: pg-password
      key: password
    tls:
      mode: Required
      verify: true
```

### Kubernetes Target

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateTarget
metadata:
  name: prod-cluster
spec:
  connectionRef: my-warpgate
  name: prod-cluster
  kubernetes:
    clusterURL: https://10.0.2.10:6443
    authKind: Token
    tokenSecretRef:
      name: prod-cluster-sa
    tls:
      mode: Required
      verify: true
```

### Kubernetes Target with a ServiceAccount Token

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: warpgate-k8s-drinking
  annotations:
    # The audiences tokens for this ServiceAccount may be issued for...
    warpgate.warp.tech/token-audiences: zone10-drinking
    # ...and the clusterURLs they may be sent to (exact match).
    warpgate.warp.tech/token-cluster-urls: https://192.168.42.49:6443
---
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateTarget
metadata:
  name: k8s-drinking
spec:
  connectionRef: my-warpgate
  name: k8s-drinking
  kubernetes:
    clusterURL: https://192.168.42.49:6443
    authKind: Token
    serviceAccountToken:
      name: warpgate-k8s-drinking
      audiences: [zone10-drinking]
      expirationSeconds: 7200
```

The operator also needs permission to request the token (Helm value `serviceAccountTokens.allowed`).
See [ServiceAccount tokens](#serviceaccount-tokens).

### RDP Target

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateTarget
metadata:
  name: win-jump
spec:
  connectionRef: my-warpgate
  name: win-jump
  requireApproval: true
  rdp:
    host: 10.0.3.5
    username: Administrator
    domain: CORP
    passwordSecretRef:
      name: win-jump-password
```

### VNC Target

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateTarget
metadata:
  name: kiosk
spec:
  connectionRef: my-warpgate
  name: kiosk
  vnc:
    host: 10.0.4.7
    passwordSecretRef:
      name: kiosk-vnc
```

## Validation

The following rules are enforced by the admission webhook on create and update:

- `spec.connectionRef` must not be empty
- `spec.name` must not be empty
- Exactly one of `spec.ssh`, `spec.http`, `spec.mysql`, `spec.postgresql`, `spec.kubernetes`, `spec.rdp`, or `spec.vnc` must be set
- **SSH targets:** `ssh.host` and `ssh.username` are required; `ssh.port` must be between 1 and 65535; `ssh.authKind` must be `Password`, `PublicKey`, or `IamRole`
- **HTTP targets:** `http.url` is required
- **MySQL targets:** `mysql.host` and `mysql.username` are required; `mysql.port` must be between 1 and 65535
- **PostgreSQL targets:** `postgresql.host` and `postgresql.username` are required; `postgresql.port` must be between 1 and 65535; `postgresql.protocolVersion` must be `3.0` or `3.2` when set
- **Kubernetes targets:** `kubernetes.clusterURL` is required; `kubernetes.authKind` must be `Token`, `Certificate`, or `IamRole`
  - `Token` needs exactly one of `tokenSecretRef` and `serviceAccountToken`
  - `Certificate` needs both `certificateSecretRef` and `privateKeySecretRef`
  - `serviceAccountToken` is rejected for `Certificate` and `IamRole`
  - `serviceAccountToken.name` must be a valid ServiceAccount name; `audiences` needs 1-8 unique values without commas, whitespace or control characters; `expirationSeconds` must be between 7200 and 172800
- Updates that don't change the spec (finalizers, labels, annotations) and updates of objects being deleted are not validated, so objects admitted under older rules can always finish deleting
- **RDP targets:** `rdp.host` and `rdp.username` are required; `rdp.port` must be between 1 and 65535
- **VNC targets:** `vnc.host` is required; `vnc.port` must be between 1 and 65535
- **TLS config** (HTTP, MySQL, PostgreSQL, Kubernetes): `tls.mode` must be one of `Disabled`, `Preferred`, or `Required`

## Defaults

The following defaults are applied on create and update:

- `spec.ssh.port` defaults to `22` if not set
- `spec.ssh.authKind` defaults to `"PublicKey"` if not set
- `spec.mysql.port` defaults to `3306` if not set
- `spec.postgresql.port` defaults to `5432` if not set
- `spec.rdp.port` defaults to `3389` and `spec.vnc.port` to `5900` if not set
- `tls.mode` defaults to `"Preferred"` for HTTP, MySQL, PostgreSQL, and Kubernetes targets when a TLS block is present but the mode is empty

## Notes

- Exactly one target type must be set. Setting zero or more than one is invalid.
- Password secrets must exist in the same namespace as the `WarpgateTarget` CR.
- For SSH targets with `authKind: Password`, the `passwordSecretRef` field is required.

## ServiceAccount Tokens

With `kubernetes.serviceAccountToken`, the operator gets the target's bearer token from the Kubernetes
[TokenRequest API](https://kubernetes.io/docs/reference/kubernetes-api/authentication-resources/token-request-v1/)
instead of a Secret:

1. It reads the ServiceAccount (uncached, so no ServiceAccount informer is needed) and checks its two opt-in
   annotations.
2. It requests a token with exactly the listed `audiences` and `expirationSeconds`.
3. It sends the token to Warpgate and keeps it in memory, never in a Secret, the status, a log or an event.
4. After half of the lifetime the API server actually granted, it requests a new one. It also requests one at once
   when the ServiceAccount (by UID), `clusterURL`, the audiences or the lifetime change, and after an operator
   restart.

Two things must allow a token before one is requested:

- **The ServiceAccount opts in** with two comma-separated, exact-match annotations:
  - `warpgate.warp.tech/token-audiences`: every value in `audiences` must be in it;
  - `warpgate.warp.tech/token-cluster-urls`: `clusterURL` must be in it, so a token is only ever sent where the
    ServiceAccount's owner agreed. Without this pin, anyone who can edit the WarpgateTarget could point
    `clusterURL` at their own host and receive a valid token.
- **The operator may request it.** The generated ClusterRole has no `serviceaccounts/token` permission. With Helm,
  list each ServiceAccount:

  ```yaml
  serviceAccountTokens:
    allowed:
      - namespace: warpgate-operator
        serviceAccounts: [warpgate-k8s-drinking]
  ```

  This renders a Role and RoleBinding in that namespace: `create` on `serviceaccounts/token` and `get` on
  `serviceaccounts`, both limited by `resourceNames`. For raw-manifest installs, see
  [`config/samples/serviceaccount-token-rbac.yaml`](../../config/samples/serviceaccount-token-rbac.yaml).

The trust boundary is the ServiceAccount's namespace. Anyone who can change ServiceAccounts there (`update`,
`patch`, or delete and recreate one with a listed name) controls the annotations. Anyone who can create Pods there
can run as the ServiceAccount and get tokens with any audience without the operator. Keep these ServiceAccounts in
a namespace only administrators can write to, for example the operator's own.

### Condition Reasons

| Reason | Meaning |
|--------|---------|
| `ServiceAccountNotFound` | The ServiceAccount doesn't exist in the target's namespace |
| `ServiceAccountNotOptedIn` | The ServiceAccount lacks `warpgate.warp.tech/token-audiences` or `warpgate.warp.tech/token-cluster-urls` |
| `TokenAudienceNotAllowed` | An audience in the spec is missing from `token-audiences` |
| `TokenClusterURLNotAllowed` | `clusterURL` is not in `token-cluster-urls` |
| `TokenRequestForbidden` | The operator may not read the ServiceAccount or request its tokens (not in `serviceAccountTokens.allowed`) |
| `TokenRequestError` | Any other failure (API errors, an empty token or expiry in the response) |

After a token failure, or a failed Warpgate update of a target holding a token, the operator retries within a
minute, sooner than the reconcile interval. Warpgate keeps the previous token until it expires, so a failed
renewal does not cut access at once.

### Lifetime and Warpgate Sessions

Warpgate reads a target's token when a Kubernetes session opens and keeps it for that session, at most
`kubernetes.session_max_age` (30 minutes by default). New sessions get a renewed token at once. Ones already open
keep the old token for up to that long. The token is renewed at half its lifetime, so keep `expirationSeconds`
above four times `session_max_age`: that leaves twice `session_max_age` after a renewal, with room for retries.
The minimum of 7200 (2h) covers the default.

### Choosing Audiences

Without audiences, TokenRequest issues a token for the API server's own audiences, valid against the operator's
cluster. That is why `audiences` is required. Pick a value only the target cluster accepts. Don't list the local
API server's audience (such as `https://kubernetes.default.svc.cluster.local` or `k3s`) in the annotation unless
the target is the operator's own cluster.

See [docs/guides/kubernetes-serviceaccount-tokens.md](../guides/kubernetes-serviceaccount-tokens.md) for setting
up a remote cluster to accept these tokens.
