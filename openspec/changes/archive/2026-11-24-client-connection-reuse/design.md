# Design

## Client cache

`internal/controller/helpers.go` holds a package-level `clientCache` shared by all controllers in the manager. `getWarpgateClient` and the connection reconciler both go through `clientForConnection`, which resolves the auth and CA Secrets into a `warpgate.Config` and asks the cache for a client.

The cache key is the connection's namespace/name. Each entry keeps the resolved `warpgate.Config` it was built from and compares it field by field on lookup. When anything differs, the old client's idle connections are closed and a new client replaces it. In-flight requests on the old client finish normally, since only idle connections are closed. Comparing resolved values (rather than Secret resourceVersions) avoids churn on no-op Secret updates and covers every input that affects the client. No hash is stored, since the client holds the credentials in memory anyway and hashing a password trips gosec and CodeQL.

The connection reconciler evicts the entry when the connection is being deleted or is gone.

## Context and timeout

Threading `context.Context` through every resource helper would touch about 45 methods and all callers. Instead `Client.WithContext(ctx)` returns a shallow copy that shares the HTTP client and login session but sends requests with `ctx`. The cached client is returned to controllers already bound to the reconcile context. `http.Client.Timeout` (30s, `Config.Timeout` to override) covers callers without a deadline.

## Session auth on a shared client

The `authed` flag moved into a `session` struct behind a mutex, so concurrent workers wait for one login. Since a cached client outlives Warpgate's session cookie, a 401 on session auth invalidates the session and retries the request once after logging in again. A generation counter keeps a stale 401 from discarding a session another worker just created. Token auth never retries.
