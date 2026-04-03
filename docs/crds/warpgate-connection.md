# WarpgateConnection

A `WarpgateConnection` represents a connection to a Warpgate instance. All other CRDs reference a connection by name via the `connectionRef` field, so you need at least one before creating any other resources.

The operator uses the connection details to authenticate against the Warpgate REST API using a token stored in a Kubernetes Secret.

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `host` | `string` | Yes | - | URL of the Warpgate instance (e.g. `https://warpgate.example.com`) |
| `tokenSecretRef.name` | `string` | Yes | - | Name of the Kubernetes Secret containing the API token |
| `tokenSecretRef.key` | `string` | No | `token` | Key within the Secret that holds the token value |
| `insecureSkipVerify` | `bool` | No | `false` | Disable TLS certificate verification (not recommended for production) |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | `[]Condition` | Standard Kubernetes conditions reflecting the connection state |

Standard condition types: `Available`, `Progressing`, `Degraded`.

## Example

First, create the Secret holding your Warpgate admin API token:

```bash
kubectl create secret generic warpgate-token \
  --from-literal=token=YOUR_WARPGATE_ADMIN_TOKEN
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
    name: warpgate-token
    key: token
  insecureSkipVerify: false
```

## Validation

The following rules are enforced by the admission webhook on create and update:

- `spec.host` must not be empty and must start with `http://` or `https://`
- `spec.tokenSecretRef.name` must not be empty

## Defaults

The following defaults are applied on create and update:

- `spec.tokenSecretRef.key` defaults to `"token"` if not set

## Notes

- The token Secret must exist in the same namespace as the `WarpgateConnection` CR.
- If `tokenSecretRef.key` is omitted, the operator defaults to looking up the key `token` in the referenced Secret.
- The `insecureSkipVerify` flag is provided for development/testing environments with self-signed certificates. Avoid using it in production.
- Multiple `WarpgateConnection` resources can coexist in the same namespace, each pointing to a different Warpgate instance. Other CRDs select which instance to use via `connectionRef`.
