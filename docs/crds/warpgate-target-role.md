# WarpgateTargetRole

A `WarpgateTargetRole` binds a Warpgate target to a Warpgate role. Users assigned to the same role (via `WarpgateUserRole`) will be able to access the target through the Warpgate bastion.

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `connectionRef` | `string` | Yes | - | Name of the `WarpgateConnection` CR in the same namespace |
| `targetName` | `string` | Yes | - | Warpgate target name |
| `roleName` | `string` | Yes | - | Warpgate role name |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `targetID` | `string` | Resolved Warpgate target UUID |
| `roleID` | `string` | Resolved Warpgate role UUID |
| `conditions` | `[]Condition` | Standard Kubernetes conditions |

## Print Columns

| Column | Source |
|--------|--------|
| TargetName | `.spec.targetName` |
| RoleName | `.spec.roleName` |
| Ready | `.status.conditions[?(@.type=="Ready")].status` |

## Example

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateTargetRole
metadata:
  name: production-ssh-developers
spec:
  connectionRef: my-warpgate
  targetName: production-ssh
  roleName: developers
```

## Validation

The following rules are enforced by the admission webhook on create and update:

- `spec.connectionRef` must not be empty
- `spec.targetName` must not be empty
- `spec.roleName` must not be empty

## Notes

- The target and role referenced by `targetName` and `roleName` must already exist in Warpgate.
- The operator resolves names to UUIDs and stores them in `status.targetID` and `status.roleID`.
- Deleting the CR removes the target-role binding from Warpgate via the finalizer.
