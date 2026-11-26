# Client Connection Reuse

## Summary

Stop the operator from leaking HTTP connections and goroutines to Warpgate, and bound how long a single API call can take.

## Motivation

`getWarpgateClient()` built a new `warpgate.Client` on every reconcile, each with its own zero-value `http.Transport`. That transport has no idle timeout and nothing ever closed it, so every reconcile left behind a keep-alive TLS connection plus its read and write loop goroutines.

With about 200 CRs on the default 5 minute interval, v0.5.0 in production grew from 339 to 3511 sockets in roughly 100 minutes and hit its 256Mi memory limit. The runtime then thrashed, the webhook and `/healthz` stopped answering, Flux failed with `failed calling webhook ... context deadline exceeded`, and kubelet killed the pod on liveness (exit 137) about every two hours.

Separately, the HTTP client had no timeout and requests carried no context, so a hung Warpgate could block a controller worker forever.

## What Changes

- Controllers share one cached client per `WarpgateConnection` (keyed by namespace/name). The cache entry is replaced when the resolved config changes (host, TLS settings, CA bundle, credentials) and dropped when the connection is deleted; replaced clients close their idle connections.
- The transport is cloned from `http.DefaultTransport` (proxy env, idle timeout, pool limits) before the TLS settings are applied.
- `http.Client.Timeout` defaults to 30s, and requests use `http.NewRequestWithContext` with the reconcile context via `Client.WithContext`.
- Session (username/password) login state is mutex-guarded, and an expired session (401) triggers one re-login and retry.

## Breaking Changes

None.
