.PHONY: build vet test fmt deploy release clean help regression-gate pre-commit

VERSION ?= dev
REGISTRY ?= registry.iot2.win/k8ops

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build all binaries
	go build ./...

vet: ## Run go vet
	go vet ./...

test: ## Run all tests
	go test ./... -count=1

test-race: ## Run tests with race detector
	go test -race -timeout 5m ./...

fmt: ## Format all Go files
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')
	goimports -w $$(find . -name '*.go' -not -path './vendor/*') 2>/dev/null || true

check-fmt: ## Check if all files are formatted
	@UNFORMATTED=$$(gofmt -l . | grep -v vendor/ || true); \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "Files need formatting:"; echo "$$UNFORMATTED"; exit 1; \
	fi; echo "All files formatted"

lint: ## Run golangci-lint
	golangci-lint run --timeout 5m ./...

coverage: ## Run tests with coverage
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | grep total

docker-build: ## Build Docker image for local registry
	docker buildx build --platform linux/amd64 --build-arg VERSION=$(VERSION) \
		-t $(REGISTRY):$(VERSION) -t $(REGISTRY):latest --push .

deploy: ## Deploy to k8s cluster (usage: make deploy VERSION=v14XX)
	@if [ "$(VERSION)" = "dev" ]; then echo "Usage: make deploy VERSION=v14XX"; exit 1; fi
	@echo "=== Pre-deploy regression gate ==="
	$(MAKE) regression-gate
	@echo "=== All checks passed, building image ==="
	$(MAKE) docker-build VERSION=$(VERSION)
	@echo "=== Deploying $(VERSION) ==="
	kubectl set image daemonset/k8ops k8ops=$(REGISTRY):$(VERSION) -n k8ops-system
	@echo "Waiting for rollout..."
	@sleep 15
	@curl -sk -o /dev/null -w 'Health: %{http_code}\n' https://k8ops.iot2.win/

deploy-check: ## Check deployment health
	@echo "Pod status:"
	@kubectl get pods -n k8ops-system --no-headers | head -5
	@echo ""
	@echo "Health check:"
	@curl -sk -o /dev/null -w 'HTTP %{http_code}\n' https://k8ops.iot2.win/
	@echo ""
	@echo "Version:"
	@curl -sk https://k8ops.iot2.win/api/version 2>/dev/null | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'  {d.get(\"version\",\"?\")} (built {d.get(\"buildDate\",\"?\")[:19]}')')" 2>/dev/null || true

release: ## Tag and push a release (usage: make release VERSION=v14XX)
	@if [ "$(VERSION)" = "dev" ]; then echo "Usage: make release VERSION=v14XX"; exit 1; fi
	$(MAKE) regression-gate
	git tag -a $(VERSION) -m "Release $(VERSION)"
	git push origin $(VERSION)
	@echo "Release triggered. Monitor with: gh run watch"

helm-lint: ## Validate Helm chart
	helm lint deploy/helm/k8ops/

helm-template: ## Render Helm templates
	helm template deploy/helm/k8ops/ --debug

clean: ## Clean build artifacts
	rm -f coverage.out
	rm -rf dist/
	go clean -cache 2>/dev/null || true

# Full regression gate — must pass before any deploy or release
regression-gate: check-fmt build vet lint test
	@echo "=== Regression gate PASSED ==="

# Pre-commit checklist (same as regression-gate)
pre-commit: fmt regression-gate
	@echo "All checks passed. Ready to commit."
