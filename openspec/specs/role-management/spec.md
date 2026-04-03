# Role Management

## Overview

The `WarpgateRole` CRD manages roles in a Warpgate instance. Each CR maps to a single Warpgate role, supporting full CRUD with drift reconciliation -- if the role is modified or deleted out-of-band in Warpgate, the controller brings it back in line with the CRD spec.

## Requirements

### REQ-ROLE-001: Role CRD Definition
**Status:** ADDED

The operator provides a `WarpgateRole` custom resource with the following spec fields:
- `connectionRef` (required) -- name of the `WarpgateConnection` CR in the same namespace.
- `name` (required) -- the role name in Warpgate.
- `description` (optional) -- human-readable description.

Status fields:
- `externalID` -- the Warpgate-assigned UUID for this role.
- `conditions` -- standard Kubernetes conditions list.

Print columns: `Name`, `ExternalID`, `Ready`.

**Scenarios:**
- **Given** a valid `WarpgateRole` CR **When** the controller reconciles for the first time **Then** it creates the role via the Warpgate API and stores the returned UUID in `status.externalID`.
- **Given** a `WarpgateRole` whose `status.externalID` is already set **When** the controller reconciles **Then** it updates the existing role in Warpgate with the current spec values.

### REQ-ROLE-002: Drift Detection and Correction
**Status:** ADDED

On each reconciliation (every 5 minutes), the controller pushes the CRD spec to Warpgate, overwriting any out-of-band changes. If the role was deleted externally, the controller detects the 404, clears `externalID`, and recreates it on the next loop.

**Scenarios:**
- **Given** a synced `WarpgateRole` whose corresponding Warpgate role was renamed via the UI **When** the controller reconciles **Then** it overwrites the Warpgate role name back to the CRD spec value.
- **Given** a synced `WarpgateRole` whose Warpgate-side role was deleted **When** the controller tries to update **Then** it receives a 404, clears `status.externalID`, sets `Ready=False` with reason `NotFound`, and requeues immediately to recreate.

### REQ-ROLE-003: Finalizer-Based Cleanup
**Status:** ADDED

The controller attaches the `warpgate.warp.tech/finalizer`. When the CR is deleted, it deletes the role in Warpgate before removing the finalizer.

**Scenarios:**
- **Given** a `WarpgateRole` CR is deleted **When** the finalizer runs **Then** it calls `DELETE /roles/{id}` in Warpgate and removes the finalizer.
- **Given** a `WarpgateRole` CR is deleted but the role was already removed from Warpgate **When** the finalizer runs **Then** it ignores the 404 and removes the finalizer anyway.

### REQ-ROLE-004: Connection Reference Resolution
**Status:** ADDED

The controller resolves the `connectionRef` to a `WarpgateConnection` CR in the same namespace, reads the token Secret, and builds an API client. If the connection is unavailable, the role is marked as not ready.

**Scenarios:**
- **Given** a `WarpgateRole` referencing a non-existent `WarpgateConnection` **When** the controller reconciles **Then** it sets `Ready=False` with reason `ClientError`.
