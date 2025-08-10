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

.PHONY: test
test: fmt vet ## Run tests.
	go test ./... -coverprofile cover.out

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: build
build: fmt vet ## Build manager binary.
	go build -o bin/manager main.go

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

.PHONY: package-helm
package-helm: generate-crds update-version ## Package Helm chart with CRDs
	mkdir -p charts/
	# Copy CRDs to Helm chart templates
	mkdir -p helm/vault-autounseal-operator/templates/crds/
	cp generated/*.yaml helm/vault-autounseal-operator/templates/crds/ 2>/dev/null || echo "No CRDs to copy"
	# Package the chart
	helm package helm/vault-autounseal-operator/ --destination ./charts/
	ls -la charts/

.PHONY: release
release: package-helm ## Create release artifacts
	mkdir -p release/
	cp -r charts/* release/
	cp -r generated/* release/ 2>/dev/null || echo "No CRDs to copy"
	# Create install script
	cat > release/install.sh << 'EOF'
#!/bin/bash
set -e

NAMESPACE=$${NAMESPACE:-vault-system}
CHART_VERSION=$${CHART_VERSION:-$(VERSION)}

echo "Installing Vault Auto-Unseal Operator v$(VERSION) to namespace: $$NAMESPACE"

# Create namespace if it doesn't exist
kubectl create namespace $$NAMESPACE --dry-run=client -o yaml | kubectl apply -f -

# Install using Helm
helm upgrade --install vault-autounseal-operator \
  ./vault-autounseal-operator-$(VERSION).tgz \
  --namespace $$NAMESPACE \
  --wait

echo "Installation complete!"
EOF
	chmod +x release/install.sh

.PHONY: clean
clean: ## Clean build artifacts
	rm -rf bin/
	rm -rf cover.out
	rm -rf charts/
	rm -rf generated/
	rm -rf config/
	rm -rf release/
