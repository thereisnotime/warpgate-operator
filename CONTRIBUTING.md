# Contributing

Thanks for your interest in contributing to the Warpgate Operator. This guide covers everything you need to get a development environment running and submit changes.

## Getting Started

### Prerequisites

- [Go](https://go.dev/dl/) 1.26+
- [just](https://github.com/casey/just) (task runner -- replaces Make for most workflows)
- [Podman](https://podman.io/) (container runtime)
- [minikube](https://minikube.sigs.k8s.io/) (local Kubernetes cluster)
- [Helm](https://helm.sh/) (chart linting and templating)
- [pre-commit](https://pre-commit.com/) (git hook management)
- Node.js/npx (for markdownlint and commitlint)

## Setup

```bash
git clone git@github.com:thereisnotime/warpgate-operator.git
cd warpgate-operator
just setup    # installs pre-commit hooks and downloads Go tooling
just build    # build the operator binary to verify everything works
```

## Development Workflow

### Commands

All development tasks are driven through `just`. Run `just` with no arguments to see every available recipe.

| Recipe | Description |
|--------|-------------|
| `just build` | Build the operator binary |
| `just run` | Run the operator locally against current kubeconfig |
| `just test` | Run unit tests |
| `just test-e2e` | Run E2E tests against minikube |
| `just manifests` | Generate CRD manifests and RBAC |
| `just generate` | Generate DeepCopy boilerplate |
| `just fmt` | Run `go fmt` |
| `just vet` | Run `go vet` |
| `just check` | Run all linters and tests (use before pushing) |
| `just lint` | Run all linters (Go, Markdown, YAML, Helm, manifests) |
| `just lint-fix` | Run linters with auto-fix where possible |
| `just lint-go` | Run golangci-lint only |
| `just lint-md` | Run markdownlint only |
| `just lint-yaml` | Run yamllint only |
| `just lint-helm` | Lint the Helm chart |
| `just lint-commit` | Validate the last commit message |

### Local Testing with Minikube

The justfile includes recipes for a full local dev loop using minikube with the podman driver.

```bash
just minikube-up          # start cluster
just minikube-deploy      # full cycle: start cluster + build + load image + install CRDs + deploy
just minikube-logs        # tail operator logs
just minikube-status      # show cluster status
just minikube-teardown    # undeploy + uninstall CRDs + destroy cluster
```

| Variable | Default | Description |
|----------|---------|-------------|
| `MINIKUBE_PROFILE` | `warpgate-operator` | Minikube profile name |
| `MINIKUBE_CPUS` | `2` | CPU allocation |
| `MINIKUBE_MEMORY` | `4096` | Memory allocation (MB) |
| `MINIKUBE_K8S_VERSION` | `stable` | Kubernetes version |

### Running Tests

```bash
just test       # unit tests with envtest (generates manifests, runs fmt/vet first)
just test-e2e   # end-to-end tests against a minikube cluster
```

## Building and Deploying

```bash
just image-build IMG=ghcr.io/thereisnotime/warpgate-operator:latest
just image-push IMG=ghcr.io/thereisnotime/warpgate-operator:latest
just deploy IMG=ghcr.io/thereisnotime/warpgate-operator:latest
just build-installer IMG=ghcr.io/thereisnotime/warpgate-operator:latest  # outputs dist/install.yaml
```

## Code Style

### Conventional Commits

This project uses [conventional commits](https://www.conventionalcommits.org/). Every commit message must follow the format:

```text
<type>(optional scope): <description>
```

Allowed types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`

Examples:

```text
feat(crd): add WarpgateRole custom resource
fix(controller): handle nil pointer on missing secret ref
docs: update quickstart guide
chore(deps): bump controller-runtime to v0.20
```

### Go Linting

golangci-lint is downloaded automatically. Run `just lint-go` to check or `just lint-go-fix` to auto-fix.

### Markdown Linting

Markdown files are linted with markdownlint-cli2. Configuration lives in `.markdownlint.yaml` and `.markdownlint-cli2.yaml`. Run `just lint-md` to check.

## Project Structure

```text
api/                    # CRD Go types (grouped by version)
cmd/                    # Operator entrypoint
config/
  crd/                  # Generated CRD manifests
  rbac/                 # RBAC role and bindings
  manager/              # Operator deployment manifests
  default/              # Kustomize overlay combining everything
  samples/              # Example CRs
charts/                 # Helm chart
docs/crds/              # Per-CRD reference documentation
internal/
  controller/           # Reconciler implementations
  warpgate/             # Warpgate REST API client
hack/                   # Boilerplate headers and helper scripts
test/e2e/               # End-to-end tests
```

## Submitting Changes

1. Fork the repo and create a feature branch from `main`.
2. Make your changes, keeping commits focused and using conventional commit messages.
3. Run `just check` to make sure linting and tests pass locally.
4. Open a pull request with a conventional commit style title (e.g. `feat(crd): add SSH target resource`).
5. CI will run the full lint and test suite -- make sure it passes before requesting review.
6. Keep PRs focused -- one logical change per PR when possible.
