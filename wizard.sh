#!/bin/bash
# =============================================================================
# k8ops — Interactive Installation Wizard
# =============================================================================
# Guides users through configuring DB backend, SSO, and AI provider.
#
# Usage:
#   ./wizard.sh                    # Interactive mode
#   ./wizard.sh --values config.yaml  # Non-interactive from values file
#   ./wizard.sh --dry-run          # Generate configs without deploying
#   ./wizard.sh --help             # Show help
set -euo pipefail

VERSION="v1.0"
NAMESPACE="k8ops-system"
REGISTRY="registry.iot2.win/k8ops"
IMAGE_TAG="latest"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors
B='\033[1m'; CYAN='\033[36m'; GR='\033[32m'; YL='\033[33m'; RD='\033[31m'; BL='\033[34m'; RST='\033[0m'

# Defaults
DRY_RUN=false
VALUES_FILE=""
W_MODE="daemonset"
W_DB_DRIVER="sqlite"
W_DB_DSN=""
W_DB_INTERNAL=false
W_DB_INTERNAL_TYPE=""
W_DB_PVC_SIZE="10Gi"
W_SSO_PROVIDER=""
W_SSO_ISSUER=""
W_SSO_CLIENT_ID=""
W_SSO_CLIENT_SECRET=""
W_SSO_LDAP_SERVER=""
W_SSO_LDAP_BASE=""
W_SSO_LDAP_BIND_DN=""
W_SSO_LDAP_BIND_PW=""
W_AI_PROVIDER="openai"
W_AI_MODEL=""
W_AI_ENDPOINT=""
W_AI_API_KEY=""
W_DASHBOARD_HOST="k8ops.example.com"

# --- Helpers ---
title() {
  echo ""
  echo -e "${CYAN}${B}╔══════════════════════════════════════════════════════════╗${RST}"
  echo -e "${CYAN}${B}║  $*${RST}"
  echo -e "${CYAN}${B}╚══════════════════════════════════════════════════════════╝${RST}"
}

step() { echo ""; echo -e "${BL}${B}[$1]${RST} $2"; }
ask()  { echo -ne "${YL}▶ $1${RST} "; }
ok()   { echo -e "${GR}✓${RST} $*"; }
warn() { echo -e "${YL}⚠${RST} $*"; }
err()  { echo -e "${RD}✗${RST} $*" >&2; }

ask_choice() {
  local prompt="$1" default="$2"; shift 2
  local options=("$@")
  echo -e "\n${BL}${B}$prompt${RST}"
  for i in "${!options[@]}"; do
    if [[ "${options[$i]}" == "$default" ]]; then
      echo -e "  ${GR}${B}[$((i+1))]${RST} ${options[$i]} ${GR}(default)${RST}"
    else
      echo -e "  $((i+1)) ${options[$i]}"
    fi
  done
  ask "Choice (1-${#options[@]}):"
  read -r choice
  if [[ -z "$choice" ]]; then
    echo "$default"
  elif [[ "$choice" =~ ^[0-9]+$ ]] && (( choice >= 1 && choice <= ${#options[@]} )); then
    echo "${options[$((choice-1))]}"
  else
    echo "$default"
  fi
}

ask_input() {
  local prompt="$1" default="${2:-}"
  ask "$prompt"
  read -r val
  if [[ -z "$val" ]]; then
    echo "$default"
  else
    echo "$val"
  fi
}

ask_password() {
  local prompt="$1"
  ask "$prompt"
  read -rs val
  echo ""
  echo "$val"
}

confirm() {
  ask "$1 [Y/n]:"
  read -r ans
  [[ -z "$ans" || "$ans" =~ ^[Yy] ]]
}

# --- Prerun checks ---
check_prereqs() {
  local missing=()
  command -v kubectl &>/dev/null || missing+=("kubectl")
  if [[ ${#missing[@]} -gt 0 ]]; then
    err "Missing required tools: ${missing[*]}"
    exit 1
  fi

  # Check cluster connectivity
  if ! kubectl cluster-info &>/dev/null; then
    warn "Cannot reach Kubernetes cluster. Make sure kubeconfig is configured."
    if ! confirm "Continue anyway?"; then
      exit 1
    fi
  else
    ok "Kubernetes cluster reachable"
  fi
}

# --- Parse values file ---
load_values() {
  if [[ -z "$VALUES_FILE" || ! -f "$VALUES_FILE" ]]; then
    return
  fi
  echo -e "Loading configuration from ${BL}$VALUES_FILE${RST}..."
  # Simple YAML parsing (key: value per line, skip comments)
  while IFS=': ' read -r key val; do
    key="${key// /}"; val="${val//\"/}"
    [[ "$key" =~ ^# ]] && continue
    [[ -z "$key" || -z "$val" ]] && continue
    case "$key" in
      mode)             W_MODE="$val" ;;
      db.driver)        W_DB_DRIVER="$val" ;;
      db.dsn)           W_DB_DSN="$val" ;;
      db.internal)      W_DB_INTERNAL="$val" ;;
      db.pvcSize)       W_DB_PVC_SIZE="$val" ;;
      sso.provider)     W_SSO_PROVIDER="$val" ;;
      sso.issuer)       W_SSO_ISSUER="$val" ;;
      sso.clientId)     W_SSO_CLIENT_ID="$val" ;;
      sso.clientSecret) W_SSO_CLIENT_SECRET="$val" ;;
      sso.ldapServer)   W_SSO_LDAP_SERVER="$val" ;;
      sso.ldapBase)     W_SSO_LDAP_BASE="$val" ;;
      sso.ldapBindDN)   W_SSO_LDAP_BIND_DN="$val" ;;
      sso.ldapBindPW)   W_SSO_LDAP_BIND_PW="$val" ;;
      ai.provider)      W_AI_PROVIDER="$val" ;;
      ai.model)         W_AI_MODEL="$val" ;;
      ai.endpoint)      W_AI_ENDPOINT="$val" ;;
      ai.apiKey)        W_AI_API_KEY="$val" ;;
      dashboard.host)   W_DASHBOARD_HOST="$val" ;;
    esac
  done < "$VALUES_FILE"
  ok "Configuration loaded"
}

# --- Wizard Steps ---
wizard_welcome() {
  title "k8ops Installation Wizard $VERSION"
  echo ""
  echo "  This wizard will guide you through configuring:"
  echo "    1. Deployment mode (DaemonSet / Deployment)"
  echo "    2. Database backend (SQLite / MySQL / PostgreSQL)"
  echo "    3. SSO integration (GitHub / Google / Keycloak / LDAP / ...)"
  echo "    4. AI Provider (OpenAI / Anthropic / Gemini)"
  echo ""
  echo "  Press Ctrl+C at any time to abort."
}

wizard_mode() {
  step "1/5" "Deployment Mode"
  W_MODE=$(ask_choice \
    "Choose deployment mode:" \
    "daemonset" \
    "daemonset — Run on every node (recommended for K3s/K8s clusters)" \
    "deployment — Single replica with PVC (recommended for managed K8s)")

  if [[ "$W_MODE" == "deployment" ]]; then
    ok "Deployment mode selected (single replica with PVC)"
  else
    ok "DaemonSet mode selected (all nodes)"
  fi
}

wizard_db() {
  step "2/5" "Database Backend"
  W_DB_DRIVER=$(ask_choice \
    "Choose database backend for auth/users:" \
    "sqlite" \
    "sqlite — Embedded (zero config, recommended for small clusters)" \
    "mysql — MySQL/MariaDB (for HA or shared deployments)" \
    "postgres — PostgreSQL (for HA or shared deployments)")

  if [[ "$W_DB_DRIVER" == "sqlite" ]]; then
    ok "SQLite selected — no external DB needed"
    W_DB_INTERNAL=false
  else
    local db_type="$W_DB_DRIVER"
    W_DB_INTERNAL=$(ask_choice \
      "Deploy $db_type internally or use external?" \
      "internal" \
      "internal — Deploy $db_type StatefulSet in cluster (with PVC)" \
      "external — Connect to existing $db_type instance")

    if [[ "$W_DB_INTERNAL" == "internal" ]]; then
      W_DB_INTERNAL=true
      W_DB_INTERNAL_TYPE="$db_type"
      W_DB_PVC_SIZE=$(ask_input "PVC size for $db_type data:" "10Gi")
      local db_pass
      db_pass=$(ask_password "Set $db_type root password:" )
      local db_name="k8ops"
      if [[ "$db_type" == "mysql" ]]; then
        W_DB_DSN="k8ops:CHANGE_ME@tcp(k8ops-${db_type}-0.k8ops-${db_type}:3306)/${db_name}?charset=utf8mb4&parseTime=True"
      else
        W_DB_DSN="host=k8ops-${db_type}-0.k8ops-${db_type} user=k8ops password=CHANGE_ME dbname=${db_name} sslmode=disable"
      fi
      ok "Internal $db_type will be deployed (${W_DB_PVC_SIZE} PVC)"
    else
      W_DB_INTERNAL=false
      echo ""
      echo -e "${YL}Enter the DSN connection string for your $db_type instance:${RST}"
      if [[ "$db_type" == "mysql" ]]; then
        echo -e "  Format: ${BL}user:password@tcp(host:3306)/dbname?charset=utf8mb4&parseTime=True${RST}"
      else
        echo -e "  Format: ${BL}host=localhost user=postgres password=YOURPASS dbname=k8ops sslmode=disable${RST}"
      fi
      W_DB_DSN=$(ask_password "DSN connection string:")
      if [[ -z "$W_DB_DSN" ]]; then
        err "DSN is required for external database"
        exit 1
      fi
      ok "External $db_type configured"
    fi
  fi
}

wizard_sso() {
  step "3/5" "SSO / Identity Provider"
  if ! confirm "Configure SSO/Identity Provider integration?"; then
    ok "Skipping SSO — local auth only"
    return
  fi

  local providers=(
    "skip — No SSO (use local accounts only)"
    "github — GitHub OAuth"
    "google — Google Workspace / Gmail"
    "microsoft — Microsoft Entra ID (Azure AD)"
    "gitlab — GitLab.com / self-hosted"
    "keycloak — Keycloak IAM"
    "okta — Okta Workforce Identity"
    "auth0 — Auth0 by Okta"
    "ldap — LDAP / Active Directory"
    "custom — Custom OIDC provider"
  )

  local choice
  choice=$(ask_choice "Select identity provider:" "skip" "${providers[@]}")

  case "$choice" in
    skip|"") ok "SSO skipped"; return ;;
    github)      W_SSO_PROVIDER="github";      W_SSO_ISSUER="https://token.actions.githubusercontent.com" ;;
    google)      W_SSO_PROVIDER="google";      W_SSO_ISSUER="https://accounts.google.com" ;;
    microsoft)   W_SSO_PROVIDER="microsoft";   W_SSO_ISSUER="https://login.microsoftonline.com/common/v2.0" ;;
    gitlab)      W_SSO_PROVIDER="gitlab" ;;
    keycloak)    W_SSO_PROVIDER="keycloak" ;;
    okta)        W_SSO_PROVIDER="okta" ;;
    auth0)       W_SSO_PROVIDER="auth0" ;;
    ldap)        W_SSO_PROVIDER="ldap" ;;
    custom)      W_SSO_PROVIDER="custom-oidc" ;;
  esac

  local redirect_url="https://${W_DASHBOARD_HOST}/api/auth/oidc/${W_SSO_PROVIDER}/callback"

  if [[ "$W_SSO_PROVIDER" == "ldap" ]]; then
    echo -e "\n${YL}LDAP Configuration:${RST}"
    echo -e "  Configure your LDAP/AD server details below."
    W_SSO_LDAP_SERVER=$(ask_input "LDAP server URL (ldap://host:389 or ldaps://host:636):")
    W_SSO_LDAP_BASE=$(ask_input "Search base DN (e.g. ou=users,dc=example,dc=com):")
    W_SSO_LDAP_BIND_DN=$(ask_input "Bind DN (service account):")
    W_SSO_LDAP_BIND_PW=$(ask_password "Bind password:")
    ok "LDAP configured: $W_SSO_LDAP_SERVER"
  else
    if [[ -z "$W_SSO_ISSUER" || "$W_SSO_PROVIDER" == "custom-oidc" ]]; then
      echo ""
      W_SSO_ISSUER=$(ask_input "OIDC Issuer URL (e.g. https://auth.example.com/realms/myrealm):")
    fi
    echo ""
    echo -e "${YL}OIDC Configuration:${RST}"
    echo -e "  ${BL}Redirect URL: ${redirect_url}${RST}"
    echo -e "  Register this URL in your identity provider's settings."
    echo ""
    W_SSO_CLIENT_ID=$(ask_input "Client ID:")
    W_SSO_CLIENT_SECRET=$(ask_password "Client Secret:")
    ok "$W_SSO_PROVIDER OIDC configured"
    echo -e "  ${YL}Remember to register redirect URL:${RST}"
    echo -e "  ${BL}  ${redirect_url}${RST}"
  fi
}

wizard_ai() {
  step "4/5" "AI Provider"
  if ! confirm "Configure AI provider now? (can be done later via Settings)"; then
    ok "AI provider can be configured later from dashboard Settings"
    return
  fi

  W_AI_PROVIDER=$(ask_choice \
    "Select AI provider:" \
    "openai" \
    "openai — OpenAI GPT-4/GPT-3.5" \
    "anthropic — Claude 3/3.5" \
    "gemini — Google Gemini" \
    "custom — Custom OpenAI-compatible endpoint")

  case "$W_AI_PROVIDER" in
    openai)    W_AI_MODEL=$(ask_input "Model:" "gpt-4o") ;;
    anthropic) W_AI_MODEL=$(ask_input "Model:" "claude-sonnet-4-20250514") ;;
    gemini)    W_AI_MODEL=$(ask_input "Model:" "gemini-1.5-flash") ;;
    custom)
      W_AI_ENDPOINT=$(ask_input "API endpoint:")
      W_AI_MODEL=$(ask_input "Model name:")
      ;;
  esac
  W_AI_API_KEY=$(ask_password "API Key:")
  ok "AI provider: $W_AI_PROVIDER / $W_AI_MODEL"
}

wizard_summary() {
  step "5/5" "Configuration Summary"
  echo ""
  echo -e "  ${B}Mode:${RST}        $W_MODE"
  echo -e "  ${B}Database:${RST}    $W_DB_DRIVER"
  if [[ "$W_DB_DRIVER" != "sqlite" ]]; then
    if [[ "$W_DB_INTERNAL" == true ]]; then
      echo -e "  ${B}DB Deploy:${RST}   Internal StatefulSet (${W_DB_PVC_SIZE} PVC)"
    else
      echo -e "  ${B}DB DSN:${RST}      [hidden]"
    fi
  fi
  if [[ -n "$W_SSO_PROVIDER" && "$W_SSO_PROVIDER" != "skip" ]]; then
    if [[ "$W_SSO_PROVIDER" == "ldap" ]]; then
      echo -e "  ${B}SSO:${RST}        LDAP ($W_SSO_LDAP_SERVER)"
    else
      echo -e "  ${B}SSO:${RST}        $W_SSO_PROVIDER (OIDC)"
    fi
  else
    echo -e "  ${B}SSO:${RST}        Local auth only"
  fi
  if [[ -n "$W_AI_API_KEY" ]]; then
    echo -e "  ${B}AI:${RST}         $W_AI_PROVIDER / $W_AI_MODEL"
  else
    echo -e "  ${B}AI:${RST}         Not configured (deferred)"
  fi
  echo -e "  ${B}Host:${RST}       $W_DASHBOARD_HOST"
  echo ""

  if ! confirm "Proceed with deployment?"; then
    warn "Aborted by user"
    exit 0
  fi
}

# --- Generate manifests ---
generate_values() {
  local out="$SCRIPT_DIR/.wizard-values-generated.yaml"
  cat > "$out" << EOF
# k8ops generated configuration — $(date)
mode: $W_MODE
dashboard:
  host: $W_DASHBOARD_HOST

db:
  driver: $W_DB_DRIVER
$([[ -n "$W_DB_DSN" ]] && echo "  dsn: \"$W_DB_DSN\"" || true)
$([[ "$W_DB_INTERNAL" == true ]] && echo "  internal: true" || echo "  internal: false")
$([[ "$W_DB_INTERNAL" == true ]] && echo "  pvcSize: $W_DB_PVC_SIZE" || true)

sso:
$([[ -n "$W_SSO_PROVIDER" && "$W_SSO_PROVIDER" != "skip" ]] && echo "  provider: $W_SSO_PROVIDER" || echo "  provider: none")
$([[ -n "$W_SSO_ISSUER" ]] && echo "  issuer: $W_SSO_ISSUER" || true)
$([[ -n "$W_SSO_CLIENT_ID" ]] && echo "  clientId: $W_SSO_CLIENT_ID" || true)
$([[ -n "$W_SSO_LDAP_SERVER" ]] && echo "  ldapServer: $W_SSO_LDAP_SERVER" || true)
$([[ -n "$W_SSO_LDAP_BASE" ]] && echo "  ldapBase: $W_SSO_LDAP_BASE" || true)
$([[ -n "$W_SSO_LDAP_BIND_DN" ]] && echo "  ldapBindDN: $W_SSO_LDAP_BIND_DN" || true)

ai:
  provider: $W_AI_PROVIDER
$([[ -n "$W_AI_MODEL" ]] && echo "  model: $W_AI_MODEL" || true)
$([[ -n "$W_AI_ENDPOINT" ]] && echo "  endpoint: $W_AI_ENDPOINT" || true)
$([[ -n "$W_AI_API_KEY" ]] && echo "  apiKey: [hidden]" || true)
EOF
  echo "$out"
}

generate_db_manifests() {
  if [[ "$W_DB_INTERNAL" != true ]]; then
    return
  fi

  local out="$SCRIPT_DIR/.wizard-db.yaml"
  local db_type="$W_DB_INTERNAL_TYPE"
  local image="" port=""

  if [[ "$db_type" == "mysql" ]]; then
    image="mysql:8.0"
    port="3306"
  else
    image="postgres:16-alpine"
    port="5432"
  fi

  cat > "$out" << EOF
---
# Auto-generated by wizard.sh — Internal $db_type for k8ops auth
apiVersion: v1
kind: Service
metadata:
  name: k8ops-${db_type}
  namespace: ${NAMESPACE}
  labels:
    app.kubernetes.io/name: k8ops-${db_type}
spec:
  ports:
  - port: ${port}
    targetPort: ${port}
  selector:
    app.kubernetes.io/name: k8ops-${db_type}
  clusterIP: None
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: k8ops-${db_type}
  namespace: ${NAMESPACE}
  labels:
    app.kubernetes.io/name: k8ops-${db_type}
spec:
  serviceName: k8ops-${db_type}
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: k8ops-${db_type}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: k8ops-${db_type}
    spec:
      containers:
      - name: ${db_type}
        image: ${image}
        ports:
        - containerPort: ${port}
        env:
        - name: MYSQL_ROOT_PASSWORD
          value: "CHANGE_ME_ROOT"
        - name: MYSQL_DATABASE
          value: "k8ops"
        - name: MYSQL_USER
          value: "k8ops"
        - name: MYSQL_PASSWORD
          value: "CHANGE_ME"
        # For PostgreSQL, use:
        # - name: POSTGRES_DB
        #   value: "k8ops"
        # - name: POSTGRES_USER
        #   value: "k8ops"
        # - name: POSTGRES_PASSWORD
        #   value: "k8ops"
        volumeMounts:
        - name: data
          mountPath: /var/lib/${db_type}
        readinessProbe:
          tcpSocket:
            port: ${port}
          initialDelaySeconds: 10
          periodSeconds: 5
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: ${W_DB_PVC_SIZE}
EOF
  echo "$out"
}

generate_secrets() {
  local out="$SCRIPT_DIR/.wizard-secrets.yaml"
  cat > "$out" << 'HEADER'
---
# Auto-generated by wizard.sh — k8ops secrets
apiVersion: v1
kind: Secret
metadata:
  name: k8ops-credentials
  namespace: k8ops-system
type: Opaque
stringData:
HEADER

  if [[ -n "$W_AI_API_KEY" ]]; then
    echo "  api-key: \"$W_AI_API_KEY\"" >> "$out"
  fi

  if [[ "$W_DB_DRIVER" != "sqlite" && -n "$W_DB_DSN" ]]; then
    cat >> "$out" << EOF
---
apiVersion: v1
kind: Secret
metadata:
  name: k8ops-db
  namespace: k8ops-system
type: Opaque
stringData:
  driver: "$W_DB_DRIVER"
  dsn: "$W_DB_DSN"
EOF
  fi

  if [[ -n "$W_SSO_CLIENT_SECRET" ]]; then
    cat >> "$out" << EOF
---
apiVersion: v1
kind: Secret
metadata:
  name: k8ops-sso
  namespace: k8ops-system
type: Opaque
stringData:
  provider: "$W_SSO_PROVIDER"
  issuer: "$W_SSO_ISSUER"
  client-id: "$W_SSO_CLIENT_ID"
  client-secret: "$W_SSO_CLIENT_SECRET"
EOF
  fi

  if [[ -n "$W_SSO_LDAP_BIND_PW" ]]; then
    cat >> "$out" << EOF
---
apiVersion: v1
kind: Secret
metadata:
  name: k8ops-ldap
  namespace: k8ops-system
type: Opaque
stringData:
  server: "$W_SSO_LDAP_SERVER"
  base: "$W_SSO_LDAP_BASE"
  bind-dn: "$W_SSO_LDAP_BIND_DN"
  bind-pw: "$W_SSO_LDAP_BIND_PW"
EOF
  fi

  echo "$out"
}

deploy() {
  step "▶" "Deploying k8ops..."

  local values_file
  values_file=$(generate_values)
  ok "Generated: $values_file"

  local secrets_file
  secrets_file=$(generate_secrets)
  ok "Generated: $secrets_file"

  local db_file=""
  if [[ "$W_DB_INTERNAL" == true ]]; then
    db_file=$(generate_db_manifests)
    ok "Generated: $db_file"
  fi

  if [[ "$DRY_RUN" == true ]]; then
    echo ""
    warn "Dry-run mode — manifest files generated, not deployed."
    echo -e "  Values:  ${BL}$values_file${RST}"
    echo -e "  Secrets: ${BL}$secrets_file${RST}"
    [[ -n "$db_file" ]] && echo -e "  DB:      ${BL}$db_file${RST}"
    echo ""
    echo "Review and deploy manually with:"
    echo -e "  ${BL}kubectl apply -f $secrets_file${RST}"
    [[ -n "$db_file" ]] && echo -e "  ${BL}kubectl apply -f $db_file${RST}"
    return
  fi

  # Apply secrets
  kubectl apply -f "$secrets_file" 2>/dev/null && ok "Secrets applied" || warn "Secrets apply failed (may already exist)"

  # Apply DB if internal
  if [[ -n "$db_file" ]]; then
    echo ""
    step "▶" "Deploying internal $W_DB_INTERNAL_TYPE..."
    kubectl apply -f "$db_file" 2>/dev/null
    echo -n "Waiting for DB to be ready..."
    kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name="k8ops-$W_DB_INTERNAL_TYPE" -n "$NAMESPACE" --timeout=120s 2>/dev/null && ok "DB ready" || warn "DB not ready yet (may need more time)"
  fi

  # Deploy k8ops
  step "▶" "Deploying k8ops ($W_MODE mode)..."
  if [[ "$W_MODE" == "deployment" ]]; then
    kubectl apply -k "$SCRIPT_DIR/config/deploy/overlays/local" 2>/dev/null || {
      err "Deployment overlay failed"
      exit 1
    }
  else
    kubectl apply -k "$SCRIPT_DIR/config/daemonset" 2>/dev/null || {
      err "DaemonSet kustomize failed"
      exit 1
    }
  fi

  echo ""
  step "▶" "Waiting for pods..."
  sleep 5
  kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=k8ops -n "$NAMESPACE" --timeout=180s 2>/dev/null && ok "k8ops pods ready" || {
    warn "Pods still starting..."
    kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/name=k8ops
  }

  echo ""
  ok "k8ops deployed successfully!"
  echo ""
  echo -e "  ${B}Access:${RST}  https://${W_DASHBOARD_HOST}"
  echo -e "  ${B}Login:${RST}  admin / admin (change immediately)"
  [[ -n "$W_SSO_PROVIDER" && "$W_SSO_PROVIDER" != "skip" ]] && echo -e "  ${B}SSO:${RST}   $W_SSO_PROVIDER configured — register via Settings > Auth Providers"
  echo ""
}

# --- Main ---
main() {
  # Parse args
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --values)  VALUES_FILE="$2"; shift 2 ;;
      --dry-run) DRY_RUN=true; shift ;;
      --help|-h)
        echo "Usage: wizard.sh [--values FILE] [--dry-run] [--help]"
        echo ""
        echo "Options:"
        echo "  --values FILE   Non-interactive mode, read config from YAML file"
        echo "  --dry-run       Generate manifests without deploying"
        echo "  --help          Show this help"
        exit 0 ;;
      *) err "Unknown option: $1"; exit 1 ;;
    esac
  done

  check_prereqs

  if [[ -n "$VALUES_FILE" ]]; then
    load_values
  else
    wizard_welcome
    W_DASHBOARD_HOST=$(ask_input "Dashboard hostname (e.g. k8ops.example.com):")
    wizard_mode
    wizard_db
    wizard_sso
    wizard_ai
    wizard_summary
  fi

  deploy
}

main "$@"
