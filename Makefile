# Image URL to use for building/pushing image targets
IMG ?= ghcr.io/autokubeio/nodepool:latest

# Get the currently used golang install path
GOPATH ?= $(shell go env GOPATH)
GOBIN ?= $(GOPATH)/bin

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.31.0

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

.PHONY: manifests
manifests: controller-gen ## Generate CRD manifests.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: fmt vet ## Run tests.
	go test ./...

.PHONY: lint
lint: ## Run golangci-lint linter
	golangci-lint run

##@ Build

.PHONY: build
build: fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: fmt vet ## Run a controller from your host.
	./scripts/run-with-env.sh go run cmd/main.go

.PHONY: docker-build
docker-build: ## Build docker image.
	docker build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image.
	docker push ${IMG}

.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for multiple platforms.
	docker buildx build --platform linux/amd64,linux/arm64 -t ${IMG} --push .

##@ Deployment

.PHONY: install
install: ## Install CRDs into the K8s cluster.
	helm template nodepool charts/nodepool --show-only templates/crd.yaml --set hcloudToken=dummy | kubectl apply -f -

.PHONY: uninstall
uninstall: ## Uninstall CRDs from the K8s cluster.
	kubectl delete crd nodepools.autokube.io

.PHONY: deploy
deploy: ## Deploy controller to the K8s cluster.
	helm upgrade --install nodepool-operator ./charts/nodepool \
		--namespace nodepool-system --create-namespace \
		--set image.repository=$(shell echo ${IMG} | cut -d':' -f1) \
		--set image.tag=$(shell echo ${IMG} | cut -d':' -f2)

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster.
	helm uninstall nodepool-operator --namespace nodepool-system

##@ Helm

.PHONY: helm-lint
helm-lint: ## Lint Helm chart.
	helm lint charts/nodepool

.PHONY: helm-package
helm-package: ## Package Helm chart.
	helm package charts/nodepool -d dist/

.PHONY: release-manifests
release-manifests: manifests ## Generate combined installation manifest for GitHub releases
	@echo "Generating release manifests..."
	@mkdir -p dist
	@echo "# NodePool Operator - Complete Installation Manifest" > dist/install.yaml
	@echo "# This manifest includes CRDs, RBAC, and Deployment" >> dist/install.yaml
	@echo "# Apply with: kubectl apply -f install.yaml" >> dist/install.yaml
	@echo "" >> dist/install.yaml
	@echo "# ============================================" >> dist/install.yaml
	@echo "# Custom Resource Definitions (CRDs)" >> dist/install.yaml
	@echo "# ============================================" >> dist/install.yaml
	@cat config/crd/bases/autokube.io_nodepools.yaml >> dist/install.yaml
	@echo "" >> dist/install.yaml
	@echo "---" >> dist/install.yaml
	@echo "# ============================================" >> dist/install.yaml
	@echo "# RBAC Configuration" >> dist/install.yaml
	@echo "# ============================================" >> dist/install.yaml
	@cat config/rbac/role.yaml >> dist/install.yaml
	@echo "" >> dist/install.yaml
	@echo "---" >> dist/install.yaml
	@echo "# ============================================" >> dist/install.yaml
	@echo "# Controller Deployment" >> dist/install.yaml
	@echo "# ============================================" >> dist/install.yaml
	@cat config/manager/manager.yaml >> dist/install.yaml
	@echo "" >> dist/install.yaml
	@echo "✅ Release manifests generated in dist/install.yaml"
	@echo "📦 Apply with: kubectl apply -f dist/install.yaml"

.PHONY: helm-install
helm-install: ## Install Helm chart locally.
	helm upgrade --install nodepool ./charts/nodepool \
		--namespace hcloud-system --create-namespace \
		--set hcloudToken=${HCLOUD_TOKEN}

##@ Dependencies

.PHONY: deps
deps: ## Download dependencies.
	go mod download
	go mod tidy

.PHONY: deps-update
deps-update: ## Update dependencies.
	go get -u ./...
	go mod tidy

##@ Release

.PHONY: release-dry-run
release-dry-run: ## Test release process without publishing.
	goreleaser release --snapshot --rm-dist

.PHONY: release
release: ## Create a new release (requires git tag).
	goreleaser release --rm-dist

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

## Tool Versions
CONTROLLER_TOOLS_VERSION ?= v0.16.5

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen && $(LOCALBIN)/controller-gen --version | grep -q $(CONTROLLER_TOOLS_VERSION) || \
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
