# k8ops Makefile

# Version info (inject via -ldflags)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# LDFLAGS inject version info into dashboard package
LDFLAGS := -X github.com/ggai/k8ops/internal/dashboard.Version=$(VERSION) \
           -X github.com/ggai/k8ops/internal/dashboard.GitCommit=$(GIT_COMMIT) \
           -X github.com/ggai/k8ops/internal/dashboard.BuildDate=$(BUILD_DATE)

# Image settings
IMAGE_REPO ?= ghcr.io/ggai/k8ops
IMAGE_TAG ?= $(VERSION)
IMG ?= $(IMAGE_REPO):$(IMAGE_TAG)

# Go settings
GO := go
GOFMT := gofmt
GOFLAGS := -trimpath

# Controller tool versions
CONTROLLER_TOOLS_VERSION ?= v0.15.0
CONTROLLER_GEN := $(shell pwd)/bin/controller-gen
KUSTOMIZE_VERSION ?= v5.4.0
KUSTOMIZE := $(shell pwd)/bin/kustomize
ENVTEST_K8S_VERSIONS ?= 1.30.0
GOLANGCI_LINT_VERSION ?= v2.12.2
GOLANGCI_LINT := $(shell pwd)/bin/golangci-lint

.PHONY: all
all: build test

# ----------------------------------------------------------------------------
# Build
# ----------------------------------------------------------------------------

.PHONY: build
build: ## Build manager binary with version info
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/manager ./cmd/manager

.PHONY: build-cli
build-cli: ## Build CLI tool
	$(GO) build $(GOFLAGS) -o bin/k8ops ./cmd/k8ops

.PHONY: run
run: ## Run manager locally
	$(GO) run ./cmd/manager

.PHONY: docker-build
docker-build: ## Build Docker image
	docker build --build-arg VERSION=$(VERSION) -t $(IMG) .

.PHONY: docker-push
docker-push: ## Push Docker image
	docker push $(IMG)

.PHONY: docker-local
docker-local: ## Build and push to local registry (registry.iot2.win)
	docker build --build-arg VERSION=$(VERSION) -t registry.iot2.win/k8ops:$(VERSION) -t registry.iot2.win/k8ops:latest .
	docker push registry.iot2.win/k8ops:$(VERSION)
	docker push registry.iot2.win/k8ops:latest
	@echo "Pushed registry.iot2.win/k8ops:$(VERSION) and :latest"

# ----------------------------------------------------------------------------
# Test
# ----------------------------------------------------------------------------

.PHONY: test
test: ## Run all unit tests
	$(GO) test ./internal/... -count=1 -timeout 180s

.PHONY: test-race
test-race: ## Run tests with race detector
	$(GO) test ./internal/... -race -count=1 -timeout 180s

.PHONY: test-verbose
test-verbose: ## Run tests with verbose output
	$(GO) test ./internal/... -v -count=1 -timeout 120s

.PHONY: cover
cover: ## Generate test coverage report
	$(GO) test ./internal/... -coverprofile=coverage.out -timeout 120s
	$(GO) tool cover -func=coverage.out | tail -1
	@echo "Run 'go tool cover -html=coverage.out' for detailed report"

.PHONY: cover-html
cover-html: cover ## Generate HTML coverage report
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

.PHONY: integration-test
integration-test: ## Run integration tests (requires cluster access)
	$(GO) test ./test/... -count=1 -timeout 300s -tags=integration

# ----------------------------------------------------------------------------
# Lint
# ----------------------------------------------------------------------------

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Run golangci-lint
	$(GOLANGCI_LINT) run --timeout 5m ./...

.PHONY: fmt
fmt: ## Format Go code
	$(GOFMT) -s -w .

.PHONY: fmt-check
fmt-check: ## Check if code is formatted
	@if [ -n "$$($(GOFMT) -l .)" ]; then \
		echo "Files need formatting:"; \
		$(GOFMT) -l .; \
		exit 1; \
	fi

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: check
check: fmt-check vet lint ## Run all checks (fmt + vet + lint)

# ----------------------------------------------------------------------------
# CRD / Manifests
# ----------------------------------------------------------------------------

.PHONY: manifests
manifests: $(CONTROLLER_GEN) ## Generate CRD manifests and deepcopy code
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd:maxDescLen=0 paths="./api/..." output:crd:artifacts:config=config/crd/bases output:rbac:artifacts:config=config/rbac

.PHONY: generate
generate: $(CONTROLLER_GEN) ## Generate deepcopy code
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./api/..."

# ----------------------------------------------------------------------------
# Deploy
# ----------------------------------------------------------------------------

.PHONY: deploy
deploy: ## Deploy to cluster
	kubectl apply -k config/deploy/base

.PHONY: deploy-prod
deploy-prod: ## Deploy with production overlay
	kubectl apply -k config/deploy/overlays/prod

.PHONY: undeploy
undeploy: ## Remove from cluster
	kubectl delete -k config/deploy/base --ignore-not-found

# ----------------------------------------------------------------------------
# Tools
# ----------------------------------------------------------------------------

$(CONTROLLER_GEN):
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION))

$(KUSTOMIZE):
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION))

$(GOLANGCI_LINT):
	@echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."
	@OS=$$(uname -s | tr '[:upper:]' '[:lower:]') && \
	ARCH=$$(uname -m | sed 's/aarch64/arm64/' | sed 's/x86_64/amd64/') && \
	VER_NUM=$$(echo $(GOLANGCI_LINT_VERSION) | sed 's/^v//') && \
	URL="https://github.com/golangci/golangci-lint/releases/download/$(GOLANGCI_LINT_VERSION)/golangci-lint-$${VER_NUM}-$${OS}-$${ARCH}.tar.gz" && \
	tmpdir=$$(mktemp -d) && \
	curl -sL "$$URL" | tar xz -C $$tmpdir && \
	mkdir -p bin && \
	cp $$tmpdir/golangci-lint-$${VER_NUM}-$${OS}-$${ARCH}/golangci-lint $(GOLANGCI_LINT) && \
	chmod +x $(GOLANGCI_LINT) && \
	rm -rf $$tmpdir

# go-get-tool downloads a binary tool into bin/.
define go-get-tool
@[ -f "$(1)-$(3)" ] || { \
set -e ;\
tmpdir=$$(mktemp -d) ;\
cd $$tmpdir ;\
$(GO) mod init tmp ;\
echo "Downloading $(3)" ;\
GOBIN=$$(pwd) $(GO) install "$(2)" ;\
mv $$(basename "$(2)") $(1) ;\
rm -rf $$tmpdir ;\
}
endef

# ----------------------------------------------------------------------------
# Help
# ----------------------------------------------------------------------------

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_/-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
