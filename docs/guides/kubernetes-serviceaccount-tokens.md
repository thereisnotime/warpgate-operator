# Kubernetes Targets with Short-Lived ServiceAccount Tokens

This guide sets up a Warpgate Kubernetes target whose bearer token the operator requests from the Kubernetes
TokenRequest API, instead of a long-lived token in a Secret. Field reference:
[WarpgateTarget, ServiceAccount tokens](../crds/warpgate-target.md#serviceaccount-tokens).

What you get:

- **No long-lived credential.** Tokens live 2h to 48h (default 2h) and are renewed automatically at half their
  lifetime.
- **Nothing secret in Secrets, Git or the CR.** The token exists only in the operator's memory and in Warpgate.
- **Nothing copied between clusters** for remote targets. The remote cluster trusts the operator cluster's
  ServiceAccount issuer and maps the token to its own RBAC.

## How It Fits Together

```text
operator cluster                                          target cluster
  ServiceAccount warpgate-k8s-drinking
    annotation warpgate.warp.tech/token-audiences
  warpgate-operator ── TokenRequest (aud, 2h) ──► API server
          │  token, renewed at 1h
          ▼
  Warpgate target k8s-drinking ── Bearer token ─────────────► API server (JWT authenticator:
                                                                trusts the operator cluster's issuer,
  issuer discovery (public keys) ◄──── fetches keys ────────── maps the token to a user, RBAC)
```

Users still log in to Warpgate with their own credentials. Warpgate then calls the cluster with the target's token.
Warpgate strips client `Impersonate-*` headers, so everyone using a target acts as its ServiceAccount: use one target
per access level (for example `cluster-admin` and `view`).

## 1. Common Setup (operator cluster)

Create the ServiceAccount next to the WarpgateTarget and opt it in: the audiences its tokens may be issued for,
and the cluster URLs they may be sent to. Both lists are comma-separated and matched exactly.

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: warpgate-k8s-drinking
  namespace: warpgate-operator
  annotations:
    warpgate.warp.tech/token-audiences: zone10-drinking
    warpgate.warp.tech/token-cluster-urls: https://192.168.42.49:6443
```

The URL pin matters: the audience says which cluster accepts the token, the URL says where Warpgate sends it.
Without it, anyone who can edit the WarpgateTarget could point `clusterURL` at their own server and collect a
valid token.

Allow the operator to request tokens for it. With Helm:

```yaml
serviceAccountTokens:
  allowed:
    - namespace: warpgate-operator
      serviceAccounts: [warpgate-k8s-drinking]
```

Without Helm, apply the Role and RoleBinding from
[`config/samples/serviceaccount-token-rbac.yaml`](../../config/samples/serviceaccount-token-rbac.yaml) after
adjusting names.

Both are needed: the annotations are the ServiceAccount owner's consent, the Role is the cluster admin's. The
generated ClusterRole never includes `serviceaccounts/token`, so a WarpgateTarget can't make the operator request
tokens for a ServiceAccount that isn't listed.

**Put these ServiceAccounts in a namespace only administrators can write to,** such as the operator's own. That
namespace is the real trust boundary:

- whoever can `update`, `patch`, or delete and recreate a listed ServiceAccount there controls its annotations;
- whoever can create Pods there can run as the ServiceAccount and get tokens with any audience, without the
  operator;
- whoever can create or edit WarpgateTargets there can use the ServiceAccount's tokens through Warpgate, within the
  audiences and URLs it allows.

## 2a. Target: the Operator's Own Cluster

The token has to be valid for the local API server, so the audience is the API server's own audience. That is
usually the ServiceAccount issuer URL (`https://kubernetes.default.svc.cluster.local` on k3s and kubeadm, or what
`--api-audiences` says). List it explicitly, since it makes the token usable against this cluster:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: warpgate-k8s-local
  namespace: warpgate-operator
  annotations:
    warpgate.warp.tech/token-audiences: https://kubernetes.default.svc.cluster.local
    warpgate.warp.tech/token-cluster-urls: https://kubernetes.default.svc:443
---
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateTarget
metadata:
  name: k8s-local
  namespace: warpgate-operator
spec:
  connectionRef: my-warpgate
  name: k8s-local
  kubernetes:
    clusterURL: https://kubernetes.default.svc:443
    authKind: Token
    serviceAccountToken:
      name: warpgate-k8s-local
      audiences: [https://kubernetes.default.svc.cluster.local]
    tls:
      mode: Required
      verify: false   # Warpgate checks public roots only; the cluster CA is private
```

Warpgate has no CA option for Kubernetes targets, so an API server with the cluster's own CA needs
`verify: false`. Here the connection stays inside the cluster network.

Bind the ServiceAccount to the access it should have, for example
`kubectl create clusterrolebinding warpgate-k8s-local --clusterrole=view --serviceaccount=warpgate-operator:warpgate-k8s-local`
(or the same in Git).

## 2b. Target: Another Cluster

The target cluster must accept JWTs from the operator cluster's ServiceAccount issuer. This uses structured
authentication (`AuthenticationConfiguration`, GA since Kubernetes 1.34).

### Give the operator cluster a unique issuer

Kubernetes refuses a JWT authenticator whose issuer matches one of its own ServiceAccount issuers. Distributions
that default every cluster to `https://kubernetes.default.svc.cluster.local` (k3s, kubeadm) therefore collide. On
the operator cluster's API servers:

```text
--service-account-issuer=https://k8s-oidc.example.com/cluster-a            # first: signs new tokens
--service-account-issuer=https://kubernetes.default.svc.cluster.local      # old: still accepted
--service-account-jwks-uri=https://k8s-oidc.example.com/cluster-a/openid/v1/jwks
--api-audiences=https://kubernetes.default.svc.cluster.local               # keep what it was
```

- **Keep the old issuer listed** so existing tokens stay valid while kubelet refreshes projected tokens (within
  about an hour).
- **Set `--api-audiences` explicitly.** Unset, it defaults to the issuer list, which would quietly change what
  in-cluster tokens are accepted for.
- **On k3s**, pass these as `kube-apiserver-arg` in a `config.yaml.d` drop-in. On Talos, use
  `cluster.apiServer.extraArgs`. Roll it out one control-plane node at a time.

### Publish the issuer's public keys

The target cluster's API server fetches `<issuer>/.well-known/openid-configuration` and the JWKS it points to.
They contain public keys only. Most API servers don't serve them anonymously (k3s answers 401), so serve copies:
for example a small Deployment that fetches both documents with its own ServiceAccount token (allowed by the
default `system:service-account-issuer-discovery` binding) and serves them behind an Ingress with a trusted
certificate at the issuer URL.

### Trust the issuer on the target cluster

`/etc/kubernetes/authn-config.yaml` on each of its API servers, passed with `--authentication-config`:

```yaml
apiVersion: apiserver.config.k8s.io/v1
kind: AuthenticationConfiguration
jwt:
  - issuer:
      url: https://k8s-oidc.example.com/cluster-a
      audiences: [zone10-drinking]
    claimValidationRules:
      # Accept exactly this ServiceAccount, nothing else the issuer signs.
      - expression: 'claims.sub == "system:serviceaccount:warpgate-operator:warpgate-k8s-drinking"'
        message: only Warpgate's ServiceAccount for this cluster
      # Optional: pin the ServiceAccount UID, so a recreated ServiceAccount is not trusted until updated here.
      # - expression: 'claims["kubernetes.io"].serviceaccount.uid == "<uid>"'
      #   message: unexpected ServiceAccount UID
    claimMappings:
      username:
        expression: '"cluster-a:" + claims.sub'
      groups:
        expression: '["cluster-a:warpgate"]'
```

- The username prefix keeps the identity apart from the target cluster's own ServiceAccounts.
- The API server reloads this file when it changes. The first `--authentication-config` flag needs a restart.
- It must be able to reach the issuer URL over HTTPS.

Then RBAC on the target cluster:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: warpgate-cluster-a-admin
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
  - apiGroup: rbac.authorization.k8s.io
    kind: User
    name: cluster-a:system:serviceaccount:warpgate-operator:warpgate-k8s-drinking
```

And the target on the operator cluster:

```yaml
apiVersion: warpgate.warpgate.warp.tech/v1alpha1
kind: WarpgateTarget
metadata:
  name: k8s-drinking
  namespace: warpgate-operator
spec:
  connectionRef: my-warpgate
  name: k8s-drinking
  kubernetes:
    clusterURL: https://192.168.42.49:6443
    authKind: Token
    serviceAccountToken:
      name: warpgate-k8s-drinking
      audiences: [zone10-drinking]
```

Warpgate must be able to reach `clusterURL`. Warpgate has no CA option for Kubernetes targets: a cluster with a
private CA needs `tls.verify: false`, or a front end with a publicly trusted certificate.

## 3. Check It

```bash
kubectl -n warpgate-operator get warpgatetarget k8s-drinking \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")].reason}{" "}{.status.serviceAccountToken}{"\n"}'
```

Then, with the kubeconfig from Warpgate's access instructions:

```bash
kubectl auth whoami   # cluster-a:system:serviceaccount:warpgate-operator:warpgate-k8s-drinking
kubectl get nodes
```

## Renewal and Warpgate Sessions

- **Renewal timing.** The operator renews at half the lifetime the API server granted. The API server may shorten
  the requested one (`--service-account-max-token-expiration`). The target is requeued for that time even with a
  long reconcile interval.
- **What Warpgate does with a new token.** Warpgate reads the token when a Kubernetes session opens and keeps it for
  that session, at most `kubernetes.session_max_age` (30 minutes by default). It also reuses its upstream client
  for at most 5 minutes while the target is unchanged. A renewed token reaches new sessions at once, and existing
  ones within `session_max_age`.
- **Lifetime rule.** Keep `expirationSeconds` above four times `session_max_age`, which leaves twice
  `session_max_age` after a renewal. The minimum of 7200 covers the default.
- **When renewal fails** (the TokenRequest, or the Warpgate update that delivers the new token): the target goes
  `Ready=False` and is retried within a minute. Warpgate keeps the previous token until it expires.
- **When the operator restarts or fails over.** It requests a new token. The previous one expires on its own.

## Revoking Access

| Goal | Do |
|------|----|
| Cut access on a remote cluster now | Delete its RoleBinding, or remove the issuer from its `AuthenticationConfiguration` (reloaded live) |
| Cut access on the operator's cluster now | Delete or recreate the ServiceAccount (its tokens stop validating locally) |
| Stop new tokens | Remove an annotation, the Helm `serviceAccountTokens.allowed` entry, or the WarpgateTarget |

Already-issued tokens can't be revoked one by one. On a remote cluster that doesn't check bindings they stay valid
until they expire, which is why lifetimes are short and RBAC on the target cluster is the real off switch.

## Security Model

| Risk | Mitigation |
|------|------------|
| A WarpgateTarget makes the operator mint tokens for any ServiceAccount | Only those named in a Role with `resourceNames`. None by default. (A compromised operator can still create Secrets and Deployments cluster-wide; this guard is about what WarpgateTargets can trigger, not about containing the operator) |
| Someone who can create WarpgateTargets gets tokens for unrelated ServiceAccounts in their namespace | The ServiceAccount must opt in with both annotations. The namespace is the boundary: see "Put these ServiceAccounts in a namespace only administrators can write to" |
| A WarpgateTarget sends a valid token to a server its editor controls | `clusterURL` must be listed in `token-cluster-urls`. A URL change also gets a fresh token, never the cached one |
| A token usable against the operator's own cluster by accident | `audiences` is required, and each audience must be allowed by the annotation |
| A man in the middle on the way to the cluster | Use `tls.verify: true` where the cluster has a publicly trusted certificate. With `verify: false`, keep the path on a trusted network or tunnel |
| The token leaks via the CR, logs or errors | Status holds timestamps only. Errors from the Warpgate API (which include response bodies) are redacted before they reach a condition or a log |
| A stolen token | It expires within its lifetime (default 2h). The target cluster's JWT rules accept only this ServiceAccount, and its RBAC decides what it can do |
| A remote cluster trusts too much | `claimValidationRules` pin the subject, optionally the UID. Prefixed usernames keep it apart from local identities |

## Troubleshooting

| `Ready` reason | Fix |
|----------------|-----|
| `ServiceAccountNotFound` | Create the ServiceAccount in the target's namespace |
| `ServiceAccountNotOptedIn` | Add `warpgate.warp.tech/token-audiences` and `warpgate.warp.tech/token-cluster-urls` to the ServiceAccount |
| `TokenAudienceNotAllowed` | Add the missing audiences to `token-audiences` (comma-separated, exact match) |
| `TokenClusterURLNotAllowed` | Add the target's `clusterURL`, exactly as written in the spec, to `token-cluster-urls` |
| `TokenRequestForbidden` | Add the ServiceAccount to `serviceAccountTokens.allowed`, or apply the sample Role |
| `TokenRequestError` | Check the message. API errors are retried within a minute |
| `Ready=True` but `kubectl` gets 401 | Usually the target cluster: issuer URL, audience, JWKS reachability or `claimValidationRules`. Its API server logs say which |
