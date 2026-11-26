# Mount the cert-manager TLS Secret

## Summary

Serve the cert-manager certificate from a `WarpgateInstance` and let the auto-created connection verify it.

## Motivation

With `tls.certManager` on (the default) and no `tls.secretName`, the operator requested a `Certificate` but never mounted its `<name>-tls` Secret. Warpgate kept serving the self-signed certificate from its own setup, the cert-manager certificate went unused, and the auto-created `WarpgateConnection` had to skip TLS verification. The workaround was to also set `tls.secretName: <name>-tls`.

## What Changes

- The `<name>-tls` Secret is mounted and copied into `/data` exactly like `tls.secretName`. `tls.secretName` still wins when both are set.
- The Deployment waits for cert-manager to issue the Secret (`Ready=False/CertificatePending`, requeue, Secret watch) instead of starting a pod with a missing volume.
- The auto-created connection verifies against the cert-manager Secret's `ca.crt`, falling back to `tls.crt`.
- A hash of the served `tls.crt` goes into a pod template annotation so renewals (and `tls.secretName` rotations) restart the pods.
- The init script copies the TLS pair after `unattended-setup`, so setup can't replace it on first boot.
- The `tls.verifyConnection` webhook warning only fires when cert-manager is explicitly disabled.

## Behaviour Changes

- Instances with cert-manager TLS now serve the cert-manager certificate and get a verifying connection.
- Without cert-manager installed and `tls.certManager` left at its default, the Deployment is no longer created; set `tls.certManager: false`.
- Pods restart when the served certificate changes.
