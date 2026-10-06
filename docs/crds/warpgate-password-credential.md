# WarpgatePasswordCredential

A `WarpgatePasswordCredential` adds a password credential to an existing Warpgate user. The password value is read from a Kubernetes Secret.

This is useful when you want explicit control over password credentials. For automatic password generation, see the `generatePassword` feature on [WarpgateUser](warpgate-user.md) instead.

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `connectionRef` | `string` | Yes | - | Name of the `WarpgateConnection` CR in the same namespace |
| `username` | `string` | Yes | - | Warpgate username to add the credential to |
| `passwordSecretRef.name` | `string` | Yes | - | Name of the Kubernetes Secret containing the password |
| `passwordSecretRef.key` | `string` | No | `password` | Key within the Secret that holds the password value |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `userID` | `string` | Resolved Warpgate user UUID |
| `credentialID` | `string` | Warpgate-assigned credential UUID |
| `appliedSecretVersion` | `string` | `<secret>/<key>@<resourceVersion>` of the password Secret last pushed to Warpgate |
| `conditions` | `[]Condition` | Standard Kubernetes conditions |

## Print Columns

| Column | Source |
|--------|--------|
| Username | `.spec.username` |
| CredentialID | `.status.credentialID` |
| Ready | `.status.conditions[?(@.type=="Ready")].status` |

## Example

First, create the Secret containing the password:

```bash
kubectl create secret generic john-doe-password \
  --from-literal=password='s3cur3-p@ssw0rd'
```

Then create the credential:

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgatePasswordCredential
metadata:
  name: john-doe-password-cred
spec:
  connectionRef: my-warpgate
  username: john.doe
  passwordSecretRef:
    name: john-doe-password
    key: password
```

## Validation

The following rules are enforced by the admission webhook on create and update:

- `spec.connectionRef` must not be empty
- `spec.username` must not be empty
- `spec.passwordSecretRef.name` must not be empty

## Defaults

The following defaults are applied on create and update:

- `spec.passwordSecretRef.key` defaults to `"password"` if not set

## Notes

- The referenced Secret must exist in the same namespace as the CR.
- The user specified by `username` must already exist in Warpgate.
- Deleting the CR removes the password credential from the Warpgate user via the finalizer.
- The operator watches the referenced Secret. When it changes, the old Warpgate credential is deleted and a new one
  is created with the new password, so the previous password stops working right away. Warpgate has no in-place
  password update, which is why the credential ID changes on rotation. Credentials created by older operator versions
  are re-applied once after upgrading, since there is no recorded Secret version for them yet.
- If you just need a basic auto-generated password, use `WarpgateUser` with `generatePassword: true` instead of creating this resource manually.
