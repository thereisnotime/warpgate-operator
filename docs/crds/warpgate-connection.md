# WarpgateConnection

A `WarpgateConnection` represents a connection to a Warpgate instance. All other CRDs reference a connection by name via the `connectionRef` field, so you need at least one before creating any other resources.

The operator uses the connection details to authenticate against the Warpgate REST API using session-based auth with a username and password stored in a Kubernetes Secret.

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `host` | `string` | Yes | - | URL of the Warpgate instance (e.g. `https://warpgate.example.com`) |
| `tokenSecretRef.name` | `string` | Yes | - | Name of the Kubernetes Secret containing the login credentials |
| `insecureSkipVerify` | `bool` | No | `false` | Disable TLS certificate verification (not recommended for production) |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | `[]Condition` | Standard Kubernetes conditions reflecting the connection state |

Standard condition types: `Available`, `Progressing`, `Degraded`.

## Example

First, create the Secret holding your Warpgate admin credentials:

```bash
kubectl create secret generic warpgate-credentials \
  --from-literal=username=YOUR_WARPGATE_ADMIN_USER \
  --from-literal=password=YOUR_WARPGATE_ADMIN_PASSWORD
```

Then create the connection:

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateConnection
metadata:
  name: my-warpgate
spec:
  host: https://warpgate.example.com
  tokenSecretRef:
    name: warpgate-credentials
  insecureSkipVerify: false
```

## Validation

The following rules are enforced by the admission webhook on create and update:

- `spec.host` must not be empty and must start with `http://` or `https://`
- `spec.tokenSecretRef.name` must not be empty

## Defaults

No defaults are applied. The referenced Secret must contain `username` and `password` keys.

## Notes

- The credentials Secret must exist in the same namespace as the `WarpgateConnection` CR.
- The operator reads the `username` and `password` keys from the referenced Secret for session-based authentication.
- The `insecureSkipVerify` flag is provided for development/testing environments with self-signed certificates. Avoid using it in production.
- Multiple `WarpgateConnection` resources can coexist in the same namespace, each pointing to a different Warpgate instance. Other CRDs select which instance to use via `connectionRef`.
