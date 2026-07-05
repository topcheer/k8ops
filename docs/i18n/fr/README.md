# k8ops — Kubernetes AI Operations Operator

<div align="center">

**Un opérateur AIOps Kubernetes qui diagnostique les problèmes, applique des corrections automatiques et optimise votre cluster grâce à l'IA.**

[![GitHub release](https://img.shields.io/github/v/release/topcheer/k8ops?style=flat-square)](https://github.com/topcheer/k8ops/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/topcheer/k8ops/ci.yml?branch=main&style=flat-square&label=CI)](https://github.com/topcheer/k8ops/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/topcheer/k8ops?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![Docker](https://img.shields.io/badge/Docker-ghcr.io-2496ED?style=flat-square&logo=docker)](https://github.com/topcheer/k8ops/pkgs/container/k8ops)
[![Built with ggcode](https://img.shields.io/badge/Built%20with-ggcode-6C43BC?style=flat-square)](https://github.com/topcheer/ggcode)

</div>

---

**Langues：** [English](../../README.md) | [中文](../zh-CN/README.md) | [日本語](../ja/README.md) | [한국어](../ko/README.md) | [Español](../es/README.md) | [Français](README.md) | [Deutsch](../de/README.md)

---

## Fonctionnalités

### Opérations assistées par IA
- **Diagnostics intelligents** — Soumettez une description de problème, obtenez une analyse de cause racine basée sur l'IA avec un raisonnement augmenté par des outils (kubectl describe, logs, événements, métriques)
- **Correction automatique** — L'IA propose et (avec approbation) exécute des actions de correction sûres : redémarrage de pods, mise à l'échelle de déploiements, nettoyage de ressources
- **Suggestions d'optimisation** — Analyse continue de l'utilisation des ressources, des écarts HPA/PDB et des opportunités de réduction des coûts
- **Chat en streaming** — Streaming SSE en temps réel avec blocs de réflexion, transparence des appels d'outils et rendu des résultats basé sur les différences

### Sécurité entreprise
- **Authentification multi-fournisseurs** — Locale (bcrypt), LDAP (avec vérification TLS configurable), OIDC (GitHub, Google, GitLab, Keycloak, Okta, Auth0, Microsoft)
- **RBAC** — Contrôle d'accès basé sur les rôles avec rôles admin/opérateur/observateur et permissions limitées à un namespace
- **Protection CSRF OIDC** — Cookies d'état par fournisseur avec validation `ConstantTimeCompare`
- **Liste blanche CORS** — Liste blanche basée sur l'origine (pas de wildcard avec identifiants), en-tête `Vary: Origin`
- **Journalisation d'audit** — Chaque action IA, exécution d'outil et appel LLM enregistré en tant qu'événements d'audit structurés
- **Persistance JWT** — Secrets JWT signés stockés dans des Secrets K8s avec repli optionnel
- **Limitation de débit** — Limiteur de débit à jeton (token-bucket) sur les points de terminaison de connexion pour prévenir les attaques par force brute
- **En-têtes de sécurité** — X-Content-Type-Options, X-Frame-Options, HSTS, CSP

### Opérations et fiabilité
- **Arrêt en douceur** — Gestion SIGTERM/SIGINT avec drainage SSE, vidage SQLite WAL et arrêt du contrôleur
- **TTL des conversations** — Nettoyage automatique des sessions de chat inactives (délai d'expiration de 30 min, 1000 conversations maximum)
- **Disjoncteur** — Appels LLM résilients avec reprise, temporisation et disjoncteur configurables
- **Métriques Prometheus** — Jauges de santé du cluster, compteurs de conversations, métriques d'exécution d'outils

### Déploiement
- **Kustomize** — Déploiement de base + superposition (overlay) avec des valeurs par défaut prêtes pour la production
- **Interface Web intégrée** — Binaire unique, aucune dépendance externe de frontend
- **SQLite + CRDs K8s** — Persistance légère, aucune base de données externe requise
- **Persistance PVC** — Les données survivent aux redémarrages de pods

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

Voir [docs/ARCHITECTURE.md](../../docs/ARCHITECTURE.md) pour la documentation détaillée des composants.

---

## Démarrage rapide

### Prérequis
- Kubernetes 1.24+ (k3s / k8s / EKS / GKE / AKS)
- kubectl configuré
- Une clé API LLM (OpenAI, DeepSeek, ZAI ou tout fournisseur compatible OpenAI)

### 1. Déployer sur Kubernetes

**Option A : Mode déploiement (recommandé)**

```bash
# Une seule commande — inclut namespace, RBAC, secrets, ingress, TLS
kubectl apply -k config/deploy/overlays/local

# Ou créez votre propre superposition (overlay)
cp -r config/deploy/overlays/local config/deploy/overlays/myorg
# Modifiez myorg/kustomization.yaml : définissez votre domaine, registre, CORS
kubectl apply -k config/deploy/overlays/myorg
```

**Option B : Mode DaemonSet (diagnostics par nœud)**

```bash
kubectl apply -f config/daemonset-local.yaml
```

**Option C : install.sh (interactif)**

```bash
./install.sh install    # déployer
./install.sh status     # vérifier le statut
./install.sh uninstall  # supprimer
```

Voir [docs/DEPLOYMENT.md](../../docs/DEPLOYMENT.md) pour le guide de déploiement détaillé.

### 2. Configurer le fournisseur LLM

```bash
# Via le tableau de bord : onglet Paramètres → remplir le type de fournisseur, la clé API, le modèle
# Ou via les variables d'environnement dans la ConfigMap de la superposition :

configMapGenerator:
- name: k8ops-config
  literals:
  - PROVIDER_TYPE=openai
  - PROVIDER_MODEL=gpt-4o
  - PROVIDER_ENDPOINT=https://api.openai.com/v1

# Clé API via Secret :
secretGenerator:
- name: k8ops-credentials
  literals:
  - api-key=sk-your-key-here
```

### 3. Accéder au tableau de bord

```bash
# Via Ingress (si configuré)
# Ouvrez https://<votre-domaine>  (ex. https://k8ops.iot2.win)

# Ou redirection de port (port-forward)
kubectl port-forward svc/k8ops-dashboard 9090:9090 -n k8ops-system
# Ouvrez http://localhost:9090
# Identifiants par défaut : admin / admin (changement de mot de passe sera demandé)
```

### 4. Déclencher un diagnostic

```bash
# Via kubectl (CRD)
kubectl apply -f examples/diagnostic.yaml

# Via CLI
go run ./cmd/k8ops diagnose --problem "pods in production keep CrashLoopBackOff"

# Via l'interface de chat du tableau de bord Web
```

---

## Configuration

Toute la configuration se fait via ConfigMap/Secret (gérée par les superpositions Kustomize). Voir [config/deploy/overlays/local/kustomization.yaml](../../config/deploy/overlays/local/kustomization.yaml) pour un exemple fonctionnel.

### Principal
| Variable | Valeur par défaut | Description |
|----------|-------------------|-------------|
| `PROVIDER_TYPE` | `openai` | Type de fournisseur LLM |
| `PROVIDER_MODEL` | `gpt-4o` | Nom du modèle |
| `PROVIDER_ENDPOINT` | `https://api.openai.com/v1` | URL de base du fournisseur LLM |
| `AIOPS_API_KEY` | (requis) | Clé API LLM (depuis le Secret) |

### Sécurité
| Variable | Valeur par défaut | Description |
|----------|-------------------|-------------|
| `AUTH_JWT_SECRET` | (auto-généré) | Secret de signature JWT (persisté dans un Secret K8s) |
| `CORS_ALLOWED_ORIGINS` | (vide) | Origines autorisées séparées par des virgules |
| `LDAP_SERVER` | (vide) | URL du serveur LDAP |
| `LDAP_SKIP_TLS_VERIFY` | `false` | Ignorer la vérification du certificat TLS LDAP |
| `OIDC_ISSUER` | (vide) | URL de l'émetteur OIDC |

### Notifications
| Variable | Valeur par défaut | Description |
|----------|-------------------|-------------|
| `SLACK_WEBHOOK_URL` | (vide) | Webhook entrant Slack pour les notifications |

### IA / Chat
| Variable | Valeur par défaut | Description |
|----------|-------------------|-------------|
| `MAX_STEPS` | `15` | Nombre maximum d'étapes de raisonnement de l'agent par requête |
| `CONVERSATION_TTL` | `30m` | Délai d'expiration des conversations inactives |
| `MAX_CONVERSATIONS` | `1000` | Nombre maximum de conversations simultanées |

---

## API

Le tableau de bord expose une API REST à l'adresse `http://<hôte>:9090/api/`. Points de terminaison principaux :

| Méthode | Chemin | Description | Auth |
|---------|--------|-------------|------|
| GET | `/api/health` | Vérification de l'état de santé | Public |
| GET | `/api/version` | Version du build | Public |
| GET | `/api/cluster/overview` | Résumé du cluster | Observateur+ |
| GET | `/api/cluster/nodes` | Liste des nœuds + santé | Observateur+ |
| GET | `/api/cluster/pods` | Liste des pods avec statut | Observateur+ |
| POST | `/api/chat/stream` | Chat IA (streaming SSE) | Observateur+ |
| GET | `/api/resources/{type}` | Requête de ressource K8s | Observateur+ |
| POST | `/api/auth/login` | Connexion locale/LDAP | Public |
| GET | `/api/auth/status` | Config d'auth + fournisseurs | Public |
| GET | `/api/auth/providers` | Lister les fournisseurs d'auth | Admin |
| GET/POST | `/api/rbac/users` | Gestion des utilisateurs | Admin |
| GET/POST | `/api/rbac/roles` | Gestion des rôles | Admin |

Voir [docs/API.md](../../docs/API.md) pour la référence complète de l'API.

---

## Développement

### Prérequis
- Go 1.22+
- kubectl (pour les tests d'intégration)
- Accès à un cluster Kubernetes (pour les tests de contrôleur)

### Construire et tester

```bash
# Construire le binaire manager
make build

# Exécuter tous les tests
make test

# Exécuter les tests avec détecteur de concurrence (race detector)
go test -race -count=1 ./internal/...

# Générer les CRDs
make manifests

# Construire l'image Docker
make docker-build IMG=ghcr.io/topcheer/k8ops:latest
```

### Structure du projet

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

Voir [CONTRIBUTING.md](../../CONTRIBUTING.md) pour les directives de développement.

---

## Développement local

Exécutez k8ops directement sur votre station de travail sans déploiement Kubernetes :

```bash
# Construire
go build -o k8ops-manager ./cmd/manager/

# Exécuter
AIOPS_API_KEY=your-key ./k8ops-manager \
  --leader-elect=false \
  --dashboard-address=:9090 \
  --auth-db-path=/tmp/k8ops.db
```

Le binaire découvre automatiquement votre kubeconfig (`~/.kube/config`), donc toutes les données K8s proviennent de votre cluster connecté. Voir [docs/LOCAL_RUN.md](../../docs/LOCAL_RUN.md) pour plus de détails.

---

## Documentation

| Document | Description |
|----------|-------------|
| [docs/USER_GUIDE.md](../../docs/USER_GUIDE.md) | Guide utilisateur complet (les 15 fonctionnalités) |
| [docs/ARCHITECTURE.md](../../docs/ARCHITECTURE.md) | Architecture du système et conception des composants |
| [docs/DEPLOYMENT.md](../../docs/DEPLOYMENT.md) | Guide de déploiement (Deployment / DaemonSet / Helm) |
| [docs/LOCAL_RUN.md](../../docs/LOCAL_RUN.md) | Exécution du binaire k8ops en local (sans déploiement K8s) |
| [docs/API.md](../../docs/API.md) | Référence de l'API REST |
| [docs/SECURITY.md](../../docs/SECURITY.md) | Politique de sécurité et modèle RBAC |
| [CHANGELOG.md](../../CHANGELOG.md) | Historique des versions (v0.1.0 → v14.1) |

---

## Sécurité

Voir [SECURITY.md](../../SECURITY.md) pour la politique de sécurité complète, incluant :
- Méthodes et configuration d'authentification
- Modèle RBAC et portée des namespaces
- Traitement des vulnérabilités signalées

---

## Journal des modifications

Voir [CHANGELOG.md](../../CHANGELOG.md).

---

## Licence

GNU Affero General Public License v3.0 (AGPL-3.0). Voir [LICENSE](../../LICENSE).
