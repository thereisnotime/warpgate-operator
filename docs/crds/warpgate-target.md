# WarpgateTarget

A `WarpgateTarget` represents a target host in Warpgate that users can connect to through the bastion.
Four target types are supported: SSH, HTTP, MySQL, and PostgreSQL. Exactly one type must be set per CR.

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `connectionRef` | `string` | Yes | - | Name of the `WarpgateConnection` CR in the same namespace |
| `name` | `string` | Yes | - | Target name in Warpgate |
| `description` | `string` | No | `""` | Human-readable description |
| `ssh` | `object` | No | - | SSH target configuration (mutually exclusive with other types) |
| `http` | `object` | No | - | HTTP target configuration |
| `mysql` | `object` | No | - | MySQL target configuration |
| `postgresql` | `object` | No | - | PostgreSQL target configuration |

### SSH Target

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `ssh.host` | `string` | Yes | - | Hostname or IP of the SSH target |
| `ssh.port` | `int` | Yes | - | SSH port |
| `ssh.username` | `string` | Yes | - | SSH username |
| `ssh.authKind` | `string` | Yes | - | Authentication method: `Password` or `PublicKey` |
| `ssh.passwordSecretRef` | `object` | No | - | Secret reference for SSH password (required if `authKind: Password`) |
| `ssh.allowInsecureAlgos` | `bool` | No | `false` | Allow insecure SSH algorithms |

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
| `mysql.passwordSecretRef` | `object` | No | - | Secret reference for the MySQL password |
| `mysql.tls` | `object` | No | - | TLS configuration |

### PostgreSQL Target

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `postgresql.host` | `string` | Yes | - | Hostname or IP of the PostgreSQL server |
| `postgresql.port` | `int` | Yes | - | PostgreSQL port |
| `postgresql.username` | `string` | Yes | - | PostgreSQL username |
| `postgresql.passwordSecretRef` | `object` | No | - | Secret reference for the PostgreSQL password |
| `postgresql.tls` | `object` | No | - | TLS configuration |

### TLS Config (shared)

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `tls.mode` | `string` | Yes | - | TLS mode: `Disabled`, `Preferred`, or `Required` |
| `tls.verify` | `bool` | No | `false` | Enable TLS certificate verification |

### SecretKeyRef (shared)

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | `string` | Yes | - | Name of the Kubernetes Secret |
| `key` | `string` | No | `token` | Key within the Secret |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `externalID` | `string` | Warpgate-assigned target UUID |
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
    passwordSecretRef:
      name: pg-password
      key: password
    tls:
      mode: Required
      verify: true
```

## Notes

- Exactly one of `ssh`, `http`, `mysql`, or `postgresql` must be set. Setting zero or more than one is invalid.
- Password secrets must exist in the same namespace as the `WarpgateTarget` CR.
- For SSH targets with `authKind: Password`, the `passwordSecretRef` field is required.
