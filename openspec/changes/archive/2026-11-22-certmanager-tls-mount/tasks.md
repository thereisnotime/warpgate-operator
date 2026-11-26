# Tasks

- [x] Resolve the served TLS Secret from `tls.secretName` or the cert-manager Certificate
- [x] Mount and copy the cert-manager Secret, skip self-signed generation
- [x] Ensure cert-manager resources before the Deployment and wait for the Secret (`CertificatePending`, requeue, Secret watch)
- [x] Verify the auto-created connection against the cert-manager Secret
- [x] Roll pods when the served certificate changes
- [x] Narrow the `verifyConnection` webhook warning to `certManager: false`
- [x] Load stub cert-manager CRDs in envtest and cover the new paths
- [x] Regenerate CRDs and Helm chart CRDs
- [x] Update CRD docs, CONTRIBUTING and living specs
