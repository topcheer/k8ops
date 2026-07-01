# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v14.1] — 2026-07-01

### Added
- AI Diagnostic Action Cards — kubectl/shell code blocks auto-enhanced with Run+Copy buttons
- GitHub Actions CI/CD — `.github/workflows/ci.yml` (test+lint+docker build), `release.yml` (GoReleaser)
- GoReleaser config — multi-platform binary builds (linux/darwin, amd64/arm64), Docker multi-arch manifests
- User Guide (`docs/USER_GUIDE.md`) — comprehensive 15-section user manual
- Local Run Guide (`docs/LOCAL_RUN.md`) — bare-metal binary execution guide

### Changed
- Agent context management confirmed working: memory.Conversation with 20k token threshold, LLM-based summarization

## [v14.0] — 2026-07-01

### Changed — Breaking
- **ES Modules migration** — all frontend JS files converted to ES modules
  - New `modules/utils.js` shared utility module (single source of truth for escapeHtml, fetchJSON, etc.)
  - New `main.js` entry point bridges module exports to `window` for inline handler compatibility
  - `index.html` uses single `<script type="module" src="/main.js">` instead of 9 script tags
  - Zero duplicate function definitions (previously `escapeHtml` existed in both core.js and chat.js)

## [v13.11] — 2026-07-01

### Fixed
- **P0: CSP blocking inline JS** — `script-src 'self'` prevented all 87 inline event handlers from working; added `'unsafe-inline'`
- **P1: gzip middleware SSE panic** — `Flush()` operated on closed gzip writer; added `gzClosed` flag
- **P1: closeLogViewer stale reference** — `logEventSource` replaced with `logFetchController` (old SSE viewer)
- **P1: duplicate clearLogs** — removed dead-code version, kept correct v13.4 implementation
- **P2: chat.js duplicate comment** — merged two "Step 1" annotation lines

## [v13.10] — 2026-07-01

### Added
- Audit log query+filter UI — severity dropdown, stats cards (Total/Success/Failed/Critical/Warning), severity badges
- Natural language → kubectl — enhanced system prompt with explicit NL examples, AI auto-translates queries to tool calls

## [v13.9] — 2026-07-01

### Added
- Dark/light theme toggle — 14 CSS variables, light theme override blocks, localStorage persistence, sidebar toggle button
- YAML editor — edit mode with textarea, server-side apply via `POST /api/yaml/apply`, dry-run support, success/error feedback

### Changed
- Dockerfile binary size optimization — `-ldflags="-s -w"` reduces binary from 83MB → 58MB

## [v13.8] — 2026-07-01

### Added
- API gzip compression — `compress/gzip` middleware for `/api/` JSON responses, SSE excluded
- Table search filter — real-time search input on Nodes/Pods/Events tables with match count badge

## [v13.7] — 2026-07-01

### Added
- Cluster topology visualization — SVG node→Pod graph, health status colors, resource bars, crash loop pulse animation
- Notification center — bell icon with pulse badge, dropdown panel with warning/critical alerts, 60s polling

## [v13.6] — 2026-06-30

### Added
- Live event stream UI — EventSource consuming `/api/events/stream`, real-time scrolling feed, NEW/DEL badges, warning highlighting
- Multi-namespace switcher — top bar dropdown, localStorage persistence, affects Pods/Events/Nodes

## [v13.5] — 2026-06-30

### Added
- Node resource utilization bars — per-node CPU/MEM/Pod usage with color coding
- Sparkline mini charts — SVG trend charts on overview cards for node/warning history

## [v13.4] — 2026-06-30

### Added
- Pod log viewer upgrade — SSE streaming, log level highlighting (ERROR/WARN/DEBUG), search filter, auto-scroll toggle, download

## [v13.3] — 2026-06-30

### Added
- Real-time event SSE stream — `/api/events/stream` endpoint with K8s Watch integration

## [v13.2] — 2026-06-30

### Added
- Ctrl+K Command Palette — global search + quick navigation
- Code block copy button — copy-to-clipboard on chat code blocks

## [v13.1] — 2026-06-30

### Added
- Per-user chat rate limiting — 20 burst, 10 req/min per user
- Health/readiness probes — `/healthz` and `/readyz` endpoints for K8s

## [v13.0] — 2026-06-30

### Security
- **P0 XSS fix** — markdown link URL sanitization (allowlist http/https/mailto only)
- **P0 CSP header** — Content-Security-Policy added to all responses
- **P0 SSE timeout fix** — `WriteTimeout` set to 0 for long AI streaming responses

### Changed
- **P1 imagePullPolicy** — changed from `IfNotPresent` to `Always` (required for mutable `:latest` tag)

### Added
- Per-user rate limiting infrastructure
- Backup/HA documentation in DEPLOYMENT.md (MySQL HA + CronJob backup strategy)

## [v0.1.0] — 2026-03-15

### Added
- Initial release
- AI-powered diagnostic engine with streaming LLM responses
- Auto-remediation with human-in-the-loop approval workflow
- K8s resource optimization recommendations
- Embedded dashboard web UI (vanilla HTML/JS/CSS)
- Kustomize deployment (base + production overlay)
- SQLite + K8s CRD persistence
- Multi-provider AI support (OpenAI, DeepSeek, ZAI, Anthropic)
- Audit logging with severity levels
- RBAC with namespace scoping via impersonation
- Cost visibility / FinOps module
- Prometheus metrics export
- CLI tool (`k8ops` command)

### Known Limitations
- No WebSocket terminal (exec via tool only, not interactive)
- Host tools bypass K8s RBAC impersonation
- No Helm chart (Kustomize only)
- bcrypt operations slow under `-race` (TestChangePassword_NonLocalUser may timeout)

[v14.1]: https://github.com/topcheer/k8ops/releases/tag/v14.1
[v14.0]: https://github.com/topcheer/k8ops/releases/tag/v14.0
[v13.11]: https://github.com/topcheer/k8ops/releases/tag/v13.11
[v13.10]: https://github.com/topcheer/k8ops/releases/tag/v13.10
[v13.9]: https://github.com/topcheer/k8ops/releases/tag/v13.9
[v13.8]: https://github.com/topcheer/k8ops/releases/tag/v13.8
[v13.7]: https://github.com/topcheer/k8ops/releases/tag/v13.7
[v13.6]: https://github.com/topcheer/k8ops/releases/tag/v13.6
[v13.5]: https://github.com/topcheer/k8ops/releases/tag/v13.5
[v13.4]: https://github.com/topcheer/k8ops/releases/tag/v13.4
[v13.3]: https://github.com/topcheer/k8ops/releases/tag/v13.3
[v13.2]: https://github.com/topcheer/k8ops/releases/tag/v13.2
[v13.1]: https://github.com/topcheer/k8ops/releases/tag/v13.1
[v13.0]: https://github.com/topcheer/k8ops/releases/tag/v13.0
[v0.1.0]: https://github.com/topcheer/k8ops/releases/tag/v0.1.0
