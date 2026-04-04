# Tasks

- [x] Define WarpgateInstance CRD types (spec, status, sub-types)
- [x] Add Kubebuilder markers for validation, defaults, print columns, scale subresource
- [x] Generate CRD manifests and deepcopy boilerplate
- [x] Implement WarpgateInstance controller (StatefulSet, Services, ConfigMap)
- [x] Implement cert-manager Certificate creation and self-signed Issuer fallback
- [x] Implement auto-creation of WarpgateConnection CR with owner references
- [x] Add status updates (readyReplicas, version, connectionRef, endpoint, conditions)
- [x] Add finalizer-based cleanup
- [x] Write controller unit tests
- [x] Add RBAC markers for StatefulSet, Service, ConfigMap, Certificate permissions
- [x] Update Helm chart with WarpgateInstance CRD and RBAC
- [x] Create CRD documentation (docs/crds/warpgate-instance.md)
- [x] Update README with WarpgateInstance in CRD table and features
- [x] Add OpenSpec spec for instance-management
