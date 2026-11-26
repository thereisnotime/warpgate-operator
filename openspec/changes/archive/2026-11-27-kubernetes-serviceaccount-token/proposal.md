# Kubernetes targets with short-lived ServiceAccount tokens

## Summary

Let a Kubernetes `WarpgateTarget` get its bearer token from the Kubernetes TokenRequest API instead of a Secret.
The operator requests a short-lived token for a ServiceAccount in the target's namespace, sends it to Warpgate,
and requests a new one before it expires. The token is never written to a Secret, the CR status, a log line or an
event.

## Motivation

Today `authKind: Token` reads a token from a Secret. In practice that is a legacy ServiceAccount token Secret:
a token that never expires, readable by anything that can read Secrets in that namespace, and, for a cluster other
than the one the operator runs in, one that has to be copied across by hand. Warpgate itself cannot mint or rotate
Kubernetes credentials (warp-tech/warpgate#2159).

With TokenRequest:

- the token lives only in the operator's memory and in Warpgate, and expires on its own (2h to 48h, default 2h);
- nothing has to be copied between clusters: a remote cluster can trust the operator cluster's ServiceAccount
  issuer (structured authentication, `AuthenticationConfiguration`) and map the token to its own RBAC;
- rotation is automatic.

## What Changes

- New optional `spec.kubernetes.serviceAccountToken` (`name`, `audiences`, `expirationSeconds`). With
  `authKind: Token`, exactly one of `tokenSecretRef` and `serviceAccountToken` must be set. It is rejected for the
  other auth kinds.
- The controller requests the token with TokenRequest, keeps it in memory, reuses it until half its lifetime has
  passed, and requeues the target in time to renew it.
- Two guards before any token is requested:
  - the ServiceAccount has to opt in with two annotations: `warpgate.warp.tech/token-audiences` (the audiences
    it may be issued for) and `warpgate.warp.tech/token-cluster-urls` (where its tokens may be sent);
  - the operator needs RBAC for `serviceaccounts/token`, which it does not get by default.
- New status block `status.serviceAccountToken` with `expirationTimestamp` and `renewAfter`, and new `Ready`
  condition reasons for every failure.
- Errors that pass through the Warpgate API client have the token redacted before they reach a condition or a log.
- Helm: new `serviceAccountTokens.allowed` value. It renders a Role and RoleBinding per namespace, limited by
  `resourceNames` to the listed ServiceAccounts. It is empty by default.
- Defensive fix: a Kubernetes target with a missing Secret reference now fails validation and sets `BuildError`
  instead of crashing the reconciler with a nil dereference.

## Breaking Changes

- None for existing targets that set `tokenSecretRef`.
- The webhook now rejects `authKind: Token` with neither `tokenSecretRef` nor `serviceAccountToken`, and
  `authKind: Certificate` without both certificate Secret references. Those specs could never work: the reconciler
  crashed on them.
