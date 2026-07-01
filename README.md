# k8ops — Kubernetes AI Operations Operator

<div align="center">

**A Kubernetes AIOps operator that diagnoses issues, auto-remediates, and optimizes your cluster using AI.**

[![GitHub release](https://img.shields.io/github/v/release/topcheer/k8ops?style=flat-square)](https://github.com/topcheer/k8ops/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/topcheer/k8ops/ci.yml?branch=main&style=flat-square&label=CI)](https://github.com/topcheer/k8ops/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/topcheer/k8ops?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![Docker](https://img.shields.io/badge/Docker-ghcr.io-2496ED?style=flat-square&logo=docker)](https://github.com/topcheer/k8ops/pkgs/container/k8ops)
[![Built with ggcode](https://img.shields.io/badge/Built%20with-ggcode-6C43BC?style=flat-square)](https://github.com/topcheer/ggcode)

</div>

---

## Features

### AI-Powered Operations
- **Intelligent Diagnostics** — Submit a problem description, get AI-driven root-cause analysis with tool-augmented reasoning (kubectl describe, logs, events, metrics)
- **Auto-Remediation** — AI proposes and (with approval) executes safe remediation actions: restart pods, scale deployments, clean up resources
- **Optimization Suggestions** — Continuous analysis of resource usage, HPA/PDB gaps, and cost-saving opportunities
- **Streaming Chat** — Real-time SSE streaming with thinking blocks, tool call transparency, and diff-based result rendering

### Enterprise Security
- **Multi-Provider Auth** — Local (bcrypt), LDAP (with configurable TLS verification), OIDC (GitHub, Google, GitLab, Keycloak, Okta, Auth0, Microsoft)
- **RBAC** — Role-based access control with admin/operator/viewer roles and namespace-scoped permissions
- **OIDC CSRF Protection** — Per-provider state cookies with `ConstantTimeCompare` validation
- **CORS Allowlist** — Origin-based allowlist (no wildcard with credentials), `Vary: Origin` header
- **Audit Logging** — Every AI action, tool execution, and LLM call recorded as structured audit events
- **JWT Persistence** — Signed JWT secrets stored in K8s Secrets with optional fallback
- **Rate Limiting** — Token-bucket rate limiter on login endpoints to prevent brute-force attacks
- **Security Headers** — X-Content-Type-Options, X-Frame-Options, HSTS, CSP

### Operations & Reliability
- **Graceful Shutdown** — SIGTERM/SIGINT handling with SSE drain, SQLite WAL flush, and controller stop
- **Conversation TTL** — Automatic cleanup of idle chat sessions (30min timeout, 1000 max conversations)
- **Circuit Breaker** — Resilient LLM calls with configurable retry, backoff, and circuit breaking
- **Prometheus Metrics** — Cluster health gauges, conversation counters, tool execution metrics

### Deployment
- **Kustomize** — Base + overlay deployment with production-ready defaults
- **Embedded Web UI** — Single binary, no external frontend dependencies
- **SQLite + K8s CRDs** — Lightweight persistence, no external database required
- **PVC Persistence** — Data survives pod restarts

---

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Dashboard / Web UI                     │
│  (Embedded SPA + REST API + SSE streaming)               │
├─────────────────────────────────────────────────────────┤
│            Auth (Local/LDAP/OIDC) + RBAC                 │
├─────────────────────────────────────────────────────────┤
│                      AI Agent                            │
│  (LLM reasoning + tool calling + streaming)              │
├──────────┬──────────┬──────────┬────────────────────────┤
│  Chat    │  Safety  │  Audit   │  Resilience            │
│  Engine  │  Checker │  Logger  │  (Circuit Breaker)     │
├──────────┴──────────┴──────────┴────────────────────────┤
│                    Tool Registry                         │
│  (kubectl get/describe/logs, exec, events, metrics)      │
├─────────────────────────────────────────────────────────┤
│              Controller Runtime + CRDs                   │
│  (DiagnosticReport, RemediationPlan, OptimizationSuggestion) │
├─────────────────────────────────────────────────────────┤
│                   Kubernetes API                         │
│  (Impersonation: user-scoped RBAC)                       │
└─────────────────────────────────────────────────────────┘
```

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for detailed component documentation.

---

## Quick Start

### Prerequisites
- Kubernetes 1.24+ (k3s / k8s / EKS / GKE / AKS)
- kubectl configured
- An LLM API key (OpenAI, DeepSeek, ZAI, or any OpenAI-compatible provider)

### 1. Deploy to Kubernetes

**Option A: Deployment mode (recommended)**

```bash
# One command — includes namespace, RBAC, secrets, ingress, TLS
kubectl apply -k config/deploy/overlays/local

# Or create your own overlay
cp -r config/deploy/overlays/local config/deploy/overlays/myorg
# Edit myorg/kustomization.yaml: set your domain, registry, CORS
kubectl apply -k config/deploy/overlays/myorg
```

**Option B: DaemonSet mode (per-node diagnostics)**

```bash
kubectl apply -f config/daemonset-local.yaml
```

**Option C: install.sh (interactive)**

```bash
./install.sh install    # deploy
./install.sh status     # check status
./install.sh uninstall  # remove
```

See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for detailed deployment guide.

### 2. Configure LLM Provider

```bash
# Via Dashboard: Settings tab → fill in provider type, API key, model
# Or via environment variables in the overlay ConfigMap:

configMapGenerator:
- name: k8ops-config
  literals:
  - PROVIDER_TYPE=openai
  - PROVIDER_MODEL=gpt-4o
  - PROVIDER_ENDPOINT=https://api.openai.com/v1

# API key via Secret:
secretGenerator:
- name: k8ops-credentials
  literals:
  - api-key=sk-your-key-here
```

### 3. Access the Dashboard

```bash
# Via Ingress (if configured)
# Open https://<your-domain>  (e.g. https://k8ops.iot2.win)

# Or port-forward
kubectl port-forward svc/k8ops-dashboard 9090:9090 -n k8ops-system
# Open http://localhost:9090
# Default login: admin / admin (will prompt password change)
```

### 4. Trigger a Diagnostic

```bash
# Via kubectl (CRD)
kubectl apply -f examples/diagnostic.yaml

# Via CLI
go run ./cmd/k8ops diagnose --problem "pods in production keep CrashLoopBackOff"

# Via the web dashboard chat interface
```

---

## Configuration

All configuration is via ConfigMap/Secret (managed by Kustomize overlays). See [config/deploy/overlays/local/kustomization.yaml](config/deploy/overlays/local/kustomization.yaml) for a working example.

### Core
| Variable | Default | Description |
|----------|---------|-------------|
| `PROVIDER_TYPE` | `openai` | LLM provider type |
| `PROVIDER_MODEL` | `gpt-4o` | Model name |
| `PROVIDER_ENDPOINT` | `https://api.openai.com/v1` | LLM provider base URL |
| `AIOPS_API_KEY` | (required) | LLM API key (from Secret) |

### Security
| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_JWT_SECRET` | (auto-generated) | JWT signing secret (persisted in K8s Secret) |
| `CORS_ALLOWED_ORIGINS` | (empty) | Comma-separated allowed origins |
| `LDAP_SERVER` | (empty) | LDAP server URL |
| `LDAP_SKIP_TLS_VERIFY` | `false` | Skip LDAP TLS certificate verification |
| `OIDC_ISSUER` | (empty) | OIDC issuer URL |

### Notifications
| Variable | Default | Description |
|----------|---------|-------------|
| `SLACK_WEBHOOK_URL` | (empty) | Slack incoming webhook for notifications |

### AI / Chat
| Variable | Default | Description |
|----------|---------|-------------|
| `MAX_STEPS` | `15` | Max agent reasoning steps per request |
| `CONVERSATION_TTL` | `30m` | Idle conversation timeout |
| `MAX_CONVERSATIONS` | `1000` | Maximum concurrent conversations |

---

## API

The dashboard exposes a REST API at `http://<host>:9090/api/`. Key endpoints:

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/health` | Health check | Public |
| GET | `/api/version` | Build version | Public |
| GET | `/api/cluster/overview` | Cluster summary | Viewer+ |
| GET | `/api/cluster/nodes` | Node list + health | Viewer+ |
| GET | `/api/cluster/pods` | Pod list with status | Viewer+ |
| POST | `/api/chat/stream` | AI chat (SSE streaming) | Viewer+ |
| GET | `/api/resources/{type}` | K8s resource query | Viewer+ |
| POST | `/api/auth/login` | Local/LDAP login | Public |
| GET | `/api/auth/status` | Auth config + providers | Public |
| GET | `/api/auth/providers` | List auth providers | Admin |
| GET/POST | `/api/rbac/users` | User management | Admin |
| GET/POST | `/api/rbac/roles` | Role management | Admin |

See [docs/API.md](docs/API.md) for the complete API reference.

---

## Development

### Prerequisites
- Go 1.22+
- kubectl (for integration tests)
- Access to a Kubernetes cluster (for controller tests)

### Build & Test

```bash
# Build the manager binary
make build

# Run all tests
make test

# Run tests with race detector
go test -race -count=1 ./internal/...

# Generate CRDs
make manifests

# Build Docker image
make docker-build IMG=ghcr.io/topcheer/k8ops:latest
```

### Project Structure

```
k8ops/
├── api/v1alpha1/           # CRD type definitions
├── cmd/
│   ├── manager/            # Operator entry point
│   └── k8ops/              # CLI tool
├── config/
│   ├── crd/                # CRD manifests
│   ├── deploy/             # Kustomize deployment (base + overlays)
│   │   ├── base/           # Namespace, SA, RBAC, Deployment, Service, Ingress
│   │   └── overlays/
│   │       ├── local/      # Local network overlay (registry, domain, CORS)
│   │       └── prod/       # Production overlay template
│   └── daemonset/          # DaemonSet manifests (per-node deployment)
├── internal/
│   ├── agent/              # AI agent (reasoning + tool calling)
│   ├── audit/              # Structured audit logging
│   ├── auth/               # Authentication (Local/LDAP/OIDC) + RBAC
│   ├── chat/               # Chat engine with conversation management
│   ├── collector/          # Cluster event collector
│   ├── controller/         # CRD controllers (diagnostic/optimization/remediation)
│   ├── dashboard/          # Web UI + REST API
│   │   └── web/            # Embedded frontend (HTML/JS/CSS)
│   ├── memory/             # Conversation memory store
│   ├── metrics/            # Prometheus metrics
│   ├── provider/           # LLM provider interface
│   ├── providermanager/    # Multi-provider management
│   ├── resilience/         # Circuit breaker + rate limiter
│   ├── safety/             # Safety checker (dry-run validation)
│   └── tools/              # K8s and host tools (kubectl, exec, etc.)
├── docs/                   # Architecture, API, Security, Deployment docs
├── install.sh              # One-click install/uninstall script
├── .env.example            # Environment variable reference
└── examples/               # Example CRD manifests
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines.

---

## Local Development

Run k8ops directly on your workstation without a Kubernetes deployment:

```bash
# Build
go build -o k8ops-manager ./cmd/manager/

# Run
AIOPS_API_KEY=your-key ./k8ops-manager \
  --leader-elect=false \
  --dashboard-address=:9090 \
  --auth-db-path=/tmp/k8ops.db
```

The binary automatically discovers your kubeconfig (`~/.kube/config`), so all K8s data comes from your connected cluster. See [docs/LOCAL_RUN.md](docs/LOCAL_RUN.md) for details.

---

## Documentation

| Document | Description |
|----------|-------------|
| [docs/USER_GUIDE.md](docs/USER_GUIDE.md) | Comprehensive user manual (all 15 features) |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | System architecture and component design |
| [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) | Deployment guide (Deployment / DaemonSet / Helm) |
| [docs/LOCAL_RUN.md](docs/LOCAL_RUN.md) | Running k8ops binary locally (no K8s deployment) |
| [docs/API.md](docs/API.md) | REST API reference |
| [docs/SECURITY.md](docs/SECURITY.md) | Security policy and RBAC model |
| [CHANGELOG.md](CHANGELOG.md) | Release history (v0.1.0 → v14.1) |

---

## Security

See [SECURITY.md](SECURITY.md) for the full security policy, including:
- Authentication methods and configuration
- RBAC model and namespace scoping
- Reported vulnerability handling

---

## Changelog

See [CHANGELOG.md](CHANGELOG.md).

---

## License

GNU Affero General Public License v3.0 (AGPL-3.0). See [LICENSE](LICENSE).
