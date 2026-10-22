# Tasks

- [x] Pass the database URL to the init script through `DATABASE_URL`
- [x] Add `databaseURLSecretRef`, deprecate `databaseURL`, keep the URL out of the ConfigMap
- [x] Emit `externalHost` through a YAML marshaller
- [x] Fold the init script into the pod template hash
- [x] Make `TLSConfigSpec.verify` a `*bool` defaulting to `true` (CRD, webhook, converter)
- [x] Watch Secrets from the password credential controller and replace credentials on rotation
- [x] Add `WarpgateConnection.spec.caSecretRef` and wire it into the HTTP client
- [x] Add `WarpgateInstance.spec.tls.verifyConnection` and verify the auto-created connection when the CA is known
- [x] Unit and envtest coverage for each fix
- [x] Regenerate CRDs, deepcopy and Helm chart CRDs
- [x] Update CRD docs and living specs
