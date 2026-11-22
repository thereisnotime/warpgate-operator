# Design

## Cache

`scope.ParseSecrets` parses the selector and returns `ByObject{&corev1.Secret{}: {Label: sel}}`. Namespaces are left nil so the namespace scope (`DefaultNamespaces`) still applies. Owns/Watches on Secrets, including the password credential rotation watch, go through the same informer, so they only see matching Secrets too.

## Labels for created Secrets

The selector must be satisfiable by a fixed label set: `=`/`==`/`in` contribute their (first) value, a bare `key` contributes `key=true`, negative terms contribute nothing. The derived set must match the selector, otherwise startup fails. A wrapper around the manager client adds the set on Create/Update/Patch of Secrets, which covers the user password, ticket and instance admin auth Secrets without touching each controller.

## Not visible vs missing

A cached Get for an unlabelled Secret returns NotFound, which would look like a typo in the reference. On NotFound the wrapper does a metadata-only Get through the API reader (`PartialObjectMetadata`, so the Secret's data is never fetched). If the Secret exists and doesn't match, it returns `NotSelectedError`, a NotFound `StatusError` with a message naming the Secret and the labels to add. Keeping it NotFound means existing `IsNotFound` handling is unchanged. `secretErrorReason` maps it to the `SecretNotSelected` reason on the conditions that report Secret errors.

The extra API call only happens on the error path.

## Deletion

User and ticket finalizers used to Get the Secret and delete it only if found. With the selector, legacy unlabelled Secrets would leak, so they now delete by name and ignore NotFound.

## RBAC

RBAC can't grant by label, so the selector is not a security boundary against a compromised ServiceAccount; the namespace scope (RoleBindings per namespace, already in the chart) is. Splitting the ClusterRole into per-namespace Roles was not needed: a RoleBinding to a ClusterRole only grants inside that namespace.
