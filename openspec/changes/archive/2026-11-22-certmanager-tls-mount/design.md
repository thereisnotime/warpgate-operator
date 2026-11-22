# Design

## Served Secret

`servedTLSSecret` returns `tls.secretName` if set, otherwise `<name>-tls` when cert-manager is enabled, otherwise empty. The volume, init mount, init copy step, self-signed generation and `connectionTLS` all key off it, so the cert-manager case behaves exactly like the user-provided case.

## Waiting for issuance

cert-manager resources are now ensured before the Deployment. When the pod would mount the cert-manager Secret, the controller checks that it exists with `tls.crt` and `tls.key`. If not, it sets `Ready=False/CertificatePending`, skips the Deployment, Services and connection, and returns `RequeueAfter: 10s`. A Secret watch maps Secrets to the instances that serve them, so the instance usually reconciles as soon as cert-manager writes the Secret; the requeue is a fallback.

Holding the Deployment back avoids a pod stuck in `ContainerCreating` on a missing volume and keeps the connection from being created against a certificate that doesn't exist yet. The cost is that a cluster without cert-manager and the default `certManager: true` gets no Deployment; previously it got one serving a self-signed certificate while reporting `CertManagerFailed`.

## Renewal

The init container copies the pair only at pod start. With the self-signed issuer, `ca.crt` is the certificate itself, so after a renewal the connection would trust the new certificate while Warpgate still served the old one. `ensureDeployment` adds `warpgate.warp.tech/tls-hash` (FNV-1a of `tls.crt`, public data) to the pod template and rolls the Deployment when it changes. This also applies to `tls.secretName`.

## Tests

envtest gets minimal `Issuer`/`Certificate` CRDs from `internal/controller/testdata/crds/` (schemaless), and tests create the issued Secret themselves.
