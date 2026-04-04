# Instance Management

## Overview

The `WarpgateInstance` CRD deploys and manages Warpgate bastion host instances directly on Kubernetes. The controller provisions a StatefulSet, Services, ConfigMap, and optional TLS resources, then keeps them converged with the CR spec. When `createConnection` is enabled, it also auto-creates a `WarpgateConnection` CR so other CRDs can reference the deployed instance without manual setup.

## Requirements

### REQ-INST-001: Instance CRD Definition

**Status:** ADDED

The operator provides a `WarpgateInstance` custom resource with the following spec fields:

- `version` (required) -- Warpgate image tag to deploy.
- `image` (optional) -- full image reference override, defaults to `ghcr.io/warp-tech/warpgate:<version>`.
- `replicas` (optional, default `1`) -- number of pods, minimum 1.
- `adminPasswordSecretRef` (required) -- reference to a Secret containing the initial admin password. Sub-fields: `name` and `key`.
- `http` (optional) -- HTTP listener configuration: `enabled` (default `true`), `port` (default `8888`), `serviceType` (default `ClusterIP`).
- `ssh` (optional) -- SSH listener configuration: `enabled`, `port` (default `2222`), `serviceType` (default `ClusterIP`).
- `mysql` (optional) -- MySQL proxy listener: `enabled`, `port`.
- `postgresql` (optional) -- PostgreSQL proxy listener: `enabled`, `port`.
- `storage` (optional) -- PVC configuration: `size` (default `1Gi`), `storageClassName`.
- `tls` (optional) -- TLS configuration: `certManager` (default `true`), `issuerRef` with `name` and `kind`.
- `resources` (optional) -- CPU/memory requests and limits.
- `nodeSelector` (optional) -- node scheduling constraints.
- `tolerations` (optional) -- scheduling tolerations.
- `createConnection` (optional, default `true`) -- auto-create a `WarpgateConnection` CR.
- `externalHost` (optional) -- external hostname for cookie domain and URL generation.

Status fields:

- `readyReplicas` -- number of ready pods.
- `version` -- currently deployed version.
- `connectionRef` -- name of the auto-created `WarpgateConnection` CR.
- `endpoint` -- internal service URL.
- `conditions` -- standard Kubernetes conditions list (map by type).

Print columns: `Version`, `Replicas`, `Ready`, `Endpoint`, `Age`.

Scale subresource: `spec.replicas` maps to `status.readyReplicas`.

**Scenarios:**

- **Given** a valid `WarpgateInstance` CR is created **When** the controller reconciles **Then** it creates a StatefulSet, HTTP Service, ConfigMap, and (if TLS is enabled) a Certificate, and sets the `Ready` condition to `True` once the StatefulSet is available.
- **Given** a `WarpgateInstance` with `ssh.enabled: true` **When** the controller reconciles **Then** it also creates an SSH Service with the configured port and service type.
- **Given** a `WarpgateInstance` referencing a non-existent admin password Secret **When** the controller reconciles **Then** it sets the `Ready` condition to `False` with a descriptive reason.

### REQ-INST-002: StatefulSet Management

**Status:** ADDED

The controller creates and manages a StatefulSet for the Warpgate pods, using `volumeClaimTemplates` for persistent storage. The StatefulSet template includes a config hash annotation so that ConfigMap changes trigger rolling restarts.

**Scenarios:**

- **Given** a `WarpgateInstance` with `replicas: 2` **When** the controller reconciles **Then** the StatefulSet has 2 replicas and `status.readyReplicas` reflects the actual count.
- **Given** a `WarpgateInstance` whose spec changes (e.g. new version) **When** the controller reconciles **Then** the StatefulSet is updated and pods roll out with the new configuration.
- **Given** the user runs `kubectl scale warpgateinstance <name> --replicas=3` **When** the controller reconciles **Then** the StatefulSet scales to 3 replicas.

### REQ-INST-003: TLS via cert-manager

**Status:** ADDED

When `tls.certManager` is true, the controller creates a cert-manager `Certificate` resource. If `tls.issuerRef` is provided, it references that issuer. Otherwise, the controller creates a self-signed `Issuer` in the same namespace.

**Scenarios:**

- **Given** a `WarpgateInstance` with `tls.certManager: true` and no `issuerRef` **When** the controller reconciles **Then** it creates a self-signed Issuer and a Certificate referencing it.
- **Given** a `WarpgateInstance` with `tls.issuerRef` pointing to a ClusterIssuer **When** the controller reconciles **Then** the Certificate references the specified ClusterIssuer.
- **Given** cert-manager is not installed in the cluster **When** the controller creates a Certificate resource **Then** the resource fails to reconcile and the `WarpgateInstance` status reflects the issue.

### REQ-INST-004: Auto-Created WarpgateConnection

**Status:** ADDED

When `createConnection` is true, the controller creates a `WarpgateConnection` CR in the same namespace, pointing to the deployed instance's internal Service URL. The connection is owned by the `WarpgateInstance` (via `ownerReferences`) and its name is stored in `status.connectionRef`.

**Scenarios:**

- **Given** a `WarpgateInstance` with `createConnection: true` **When** the controller reconciles and the instance is ready **Then** it creates a `WarpgateConnection` CR with the internal endpoint URL and an auth Secret reference.
- **Given** a `WarpgateInstance` is deleted **When** Kubernetes processes the owner references **Then** the auto-created `WarpgateConnection` is garbage-collected.
- **Given** a `WarpgateInstance` with `createConnection: false` **When** the controller reconciles **Then** no `WarpgateConnection` CR is created.

### REQ-INST-005: Finalizer-Based Cleanup

**Status:** ADDED

The controller adds the `warpgate.warp.tech/finalizer` to every `WarpgateInstance`. On deletion, the finalizer handles cleanup of any resources not covered by owner-reference garbage collection, then removes the finalizer.

**Scenarios:**

- **Given** a newly created `WarpgateInstance` **When** the controller reconciles **Then** the finalizer is added.
- **Given** a `WarpgateInstance` marked for deletion **When** the controller reconciles **Then** it cleans up resources and removes the finalizer.

### REQ-INST-006: Drift Reconciliation

**Status:** ADDED

The controller requeues every 5 minutes. On each pass it compares the owned resources (StatefulSet, Services, ConfigMap, Certificate) against the desired state derived from the CR spec and converges any drift.

**Scenarios:**

- **Given** a deployed `WarpgateInstance` whose StatefulSet was manually edited **When** the controller reconciles **Then** it overwrites the StatefulSet back to the desired state.
- **Given** a deployed `WarpgateInstance` whose HTTP Service was deleted **When** the controller reconciles **Then** it recreates the Service.
