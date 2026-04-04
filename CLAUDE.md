# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A Kubernetes operator written in Go that manages [Warpgate](https://github.com/warp-tech/warpgate)
(bastion host / privileged access management) resources via CRDs — mirroring the full surface of the
[warp-tech/warpgate Terraform provider](https://registry.terraform.io/providers/warp-tech/warpgate/latest/docs).

## Design Decisions

- **Framework:** Kubebuilder (generates CRD scaffolding, RBAC, manifests)
- **Multi-instance support:** Dedicated `WarpgateConnection` CRD pointing to different Warpgate instances (not single-instance-per-cluster)
- **Drift handling:** Full reconciliation — overwrite drift back to CRD spec (Kubernetes-native pattern)
- **Distribution:** Helm chart
- **API communication:** REST API with dual auth — bearer token (recommended) or username/password session fallback
- **Secrets:** Sensitive fields (tokens, passwords, SSH keys) reference Kubernetes Secrets, not inline CRD specs
- **Cleanup:** Finalizer-based — deleting a CR deletes the corresponding Warpgate resource
- **Scope:** Operator is cluster-scoped, CRs are namespace-scoped

## CRDs

10 CRDs:

- WarpgateInstance (deploy and manage Warpgate servers on Kubernetes)
- WarpgateConnection (connect to external or self-hosted instances)
- Role, User, Target (SSH, HTTP, MySQL, PostgreSQL)
- UserRole, TargetRole (bindings)
- PasswordCredential, PublicKeyCredential
- Ticket

API group: `warpgate.warpgate.warp.tech/v1alpha1`.

## Common Commands

Once scaffolded with Kubebuilder:

```bash
# Generate CRD manifests and RBAC from Go types
make manifests

# Generate deepcopy, client, informer boilerplate
make generate

# Build the operator binary
make build

# Run tests
make test

# Run a single test (example)
go test ./internal/controller/... -run TestRoleReconciler -v

# Install CRDs into a cluster
make install

# Run the operator locally against current kubeconfig
make run

# Build and push the operator container image
make docker-build docker-push IMG=<registry>/warpgate-operator:<tag>

# Deploy to cluster
make deploy IMG=<registry>/warpgate-operator:<tag>

# Undeploy
make undeploy
```

## Runtime Environment Variables

| Variable | Description |
| --- | --- |
| `WARPGATE_HOST` | Warpgate instance URL (e.g. `https://warpgate.example.com`) |
| `WARPGATE_TOKEN` | Admin-level API token from Warpgate (bearer auth, recommended) |
| `WARPGATE_USERNAME` | Admin username for Warpgate (session auth fallback) |
| `WARPGATE_PASSWORD` | Admin password for Warpgate (session auth fallback) |
