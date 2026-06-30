# Contributing to k8ops

## Development Setup

### Prerequisites

- **Go** 1.26+
- **kubectl** (access to a dev cluster or kind/minikube)
- **make**
- **controller-gen** (for CRD generation): `go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest`
- **kustomize** (for deployment manifests)

### Getting Started

```bash
# Clone
 git clone https://github.com/ggai/k8ops.git
cd k8ops

# Install dependencies
go mod download

# Build
make build              # → bin/manager, bin/k8ops

# Run tests
make test               # all tests with coverage
make test-unit          # internal/... only, verbose

# Run locally against a cluster
make run PROVIDER_TYPE=openai PROVIDER_MODEL=gpt-4o

# Generate CRDs (after editing api/v1alpha1 types)
make manifests generate
```

### Development Cluster

For local development, use [kind](https://kind.sigs.k8s.io/):

```bash
kind create cluster
tmake install          # install CRDs
make deploy             # deploy k8ops
```

## Testing

### Test Commands

```bash
# All tests with race detector (preferred)
go test -race -count=1 ./internal/...

# Specific package
go test -race -v ./internal/chat/ -count=1

# Coverage report
go test -cover ./internal/...

# Benchmarks
go test -bench=. ./internal/chat/ -benchmem
```

### Testing Guidelines

1. **Always use `-race` flag** — k8ops is highly concurrent (channels, goroutines, mutexes)
2. **Table-driven tests** — use `[]struct{name string; ...}` pattern for multiple scenarios
3. **No real LLM calls in tests** — use mock providers that implement `provider.Provider`
4. **In-memory SQLite** — use `:memory:` with `SetMaxOpenConns(1)` for store tests
5. **Test file naming**: `_test.go` suffix, same package (white-box)
6. **Subtests** — use `t.Run(name, func(t *testing.T){...})` for clarity

### Mock Provider Pattern

```go
type mockProvider struct{}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) Complete(_ context.Context, _ provider.CompletionRequest) (*provider.CompletionResponse, error) {
    return &provider.CompletionResponse{Content: "mock response"}, nil
}
func (m *mockProvider) StreamComplete(_ context.Context, _ provider.CompletionRequest, _ func(string)) (*provider.CompletionResponse, error) {
    return &provider.CompletionResponse{Content: "mock"}, nil
}
```

### GORM Gotchas (for store tests)

1. **`default:true` on bool fields**: GORM overrides `false` zero-value with the DB default on `Create`. Use `db.Model().Where().Update("field", false)` to set to `false`.
2. **SQLite unique constraint**: The glebarez SQLite driver returns raw `"UNIQUE constraint failed"` rather than `gorm.ErrDuplicatedKey`. Check error string instead.

## Code Style

### Formatting

```bash
make fmt   # gofmt + goimports
``n
- **gofmt** is mandatory — CI will reject unformatted code
- **golangci-lint** for additional checks (recommended)
- 2-space indentation is NOT used in Go; use tabs (gofmt default)

### Logging

Use `log/slog` (structured logging) exclusively:

```go
// Good
logger.Info("diagnostic completed", "report", report.Name, "duration", elapsed)
logger.Warn("provider call failed", "error", err, "provider", providerType)

// Bad — do not use fmt.Println or log.Println in production code
fmt.Println("done")
```

### Error Handling

- **Always wrap errors with context**: `fmt.Errorf("failed to X: %w", err)`
- **Never ignore errors**: use `slog.Warn` if you must discard
- **Sentinel errors**: define package-level `var ErrXxx = errors.New(...)`

### Imports

Organize imports in three groups separated by blank lines:

```go
import (
    // 1. Standard library
    "context"
    "fmt"

    // 2. Third-party
    "github.com/stretchr/testify/assert"
    ctrl "sigs.k8s.io/controller-runtime"

    // 3. Local
    "github.com/ggai/k8ops/internal/provider"
)
```

### File Organization

- Keep files under ~500 lines. Split large files by responsibility.
- One type per file when practical.
- Test files (`_test.go`) alongside source files.

## Pull Request Process

### Branch Naming

```
feature/<short-description>     # e.g., feature/oidc-csrf-protection
fix/<short-description>         # e.g., fix/jwt-secret-persistence
docs/<short-description>        # e.g., docs/architecture-guide
refactor/<short-description>    # e.g., refactor/split-handlers-rbac
```

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add OIDC CSRF state protection
fix: persist JWT secret via K8s Secret
docs: add architecture documentation
refactor: split handlers_rbac.go into focused files
test: add store CRUD tests for auth package
chore: update dependencies
```

### PR Template

```markdown
## Summary

Brief description of what this PR does and why.

## Changes

- [ ] Change 1
- [ ] Change 2

## Testing

- [ ] `go test -race -count=1 ./internal/...` passes
- [ ] `go vet ./...` passes
- [ ] New tests added for new functionality
- [ ] Manual testing performed (describe)

## Checklist

- [ ] Code formatted (`make fmt`)
- [ ] No `fmt.Println` in production code (use `slog`)
- [ ] Errors wrapped with context
- [ ] No secrets or API keys committed
- [ ] Documentation updated if needed
```

### Review Criteria

1. **Correctness** — does it work? edge cases handled?
2. **Tests** — are there tests? do they use `-race`?
3. **Security** — no secrets, no `InsecureSkipVerify` without option, proper auth checks
4. **Performance** — no obvious O(n^2) or unnecessary allocations
5. **Style** — structured logging, wrapped errors, organized imports

## Release Process

1. Update `version.json` and native project versions
2. Tag: `git tag v1.x.y`
3. Build Docker image: `make docker-build IMG=...`
4. Update deployment manifests
5. Write release notes

## Questions?

Open an issue with the `question` label or reach out on the team chat.
