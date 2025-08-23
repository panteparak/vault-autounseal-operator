# Image URL to use all building/pushing image targets
IMG ?= vault-autounseal-operator:latest

# Version
VERSION ?= $(shell cat VERSION 2>/dev/null || echo "0.1.0")

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: tidy
tidy: ## Run go mod tidy to clean up module dependencies.
	go mod tidy

.PHONY: verify
verify: fmt vet tidy ## Run verification checks (format, vet, tidy).
	@echo "✅ Verification completed"

.PHONY: quality
quality: verify lint test-unit ## Run all code quality checks.
	@echo "✅ Code quality checks completed"

.PHONY: security-scan
security-scan: ## Run security vulnerability scans.
	@command -v govulncheck >/dev/null 2>&1 || go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

##@ Testing (Modular Test Structure)

# New modular test system (tests/ directory)
.PHONY: test-all-modules test-unit-modules test-integration-modules test-e2e-modules
.PHONY: test-performance-modules test-chaos-modules test-boundary-modules
.PHONY: test-setup-modules test-clean-modules test-coverage-modules

test-all-modules: ## Run all test modules
	@cd tests && $(MAKE) test-all

test-unit-modules: ## Run unit tests
	@cd tests && $(MAKE) test-unit

test-integration-modules: ## Run integration tests
	@cd tests && $(MAKE) test-integration

test-e2e-modules: ## Run end-to-end tests
	@cd tests && $(MAKE) test-e2e

test-performance-modules: ## Run performance tests
	@cd tests && $(MAKE) test-performance

test-chaos-modules: ## Run chaos engineering tests
	@cd tests && $(MAKE) test-chaos

test-boundary-modules: ## Run boundary condition tests
	@cd tests && $(MAKE) test-boundary

test-setup-modules: ## Set up test environment
	@cd tests && $(MAKE) test-setup

test-clean-modules: ## Clean test artifacts
	@cd tests && $(MAKE) test-clean

test-coverage-modules: ## Generate test coverage report
	@cd tests && $(MAKE) test-coverage

# Legacy test targets (for backward compatibility)
.PHONY: test
test: fmt vet ## Run legacy tests.
	go test ./... -coverprofile cover.out

# Main test targets that delegate to modular test system
.PHONY: test-unit test-integration test-e2e test-performance test-chaos test-boundary
.PHONY: test-integration-verbose test-all

test-unit: ## Run unit tests
	@echo "🔬 Running unit tests..."
	@cd tests && $(MAKE) test-unit

test-integration: ## Run integration tests using modular system
	@echo "🔗 Running integration tests..."
	@cd tests && $(MAKE) test-integration

test-e2e: ## Run end-to-end tests
	@echo "🌐 Running end-to-end tests..."
	@cd tests && $(MAKE) test-e2e

test-performance: ## Run performance tests
	@echo "⚡ Running performance tests..."
	@cd tests && $(MAKE) test-performance

test-chaos: ## Run chaos engineering tests
	@echo "🌪️ Running chaos tests..."
	@cd tests && $(MAKE) test-chaos

test-boundary: ## Run boundary condition tests
	@echo "🏔️ Running boundary tests..."
	@cd tests && $(MAKE) test-boundary

test-all: ## Run all test categories
	@echo "🧪 Running all test categories..."
	@cd tests && $(MAKE) test-all

test-integration-verbose: ## Run integration tests with verbose output
	@echo "🔗 Running integration tests (verbose)..."
	@cd tests && $(MAKE) test-integration VERBOSE=true

# Convenient short-form targets
.PHONY: test-unit-quick test-smoke test-quick
test-unit-quick: ## Run unit tests without coverage
	@echo "🔬 Running unit tests (quick)..."
	@cd tests && go test ./unit/...

test-smoke: ## Run quick smoke tests
	@echo "💨 Running smoke tests..."
	@cd tests && $(MAKE) test-smoke

test-quick: ## Run basic validation tests only
	@echo "⚡ Running quick validation tests..."
	@cd tests && go test ./unit/validation/ -run TestValidationTestSuite/TestDefaultKeyValidatorBasic -v

# Test with different modes
.PHONY: test-short test-verbose test-race test-coverage
test-short: ## Run all tests in short mode (skips long-running tests)
	@echo "⏱️ Running tests in short mode..."
	@cd tests && go test --short ./...

test-verbose: ## Run tests with verbose output
	@echo "📝 Running tests with verbose output..."
	@cd tests && $(MAKE) test-unit VERBOSE=true

test-race: ## Run tests with race detection
	@echo "🏁 Running tests with race detection..."
	@cd tests && $(MAKE) test-race

test-coverage: ## Generate test coverage report
	@echo "📊 Generating test coverage..."
	@cd tests && $(MAKE) test-coverage

# Test maintenance targets
.PHONY: test-setup test-clean test-deps
test-setup: ## Set up test environment
	@echo "🔧 Setting up test environment..."
	@cd tests && $(MAKE) test-setup

test-clean: ## Clean test artifacts and cache
	@echo "🧹 Cleaning test artifacts..."
	@cd tests && $(MAKE) test-clean
	@go clean -testcache

test-deps: ## Download test dependencies
	@echo "📦 Downloading test dependencies..."
	@cd tests && $(MAKE) test-deps

# Default test target now uses modular system
.PHONY: test-default
test-default: test-all ## Run all tests using modular system

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: build
build: verify ## Build manager binary with version info.
	@mkdir -p bin
	@VERSION=$$(cat VERSION 2>/dev/null || echo "dev"); \
	BUILD_TIME=$$(date -u +"%Y-%m-%dT%H:%M:%SZ"); \
	GIT_COMMIT=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	go build -ldflags="-X main.version=$$VERSION -X main.buildTime=$$BUILD_TIME -X main.gitCommit=$$GIT_COMMIT" \
		-o bin/manager main.go
	@echo "✅ Built manager binary with version info"

.PHONY: build-debug
build-debug: verify ## Build manager binary with debug symbols.
	@mkdir -p bin
	go build -gcflags="all=-N -l" -o bin/manager-debug main.go

.PHONY: cross-compile
cross-compile: verify ## Cross-compile binaries for multiple platforms.
	@mkdir -p bin
	@VERSION=$$(cat VERSION 2>/dev/null || echo "dev"); \
	BUILD_TIME=$$(date -u +"%Y-%m-%dT%H:%M:%SZ"); \
	GIT_COMMIT=$$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	LDFLAGS="-X main.version=$$VERSION -X main.buildTime=$$BUILD_TIME -X main.gitCommit=$$GIT_COMMIT"; \
	for os in linux darwin windows; do \
		for arch in amd64 arm64; do \
			echo "Building for $$os/$$arch..."; \
			GOOS=$$os GOARCH=$$arch go build -ldflags="$$LDFLAGS" -o bin/manager-$$os-$$arch main.go; \
		done; \
	done
	@echo "✅ Cross-compilation completed"

.PHONY: run
run: fmt vet ## Run a controller from your host.
	go run ./main.go

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	docker build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

##@ Deployment

.PHONY: install
install: ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	kubectl apply -f manifests/crd.yaml

.PHONY: uninstall
uninstall: ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	kubectl delete -f manifests/crd.yaml

.PHONY: deploy
deploy: ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	kubectl apply -f manifests/rbac.yaml
	kubectl apply -f manifests/deployment.yaml

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	kubectl delete -f manifests/deployment.yaml
	kubectl delete -f manifests/rbac.yaml

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen

## Tool Versions
GOLANGCI_LINT_VERSION ?= v1.54.2
CONTROLLER_TOOLS_VERSION ?= v0.14.0

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(LOCALBIN) $(GOLANGCI_LINT_VERSION)

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

##@ Release

.PHONY: generate-crds
generate-crds: controller-gen ## Generate CRD files
	mkdir -p config/crd/bases generated/
	$(CONTROLLER_GEN) crd:allowDangerousTypes=true paths="./pkg/api/..." output:crd:artifacts:config=config/crd/bases
	cp config/crd/bases/*.yaml generated/ 2>/dev/null || echo "No CRDs generated"

.PHONY: update-version
update-version: ## Update version in Helm chart
	@if [ -z "$(VERSION)" ]; then echo "VERSION is required"; exit 1; fi
	sed -i.bak 's/^version: .*/version: $(VERSION)/' helm/vault-autounseal-operator/Chart.yaml
	sed -i.bak 's/^appVersion: .*/appVersion: "$(VERSION)"/' helm/vault-autounseal-operator/Chart.yaml
	rm -f helm/vault-autounseal-operator/Chart.yaml.bak

.PHONY: helm-lint
helm-lint: ## Lint Helm chart
	helm lint helm/vault-autounseal-operator/

.PHONY: helm-template
helm-template: ## Generate Kubernetes manifests from Helm chart
	mkdir -p manifests/helm-generated/
	helm template vault-autounseal-operator helm/vault-autounseal-operator/ \
		--namespace vault-system \
		--set image.tag=$(VERSION) \
		> manifests/helm-generated/all-in-one.yaml

.PHONY: package-helm
package-helm: generate-crds update-version helm-lint ## Package Helm chart with CRDs
	mkdir -p charts/
	# Copy CRDs to Helm chart templates
	mkdir -p helm/vault-autounseal-operator/templates/crds/
	cp generated/*.yaml helm/vault-autounseal-operator/templates/crds/ 2>/dev/null || echo "No CRDs to copy"
	# Package the chart
	helm package helm/vault-autounseal-operator/ --destination ./charts/
	ls -la charts/

.PHONY: docs-build
docs-build: generate-crds ## Build documentation site locally
	@echo "🏗️ Building documentation site..."
	mkdir -p docs-site/static/crds/ docs-site/static/manifests/
	cp generated/*.yaml docs-site/static/crds/ 2>/dev/null || echo "No CRDs to copy"
	cp -r manifests/* docs-site/static/manifests/ 2>/dev/null || echo "No manifests to copy"
	cp -r examples/* docs-site/static/examples/ 2>/dev/null || echo "No examples to copy"
	@echo "✅ Documentation build completed. Open docs-site/index.html to preview."

.PHONY: docs-serve
docs-serve: docs-build ## Serve documentation site locally
	@echo "🌐 Starting local documentation server..."
	@command -v python3 >/dev/null 2>&1 || { echo "Python3 is required to serve docs locally"; exit 1; }
	cd docs-site && python3 -m http.server 8080
	@echo "📖 Documentation available at http://localhost:8080"

.PHONY: release
release: package-helm helm-template ## Create release artifacts
	mkdir -p release/
	cp -r charts/* release/
	cp -r generated/* release/ 2>/dev/null || echo "No CRDs to copy"
	cp -r manifests/helm-generated/* release/ 2>/dev/null || echo "No generated manifests to copy"
	# Create install script
	@echo '#!/bin/bash' > release/install.sh
	@echo 'set -e' >> release/install.sh
	@echo '' >> release/install.sh
	@echo 'NAMESPACE=$${NAMESPACE:-vault-system}' >> release/install.sh
	@echo 'CHART_VERSION=$${CHART_VERSION:-$(VERSION)}' >> release/install.sh
	@echo 'RELEASE_NAME=$${RELEASE_NAME:-vault-autounseal-operator}' >> release/install.sh
	@echo '' >> release/install.sh
	@echo 'echo "Installing Vault Auto-Unseal Operator v$(VERSION) to namespace: $$NAMESPACE"' >> release/install.sh
	@echo '' >> release/install.sh
	@echo '# Create namespace if it does not exist' >> release/install.sh
	@echo 'kubectl create namespace $$NAMESPACE --dry-run=client -o yaml | kubectl apply -f -' >> release/install.sh
	@echo '' >> release/install.sh
	@echo '# Check if Helm is available' >> release/install.sh
	@echo 'if ! command -v helm &> /dev/null; then' >> release/install.sh
	@echo '    echo "Error: Helm is not installed. Please install Helm first."' >> release/install.sh
	@echo '    echo "Visit: https://helm.sh/docs/intro/install/"' >> release/install.sh
	@echo '    exit 1' >> release/install.sh
	@echo 'fi' >> release/install.sh
	@echo '' >> release/install.sh
	@echo '# Install using Helm' >> release/install.sh
	@echo 'echo "Installing via Helm..."' >> release/install.sh
	@echo 'helm upgrade --install $$RELEASE_NAME \' >> release/install.sh
	@echo '  ./vault-autounseal-operator-$(VERSION).tgz \' >> release/install.sh
	@echo '  --namespace $$NAMESPACE \' >> release/install.sh
	@echo '  --wait \' >> release/install.sh
	@echo '  --timeout 300s' >> release/install.sh
	@echo '' >> release/install.sh
	@echo 'echo "Installation complete!"' >> release/install.sh
	@echo 'echo ""' >> release/install.sh
	@echo 'echo "To check the status:"' >> release/install.sh
	@echo 'echo "  kubectl get pods -n $$NAMESPACE"' >> release/install.sh
	@echo 'echo "  kubectl logs -n $$NAMESPACE deployment/vault-autounseal-operator"' >> release/install.sh
	chmod +x release/install.sh
	@echo "✅ Release artifacts created in ./release/"

.PHONY: clean
clean: ## Clean build artifacts
	rm -rf bin/
	rm -rf cover.out
	rm -rf charts/
	rm -rf generated/
	rm -rf config/
	rm -rf release/
