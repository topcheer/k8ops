# k8ops Security

## Authentication

k8ops supports three authentication methods, configurable per deployment:

### Local Authentication

- Username/password stored in SQLite
- Passwords hashed with bcrypt
- Admin bootstrap via `AUTH_DEFAULT_ROLE` env var

### LDAP/Active Directory

- Configurable server URL, bind DN, search base
- `SkipTLSVerify` option (default: `false`) for self-signed certs
- Multi-provider support: multiple LDAP servers can be configured simultaneously

### OIDC (OpenID Connect)

- Supports any OIDC-compatible IdP (Google, GitHub, Keycloak, etc.)
- **CSRF protection**: state parameter validated with `crypto/subtle.ConstantTimeCompare`
- **Per-provider cookie**: `oidc_state_{provider}` prevents multi-provider collision
- **Secure flag**: auto-detected via TLS or `X-Forwarded-Proto` header
- **HttpOnly + SameSite**: state cookie not accessible via JavaScript

## RBAC Model

### Roles

| Role | Scope | Permissions |
|------|-------|------------|
| `admin` | cluster | Full access: manage users, providers, all namespaces |
| `operator` | cluster | Read all + chat + execute diagnostics |
| `viewer` | cluster | Read-only: view dashboards, reports |
| `ns-admin` | namespace | Admin within assigned namespaces |
| `ns-viewer` | namespace | Read-only within assigned namespaces |

### Namespace Scoping

Users with namespace-scoped roles are restricted to their assigned namespaces via:

1. **K8s RBAC sync**: `RoleBinding` resources created per namespace
2. **API impersonation**: Dashboard API calls use user identity when talking to K8s API
3. **Namespace filtering**: API responses filtered to allowed namespaces

### Built-in Role Protection

Built-in roles (`admin`, `operator`, `viewer`) are marked `Builtin: true` and cannot be deleted via the API.

## Security Features

### CORS Allowlist

- Configured via `CORS_ALLOWED_ORIGINS` env var (comma-separated)
- No wildcard (`*`) when credentials are involved
- Same-origin only if not configured

### OIDC CSRF Protection

- State parameter: random nonce per authentication attempt
- Validated with `subtle.ConstantTimeCompare` (timing-safe)
- Stored in HttpOnly cookie with Secure + SameSite flags

### JWT Persistence

- JWT signing secret persisted in K8s Secret `k8ops-auth` (key: `jwt-secret`)
- Falls back to ephemeral random secret with warning log if Secret absent
- Prevents session invalidation on pod restart

### Audit Logging

All sensitive operations are logged:

- User login/logout
- Provider configuration changes
- Diagnostic execution
- Remediation actions
- User role changes

### Rate Limiting

- `resilience.RateLimiter` available (not yet wired to HTTP layer — future work)

### Graceful Shutdown

- `SIGTERM`/`SIGINT` → drain SSE connections → flush SQLite WAL → stop manager
- Prevents data corruption on pod eviction

## Security Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_DB_DRIVER` | `sqlite` | Database driver |
| `AUTH_DB_DSN` | — | Database connection string |
| `AUTH_DB_PATH` | `/data/k8ops.db` | SQLite database path |
| `AUTH_JWT_SECRET` | (random) | JWT signing secret (persist via K8s Secret) |
| `AUTH_DEFAULT_ROLE` | `viewer` | Role for new users |
| `CORS_ALLOWED_ORIGINS` | — | Comma-separated allowed origins |
| `AIOPS_API_KEY` | — | LLM provider API key |

### K8s Secret Management

```yaml
# K8s Secret for JWT persistence
apiVersion: v1
kind: Secret
metadata:
  name: k8ops-auth
  namespace: k8ops-system
type: Opaque
stringData:
  jwt-secret: "<openssl rand -base64 32>"
```

The deployment reads this via:
```yaml
env:
- name: AUTH_JWT_SECRET
  valueFrom:
    secretKeyRef:
      name: k8ops-auth
      key: jwt-secret
      optional: true  # falls back to random if absent
```

### LDAP TLS Configuration

LDAP providers support `skip_tls_verify` (default: `false`):

```json
{
  "ldap": {
    "server": "ldaps://ldap.corp.com",
    "skip_tls_verify": false
  }
}
```

Only set `skip_tls_verify: true` for development with self-signed certificates.

## Known Limitations

1. **No rate limiting on login** — `resilience.RateLimiter` exists but is not wired to HTTP handlers
2. **No HTTPS termination** — k8ops serves HTTP; TLS must be handled by ingress controller
3. **SQLite single-node** — no HA database; suitable for single-replica deployments
4. **No session revocation** — JWT tokens valid until expiry (24h); no server-side revocation list

## Security Reporting

To report a security vulnerability:

1. **Do NOT open a public GitHub issue**
2. Email security@ggai.dev with details and reproduction steps
3. We will acknowledge within 48 hours and provide a fix timeline
4. Responsible disclosure is appreciated

## Future Security Improvements

- [ ] Wire rate limiting to login API
- [ ] Add session revocation (denylist)
- [ ] Support external OAuth providers for RBAC
- [ ] Add mTLS for service-to-service communication
- [ ] Implement secrets encryption at rest (beyond PVC)
