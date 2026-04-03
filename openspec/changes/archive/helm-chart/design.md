# Design

## CRDs: templates/ vs crds/

Helm supports CRDs in a dedicated `crds/` directory, but those are only installed on first install and never updated or deleted. Placing CRDs in `templates/` instead gives us full Helm lifecycle management — upgrades apply CRD changes, and `crds.install: false` lets users who manage CRDs out-of-band skip them entirely. The trade-off is that `helm uninstall` will remove the CRDs (and all CRs), but that's acceptable since we document it and most production users manage CRDs separately anyway.

**Decision:** CRDs live in `templates/crds/` gated by `.Values.crds.install`.

## values.yaml Structure

Top-level keys:

- **image** — repository, tag, pullPolicy, pullSecrets
- **rbac** — `create: true` controls ClusterRole/ClusterRoleBinding generation
- **serviceAccount** — `create: true`, `name`, `annotations` (for IRSA/workload identity)
- **crds** — `install: true` toggles CRD templates
- **resources** — requests/limits for the manager container
- **metrics** — `enabled: true`, `service.port: 8443`, `service.type: ClusterIP`
- **health** — liveness/readiness probe paths and ports (from controller-runtime defaults)
- **leaderElection** — `enabled: true`, `resourceName` override
- **replicas** — defaults to 1 (leader election handles multi-replica)
- **tolerations**, **nodeSelector**, **affinity** — standard scheduling knobs
- **securityContext** / **podSecurityContext** — non-root, read-only rootfs, drop all caps

## RBAC Rules

Derived from controller-gen output in `config/rbac/`. The ClusterRole aggregates all permissions the controllers need: full CRUD on the 9 CRDs, read/write on Secrets (for credentials and tickets), Events, and the coordination lease for leader election. Kept as a single ClusterRole bound to the operator's ServiceAccount.

## Metrics Service

A ClusterIP Service on port 8443 exposing the controller-runtime `/metrics` endpoint. Gated by `.Values.metrics.enabled` so clusters without Prometheus can skip it. ServiceMonitor template is not included initially — users can add one via raw manifests or a separate chart.

## NOTES.txt

Post-install notes print:
- How to verify the operator is running (`kubectl get pods`)
- How to create a WarpgateConnection CR to get started
- Link to the docs/crds/ documentation
