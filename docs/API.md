# k8ops API Reference

All endpoints are served on the dashboard port (default `:9090`).

**Authentication:** JWT cookie (`k8ops_token`) or `Authorization: Bearer <token>` header.
**Content-Type:** `application/json` for all POST/PUT requests.

---

## Health & System

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/health` | None | Liveness probe — returns `{"status":"ok"}` |
| GET | `/api/version` | None | Build version, git commit, Go version |

## Cluster

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/cluster/overview` | Required | Cluster summary: node count, pod count, CPU/memory usage, warnings (30s cache) |
| GET | `/api/nodes` | Required | List all nodes with resource usage and conditions (30s cache) |
| GET | `/api/nodes/{node}/pods` | Required | Pods running on a specific node |
| GET | `/api/pods` | Required | List all pods across namespaces (30s cache) |
| GET | `/api/pods/{namespace}/{name}/containers` | Required | Container list for a pod |
| GET | `/api/pods/{namespace}/{name}/logs?container=&follow=&tailLines=` | Required | Pod logs (supports SSE streaming with `follow=true`) |
| GET | `/api/events?namespace=&warning=` | Required | Kubernetes events, optionally filtered by namespace/warning |
| GET | `/api/resources?kind=&namespace=` | Required | Generic resource lister (Deployments, Services, etc.) (60s cache) |
| GET | `/api/crds?with_counts=true` | Required | Custom Resource Definitions (10min cache with counts) |
| GET | `/api/crd-resources?group=&version=&resource=&namespace=` | Required | CRD instances (60s cache) |
| GET | `/api/yaml?namespace=&name=&group=&version=&resource=&kind=` | Required | YAML view of any Kubernetes resource |

## Diagnostics & Remediation

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/diagnostics` | Required | List DiagnosticReport CRs, optional `?namespace=` filter |
| GET | `/api/diagnostics/{namespace}/{name}` | Required | Diagnostic detail with AI analysis |
| GET | `/api/remediations` | Required | List Remediation CRs, optional `?namespace=` filter |
| GET | `/api/optimizations` | Required | List Optimization CRs, optional `?namespace=` filter |

## AI Chat

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/chat` | Required | Send message to AI assistant (SSE streaming response) |
| GET | `/api/chat/conversations?id=` | Required | List conversations or get one by ID |

### POST /api/chat

**Request:**
```json
{
  "message": "Why is my pod crashing?",
  "conversation_id": "optional-existing-id",
  "stream": true
}
```

**Response:** SSE stream of AI analysis with tool calls and results.

### GET /api/chat/conversations

Returns conversation history. Pass `?id=<uuid>` for a single conversation.

## Provider Management

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/provider/status` | Required | Current AI provider config (masked API key) |
| POST | `/api/provider/update` | Required | Update provider type/model/endpoint at runtime |
| POST | `/api/provider/reload` | Required | Reload provider config from K8opsConfig CRD |
| GET | `/api/tools` | Required | List registered diagnostic tools |

## Auth

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/auth/login` | Public | Local login (rate-limited) |
| POST | `/api/auth/logout` | Required | Clear auth cookie |
| GET | `/api/auth/me` | Required | Current user info |
| POST | `/api/auth/change-password` | Required | Change own password |
| GET | `/api/auth/status` | Public | Auth configuration status (auth_enabled, user_count, ldap/oidc flags) |
| GET | `/api/auth/provider-presets` | Public | Available OIDC/LDAP provider templates |

### POST /api/auth/login

**Request:**
```json
{
  "username": "admin",
  "password": "admin"
}
```

**Response (200):**
```json
{
  "user": {"id": 1, "username": "admin", "role": "admin", "display_name": "Administrator"},
  "must_change": true,
  "redirect_url": "/"
}
```

Sets `k8ops_token` cookie (HttpOnly, SameSite=Lax, 24h).

**Error (401):**
```json
{"error": "invalid username or password"}
```

## OIDC

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/auth/oidc/{provider}/login` | Public | Redirect to OIDC provider (sets CSRF state cookie) |
| GET | `/api/auth/oidc/{provider}/callback` | Public | OIDC callback (validates state, creates user session) |

## Auth Provider Management (Admin)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/auth/providers` | Admin | List configured auth providers |
| POST | `/api/auth/providers` | Admin | Create auth provider (LDAP/OIDC) |
| GET | `/api/auth/providers/{id}` | Admin | Get provider by ID |
| PUT | `/api/auth/providers/{id}` | Admin | Update provider config |
| DELETE | `/api/auth/providers/{id}` | Admin | Delete provider |

## User Management (Admin)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/admin/users` | Admin | List all users |
| POST | `/api/admin/users` | Admin | Create user (default role: viewer, MustChangePwd=true) |
| GET | `/api/admin/users/{id}` | Admin | Get user by ID |
| PUT | `/api/admin/users/{id}` | Admin | Update user (role, namespaces, etc.) |
| DELETE | `/api/admin/users/{id}` | Admin | Delete user |
| POST | `/api/admin/users/{id}/reset-password` | Admin | Reset password (sets MustChangePwd=true) |
| GET | `/api/admin/auth-config` | Admin | Get auth configuration |
| PUT | `/api/admin/auth-config` | Admin | Update auth configuration |

## API Keys

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/auth/api-keys` | Required | List own API keys |
| POST | `/api/auth/api-keys` | Required | Create API key |
| DELETE | `/api/auth/api-keys/{id}` | Required | Revoke API key |

## RBAC Management (Admin)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/rbac/clusterroles` | Admin | List cluster roles |
| GET | `/api/rbac/clusterroles/{name}` | Admin | Get cluster role by name |
| DELETE | `/api/rbac/clusterroles/{name}` | Admin | Delete cluster role |
| GET | `/api/rbac/roles?namespace=` | Admin | List namespaced roles |
| GET | `/api/rbac/roles/{namespace}/{name}` | Admin | Get namespaced role |
| DELETE | `/api/rbac/roles/{namespace}/{name}` | Admin | Delete namespaced role |
| GET | `/api/rbac/rolebindings?namespace=` | Admin | List role bindings |
| GET | `/api/rbac/rolebindings/{namespace}/{name}` | Admin | Get role binding |
| DELETE | `/api/rbac/rolebindings/{namespace}/{name}` | Admin | Delete role binding |
| GET | `/api/rbac/api-resources` | Admin | List Kubernetes API resource types |
| GET | `/api/rbac/namespaces` | Admin | List all namespaces |
| GET | `/api/rbac/role-mapping?role=&kind=&name=&namespace=` | Admin | View role-to-subject mapping |
| GET | `/api/rbac/role-defs` | Admin | List k8ops custom role definitions |
| GET | `/api/rbac/subjects?kind=&namespace=` | Admin | List subjects (users/groups/service accounts) |

## Audit

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/audit?namespace=&limit=` | Required | Audit log entries (paginated) |
| GET | `/api/audit/stats` | Required | Audit statistics summary |

## Config

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/config` | Required | k8ops controller configuration (provider type/model, features) |

---

## Error Responses

All errors return JSON:

```json
{"error": "descriptive error message"}
```

| Code | Meaning |
|------|---------|
| 400 | Bad request (missing/invalid parameters) |
| 401 | Unauthorized (missing/expired/invalid token) |
| 403 | Forbidden (insufficient role) |
| 404 | Resource not found |
| 500 | Internal server error |
| 503 | Service unavailable (AI provider not configured) |

## Roles

| Role | Permissions |
|------|-------------|
| `admin` | Full access including user/RBAC/provider management |
| `operator` | Dashboard + diagnostics + chat (no user management) |
| `viewer` | Read-only dashboard + chat |
| `ns-admin` | Admin within assigned namespaces only |
| `ns-viewer` | Viewer within assigned namespaces only |
