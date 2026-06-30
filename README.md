# k8ops — Kubernetes AI Operations Operator

<div align="center">

**A Kubernetes AIOps operator that diagnoses issues, auto-remediates, and optimizes your cluster using AI.**

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
- Kubernetes 1.24+
- kubectl configured
- An LLM API key (OpenAI, DeepSeek, ZAI, or any OpenAI-compatible provider)

### 1. Deploy to Kubernetes

```bash
# Clone and enter the repo
git clone https://github.com/ggai/k8ops.git
cd k8ops

# Deploy with Kustomize
kubectl apply -k config/deploy/base

# Or with production overlay (custom domain + TLS)
cp -r config/deploy/overlays/prod config/deploy/overlays/myorg
# Edit myorg/kustomization.yaml with your domain + TLS
kubectl apply -k config/deploy/overlays/myorg
```

### 2. Configure LLM Provider

```bash
# Set your LLM API key (required)
kubectl set env deployment/k8ops-manager \
  LLM_API_KEY=your-api-key \
  LLM_BASE_URL=https://api.openai.com/v1 \
  LLM_MODEL=gpt-4o \
  -n k8ops

# Or use DeepSeek / ZAI / any OpenAI-compatible provider
kubectl set env deployment/k8ops-manager \
  LLM_API_KEY=your-key \
  LLM_BASE_URL=https://api.deepseek.com/v1 \
  LLM_MODEL=deepseek-chat \
  -n k8ops
```

### 3. Access the Dashboard

```bash
# Port-forward to access locally
kubectl port-forward svc/k8ops-dashboard 9090:9090 -n k8ops

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

All configuration is via environment variables. See [config/deploy/values.example.yaml](config/deploy/values.example.yaml) for the full list.

### Core
| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `9090` | Dashboard listen port |
| `LLM_API_KEY` | (required) | LLM provider API key |
| `LLM_BASE_URL` | `https://api.openai.com/v1` | LLM provider base URL |
| `LLM_MODEL` | `gpt-4o` | Model name |
| `LLM_SYSTEM_PROMPT` | (built-in) | Custom system prompt for AI agent |

### Security
| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_JWT_SECRET` | (auto-generated) | JWT signing secret (persisted in K8s Secret) |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:9090` | Comma-separated allowed origins |
| `DASHBOARD_TLS_CERT` | (empty) | TLS cert file path (enables HTTPS) |
| `DASHBOARD_TLS_KEY` | (empty) | TLS key file path (enables HTTPS) |
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
make docker-build IMG=ghcr.io/ggai/k8ops:latest
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
│   └── deploy/             # Kustomize deployment
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
└── examples/               # Example CRD manifests
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines.

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

Apache License 2.0
