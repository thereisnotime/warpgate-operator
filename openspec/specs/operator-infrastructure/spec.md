# Operator Infrastructure

## Overview

Cross-cutting concerns for the warpgate-operator project: CI/CD pipelines, security scanning, Helm chart packaging, and repository configuration. These aren't CRD-specific but are essential for shipping and maintaining the operator.

## Requirements

### REQ-INFRA-001: CI Pipeline
**Status:** ADDED

A GitHub Actions CI workflow runs on every push and pull request to `main`. It lints Go code, runs unit tests, and builds the operator binary.

**Scenarios:**
- **Given** a push to any branch **When** the CI workflow triggers **Then** it runs Go lint, unit tests, and build steps.
- **Given** a pull request to `main` **When** the CI workflow triggers **Then** it runs the same checks and reports status on the PR.

### REQ-INFRA-002: Security Scanning
**Status:** ADDED

A dedicated security workflow scans the codebase for vulnerabilities.

**Scenarios:**
- **Given** a code change is pushed **When** the security workflow runs **Then** it scans for known vulnerabilities and reports findings.

### REQ-INFRA-003: E2E Testing Pipeline
**Status:** ADDED

An end-to-end testing workflow validates the operator against a real (or simulated) Kubernetes cluster with a Warpgate instance.

**Scenarios:**
- **Given** the E2E workflow is triggered **When** tests execute **Then** they exercise the full CRD lifecycle (create, update, delete) against a running Warpgate instance.

### REQ-INFRA-004: Helm Chart Testing
**Status:** ADDED

A Helm test workflow validates the operator's Helm chart for correctness (lint, template rendering, install).

**Scenarios:**
- **Given** changes to chart templates **When** the Helm test workflow runs **Then** it lints the chart and verifies templates render correctly.

### REQ-INFRA-005: Release Workflow
**Status:** ADDED

A release workflow handles building and publishing the operator container image and Helm chart when a new release is tagged.

**Scenarios:**
- **Given** a new Git tag is pushed **When** the release workflow triggers **Then** it builds the container image, pushes it to the registry, and packages the Helm chart.

### REQ-INFRA-006: Dependency Management
**Status:** ADDED

Dependabot is configured to keep Go module and GitHub Actions dependencies up to date.

**Scenarios:**
- **Given** a new version of a Go dependency is released **When** Dependabot detects it **Then** it opens a PR to update the dependency.

### REQ-INFRA-007: Kubebuilder Scaffolding
**Status:** ADDED

The project uses Kubebuilder for CRD scaffolding, RBAC generation, and deepcopy code generation. Standard `make manifests` and `make generate` commands produce all generated artifacts.

**Scenarios:**
- **Given** a change to CRD type definitions in `api/v1alpha1/` **When** `make manifests` is run **Then** CRD YAML manifests and RBAC roles are regenerated.
- **Given** a change to CRD type definitions **When** `make generate` is run **Then** deepcopy methods are regenerated.

### REQ-INFRA-008: Shared Helper Functions
**Status:** ADDED

A shared `getWarpgateClient` helper function builds a Warpgate API client from a `connectionRef` name. All resource controllers (role, user, target, bindings, credentials, tickets) use this helper instead of duplicating the connection-resolution logic.

**Scenarios:**
- **Given** any resource controller needs a Warpgate API client **When** it calls `getWarpgateClient(ctx, client, namespace, connectionName)` **Then** it receives a configured `*warpgate.Client` or an error if the connection or Secret is missing.

### REQ-INFRA-009: Consistent Finalizer Pattern
**Status:** ADDED

All controllers use the same finalizer name `warpgate.warp.tech/finalizer` and follow the same lifecycle pattern: add finalizer on first reconcile, run cleanup logic on deletion, remove finalizer after cleanup.

**Scenarios:**
- **Given** any CRD resource is created **When** the controller first reconciles **Then** it adds the `warpgate.warp.tech/finalizer`.
- **Given** any CRD resource is deleted **When** the controller reconciles the deletion **Then** it runs resource-specific cleanup, removes the finalizer, and allows Kubernetes garbage collection to proceed.

### REQ-INFRA-010: Periodic Reconciliation
**Status:** ADDED

All controllers requeue after 5 minutes (or 30 seconds for transient errors on binding controllers) to continuously detect and correct drift between the CRD spec and the actual state in Warpgate.

**Scenarios:**
- **Given** a successfully reconciled resource **When** 5 minutes elapse **Then** the controller reconciles again to detect and correct any drift.
- **Given** a binding controller that failed to resolve a name **When** reconciliation fails **Then** it requeues after 30 seconds for faster retry.

### REQ-INFRA-011: Security Scanning Tools
**Status:** ADDED

The project includes SAST (gosec), SCA (govulncheck, trivy), and secret scanning (gitleaks) both in CI and as local just recipes. Pre-commit hooks enforce gitleaks and gosec on every commit.

**Scenarios:**
- **Given** a developer runs `just sec` **When** all scanners execute **Then** gosec, govulncheck, and gitleaks run and report any findings.
- **Given** a push to `main` **When** the security workflow runs **Then** gosec, govulncheck, trivy, and gitleaks all pass.

### REQ-INFRA-012: Deterministic Local Development
**Status:** ADDED

`just minikube-deploy` handles the full lifecycle from a clean state: starts minikube with podman, installs cert-manager, builds and loads the operator image, creates a self-signed webhook certificate, installs all 9 CRDs, deploys the operator, injects the CA bundle into webhook configs, and waits for the pod to be ready. No manual steps required.

**Scenarios:**
- **Given** no minikube cluster exists **When** `just minikube-deploy` is run **Then** it creates the cluster, installs all dependencies, and deploys a fully functional operator with webhooks.
- **Given** a running deployment **When** `just minikube-teardown` is run **Then** it undeploys the operator, removes CRDs, and destroys the cluster.

### REQ-INFRA-013: Webhook Certificate Management
**Status:** ADDED

Webhook TLS certificates are managed by cert-manager. The Helm chart includes a self-signed Issuer and Certificate, with CA injection annotations on webhook configurations. For local development, the justfile provisions certificates automatically.

**Scenarios:**
- **Given** the Helm chart is installed with `webhooks.certManager.enabled: true` **When** cert-manager processes the Certificate resource **Then** a TLS secret is created and the CA bundle is injected into webhook configurations.

### REQ-INFRA-014: Branch Protection
**Status:** ADDED

The `main` branch requires all changes through PRs with 6 required status checks (Go Lint, Unit Tests, Build, Markdown Lint, Helm Lint, Validate CRD Manifests). Squash merge only, linear history enforced, review threads must be resolved.

**Scenarios:**
- **Given** a PR to `main` **When** any required status check fails **Then** the PR cannot be merged.
- **Given** a direct push to `main` by a non-admin **Then** the push is rejected.
