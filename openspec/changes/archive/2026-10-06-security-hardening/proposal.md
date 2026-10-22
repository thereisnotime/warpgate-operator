# Security Hardening

## Summary

Fix five security issues in how the operator handles database credentials, TLS verification, and password rotation.

## Motivation

An audit of the controllers turned up these problems on `main`:

1. `WarpgateInstance.spec.databaseURL` was interpolated into the init container's `sh -c` script, so a crafted value ran arbitrary commands.
2. The same URL, usually with a database password, was written in plain text into the generated `warpgate.yaml` ConfigMap.
3. A `WarpgateTarget` `tls` block that set only `mode` was sent to Warpgate with `verify: false`, silently disabling certificate checks.
4. Rotating the Secret behind a `WarpgatePasswordCredential` never reached Warpgate, so the old password stayed valid.
5. The `WarpgateConnection` auto-created for a `WarpgateInstance` always set `insecureSkipVerify: true`, even when the instance served a certificate the operator could verify.

## What Changes

- Init script reads the database URL from `DATABASE_URL` instead of embedding it.
- New `WarpgateInstance.spec.databaseURLSecretRef`; `databaseURL` is deprecated. The URL is no longer written to the ConfigMap; the init container appends it to `/data/warpgate.yaml`. `externalHost` goes through a YAML marshaller.
- `TLSConfigSpec.verify` becomes `*bool` defaulting to `true`.
- The password credential controller watches Secrets and replaces the Warpgate credential when the Secret changes, tracking `status.appliedSecretVersion`.
- New `WarpgateConnection.spec.caSecretRef` and `WarpgateInstance.spec.tls.verifyConnection`; the auto-created connection verifies against `tls.secretName` when set.

## Breaking Changes

- Target `tls` blocks without `verify` now verify certificates.
- Instances with `tls.secretName` get a verifying connection; the certificate must cover `<name>-http.<namespace>.svc` (or set `tls.verifyConnection: false`).
