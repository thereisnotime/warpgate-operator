# WarpgateRole

A `WarpgateRole` represents a role in Warpgate. Roles are used to group access permissions and are assigned to users via `WarpgateUserRole` and to targets via `WarpgateTargetRole`.

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `connectionRef` | `string` | Yes | - | Name of the `WarpgateConnection` CR in the same namespace |
| `name` | `string` | Yes | - | Role name in Warpgate |
| `description` | `string` | No | `""` | Human-readable description |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `externalID` | `string` | Warpgate-assigned UUID for this role |
| `conditions` | `[]Condition` | Standard Kubernetes conditions |

## Print Columns

| Column | Source |
|--------|--------|
| Name | `.spec.name` |
| ExternalID | `.status.externalID` |
| Ready | `.status.conditions[?(@.type=="Ready")].status` |

## Example

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateRole
metadata:
  name: developers
spec:
  connectionRef: my-warpgate
  name: developers
  description: Role for developer access
```

## Validation

The following rules are enforced by the admission webhook on create and update:

- `spec.connectionRef` must not be empty
- `spec.name` must not be empty

## Notes

- Deleting a `WarpgateRole` CR triggers the finalizer to remove the role from Warpgate.
- If the role is still bound to users or targets in Warpgate, the deletion behavior depends on the Warpgate API (the operator does not cascade-delete bindings automatically).
