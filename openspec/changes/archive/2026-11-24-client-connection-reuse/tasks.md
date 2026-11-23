# Tasks

- [x] Clone `http.DefaultTransport` for the Warpgate client and set a default 30s timeout
- [x] Add `Client.WithContext` and use `http.NewRequestWithContext` for API calls and login
- [x] Make session login state safe for concurrent use and re-login once on 401
- [x] Cache one client per `WarpgateConnection`, replace it on config change, evict on delete
- [x] Route `getWarpgateClient` and the connection reconciler through the cache
- [x] Regression test counting server-side connections across many reconciles
- [x] Tests for cache invalidation (secret and host change), eviction, timeout, cancellation and session re-login
- [x] Update the connection-management living spec
