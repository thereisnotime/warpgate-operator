# WarpgateUser

A `WarpgateUser` represents a user account in Warpgate. It supports automatic password generation and per-protocol credential policies.

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `connectionRef` | `string` | Yes | - | Name of the `WarpgateConnection` CR in the same namespace |
| `username` | `string` | Yes | - | Warpgate username |
| `description` | `string` | No | `""` | Human-readable description |
| `generatePassword` | `*bool` | No | `true` | Auto-generate a random password and store it in a Kubernetes Secret |
| `passwordLength` | `*int` | No | `32` | Length of the auto-generated password (min 16, max 128) |
| `credentialPolicy` | `object` | No | - | Allowed credential types per protocol (see below) |

### Credential Policy

The `credentialPolicy` field controls which authentication methods are allowed for each protocol:

| Field | Type | Description |
|-------|------|-------------|
| `credentialPolicy.http` | `[]string` | Allowed credential types for HTTP (e.g. `Password`) |
| `credentialPolicy.ssh` | `[]string` | Allowed credential types for SSH (e.g. `PublicKey`, `Password`) |
| `credentialPolicy.mysql` | `[]string` | Allowed credential types for MySQL |
| `credentialPolicy.postgres` | `[]string` | Allowed credential types for PostgreSQL |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `externalID` | `string` | Warpgate-assigned UUID for this user |
| `passwordCredentialID` | `string` | UUID of the auto-generated password credential in Warpgate |
| `passwordSecretRef` | `string` | Name of the auto-created Secret containing the generated password |
| `conditions` | `[]Condition` | Standard Kubernetes conditions |

## Print Columns

| Column | Source |
|--------|--------|
| Username | `.spec.username` |
| ExternalID | `.status.externalID` |
| Ready | `.status.conditions[?(@.type=="Ready")].status` |

## Auto-Generated Password

When `generatePassword` is `true` (the default), the operator:

1. Generates a cryptographically random password of `passwordLength` characters.
2. Creates the password as a credential in Warpgate for the user.
3. Stores the password in a Kubernetes Secret named `<cr-name>-password` in the same namespace.

The Secret name is recorded in `status.passwordSecretRef` and the Warpgate credential UUID in `status.passwordCredentialID`.

Set `generatePassword: false` to skip this behavior entirely. You can then manage credentials separately using `WarpgatePasswordCredential` or `WarpgatePublicKeyCredential` CRs.

## Example

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateUser
metadata:
  name: john-doe
spec:
  connectionRef: my-warpgate
  username: john.doe
  description: Developer user
  generatePassword: true
  passwordLength: 48
  credentialPolicy:
    ssh:
      - PublicKey
    http:
      - Password
```

After reconciliation, retrieve the auto-generated password:

```bash
kubectl get secret john-doe-password -o jsonpath='{.data.password}' | base64 -d
```

## Validation

The following rules are enforced by the admission webhook on create and update:

- `spec.connectionRef` must not be empty
- `spec.username` must not be empty
- `spec.passwordLength` (when set) must be between 16 and 128

## Defaults

The following defaults are applied on create and update:

- `spec.generatePassword` defaults to `true` if not set
- `spec.passwordLength` defaults to `32` if not set

## Example Without Password Generation

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateUser
metadata:
  name: service-account
spec:
  connectionRef: my-warpgate
  username: svc-deploy
  generatePassword: false
  credentialPolicy:
    ssh:
      - PublicKey
```
