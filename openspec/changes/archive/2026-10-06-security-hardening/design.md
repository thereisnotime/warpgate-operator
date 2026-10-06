# Design

## Database URL (init script and ConfigMap)

Warpgate's config loader only reads the YAML file (no environment source), so `database_url` has to end up in `/data/warpgate.yaml`. The ConfigMap now omits it whenever an external database is configured. The init container gets `DATABASE_URL` (from `databaseURLSecretRef` via `secretKeyRef`, or the deprecated literal) and, after copying the ConfigMap file, deletes any `database_url:` line and appends `database_url: '<value>'` with single quotes doubled. Single-quoted YAML needs no other escaping, and the value only ever flows through `printf`/`sed` as data.

`--database-url "${DATABASE_URL}"` replaces the interpolated flag in `unattended-setup`, matching how `ADMIN_PASSWORD` was already handled.

The init script text is folded into the pod template hash. Without that, existing Deployments would keep the old script while the ConfigMap lost `database_url`, and the next restart would fall back to SQLite.

`databaseURL` keeps working for backward compatibility. The webhook warns that it is deprecated, and that it is ignored when `databaseURLSecretRef` is also set. Rotating the database Secret takes effect on the next pod restart; the operator does not restart pods for Secret changes.

## Target TLS verify

`Verify` becomes `*bool` with `+kubebuilder:default=true`, the webhook defaulter sets it for any present `tls` block, and `toWarpgateTLS` maps `nil` to `true`. All three layers agree so objects created before the CRD upgrade (stored without the field) also verify.

## Password rotation

Warpgate's API has no update for password credentials. The controller records `<secret>/<key>@<resourceVersion>` in `status.appliedSecretVersion`. When the current value differs (or the resolved user changed), it deletes the old credential and then creates a new one. Delete-first means the old password never outlives the rotation; the cost is a brief window with no password credential. If the delete fails, the old ID is kept and `Ready=False/RotateFailed` is reported.

The resourceVersion is used instead of a password hash so no derivative of the secret lands in the CR status. Credentials without a recorded version (created by older operator versions) are re-applied once after upgrade.

A `Watches(&corev1.Secret{}, EnqueueRequestsFromMapFunc(...))` lists credentials in the Secret's namespace and enqueues those whose `passwordSecretRef.name` matches. The manager already caches Secrets for the other controllers, so this adds no new informer.

## Auto-created connection TLS

`WarpgateConnection.spec.caSecretRef` points to a PEM bundle used as `RootCAs`. Both client builders (connection controller and shared helper) load it and reject a Secret without a PEM certificate.

For a `WarpgateInstance`:

| `tls.secretName` | `tls.verifyConnection` | Result |
|---|---|---|
| set | unset / true | verify with `caSecretRef` (`ca.crt`, else `tls.crt`) |
| set | false | skip verification |
| unset | unset / false | skip verification (pod-generated self-signed cert) |
| unset | true | reconcile error, no connection |

`tls.certManager` on its own does not mount the cert-manager Secret into the pod (the pod serves Warpgate's own self-signed certificate), so it falls in the "unset" rows. Users who want the cert-manager certificate served and verified set `tls.secretName: <name>-tls`.
