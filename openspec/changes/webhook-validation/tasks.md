# Tasks

- [x] Enable webhook scaffolding in Kubebuilder
- [x] Implement WarpgateConnection validation (host URL format, secret ref)
- [x] Implement WarpgateTarget validation (exactly one type, valid TLS mode)
- [x] Implement WarpgateTarget defaults (SSH port 22, MySQL 3306, PG 5432)
- [x] Implement credential CRD validation
- [x] Implement ticket CRD validation (expiry format, immutability)
- [x] Implement binding CRD validation (UserRole, TargetRole)
- [x] Implement Role and User validation and defaults
- [ ] Add cert-manager to Helm chart
- [x] Uncomment webhook sections in kustomize configs
- [x] Write webhook unit tests
- [ ] Update E2E tests for webhook rejection
- [ ] Update documentation
