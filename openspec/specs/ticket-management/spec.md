# Ticket Management

## Overview

The `WarpgateTicket` CRD manages access tickets in Warpgate. Tickets are create-only (immutable after creation) -- the controller creates the ticket once, stores the ticket secret in an auto-generated Kubernetes Secret with an owner reference, and never modifies it afterward.

## Requirements

### REQ-TICKET-001: Ticket CRD Definition
**Status:** ADDED

The operator provides a `WarpgateTicket` custom resource with the following spec fields:
- `connectionRef` (required) -- name of the `WarpgateConnection` CR in the same namespace.
- `username` (optional) -- the Warpgate username the ticket is for.
- `targetName` (optional) -- the Warpgate target the ticket grants access to.
- `expiry` (optional) -- ticket expiration time in RFC3339 format.
- `numberOfUses` (optional) -- maximum number of times the ticket can be used.
- `description` (optional) -- human-readable description.

Status fields:
- `ticketID` -- the Warpgate-assigned ticket UUID.
- `secretRef` -- the name of the auto-created Secret containing the ticket secret value.
- `conditions` -- standard Kubernetes conditions list.

Print columns: `Username`, `Target`, `TicketID`, `Ready`.

**Scenarios:**
- **Given** a valid `WarpgateTicket` CR **When** the controller reconciles for the first time **Then** it creates the ticket via the Warpgate API, stores the ticket ID in `status.ticketID`, creates a Secret named `<cr-name>-secret` containing the ticket secret value, and sets `Ready=True`.

### REQ-TICKET-002: Create-Only Semantics
**Status:** ADDED

Tickets are immutable. Once a ticket is created (status.ticketID is set), the controller does not attempt to update it on subsequent reconciliations. It simply re-asserts the Ready condition.

**Scenarios:**
- **Given** a `WarpgateTicket` with `status.ticketID` already set **When** the controller reconciles **Then** it skips the create step and sets `Ready=True`.
- **Given** a `WarpgateTicket` spec is modified after creation **When** the controller reconciles **Then** no changes are pushed to Warpgate since the ticket is immutable.

### REQ-TICKET-003: Auto-Created Secret
**Status:** ADDED

When a ticket is created, the controller creates a Kubernetes Secret named `<cr-name>-secret` in the same namespace. The Secret contains the key `"secret"` with the ticket secret value returned by the Warpgate API. The Secret has an owner reference pointing to the `WarpgateTicket` CR.

**Scenarios:**
- **Given** a new `WarpgateTicket` CR **When** the ticket is successfully created in Warpgate **Then** a Secret `<cr-name>-secret` is created with key `"secret"` and an owner reference to the ticket CR.
- **Given** the ticket creation succeeds but the Secret creation fails **When** the controller reconciles **Then** it sets `Ready=False` with reason `SecretCreateFailed`.

### REQ-TICKET-004: Finalizer-Based Cleanup
**Status:** ADDED

On deletion, the controller deletes the ticket in Warpgate and the auto-created Secret before removing the finalizer.

**Scenarios:**
- **Given** a `WarpgateTicket` CR is deleted **When** the finalizer runs **Then** it deletes the ticket from Warpgate, deletes the `<cr-name>-secret` Secret, and removes the finalizer.
- **Given** a `WarpgateTicket` CR is deleted but the ticket was already gone from Warpgate **When** the finalizer runs **Then** it ignores the 404, still cleans up the Secret, and removes the finalizer.
