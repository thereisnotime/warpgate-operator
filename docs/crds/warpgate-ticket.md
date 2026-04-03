# WarpgateTicket

A `WarpgateTicket` creates a one-time (or limited-use) access ticket in Warpgate. The operator automatically creates a Kubernetes Secret
containing the ticket secret value so other workloads can consume it.

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `connectionRef` | `string` | Yes | - | Name of the `WarpgateConnection` CR in the same namespace |
| `username` | `string` | No | - | Warpgate username the ticket is for |
| `targetName` | `string` | No | - | Warpgate target the ticket grants access to |
| `expiry` | `string` | No | - | Expiration time in RFC 3339 format (e.g. `2026-12-31T23:59:59Z`) |
| `numberOfUses` | `*int` | No | - | Maximum number of times the ticket can be used |
| `description` | `string` | No | `""` | Human-readable description |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `ticketID` | `string` | Warpgate-assigned ticket UUID |
| `secretRef` | `string` | Name of the auto-created Secret containing the ticket secret |
| `conditions` | `[]Condition` | Standard Kubernetes conditions |

## Print Columns

| Column | Source |
|--------|--------|
| Username | `.spec.username` |
| Target | `.spec.targetName` |
| TicketID | `.status.ticketID` |
| Ready | `.status.conditions[?(@.type=="Ready")].status` |

## Auto-Created Secret

When the operator successfully creates the ticket in Warpgate, it stores the ticket secret value in a Kubernetes Secret named `<cr-name>-secret` in the same namespace. The secret key is `ticket`.

Retrieve the ticket secret:

```bash
kubectl get secret onetime-access-secret -o jsonpath='{.data.ticket}' | base64 -d
```

## Example

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateTicket
metadata:
  name: onetime-access
spec:
  connectionRef: my-warpgate
  username: john.doe
  targetName: production-ssh
  numberOfUses: 1
  description: One-time access for maintenance window
```

### Ticket with Expiry

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateTicket
metadata:
  name: timed-access
spec:
  connectionRef: my-warpgate
  username: john.doe
  targetName: production-ssh
  expiry: "2026-12-31T23:59:59Z"
  description: Access valid until end of year
```

## Notes

- Both `username` and `targetName` are optional -- you can create tickets scoped to just a user, just a target, or both.
- The auto-created Secret is owned by the `WarpgateTicket` CR, so deleting the CR also deletes the Secret.
- Deleting the CR removes the ticket from Warpgate via the finalizer.
