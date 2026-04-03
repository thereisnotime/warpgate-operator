# Credential Management

## Overview

The `WarpgatePasswordCredential` and `WarpgatePublicKeyCredential` CRDs manage user credentials in Warpgate. Password credentials read the password from a Kubernetes Secret; public key credentials take the SSH public key directly in the spec. Both resolve the target user by username at reconciliation time.

## Requirements

### REQ-CRED-001: Password Credential CRD Definition
**Status:** ADDED

The operator provides a `WarpgatePasswordCredential` custom resource with the following spec fields:
- `connectionRef` (required) -- name of the `WarpgateConnection` CR in the same namespace.
- `username` (required) -- the Warpgate username to attach the credential to.
- `passwordSecretRef` (required) -- a `SecretKeyRef` pointing to a Kubernetes Secret containing the password. Key defaults to `"password"`.

Status fields:
- `userID` -- the resolved Warpgate user UUID.
- `credentialID` -- the Warpgate-assigned credential UUID.
- `conditions` -- standard Kubernetes conditions list.

Print columns: `Username`, `CredentialID`, `Ready`.

**Scenarios:**
- **Given** a valid `WarpgatePasswordCredential` CR with a matching user and Secret **When** the controller reconciles for the first time **Then** it resolves the username to a UUID, reads the password from the Secret, creates the credential in Warpgate, and stores the credential ID in status.
- **Given** a `WarpgatePasswordCredential` referencing a non-existent user **When** the controller reconciles **Then** it sets `Ready=False` with reason `UserNotFound`.
- **Given** a `WarpgatePasswordCredential` referencing a Secret with a missing key **When** the controller reconciles **Then** it sets `Ready=False` with reason `SecretKeyMissing`.

### REQ-CRED-002: Public Key Credential CRD Definition
**Status:** ADDED

The operator provides a `WarpgatePublicKeyCredential` custom resource with the following spec fields:
- `connectionRef` (required) -- name of the `WarpgateConnection` CR in the same namespace.
- `username` (required) -- the Warpgate username to attach the credential to.
- `label` (required) -- a human-readable label for the key.
- `opensshPublicKey` (required) -- the SSH public key in OpenSSH format.

Status fields:
- `userID` -- the resolved Warpgate user UUID.
- `credentialID` -- the Warpgate-assigned credential UUID.
- `conditions` -- standard Kubernetes conditions list.

Print columns: `Username`, `Label`, `CredentialID`, `Ready`.

**Scenarios:**
- **Given** a valid `WarpgatePublicKeyCredential` CR **When** the controller reconciles for the first time **Then** it resolves the username, creates the public key credential in Warpgate, and stores the credential ID in status.
- **Given** a `WarpgatePublicKeyCredential` whose credential was deleted in Warpgate **When** the controller tries to update **Then** it detects the 404, clears `credentialID`, and requeues to recreate.

### REQ-CRED-003: Public Key Drift Reconciliation
**Status:** ADDED

Public key credentials support full update -- if the label or key changes in the CRD spec, the controller pushes the update to Warpgate. Password credentials are create-only (no update path).

**Scenarios:**
- **Given** a synced `WarpgatePublicKeyCredential` whose `label` is changed **When** the controller reconciles **Then** it updates the credential in Warpgate with the new label and key.
- **Given** a synced `WarpgatePasswordCredential` that already has a `credentialID` **When** the controller reconciles **Then** it does not attempt to update the credential (create-only behavior).

### REQ-CRED-004: Finalizer-Based Cleanup
**Status:** ADDED

Both credential CRDs use the `warpgate.warp.tech/finalizer`. On deletion, the controller deletes the credential in Warpgate using the stored user ID and credential ID.

**Scenarios:**
- **Given** a `WarpgatePasswordCredential` CR is deleted **When** the finalizer runs **Then** it calls the Warpgate API to delete the password credential and removes the finalizer.
- **Given** a `WarpgatePublicKeyCredential` CR is deleted but the credential was already removed from Warpgate **When** the finalizer runs **Then** it ignores the 404 and removes the finalizer.

### REQ-CRED-005: Username Resolution
**Status:** ADDED

Both credential controllers resolve the target user by username via the Warpgate API (`GetUserByUsername`) and store the resolved UUID in `status.userID`. This happens on every reconciliation, not just the first one.

**Scenarios:**
- **Given** a credential CR referencing username `"alice"` **When** the controller reconciles **Then** it calls the Warpgate API to resolve `"alice"` to a UUID and stores it in `status.userID`.
