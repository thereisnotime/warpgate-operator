# User Management

## Overview

The `WarpgateUser` CRD manages users in a Warpgate instance. Beyond basic CRUD and drift reconciliation, it supports per-protocol credential policies and automatic password generation -- creating a random password credential in Warpgate and storing it in a Kubernetes Secret.

## Requirements

### REQ-USER-001: User CRD Definition
**Status:** ADDED

The operator provides a `WarpgateUser` custom resource with the following spec fields:
- `connectionRef` (required) -- name of the `WarpgateConnection` CR in the same namespace.
- `username` (required) -- the Warpgate username.
- `description` (optional) -- human-readable description.
- `credentialPolicy` (optional) -- allowed credential types per protocol, with sub-fields: `http`, `ssh`, `mysql`, `postgres` (each a string array).
- `generatePassword` (optional, default `true`) -- when true, auto-generates a random password credential.
- `passwordLength` (optional, default `32`, min `16`, max `128`) -- length of the auto-generated password.

Status fields:
- `externalID` -- the Warpgate-assigned UUID for this user.
- `passwordCredentialID` -- UUID of the auto-generated password credential.
- `passwordSecretRef` -- name of the auto-created Secret containing the generated password.
- `conditions` -- standard Kubernetes conditions list.

Print columns: `Username`, `ExternalID`, `Ready`.

**Scenarios:**
- **Given** a valid `WarpgateUser` CR **When** the controller reconciles for the first time **Then** it creates the user via the Warpgate API and stores the UUID in `status.externalID`.
- **Given** a `WarpgateUser` whose `status.externalID` is set **When** the controller reconciles **Then** it updates the user in Warpgate (username, description, credential policy).

### REQ-USER-002: Automatic Password Generation
**Status:** ADDED

When `generatePassword` is true (the default) and no password credential has been created yet, the controller generates a cryptographically random password, creates a password credential in Warpgate, and stores the password in a Kubernetes Secret named `<cr-name>-password`.

**Scenarios:**
- **Given** a new `WarpgateUser` with `generatePassword: true` (or omitted) **When** the user is created in Warpgate **Then** the controller generates a random password of `passwordLength` bytes, creates a password credential via the API, and creates a Secret `<cr-name>-password` containing keys `password` and `username`.
- **Given** a `WarpgateUser` with `generatePassword: false` **When** the controller reconciles **Then** no password credential or Secret is created.
- **Given** a `WarpgateUser` that already has a `passwordCredentialID` in status **When** the controller reconciles **Then** it does not regenerate the password.

### REQ-USER-003: Password Secret Ownership
**Status:** ADDED

The auto-created password Secret has an owner reference pointing to the `WarpgateUser` CR, along with standard `app.kubernetes.io` labels for discoverability.

**Scenarios:**
- **Given** a `WarpgateUser` with auto-generated password **When** the password Secret is created **Then** it has `controllerReference` set to the `WarpgateUser` and labels `app.kubernetes.io/managed-by: warpgate-operator`, `app.kubernetes.io/name: warpgate-user-password`, `app.kubernetes.io/instance: <cr-name>`.

### REQ-USER-004: Drift Detection and Correction
**Status:** ADDED

On each reconciliation, the controller pushes the full user spec to Warpgate. If the user was deleted externally, the controller detects the 404, clears `externalID`, and requeues to recreate.

**Scenarios:**
- **Given** a synced `WarpgateUser` whose Warpgate-side user was deleted **When** the controller tries to update **Then** it clears `status.externalID`, sets `Ready=False` with reason `NotFound`, and requeues.

### REQ-USER-005: Finalizer-Based Cleanup
**Status:** ADDED

On deletion, the controller cleans up in order: the auto-generated password credential in Warpgate, the password Secret in Kubernetes, and finally the user in Warpgate.

**Scenarios:**
- **Given** a `WarpgateUser` CR is deleted **When** the finalizer runs **Then** it deletes the password credential from Warpgate, deletes the `<cr-name>-password` Secret, deletes the user from Warpgate, and removes the finalizer.
- **Given** a `WarpgateUser` CR is deleted and the Warpgate user was already removed **When** the finalizer runs **Then** it ignores 404 errors and still cleans up the Secret and finalizer.
