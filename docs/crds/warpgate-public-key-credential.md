# WarpgatePublicKeyCredential

A `WarpgatePublicKeyCredential` adds an SSH public key credential to an existing Warpgate user.

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `connectionRef` | `string` | Yes | - | Name of the `WarpgateConnection` CR in the same namespace |
| `username` | `string` | Yes | - | Warpgate username to add the credential to |
| `label` | `string` | Yes | - | Human-readable label for the key (e.g. `laptop-key`) |
| `opensshPublicKey` | `string` | Yes | - | SSH public key in OpenSSH format |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `userID` | `string` | Resolved Warpgate user UUID |
| `credentialID` | `string` | Warpgate-assigned credential UUID |
| `conditions` | `[]Condition` | Standard Kubernetes conditions |

## Print Columns

| Column | Source |
|--------|--------|
| Username | `.spec.username` |
| Label | `.spec.label` |
| CredentialID | `.status.credentialID` |
| Ready | `.status.conditions[?(@.type=="Ready")].status` |

## Example

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgatePublicKeyCredential
metadata:
  name: john-doe-laptop
spec:
  connectionRef: my-warpgate
  username: john.doe
  label: laptop-key
  opensshPublicKey: ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAEXAMPLEKEY john@laptop
```

## Validation

The following rules are enforced by the admission webhook on create and update:

- `spec.connectionRef` must not be empty
- `spec.username` must not be empty
- `spec.label` must not be empty
- `spec.opensshPublicKey` must not be empty and must start with `ssh-` (e.g. `ssh-rsa`, `ssh-ed25519`)

## Notes

- The user specified by `username` must already exist in Warpgate.
- The `opensshPublicKey` value should be the full public key string as output by `ssh-keygen` (algorithm, base64 key, comment).
- Deleting the CR removes the public key credential from the Warpgate user via the finalizer.
