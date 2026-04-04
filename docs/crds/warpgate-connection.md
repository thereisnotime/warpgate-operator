# WarpgateConnection

A `WarpgateConnection` represents a connection to a Warpgate instance. All other CRDs reference a connection by name via the `connectionRef` field, so you need at least one before creating any other resources.

You can create a `WarpgateConnection` manually, or let a [`WarpgateInstance`](warpgate-instance.md) auto-create one when deploying a self-hosted Warpgate server.

The operator supports two authentication modes against the Warpgate REST API:

1. **Bearer token** (recommended) -- a single API token that bypasses OTP/2FA requirements.
2. **Username/password** (fallback) -- session-based authentication, requires OTP to be disabled on the Warpgate instance.

Auth mode is auto-detected from the referenced Secret: if the `token` key exists, bearer token auth is used. Otherwise the operator falls back to username/password session auth.

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `host` | `string` | Yes | - | URL of the Warpgate instance (e.g. `https://warpgate.example.com`) |
| `authSecretRef.name` | `string` | Yes | - | Name of the Kubernetes Secret containing auth credentials |
| `authSecretRef.tokenKey` | `string` | No | `token` | Key in the Secret that holds the API token (bearer auth) |
| `authSecretRef.usernameKey` | `string` | No | `username` | Key in the Secret that holds the username (session auth) |
| `authSecretRef.passwordKey` | `string` | No | `password` | Key in the Secret that holds the password (session auth) |
| `insecureSkipVerify` | `bool` | No | `false` | Disable TLS certificate verification (not recommended for production) |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | `[]Condition` | Standard Kubernetes conditions reflecting the connection state |

Standard condition types: `Available`, `Progressing`, `Degraded`.

## Auth Mode: Bearer Token (recommended)

Create a Secret with your Warpgate API token:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: warpgate-auth
stringData:
  token: YOUR_WARPGATE_API_TOKEN
```

Then create the connection:

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateConnection
metadata:
  name: my-warpgate
spec:
  host: https://warpgate.example.com
  authSecretRef:
    name: warpgate-auth
  insecureSkipVerify: false
```

The operator sees the `token` key in the Secret and uses bearer token authentication. This is the recommended approach because it works regardless of OTP/2FA settings on the Warpgate instance.

## Auth Mode: Username/Password (fallback)

For Warpgate instances without OTP enabled, you can use username/password authentication instead:

```bash
kubectl create secret generic warpgate-auth \
  --from-literal=username=YOUR_WARPGATE_ADMIN_USER \
  --from-literal=password=YOUR_WARPGATE_ADMIN_PASSWORD
```

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateConnection
metadata:
  name: my-warpgate
spec:
  host: https://warpgate.example.com
  authSecretRef:
    name: warpgate-auth
```

Since the Secret has no `token` key, the operator falls back to session-based auth using the `username` and `password` keys.

## Custom Key Names

If your Secret uses non-default key names, specify them explicitly:

```yaml
spec:
  authSecretRef:
    name: my-secret
    tokenKey: api-token
    usernameKey: admin_user
    passwordKey: admin_pass
```

## Validation

The following rules are enforced by the admission webhook on create and update:

- `spec.host` must not be empty and must start with `http://` or `https://`
- `spec.authSecretRef.name` must not be empty

## Defaults

The admission webhook applies these defaults if not set:

- `authSecretRef.tokenKey` defaults to `token`
- `authSecretRef.usernameKey` defaults to `username`
- `authSecretRef.passwordKey` defaults to `password`

## Notes

- **Auto-created by WarpgateInstance:** When a `WarpgateInstance` CR has `createConnection: true` (the default),
  the operator automatically creates a `WarpgateConnection` in the same namespace, pre-configured to talk to the
  deployed instance. You don't need to create one manually in that case -- just reference the auto-created
  connection name (found in the `WarpgateInstance` status) from your other CRDs.
- The auth Secret must exist in the same namespace as the `WarpgateConnection` CR.
- If the Secret contains the token key, bearer auth is used regardless of whether username/password keys also exist.
- Username/password session auth requires OTP to be disabled on the Warpgate instance. If OTP is enabled, use token auth instead.
- The `insecureSkipVerify` flag is provided for development/testing environments with self-signed certificates. Avoid using it in production.
- Multiple `WarpgateConnection` resources can coexist in the same namespace, each pointing to a different Warpgate instance. Other CRDs select which instance to use via `connectionRef`.
