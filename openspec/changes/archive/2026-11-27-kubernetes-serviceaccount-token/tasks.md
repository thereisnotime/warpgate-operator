# Tasks

- [x] API: `ServiceAccountTokenSpec`, `status.serviceAccountToken`, CRD markers; `make manifests generate`, chart CRDs
- [x] Webhook: Token needs exactly one source; forbidden for Certificate/IamRole; Certificate needs both refs; name, audience and lifetime rules
- [x] Controller: ServiceAccount lookup via API reader, consent annotation, TokenRequest, in-memory cache with fingerprint, renewal timing, requeue, status, condition reasons, redaction
- [x] Controller: nil-safe Secret references (no panic on a missing ref)
- [x] Wiring in `cmd/main.go` (API reader)
- [x] Helm: `serviceAccountTokens.allowed` renders Role + RoleBinding with `resourceNames`; chart README
- [x] Sample manifests (target, ServiceAccount with annotation, Role/RoleBinding for raw installs)
- [x] Tests: webhook table tests; unit tests for timing, fingerprint, annotation parsing and redaction; envtest against the real TokenRequest API (issue, reuse, renew, spec change, recreated ServiceAccount, consent failures, forbidden, no token in status, cleanup); Helm template test
- [x] Docs: `docs/crds/warpgate-target.md`, a guide for local and remote clusters (AuthenticationConfiguration), README feature list
- [x] Living spec `openspec/specs/target-management/spec.md`
- [x] `just check` green, security scan (`just sec`)
