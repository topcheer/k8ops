# GGCODE.md

> Project guidance for AI coding agents working on k8ops.
> Verify and update when conventions change.

## Project Overview

**k8ops** is a Kubernetes AIOps platform — an operator + embedded web dashboard that uses AI agents to diagnose cluster issues, suggest optimizations, and execute remediations. Runs as a DaemonSet (host-network, privileged, nsenter to host) or Deployment (cluster-internal).

- **Module:** `github.com/ggai/k8ops`
- **Go version:** 1.26+
- **Key deps:** controller-runtime, client-go, k8s.io/api (v0.36.x), gorm (sqlite/mysql/postgres), prometheus client, slog
- **Image base:** `gcr.io/distroless/static-debian12:nonroot` (~28.6MB final image, no shell)
- **Frontend:** Vanilla HTML/JS/CSS embedded into the Go binary via `//go:embed web/*`

## Directory Map

| Path | Description |
|------|-------------|
| `api/v1alpha1/` | CRD types: DiagnosticReport, RemediationPlan, OptimizationSuggestion, K8opsConfig |
| `cmd/manager/` | Operator entrypoint (main binary `/manager`) |
| `cmd/k8ops/` | CLI tool (`/usr/local/bin/k8ops`) |
| `internal/agent/` | AI agent loop, tool execution, autopilot continuation |
| `internal/audit/` | Structured audit logging (JSONL file + ring buffer, rotation) |
| `internal/auth/` | Auth: local users, LDAP, OIDC, JWT, API keys, RBAC sync |
| `internal/chat/` | Chat session management, conversation memory (auto-summarize at 20k tokens) |
| `internal/collector/` | Cluster event collector (watches Warning events) |
| `internal/controller/` | Controller-runtime reconcilers for CRDs |
| `internal/cost/` | FinOps: cost estimation and recommendations |
| `internal/dashboard/` | Embedded HTTP server — REST API + SPA frontend |
| `internal/dashboard/web/` | Frontend: `index.html`, per-feature JS modules, `modules/utils.js` |
| `internal/memory/` | Project memory + auto-memory loading |
| `internal/metrics/` | Prometheus metrics definitions |
| `internal/provider/` | AI provider adapters: `openai/`, `anthropic/`, `gemini/` |
| `internal/providermanager/` | Provider lifecycle, failover, circuit breaker |
| `internal/resilience/` | Circuit breaker, retry, rate limiter |
| `internal/safety/` | Dry-run validation for remediation actions |
| `internal/tools/` | Agent tools: `k8s/` (KubeClient), `host/` (nsenter), `remediation/`, `registry.go` |
| `config/` | K8s manifests: `daemonset/`, `deploy/base/`, `deploy/overlays/`, `rbac/`, `crd/` |
| `scripts/` | Deploy automation: `deploy.sh`, `rollback.sh`, `pre-deploy-check.sh`, `backup.sh` |
| `docs/` | 12 docs: Architecture, Deployment, User Guide, Runbook, API, Security, Troubleshooting |
| `test/` | Integration tests (build tag: `integration`) |

## Build & Validation Commands

```bash
# Quick build + vet (minimum required before commit)
go build ./... && go vet ./...

# Run tests (internal packages, 120s timeout)
go test ./internal/... -count=1 -timeout 120s

# Tests with race detector (recommended for concurrent code)
go test -race ./internal/... -count=1 -timeout 180s

# Specific package
go test -v ./internal/dashboard/ -count=1 -timeout 60s

# Coverage report
go test ./internal/... -coverprofile=coverage.out && go tool cover -func=coverage.out

# Lint (golangci-lint with govet + errcheck)
make lint

# Format check
make fmt-check

# Full CI-style check
make check   # fmt-check + vet + lint

# Build binaries
make build        # → bin/manager (with version ldflags)
make build-cli    # → bin/k8ops

# Generate CRDs (after editing api/v1alpha1 types)
make manifests generate
```

### Docker Build & Deploy

```bash
# Build and push (uses buildx with cache mounts)
VERSION=v14XX
docker buildx build --platform linux/amd64 --build-arg VERSION=$VERSION \
  -t registry.iot2.win/k8ops:$VERSION -t registry.iot2.win/k8ops:latest --push .

# Deploy to cluster
kubectl set image daemonset/k8ops k8ops=registry.iot2.win/k8ops:$VERSION -n k8ops-system
sleep 15 && curl -sk -o /dev/null -w '%{http_code}' https://k8ops.iot2.win/
```

### One-Command Deploy with Auto-Rollback

```bash
./scripts/deploy.sh v14XX      # pre-check → build → rollout → health → auto-rollback on failure
./scripts/rollback.sh          # quick rollback to previous revision
```

## Code Conventions

### Go Style

- **gofmt is mandatory** — CI rejects unformatted code. Run `gofmt -w .` before committing.
- **Tabs for indentation** (gofmt default) — never spaces in `.go` files.
- **Imports** organized in 3 groups: stdlib, third-party, local (separated by blank lines).
- **Error handling**: always wrap with context: `fmt.Errorf("failed to X: %w", err)`. Never ignore errors.
- **Logging**: use `log/slog` exclusively (structured JSON). Never `fmt.Println` or `log.Println` in production.
- **Files under ~500 lines** — split large files by responsibility.

### Testing

- **Always use `-race` flag** — k8ops is highly concurrent.
- **Table-driven tests** with `t.Run(name, func(t *testing.T){...})`.
- **White-box testing** — test files in same package (`package dashboard`, not `dashboard_test`).
- **Mock providers** — implement `provider.Provider` interface, never make real LLM calls in tests.
- **In-memory SQLite** — use `:memory:` with `SetMaxOpenConns(1)` for DB tests.
- **Fake k8s client** — use `k8sfake.NewSimpleClientset()` + `ctrlfake.NewClientBuilder()`.

### K8s API Type Gotchas

- `*int32` fields (e.g. `AverageUtilization`) — dereference with nil check: `*ptr`, NOT `.IntVal`.
  `.IntVal` belongs to `intstr.IntOrString`, a completely different type.

### Frontend Conventions

- **XSS prevention**: ALL dynamic content must go through `escapeHtml()` from `modules/utils.js`.
- **ES modules**: use `import { escapeHtml, fetchJSON } from './modules/utils.js'`.
- **Module registration**: add to `main.js` imports + `allModules` array; add tab trigger in `core.js`.
- **Tab HTML**: add `<button onclick="showTab('name', this)">` in nav + `<div id="tab-name">` in body.
- **Comments**: English in code, Chinese in documentation (`docs/`).

### Adding a New API Endpoint

1. Create handler in `internal/dashboard/handlers_<feature>.go`.
2. Write tests in `handlers_<feature>_test.go` (same package).
3. Register route in `server.go` (`mux.HandleFunc`).
4. Add to OpenAPI spec in `handlers_openapi.go`.
5. If frontend: create `web/<feature>.js`, wire into `main.js` + `core.js` + `index.html`.

## Architecture Notes

- **Dashboard server**: embedded in `/manager` binary, serves on `:9090` (dashboard), `:8080` (metrics), `:8081` (health).
- **Metrics endpoint** (`/metrics`): localhost-only via `localOnlyMiddleware`.
- **Response caching**: `cacheMiddleware(duration)` wraps expensive endpoints.
- **API perf tracking**: in-memory ring buffer (5000 samples), p50/p95/p99 per endpoint.
- **Auth middleware**: JWT-based, impersonation to k8s API for RBAC enforcement.
- **Provider pattern**: `provider.Provider` interface → concrete adapters (openai/anthropic/gemini).
- **Agent tools**: registered via `tools/registry.go`, thread-safe (`sync.RWMutex`).
- **Audit log**: JSONL file + in-memory ring buffer (500 events). Auto-rotation at 100MB.
- **GORM gotchas**: `default:true` bool fields need `.Update()` to set false; SQLite returns raw `"UNIQUE constraint failed"` not `gorm.ErrDuplicatedKey`.

## Environment Variables

Key env vars (see `.env.example` for full reference):

| Variable | Default | Purpose |
|----------|---------|---------|
| `PROVIDER_TYPE` | `openai` | AI provider: openai, anthropic, gemini |
| `PROVIDER_MODEL` | — | Model name |
| `AIOPS_API_KEY` | — | LLM API key (from Secret `k8ops-credentials`) |
| `AUTH_JWT_SECRET` | — | JWT signing secret (from Secret `k8ops-auth`) |
| `AUTH_DB_DRIVER` | `sqlite` | sqlite, postgres, mysql |
| `AUTH_DB_PATH` | `/data/k8ops.db` | SQLite file path |
| `DASHBOARD_ADDRESS` | `:9090` | HTTP listen address |
| `LOG_LEVEL` | `info` | debug, info, warn, error (debug adds source file:line) |
| `DEBUG` | `false` | Verbose logging |
| `CORS_ALLOWED_ORIGINS` | — | Comma-separated allowed origins |

## Deployment Architecture

- **DaemonSet mode** (primary): runs on every node, privileged, hostPID + hostNetwork, nsenter to host.
- **Deployment mode**: 1+ replicas with PVC, zero-downtime rolling updates.
- **Probes**: startup (150s window), liveness (`/healthz:8081`), readiness (`/readyz:8081`).
- **preStop hook**: `/manager --pre-stop` (5s drain for Ingress deregistration).
- **PDB**: `minAvailable: 1`.
- **NetworkPolicy**: ingress restricted to kube-system (dashboard) + monitoring (metrics).
- **PriorityClass**: `system-cluster-critical`.

## Cron Auto-Progress Workflow

k8ops has an autonomous improvement loop (cron `*/42 * * * *`). Each run:

1. Checks pod age (skip if <5min).
2. Selects weakest of 6 dimensions: Product, Deployment, Operations, Security, Documentation, Scalability.
3. Implements a feature or improvement in that dimension.
4. Builds, tests, deploys, verifies.
5. Commits and saves progress to project memory (key: `k8ops-cron-status`).

**Current state**: v14.50, all 6 dimensions at STRONG++, 84+ tests, 12 docs, 40+ API endpoints.
