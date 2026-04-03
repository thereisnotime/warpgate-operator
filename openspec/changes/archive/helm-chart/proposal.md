# Helm Chart

## Summary
Package the operator as a production-quality Helm chart for easy deployment.

## Motivation
Users need a standard way to install the operator on any Kubernetes cluster without building from source. Helm is the de facto package manager for Kubernetes.

## What Changes
- Helm chart at charts/warpgate-operator/
- All 9 CRDs included as templates (togglable via crds.install)
- Configurable deployment (replicas, resources, security context, tolerations)
- RBAC, service account, metrics service as templates
- Published to OCI registry via CI/CD
