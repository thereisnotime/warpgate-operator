# WarpgateInstance CRD

## Summary

Add a `WarpgateInstance` CRD that deploys and manages Warpgate bastion host instances directly on Kubernetes, giving the operator full lifecycle control over the Warpgate server itself -- not just the resources inside it.

## Motivation

The existing CRDs (Role, User, Target, etc.) all assume Warpgate is already running somewhere and reachable via API. That's fine when Warpgate is deployed separately, but many teams want a fully self-contained setup where the operator manages everything end-to-end. Without this, users need to maintain a separate Helm chart or Deployment for Warpgate itself, which splits the management surface and makes it harder to reason about the full stack.

With `WarpgateInstance`, the operator can deploy Warpgate, configure TLS, provision storage, and automatically create a `WarpgateConnection` CR -- so other CRDs can reference the deployed instance immediately without any manual wiring.

## What Changes

- New `WarpgateInstance` CRD with spec covering version, replicas, protocol listeners (HTTP, SSH, MySQL, PostgreSQL), storage, TLS, resources, scheduling, and auto-connection creation
- Controller that manages a StatefulSet, Services, ConfigMap, and optionally a cert-manager Certificate
- Auto-creation of a `WarpgateConnection` CR pointing to the deployed instance's internal Service URL
- Status tracking for ready replicas, deployed version, endpoint, and connection reference
- Scale subresource support for `kubectl scale` and HPA compatibility
