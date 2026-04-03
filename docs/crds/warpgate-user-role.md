# WarpgateUserRole

A `WarpgateUserRole` binds a Warpgate user to a Warpgate role. This is the mechanism for granting users access to targets that are associated with the same role (via `WarpgateTargetRole`).

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `connectionRef` | `string` | Yes | - | Name of the `WarpgateConnection` CR in the same namespace |
| `username` | `string` | Yes | - | Warpgate username to bind |
| `roleName` | `string` | Yes | - | Warpgate role name to bind |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `userID` | `string` | Resolved Warpgate user UUID |
| `roleID` | `string` | Resolved Warpgate role UUID |
| `conditions` | `[]Condition` | Standard Kubernetes conditions |

## Print Columns

| Column | Source |
|--------|--------|
| Username | `.spec.username` |
| RoleName | `.spec.roleName` |
| Ready | `.status.conditions[?(@.type=="Ready")].status` |

## Example

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateUserRole
metadata:
  name: john-doe-developers
spec:
  connectionRef: my-warpgate
  username: john.doe
  roleName: developers
```

## Notes

- The user and role referenced by `username` and `roleName` must already exist in Warpgate (either managed by corresponding CRs or created externally).
- The operator resolves names to UUIDs and stores them in `status.userID` and `status.roleID`.
- Deleting the CR removes the user-role binding from Warpgate via the finalizer.
