# warpgate-operator justfile
# Run `just` to see all available recipes

set shell := ["bash", "-euo", "pipefail", "-c"]

# Configuration
img := env("IMG", "controller:latest")
container_tool := env("CONTAINER_TOOL", "podman")
platforms := "linux/arm64,linux/amd64"
localbin := justfile_directory() / "bin"

# Minikube settings
minikube_profile := env("MINIKUBE_PROFILE", "warpgate-operator")
minikube_cpus := env("MINIKUBE_CPUS", "2")
minikube_memory := env("MINIKUBE_MEMORY", "4096")
minikube_k8s_version := env("MINIKUBE_K8S_VERSION", "stable")

# Tool versions
kustomize_version := "v5.8.1"
controller_tools_version := "v0.20.1"
golangci_lint_version := "v2.8.0"

# Tool paths
kustomize := localbin / "kustomize"
controller_gen := localbin / "controller-gen"
envtest := localbin / "setup-envtest"
golangci_lint := localbin / "golangci-lint"

# Derived versions from go.mod
envtest_version := `v=$(go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' sigs.k8s.io/controller-runtime 2>/dev/null); printf '%s' "$v" | sed -E 's/^v?([0-9]+)\.([0-9]+).*/release-\1.\2/'`
envtest_k8s_version := `v=$(go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' k8s.io/api 2>/dev/null); printf '%s' "$v" | sed -E 's/^v?[0-9]+\.([0-9]+).*/1.\1/'`

# Default recipe
default: build

# Run all checks (lint + test) — use before pushing
check: lint test

# ─── General ──────────────────────────────────────────────────────────

# Show all available recipes
help:
    @just --list

# ─── Development ──────────────────────────────────────────────────────

# Generate CRD manifests, RBAC, and webhook configs
manifests: _controller-gen
    "{{controller_gen}}" rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Generate DeepCopy and related boilerplate
generate: _controller-gen
    "{{controller_gen}}" object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Run go fmt
fmt:
    go fmt ./...

# Run go vet
vet:
    go vet ./...

# Run unit tests (not e2e)
test: manifests generate fmt vet _setup-envtest
    KUBEBUILDER_ASSETS="$("{{envtest}}" use {{envtest_k8s_version}} --bin-dir "{{localbin}}" -p path)" \
        go test $(go list ./... | grep -v /e2e) -coverprofile cover.out

# ─── Build ────────────────────────────────────────────────────────────

# Build the operator binary
build: manifests generate fmt vet
    go build -o bin/manager cmd/main.go

# Run the operator locally against current kubeconfig
run: manifests generate fmt vet
    go run ./cmd/main.go

# Build container image
image-build:
    {{container_tool}} build -t {{img}} -f Containerfile .

# Push container image
image-push:
    {{container_tool}} push {{img}}

# Build and push multi-platform image
image-buildx:
    sed -e '1 s/\(^FROM\)/FROM --platform=$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=$\{BUILDPLATFORM\}/' Containerfile > Containerfile.cross
    -{{container_tool}} buildx create --name warpgate-operator-builder
    {{container_tool}} buildx use warpgate-operator-builder
    -{{container_tool}} buildx build --push --platform={{platforms}} --tag {{img}} -f Containerfile.cross .
    -{{container_tool}} buildx rm warpgate-operator-builder
    rm -f Containerfile.cross

# Generate consolidated install YAML
build-installer: manifests generate _kustomize
    mkdir -p dist
    cd config/manager && "{{kustomize}}" edit set image controller={{img}}
    "{{kustomize}}" build config/default > dist/install.yaml

# ─── Deployment ───────────────────────────────────────────────────────

# Install CRDs into the current cluster
install: manifests _kustomize
    @out="$("{{kustomize}}" build config/crd 2>/dev/null || true)"; \
    if [ -n "$out" ]; then echo "$out" | kubectl apply -f -; else echo "No CRDs to install; skipping."; fi

# Uninstall CRDs from the current cluster
uninstall: manifests _kustomize
    @out="$("{{kustomize}}" build config/crd 2>/dev/null || true)"; \
    if [ -n "$out" ]; then echo "$out" | kubectl delete --ignore-not-found -f -; else echo "No CRDs to delete; skipping."; fi

# Deploy the operator to the current cluster
deploy: manifests _kustomize
    cd config/manager && "{{kustomize}}" edit set image controller={{img}}
    "{{kustomize}}" build config/default | kubectl apply -f -

# Remove the operator from the current cluster
undeploy: _kustomize
    "{{kustomize}}" build config/default | kubectl delete --ignore-not-found -f -

# ─── Minikube (podman) ───────────────────────────────────────────────

# Start a minikube cluster with podman driver
minikube-up:
    #!/usr/bin/env bash
    set -euo pipefail
    if minikube status -p {{minikube_profile}} &>/dev/null; then
        echo "Minikube profile '{{minikube_profile}}' is already running."
    else
        echo "Starting minikube profile '{{minikube_profile}}' with podman driver..."
        minikube start \
            --profile {{minikube_profile}} \
            --driver=podman \
            --container-runtime=containerd \
            --cpus={{minikube_cpus}} \
            --memory={{minikube_memory}} \
            --kubernetes-version={{minikube_k8s_version}} \
            --addons=default-storageclass,storage-provisioner
        echo "Minikube is ready. kubectl context set to '{{minikube_profile}}'."
    fi

# Stop the minikube cluster (preserves state)
minikube-stop:
    minikube stop -p {{minikube_profile}}

# Destroy the minikube cluster completely
minikube-destroy:
    minikube delete -p {{minikube_profile}}

# Show minikube cluster status
minikube-status:
    minikube status -p {{minikube_profile}}

# Open the Kubernetes dashboard
minikube-dashboard:
    minikube dashboard -p {{minikube_profile}}

# Build image and load it into minikube (no push needed)
minikube-load: image-build
    minikube image load {{img}} -p {{minikube_profile}}

# Full dev cycle: start minikube, build, load image, deploy
minikube-deploy: minikube-up minikube-load install deploy

# Tear down: undeploy, uninstall CRDs, destroy cluster
minikube-teardown: undeploy uninstall minikube-destroy

# Show operator logs from minikube
minikube-logs:
    kubectl logs -n warpgate-operator-system -l control-plane=controller-manager -f --tail=100

# ─── Linting ─────────────────────────────────────────────────────────

# Run Go linter (golangci-lint)
lint-go: _golangci-lint
    "{{golangci_lint}}" run

# Run Go linter with auto-fix
lint-go-fix: _golangci-lint
    "{{golangci_lint}}" run --fix

# Lint Markdown files
lint-md:
    npx --yes markdownlint-cli2 "**/*.md" "#node_modules"

# Lint YAML files
lint-yaml:
    npx --yes yamllint -c .yamllint.yaml $(find . -name '*.yaml' -o -name '*.yml' | grep -v node_modules | grep -v bin/ | grep -v charts/warpgate-operator/templates)

# Lint Helm chart
lint-helm:
    helm lint charts/warpgate-operator

# Lint commit messages (checks last commit)
lint-commit:
    npx --yes @commitlint/cli --from HEAD~1 --to HEAD --config .commitlintrc.yaml

# Validate generated manifests are up to date
lint-manifests: manifests generate
    git diff --exit-code config/ api/

# Run all linters
lint: lint-go lint-md lint-yaml lint-helm lint-manifests

# Run all linters with auto-fix where possible
lint-fix: lint-go-fix
    npx --yes markdownlint-cli2 --fix "**/*.md" "#node_modules"

# ─── E2E Testing ─────────────────────────────────────────────────────

# Run e2e tests against minikube
test-e2e: minikube-up manifests generate fmt vet
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Building and loading operator image into minikube..."
    {{container_tool}} build -t {{img}} -f Containerfile .
    minikube image load {{img}} -p {{minikube_profile}}
    echo "Running e2e tests..."
    MINIKUBE_PROFILE={{minikube_profile}} go test -tags=e2e ./test/e2e/ -v -ginkgo.v

# ─── Setup ───────────────────────────────────────────────────────────

# Install pre-commit hooks for conventional commits and linting
setup-hooks:
    pip install --user pre-commit || pip3 install --user pre-commit
    pre-commit install --hook-type commit-msg --hook-type pre-push --hook-type pre-commit

# Install all development dependencies
setup: setup-hooks _golangci-lint _controller-gen _kustomize
    @echo "Development environment ready."

# ─── Tool Dependencies (private) ─────────────────────────────────────

[private]
_kustomize:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p "{{localbin}}"
    if [ ! -f "{{kustomize}}-{{kustomize_version}}" ] || [ "$(readlink -- "{{kustomize}}" 2>/dev/null)" != "{{kustomize}}-{{kustomize_version}}" ]; then
        echo "Downloading kustomize {{kustomize_version}}"
        rm -f "{{kustomize}}"
        GOBIN="{{localbin}}" go install sigs.k8s.io/kustomize/kustomize/v5@{{kustomize_version}}
        mv "{{localbin}}/kustomize" "{{kustomize}}-{{kustomize_version}}"
    fi
    ln -sf "$(realpath "{{kustomize}}-{{kustomize_version}}")" "{{kustomize}}"

[private]
_controller-gen:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p "{{localbin}}"
    if [ ! -f "{{controller_gen}}-{{controller_tools_version}}" ] || [ "$(readlink -- "{{controller_gen}}" 2>/dev/null)" != "{{controller_gen}}-{{controller_tools_version}}" ]; then
        echo "Downloading controller-gen {{controller_tools_version}}"
        rm -f "{{controller_gen}}"
        GOBIN="{{localbin}}" go install sigs.k8s.io/controller-tools/cmd/controller-gen@{{controller_tools_version}}
        mv "{{localbin}}/controller-gen" "{{controller_gen}}-{{controller_tools_version}}"
    fi
    ln -sf "$(realpath "{{controller_gen}}-{{controller_tools_version}}")" "{{controller_gen}}"

[private]
_envtest:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p "{{localbin}}"
    if [ ! -f "{{envtest}}-{{envtest_version}}" ] || [ "$(readlink -- "{{envtest}}" 2>/dev/null)" != "{{envtest}}-{{envtest_version}}" ]; then
        echo "Downloading setup-envtest {{envtest_version}}"
        rm -f "{{envtest}}"
        GOBIN="{{localbin}}" go install sigs.k8s.io/controller-runtime/tools/setup-envtest@{{envtest_version}}
        mv "{{localbin}}/setup-envtest" "{{envtest}}-{{envtest_version}}"
    fi
    ln -sf "$(realpath "{{envtest}}-{{envtest_version}}")" "{{envtest}}"

[private]
_setup-envtest: _envtest
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Setting up envtest binaries for Kubernetes version {{envtest_k8s_version}}..."
    "{{envtest}}" use {{envtest_k8s_version}} --bin-dir "{{localbin}}" -p path || {
        echo "Error: Failed to set up envtest binaries for version {{envtest_k8s_version}}."
        exit 1
    }

[private]
_golangci-lint:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p "{{localbin}}"
    if [ ! -f "{{golangci_lint}}-{{golangci_lint_version}}" ] || [ "$(readlink -- "{{golangci_lint}}" 2>/dev/null)" != "{{golangci_lint}}-{{golangci_lint_version}}" ]; then
        echo "Downloading golangci-lint {{golangci_lint_version}}"
        rm -f "{{golangci_lint}}"
        GOBIN="{{localbin}}" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@{{golangci_lint_version}}
        mv "{{localbin}}/golangci-lint" "{{golangci_lint}}-{{golangci_lint_version}}"
    fi
    ln -sf "$(realpath "{{golangci_lint}}-{{golangci_lint_version}}")" "{{golangci_lint}}"
    if [ -f .custom-gcl.yml ]; then
        echo "Building custom golangci-lint with plugins..."
        "{{golangci_lint}}" custom --destination "{{localbin}}" --name golangci-lint-custom
        mv -f "{{localbin}}/golangci-lint-custom" "{{golangci_lint}}"
    fi
