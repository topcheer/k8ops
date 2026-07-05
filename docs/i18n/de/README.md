# k8ops — Kubernetes AI Operations Operator

<div align="center">

**Ein Kubernetes-AIOps-Operator, der Probleme diagnostiziert, automatisch behebt und Ihren Cluster mithilfe von KI optimiert.**

[![GitHub release](https://img.shields.io/github/v/release/topcheer/k8ops?style=flat-square)](https://github.com/topcheer/k8ops/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/topcheer/k8ops/ci.yml?branch=main&style=flat-square&label=CI)](https://github.com/topcheer/k8ops/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/topcheer/k8ops?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![Docker](https://img.shields.io/badge/Docker-ghcr.io-2496ED?style=flat-square&logo=docker)](https://github.com/topcheer/k8ops/pkgs/container/k8ops)
[![Built with ggcode](https://img.shields.io/badge/Built%20with-ggcode-6C43BC?style=flat-square)](https://github.com/topcheer/ggcode)

</div>

---

**Sprachen:** [English](../../README.md) | [中文](../zh-CN/README.md) | [日本語](../ja/README.md) | [한국어](../ko/README.md) | [Español](../es/README.md) | [Français](../fr/README.md) | [Deutsch](README.md)

---

## Funktionen

### KI-gestützte Operationen
- **Intelligente Diagnose** — Beschreiben Sie ein Problem und erhalten Sie eine KI-gestützte Ursachenanalyse mit werkzeuggestütztem Reasoning (kubectl describe, logs, events, metrics)
- **Automatische Fehlerbehebung** — Die KI schlägt sichere Behebungsmaßnahmen vor und führt diese (nach Freigabe) aus: Pods neu starten, Deployments skalieren, Ressourcen bereinigen
- **Optimierungsvorschläge** — Kontinuierliche Analyse der Ressourcennutzung, HPA/PDB-Lücken und Kostensenkungsmöglichkeiten
- **Streaming-Chat** — Echtzeit-SSE-Streaming mit Thinking-Blöcken, Transparenz bei Werkzeugaufrufen und diff-basierter Ergebnisdarstellung

### Unternehmenssicherheit
- **Multi-Provider-Authentifizierung** — Local (bcrypt), LDAP (mit konfigurierbarer TLS-Verifizierung), OIDC (GitHub, Google, GitLab, Keycloak, Okta, Auth0, Microsoft)
- **RBAC** — Rollenbasierte Zugriffssteuerung mit Admin/Operator/Viewer-Rollen und Namespace-bezogenen Berechtigungen
- **OIDC-CSRF-Schutz** — Pro-Provider-State-Cookies mit `ConstantTimeCompare`-Validierung
- **CORS-Allowlist** — Origin-basierte Allowlist (kein Wildcard mit Credentials), `Vary: Origin`-Header
- **Audit-Protokollierung** — Jede KI-Aktion, jede Werkzeugausführung und jeder LLM-Aufruf wird als strukturiertes Audit-Ereignis aufgezeichnet
- **JWT-Persistenz** — Signierte JWT-Secrets in K8s-Secrets gespeichert mit optionalem Fallback
- **Ratenbegrenzung** — Token-Bucket-Ratenbegrenzer auf Login-Endpunkten zum Schutz vor Brute-Force-Angriffen
- **Sicherheits-Header** — X-Content-Type-Options, X-Frame-Options, HSTS, CSP

### Betrieb & Zuverlässigkeit
- **Graceful Shutdown** — SIGTERM/SIGINT-Behandlung mit SSE-Draining, SQLite-WAL-Flush und Controller-Stopp
- **Conversation-TTL** — Automatische Bereinigung inaktiver Chat-Sessions (30 Min Timeout, maximal 1000 Konversationen)
- **Circuit Breaker** — Robuste LLM-Aufrufe mit konfigurierbarem Retry, Backoff und Circuit Breaking
- **Prometheus-Metriken** — Cluster-Health-Gauges, Konversationszähler, Metriken zur Werkzeugausführung

### Bereitstellung
- **Kustomize** — Base + Overlay-Bereitstellung mit produktionsbereiten Standardeinstellungen
- **Eingebettete Web-UI** — Einzelne Binärdatei, keine externen Frontend-Abhängigkeiten
- **SQLite + K8s-CRDs** — Leichtgewichtige Persistenz, keine externe Datenbank erforderlich
- **PVC-Persistenz** — Daten überstehen Pod-Neustarts

---

## Architektur

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

Siehe [docs/ARCHITECTURE.md](../../docs/ARCHITECTURE.md) für detaillierte Komponentendokumentation.

---

## Schnellstart

### Voraussetzungen
- Kubernetes 1.24+ (k3s / k8s / EKS / GKE / AKS)
- kubectl konfiguriert
- Ein LLM-API-Schlüssel (OpenAI, DeepSeek, ZAI oder ein beliebiger OpenAI-kompatibler Provider)

### 1. Auf Kubernetes bereitstellen

**Option A: Deployment-Modus (empfohlen)**

```bash
# Ein Befehl — inklusive Namespace, RBAC, Secrets, Ingress, TLS
kubectl apply -k config/deploy/overlays/local

# Oder erstellen Sie Ihr eigenes Overlay
cp -r config/deploy/overlays/local config/deploy/overlays/myorg
# Bearbeiten Sie myorg/kustomization.yaml: Domain, Registry, CORS festlegen
kubectl apply -k config/deploy/overlays/myorg
```

**Option B: DaemonSet-Modus (Diagnose pro Knoten)**

```bash
kubectl apply -f config/daemonset-local.yaml
```

**Option C: install.sh (interaktiv)**

```bash
./install.sh install    # bereitstellen
./install.sh status     # Status prüfen
./install.sh uninstall  # entfernen
```

Siehe [docs/DEPLOYMENT.md](../../docs/DEPLOYMENT.md) für einen detaillierten Bereitstellungsleitfaden.

### 2. LLM-Provider konfigurieren

```bash
# Über das Dashboard: Tab Einstellungen → Provider-Typ, API-Schlüssel, Modell eintragen
# Oder über Umgebungsvariablen in der Overlay-ConfigMap:

configMapGenerator:
- name: k8ops-config
  literals:
  - PROVIDER_TYPE=openai
  - PROVIDER_MODEL=gpt-4o
  - PROVIDER_ENDPOINT=https://api.openai.com/v1

# API-Schlüssel über Secret:
secretGenerator:
- name: k8ops-credentials
  literals:
  - api-key=sk-your-key-here
```

### 3. Auf das Dashboard zugreifen

```bash
# Über Ingress (falls konfiguriert)
# Öffnen Sie https://<your-domain>  (z.B. https://k8ops.iot2.win)

# Oder Port-Forward
kubectl port-forward svc/k8ops-dashboard 9090:9090 -n k8ops-system
# Öffnen Sie http://localhost:9090
# Standard-Login: admin / admin (Passwortänderung wird verlangt)
```

### 4. Eine Diagnose auslösen

```bash
# Über kubectl (CRD)
kubectl apply -f examples/diagnostic.yaml

# Über die CLI
go run ./cmd/k8ops diagnose --problem "pods in production keep CrashLoopBackOff"

# Über die Chat-Oberfläche des Web-Dashboards
```

---

## Konfiguration

Die gesamte Konfiguration erfolgt über ConfigMap/Secret (verwaltet durch Kustomize-Overlays). Siehe [config/deploy/overlays/local/kustomization.yaml](../../config/deploy/overlays/local/kustomization.yaml) für ein funktionierendes Beispiel.

### Kern
| Variable | Standardwert | Beschreibung |
|----------|-------------|-------------|
| `PROVIDER_TYPE` | `openai` | LLM-Provider-Typ |
| `PROVIDER_MODEL` | `gpt-4o` | Modellname |
| `PROVIDER_ENDPOINT` | `https://api.openai.com/v1` | LLM-Provider-Basis-URL |
| `AIOPS_API_KEY` | (erforderlich) | LLM-API-Schlüssel (aus Secret) |

### Sicherheit
| Variable | Standardwert | Beschreibung |
|----------|-------------|-------------|
| `AUTH_JWT_SECRET` | (automatisch generiert) | JWT-Signatur-Secret (in K8s-Secret gespeichert) |
| `CORS_ALLOWED_ORIGINS` | (leer) | Komma-getrennte erlaubte Origins |
| `LDAP_SERVER` | (leer) | LDAP-Server-URL |
| `LDAP_SKIP_TLS_VERIFY` | `false` | LDAP-TLS-Zertifikatsverifizierung überspringen |
| `OIDC_ISSUER` | (leer) | OIDC-Issuer-URL |

### Benachrichtigungen
| Variable | Standardwert | Beschreibung |
|----------|-------------|-------------|
| `SLACK_WEBHOOK_URL` | (leer) | Slack-Incoming-Webhook für Benachrichtigungen |

### KI / Chat
| Variable | Standardwert | Beschreibung |
|----------|-------------|-------------|
| `MAX_STEPS` | `15` | Maximale Agent-Reasoning-Schritte pro Anfrage |
| `CONVERSATION_TTL` | `30m` | Inaktivitäts-Timeout für Konversationen |
| `MAX_CONVERSATIONS` | `1000` | Maximale gleichzeitige Konversationen |

---

## API

Das Dashboard stellt eine REST-API unter `http://<host>:9090/api/` bereit. Wichtige Endpunkte:

| Methode | Pfad | Beschreibung | Auth |
|---------|------|-------------|------|
| GET | `/api/health` | Health-Check | Öffentlich |
| GET | `/api/version` | Build-Version | Öffentlich |
| GET | `/api/cluster/overview` | Cluster-Übersicht | Viewer+ |
| GET | `/api/cluster/nodes` | Knoten-Liste + Health | Viewer+ |
| GET | `/api/cluster/pods` | Pod-Liste mit Status | Viewer+ |
| POST | `/api/chat/stream` | KI-Chat (SSE-Streaming) | Viewer+ |
| GET | `/api/resources/{type}` | K8s-Ressourcenabfrage | Viewer+ |
| POST | `/api/auth/login` | Local/LDAP-Login | Öffentlich |
| GET | `/api/auth/status` | Auth-Konfiguration + Provider | Öffentlich |
| GET | `/api/auth/providers` | Auth-Provider auflisten | Admin |
| GET/POST | `/api/rbac/users` | Benutzerverwaltung | Admin |
| GET/POST | `/api/rbac/roles` | Rollenverwaltung | Admin |

Siehe [docs/API.md](../../docs/API.md) für die vollständige API-Referenz.

---

## Entwicklung

### Voraussetzungen
- Go 1.22+
- kubectl (für Integrationstests)
- Zugriff auf einen Kubernetes-Cluster (für Controller-Tests)

### Bauen & Testen

```bash
# Manager-Binärdatei bauen
make build

# Alle Tests ausführen
make test

# Tests mit Race-Detector ausführen
go test -race -count=1 ./internal/...

# CRDs generieren
make manifests

# Docker-Image bauen
make docker-build IMG=ghcr.io/topcheer/k8ops:latest
```

### Projektstruktur

```
k8ops/
├── api/v1alpha1/           # CRD-Typdefinitionen
├── cmd/
│   ├── manager/            # Operator-Einstiegspunkt
│   └── k8ops/              # CLI-Werkzeug
├── config/
│   ├── crd/                # CRD-Manifeste
│   ├── deploy/             # Kustomize-Bereitstellung (Base + Overlays)
│   │   ├── base/           # Namespace, SA, RBAC, Deployment, Service, Ingress
│   │   └── overlays/
│   │       ├── local/      # Lokales Netzwerk-Overlay (Registry, Domain, CORS)
│   │       └── prod/       # Produktions-Overlay-Vorlage
│   └── daemonset/          # DaemonSet-Manifeste (Bereitstellung pro Knoten)
├── internal/
│   ├── agent/              # KI-Agent (Reasoning + Tool-Calling)
│   ├── audit/              # Strukturierte Audit-Protokollierung
│   ├── auth/               # Authentifizierung (Local/LDAP/OIDC) + RBAC
│   ├── chat/               # Chat-Engine mit Konversationsverwaltung
│   ├── collector/          # Cluster-Ereigniskollektor
│   ├── controller/         # CRD-Controller (Diagnose/Optimierung/Behebung)
│   ├── dashboard/          # Web-UI + REST-API
│   │   └── web/            # Eingebettetes Frontend (HTML/JS/CSS)
│   ├── memory/             # Konversations-Memory-Store
│   ├── metrics/            # Prometheus-Metriken
│   ├── provider/           # LLM-Provider-Schnittstelle
│   ├── providermanager/    # Multi-Provider-Verwaltung
│   ├── resilience/         # Circuit Breaker + Ratenbegrenzer
│   ├── safety/             # Safety-Checker (Dry-Run-Validierung)
│   └── tools/              # K8s- und Host-Werkzeuge (kubectl, exec, etc.)
├── docs/                   # Architektur-, API-, Sicherheits-, Bereitstellungsdokumentation
├── install.sh              # Ein-Klick-Installations-/Deinstallationsskript
├── .env.example            # Referenz der Umgebungsvariablen
└── examples/               # Beispiel-CRD-Manifeste
```

Siehe [CONTRIBUTING.md](../../CONTRIBUTING.md) für Entwicklungsrichtlinien.

---

## Lokale Entwicklung

Führen Sie k8ops direkt auf Ihrer Workstation ohne eine Kubernetes-Bereitstellung aus:

```bash
# Bauen
go build -o k8ops-manager ./cmd/manager/

# Ausführen
AIOPS_API_KEY=your-key ./k8ops-manager \
  --leader-elect=false \
  --dashboard-address=:9090 \
  --auth-db-path=/tmp/k8ops.db
```

Die Binärdatei erkennt automatisch Ihre kubeconfig (`~/.kube/config`), sodass alle K8s-Daten aus Ihrem verbundenen Cluster stammen. Siehe [docs/LOCAL_RUN.md](../../docs/LOCAL_RUN.md) für Details.

---

## Dokumentation

| Dokument | Beschreibung |
|----------|-------------|
| [docs/USER_GUIDE.md](../../docs/USER_GUIDE.md) | Umfassendes Benutzerhandbuch (alle 15 Funktionen) |
| [docs/ARCHITECTURE.md](../../docs/ARCHITECTURE.md) | Systemarchitektur und Komponenten-Design |
| [docs/DEPLOYMENT.md](../../docs/DEPLOYMENT.md) | Bereitstellungsleitfaden (Deployment / DaemonSet / Helm) |
| [docs/LOCAL_RUN.md](../../docs/LOCAL_RUN.md) | Lokale Ausführung der k8ops-Binärdatei (ohne K8s-Bereitstellung) |
| [docs/API.md](../../docs/API.md) | REST-API-Referenz |
| [docs/SECURITY.md](../../docs/SECURITY.md) | Sicherheitsrichtlinie und RBAC-Modell |
| [CHANGELOG.md](../../CHANGELOG.md) | Versionshistorie (v0.1.0 → v14.1) |

---

## Sicherheit

Siehe [SECURITY.md](../../SECURITY.md) für die vollständige Sicherheitsrichtlinie, einschließlich:
- Authentifizierungsmethoden und Konfiguration
- RBAC-Modell und Namespace-Scoping
- Umgang mit gemeldeten Schwachstellen

---

## Änderungsprotokoll

Siehe [CHANGELOG.md](../../CHANGELOG.md).

---

## Lizenz

GNU Affero General Public License v3.0 (AGPL-3.0). Siehe [LICENSE](../../LICENSE).
