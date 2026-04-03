# Initial Implementation

## Summary
Implement a Kubernetes operator for Warpgate bastion host management, mirroring the full surface of the Warpgate Terraform provider as CRDs.

## Motivation
Managing Warpgate resources (roles, users, targets, credentials, tickets) through Terraform works but doesn't integrate with Kubernetes-native workflows. Teams running Warpgate alongside Kubernetes clusters need a way to manage access declaratively through CRDs, with drift reconciliation and finalizer-based cleanup.

## What Changes
- 9 CRDs: WarpgateConnection, Role, User, Target, UserRole, TargetRole, PasswordCredential, PublicKeyCredential, Ticket
- Full reconciliation controllers with finalizer cleanup
- Warpgate REST API client matching the Terraform provider
- Auto-generated passwords for users
- Auto-created secrets for tickets
- Helm chart for deployment
- CI/CD with GitHub Actions, semantic versioning, conventional commits
- Security scanning (gosec, govulncheck, trivy, gitleaks)
- 85%+ controller coverage, 100% API client coverage
