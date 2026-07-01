# k8ops Installation Wizard Guide

The interactive installation wizard (`wizard.sh`) guides you through configuring
all major k8ops components before deployment: database backend, SSO integration,
and AI provider.

## Quick Start

### Interactive Mode

```bash
git clone https://github.com/topcheer/k8ops.git
cd k8ops
./wizard.sh
```

### Non-Interactive Mode

```bash
# Edit config/wizard-values.yaml with your settings, then:
./wizard.sh --values config/wizard-values.yaml
```

### Dry-Run (Generate Manifests Only)

```bash
./wizard.sh --dry-run
# Review generated files: .wizard-*.yaml
# Deploy manually with kubectl apply -f ...
```

## Wizard Steps

### Step 1: Deployment Mode

| Mode | Description | Best For |
|------|-------------|----------|
| **DaemonSet** | Runs on every node | K3s/bare-metal clusters, node-level monitoring |
| **Deployment** | Single replica with PVC | Managed K8s (EKS/GKE/AKS), cost-sensitive setups |

### Step 2: Database Backend

k8ops uses a database for user accounts, roles, and auth providers.

| Backend | Use Case | HA | Setup |
|---------|----------|----|-------|
| **SQLite** | Small clusters, single-node | No | Zero config (embedded) |
| **MySQL** | Multi-replica, shared auth | Yes | Internal StatefulSet or external connection |
| **PostgreSQL** | Multi-replica, shared auth | Yes | Internal StatefulSet or external connection |

#### Internal vs External Database

- **Internal**: The wizard deploys a MySQL/PostgreSQL StatefulSet in the `k8ops-system`
  namespace with a PVC. Fully managed — no external dependencies.
- **External**: Connect to your existing database. You provide the DSN connection string.

#### DSN Formats

**MySQL:**
```
k8ops:password@tcp(mysql-host:3306)/k8ops?charset=utf8mb4&parseTime=True
```

**PostgreSQL:**
```
host=postgres-host user=k8ops password=secret dbname=k8ops sslmode=disable
```

### Step 3: SSO / Identity Provider

k8ops supports multiple SSO providers with built-in presets:

| Provider | Type | Preset |
|----------|------|--------|
| **GitHub** | OIDC | Pre-configured issuer |
| **Google** | OIDC | Pre-configured issuer |
| **Microsoft** (Entra ID) | OIDC | Pre-configured issuer |
| **GitLab** | OIDC | Pre-configured issuer |
| **Keycloak** | OIDC | Custom issuer (your realm) |
| **Okta** | OIDC | Custom issuer |
| **Auth0** | OIDC | Custom issuer |
| **LDAP / AD** | LDAP | Server + bind DN |
| **Custom OIDC** | OIDC | Manual issuer URL |

#### OIDC Redirect URL

When registering your application with the identity provider, use this redirect URL:

```
https://<your-dashboard-host>/api/auth/oidc/<provider-name>/callback
```

Example for GitHub:
```
https://k8ops.example.com/api/auth/oidc/github/callback
```

#### LDAP Configuration

Provide:
- **Server URL**: `ldap://host:389` or `ldaps://host:636`
- **Search Base**: e.g. `ou=users,dc=example,dc=com`
- **Bind DN**: Service account DN, e.g. `cn=admin,dc=example,dc=com`
- **Bind Password**: Service account password

SSO can be skipped during installation and configured later via **Settings > Auth Providers** in the dashboard.

### Step 4: AI Provider

| Provider | Models | Notes |
|----------|--------|-------|
| **OpenAI** | gpt-4o, gpt-4o-mini | Default |
| **Anthropic** | claude-sonnet-4-20250514 | Claude family |
| **Gemini** | gemini-1.5-flash | Google AI |
| **Custom** | Any | OpenAI-compatible endpoint |

AI provider can be deferred to post-installation via **Settings** in the dashboard.

### Step 5: Confirm & Deploy

The wizard displays a summary of all choices. After confirmation, it:

1. Generates Kubernetes manifests (secrets, optional DB StatefulSet)
2. Applies them to the cluster
3. Deploys k8ops (DaemonSet or Deployment)
4. Waits for pods to be ready
5. Shows access URL and login credentials

## Post-Installation

### Default Login

- Username: `admin`
- Password: `admin`
- **Change immediately after first login**

### Configure SSO After Installation

If you skipped SSO during installation:

1. Navigate to **Settings > Auth Providers**
2. Click **Add Provider**
3. Select a preset (GitHub, Google, etc.)
4. Enter Client ID and Client Secret
5. Save and enable

### Environment Variables Reference

The wizard sets these environment variables (can also be set manually):

| Variable | Description | Default |
|----------|-------------|---------|
| `AUTH_DB_DRIVER` | Database driver | `sqlite` |
| `AUTH_DB_DSN` | Database connection string | (empty) |
| `AUTH_DB_PATH` | SQLite file path | `/data/k8ops.db` |
| `AUTH_JWT_SECRET` | JWT signing secret | (auto-generated) |
| `AUTH_DEFAULT_ROLE` | Default role for SSO users | `viewer` |
| `AIOPS_API_KEY` | AI provider API key | (empty) |

## CLI Flags

```bash
./manager \
  --auth-db-driver=postgres \
  --auth-db-dsn="host=localhost user=k8ops password=secret dbname=k8ops sslmode=disable" \
  --auth-jwt-secret=my-secret \
  --provider-type=openai \
  --provider-model=gpt-4o \
  --provider-api-key=sk-... \
  --dashboard-address=:9090
```

## Troubleshooting

### SQLite "out of memory" error

This occurs when the SQLite database path is not writable (e.g. read-only container filesystem).
Ensure `/data` is backed by an `emptyDir` or PVC volume.

### MySQL/PostgreSQL connection failed

1. Verify the DSN format matches your database type
2. Check network connectivity from k8ops pods to the database
3. Ensure the database user has CREATE/ALTER permissions (for auto-migration)

### SSO redirect not working

1. Verify the redirect URL matches exactly (including trailing slash)
2. Check that HTTPS is properly configured (OIDC requires HTTPS)
3. Ensure the identity provider has the correct redirect URL registered
