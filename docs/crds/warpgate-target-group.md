# WarpgateTargetGroup

A `WarpgateTargetGroup` organizes targets for visual grouping in the Warpgate UI.
Targets can be assigned to a group via `spec.groupRef` on a `WarpgateTarget`.

Mirrors the [Terraform `warpgate_target_group`](https://registry.terraform.io/providers/warp-tech/warpgate/latest/docs/resources/target_group) resource.

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `connectionRef` | `string` | Yes | - | Name of the `WarpgateConnection` CR in the same namespace |
| `name` | `string` | Yes | - | Target group name in Warpgate |
| `description` | `string` | No | `""` | Human-readable description |
| `color` | `string` | No | `""` | UI color: `Primary`, `Secondary`, `Success`, `Danger`, `Warning`, `Info`, `Light`, or `Dark` |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `externalID` | `string` | Warpgate-assigned UUID for this target group |
| `conditions` | `[]Condition` | Standard Kubernetes conditions |

## Print Columns

| Column | Source |
|--------|--------|
| Name | `.spec.name` |
| Color | `.spec.color` |
| ExternalID | `.status.externalID` |
| Ready | `.status.conditions[?(@.type=="Ready")].status` |

## Example

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateTargetGroup
metadata:
  name: production
spec:
  connectionRef: my-warpgate
  name: production
  description: Production environment servers
  color: Danger
---
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateTarget
metadata:
  name: prod-ssh
spec:
  connectionRef: my-warpgate
  name: prod-ssh
  groupRef: production
  ssh:
    host: 10.0.0.10
    port: 22
    username: admin
    authKind: PublicKey
```

## Validation

The following rules are enforced by the admission webhook on create and update:

- `spec.connectionRef` must not be empty
- `spec.name` must not be empty
- `spec.color`, when set, must be one of the allowed enum values

## Notes

- Deleting a `WarpgateTargetGroup` CR triggers the finalizer to remove the group from Warpgate.
- Assign targets to the group with `WarpgateTarget.spec.groupRef` (Kubernetes CR name in the same namespace). The group must be synced (`status.externalID` set) before the target can reconcile.
