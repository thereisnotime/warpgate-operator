# Access Bindings

## Overview

The `WarpgateUserRole` and `WarpgateTargetRole` CRDs manage the many-to-many bindings between users/targets and roles in Warpgate. These are join-table resources -- they don't create new entities but link existing ones. The controller resolves human-readable names (username, role name, target name) to Warpgate UUIDs at reconciliation time.

## Requirements

### REQ-BIND-001: UserRole CRD Definition
**Status:** ADDED

The operator provides a `WarpgateUserRole` custom resource with the following spec fields:
- `connectionRef` (required) -- name of the `WarpgateConnection` CR in the same namespace.
- `username` (required) -- the Warpgate username to bind.
- `roleName` (required) -- the Warpgate role name to bind.

Status fields:
- `userID` -- the resolved Warpgate user UUID.
- `roleID` -- the resolved Warpgate role UUID.
- `conditions` -- standard Kubernetes conditions list.

Print columns: `Username`, `RoleName`, `Ready`.

**Scenarios:**
- **Given** a valid `WarpgateUserRole` CR where both the user and role exist in Warpgate **When** the controller reconciles **Then** it resolves the username and role name to UUIDs, creates the binding (idempotent), stores the IDs in status, and sets `Ready=True`.

### REQ-BIND-002: TargetRole CRD Definition
**Status:** ADDED

The operator provides a `WarpgateTargetRole` custom resource with the following spec fields:
- `connectionRef` (required) -- name of the `WarpgateConnection` CR in the same namespace.
- `targetName` (required) -- the Warpgate target name.
- `roleName` (required) -- the Warpgate role name.

Status fields:
- `targetID` -- the resolved Warpgate target UUID.
- `roleID` -- the resolved Warpgate role UUID.
- `conditions` -- standard Kubernetes conditions list.

Print columns: `TargetName`, `RoleName`, `Ready`.

**Scenarios:**
- **Given** a valid `WarpgateTargetRole` CR where both the target and role exist in Warpgate **When** the controller reconciles **Then** it resolves the target name and role name to UUIDs, creates the binding (idempotent), and sets `Ready=True`.

### REQ-BIND-003: Name-to-ID Resolution
**Status:** ADDED

Binding CRDs reference users, targets, and roles by name rather than UUID. The controller resolves these names to Warpgate UUIDs on each reconciliation. If a referenced entity doesn't exist yet, the controller retries with a short backoff.

**Scenarios:**
- **Given** a `WarpgateUserRole` referencing a username that doesn't exist in Warpgate **When** the controller reconciles **Then** it sets `Ready=False` with reason `UserNotFound` and requeues after 30 seconds.
- **Given** a `WarpgateTargetRole` referencing a role name that doesn't exist **When** the controller reconciles **Then** it sets `Ready=False` with reason `RoleNotFound` and requeues after 30 seconds.

### REQ-BIND-004: Idempotent Binding Creation
**Status:** ADDED

The binding creation API calls are idempotent -- calling them when the binding already exists does not produce an error. This means every reconciliation loop re-asserts the binding.

**Scenarios:**
- **Given** a `WarpgateUserRole` that was already bound **When** the controller reconciles again **Then** it calls the create-binding API again without error and remains `Ready=True`.

### REQ-BIND-005: Finalizer-Based Cleanup
**Status:** ADDED

On deletion, the controller removes the binding from Warpgate using the resolved IDs stored in status.

**Scenarios:**
- **Given** a `WarpgateUserRole` CR is deleted **When** the finalizer runs **Then** it calls the Warpgate API to remove the user-role binding and removes the finalizer.
- **Given** a `WarpgateTargetRole` CR is deleted but the binding was already removed **When** the finalizer runs **Then** it ignores the 404 and removes the finalizer.
