# Architecture

This document describes how the warpgate-operator is structured, how its components relate, and how it manages the lifecycle of Warpgate resources.

## Table of Contents

- [High-Level Overview](#high-level-overview)
- [Data Model](#data-model)
- [Reconciliation Flow](#reconciliation-flow)
- [Authentication Chain](#authentication-chain)
- [Target Type Dispatch](#target-type-dispatch)
- [WarpgateInstance Deployment](#warpgateinstance-deployment)
- [Controller Map](#controller-map)

---

## High-Level Overview

The operator bridges Kubernetes-native resource declarations with the Warpgate REST API. Every CRD is a desired-state declaration; the operator continuously drives actual state toward it.

```mermaid
%%{init: {"theme": "base", "themeVariables": {"primaryColor": "#4f46e5", "primaryTextColor": "#fff", "primaryBorderColor": "#3730a3", "lineColor": "#6366f1", "secondaryColor": "#0ea5e9", "tertiaryColor": "#f0f9ff", "background": "#ffffff", "nodeBorder": "#3730a3", "clusterBkg": "#eff6ff", "titleColor": "#1e1b4b", "edgeLabelBackground": "#eff6ff"}}}%%
flowchart LR
    subgraph k8s ["☸  Kubernetes Cluster"]
        direction TB
        subgraph crds ["Custom Resources"]
            conn["WarpgateConnection"]
            role["WarpgateRole"]
            user["WarpgateUser"]
            tgt["WarpgateTarget"]
            inst["WarpgateInstance"]
            more["… 7 more CRDs"]
        end
        op["warpgate-operator\n(controller-manager)"]
        sec["Kubernetes Secrets\n(tokens / passwords / keys)"]
        webhook["Admission Webhooks\n(validate + default)"]
    end

    subgraph wg ["🔒  Warpgate"]
        api["REST API\n/@warpgate/admin/api"]
        bastion["Bastion Engine\n(SSH · HTTP · DB · RDP · VNC)"]
    end

    crds -- "watch / reconcile" --> op
    op -- "read credentials" --> sec
    op -- "CRUD" --> api
    api --> bastion
    webhook -. "validate on create/update" .-> crds
```

---

## Data Model

Every resource CR declares a `connectionRef` pointing to a `WarpgateConnection`. The connection holds the URL and credentials needed to reach that Warpgate instance.
All other CRDs are siblings referencing the same connection; bindings link pairs of resources together.

```mermaid
%%{init: {"theme": "base", "themeVariables": {"primaryColor": "#7c3aed", "primaryTextColor": "#fff", "primaryBorderColor": "#5b21b6", "lineColor": "#8b5cf6", "secondaryColor": "#10b981", "tertiaryColor": "#f5f3ff", "background": "#ffffff", "nodeBorder": "#5b21b6", "clusterBkg": "#f5f3ff"}}}%%
erDiagram
    WarpgateInstance ||--o| WarpgateConnection : "auto-creates on sync"

    WarpgateConnection ||--o{ WarpgateRole : "connectionRef"
    WarpgateConnection ||--o{ WarpgateUser : "connectionRef"
    WarpgateConnection ||--o{ WarpgateTarget : "connectionRef"
    WarpgateConnection ||--o{ WarpgateTargetGroup : "connectionRef"
    WarpgateConnection ||--o{ WarpgateUserRole : "connectionRef"
    WarpgateConnection ||--o{ WarpgateTargetRole : "connectionRef"
    WarpgateConnection ||--o{ WarpgatePasswordCredential : "connectionRef"
    WarpgateConnection ||--o{ WarpgatePublicKeyCredential : "connectionRef"
    WarpgateConnection ||--o{ WarpgateTicket : "connectionRef"

    WarpgateRole ||--o{ WarpgateUserRole : "roleRef"
    WarpgateUser ||--o{ WarpgateUserRole : "userRef"

    WarpgateRole ||--o{ WarpgateTargetRole : "roleRef"
    WarpgateTarget ||--o{ WarpgateTargetRole : "targetRef"

    WarpgateTargetGroup ||--o{ WarpgateTarget : "groupRef (on target)"

    WarpgateUser ||--o{ WarpgatePasswordCredential : "userRef"
    WarpgateUser ||--o{ WarpgatePublicKeyCredential : "userRef"

    WarpgateUser ||--o{ WarpgateTicket : "username (issued to)"
    WarpgateTarget ||--o{ WarpgateTicket : "targetRef"
```

---

## Reconciliation Flow

Every resource controller follows the same loop. The operator reconciles on CR changes and on a configurable periodic interval (default 5 minutes) to correct drift.

```mermaid
%%{init: {"theme": "base", "themeVariables": {"primaryColor": "#0369a1", "primaryTextColor": "#fff", "primaryBorderColor": "#075985", "lineColor": "#0ea5e9", "secondaryColor": "#f0f9ff", "tertiaryColor": "#e0f2fe", "background": "#ffffff", "nodeBorder": "#075985", "clusterBkg": "#f0f9ff", "edgeLabelBackground": "#e0f2fe"}}}%%
flowchart TD
    A([CR event or\nperiodic requeue]) --> B[Fetch CR from\nKubernetes API]
    B --> C{CR found?}
    C -- No --> Z([Done — object\ndeleted, ignore])
    C -- Yes --> D{Deletion\ntimestamp set?}

    D -- Yes --> E[Finalizer path:\nCall Warpgate DELETE API]
    E --> F[Remove finalizer\nfrom CR]
    F --> Z

    D -- No --> G[Ensure finalizer\nis present on CR]
    G --> H[Resolve WarpgateConnection\nin same namespace]
    H --> I{Connection\nReady?}
    I -- No --> J([Requeue — wait\nfor connection])
    I -- Yes --> K[Read auth credentials\nfrom Kubernetes Secret]
    K --> L[Build Warpgate\nREST client]
    L --> M{ExternalID\nalready set?}

    M -- No --> N[POST /targets\nor /users or /roles …]
    N --> O[Write ExternalID\nto CR status]
    M -- Yes --> P[PUT /targets/:id\nor /users/:id …]
    P --> Q{PUT returns\n404?}
    Q -- Yes --> R[Clear ExternalID,\nrequeue to recreate]
    Q -- No --> O

    O --> S[Set Ready=True\ncondition on CR]
    S --> T([Requeue after\nReconcileInterval])
```

### Status Conditions

Every CR exposes a `Ready` condition. The `reason` field narrows down failures:

```mermaid
%%{init: {"theme": "base", "themeVariables": {"primaryColor": "#dc2626", "primaryTextColor": "#fff", "primaryBorderColor": "#991b1b", "lineColor": "#ef4444", "secondaryColor": "#16a34a", "tertiaryColor": "#fef2f2", "background": "#ffffff", "nodeBorder": "#991b1b"}}}%%
stateDiagram-v2
    [*] --> Pending : CR created
    Pending --> Ready : API call succeeded
    Pending --> ClientError : WarpgateConnection missing\nor credentials bad
    Pending --> BuildError : Secret missing / bad\nor dependency not synced yet
    Pending --> CreateError : Warpgate API POST failed
    Ready --> UpdateError : Warpgate API PUT failed
    Ready --> NotFound : Warpgate API PUT → 404\n(resource deleted out-of-band)
    NotFound --> Pending : ExternalID cleared, requeued
    ClientError --> Pending : requeued
    BuildError --> Pending : requeued
    CreateError --> Pending : requeued
    UpdateError --> Ready : next reconcile succeeds
```

---

## Authentication Chain

The operator supports two auth modes against Warpgate: bearer token (preferred) and username/password session fallback. Both are resolved from Kubernetes Secrets.

```mermaid
%%{init: {"theme": "base", "themeVariables": {"primaryColor": "#d97706", "primaryTextColor": "#fff", "primaryBorderColor": "#92400e", "lineColor": "#f59e0b", "secondaryColor": "#fef3c7", "tertiaryColor": "#fffbeb", "background": "#ffffff", "nodeBorder": "#92400e", "clusterBkg": "#fffbeb", "edgeLabelBackground": "#fef3c7"}}}%%
flowchart TD
    conn["WarpgateConnection CR\nspec.host = https://wg.example.com"] --> tok{authSecretRef\nset?}

    tok -- Yes --> sec1["Kubernetes Secret\n→ key 'token'"]
    sec1 --> bearer["Authorization: Bearer &lt;token&gt;\non every request"]

    tok -- No --> sec2["Kubernetes Secret\nspec.usernameSecretRef / passwordSecretRef"]
    sec2 --> login["POST /@warpgate/api/auth/login\n→ set-cookie: warpgate-session"]
    login --> session["Session cookie\non subsequent requests"]

    bearer --> api["Warpgate Admin API\n/@warpgate/admin/api/…"]
    session --> api
```

---

## Target Type Dispatch

The target controller resolves which Warpgate options struct to build based on which field in the spec is non-nil. Exactly one type field is permitted (enforced by the admission webhook).

```mermaid
%%{init: {"theme": "base", "themeVariables": {"primaryColor": "#059669", "primaryTextColor": "#fff", "primaryBorderColor": "#065f46", "lineColor": "#10b981", "secondaryColor": "#d1fae5", "tertiaryColor": "#ecfdf5", "background": "#ffffff", "nodeBorder": "#065f46", "clusterBkg": "#ecfdf5", "edgeLabelBackground": "#d1fae5"}}}%%
flowchart TD
    spec["WarpgateTarget spec"] --> sw{Which field\nis non-nil?}

    sw -- ssh --> SSH["buildSSHOptions\n→ SSHOptions{kind:Ssh}\n• host / port / username\n• auth: Password · PublicKey · IamRole\n• password from Secret\n• jump_host UUID from\n  referenced WarpgateTarget"]

    sw -- http --> HTTP["buildHTTPOptions\n→ HTTPOptions{kind:Http}\n• url / tls / headers\n• externalHost"]

    sw -- mysql --> MySQL["buildMySQLOptions\n→ MySQLOptions{kind:MySql}\n• host / port / username\n• auth password from Secret\n• tls / defaultDatabaseName"]

    sw -- postgresql --> PG["buildPostgreSQLOptions\n→ PostgresOptions{kind:Postgres}\n• host / port / username\n• auth password from Secret\n• tls / protocolVersion\n  (default 3.2)"]

    sw -- kubernetes --> K8S["buildKubernetesOptions\n→ KubernetesOptions{kind:Kubernetes}\n• clusterURL / tls\n• auth: Token · Certificate · IamRole\n• token / cert / key from Secrets"]

    sw -- rdp --> RDP["buildRDPOptions\n→ RDPOptions{kind:Rdp}\n• host / port / username / domain\n• password from Secret (optional)\n• compression default remotefx\n• tlsSecurity default Tls12"]

    sw -- vnc --> VNC["buildVNCOptions\n→ VNCOptions{kind:Vnc}\n• host / port\n• auth: None (no secret)\n  or Password (from Secret)"]

    SSH & HTTP & MySQL & PG & K8S & RDP & VNC --> marshal["json.Marshal → RawMessage\nembedded in TargetRequest.Options"]
    marshal --> api["POST/PUT\n/@warpgate/admin/api/targets"]
```

---

## WarpgateInstance Deployment

`WarpgateInstance` is the only CRD that manages Kubernetes workloads directly. It deploys Warpgate itself and optionally auto-creates a `WarpgateConnection` pointing to it.

```mermaid
%%{init: {"theme": "base", "themeVariables": {"primaryColor": "#7c3aed", "primaryTextColor": "#fff", "primaryBorderColor": "#5b21b6", "lineColor": "#8b5cf6", "secondaryColor": "#ede9fe", "tertiaryColor": "#f5f3ff", "background": "#ffffff", "nodeBorder": "#5b21b6", "clusterBkg": "#f5f3ff", "edgeLabelBackground": "#ede9fe"}}}%%
flowchart TD
    wgi["WarpgateInstance CR\n(version, replicas, listeners, storage, tls)"]

    wgi --> cm["ConfigMap\nwarpgate.yaml\n(generated from spec)"]
    wgi --> pvc["PersistentVolumeClaim\ndata volume\n(create-only — no spec updates)"]
    wgi --> dep["Deployment\n(image, env, mounts,\nliveness/readiness probes)"]
    wgi --> svc["Services\n• ClusterIP per enabled listener\n  HTTP · SSH · MySQL · PostgreSQL\n  · Kubernetes · (RDP/VNC via HTTP)"]

    dep -- mounts --> cm
    dep -- mounts --> pvc
    dep -- mounts --> keys["SSH Keys Secret\n(host + client keys;\nWarpgate generates if absent)"]

    wgi -. "autoConnect: true" .-> wgconn["WarpgateConnection CR\nauto-created, pointing at\nthe ClusterIP service"]

    dep --> ready["Deployment Ready\n→ WarpgateInstance\nReady=True"]
```

---

## Controller Map

| Controller | Watches | Warpgate API endpoint | Notes |
|---|---|---|---|
| `WarpgateConnectionReconciler` | `WarpgateConnection` | — (validates connectivity) | No ExternalID; validates creds on each reconcile |
| `WarpgateInstanceReconciler` | `WarpgateInstance` | — (manages k8s workloads) | Creates Deployment, Services, PVC, ConfigMap |
| `WarpgateRoleReconciler` | `WarpgateRole` | `/roles` | Standard CRUD |
| `WarpgateUserReconciler` | `WarpgateUser` | `/users` | Auto-generates password; stores in Secret |
| `WarpgateTargetReconciler` | `WarpgateTarget` | `/targets` | 7 type dispatchers; secret lookups |
| `WarpgateTargetGroupReconciler` | `WarpgateTargetGroup` | `/target-groups` | Standard CRUD |
| `WarpgateUserRoleReconciler` | `WarpgateUserRole` | `/users/:id/roles/:id` | Waits for both User and Role to have ExternalID |
| `WarpgateTargetRoleReconciler` | `WarpgateTargetRole` | `/targets/:id/roles/:id` | Waits for both Target and Role to have ExternalID |
| `WarpgatePasswordCredentialReconciler` | `WarpgatePasswordCredential` | `/users/:id/credentials` | Reads password from Secret |
| `WarpgatePublicKeyCredentialReconciler` | `WarpgatePublicKeyCredential` | `/users/:id/credentials` | Reads public key from Secret |
| `WarpgateTicketReconciler` | `WarpgateTicket` | `/tickets` | Writes generated ticket value to a Secret |
| `HTTPRouteWatcherController` | `HTTPRoute` (Gateway API) | — | Watches routes to trigger target reconciles |
