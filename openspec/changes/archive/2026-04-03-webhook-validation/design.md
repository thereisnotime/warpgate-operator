# Design

## Validation Rules Per CRD

Each CRD gets a `ValidateCreate` and `ValidateUpdate` webhook. `ValidateDelete` is left as a no-op since finalizers already handle cleanup.

### WarpgateConnection
- `spec.host` must be a valid URL (https scheme, no trailing slash)
- `spec.authSecretRef` must reference a non-empty secret name (the Secret must contain either a `token` key for bearer auth, or `username` and `password` keys for session auth)
- Immutable fields: `spec.host` cannot change after creation (forces delete + recreate to avoid orphaned resources)

### WarpgateTarget
- Exactly one target type must be set (`ssh`, `http`, `mysql`, `postgresql`) — reject if zero or more than one
- `spec.ssh.host` required when SSH type is set
- `spec.http.url` must be a valid URL when HTTP type is set
- `spec.tlsMode` must be one of: `Disabled`, `Preferred`, `Required` (if specified)
- Port values must be in range 1–65535

### WarpgateRole
- `spec.name` must be non-empty and match `^[a-zA-Z0-9_-]+$`

### WarpgateUser
- `spec.username` must be non-empty
- `spec.credentialPolicy` must be one of: `Password`, `PublicKey`, `Totp`, `WebUserApproval` (if specified)

### WarpgateUserRole / WarpgateTargetRole
- Both `spec.*Ref` fields must be non-empty
- Cross-namespace references are rejected (binding must be in same namespace as referenced resources)

### WarpgatePasswordCredential
- `spec.userRef` must be non-empty
- `spec.passwordSecretRef` must reference a valid secret name and key

### WarpgatePublicKeyCredential
- `spec.userRef` must be non-empty
- `spec.publicKey` must start with a valid SSH key prefix (`ssh-rsa`, `ssh-ed25519`, `ecdsa-sha2-*`, etc.)

### WarpgateTicket
- `spec.targetRef` and `spec.userRef` must be non-empty
- `spec.expiry` must be a valid RFC 3339 timestamp in the future (on create)

## Defaulting Logic

Defaulting webhooks set sensible values when fields are omitted:

| CRD | Field | Default |
|---|---|---|
| WarpgateTarget (SSH) | `spec.ssh.port` | `22` |
| WarpgateTarget (MySQL) | `spec.mysql.port` | `3306` |
| WarpgateTarget (PostgreSQL) | `spec.postgresql.port` | `5432` |
| WarpgateTarget | `spec.tlsMode` | `Preferred` |
| WarpgatePasswordCredential | `spec.passwordSecretRef.key` | `password` |

## cert-manager Integration

Webhook endpoints require TLS. Rather than self-signed certs or manual cert management, we use cert-manager:

- A `Certificate` resource requests a self-signed CA cert
- An `Issuer` (self-signed) is deployed alongside the operator
- The webhook server mounts the TLS secret automatically
- Helm chart includes cert-manager resources gated behind `webhooks.enabled` and `certManager.enabled` values

This is the standard Kubebuilder pattern and avoids reinventing certificate rotation.

## Kubebuilder Webhook Markers

Each type gets markers like:

```go
// +kubebuilder:webhook:path=/validate-warpgate-v1alpha1-target,mutating=false,failurePolicy=fail,groups=warpgate.contrib.fluxcd.io,resources=warpgatetargets,verbs=create;update,versions=v1alpha1,name=vwarpgatetarget.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/mutate-warpgate-v1alpha1-target,mutating=true,failurePolicy=fail,groups=warpgate.contrib.fluxcd.io,resources=warpgatetargets,verbs=create;update,versions=v1alpha1,name=mwarpgatetarget.kb.io,admissionReviewVersions=v1
```

`make manifests` generates the `ValidatingWebhookConfiguration` and `MutatingWebhookConfiguration` YAML from these markers.

## Testing Strategy

- Unit tests for each validation and defaulting function (table-driven, no webhook server needed)
- Integration tests using envtest with webhook server enabled
- E2E tests that apply invalid manifests and assert rejection messages
