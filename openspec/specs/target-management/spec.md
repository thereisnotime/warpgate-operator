# Target Management

## Overview

The `WarpgateTarget` CRD manages targets (SSH, HTTP, MySQL, PostgreSQL) in a Warpgate instance. Each CR defines exactly one target type via mutually exclusive spec sections. Passwords for target connections are read from Kubernetes Secrets, never stored inline.

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

### REQ-TARGET-004: MySQL Target
**Status:** ADDED

MySQL targets support `host`, `port`, `username`, optional `passwordSecretRef`, and optional `tls` configuration.

**Scenarios:**
- **Given** a MySQL target with a `passwordSecretRef` **When** the controller reconciles **Then** it reads the password from the Secret (defaulting to key `"password"`) and includes it in the API request.

### REQ-TARGET-005: PostgreSQL Target
**Status:** ADDED

PostgreSQL targets support `host`, `port`, `username`, optional `passwordSecretRef`, and optional `tls` configuration.

**Scenarios:**
- **Given** a PostgreSQL target with TLS disabled and no password **When** the controller reconciles **Then** it creates the target without password or TLS settings.

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
