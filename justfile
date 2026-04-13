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
gosec_version := "v2.25.0"
govulncheck_version := "v1.1.4"

# Build-time version info (injected via ldflags)
version := `git describe --tags --always --dirty 2>/dev/null || echo "dev"`
commit := `git rev-parse --short HEAD 2>/dev/null || echo "none"`
date := `date -u +%Y-%m-%dT%H:%M:%SZ`
ldflags := "-s -w -X github.com/thereisnotime/warpgate-operator/internal/version.Version=" + version + " -X github.com/thereisnotime/warpgate-operator/internal/version.Commit=" + commit + " -X github.com/thereisnotime/warpgate-operator/internal/version.Date=" + date

# Tool paths
kustomize := localbin / "kustomize"
controller_gen := localbin / "controller-gen"
envtest := localbin / "setup-envtest"
golangci_lint := localbin / "golangci-lint"

# Derived versions from go.mod
envtest_version := `v=$(go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' sigs.k8s.io/controller-runtime 2>/dev/null); printf '%s' "$v" | sed -E 's/^v?([0-9]+)\.([0-9]+).*/release-\1.\2/'`
envtest_k8s_version := `v=$(go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' k8s.io/api 2>/dev/null); printf '%s' "$v" | sed -E 's/^v?[0-9]+\.([0-9]+).*/1.\1/'`

# Default recipe
default: help

# Run all checks (lint + test) — use before pushing
check: lint test

# ─── General ──────────────────────────────────────────────────────────

# Show all available recipes
help:
    @just --list

# ─── Development ──────────────────────────────────────────────────────

# Generate CRD manifests, RBAC, webhook configs, and sync Helm chart CRDs
manifests: _controller-gen
    "{{controller_gen}}" rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases
    just _sync-chart-crds

# Sync generated CRDs into the Helm chart templates
[private]
_sync-chart-crds:
    #!/usr/bin/env bash
    set -euo pipefail
    for crd in config/crd/bases/*.yaml; do
        name=$(basename "$crd" | sed 's/warpgate.warpgate.warp.tech_//')
        chart_file="charts/warpgate-operator/templates/crds/${name}"
        echo '{''{'- if .Values.crds.install -'}''}' > "$chart_file"
        cat "$crd" >> "$chart_file"
        echo '{''{'- end '}''}' >> "$chart_file"
    done
    echo "Synced $(ls config/crd/bases/*.yaml | wc -l) CRDs to Helm chart."

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
        go test -race -shuffle=on -covermode=atomic $(go list ./... | grep -v /e2e) -coverprofile cover.out

# ─── Build ────────────────────────────────────────────────────────────

# Build the operator binary
build: manifests generate fmt vet
    go build -ldflags "{{ldflags}}" -o bin/manager cmd/main.go

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
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Loading image into minikube..."
    podman save {{img}} -o /tmp/warpgate-operator-image.tar
    minikube image load /tmp/warpgate-operator-image.tar -p {{minikube_profile}}
    rm -f /tmp/warpgate-operator-image.tar
    echo "Image loaded."

# Install cert-manager (required for webhooks)
minikube-certmanager:
    #!/usr/bin/env bash
    set -euo pipefail
    if kubectl get namespace cert-manager &>/dev/null; then
        echo "cert-manager already installed."
    else
        echo "Installing cert-manager..."
        kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
        echo "Waiting for cert-manager to be ready..."
        kubectl wait --for=condition=Available deployment/cert-manager -n cert-manager --timeout=120s
        kubectl wait --for=condition=Available deployment/cert-manager-webhook -n cert-manager --timeout=120s
        kubectl wait --for=condition=Available deployment/cert-manager-cainjector -n cert-manager --timeout=120s
        echo "cert-manager is ready."
    fi

# Create webhook certificate (self-signed)
[private]
_minikube-webhook-cert:
    #!/usr/bin/env bash
    set -euo pipefail
    kubectl get namespace warpgate-operator-system &>/dev/null || kubectl create namespace warpgate-operator-system
    if kubectl get certificate warpgate-operator-webhook-cert -n warpgate-operator-system &>/dev/null; then
        echo "Webhook certificate already exists."
    else
        echo "Creating self-signed webhook certificate..."
        cat <<CERTEOF | kubectl apply -f -
    apiVersion: cert-manager.io/v1
    kind: Issuer
    metadata:
      name: warpgate-operator-selfsigned
      namespace: warpgate-operator-system
    spec:
      selfSigned: {}
    ---
    apiVersion: cert-manager.io/v1
    kind: Certificate
    metadata:
      name: warpgate-operator-webhook-cert
      namespace: warpgate-operator-system
    spec:
      secretName: webhook-server-cert
      issuerRef:
        name: warpgate-operator-selfsigned
        kind: Issuer
      dnsNames:
        - warpgate-operator-webhook-service.warpgate-operator-system.svc
        - warpgate-operator-webhook-service.warpgate-operator-system.svc.cluster.local
    CERTEOF
        echo "Waiting for certificate to be ready..."
        kubectl wait --for=condition=Ready certificate/warpgate-operator-webhook-cert -n warpgate-operator-system --timeout=60s
    fi

# Patch webhook configs with CA bundle and fix image pull policy
[private]
_minikube-post-deploy:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Annotating webhook configurations for cert-manager CA injection..."
    kubectl annotate mutatingwebhookconfiguration warpgate-operator-mutating-webhook-configuration \
        cert-manager.io/inject-ca-from=warpgate-operator-system/warpgate-operator-webhook-cert --overwrite 2>/dev/null || true
    kubectl annotate validatingwebhookconfiguration warpgate-operator-validating-webhook-configuration \
        cert-manager.io/inject-ca-from=warpgate-operator-system/warpgate-operator-webhook-cert --overwrite 2>/dev/null || true
    echo "Fixing image reference and pull policy for local images..."
    kubectl set image deployment/warpgate-operator-controller-manager \
        -n warpgate-operator-system manager=localhost/{{img}} 2>/dev/null || true
    kubectl patch deployment -n warpgate-operator-system warpgate-operator-controller-manager \
        --type=json -p='[{"op":"replace","path":"/spec/template/spec/containers/0/imagePullPolicy","value":"Never"}]' 2>/dev/null || true
    echo "Waiting for operator pod to be ready..."
    kubectl rollout status deployment/warpgate-operator-controller-manager -n warpgate-operator-system --timeout=180s
    echo "Operator is running."

# Full dev cycle: start minikube, build, load image, deploy (fully automated)
minikube-deploy: minikube-up minikube-certmanager minikube-load _minikube-webhook-cert install deploy _minikube-post-deploy

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
    npx --yes markdownlint-cli2

# Lint YAML files
lint-yaml:
    yamllint -c .yamllint.yaml $(find . -name '*.yaml' -o -name '*.yml' | grep -v node_modules | grep -v bin/ | grep -v charts/warpgate-operator/templates)

# Lint Helm chart
lint-helm:
    helm lint charts/warpgate-operator

# Lint commit messages (checks last commit)
lint-commit:
    npx --yes @commitlint/cli --from HEAD~1 --to HEAD --config .commitlintrc.yaml

# Validate generated manifests and Helm chart CRDs are up to date
lint-manifests: manifests generate
    git diff --exit-code config/ api/ charts/warpgate-operator/templates/crds/

# Run all linters
lint: lint-go lint-md lint-yaml lint-helm lint-manifests

# Run all linters with auto-fix where possible
lint-fix: lint-go-fix
    npx --yes markdownlint-cli2 --fix

# ─── Security ────────────────────────────────────────────────────────

# Run Go security scanner (gosec)
sec-gosec:
    go install github.com/securego/gosec/v2/cmd/gosec@{{gosec_version}}
    gosec -exclude-dir=test ./...

# Run Go vulnerability checker
sec-vulncheck:
    go install golang.org/x/vuln/cmd/govulncheck@{{govulncheck_version}}
    govulncheck ./...

# Run secret scanner (gitleaks)
sec-secrets:
    gitleaks detect --source . -v

# Run all security checks
sec: sec-gosec sec-vulncheck sec-secrets

# ─── E2E Testing ─────────────────────────────────────────────────────

# Run e2e smoke tests against minikube (fully automated)
test-e2e: minikube-deploy
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Running E2E smoke tests against minikube..."

    echo "=== Verifying operator pod is running ==="
    kubectl rollout status deployment/warpgate-operator-controller-manager -n warpgate-operator-system --timeout=120s
    kubectl get pods -n warpgate-operator-system -l control-plane=controller-manager

    echo "=== Verifying all 10 CRDs installed ==="
    CRDS=$(kubectl get crds -o name | grep warpgate | wc -l)
    if [ "$CRDS" -ne 10 ]; then
        echo "FAIL: expected 10 CRDs, got $CRDS"
        exit 1
    fi
    echo "OK: $CRDS CRDs installed"

    echo "=== Testing webhook validation (should reject invalid host) ==="
    OUTPUT=$(kubectl apply -f - 2>&1 <<'YAML' || true
    apiVersion: warpgate.warpgate.warp.tech/v1alpha1
    kind: WarpgateConnection
    metadata:
      name: e2e-bad-conn
    spec:
      authSecretRef:
        name: fake
    YAML
    )
    if echo "$OUTPUT" | grep -q 'spec.host must'; then
        echo "OK: webhook rejected missing host"
    else
        echo "FAIL: webhook did not reject invalid connection"
        echo "Output: $OUTPUT"
        exit 1
    fi

    echo "=== Testing webhook validation (should reject no target type) ==="
    kubectl create secret generic e2e-auth --from-literal=token=fake --dry-run=client -o yaml | kubectl apply -f - 2>/dev/null
    kubectl apply -f - 2>/dev/null <<'YAML' || true
    apiVersion: warpgate.warpgate.warp.tech/v1alpha1
    kind: WarpgateConnection
    metadata:
      name: e2e-conn
    spec:
      host: https://warpgate.example.com
      authSecretRef:
        name: e2e-auth
    YAML
    OUTPUT=$(kubectl apply -f - 2>&1 <<'YAML' || true
    apiVersion: warpgate.warpgate.warp.tech/v1alpha1
    kind: WarpgateTarget
    metadata:
      name: e2e-bad-target
    spec:
      connectionRef: e2e-conn
      name: bad
    YAML
    )
    if echo "$OUTPUT" | grep -q 'exactly one'; then
        echo "OK: webhook rejected target with no type"
    else
        echo "FAIL: webhook did not reject invalid target"
        echo "Output: $OUTPUT"
        exit 1
    fi

    echo "=== Testing webhook defaulting (SSH port should default to 22) ==="
    kubectl apply -f - <<'YAML'
    apiVersion: warpgate.warpgate.warp.tech/v1alpha1
    kind: WarpgateTarget
    metadata:
      name: e2e-ssh-target
    spec:
      connectionRef: e2e-conn
      name: test-ssh
      ssh:
        host: 10.0.0.1
        username: root
    YAML
    PORT=$(kubectl get warpgatetarget e2e-ssh-target -o jsonpath='{.spec.ssh.port}')
    if [ "$PORT" = "22" ]; then
        echo "OK: SSH port defaulted to 22"
    else
        echo "FAIL: expected port 22, got $PORT"
        exit 1
    fi

    echo "=== Testing valid resource creation ==="
    kubectl apply -f - <<'YAML'
    apiVersion: warpgate.warpgate.warp.tech/v1alpha1
    kind: WarpgateRole
    metadata:
      name: e2e-role
    spec:
      connectionRef: e2e-conn
      name: e2e-developers
    YAML
    kubectl apply -f - <<'YAML'
    apiVersion: warpgate.warpgate.warp.tech/v1alpha1
    kind: WarpgateUser
    metadata:
      name: e2e-user
    spec:
      connectionRef: e2e-conn
      username: e2e-test-user
    YAML
    echo "OK: role and user created"

    echo "=== Checking operator logs for reconciliation ==="
    kubectl logs -n warpgate-operator-system -l control-plane=controller-manager --tail=5 2>&1 | head -5

    echo "=== Cleanup ==="
    kubectl delete warpgatetarget e2e-ssh-target --ignore-not-found
    kubectl delete warpgaterole e2e-role --ignore-not-found
    kubectl delete warpgateuser e2e-user --ignore-not-found
    kubectl delete warpgateconnection e2e-conn --ignore-not-found
    kubectl delete secret e2e-auth --ignore-not-found

    echo ""
    echo "=== ALL E2E SMOKE TESTS PASSED ==="

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
    if [ ! -f "{{kustomize}}-{{kustomize_version}}" ]; then
        echo "Downloading kustomize {{kustomize_version}}"
        rm -f "{{kustomize}}"
        GOBIN="{{localbin}}" go install sigs.k8s.io/kustomize/kustomize/v5@{{kustomize_version}}
        mv -n "{{localbin}}/kustomize" "{{kustomize}}-{{kustomize_version}}" 2>/dev/null || true
    fi
    ln -sf "{{kustomize}}-{{kustomize_version}}" "{{kustomize}}"

[private]
_controller-gen:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p "{{localbin}}"
    if [ ! -f "{{controller_gen}}-{{controller_tools_version}}" ]; then
        echo "Downloading controller-gen {{controller_tools_version}}"
        rm -f "{{controller_gen}}"
        GOBIN="{{localbin}}" go install sigs.k8s.io/controller-tools/cmd/controller-gen@{{controller_tools_version}}
        mv -n "{{localbin}}/controller-gen" "{{controller_gen}}-{{controller_tools_version}}" 2>/dev/null || true
    fi
    ln -sf "{{controller_gen}}-{{controller_tools_version}}" "{{controller_gen}}"

[private]
_envtest:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p "{{localbin}}"
    if [ ! -f "{{envtest}}-{{envtest_version}}" ]; then
        echo "Downloading setup-envtest {{envtest_version}}"
        rm -f "{{envtest}}"
        GOBIN="{{localbin}}" go install sigs.k8s.io/controller-runtime/tools/setup-envtest@{{envtest_version}}
        mv -n "{{localbin}}/setup-envtest" "{{envtest}}-{{envtest_version}}" 2>/dev/null || true
    fi
    ln -sf "{{envtest}}-{{envtest_version}}" "{{envtest}}"

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
    if [ ! -f "{{golangci_lint}}-{{golangci_lint_version}}" ]; then
        echo "Downloading golangci-lint {{golangci_lint_version}}"
        rm -f "{{golangci_lint}}"
        GOBIN="{{localbin}}" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@{{golangci_lint_version}}
        mv -n "{{localbin}}/golangci-lint" "{{golangci_lint}}-{{golangci_lint_version}}" 2>/dev/null || true
        ln -sf "{{golangci_lint}}-{{golangci_lint_version}}" "{{golangci_lint}}"
        if [ -f .custom-gcl.yml ]; then
            echo "Building custom golangci-lint with plugins..."
            "{{golangci_lint}}" custom --destination "{{localbin}}" --name golangci-lint-custom
            mv -f "{{localbin}}/golangci-lint-custom" "{{golangci_lint}}-{{golangci_lint_version}}"
            ln -sf "{{golangci_lint}}-{{golangci_lint_version}}" "{{golangci_lint}}"
        fi
    else
        ln -sf "{{golangci_lint}}-{{golangci_lint_version}}" "{{golangci_lint}}"
    fi
