# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added — Enterprise Security Hardening
- Login rate limiting middleware (token-bucket, 5 attempts/min)
- Security headers middleware (X-Content-Type-Options, X-Frame-Options, HSTS, CSP)
- `/api/version` endpoint with build metadata (version, commit, date)
- Prometheus `/metrics` endpoint for cluster observability
- TLS/HTTPS support for dashboard server (configurable via env vars)
- Webhook notification system (Slack integration for diagnostic/remediation events)
- API key authentication (Bearer token for CLI/API integration)
- Audit log query API (`/api/audit/events` with filtering and pagination)
- Approval workflow for AI-initiated remediation actions
- Cost visibility / FinOps (namespace cost summary + rightsizing recommendations)

### Added — Documentation
- `docs/ARCHITECTURE.md` — 6-layer architecture overview and data flow
- `docs/SECURITY.md` — Security policy and feature documentation
- `docs/API.md` — Complete REST API reference (30+ endpoints)
- `CONTRIBUTING.md` — Development guidelines and PR process
- `CHANGELOG.md` — This file

### Changed — Documentation
- `README.md` — Comprehensive rewrite with enterprise feature list, architecture diagram, configuration tables, and API reference
- `config/deploy/base/deployment.yaml` — Added CORS_ALLOWED_ORIGINS, SLACK_WEBHOOK_URL, TLS env vars
- `config/deploy/values.example.yaml` — Updated with all new configuration options

## [0.1.0] — 2025-06-27

### Added — Core Platform
- AI agent with LLM-powered reasoning and tool calling (OpenAI/DeepSeek/ZAI compatible)
- Streaming chat with SSE, thinking blocks, and diff-based tool result rendering
- K8s controllers: DiagnosticReport, RemediationPlan, OptimizationSuggestion CRDs
- Tool registry: kubectl get/describe/logs/exec/events, patch, cordon, drain
- Safety checker: dry-run validation before any AI-initiated action
- Audit logger: structured JSON audit trail for all AI actions
- Resilience: token-bucket rate limiter, circuit breaker, exponential backoff
- Embedded web dashboard with overview, nodes, pods, resources, RBAC, chat views
- Kustomize deployment: base + production overlay
- CLI tool (`cmd/k8ops`) for diagnostics, remediation, and optimization

### Added — Authentication & Authorization
- Local authentication with bcrypt password hashing
- LDAP authentication (LDAPS + StartTLS)
- OIDC authentication (GitHub, Google, Microsoft, GitLab, Keycloak, Okta, Auth0)
- Role-based access control: admin / operator / viewer + namespace-scoped permissions
- K8s impersonation: all API calls executed with user's RBAC context
- Bootstrap admin on first startup with forced password change

### Security
- **P0-1**: Removed 81MB committed manager binary, added to `.gitignore`
- **P0-2**: Fixed `fmt.Sprintf("%d", bool)` vet error in `handlers_resources.go`
- **P0-3**: LDAP `InsecureSkipVerify` changed from hardcoded `true` to configurable `skip_tls_verify` (default: `false`)
- **P0-4**: CORS changed from wildcard `*` to allowlist via `CORS_ALLOWED_ORIGINS` env var with `Vary: Origin` header
- **P0-5**: OIDC state validation upgraded from string comparison to `crypto/subtle.ConstantTimeCompare` with per-provider httpOnly cookies (CSRF protection)

### Infrastructure
- **P1-6**: Graceful shutdown — SIGTERM/SIGINT handler drains SSE connections, flushes SQLite WAL, stops controller manager
- **P1-7**: Conversation TTL cleanup — background goroutine evicts idle conversations (30min timeout) and enforces max cap (1000), with mutex-protected concurrent access
- **P1-8**: JWT secret persistence — stored in K8s Secret, optional override via `AUTH_JWT_SECRET_PATH`, fallback to auto-generated
- **P1-9**: PVC persistence verified — SQLite data path confirmed through PVC → /data → SQLite chain
- **P1-11**: Tool registry concurrency protection — `sync.RWMutex` on Register/Get/List/Definitions operations

### Code Quality
- **P2-12**: Fixed 8 instances of ignored errors across 5 files — changed `_ = err` to `slog.Warn()` logging:
  - `handlers.go`: RBAC sync after create, cleanup on delete, RBAC sync after update, OIDC UpdateUser (4 sites)
  - `provider_handlers.go`: config unmarshal, LDAP UpdateUser (2 sites)
  - `rbac_sync.go`: RoleBinding Delete in two loops (2 sites)
  - `providers.go`: config unmarshal (2 sites)
  - `oidc.go`: Claims unmarshal (1 site)
- **P2-15**: SQLite PRAGMA deduplication — kept explicit `db.Exec("PRAGMA ...")` calls with explanatory comment (GORM DSN query params unreliable with glebarez driver)

### Tests — Added Coverage (from 0% to significant)
- `internal/auth/auth_test.go` — 20 tests: JWT generation/verification (valid/expired/wrong-secret/malformed/alg=none), login (success/wrong-password/non-existent/non-local), password change, admin create/reset user
- `internal/auth/store_test.go` — 8 tests: User/AuthProvider/RoleDef CRUD, duplicate username, enabled-provider filtering
- `internal/auth/middleware_test.go` — 16 tests: public route matching, API request detection, admin-only enforcement, cookie set/clear attributes
- `internal/auth/providers_test.go` — 7 tests: API config secret masking, config parse/set round-trip, provider presets
- `internal/auth/oidc_state_test.go` — 12 tests: ConstantTimeCompare (match/mismatch/empty/long), per-provider cookie naming, set/clear lifecycle, Secure flag detection
- `internal/auth/rbac_sync_test.go` — 12 tests: namespace splitting, RBAC sync (admin/viewer/cleanup), namespace creation, nil safety
- `internal/chat/engine_pure_test.go` — 31 subtests: engine construction, message truncation, error event formatting, tool conversion, retryable error classification (429/500/502/503/timeout vs 400/401/403/404)
- `internal/chat/engine_test.go` — 10 tests: TTL eviction (expired/active/boundary/multi), cap enforcement, start/stop cleanup, concurrent access, last-activity tracking
- `internal/tools/registry_test.go` — 11 tests: tool registration, lookup, listing, concurrent access (4 writers + 8 readers)

### Known Limitations
- No multi-cluster support (single cluster per instance)
- No WebSocket terminal (exec via tool only, not interactive)
- Host tools bypass K8s RBAC impersonation
- No Helm chart (Kustomize only)
- bcrypt operations slow under `-race` (TestChangePassword_NonLocalUser may timeout)
