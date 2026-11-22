# Secret Label Selector

## Summary

Let the operator cache and read only Secrets that carry a label, on top of the namespace scoping from v0.30.0 (issue #135).

## Motivation

Since #128 the manager watches Secrets (password rotation, CA and database URL refs), and every controller reads Secrets through the cached client. With the default setup the operator keeps every Secret of every watched namespace in memory. Namespace scoping narrows that to a set of namespaces, but inside them it still caches Secrets that have nothing to do with Warpgate.

## What Changes

- New `--secret-selector` flag and `operator.secretSelector` Helm value. When set, the Secret informer uses it as a label selector (`cache.Options.ByObject`).
- The manager client is wrapped so that Secrets the operator creates or updates get the selector's labels, and a Get of a Secret hidden by the selector returns an error that says which label is missing.
- Controllers report that error with condition reason `SecretNotSelected`.
- User and ticket finalizers delete their Secrets by name instead of through the cache.
- The Helm ClusterRole gets the `httproutes` rule the generated role already had.

## Breaking Changes

None by default. With the selector enabled, referenced Secrets need the label, and operator-created Secrets from older versions need to be labelled once.
