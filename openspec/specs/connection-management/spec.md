# Connection Management

## Overview

The `WarpgateConnection` CRD represents a connection to a Warpgate bastion host instance. It stores the host URL and a reference to a Kubernetes Secret containing credentials (username/password). All other CRDs reference a `WarpgateConnection` by name (in the same namespace) to know which Warpgate instance to talk to, enabling multi-instance support within a single cluster.

## Requirements

### REQ-CONN-001: Connection CRD Definition
**Status:** ADDED

The operator provides a `WarpgateConnection` custom resource with the following spec fields:
- `host` (required) -- URL of the Warpgate instance (e.g. `https://warpgate.example.com`).
- `tokenSecretRef` (required) -- a `SecretKeyRef` pointing to a Kubernetes Secret containing credentials (`username` and `password` keys).
- `insecureSkipVerify` (optional, default `false`) -- disables TLS certificate verification.

Status fields:
- `conditions` -- standard Kubernetes conditions list (map by type).

**Scenarios:**
- **Given** a valid `WarpgateConnection` CR is created **When** the controller reconciles it **Then** it reads the credentials from the referenced Secret, validates the connection by listing roles, and sets the `Ready` condition to `True` with reason `Connected`.
- **Given** a `WarpgateConnection` CR references a Secret that does not exist **When** the controller reconciles **Then** the `Ready` condition is set to `False` with reason `ConnectionFailed`.
- **Given** a `WarpgateConnection` CR points to an unreachable Warpgate host **When** the controller reconciles **Then** the `Ready` condition is set to `False` with reason `ConnectionFailed` and it requeues after 5 minutes.

### REQ-CONN-002: Credential Secret Format
**Status:** MODIFIED

The referenced Secret must contain `username` and `password` keys for session-based authentication.

**Scenarios:**
- **Given** a `WarpgateConnection` referencing a Secret with `username` and `password` keys **When** the controller builds the API client **Then** it reads both values and authenticates via session-based auth.
- **Given** a `WarpgateConnection` referencing a Secret missing the `username` or `password` key **When** the controller reconciles **Then** the `Ready` condition is set to `False` with reason `ConnectionFailed`.

### REQ-CONN-003: Periodic Re-validation
**Status:** ADDED

The controller periodically re-validates the connection health, even when no spec changes occur, so that transient outages are detected and reflected in the status.

**Scenarios:**
- **Given** a healthy `WarpgateConnection` with `Ready=True` **When** 5 minutes elapse **Then** the controller re-validates the connection by calling the Warpgate API.
- **Given** a previously healthy connection that has gone down **When** the periodic re-validation fires **Then** the `Ready` condition transitions to `False`.

### REQ-CONN-004: Finalizer Lifecycle
**Status:** ADDED

The controller adds the `warpgate.warp.tech/finalizer` to every `WarpgateConnection`. On deletion, the finalizer is removed (no external cleanup needed since the connection itself is not a Warpgate-side resource).

**Scenarios:**
- **Given** a newly created `WarpgateConnection` **When** the controller reconciles **Then** the `warpgate.warp.tech/finalizer` is added.
- **Given** a `WarpgateConnection` marked for deletion **When** the controller reconciles **Then** it removes the finalizer and allows Kubernetes to garbage-collect the CR.

### REQ-CONN-005: Namespace-Scoped Secret Lookup
**Status:** ADDED

The controller reads the credentials Secret from the same namespace as the `WarpgateConnection` CR. Cross-namespace Secret references are not supported.

**Scenarios:**
- **Given** a `WarpgateConnection` in namespace `team-a` referencing Secret `wg-token` **When** the controller reads the Secret **Then** it looks up `team-a/wg-token`, not a cluster-wide search.
