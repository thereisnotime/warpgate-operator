# Tasks

- [x] Parse `--secret-selector` and derive the labels for created Secrets
- [x] Limit the Secret informer with `cache.Options.ByObject`
- [x] Wrap the manager client: label written Secrets, report Secrets hidden by the selector
- [x] Report `SecretNotSelected` on resource conditions
- [x] Delete user and ticket Secrets by name in finalizers
- [x] Helm value, deployment arg and `httproutes` RBAC rule
- [x] Unit tests for the scope package and envtest for the cache and conditions
- [x] Update README, chart README and living spec
