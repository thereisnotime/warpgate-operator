# Design

## Framework: Kubebuilder

Kubebuilder was chosen over alternatives like Operator SDK (which wraps Kubebuilder anyway) or raw controller-runtime. It gives us CRD scaffolding, RBAC generation from marker comments, deepcopy codegen, and a well-structured project layout out of the box. The generated Makefile handles manifest generation, testing, and container builds with minimal customization needed.

## Multi-Instance via WarpgateConnection CRD

Rather than configuring a single Warpgate instance per operator deployment (via env vars or a ConfigMap), the operator uses a dedicated `WarpgateConnection` CRD. Each connection resource points to a different Warpgate instance with its own URL and credential secret reference. All other CRDs reference a connection by name, so a single operator deployment can manage resources across multiple Warpgate instances. This avoids the need to deploy separate operator instances per Warpgate server.

## REST API Client (Not DB Access)

The operator communicates with Warpgate through its REST API using session-based authentication (username/password), the same interface the Terraform provider uses. This keeps the operator decoupled from Warpgate internals -- no need to understand the database schema, handle migrations, or require direct database connectivity. The API client implements full CRUD for all resource types and is tested independently with 100% coverage using HTTP test servers.

## Secret References (Not Inline)

Sensitive fields like credentials, user passwords, and SSH keys are never stored directly in CRD specs. Instead, specs contain references to Kubernetes Secrets. The controller resolves these at reconciliation time. This keeps secrets out of etcd CRD storage, works naturally with external secret operators (e.g., External Secrets, Sealed Secrets), and follows Kubernetes conventions.

## Finalizer-Based Cleanup

Every CRD uses a finalizer (`warpgate.warp.tech/finalizer`) to ensure the corresponding Warpgate resource is deleted when the CR is removed. The reconciler adds the finalizer on first create, and on deletion it calls the Warpgate API to remove the resource before clearing the finalizer. This prevents orphaned resources in Warpgate when CRs are deleted.

## Drift Reconciliation

Controllers reconcile on a 5-minute requeue interval in addition to watch-triggered reconciliation. On each pass, the controller reads the current state from the Warpgate API and compares it to the desired spec. If they differ (manual changes in the Warpgate UI, API drift, etc.), the controller overwrites the remote state to match the CRD. This is the standard Kubernetes pattern -- the CRD is the source of truth.

## Auto-Generated Passwords

The WarpgateUser controller generates a random password using `crypto/rand` when a user is created without an explicit password secret reference. The generated password is stored in a Kubernetes Secret in the same namespace as the user CR. This makes it easy to bootstrap users without pre-creating secrets, while still keeping passwords out of the CRD spec.

## Helm Chart with CRDs in templates/

The Helm chart places CRDs under `templates/` rather than the conventional `crds/` directory. Helm's `crds/` directory has a significant limitation: CRDs placed there are only installed on first `helm install` and never updated on `helm upgrade`. Putting them in `templates/` means CRD schema changes are applied on every upgrade, which is important for an evolving operator. The trade-off is that `helm uninstall` will also remove the CRDs (and all CRs), which is acceptable for this use case and actually desirable for clean teardown.

## CI/CD Architecture

The CI pipeline uses GitHub Actions with several layers:

- **Linting:** golangci-lint for Go code, commitlint for conventional commit enforcement
- **Testing:** Unit tests with race detection and coverage reporting to Codecov, targeting 85%+ controller coverage
- **Security:** gosec for static analysis, govulncheck for known vulnerabilities, trivy for container image scanning, gitleaks for secret detection
- **Release:** release-please automates semantic versioning based on conventional commits, generating changelogs and creating GitHub releases with built container images
- **E2E:** Integration tests run against a real Kubernetes cluster (kind) with a mocked Warpgate API

Branch protection on `main` requires passing CI checks and at least one review before merge.
