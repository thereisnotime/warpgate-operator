# Design

## StatefulSet Pattern

The controller creates a StatefulSet (not a Deployment) for the Warpgate pods. Warpgate stores its SQLite database and configuration under `/data`, so stable storage identity matters -- StatefulSet gives us ordered pod identity and stable PVC bindings via `volumeClaimTemplates`. Each pod gets a dedicated PVC that persists across restarts and reschedules. Scaling is supported but Warpgate itself doesn't have built-in clustering, so multi-replica setups assume an external shared storage or are used for rolling updates.

## ConfigMap Generation

The controller generates a Warpgate configuration file and stores it in a ConfigMap, which is mounted into the StatefulSet pods. The ConfigMap is regenerated on every reconciliation to reflect spec changes (listener ports, enabled protocols, external host, etc.). The StatefulSet template includes a hash annotation of the ConfigMap content so that config changes trigger a rolling restart automatically.

## Protocol Listener Services

Each enabled protocol listener (HTTP, SSH, MySQL, PostgreSQL) gets its own Kubernetes Service. HTTP and SSH support configurable Service types (`ClusterIP`, `NodePort`, `LoadBalancer`) since they're commonly exposed externally. MySQL and PostgreSQL listeners are internal-only for now (always `ClusterIP`) and only created when explicitly enabled.

## cert-manager TLS

When `tls.certManager` is true, the controller creates a cert-manager `Certificate` resource for the Warpgate instance. If the user provides an `issuerRef`, the Certificate references that Issuer or ClusterIssuer. If no issuer is specified, the controller creates a self-signed `Issuer` in the same namespace as a sensible default for development and internal deployments. The resulting TLS Secret is mounted into the StatefulSet.

The operator does not bundle cert-manager -- it must already be installed in the cluster. If cert-manager is not present and `tls.certManager` is true, the Certificate resource will fail to reconcile, which the operator surfaces via status conditions.

## Auto-Created WarpgateConnection

When `createConnection` is true (the default), the controller creates a `WarpgateConnection` CR in the same namespace, configured with the internal Service URL (e.g. `https://<name>-http.<namespace>.svc.cluster.local:8888`) and an auth Secret reference. This means users can immediately reference the deployed instance from other CRDs like `WarpgateRole` or `WarpgateUser` without manually creating a connection.

The auto-created connection is owned by the `WarpgateInstance` CR (via `ownerReferences`), so deleting the instance also cleans up the connection. The connection name is stored in `status.connectionRef`.

## Scale Subresource

The CRD exposes a Kubernetes scale subresource mapping `spec.replicas` to `status.readyReplicas`. This enables `kubectl scale warpgateinstance <name> --replicas=N` and makes the resource compatible with HPA, though horizontal autoscaling for a bastion host is an unusual use case.

## Reconciliation Strategy

The controller follows the same pattern as the other CRDs: add a finalizer on first reconciliation, compare desired state with actual state on every pass, and converge. Owned resources (StatefulSet, Services, ConfigMap, Certificate, WarpgateConnection) are created or updated as needed. On deletion, the finalizer cleans up any resources not covered by owner-reference garbage collection.

The controller requeues every 5 minutes for drift detection, consistent with the rest of the operator.
