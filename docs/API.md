# k8ops API Reference

All endpoints are served on the dashboard port (default `:9090`).

**Authentication:** JWT cookie (`k8ops_token`) or `Authorization: Bearer <token>` header.
**Content-Type:** `application/json` for all POST/PUT requests.

## OpenAPI 3.0 Spec

k8ops 自动生成 OpenAPI 3.0 规范，可用于自动生成 SDK、集成 API 网关或在 Swagger UI 中浏览。

| 端点 | 说明 |
|------|------|
| `GET /api/openapi.json` | 返回完整的 OpenAPI 3.0 JSON 规范 |
| `GET /api/docs` | 返回按标签分组的 API 文档元数据（包含 spec + tagGroups） |

**获取规范：**
```bash
curl -sk https://k8ops.iot2.win/api/openapi.json -o k8ops-openapi.json
```

**导入 Swagger Editor：**
1. 打开 https://editor.swagger.io
2. 文件 → 导入文件 → 选择 `k8ops-openapi.json`

**在 Dashboard 中浏览：** 侧边栏 → API Docs 页面提供交互式 API 浏览器，支持搜索、过滤、在线试调。

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

## Security Audit

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/security/audit` | Required | 集群安全扫描 — 检查 Pod Security Standards、RBAC、网络策略覆盖率、密钥安全 |
| GET | `/api/security/health` | Required | 平台安全健康检查 — 认证/TLS/K8s API 连接性 |

### GET /api/security/audit

扫描全集群，返回安全发现列表，按严重程度排序（critical > high > medium > low > info）。

**检查项：**
- **Pod Security:** 特权容器、root 运行、权限提升、危险 capabilities、hostPath/hostNetwork
- **RBAC:** cluster-admin 绑定、默认 SA 使用
- **Network:** 缺少 NetworkPolicy 的命名空间
- **Secrets:** Docker registry 密钥轮换建议
- **Resources:** 缺少 resource limits 的容器

**响应示例：**
```json
{
  "summary": {"critical": 0, "high": 2, "medium": 5, "low": 8, "info": 3, "total": 18},
  "findings": [
    {
      "severity": "high",
      "category": "Pod Security",
      "resource": "default/pod/nginx/container/app",
      "namespace": "default",
      "detail": "Container \"app\" allows privilege escalation",
      "fix": "Set allowPrivilegeEscalation: false in securityContext"
    }
  ],
  "scannedAt": "2025-01-15T10:30:00Z"
}
```

## Write Operations

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/scale` | Required | 扩缩容 deployment/statefulset |
| POST | `/api/pod/delete` | Required | 删除单个 Pod |
| POST | `/api/rollout/restart` | Required | 滚动重启 deployment/daemonset/statefulset |
| POST | `/api/node/cordon` | Required | 隔离/恢复节点 |
| POST | `/api/yaml/apply` | Required | 应用 YAML (kubectl apply) |

所有写操作均记录到审计日志。

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

## 新增端点 (v14.48-v14.53)

以下端点在 v14.48 至 v14.53 期间新增，已纳入 OpenAPI 3.0 规范。

### 容器镜像清单

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/images` | 集群中所有容器镜像清单，含资源限制审计和 `:latest` 标签检测 |

**响应摘要字段：** `totalImages`, `withoutLimits`, `withoutRequests`, `usingLatestTag`, `uniqueRegistries`

### 警告事件汇总

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/events/summary` | 按 Reason 聚合所有 Warning 事件，含严重级别分类和受影响命名空间统计 |

### 集群效率分析

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/efficiency` | 集群资源效率分析：无限制 Pod、过度配置容器、未充分利用节点，效率评分 0-100 |

### 安全：Secret 暴露扫描

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/secrets` | 检测硬编码凭据、Secret 轮换跟踪（90天）、未使用 Secret、敏感键名 |

### 审计搜索与导出

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/audit/events` | 审计事件搜索：支持 `actor`、`action`、`q`（全文搜索）、`severity`、日期范围过滤 |
| GET | `/api/audit/export` | 导出审计事件为 CSV 格式（可导入 SIEM 系统） |

### 备份管理

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/system/backup` | 列出所有备份文件（大小、年龄、类型） |
| POST | `/api/system/backup` | 创建数据库备份（时间戳命名） |
| DELETE | `/api/system/backup?name=X` | 删除指定备份（防路径遍历） |
| POST | `/api/system/backup/restore?name=X` | 从备份恢复数据库 |

### Alertmanager Webhook

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/webhooks/alertmanager` | 接收 Prometheus Alertmanager v4 告警，自动生成调查建议 |
| POST | `/api/webhooks/alertmanager/test` | 发送测试告警验证接收器 |

**Alertmanager 配置示例：**
```yaml
receivers:
  - name: k8ops
    webhook_configs:
      - url: http://k8ops.k8ops-system.svc:9090/api/webhooks/alertmanager
        send_resolved: true
```

### 变更日志

| 版本 | 端点 | 维度 |
|------|------|------|
| v14.49 | `GET /api/events/summary` | Product |
| v14.50 | 启动探针 + preStop | Deployment |
| v14.51 | `POST /api/webhooks/alertmanager` | Operations |
| v14.52 | `GET /api/efficiency` | Scalability |
| v14.53 | `GET /api/security/secrets` | Security |
