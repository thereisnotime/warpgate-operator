# Target Management

## Overview

The `WarpgateTarget` CRD manages targets (SSH, HTTP, MySQL, PostgreSQL, Kubernetes, RDP, VNC) in a Warpgate instance. Each CR defines exactly one target type via mutually exclusive spec sections. Passwords for target connections are read from Kubernetes Secrets, never stored inline.

## Requirements

### REQ-TARGET-001: Target CRD Definition
**Status:** ADDED

The operator provides a `WarpgateTarget` custom resource with the following spec fields:
- `connectionRef` (required) -- name of the `WarpgateConnection` CR in the same namespace.
- `name` (required) -- the target name in Warpgate.
- `description` (optional) -- human-readable description.
- `ssh` (optional) -- SSH target configuration.
- `http` (optional) -- HTTP target configuration.
- `mysql` (optional) -- MySQL target configuration.
- `postgresql` (optional) -- PostgreSQL target configuration.

Exactly one of `ssh`, `http`, `mysql`, or `postgresql` must be set.

Status fields:
- `externalID` -- the Warpgate-assigned target UUID.
- `conditions` -- standard Kubernetes conditions list.

Print columns: `Name`, `Type` (derived from Ready condition reason), `Ready`.

**Scenarios:**
- **Given** a `WarpgateTarget` with the `ssh` section populated **When** the controller reconciles **Then** it creates an SSH target in Warpgate.
- **Given** a `WarpgateTarget` with none of the target type sections set **When** the controller reconciles **Then** it sets `Ready=False` with reason `BuildError`.

### REQ-TARGET-002: SSH Target
**Status:** ADDED

SSH targets support `host`, `port`, `username`, `authKind` (enum: `Password` or `PublicKey`), an optional `passwordSecretRef` for password auth, and `allowInsecureAlgos`.

**Scenarios:**
- **Given** an SSH target with `authKind: Password` and a valid `passwordSecretRef` **When** the controller reconciles **Then** it reads the password from the referenced Secret and includes it in the Warpgate API request.
- **Given** an SSH target with `authKind: PublicKey` **When** the controller reconciles **Then** no password is read; the target is configured for key-based auth.

### REQ-TARGET-003: HTTP Target
**Status:** ADDED

HTTP targets support `url`, optional `tls` configuration (mode: `Disabled`/`Preferred`/`Required`, verify flag), optional `headers` map, and optional `externalHost` override.

**Scenarios:**
- **Given** an HTTP target with TLS mode `Required` and verify `true` **When** the controller reconciles **Then** the Warpgate API request includes the TLS configuration.
- **Given** a target whose `tls` block omits `verify` **When** it is admitted and reconciled **Then** `verify` defaults to `true` (CRD default and webhook) and the Warpgate API request has `verify: true`. Verification is only disabled when `verify: false` is set explicitly.

### REQ-TARGET-004: MySQL Target
**Status:** ADDED

MySQL targets support `host`, `port`, `username`, optional `passwordSecretRef`, and optional `tls` configuration.

**Scenarios:**
- **Given** a MySQL target with a `passwordSecretRef` **When** the controller reconciles **Then** it reads the password from the Secret (defaulting to key `"password"`) and includes it in the API request.

### REQ-TARGET-005: PostgreSQL Target
**Status:** ADDED

PostgreSQL targets support `host`, `port`, `username`, optional `passwordSecretRef`, optional `protocolVersion`, and optional `tls` configuration.

**Scenarios:**
- **Given** a PostgreSQL target with TLS disabled and no password **When** the controller reconciles **Then** it creates the target without password or TLS settings.
- **Given** a PostgreSQL target with `protocolVersion` set **When** the controller reconciles **Then** it passes the value to Warpgate as `protocol_version`.

### REQ-TARGET-006: Drift Detection and Correction
**Status:** ADDED

On each reconciliation (every 5 minutes), the controller pushes the full target spec to Warpgate. If the target was deleted externally, the controller detects the 404, clears `externalID`, and requeues to recreate.

**Scenarios:**
- **Given** a synced `WarpgateTarget` whose Warpgate-side target was deleted **When** the controller tries to update **Then** it clears `status.externalID`, sets `Ready=False` with reason `NotFound`, and requeues immediately.

### REQ-TARGET-007: Finalizer-Based Cleanup
**Status:** ADDED

On deletion, the controller deletes the target in Warpgate before removing the finalizer.

**Scenarios:**
- **Given** a `WarpgateTarget` CR is deleted **When** the finalizer runs **Then** it calls the Warpgate delete target API and removes the finalizer.
- **Given** a `WarpgateTarget` CR is deleted but the target is already gone from Warpgate **When** the finalizer runs **Then** it ignores the 404 and removes the finalizer.

### REQ-TARGET-008: Secret-Based Password Retrieval
**Status:** ADDED

Target passwords (SSH, MySQL, PostgreSQL) are always read from Kubernetes Secrets via `SecretKeyRef`. The key defaults to `"password"` when not specified.

**Scenarios:**
- **Given** a target with `passwordSecretRef` pointing to Secret `db-creds` with no key specified **When** the controller reads the password **Then** it uses `secret.Data["password"]`.
- **Given** a target with `passwordSecretRef` pointing to a non-existent Secret **When** the controller reconciles **Then** it sets `Ready=False` with reason `BuildError`.

### REQ-TARGET-009: Kubernetes Target
**Status:** ADDED

Kubernetes targets support `clusterURL`, `authKind` (enum: `Token`, `Certificate`, `IamRole`), optional `tls`, and per auth kind:
- `Token`: exactly one of `tokenSecretRef` (default key `token`) and `serviceAccountToken` (REQ-TARGET-010).
- `Certificate`: both `certificateSecretRef` (default key `certificate`) and `privateKeySecretRef` (default key `privateKey`).
- `IamRole`: no credentials; `serviceAccountToken` is rejected.

**Scenarios:**
- **Given** a Kubernetes target with `authKind: Token` and a `tokenSecretRef` **When** the controller reconciles **Then** it reads the token from the Secret and sends it in the target options.
- **Given** a Kubernetes target with `authKind: Token` and neither token source **When** it is submitted **Then** the webhook rejects it; **and When** the webhook is bypassed **Then** the controller sets `Ready=False` with reason `BuildError` and does not crash.
- **Given** a Kubernetes target with `authKind: Certificate` missing either Secret reference **When** it is submitted **Then** the webhook rejects it.

### REQ-TARGET-010: Short-Lived ServiceAccount Tokens
**Status:** ADDED

`spec.kubernetes.serviceAccountToken` (`name`, `audiences` 1-8, `expirationSeconds` 7200-172800, default 7200) makes the operator request the bearer token from the TokenRequest API for a ServiceAccount in the target's namespace.

- The ServiceAccount must carry `warpgate.warp.tech/token-audiences` (comma-separated) listing every requested audience, and `warpgate.warp.tech/token-cluster-urls` listing the target's `clusterURL` exactly. Otherwise no token is requested.
- `audiences` is required: a TokenRequest is never sent without audiences.
- The token is kept in memory per target UID and is never written to a Secret, the status, a condition, a log or an event. Errors from the Warpgate API have it redacted.
- The token is reused until half of the granted lifetime has passed. A new one is requested at that point, when the ServiceAccount UID, `clusterURL`, the audiences or the lifetime change, and after a restart.
- `status.serviceAccountToken` reports `expirationTimestamp` and `renewAfter`. The target is requeued no later than `renewAfter`, and within one minute after a token failure or a failed Warpgate update of a target holding a token. A reconcile interval of 0 still means no periodic resync for other targets.
- The generated ClusterRole has no `serviceaccounts/token` permission. The Helm value `serviceAccountTokens.allowed` grants `create` on `serviceaccounts/token` and `get` on `serviceaccounts` per namespace, limited by `resourceNames`.
- ServiceAccounts are read with the uncached API reader.

Condition reasons: `ServiceAccountNotFound`, `ServiceAccountNotOptedIn`, `TokenAudienceNotAllowed`, `TokenClusterURLNotAllowed`, `TokenRequestForbidden`, `TokenRequestError`.

Updates that leave the spec unchanged, and updates of objects being deleted, are not validated by the webhook.

**Scenarios:**
- **Given** an opted-in ServiceAccount and the RBAC **When** the controller reconciles **Then** it requests a token with exactly the requested audiences and lifetime, sends it to Warpgate, sets `Ready=True` and `status.serviceAccountToken`, and requeues at `renewAfter`.
- **Given** a cached token **When** the target is reconciled before `renewAfter` **Then** the same token is sent; **and When** reconciled after `renewAfter` **Then** a new token is requested.
- **Given** a change to the audiences or lifetime, or a recreated ServiceAccount **When** the controller reconciles **Then** it requests a new token at once.
- **Given** a ServiceAccount without either annotation, or one not allowing every requested audience or the `clusterURL` **When** the controller reconciles **Then** no token is requested, Warpgate is not called, and `Ready=False` with `ServiceAccountNotOptedIn`, `TokenAudienceNotAllowed` or `TokenClusterURLNotAllowed`.
- **Given** a target whose `clusterURL` is changed to one the ServiceAccount doesn't allow **When** the controller reconciles **Then** Warpgate receives no token for it.
- **Given** the operator lacks `create` on `serviceaccounts/token` for the ServiceAccount **When** the controller reconciles **Then** `Ready=False` with `TokenRequestForbidden` and a message naming the Helm value.
- **Given** Warpgate rejects the target with a body echoing the request **When** the controller sets the condition **Then** the token appears as `[REDACTED]`, and appears nowhere in the stored object.
- **Given** a target that stops using `serviceAccountToken`, or is deleted **When** it is reconciled **Then** the cached token and `status.serviceAccountToken` are cleared.
