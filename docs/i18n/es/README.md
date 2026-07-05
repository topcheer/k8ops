# k8ops — Kubernetes AI Operations Operator

<div align="center">

**Un operador AIOps de Kubernetes que diagnostica problemas, aplica remediación automática y optimiza su clúster mediante IA.**

[![GitHub release](https://img.shields.io/github/v/release/topcheer/k8ops?style=flat-square)](https://github.com/topcheer/k8ops/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/topcheer/k8ops/ci.yml?branch=main&style=flat-square&label=CI)](https://github.com/topcheer/k8ops/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/topcheer/k8ops?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![Docker](https://img.shields.io/badge/Docker-ghcr.io-2496ED?style=flat-square&logo=docker)](https://github.com/topcheer/k8ops/pkgs/container/k8ops)
[![Built with ggcode](https://img.shields.io/badge/Built%20with-ggcode-6C43BC?style=flat-square)](https://github.com/topcheer/ggcode)

</div>

---

**Idiomas:** [English](../../README.md) | [中文](../zh-CN/README.md) | [日本語](../ja/README.md) | [한국어](../ko/README.md) | [Español](README.md) | [Français](../fr/README.md) | [Deutsch](../de/README.md)

---

## Características

### Operaciones Impulsadas por IA
- **Diagnóstico Inteligente** — Envíe una descripción del problema y obtenga un análisis de causa raíz impulsado por IA con razonamiento aumentado por herramientas (kubectl describe, logs, eventos, métricas)
- **Remediación Automática** — La IA propone y (con aprobación) ejecuta acciones de remediación seguras: reiniciar pods, escalar deployments, limpiar recursos
- **Sugerencias de Optimización** — Análisis continuo del uso de recursos, brechas de HPA/PDB y oportunidades de ahorro de costos
- **Chat en Streaming** — Streaming SSE en tiempo real con bloques de razonamiento, transparencia en llamadas a herramientas y renderización de resultados basada en diffs

### Seguridad Empresarial
- **Autenticación Multi-Proveedor** — Local (bcrypt), LDAP (con verificación TLS configurable), OIDC (GitHub, Google, GitLab, Keycloak, Okta, Auth0, Microsoft)
- **RBAC** — Control de acceso basado en roles con roles de administrador/operador/visualizador y permisos por namespace
- **Protección CSRF de OIDC** — Cookies de estado por proveedor con validación `ConstantTimeCompare`
- **Lista de Permitidos CORS** — Lista de permitidos basada en origen (sin comodines con credenciales), cabecera `Vary: Origin`
- **Registro de Auditoría** — Cada acción de IA, ejecución de herramienta y llamada LLM registrada como evento de auditoría estructurado
- **Persistencia de JWT** — Secretos JWT firmados almacenados en K8s Secrets con respaldo opcional
- **Limitación de Tasa** — Limitador de tasa de tipo token-bucket en endpoints de inicio de sesión para prevenir ataques de fuerza bruta
- **Cabeceras de Seguridad** — X-Content-Type-Options, X-Frame-Options, HSTS, CSP

### Operaciones y Confiabilidad
- **Apagado Elegante** — Manejo de SIGTERM/SIGINT con drenado de SSE, vaciado de WAL de SQLite y detención del controlador
- **TTL de Conversación** — Limpieza automática de sesiones de chat inactivas (tiempo de espera de 30 min, máximo 1000 conversaciones)
- **Interruptor de Circuito** — Llamadas LLM resilientes con reintento configurable, retroceso exponencial y apertura del circuito
- **Métricas de Prometheus** — Indicadores de salud del clúster, contadores de conversaciones, métricas de ejecución de herramientas

### Despliegue
- **Kustomize** — Despliegue con base + overlay y valores predeterminados listos para producción
- **Interfaz Web Integrada** — Un solo binario, sin dependencias de frontend externas
- **SQLite + CRDs de K8s** — Persistencia ligera, sin base de datos externa requerida
- **Persistencia con PVC** — Los datos sobreviven a los reinicios de los pods

---

## Arquitectura

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

Consulte [docs/ARCHITECTURE.md](../../ARCHITECTURE.md) para la documentación detallada de componentes.

---

## Inicio Rápido

### Requisitos Previos
- Kubernetes 1.24+ (k3s / k8s / EKS / GKE / AKS)
- kubectl configurado
- Una clave API de LLM (OpenAI, DeepSeek, ZAI o cualquier proveedor compatible con OpenAI)

### 1. Desplegar en Kubernetes

**Opción A: Modo Deployment (recomendado)**

```bash
# Un solo comando — incluye namespace, RBAC, secrets, ingress, TLS
kubectl apply -k config/deploy/overlays/local

# O cree su propio overlay
cp -r config/deploy/overlays/local config/deploy/overlays/myorg
# Edite myorg/kustomization.yaml: establezca su dominio, registro, CORS
kubectl apply -k config/deploy/overlays/myorg
```

**Opción B: Modo DaemonSet (diagnóstico por nodo)**

```bash
kubectl apply -f config/daemonset-local.yaml
```

**Opción C: install.sh (interactivo)**

```bash
./install.sh install    # desplegar
./install.sh status     # verificar estado
./install.sh uninstall  # eliminar
```

Consulte [docs/DEPLOYMENT.md](../../DEPLOYMENT.md) para la guía de despliegue detallada.

### 2. Configurar el Proveedor LLM

```bash
# A través del Dashboard: pestaña Settings → complete el tipo de proveedor, clave API, modelo
# O mediante variables de entorno en el ConfigMap del overlay:

configMapGenerator:
- name: k8ops-config
  literals:
  - PROVIDER_TYPE=openai
  - PROVIDER_MODEL=gpt-4o
  - PROVIDER_ENDPOINT=https://api.openai.com/v1

# Clave API mediante Secret:
secretGenerator:
- name: k8ops-credentials
  literals:
  - api-key=sk-your-key-here
```

### 3. Acceder al Dashboard

```bash
# A través de Ingress (si está configurado)
# Abra https://<su-dominio>  (ej. https://k8ops.iot2.win)

# O mediante port-forward
kubectl port-forward svc/k8ops-dashboard 9090:9090 -n k8ops-system
# Abra http://localhost:9090
# Inicio de sesión predeterminado: admin / admin (solicitará cambio de contraseña)
```

### 4. Iniciar un Diagnóstico

```bash
# Mediante kubectl (CRD)
kubectl apply -f examples/diagnostic.yaml

# Mediante CLI
go run ./cmd/k8ops diagnose --problem "pods in production keep CrashLoopBackOff"

# A través de la interfaz de chat del dashboard web
```

---

## Configuración

Toda la configuración se realiza mediante ConfigMap/Secret (gestionados por overlays de Kustomize). Consulte [config/deploy/overlays/local/kustomization.yaml](config/deploy/overlays/local/kustomization.yaml) para un ejemplo funcional.

### Núcleo
| Variable | Predeterminado | Descripción |
|----------|---------------|-------------|
| `PROVIDER_TYPE` | `openai` | Tipo de proveedor LLM |
| `PROVIDER_MODEL` | `gpt-4o` | Nombre del modelo |
| `PROVIDER_ENDPOINT` | `https://api.openai.com/v1` | URL base del proveedor LLM |
| `AIOPS_API_KEY` | (obligatorio) | Clave API del LLM (desde Secret) |

### Seguridad
| Variable | Predeterminado | Descripción |
|----------|---------------|-------------|
| `AUTH_JWT_SECRET` | (auto-generado) | Secreto de firma JWT (persistido en K8s Secret) |
| `CORS_ALLOWED_ORIGINS` | (vacío) | Orígenes permitidos separados por comas |
| `LDAP_SERVER` | (vacío) | URL del servidor LDAP |
| `LDAP_SKIP_TLS_VERIFY` | `false` | Omitir verificación de certificado TLS de LDAP |
| `OIDC_ISSUER` | (vacío) | URL del emisor OIDC |

### Notificaciones
| Variable | Predeterminado | Descripción |
|----------|---------------|-------------|
| `SLACK_WEBHOOK_URL` | (vacío) | Webhook entrante de Slack para notificaciones |

### IA / Chat
| Variable | Predeterminado | Descripción |
|----------|---------------|-------------|
| `MAX_STEPS` | `15` | Pasos máximos de razonamiento del agente por solicitud |
| `CONVERSATION_TTL` | `30m` | Tiempo de espera de conversación inactiva |
| `MAX_CONVERSATIONS` | `1000` | Máximo de conversaciones concurrentes |

---

## API

El dashboard expone una API REST en `http://<host>:9090/api/`. Endpoints principales:

| Método | Ruta | Descripción | Auth |
|--------|------|-------------|------|
| GET | `/api/health` | Verificación de salud | Público |
| GET | `/api/version` | Versión del build | Público |
| GET | `/api/cluster/overview` | Resumen del clúster | Viewer+ |
| GET | `/api/cluster/nodes` | Lista de nodos + salud | Viewer+ |
| GET | `/api/cluster/pods` | Lista de pods con estado | Viewer+ |
| POST | `/api/chat/stream` | Chat con IA (streaming SSE) | Viewer+ |
| GET | `/api/resources/{type}` | Consulta de recursos K8s | Viewer+ |
| POST | `/api/auth/login` | Inicio de sesión Local/LDAP | Público |
| GET | `/api/auth/status` | Configuración de autenticación + proveedores | Público |
| GET | `/api/auth/providers` | Listar proveedores de autenticación | Admin |
| GET/POST | `/api/rbac/users` | Gestión de usuarios | Admin |
| GET/POST | `/api/rbac/roles` | Gestión de roles | Admin |

Consulte [docs/API.md](../../API.md) para la referencia completa de la API.

---

## Desarrollo

### Requisitos Previos
- Go 1.22+
- kubectl (para pruebas de integración)
- Acceso a un clúster de Kubernetes (para pruebas del controlador)

### Compilar y Probar

```bash
# Compilar el binario del manager
make build

# Ejecutar todas las pruebas
make test

# Ejecutar pruebas con detector de race conditions
go test -race -count=1 ./internal/...

# Generar CRDs
make manifests

# Compilar imagen Docker
make docker-build IMG=ghcr.io/topcheer/k8ops:latest
```

### Estructura del Proyecto

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

Consulte [CONTRIBUTING.md](CONTRIBUTING.md) para las directrices de desarrollo.

---

## Desarrollo Local

Ejecute k8ops directamente en su estación de trabajo sin un despliegue de Kubernetes:

```bash
# Compilar
go build -o k8ops-manager ./cmd/manager/

# Ejecutar
AIOPS_API_KEY=your-key ./k8ops-manager \
  --leader-elect=false \
  --dashboard-address=:9090 \
  --auth-db-path=/tmp/k8ops.db
```

El binario descubre automáticamente su kubeconfig (`~/.kube/config`), por lo que todos los datos de K8s provienen de su clúster conectado. Consulte [docs/LOCAL_RUN.md](../../LOCAL_RUN.md) para más detalles.

---

## Documentación

| Documento | Descripción |
|----------|-------------|
| [docs/USER_GUIDE.md](../../USER_GUIDE.md) | Manual de usuario completo (las 15 características) |
| [docs/ARCHITECTURE.md](../../ARCHITECTURE.md) | Arquitectura del sistema y diseño de componentes |
| [docs/DEPLOYMENT.md](../../DEPLOYMENT.md) | Guía de despliegue (Deployment / DaemonSet / Helm) |
| [docs/LOCAL_RUN.md](../../LOCAL_RUN.md) | Ejecución del binario k8ops localmente (sin despliegue K8s) |
| [docs/API.md](../../API.md) | Referencia de la API REST |
| [docs/SECURITY.md](../../SECURITY.md) | Política de seguridad y modelo RBAC |
| [CHANGELOG.md](../../CHANGELOG.md) | Historial de versiones (v0.1.0 → v14.1) |

---

## Seguridad

Consulte [SECURITY.md](../../SECURITY.md) para la política de seguridad completa, que incluye:
- Métodos de autenticación y configuración
- Modelo RBAC y alcance por namespace
- Gestión de vulnerabilidades reportadas

---

## Registro de Cambios

Consulte [CHANGELOG.md](../../CHANGELOG.md).

---

## Licencia

GNU Affero General Public License v3.0 (AGPL-3.0). Consulte [LICENSE](LICENSE).
