# Design

## Workflow Architecture

Five GitHub Actions workflows, each with a focused responsibility:

| Workflow | Trigger | Purpose |
|---|---|---|
| `ci.yml` | Push/PR to main | Lint, unit test, build, coverage upload |
| `e2e.yml` | Push/PR to main | Spin up kind cluster, deploy operator, run E2E suite |
| `helm-test.yml` | Push/PR to main | Lint and template-test the Helm chart |
| `security.yml` | Push/PR to main + scheduled | Static analysis and vulnerability scanning |
| `release.yml` | Merge to main | Semantic version bump, container build, Helm publish |

All workflows share a common Go version matrix and caching strategy via `actions/setup-go` with module caching.

## Tool Choices

**Linting:**
- `golangci-lint` — aggregated Go linter (staticcheck, errcheck, gosimple, etc.)
- `markdownlint-cli` — documentation quality
- `yamllint` — YAML hygiene for manifests and Helm templates

**Security:**
- `gosec` — Go-specific security scanner (SQL injection, hardcoded creds, etc.)
- `govulncheck` — checks Go dependencies against the vulnerability database
- `trivy` — container image scanning for OS and language-level CVEs
- `gitleaks` — prevents secrets from being committed

**Testing:**
- `go test` with `-race -coverprofile` for unit/integration tests
- `chainsaw` or custom scripts for E2E against a kind cluster

## Release Pipeline

```
merge to main
  → release-please creates/updates release PR (bumps version, changelog)
    → merge release PR
      → tag + GitHub Release
      → docker build + push to ghcr.io
      → helm package + push to OCI registry
      → generate install.yaml static manifest
```

release-please handles version bumps based on conventional commit prefixes:
- `feat:` → minor bump
- `fix:` → patch bump
- `feat!:` or `BREAKING CHANGE:` → major bump

## Branch Protection

Rules applied to `main`:
- Require status checks to pass (ci, security, helm-test)
- Require PR reviews (1 approver)
- Squash merges only — keeps history linear and clean
- Require linear history
- No force pushes
- Require conventional commit format on PR titles

## Pre-commit Hooks

Local enforcement via pre-commit framework:
- `golangci-lint` on staged `.go` files
- `gitleaks` to catch secrets before they hit remote
- `commitlint` to enforce conventional commit messages
- `yamllint` on YAML files
- `markdownlint` on documentation

These mirror the CI checks so developers catch issues before pushing.
