# Design: Kubernetes targets with short-lived ServiceAccount tokens

## API

```yaml
spec:
  kubernetes:
    clusterURL: https://192.168.42.49:6443
    authKind: Token
    serviceAccountToken:
      name: warpgate-k8s-drinking      # ServiceAccount in the target's namespace
      audiences: [zone10-drinking]     # 1-8, required, no default
      expirationSeconds: 7200          # 7200-172800, default 7200
status:
  serviceAccountToken:
    expirationTimestamp: "2026-11-27T12:00:00Z"
    renewAfter: "2026-11-27T11:00:00Z"
```

- `serviceAccountToken` and `tokenSecretRef` are mutually exclusive. With `authKind: Token` one of them is
  required (webhook and controller). For `Certificate` and `IamRole` it is forbidden.
- Only ServiceAccounts in the target's own namespace. There is no cross-namespace reference.
- `audiences` has no default. That is deliberate: with no audiences, TokenRequest issues a token for the API
  server's own audiences, which would be valid against the operator's cluster.

## Flow (per reconcile, Kubernetes target in ServiceAccount mode)

1. Read the ServiceAccount with the uncached API reader, so the operator needs no cluster-wide list/watch on
   ServiceAccounts. Missing: `ServiceAccountNotFound`.
2. Consent, two comma-separated, exact-match annotations:
   - `warpgate.warp.tech/token-audiences` must contain every requested audience;
   - `warpgate.warp.tech/token-cluster-urls` must contain `clusterURL`, so a token is only sent where the
     ServiceAccount's owner agreed. Found by review: without it, an editor of the WarpgateTarget could repoint
     `clusterURL` and receive a valid token.
   - A missing annotation gives `ServiceAccountNotOptedIn`. A value not covered gives `TokenAudienceNotAllowed` or
     `TokenClusterURLNotAllowed`. A Forbidden read gives `TokenRequestForbidden`, since the chart grants get and
     create together.
3. In-memory cache, keyed by the target's UID. Reuse the token when the cache entry's source fingerprint matches
   and `now < renewAfter`. The fingerprint is a SHA-256 over ServiceAccount name, ServiceAccount UID, clusterURL,
   sorted audiences and expirationSeconds. A recreated ServiceAccount gets a new UID, so it gets a new token. A
   different clusterURL never reuses a token. `put` also sweeps expired entries, so a target removed without a
   reconcile doesn't leave its token in memory.
4. Otherwise create a TokenRequest (`serviceaccounts/token`). There is no `boundObjectRef`: only Pods, Secrets
   and Nodes can be bound, and a remote cluster validating through a JWT authenticator does not check bindings
   anyway.
   - Forbidden: `TokenRequestForbidden`. Any other error: `TokenRequestError`. An empty token or an expiry not in
     the future: `TokenRequestError`.
5. `renewAfter = issued + (expiration - issued) / 2`, from the expiry the API server actually returned. That
   value may be shorter than requested (`--service-account-max-token-expiration`).
6. Build the Warpgate options with the token and PUT/POST the target as usual.
7. Status: write `expirationTimestamp` and `renewAfter`. Requeue at `min(reconcileInterval, renewAfter - now)`
   (at least 1s). Targets without a token keep the plain interval, and 0 keeps meaning no periodic resync.
8. Failures retry within one minute: token errors, and Warpgate create/update errors on a target holding a token.

Leaving ServiceAccount mode, or deleting the target, drops the cache entry and clears `status.serviceAccountToken`.
A failed TokenRequest drops the entry. On any failure `status.serviceAccountToken` is left as is, because Warpgate
still holds that token until it expires.

## Why renew at half the lifetime

Warpgate (v0.30) snapshots a target's options when a Kubernetes session is admitted and reuses that snapshot for
`kubernetes.session_max_age`: 30 minutes by default, from session creation. It also reuses an upstream client for
up to 5 minutes, while the options are unchanged. So a renewed token reaches new sessions at once, and existing
sessions within 30 minutes. The minimum lifetime is 2h (raised from 1h after review). Renewed at half, the old
token stays valid for at least an hour after the renewal: twice the default `session_max_age`, with time for
one-minute retries of a failed renewal. If `session_max_age` is raised, `expirationSeconds` should be at least four
times it. The docs say so.

## Security model

| Threat | Mitigation |
|---|---|
| A WarpgateTarget turns the operator into a token-minting service for every ServiceAccount | No `serviceaccounts/token` permission in the generated ClusterRole. Helm grants it per namespace, limited by `resourceNames` to listed ServiceAccounts (`serviceAccountTokens.allowed`). Raw-manifest installs add the Role themselves (sample provided). This does not contain a compromised operator: its ClusterRole can create Secrets and Deployments cluster-wide, which can yield any ServiceAccount's token. |
| Someone who may create WarpgateTargets gets a token for an unrelated ServiceAccount in the same namespace | Two consent annotations on the ServiceAccount. The namespace is the real boundary: anyone who can update, patch or recreate ServiceAccounts there, or create Pods there, can get such tokens anyway. The docs say to keep these ServiceAccounts in an admin-only namespace. |
| An editor of a WarpgateTarget repoints `clusterURL` to collect a valid token | `token-cluster-urls` pins the destinations. A URL change never reuses the cached token. |
| A webhook rule change leaves old objects stuck in deletion | `ValidateUpdate` ratchets: no validation when the spec is unchanged or the object is being deleted. |
| A token with an audience the local API server accepts | `audiences` is required. Audiences must be on the ServiceAccount's allow-list, which is the owner's explicit decision. The docs warn against listing the local API audience unless the target is the local cluster. |
| The token leaks through status, logs, events or errors | Status has only timestamps. The token is never logged. The Warpgate API client includes response bodies in errors, so every Create/Update error is redacted (the whole token and each JWT segment) before it reaches a condition or a log, keeping the `*warpgate.APIError` type so `IsNotFound` still works. A test asserts the token is absent from the whole status object. |
| Long-lived exposure if Warpgate or the operator is compromised | Lifetime 2h to 48h, default 2h. Stopping the operator stops renewal, and tokens die within the lifetime. Issued tokens are not bound to the target, so they outlive it; on a remote cluster that only checks signatures, deleting the ServiceAccount doesn't help either. Revocation: delete the ServiceAccount locally, or remove the remote cluster's RBAC binding. |
| A cache from a previous configuration is reused | The fingerprint includes ServiceAccount UID, audiences and lifetime. Spec changes or a recreated ServiceAccount produce a new token. |
| Leader failover | The cache is per process. The new leader requests a new token, and the old one expires on its own. |

## Not done

- No `boundObjectRef` (see above).
- No events: the controller has no recorder today. Conditions carry the reason, and logs carry the context
  without the token.
- No tokens for ServiceAccounts in other namespaces.
- Migrating the operator's own Warpgate API credentials to TokenRequest is out of scope.
