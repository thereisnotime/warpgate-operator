# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A Kubernetes operator written in Go that manages [Warpgate](https://github.com/warp-tech/warpgate) (bastion host / privileged access management) resources via CRDs — mirroring the full surface of the [warp-tech/warpgate Terraform provider](https://registry.terraform.io/providers/warp-tech/warpgate/latest/docs).

## Design Decisions

- **Framework:** Kubebuilder (generates CRD scaffolding, RBAC, manifests)
- **Multi-instance support:** Dedicated `WarpgateConnection` CRD pointing to different Warpgate instances (not single-instance-per-cluster)
- **Drift handling:** Full reconciliation — overwrite drift back to CRD spec (Kubernetes-native pattern)
- **Distribution:** Helm chart
- **API communication:** REST API with token auth (same as the Terraform provider), not direct DB access
- **Secrets:** Sensitive fields (tokens, passwords, SSH keys) reference Kubernetes Secrets, not inline CRD specs
- **Cleanup:** Finalizer-based — deleting a CR deletes the corresponding Warpgate resource
- **Scope:** Operator is cluster-scoped, CRs are namespace-scoped

## CRDs

Mirrors all 8 Terraform resources as CRDs:
- Role, User, Target (SSH, HTTP, MySQL, PostgreSQL)
- UserRole, TargetRole (bindings)
- PasswordCredential, PublicKeyCredential
- Ticket

Plus data-source equivalents where needed (role, user, target, ssh_own_keys).

API group: `warpgate.warp.tech` or similar.

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
|---|---|
| `WARPGATE_HOST` | Warpgate instance URL (e.g. `https://warpgate.example.com`) |
| `WARPGATE_TOKEN` | Admin-level API token from Warpgate |
