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
| v14.54 | OpenAPI 3.0 spec + API.md | Documentation |
| v14.55 | `GET /api/pdbs` `GET /api/compatibility` | Product |
| v14.56 | `GET /api/certificates/expiry` | Operations |
| v14.57 | 优雅关闭 draining gate | Deployment |
| v14.58 | `GET /api/addons/health` | Product |
| v14.59 | `GET /api/capacity/forecast` | Scalability |
| v14.60 | OpenAPI spec 补全 + API.md 更新 | Documentation |
| v14.61 | `GET /api/security/network-policies` | Security |
| v14.62 | `GET /api/diagnostics/restarts` | Operations |
| v14.63 | `GET /api/deployments/rollout` | Deployment |
| v14.64 | `GET /api/resources/waste` | Product |
| v14.65 | `GET /api/scaling/bottlenecks` | Scalability |

### Pod Disruption Budget 状态 (v14.55+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/pdbs` | 列出所有 PDB，含 disruption 状态、匹配工作负载、健康评估（healthy/at-risk/blocked），用于 drain 前安全检查 |

### K8s 发行版兼容性检测 (v14.55+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/compatibility` | 自动检测集群发行版（vanilla/k3s/RKE2/EKS/GKE/AKS/OpenShift/Talos）、版本兼容性、ARM/Windows/GPU 节点特性 |

### TLS 证书过期扫描 (v14.56+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/certificates/expiry` | 扫描所有 TLS/Opaque Secret 中的 X.509 证书，按过期时间分类（expired/critical/warning/ok），关联 Ingress 资源 |

### 服务器 Drain 状态 (v14.57+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/system/drain-status` | 报告服务器优雅关闭状态：draining、shutdownInitiated、activeConnections、uptime |

### 插件健康检测 (v14.58+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/addons/health` | 非侵入式检测 39 种常见 K8s 插件（12 类别：CNI/DNS/Ingress/CertManager/LB/Mesh/Backup/Monitoring/Policy/Storage/GitOps/VM）的健康状态 |

### 容量耗尽预测 (v14.59+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/capacity/forecast` | 预测 CPU/内存/Pod/存储 容量何时耗尽，基于增长率估算提供 days-to-exhaustion 和中文扩容建议 |

### Network Policy 审计扫描 (v14.61+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/network-policies` | 审计 NetworkPolicy 覆盖率：检测无 NetworkPolicy 的命名空间、宽松策略（0.0.0.0/0 入/出站）、部分覆盖，按严重级别分类（critical/warning/info） |

**查询参数：** `namespace`（可选，过滤命名空间）

**返回示例：**
```json
{
  "summary": {
    "totalNamespaces": 27,
    "withoutNetPol": 25,
    "findings": 18,
    "critical": 10,
    "warning": 8
  },
  "namespaces": [...]
}
```

### Pod 重启诊断 (v14.62+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/diagnostics/restarts` | 诊断 Pod 重启模式和根因：分类重启行为（crash-loop/occasional/post-deploy），提取终止原因（OOMKilled/Error/退出码）、等待状态（CrashLoopBackOff/ImagePullBackOff） |

**查询参数：** `namespace`（可选）

**诊断模式：**
- **crash-loop**: 短时间内大量重启
- **occasional**: 长时间少量重启
- **post-deploy**: 部署后立即重启

### 部署 Rollout 状态追踪 (v14.63+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/deployments/rollout` | 扫描所有 Deployment/StatefulSet/DaemonSet 的 rollout 健康状态：7 种状态（complete/in-progress/stalled/degraded/paused/failed/scaled-to-zero），检测 ProgressDeadlineExceeded、ReplicaFailure、generation mismatch |

**查询参数：**
- `namespace`（可选）— 过滤命名空间
- `status`（可选）— 过滤 rollout 状态：`failed`、`degraded`、`stalled`、`in-progress`、`paused`、`scaled-to-zero`、`complete`

**状态说明：**
| 状态 | 含义 |
|------|------|
| `complete` | 所有副本已更新且就绪 |
| `in-progress` | 正在进行滚动更新 |
| `stalled` | 控制器未观察到最新 spec（generation 不匹配） |
| `degraded` | 部分副本不可用 |
| `paused` | Deployment 被显式暂停 |
| `failed` | ProgressDeadlineExceeded，部署超时失败 |
| `scaled-to-zero` | 副本数为 0 |

### 资源浪费检测 (v14.64+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/resources/waste` | 扫描集群中的浪费和孤立资源以降低成本：6 类浪费（dead-service/unused-pvc/orphaned-configmap/orphaned-secret/empty-namespace/unattached-pv），4 级严重度（critical/high/medium/low），成本风险评估 |

**查询参数：** `namespace`（可选）

**浪费类型：**
| 类别 | 检测内容 | 默认严重度 |
|------|---------|-----------|
| `dead-service` | 无后端 endpoint 的 Service（LoadBalancer 为 critical） | medium/critical |
| `unused-pvc` | 未被任何 Pod 挂载的 PVC | high |
| `orphaned-configmap` | 未被任何 Pod 引用的 ConfigMap | low/medium |
| `orphaned-secret` | 未被任何 Pod 引用的 Secret（安全风险） | high |
| `empty-namespace` | 无运行 Pod 的命名空间 | medium |
| `unattached-pv` | Available 状态的 PV（未绑定任何 PVC） | critical |

**智能过滤：** 自动跳过 kube-system 命名空间、ServiceAccount token Secret、Helm release Secret、自动生成的 ConfigMap

### 扩展瓶颈检测 (v14.65+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/scaling/bottlenecks` | 扫描限制水平扩展的因素：7 类瓶颈（node-schedulable/node-pressure/resource-quota/hpa-stuck/pdb-blocking/storage-exhaust/image-pull-limit），4 级影响（critical/high/moderate/low），集群容量摘要 |

**查询参数：** `namespace`（可选）

**瓶颈类型：**
| 类别 | 检测内容 |
|------|---------|
| `node-schedulable` | 被隔离的节点、集群 Pod 容量超限（>75% 警告 / >90% 严重） |
| `node-pressure` | 内存、磁盘、PID 压力状态 |
| `resource-quota` | 命名空间配额超 75%/90% |
| `hpa-stuck` | HPA 达到最大副本数或缺失指标 |
| `pdb-blocking` | PDB 允许 0 次自愿中断 |
| `storage-exhaust` | 命名空间 PVC 请求超 500Gi |

**集群容量摘要：** 提供节点数、CPU/内存容量与可分配量、Pod 容量与已分配量、扩展余量

### RBAC 权限风险分析 (v14.67+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/rbac-risk` | 分析所有 RoleBinding/ClusterRoleBinding 的权限风险，0-100 评分系统，5 级风险等级（critical/high/elevated/moderate/low），检测 cluster-admin 绑定、权限提升、通配符权限、敏感资源访问 |

**查询参数：** `namespace`（可选）

**风险评分规则：**
| 检测项 | 基础分 | 附加分 |
|--------|--------|--------|
| ClusterRoleBinding + cluster-admin | 100 | - |
| 权限提升（escalate/bind/impersonate） | - | +25 |
| 通配符动词（verbs: *） | - | +25 |
| 通配符资源（resources: *） | - | +20 |
| 集群范围写操作 | - | +30 |
| 敏感资源访问（secrets/pods/exec） | - | +15 |

### CronJob 执行健康监控 (v14.68+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/cronjobs/health` | 监控所有 CronJob 的执行健康：成功率、连续失败、暂停/停滞调度、从未执行，5 级健康状态（healthy/warning/failing/suspended/no-runs） |

**查询参数：** `namespace`（可选）

**健康状态：**
| 状态 | 触发条件 |
|------|---------|
| `failing` | 连续 3 次以上失败 |
| `warning` | 1-2 次连续失败，或成功率 < 50% |
| `suspended` | CronJob 被 suspend |
| `no-runs` | 从未执行过 |
| `healthy` | 近期全部成功 |

### Service & Endpoint 健康监控 (v14.69+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/networking/health` | 扫描所有 Service 和 Ingress 的网络健康：无端点服务、选择器不匹配、端点降级、LoadBalancer 等待、Ingress 后端服务缺失/无端点，5 级健康状态 |

**查询参数：** `namespace`（可选）

**Service 健康状态：**
| 状态 | 含义 |
|------|------|
| `misconfigured` | 选择器不匹配 — 无 Pod 匹配 label |
| `no-endpoints` | 所有端点不可用 |
| `degraded` | 部分端点不可用 |
| `external` | ExternalName/LoadBalancer（信息性） |
| `healthy` | 所有端点正常 |

**Ingress 健康检查：** 检测后端 Service 是否存在、是否有可用端点，验证默认后端和规则路径

### PV/PVC 存储健康监控 (v14.70+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/storage/health` | 扫描所有 PVC/PV 的存储健康：Pending PVC 诊断、孤立 PVC（绑定但无 Pod 使用 > 1 天）、Lost/Failed PVC、Released/Failed PV 需手动清理、陈旧 Available PV 浪费容量，6 级健康状态 + 存储类分布分析 |

**查询参数：** `namespace`（可选）

**PVC 健康状态：**
| 状态 | 含义 |
|------|------|
| `failed` | PVC 配置失败 |
| `lost` | 底层 PV 已删除 |
| `pending` | 等待供给（无存储类、WaitForFirstConsumer） |
| `near-capacity` | 接近容量上限 |
| `orphaned` | 已绑定但超过 1 天无 Pod 使用 |
| `bound` | 正常绑定 |

**PV 问题检测：** Released PV（需手动清理）、Failed PV（回收失败）、陈旧 Available PV（>7 天浪费容量）

**存储类分析：** 默认类标记、provisioner、reclaim policy、binding mode、volume expansion 支持、PVC 数量分布

### ServiceAccount 安全审计 (v14.72+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/service-accounts` | 全面审计所有 ServiceAccount 的安全状况：未使用 SA、默认 SA 被 Pod 使用、不必要的 token 自动挂载、cluster-admin 绑定、集群范围权限、陈旧 SA、遗留长效 token secret |

**查询参数：** `namespace`（可选）

**风险评分：** 0-100（越高越危险），5 级严重度：critical / high / elevated / moderate / low

**检测项：**
| 检测项 | 严重度 | 说明 |
|--------|--------|------|
| 未使用 SA（>7 天无 Pod 引用） | moderate | 攻击面扩大 |
| 默认 SA 被 Pod 使用 | elevated | 违反最小权限原则 |
| cluster-admin 绑定 | critical | 集群级超级权限 |
| 不必要的 token 自动挂载 | moderate | 无需 token 的 SA 不应挂载 |
| 陈旧 SA（>30 天无使用但仍有权限） | high | 僵尸权限 |
| 遗留长效 token secret（K8s <1.24） | high | 不推荐的长效 token |

### SLO/SLA 错误预算追踪 (v14.73+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/slo` | 基于多窗口多燃烧率算法的 SLO/SLA 可用性和错误预算追踪 |

**查询参数：** `namespace`（可选）

**窗口配置：** 5m / 1h / 6h / 24h / 7d

**返回内容：**
| 字段 | 说明 |
|------|------|
| `availability` | 各窗口可用性百分比 |
| `errorBudget` | 错误预算剩余量和消耗率 |
| `burnRate` | 多窗口燃烧率（fast: 5m/1h, slow: 6h/24h） |
| `latencySLO` | P50/P95/P99 延迟百分位及目标 |
| `status` | meeting（达标）/ at-risk（风险）/ violated（违反）|

### ResourceQuota 与 LimitRange 监控 (v14.74+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/resources/quota` | 扫描所有命名空间的 ResourceQuota 利用率和 LimitRange 默认约束 |

**查询参数：** `namespace`（可选）

**配额状态级别：**
| 状态 | 使用率 | 说明 |
|------|--------|------|
| `ok` | <70% | 正常 |
| `warning` | 70-85% | 接近上限 |
| `critical` | 85-100% | 危险 |
| `exceeded` | >100% | 已超限 |
| `no-limit` | — | 无配额设置 |

**检测项：** 每命名空间的 CPU/内存/Pod/ConfigMap/Secret/存储配额利用率、无配额保护的命名空间、LimitRange 默认/最小/最大约束分析、Top 消费者排名

### 部署配置审计 (v14.75+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/deployments/audit` | 审计所有 Deployment/StatefulSet/DaemonSet 的配置最佳实践违规，8 个检查类别，每项包含严重度和修复建议 |

**查询参数：** `namespace`（可选）、`severity`（可选：critical / warning / info）

**检查类别：**
| 类别 | 检查项 |
|------|--------|
| `revision-history` | 修订历史太少（< 2，无法回滚）或太多（> 20，浪费资源） |
| `image-policy` | `:latest` 标签但 pullPolicy 不是 Always；固定标签但 pullPolicy 是 Always |
| `resources` | 缺少资源 limits/requests |
| `probes` | 缺少 liveness/readiness/startup 探针 |
| `security-context` | 特权容器、以 root 运行、可写根文件系统、允许提权 |
| `update-strategy` | Recreate 策略（停机）、OnDelete（需手动删 Pod）、分区滚动更新 |
| `lifecycle` | terminationGracePeriod 太短（< 10s）或太长（> 300s）、缺少 preStop 钩子 |
| `config-drift` | 缺少 seccomp profile |

**健康评分：** 0（完美）到 100（最差），critical=20分/warning=8分/info=2分

### 调度健康与资源碎片分析 (v14.76+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/scheduling/health` | 分析集群调度健康、节点可调度性、资源碎片化和 Pending Pod 诊断 |

**查询参数：** `namespace`（可选）

**返回内容：**
| 字段 | 说明 |
|------|------|
| `summary` | 节点统计（可调度/不可调度/已隔离/有压力）、Pending Pod 数、FailedScheduling 数、24h 驱逐数、健康评分 0-100 |
| `nodes` | 每节点可调度状态、压力类型、taints、CPU/内存/Pod 可用量和百分比 |
| `pendingPods` | Pending Pod 列表，含 CPU/内存请求、nodeSelector、解析后的调度失败原因 |
| `largestFittablePod` | 当前可调度的最大 Pod（CPU/内存/Pod 数量），最优节点 |
| `effectiveCapacity` | 理论容量 vs 有效容量（不可调度节点导致的容量损失百分比） |
| `fragmentation` | 资源碎片化指标（平均 CPU/内存碎片率、最差碎片节点、超大 Pod 检测） |
| `evictions` | 24h 内驱逐记录（Pod、节点、原因） |
| `recommendations` | 可操作的修复建议 |

**调度失败原因解析：** insufficient-cpu / insufficient-memory / untolerated-taint / node-selector-mismatch / node-affinity-mismatch / pod-affinity-conflict / pod-limit-reached / volume-binding-failure / no-nodes-available

### Pod 安全态势扫描 (v14.79+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/pods` | 审计所有运行中 Pod 的安全态势：特权容器、hostNetwork/hostPID/hostIPC、HostPath 挂载、危险 Linux capabilities、以 root 运行、允许提权、可写根文件系统、缺少安全上下文、:latest/无标签镜像、未用 digest 锁定、Secret 环境变量注入、无资源限制、host port 绑定 |

**查询参数：** `namespace`（可选）、`severity`（可选：critical / warning / info）

**风险评分：** 0（安全）到 100（极高风险），critical=25分/warning=8分/info=2分

**检查类别：**
| 类别 | 严重度 | 说明 |
|------|--------|------|
| `privileged` | critical | 特权容器 — 完全主机访问 |
| `host-network` | critical | 共享节点网络命名空间 |
| `host-pid` | critical | 可见节点所有进程 |
| `host-ipc` | critical | 共享 IPC 命名空间 |
| `host-path` | critical | 从节点挂载 HostPath 卷 |
| `dangerous-capabilities` | critical | SYS_ADMIN/NET_ADMIN/NET_RAW/SYS_PTRACE/SYS_MODULE/DAC_OVERRIDE/SETUID/SETGID |
| `runs-as-root` | warning | 以 UID 0 运行 |
| `privilege-escalation` | warning | 允许提权 |
| `missing-security-context` | warning | 缺少安全上下文 |
| `image-latest` | warning | 使用 :latest 标签 |
| `image-no-tag` | warning | 无标签（默认 :latest） |
| `host-port` | warning | 绑定主机端口 |
| `image-no-digest` | info | 未用 digest 锁定 |
| `writable-rootfs` | info | 可写根文件系统 |
| `secret-env-vars` | info | Secret 作为环境变量注入 |
| `no-resource-limits` | info | 无资源限制 |

### 事件风暴与级联故障检测 (v14.80+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/event-storm` | 分析集群 Warning 事件，检测事件风暴、级联故障和资源抖动。统计 15min/1h/24h 时间窗口的告警事件，分级风暴严重度，识别抖动资源（同资源同原因重复 3+ 次），按命名空间和原因聚合，提供可操作建议 |

**查询参数：** `namespace`（可选）

**风暴严重度：**
| 严重度 | 条件 | 说明 |
|--------|------|------|
| `critical` | >50 events/15min | 紧急排查 |
| `high` | >20 events/15min | 需要关注 |
| `medium` | >10 events/15min | 监控趋势 |
| `low` | >5 events/15min | 信息性 |

**返回内容：** 风暴检测结果、命名空间告警排名、Top 事件原因、抖动资源列表（含抖动频率）、最近 15 分钟事件时间线、受影响资源数（爆炸半径）、可操作建议

### 资源依赖图与影响范围分析 (v14.81+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/dependencies` | 追踪任意工作负载的完整依赖图（Deployment/StatefulSet/DaemonSet/Pod），评估变更影响范围 |

**查询参数：**

| 参数 | 必填 | 说明 |
|------|------|------|
| `kind` | 是 | 资源类型：Deployment / StatefulSet / DaemonSet / Pod |
| `name` | 是 | 资源名称 |
| `namespace` | 否 | 命名空间（默认：default） |

**正向依赖（该工作负载依赖什么）：** ConfigMap、Secret、PVC、ServiceAccount

**反向依赖（什么依赖该工作负载）：**
- Service（通过 label selector 匹配 Pod）
- Ingress（路由到匹配的 Service）
- NetworkPolicy（应用于该 Pod）
- HPA（以该工作负载为目标）
- 共享 ConfigMap/Secret 的其他 Pod

**影响范围评估：** blastRadius = 正向依赖数 + 反向依赖数，风险等级 low(<6) / medium(6-10) / high(11-20) / critical(>20)

### 拓扑分布合规检查 (v14.82+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/topology/spread` | 分析 Pod 在拓扑域（zone/region/node）中的分布，验证 topologySpreadConstraints 合规性 |

**查询参数：** `namespace`（可选）、`domain`（可选，拓扑域 key，默认 `kubernetes.io/hostname`，可设为 `topology.kubernetes.io/zone`）

**工作负载状态：**
| 状态 | 含义 |
|------|------|
| `balanced` | 分布均匀（actualSkew ≤ maxSkew） |
| `skewed` | 分布不均（actualSkew > maxSkew） |
| `no-constraint` | 多副本但无拓扑约束 |
| `single-replica` | 单副本（拓扑分布不适用） |

**返回内容：** 拓扑域统计、每工作负载的域分布（Pod 数/期望值）、实际偏差 vs 最大偏差、每节点的域标签和 Pod 数、建议（添加约束、标记节点、单域集群提示）

### Secret 轮转与生命周期审计 (v14.85+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/secrets/rotation` | 审计所有 Secret 的轮转合规性和生命周期管理：年龄追踪（stale >90d / very stale >180d）、未使用 Secret 检测（不被任何 Pod 引用）、TLS 证书过期检测（解析证书）、Docker registry Secret 追踪、遗留 ServiceAccount Token 检测、敏感名称检测 |

**查询参数：** `namespace`（可选）

**风险评分：** 每 Secret 风险等级（critical / high / medium / low），集群轮转评分 0-100

**检查类别：**
| 检查项 | 严重度 | 说明 |
|---------|--------|------|
| TLS 证书已过期 | critical | 立即更新 |
| Docker Secret 过期 >180d | critical | 可能包含过期的注册表凭据 |
| TLS 证书 <30d 过期 | high | 尽快安排续订 |
| Stale + 未使用 + 敏感名称 | high | 安全风险 |
| Stale Docker Secret | medium | 建议轮转 |
| Stale 但在使用中 | low | 计划轮转 |

### 健康探针有效性审计 (v14.86+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/probes` | 审计所有工作负载的 liveness/readiness/startup 探针配置，检测配置不当导致的级联重启、流量到未就绪 Pod、启动失败等问题 |

**查询参数：** `namespace`（可选）

**检查类别：**
| 检查项 | 严重度 | 说明 |
|---------|--------|------|
| 缺少 liveness | warning | 挂死容器不会被重启 |
| 缺少 readiness | warning | 流量可能到达未就绪 Pod |
| 探针过于激进 (period <5s) | warning | 对 API server 造成过大负载 |
| 超时过短 (<2s) | warning | 延迟峰值下可能误判 |
| 失败阈值过低 (<3) | warning | 对瞬时错误过于敏感 |
| readiness 间隔过长 (>60s) | info | 检测就绪慢 |
| liveness 失败阈值过高 (>10) | info | 重启恢复慢 |
| 相同的 liveness+readiness | info | 应差异化配置 |

**返回内容：** 每工作负载风险评分、集群有效性评分 (0-100)、聚合 Top 问题、可操作建议

### 工作负载陈旧度追踪 (v14.87+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/product/staleness` | 追踪所有工作负载的部署陈旧度，检测长期未更新的工作负载、使用 :latest 标签的镜像、未用 digest 锁定的镜像 |

**查询参数：** `namespace`（可选）

**陈旧度分类：**
| 状态 | 条件 | 说明 |
|------|------|------|
| `fresh` | <7d | 最近更新 |
| `recent` | <30d | 较新 |
| `stale` | <90d | 需关注 |
| `very-stale` | <180d | 建议更新 |
| `ancient` | >180d | 安全风险 |

**返回内容：** 每工作负载风险等级、镜像标签分析（:latest / digest / no-tag）、年龄分布桶、命名空间统计、集群新鲜度评分 (0-100)、可操作建议

### 资源超卖与压力分析 (v14.88+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/scalability/overcommit` | 分析所有节点的 CPU 和内存超卖比率，检测危险的 over-commit、无 limits 的 Pod、资源压力评分 |

**查询参数：** `namespace`（可选）

**每节点分析：**
| 指标 | 说明 |
|------|------|
| CPU request commit | sum(requests) / allocatable |
| CPU limit commit | sum(limits) / allocatable |
| Mem request/limit commit | 同上 |
| 压力评分 | 0-100（加权计算） |
| 风险等级 | safe / moderate / high / critical (>3x) |

**集群指标：** 总 CPU/内存超卖比率、风险节点数、无 limits 的 Pod 数、安全评分 (0-100)、命名空间资源消耗明细、可操作建议

### 镜像安全与供应链分析 (v14.92+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/images` | 扫描所有运行中容器镜像的供应链安全风险：digest 锁定、:latest 标签、无标签镜像、旧版本标签、公共 vs 私有镜像仓库、未知镜像仓库 |

**查询参数：** `namespace`（可选）

**检查类别：**
| 检查项 | 风险分 | 说明 |
|---------|--------|------|
| 无标签 | +25 | 默认使用 :latest，版本不确定 |
| 使用 :latest | +15 | 可变标签，不可复现 |
| 未用 digest 锁定 | +10 | 镜像内容可被静默替换 |
| 未知镜像仓库 | +10 | 无仓库前缀，默认 Docker Hub |
| 旧版本标签 | +15 | 可能包含已知漏洞 |
| 公共仓库 + 未锁定 | +5 | 无来源保证 |

**返回内容：** 每镜像风险等级 (critical/high/medium/low)、每仓库统计、Top 风险镜像、集群镜像安全评分 (0-100)、可操作建议

### 容量规划 (v14.50+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/capacity/planning` | 节点容量规划分析：每节点 CPU/内存请求 vs 可分配量、剩余容量、建议扩容时间、资源碎片化检测 |

### 集群健康评分聚合 (v14.93+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/health-score` | 聚合所有集群健康信号为一个综合评分 (0-100, 等级 A-F)，结合 5 个加权维度 |

**5 个加权维度：**
| 维度 | 权重 | 检查内容 |
|------|------|----------|
| Node Health | 25% | 节点就绪状态 |
| Pod Health | 25% | CrashLoop、Pending、Failed、高重启次数 |
| Workload Health | 20% | Deployment/StatefulSet/DaemonSet 就绪副本 |
| Event Activity | 15% | 最近 1 小时 Warning 事件数 |
| API Server | 15% | API server 实时延迟测量 |

**返回内容：** 总分 0-100、字母评级 A-F、状态 (healthy/warning/critical)、每维度评分详情、集群摘要 (节点/Pod/工作负载计数)、按严重度排序的 Top 问题

### HPA/VPA 资源合理配置建议 (v14.94+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/scalability/autoscale-recommendations` | 分析所有工作负载的 HPA 覆盖率和资源合理配置，检测过度配置、缺少 HPA 的多副本工作负载、HPA 效率 |

**查询参数：** `namespace`（可选）

**检测类别：**
| 检查项 | 说明 |
|---------|------|
| 缺少 HPA 的多副本工作负载 | 建议添加自动缩放 |
| CPU 请求过高 (>1 core/container) | 高置信度，建议减半 |
| 内存请求过高 (>2GB/container) | 建议右-sizing |
| HPA 达到 maxReplicas | 需要增加容量 |
| HPA 闲置 (<20% 利用率) | 建议减少 maxReplicas |

**返回内容：** 每工作负载当前 vs 建议 CPU/内存值、变化百分比、置信度、潜在 CPU 核心和内存节省、HPA 效率分析、集群自动缩放评分 (0-100)

### Ingress 与流量路由健康监控 (v14.96+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/product/ingress-health` | 分析所有 Ingress 资源的流量路由健康和配置问题 |

**查询参数：** `namespace`（可选）

**检查类别：**
| 检查项 | 严重度 | 说明 |
|---------|--------|------|
| 后端服务不存在 | critical | 引用的 Service 不存在 |
| 后端无就绪端点 | warning | Service 无 ready endpoints |
| 无 TLS 配置 | warning | 有 host 但未加密 |
| IngressClass 不存在 | critical | 指定的 class 未部署 |
| host+path 冲突 | warning | 多个 Ingress 争抢同一路由 |
| 无路由规则 | warning | Ingress 不起作用 |

**返回内容：** 每 Ingress 状态 (healthy/warning/critical)、每命名空间统计、集群健康评分 (0-100)、可操作建议

### 节点条件与资源压力分析 (v14.99+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/node-pressure` | 分析所有节点的条件状态和资源饱和度 |

**检测类别：**
| 条件 | 风险分 | 说明 |
|------|--------|------|
| NetworkUnavailable | +30 | CNI/网络未就绪 |
| DiskPressure | +25 | 磁盘满或接近满 |
| MemoryPressure | +25 | 节点内存耗尽 |
| PIDPressure | +20 | 进程数过多 |
| NotReady | →critical | kubelet/运行时问题 |
| CPU >90% | +20 | CPU 请求饱和 |
| Memory >95% | +20 | 内存请求饱和 |
| Cordoned | — | 不可调度 |

**返回内容：** 每节点风险等级 (critical/high/medium/low)、CPU/内存/Pod 使用率、条件详情（原因、消息、持续时间）、集群压力评分 (0-100)、可操作建议

### PVC 绑定与存储性能分析 (v15.00+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/scalability/pvc-analysis` | 分析所有 PVC 的绑定健康和存储性能 |

**查询参数：** `namespace`（可选）

**检测类别：**
| 检查项 | 严重度 | 说明 |
|---------|--------|------|
| Stuck PVC (>5min) | critical | 卡住的 PVC + 根因分析 |
| Lost PVC | critical | 底层 PV 可能被删除 |
| 慢绑定 (>30s) | warning | 存储供应延迟 |
| Pending PVC | warning | 正在等待绑定 |
| 缺少默认 StorageClass | info | 未设置默认 SC |

**返回内容：** 每 PVC 状态 (healthy/warning/critical)、绑定时间、每 StorageClass 统计、Stuck PVC 根因、集群存储健康评分 (0-100)

### Namespace 治理与生命周期审计 (v15.02+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/product/namespaces/lifecycle` | 审计所有命名空间的治理合规性和生命周期 |

**治理检查：**
| 检查项 | 风险分 | 说明 |
|---------|--------|------|
| 无 ResourceQuota | +15 | 资源无限消耗 |
| 无 NetworkPolicy | +15 | 流量不受限 |
| 无 LimitRange | +5 | 无默认资源限制 |
| 命名空间过期 | +10 | 无运行 Pod，清理候选 |
| 缺少必需标签 | +5 | 缺 app/team/env/owner |
| 仅 default SA | 0 | 缺少最小权限 SA |

**返回内容：** 每命名空间风险等级 (critical/high/medium/low)、合规标志、生命周期状态 (active/stale/terminating)、集群治理评分 (0-100)、可操作建议

### RBAC 有效权限与提权分析 (v15.04+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/rbac-effective` | 分析所有主体的 RBAC 有效权限和提权风险 |

聚合 ClusterRoleBindings + RoleBindings，计算每个主体 (User/Group/ServiceAccount) 的实际权限。

**检测类别：**

| 检查项 | 风险分 | 说明 |
|---------|--------|------|
| cluster-admin 等效 | →critical | 通配符 verbs + resources |
| 可创建/修改 RBAC | +25 | 自我提权路径 |
| 通配符 (*) 权限 | +20 | 过度授权 |
| 可读取 Secrets | +10 | 敏感数据泄露 |
| 可 exec Pod | +10 | 容器逃逸入口 |

**返回内容：** 每主体风险等级、提权路径详情、集群 RBAC 安全评分 (0-100)、可操作建议

### 容器 OOM Kill 追踪器 (v15.05+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/oom-tracker` | 追踪容器 OOMKill 事件和内存配置分析 |

**查询参数：** `namespace`（可选）

**检测类别：**

| 检查项 | 风险分 | 说明 |
|---------|--------|------|
| OOMKilled 容器 | +15/个 | 内存不足被杀死 |
| 高重启次数 (>=10) | +20 | CrashLoop 指标 |
| 高重启次数 (>=5) | +10 | 频繁重启 |
| 无内存限制 | +5 | OOM 行为不可预测 |
| 低内存限制 (<256MB) | — | 可能导致不必要的 OOM |
| 限制>>请求 (10x+) | — | 节点内存压力风险 |

**返回内容：** 每 Pod OOM 风险等级、Top OOM 排名、每命名空间统计、集群 OOM 风险评分 (0-100)

### 存储容量耗尽预测器 (v15.06+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/scalability/storage-forecast` | 预测存储容量何时耗尽 |

基于 PV 使用趋势和增长率估算，预测存储空间耗尽时间。

**分析维度：**

| 指标 | 说明 |
|------|------|
| 容量 vs 已用 | 支持 Longhorn actual-size 注解获取真实使用量 |
| 日增长率 | 基于使用率和 PV 年龄的启发式估算 |
| 耗尽天数 | 剩余空间 / 日增长率 |
| 预测耗尽日期 | 日期或 ">10年" 或 "无增长" |
| 风险等级 | critical(>95%) / high(>85%或<14d) / medium(<30d) / low |

**返回内容：** 每 PV 预测、集群存满天数估算、每 StorageClass 统计、高风险命名空间排名、存储健康评分 (0-100)

### DNS 解析健康检查器 (v15.08+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/product/dns-health` | 分析集群 DNS 解析健康状态 |

**CoreDNS 分析：**

| 检查项 | 说明 |
|---------|------|
| Pod 健康 | running/ready/restarts/version per pod |
| Corefile | forwarders, plugins, missing Corefile 检测 |
| 副本数 | 推荐 >= 2 用于高可用 |

**其他检测：**
- Headless Service 端点覆盖 (NXDOMAIN 风险)
- NodeLocal DNS 缓存检测
- Pod dnsConfig ndots 覆盖检测 (>5 = 过多 DNS 查询)
- External-DNS 托管服务发现

**返回内容：** CoreDNS Pod 状态、Headless Service 覆盖率、DNS 配置分析、集群 DNS 健康评分 (0-100)、可操作建议

---

### 36. ConfigMap & Secret 配置审计 (v15.14)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/product/config-audit` | 审计 ConfigMap 和 Secret 的配置最佳实践 |
| GET | `/api/product/config-audit?namespace=xxx` | 按命名空间过滤 |

**ConfigMap 分析：**

| 检查项 | 说明 |
|---------|------|
| 超大 ConfigMap | >1MB 检测（拖慢 etcd 和 API）|
| 未引用 ConfigMap | 无 Pod 通过 volume/env/envFrom 使用 |
| 空数据键 | 无 Data/BinaryData |
| 不可变标志 | immutable=true 检查 |

**Secret 分析：**

| 检查项 | 说明 |
|---------|------|
| 过期凭证 | >180 天未轮换 |
| 未引用 Secret | 安全删除候选 |
| 明文凭证键 | Opaque 类型含 password/token/key |
| 轮换建议 | External Secrets Operator / Sealed Secrets |

**返回内容：** ConfigMap/Secret 列表（含大小、引用状态、风险等级）、未引用资源列表、超大 ConfigMap、集群配置审计健康评分 (0-100)、可操作建议

---

### 37. 容器镜像部署规范分析 (v15.13)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/deployment/image-hygiene` | 分析所有运行中容器的镜像部署规范 |

**每镜像分析：**

| 指标 | 说明 |
|------|------|
| 标签策略 | 版本号 / :latest / 无标签 / @sha256 摘要锁定 |
| 仓库信任 | 可信仓库 (k8s.io, gcr.io, quay.io...) vs 不可信 |
| 风险等级 | high (latest+无摘要+docker.io) / medium / low |
| 副本数 | 使用该镜像的容器总数 |

**重复检测：** 相同基础镜像 + 不同标签 → 合并建议

**仓库分布：** 每仓库镜像数、Pod 数、latest 使用量、可信度

**返回内容：** 镜像列表（含标签策略、仓库、风险）、重复镜像、仓库分布、集群镜像规范评分 (0-100)、可操作建议

---

### 38. Deployment 滚动更新策略与健康分析 (v15.19)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/deployment/rollout-health` | 分析所有 Deployment 的滚动更新策略和健康状态 |

**每部署分析：**

| 指标 | 说明 |
|------|------|
| 策略类型 | RollingUpdate / Recreate |
| maxSurge / maxUnavailable | 滚动更新配置 |
| revisionHistoryLimit | 回滚就绪评估 |
| progressDeadlineSeconds | 卡住部署超时检测 |
| 副本状态 | 期望/已更新/就绪/可用/不可用 |

**状态分类：**
- **healthy** — 所有副本就绪且已更新
- **progressing** — 滚动更新进行中
- **stuck** — Progressing=False / ReplicaFailure / 超时
- **paused** — 已暂停

**风险检测：** Recreate 策略多副本(停机)、revisionHistoryLimit=0(无法回滚)、激进 progressDeadline(<300s)

**返回内容：** 部署列表（含策略、状态、风险）、卡住的滚动更新列表、不佳策略列表、集群滚动更新健康评分 (0-100)、可操作建议

---

### 39. 证书与 TLS 过期监控 (v15.16)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/cert-expiry` | 监控所有 TLS 证书的过期状态 |

**每证书分析：**

| 指标 | 说明 |
|------|------|
| CN / SANs | 通用名和主体备选名称 |
| 颁发者 | CA 或自签名检测 (Subject == Issuer) |
| 有效期 | NotBefore / NotAfter |
| 剩余天数 | 过期倒计时 |
| 风险等级 | critical (过期/<30d) / high (<60d) / medium (<90d) / low (>90d) |
| 引用状态 | 是否被运行中的 Pod 通过卷挂载使用 |

**返回内容：** 过期证书列表、即将过期证书列表（30/60/90天窗口）、所有证书列表、每命名空间统计、集群证书健康评分 (0-100)、可操作建议

---

### 40. PDB 合规与自愿中断风险分析 (v15.17)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/pdb-audit` | 审计 PodDisruptionBudget 合规性和自愿中断风险 |

**PDB 状态分类：**

| 状态 | 条件 | 影响 |
|------|------|------|
| healthy | allowed > 0 | 节点排空可成功 |
| blocked | allowed = 0 | 节点排空将停滞 |
| impossible | minAvailable > pods | 永远无法满足 |

**未保护部署检测：** 多副本 Deployment 无 PDB → 按副本数风险分级

**节点排空模拟：** 逐节点分析受影响 Pod，识别哪些 PDB 会阻止驱逐

**返回内容：** 受保护工作负载列表、未保护部署列表、阻塞 PDB 列表、节点排空模拟结果、集群 PDB 覆盖评分 (0-100)、可操作建议

---

### 41. 命名空间资源消耗与成本归属 (v15.18)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/scalability/ns-consumption` | 按命名空间聚合资源消耗并估算成本 |

**每命名空间分析：**

| 指标 | 说明 |
|------|------|
| CPU/内存请求 + 限制 | 聚合所有容器的 requests 和 limits |
| 存储容量 | 从已绑定 PVC 聚合 |
| 月成本估算 | CPU $28/核 + 内存 $3.8/GB + 存储 $0.10/GB |
| 资源效率 | request/limit 比率 (%) |
| 过度提交比率 | limit/request (>5x = 高风险) |
| 成本占比 | 该 NS 占集群总成本的百分比 |

**浪费分析：** 过度配置 NS、空闲 NS、浪费容量、浪费评分 (0-100)

**返回内容：** 命名空间消耗排行、Top 10 消费者、空闲命名空间、浪费分析（含成本）、可配置定价模型、FinOps 建议

---

### 42. Network Policy 合规与流量隔离审计 (v15.20)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/product/network-policy` | 审计 NetworkPolicy 覆盖率和流量隔离 |

**每命名空间分析：**

| 指标 | 说明 |
|------|------|
| Pod 数量 | 运行中的 Pod 总数 |
| 策略数量 | NetworkPolicy 数量 |
| 受保护 Pod | 被至少一个策略选中的 Pod |
| 默认拒绝 | 是否有 default-deny 策略 |
| 隔离评分 | 0-100（覆盖率 + 默认拒绝加成）|
| 风险等级 | critical (<30) / high (<60) / medium (<85) / low (>=85) |

**策略分析：** 入站/出站规则计数、Default-deny 检测、宽松出站检测 (0.0.0.0/0 = 数据泄露风险)

**返回内容：** 命名空间隔离统计、未保护 Pod 列表、所有策略列表（含宽松标记）、集群隔离评分 (0-100)、可操作建议

---

### 43. 卷安全与挂载风险审计 (v15.22)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/volume-mounts` | 审计所有 Pod 卷挂载的容器逃逸风险 |
| GET | `/api/security/volume-mounts?namespace=xxx` | 按命名空间过滤 |

**危险路径检测 (14 个已知路径)：**

| 路径 | 风险 |
|------|------|
| `/var/run/docker.sock` | Docker socket — 容器逃逸，完全控制宿主机 |
| `/proc`, `/sys`, `/dev` | 内核和硬件访问 |
| `/` | 根文件系统 — 完全宿主机访问 |
| `/etc/kubernetes` | 集群凭证窃取 |
| `/var/lib/kubelet` | kubelet 数据 — Pod 注入、凭证窃取 |
| `/var/lib/etcd` | etcd 数据 — 全集群状态和 Secrets |
| `/root/.kube` | kubeconfig — 集群管理员凭证窃取 |

**HostPath 分析：** 风险等级 (critical/high/medium/low)、读写检测、路径敏感度分类

**额外检测：** 特权容器、Host namespace 共享 (hostNetwork/hostPID/hostIPC)、SA Token 投射卷追踪

**返回内容：** 危险挂载列表、HostPath 卷列表、SA Token 卷列表、每命名空间风险统计、集群安全评分 (0-100，越高越安全)、可操作建议

---

### 44. 拓扑分布与 Pod 分配审计 (v15.23)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/topology-distribution` | 审计 Pod 在节点间的分布和拓扑约束合规性 |
| GET | `/api/operations/topology-distribution?namespace=xxx` | 按命名空间过滤 |

**每工作负载分析：**

| 指标 | 说明 |
|------|------|
| 节点分布图 | 每节点的 Pod 数量 |
| 每节点最大值 | 单节点上最多的 Pod 数 |
| 唯一节点数 | 跨多少个不同节点 |
| 分布比率 | 唯一节点 / 期望节点 |
| TSC 状态 | 是否有 topologySpreadConstraints |
| 反亲和性 | 是否有 podAntiAffinity |

**风险分类：**
- **critical** — >70% 副本在一个节点（单节点故障即全灭）
- **high** — >50% 在一个节点
- **medium** — >34% 在一个节点
- **low** — <34%（良好分布）

**返回内容：** 工作负载分布分析、集中部署列表、良好分布列表、节点负载不均衡分析、分布评分 (0-100)、可操作建议

---

### 45. 集群容量余量与扩容就绪 (v15.24)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/scalability/capacity-headroom` | 分析集群容量余量和扩容就绪状态 |

**每节点分析：**

| 指标 | 说明 |
|------|------|
| Allocatable vs Used | 可分配 vs 已用 CPU/内存 |
| Headroom % | CPU/内存/Pod 槽位剩余百分比 |
| Bottleneck | 最紧张的资源 (cpu/memory/pods) |
| Full Node | <10% 余量的节点检测 |

**Pod 调度容量估算：**

| 配置 | CPU | 内存 | 说明 |
|------|-----|------|------|
| small | 100m | 128MB | 微服务 |
| medium | 500m | 512MB | 标准服务 |
| large | 1 core | 1GB | 数据库 |
| xlarge | 2 core | 4GB | 计算密集型 |

**扩容就绪：** Cluster Autoscaler/Karpenter 检测，紧急程度 (immediate/soon/no)

**返回内容：** 集群容量汇总、每节点余量分析、瓶颈节点列表、Pod 调度容量估算、扩容就绪状态、余量评分 (0-100)、可操作建议

---

### 46. 健康探针合规审计 (v15.25)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/deployment/probe-compliance` | 审计所有 Deployment 的健康探针配置 |
| GET | `/api/deployment/probe-compliance?namespace=xxx` | 按命名空间过滤 |

**每容器分析：**

| 检查项 | 说明 |
|---------|------|
| Liveness | 检测配置/缺失 + 完整详情 (类型/路径/端口/时序) |
| Readiness | 检测配置/缺失 + 完整详情 |
| Startup | 启动探针检测 (慢启动应用保护) |
| Probe Type | httpGet / tcpSocket / exec |

**问题检测：**
- **critical** — 零探针（无任何健康监控）
- **critical** — 缺少 readiness（流量发送到不健康的 Pod）
- **warning** — 缺少 liveness（僵死容器不会自动重启）
- **info** — TCP socket 探针（不如 HTTP 可靠）
- **info** — 缺少 startup（慢启动应用误判为不健康）

**配置错误检测：** 过长 initialDelay (>120s/180s)、慢 period (>60s/30s)、高 failureThreshold (>10)、长 timeout (>10s)

**返回内容：** 工作负载探针详情、缺失 readiness 列表、缺失 liveness 列表、配置错误列表、探针合规评分 (0-100)、可操作建议

---

### 47. 标签与注解卫生审计 (v15.26)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/product/label-hygiene` | 审计所有工作负载的标签和注解卫生 |
| GET | `/api/product/label-hygiene?namespace=xxx` | 按命名空间过滤 |

**每工作负载检查：**

| 检查项 | 严重程度 | 影响 |
|---------|---------|------|
| 零标签 | critical | Service 选择器、监控、NetworkPolicy 全部失效 |
| 缺少 app.kubernetes.io/name | warning | kubectl、Helm、监控发现中断 |
| 缺少 team/owner 标签 | info | 所有权追踪和 FinOps 成本归属断裂 |
| 畸形标签键 | high | 非 DNS-1123 格式（必须小写）|
| 过多标签 (>20) | info | 性能和可管理性问题 |

**DNS-1123 合规验证：** 标签键名必须为小写字母数字、'-'、'.'，前缀/名称格式校验

**返回内容：** 工作负载标签详情、零标签列表、缺失标准标签列表、缺失团队标签列表、畸形键列表、每命名空间评分、集群合规评分 (0-100)、可操作建议

---

### 48. 服务端点暴露与攻击面审计 (v15.28)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/endpoint-exposure` | 审计所有服务的暴露面和攻击面 |
| GET | `/api/security/endpoint-exposure?namespace=xxx` | 按命名空间过滤 |

**每服务分析：**

| 指标 | 说明 |
|------|------|
| Type | ClusterIP / NodePort / LoadBalancer |
| Exposure Level | public (LB/ExternalIP) / node (NodePort) / internal |
| External IPs | 手动设置的防火墙绕过检测 |
| Port Analysis | HTTP (80/8080/3000/5000) vs HTTPS (443/8443) |
| NP Coverage | 命名空间是否有 NetworkPolicy |

**每 Ingress 分析：** 域名列表、TLS 状态（明文流量检测）、后端服务、HTTP vs TLS 路由计数

**风险检测：** critical（公开暴露+无NetworkPolicy）、high（Ingress 无 TLS）、warning（NodePort 暴露在所有节点）

**返回内容：** 暴露服务列表、Ingress 路由列表、内部服务列表、每命名空间暴露统计、攻击面评分 (0-100，越高越安全)、可操作建议

---

### 49. ImagePullBackOff 与容器启动失败追踪 (v15.29)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/image-pull-failures` | 追踪镜像拉取失败和容器启动失败 |
| GET | `/api/operations/image-pull-failures?namespace=xxx` | 按命名空间过滤 |

**每容器分析：** 镜像名、失败原因、错误消息、重启次数、风险等级

**失败类型：** ImagePullBackOff、ErrImagePull、ErrImageNeverPull、CreateContainerError、CreateContainerConfigError、CrashLoopBackOff

**根因分类：**

| 根因 | 检测方式 |
|------|---------|
| Registry 认证失败 | 错误消息含 unauthorized / authentication |
| Docker Hub 限速 | 错误消息含 rate limit / toomanyrequests |
| 无效镜像名 | ImagePullBackOff + 镜像格式错误 |
| 容器配置错误 | CreateContainerConfigError |

**返回内容：** 失败容器列表、按镜像聚合统计、按命名空间统计、按原因分布、镜像拉取健康评分 (0-100)、可操作建议

---

### 50. 资源配额使用率与限制合规审计 (v15.30)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/scalability/quota-utilization` | 审计 ResourceQuota 使用率和 LimitRange 合规性 |
| GET | `/api/scalability/quota-utilization?namespace=xxx` | 按命名空间过滤 |

**ResourceQuota 分析：**

| 指标 | 说明 |
|------|------|
| Hard/Used | 每资源的硬限制和已用量 |
| Utilization % | 精确使用率 (MilliValue 计算) |
| Risk Level | critical (>90%) / high (>80%) / medium (>60%) / low (<60%) |

**LimitRange 分析：** Default request/limit 是否存在、Max limit 强制执行检测、不完整 LimitRange 告警

**容器资源治理：** 缺失 requests 的容器（调度器盲区）、缺失 limits 的容器（吵闹邻居风险）

**返回内容：** 配额列表（含使用率）、关键配额列表、LimitRange 列表、未受限 Pod 列表、每命名空间统计、配额合规评分 (0-100)、可操作建议

---

### 51. 资源限制与强制差距审计 (v15.32)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/deployment/resource-limits` | 审计资源限制与强制差距 |
| GET | `/api/deployment/resource-limits?namespace=xxx` | 按命名空间过滤 |

**每容器分析：** CPU/内存请求与限制（人类可读 + 毫核值/MB）、请求/限制比率、风险等级

**问题检测：** 无限制容器（critical）、无内存限制（critical，OOM 级联风险）、无 CPU 限制（high）、供应不足 <1.2x（high，突发余量紧张）、供应过度 >4x（medium，容量浪费）、过度请求 >2000m CPU 或 >4Gi 内存

**合规评分 (0-100)：** 无限制(-15)、无内存限制(-8)、无CPU限制(-4)、无请求(-5)、供应不足(-3)

---

### 52. 孤立资源检测器 (v15.33)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/product/orphaned-resources` | 检测 5 种孤立资源 |
| GET | `/api/product/orphaned-resources?namespace=xxx` | 按命名空间过滤 |

**5 种资源类型：**

| 资源 | 检测逻辑 | 风险 |
|------|---------|------|
| Services | Selector 返回零个 Pod | 流量无处可去 |
| ConfigMaps | 不被任何 Pod 引用 | 存储浪费 |
| Secrets | 不被任何 Pod 引用 | 过期凭证（high）|
| PVCs | 不被任何 Pod 挂载 | 存储容量浪费 |
| Ingresses | 后端 Service 不存在 | 用户 404/502 |

**Pod 引用追踪：** 卷挂载、环境变量、envFrom、ImagePullSecrets、Projected volumes

**集群卫生评分 (0-100)：** 基于孤立率（每 1% 扣 2 分）

---

### 53. Seccomp 与 PSS Restricted 合规审计 (v15.34)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/seccomp-audit` | 审计 Seccomp 配置文件与 PSS 合规 |
| GET | `/api/security/seccomp-audit?namespace=xxx` | 按命名空间过滤 |

**每容器分析：**

| 检查项 | 说明 |
|---------|------|
| Seccomp Profile | RuntimeDefault / Localhost / Unconfined / unset |
| Capabilities | drop/add 列表，droppedALL 标记 |
| allowPrivilegeEscalation | 默认 true（如未显式 false）|
| runAsNonRoot/runAsUser | root 运行检测 |
| readOnlyRootFilesystem | 只读根文件系统 |
| Privileged Flag | 特权容器检测 |

**PSS 级别分类：** restricted (零违规) / baseline (部分合规) / privileged (不合规)

**危险 Capability 检测 (11 个)：** SYS_ADMIN, SYS_MODULE, SYS_PTRACE, NET_ADMIN, NET_RAW, DAC_OVERRIDE 等

**容器加固评分 (0-100)：** 无 seccomp(-8)、特权(-20)、未 drop ALL(-4)、可提权(-6)、root 运行(-3)

---

### 54. Pod 重启原因分析器 (v15.35)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/restart-reasons` | 分析 Pod 重启原因 |
| GET | `/api/operations/restart-reasons?namespace=xxx` | 按命名空间过滤 |

**原因分类：**

| 类别 | 检测方式 |
|------|---------|
| OOMKilled | exit 137 或 reason=OOMKilled |
| 应用错误 | exit != 0，非 OOM |
| 配置错误 | CreateContainerError, ErrImagePull |
| 超时 | DeadlineExceeded (Jobs) |
| 正常退出 | exit 0 (Jobs/CronJobs) |

**Top 20 重启最多容器** — 按重启次数排序

**集群稳定性评分 (0-100)：** 基于重启/总 Pod 比率（1.5x 惩罚）

---

### 55. 高可用与单点故障检测器 (v15.36)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/scalability/ha-audit` | 检测单点故障与 HA 合规 |
| GET | `/api/scalability/ha-audit?namespace=xxx` | 按命名空间过滤 |

**5 种 SPOF 检测：**

| 检测项 | 严重程度 | 影响 |
|--------|---------|------|
| 单副本 Deployment | critical | 任何重启导致停机 |
| 单节点分布 (多副本但全在一个节点) | critical | 节点故障全灭 |
| 多副本无 PDB | high | 自愿中断同时杀死所有 Pod |
| 无 Pod 反亲和性 | medium | 调度器可能将所有 Pod 放在同一节点 |
| 缺少 Readiness Probe | medium | 故障转移缓慢 |

**HA 评分 (0-100)：** 单副本(-15)、单节点(-12)、无PDB(-6)、无反亲和(-3)、无Readiness(-4)

---

### 56. 优雅终止与终止合规审计 (v15.38)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/deployment/graceful-shutdown` | 审计优雅终止配置 |
| GET | `/api/deployment/graceful-shutdown?namespace=xxx` | 按命名空间过滤 |

**每容器分析：**

| 检查项 | 说明 |
|---------|------|
| preStop Hook | httpGet (path:port) 或 exec (command) |
| Readiness Probe | 终止前从 endpoints 移除所需 |
| Grace Period | short (<10s) / default (30s) / custom / long (>60s) |

**风险分类：** critical（无 preStop + 无 readiness，滚动更新必丢请求）、high（无 preStop）、medium（无 readiness）、low（完全合规）

**优雅终止评分 (0-100)：** 丢弃请求(-15)、无 preStop(-5)、无 readiness(-4)、短 grace(-3)

---

### 57. PV/PVC 存储健康与容量审计 (v15.39)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/product/pvc-health` | 审计 PV/PVC 存储健康 |
| GET | `/api/product/pvc-health?namespace=xxx` | 按命名空间过滤 |

**每 PVC 分析：** Phase（Bound/Pending/Lost）、StorageClass、Access Modes、Capacity

**每 PV 分析：** Phase（Bound/Available/Released/Failed）、Reclaim Policy（Retain/Delete）

**StorageClass 分析：** Provisioner、Volume Binding Mode、AllowExpansion、默认 SC 检测、PVC 计数

**问题检测：** Pending PVC、Lost PVC、Failed PV、Released PV（孤立存储）、无扩容 SC、无默认 SC

**存储健康评分 (0-100)：** Pending(-8)、Lost(-20)、Failed PV(-15)、Released(-3)

---

### 58. CronJob 与批处理作业安全审计 (v15.40)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/batch-audit` | 审计 CronJob 与 Job 安全 |
| GET | `/api/security/batch-audit?namespace=xxx` | 按命名空间过滤 |

**每工作负载分析：**

| 检查项 | 严重程度 | 说明 |
|--------|---------|------|
| Privileged | critical | 特权容器 — 完全节点访问 |
| HostPath | critical | 挂载路径（可读写节点文件系统）|
| HostNetwork/PID | high | 共享节点网络/进程命名空间 |
| Default SA | medium | 使用默认 ServiceAccount — 可能继承广泛 RBAC |
| No Limits | medium | 无资源限制 — 可耗尽节点资源 |
| Suspicious Schedule | warning | 每分钟执行 — 潜在持久化机制 |
| No Concurrency | warning | Allow 策略 — fork-bomb 风险 |

**批处理安全评分 (0-100)：** 特权(-20)、hostPath(-15)、hostNetwork/PID(-8)、默认SA(-4)、无限制(-3)、可疑调度(-6)

---

### 59. Pod 调度延迟分析器 (v15.41)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/scheduling-latency` | 分析 Pod 调度延迟 |
| GET | `/api/operations/scheduling-latency?namespace=xxx` | 按命名空间过滤 |

**每 Pod 分析：** 从创建到 PodScheduled 的时间（秒）、当前 Phase、分配的节点、Pending 原因

**检测：**

| 检测项 | 严重程度 |
|--------|---------|
| Unschedulable Pods | warning |
| Resource Shortage (Insufficient cpu/memory) | critical |
| Slow Scheduling (>60s) | high |
| Very Slow Scheduling (>300s) | critical |

**每节点：** 平均调度时间、慢 Pod 计数

**调度效率评分 (0-100)：** unschedulable(-10)、resource shortage(-12)、very slow(-6)、slow(-3)、pending(-4)

---

### 60. 节点故障影响模拟器 (v15.42)

**路径：** `GET /api/scalability/node-failure-sim`

模拟每个节点故障后的影响，用于 HA 规划和容量管理。

**每节点分析：**

| 指标 | 说明 |
|------|------|
| Affected Pods | 节点上的非 DaemonSet/非系统 Pod 数量 |
| Can Reschedule | 能在其他节点找到资源（容量/selector/taint 检查）|
| Unschedulable | 无法在任何其他节点调度 |
| Single-Replica WL | 节点故障会导致永久停机的工作负载 |
| CPU/Memory Requests | 节点上的总请求量 |

**重调度可行性检查：** 资源容量、Node Selector 匹配、Taint/Toleration（NoSchedule/NoExecute）、Node Ready 状态

**模拟排除：** DaemonSet Pod（每节点都有）、Completed/Failed Pod、kube-system Pod

**风险分类：** critical（单副本 WL 或 >5 unschedulable）、high（>10 affected）、medium（部分 unschedulable）、low（全部可重调度）

**弹性评分 (0-100)：** 单副本节点(-12)、关键节点(-6)、unschedulable 均值(-4)

---

### 61. 部署更新策略与回滚就绪审计 (v15.44)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/deployment/update-strategy` | 审计部署更新策略 |
| GET | `/api/deployment/update-strategy?namespace=xxx` | 按命名空间过滤 |

**每部署分析：**

| 检查项 | 说明 |
|---------|------|
| Strategy Type | RollingUpdate（安全）/ Recreate（停机）|
| maxSurge / maxUnavailable | 滚动更新参数 |
| revisionHistoryLimit | 回滚能力 |
| progressDeadlineSeconds | 卡住部署检测 |

**风险分类：** critical（Recreate 策略）、high（maxUnavailable=100%）、medium（其他违规）、low（干净的 RollingUpdate）

**就绪评分 (0-100)：** Recreate(-15)、maxUnavailable=100%(-10)、maxSurge=0(-5)、低版本历史(-4)、无进度截止(-3)

---

### 62. StatefulSet 健康与有序滚动更新审计 (v15.45)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/product/statefulset-audit` | 审计 StatefulSet 健康 |
| GET | `/api/product/statefulset-audit?namespace=xxx` | 按命名空间过滤 |

**每 StatefulSet 分析：**

| 检查项 | 说明 |
|---------|------|
| Pod Management Policy | OrderedReady（慢）/ Parallel（快）|
| PVC Retention Policy | Retain（安全）/ Delete（数据丢失风险）|
| Headless Service | 存在性（Pod DNS 解析依赖）|
| VolumeClaimTemplates | 存在性（无则应改用 Deployment）|
| Partition Canary | 分区金丝雀暂停状态 |

**检测：** 无 headless service (critical)、滚动更新卡住 (high)、PVC Delete 策略 (high)、暂停金丝雀 (warning)

**健康评分 (0-100)：** 无 headless(-15)、卡住(-8)、PVC delete(-5)、partition(-4)、无 PVC(-3)

---

### 63. 资源争用与限流检测器 (v15.46)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/resource-contention` | 检测资源争用与限流 |
| GET | `/api/operations/resource-contention?namespace=xxx` | 按命名空间过滤 |

**检测项：**

| 问题 | 严重程度 | 影响 |
|------|---------|------|
| 节点 MemoryPressure/DiskPressure | critical | Pod 可能被 OOM/驱逐 |
| 高重启 Pod（≥3 次）| warning | 可能 CPU 限流导致探针超时 |
| 无 CPU 限制 | warning | 可耗尽邻居 Pod 资源 |
| 无内存限制 | warning | OOM 可级联影响共存 Pod |
| CPU 限制 <100m | warning | 负载下被限流 |
| 内存限制 <128Mi | warning | 负载下 OOMKilled |

**争用评分 (0-100)：** memory pressure(-12)、throttled(-6)、low CPU(-4)、no CPU(-2)、no memory(-3)

---

### 64. API 对象计数与 CRD 爆炸风险检测器 (v15.48)

**路径：** `GET /api/scalability/crd-explosion`

随着集群增长，过多的对象计数（ConfigMaps、Secrets、CRDs）会拖慢 API server 的 list/watch 操作并增大 etcd 大小。

**每资源类型分析：** 对象数、风险级别（>1000 critical, >500 high, >200 medium）

**每命名空间分析：** ConfigMap/Secret/Service/Pod 计数、总对象数

**检测项：** 对象计数 >1000（warning）、Secret >100/命名空间（warning）、ConfigMap >200/命名空间（info）、CRD 总数 >30（info）

**可扩展性评分 (0-100)：** very high count(-10)、high count(-5)、excessive ConfigMaps(-5)、excessive Secrets(-5)、excessive CRDs(-5)

---

### 65. Secret/ConfigMap 引用完整性检查 (v15.49)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/deployment/ref-integrity` | 检查 Secret/ConfigMap 引用完整性 |
| GET | `/api/deployment/ref-integrity?namespace=xxx` | 按命名空间过滤 |

缺失的引用是部署后 CrashLoopBackOff 的第一大原因。

**检查范围：** Deployment、StatefulSet、DaemonSet

**检查所有引用来源：** volume mounts（configMap/secret）、envFrom（configMapRef/secretRef）、env valueFrom（configMapKeyRef/secretKeyRef）

**检测项：** 引用不存在的 Secret/ConfigMap（critical，Pod 启动失败）、optional=true 但缺失（low）

**完整性评分 (0-100)：** 每个损坏引用(-15)

---

### 66. Pod 亲和性/反亲和性冲突检测 (v15.50)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/product/affinity-conflict` | 检测亲和性/反亲和性冲突 |
| GET | `/api/product/affinity-conflict?namespace=xxx` | 按命名空间过滤 |

Pod 反亲和性规则不可满足是生产环境中 Pending Pod 的主要原因之一。

**每 Pod 分析：** 是否有亲和性/反亲和性、类型（required/preferred）、TopologyKey、匹配标签、Pending 原因

**拓扑域分析：** 从节点标签构建域映射（hostname/zone/region），检查 required 反亲和性是否可满足

**检测项：** 不可满足的反亲和性（critical，拓扑域太小）、因亲和性 Pending（high）、Required 硬反亲和性（medium）

**健康评分 (0-100)：** conflicts(-15)、pending affinity(-8)、required anti-affinity(-2)

---

### 67. 节点心跳与健康租约监控 (v15.52)

**路径：** `GET /api/operations/node-lease`

通过 kube-node-lease 命名空间的 Lease 对象监控 kubelet 心跳新鲜度，对检测僵尸节点至关重要。

**每节点分析：**

| 检查项 | 说明 |
|--------|------|
| Lease 存在性 | kube-node-lease 中是否存在该节点的 Lease |
| 心跳年龄 | renewTime 到现在的秒数 |
| Holder Identity | kubelet 标识 |
| Kubelet 版本 | 节点 kubelet 版本 |
| 活跃负面条件 | MemoryPressure、DiskPressure 等 |

**检测项：** 无 Lease（critical）、心跳 >2min（critical）、心跳 >40s（high）、NotReady（warning）

**健康评分 (0-100)：** no lease(-15)、very stale(-12)、stale(-6)、NotReady(-8)

---

### 68. K8s 可扩展性瓶颈预测器 (v15.53)

**路径：** `GET /api/scalability/bottleneck-predictor`

比较实际使用量与 K8s 推荐限制，预测哪种资源将首先成为集群瓶颈。

**每资源类型分析：**

| 资源 | K8s 限制 |
|------|---------|
| Max pods per node | 110 |
| Total cluster pods | 150,000 |
| Total services | 5,000 |
| Services per node | 20 (kube-proxy) |
| Total nodes | 5,000 |
| Namespaces | 10,000 |

**状态分级：** healthy (<50%)、warning (>50%)、critical (>70%)、bottleneck (>90%)

**输出：** 主要瓶颈类型 + 比率、风险评分（0-100，越高越安全）

---

### 69. 部署镜像漂移与版本一致性检测 (v15.54)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/deployment/image-drift` | 检测镜像版本漂移 |
| GET | `/api/deployment/image-drift?namespace=xxx` | 按命名空间过滤 |

检测同一工作负载内的 Pod 运行不同镜像版本——在滚动更新停滞或手动编辑时发生。

**每工作负载分析：** 不同镜像版本列表 + Pod 数、漂移检测、:latest 标签检测、摘要存在性

**检测项：** 镜像漂移（high）、:latest 标签（medium）、无摘要（low）

**一致性评分 (0-100)：** drift(-15)、latest(-8)、no digest(-2)

---

### 70. 节点污点与 Pod 容忍度影响分析 (v15.56)

**路径：** `GET /api/product/taint-toleration`

分析节点污点和 Pod 容忍度，用于维护计划和节点池隔离。

**每节点分析：** 污点列表（NoSchedule/NoExecute/PreferNoSchedule）、cordon 状态、风险级别

**集群级污点摘要：** 每种唯一污点 → 受影响节点数

**检测项：** NoExecute（critical，正在驱逐 Pod）、cordoned 节点（warning）、NoSchedule（阻塞调度）、宽泛容忍 key=Exists（warning，可在 master 节点运行）

**影响评分 (0-100)：** NoExecute(-15)、cordoned(-8)、NoSchedule(-5)、broad tol(-3)

---

### 71. 控制平面健康检查 (v15.57)

**路径：** `GET /api/operations/control-plane`

通过检查 kube-system 中的控制平面组件 Pod，验证集群核心组件健康状态。

**检查组件：** kube-apiserver、kube-scheduler、kube-controller-manager、etcd

**每组件分析：** 就绪状态、重启次数、运行时长、kubelet 版本、风险级别

**检测项：** 组件不就绪（critical）、重启 ≥5 次（high）、重启 ≥3 次（medium）、运行 <1h（medium）、缺失 etcd/apiserver（critical）

**k3s/microk8s/kind 支持：** 无控制平面 Pod 时报告 info

**健康评分 (0-100)：** unhealthy(-20)、restarts(-5)、missing etcd(-20)、missing apiserver(-20)

---

### 72. 命名空间隔离与多租户审计 (v15.59)

**路径：** `GET /api/scalability/namespace-isolation`

审计命名空间隔离控制，确保多租户集群安全。

**每命名空间分析：** NetworkPolicy 存在性、ResourceQuota 存在性、LimitRange 存在性、PSA 标签

**隔离评分 (0-100)**

---

### 73. 部署版本历史与回滚就绪分析 (v15.60)

**路径：** `GET /api/deployment/revision-history?namespace=xxx`

**每部署分析：** revisionHistoryLimit、ReplicaSet 数量、最旧 RS 年龄

**检测项：** rhl=0（critical）、rhl<5（warning）、>10 ReplicaSets（info）

**回滚就绪评分 (0-100)**

---

### 74. ConfigMap/Secret 大小与内存压力审计 (v15.61)

**路径：** `GET /api/product/configmap-size?namespace=xxx`

**每资源分析：** 大小(KB)、键数量、最大键、是否被挂载

**检测项：** >1MB ConfigMap（warning）、>1MB Secret（warning）

**健康评分 (0-100)**

---

### 75. Pod 驱逐与节点压力历史追踪 (v15.63)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/pod-evictions` | Pod 驱逐历史追踪 |
| GET | `/api/operations/pod-evictions?namespace=xxx` | 按命名空间过滤 |

追踪被 kubelet 驱逐的 Pod，按原因分类（内存/磁盘/PID/未知），帮助识别资源不足的节点。

**每 Pod 分析：** 驱逐原因（memory/disk/pid/unknown）、驱逐时间、所在节点、驱逐消息

**每节点分析：** 总驱逐数、按原因分类、风险级别

**检测项：** 节点驱逐 ≥5（warning）、24h 内 ≥3 次驱逐（warning）

**健康评分 (0-100)：** recent(-8)、memory(-3)、disk(-3)、pid(-2)

---

### 76. API Server 审计日志配置检查 (v15.65)

**路径：** `GET /api/security/audit-policy`

验证 Kubernetes 审计日志是否正确配置——这是 PCI-DSS、SOC2、HIPAA 等合规框架的必要要求。

**检查项：** 审计是否启用、日志后端（file/webhook/both/none）、策略文件、保留天数、备份数量、敏感资源覆盖

**检测项：** 审计未启用（critical）、无策略文件（warning）、保留 <90 天（warning）

**合规评分 (0-100)：** enabled(+40)、policy(+25)、sensitive(+15)、retention(+10)、backup(+5)、both(+5)

---

### 77. CSI 驱动与存储能力审计 (v15.67)

**路径：** `GET /api/scalability/csi-audit`

审计存储类和 CSI 驱动的能力配置。

**每 StorageClass 分析：** provisioner、是否默认、绑定模式（Immediate/WaitForFirstConsumer）、是否支持卷扩展、回收策略（Delete/Retain）

**每 CSIDriver 分析：** attach required、podInfoOnMount、fsGroup policy

**检测项：** 无默认 SC（warning）、多个默认 SC（warning）、缺失 CSI 驱动（warning）、不支持扩展（info）、Delete 回收策略（info）

**健康评分 (0-100)：** no default(-15)、multiple default(-10)、no CSI driver(-20)、non-expandable(-2/SC)

**合规评分 (0-100)：** enabled(+40)、policy(+25)、sensitive(+15)、retention(+10)、backup(+5)、both(+5)

---

### 78. 部署中断与维护影响分析 (v15.69)

**路径：** `GET /api/deployment/disruption-impact`

分析 Deployment/StatefulSet 与 PodDisruptionBudget 的交互，预测哪些工作负载会阻塞节点维护。

**每工作负载分析：** PDB 存在性、minAvailable/maxUnavailable、可驱逐 Pod 数、是否阻塞 drain

**检测项：** PDB 阻塞所有驱逐（critical）、无 PDB（warning）、危险 PDB（warning）

**维护就绪评分 (0-100)：** block drain(-15)、risky PDB(-5)、no PDB(-3)

---

### 79. 批处理 Job 执行健康分析 (v15.70)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/product/job-health` | 批处理 Job 执行健康 |
| GET | `/api/product/job-health?namespace=xxx` | 按命名空间过滤 |

分析 Kubernetes Batch Job 的执行状态。

**每 Job 分析：** 状态、运行时长、完成数、成功/失败数、backoffLimit、父 CronJob

**检测项：** Job 失败（warning）、运行 >24h（warning）、已暂停（info）、无 backoffLimit（info）

**健康评分 (0-100)：** failed(-10)、longRunning(-8)、noBackoff(-2)

---

### 80. API 服务器响应速度与 Pod 启动延迟监控 (v15.72)

**路径：** `GET /api/operations/api-latency`

监控 Kubernetes API 服务器的响应能力以及 Pod 的启动速度。

**监控项：** API 响应性、等待 >2 分钟的 Pod（调度慢）、运行中不就绪的 Pod、容器启动延迟 >1 分钟

**每 Pod 分析：** 挂起时间、容器启动延迟、风险级别

**检测项：** API 无响应（critical）、调度慢 >5min（warning）、容器启动慢（info）

**响应评分 (0-100)：** slow start(-8)、not ready(-5)、container wait(-3)

---

### 81. Secret 静态加密配置检查 (v15.74)

**路径：** `GET /api/security/encryption-at-rest`

验证 Kubernetes Secret 是否在 etcd 中被加密。检查 kube-apiserver 是否配置了 --encryption-provider-config。检测 k3s 环境。

**检查项：** 加密是否启用、加密类型（aescbc/aesgcm/secretbox/none）、提供者数量、identity provider（plaintext fallback）

**检测项：** 加密未启用（critical）、有 identity provider（warning）

**安全评分 (0-100)：** enabled(+60)、no identity(+15)、provider(+15)、config(+10)

---

### 82. 集群扩展性与阈值监控 (v15.75)

**路径：** `GET /api/scalability/scale-limits`

检查集群与 Kubernetes 官方扩展限制的接近程度。

**检查的 K8s 官方限制：** Nodes(5000)、Pods(150000)、Pods/node(110)、Services(5000)、Namespaces(10000)

**状态阈值：** safe(<60%)、warning(60-80%)、critical(≥80%)

**扩展评分 (0-100)：** critical(-20)、warning(-10)

---

### 83. HPA 健康与缩放活动分析 (v15.77)

**路径：** `GET /api/product/hpa-health`

分析 HorizontalPodAutoscaler 健康状态和缩放活动。

**每 HPA 分析：** 最小/最大副本数、当前/期望副本、缩放活跃状态、指标数量、条件状态

**检测项：** 达到最大副本(warning)、无指标(warning)、缩放未激活(info)

**健康评分 (0-100)：** no metrics(-15)、at max(-5)

---

### 84. 工作负载成熟度与最佳实践评分 (v15.79)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/deployment/workload-maturity` | 工作负载成熟度评分 |

综合评分每个 Deployment 是否符合 K8s 最佳实践。

**8 项检查（权重总和=100）：**

| 检查项 | 权重 |
|--------|------|
| 资源请求 | 15 |
| 探针 | 15 |
| 多副本 | 15 |
| 反亲和性 | 15 |
| PDB | 10 |
| 安全上下文 | 10 |
| 版本历史 | 10 |
| 标签 | 10 |

**每工作负载：** 成熟度评分 (0-100)、风险级别

**集群级：** 平均成熟度评分

---

### 85. 卷挂载与附加错误追踪 (v15.81)

**路径：**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/volume-mount-errors` | 卷挂载与附加错误追踪 |

追踪因卷挂载/附加失败而卡在 Pending/ContainerCreating 状态的 Pod。

**每 Pod 分析：** 错误类型（mount_fail/attach_fail/provisioning/timeout）、错误消息、卡住时长、风险级别

**错误分类统计：** mount failures、attach/detach failures、provisioning errors、timeouts

**健康评分 (0-100)：** stuck(-10)、mount fail(-3)、attach fail(-3)、provisioning(-5)

### 88. GET /api/deployment/ephemeral-storage — 容器临时存储与 emptyDir 限制合规

检查容器的临时存储（ephemeral-storage）限制和 emptyDir 卷配置合规性。

**每 Pod 分析：** ephemeral-storage 限制是否存在、emptyDir 卷数量及大小限制、无限制 emptyDir 检测

**违规检测：** 无 ephemeral-storage 限制、无大小限制的 emptyDir（无限制消耗节点磁盘）

**合规评分 (0-100)：** 无限制(-10/per pod)、无 emptyDir limit(-5/per volume)

---

### 89. GET /api/operations/pod-startup — Pod 启动生命周期与瓶颈分析

分析 Pod 从创建到就绪的完整启动生命周期，识别启动瓶颈。

**启动阶段分解：** 调度延迟、init 容器耗时、镜像拉取/容器创建耗时、就绪探针延迟

**慢启动 Pod (>120s)：** 总启动时间、各阶段耗时明细、风险级别 (medium/high)

**卡住 Pod：** Pending/ContainerCreating 状态、等待原因、消息、卡住时长

**瓶颈分类：** scheduling (>30s)、image_pull (>60s)、init_container (>30s)、probe (>60s)、volume

**按工作负载统计：** 每种工作负载类型（Deployment/StatefulSet/DaemonSet/Job 等）的平均/最大启动时间、init 容器比例

**健康评分 (0-100)：** stuck(-5/per pod)、slow(-3/per pod)、avg>60s(-)、failed(-3/per pod)

---

### 91. GET /api/security/psa-audit — Pod Security Admission 强制执行审计

审计命名空间级别的 Pod Security Admission (PSA) 强制执行配置。

**每命名空间分析：** enforce 级别 (privileged/baseline/restricted/none)、audit 级别、warn 级别、版本绑定

**违规检测：**
- Baseline 违规：privileged、hostNetwork/PID/IPC、hostPath、危险 capabilities、hostPort
- Restricted 违规：以 root 运行、特权升级、未丢弃 capabilities、缺少 seccomp

**风险评级：** critical (无强制执行)、high (privileged)、medium (baseline 有违规)、low (restricted 干净)

**强制执行评分 (0-100)：** enforced(+40)、restricted(+25)、baseline(+10)、audit(+10)、warn(+5)、violation(-10)、privileged(-10)

---

### 92. GET /api/product/qos-priority — Pod QoS 与 PriorityClass 分布审计

分析 Pod Quality of Service (QoS) 级别分布和 PriorityClass 使用情况。

**QoS 分布：** Guaranteed（请求=限制）、Burstable（有请求无限制）、BestEffort（无请求无限制）

**PriorityClass 分析：** system-critical (>=2000000000)、high (>=1000000)、medium (>=1000)、low (<1000)

**配置错误检测：**
- 用户命名空间中的 BestEffort Pod（高驱逐风险）
- 单副本 Deployment 无 PriorityClass（可能被抢占）
- Guaranteed QoS 配合低优先级（资源浪费）
- 无资源请求的 Pod

**驱逐风险分析：** BestEffort > Burstable > Guaranteed（节点压力下驱逐顺序）

**PriorityClass 清单：** 名称、值、是否全局默认、抢占策略、关联 Pod 数

**健康评分 (0-100)：** BestEffort 比例(-30)、无 PriorityClass(-15)、无请求(-20)、无限制(-15)、配置错误(-10)

---

### 93. GET /api/scalability/fragmentation — 资源碎片化与装箱效率分析

分析集群节点的资源碎片化和 bin-packing（装箱）效率。

**每节点分析：** 可分配 CPU/内存/Pod 槽位、已请求量、可用量、效率比率（请求/可分配 %）

**碎片化检测：**
- Pod 槽位压力：有资源但无 Pod 槽位（max-pods 限制）
- 资源不平衡：CPU 可用但内存不足，或反之
- 碎片化评分 (0-100)，越高表示越碎片化

**滞留资源检测：** 可用但无法调度的 CPU 和内存（因 Pod 限制或资源不平衡）

**Pod 大小模拟：** 测试标准 Pod 大小（small 100m/128Mi、medium 500m/512Mi、large 1c/2Gi、xlarge 4c/8Gi）在集群中的可调度性

**评分：**
- Bin-packing 评分 (0-100)：100 = 最优资源利用率
- 碎片化评分 (0-100)：100 = 无碎片化

---

### 95. GET /api/deployment/config-sync — ConfigMap/Secret 配置同步与陈旧检测

检测 ConfigMap/Secret 更新后仍在使用旧配置的 Pod。

**引用方式分析：**
- env/envFrom 引用：ConfigMap/Secret 更改后**不自动更新**
- volume 挂载：ConfigMap/Secret 更改后**自动更新**（Kubelet 定期刷新）
- subPath 挂载：**不自动更新**（即使作为 volume 挂载）

**陈旧 Pod 检测：** 交叉比对 Pod 启动时间与 ConfigMap/Secret 创建时间

**Reloader 缺失检测：** 使用 env var 引用但缺少 Reloader 注解的工作负载

**不可变配置检测：** 检查 immutable: true 的 ConfigMap/Secret

**陈旧评分 (0-100)：** 陈旧 Pod(-35)、env 引用比例(-20)、subPath(-15)、缺少 Reloader(-20)

---

### 96. GET /api/operations/kubelet-health — Kubelet 与容器运行时健康监控

监控所有节点的 kubelet 和容器运行时健康状况。

**每节点分析：** kubelet 版本、运行时版本、OS 镜像、内核版本、架构

**心跳检测：** 最后心跳时间与新旧度（>60s 警告、>120s 高危、>300s 严重）

**条件追踪：** NotReady、DiskPressure、MemoryPressure、PIDPressure、NetworkUnavailable

**版本偏差检测：** 不同 kubelet 版本（major.minor 级别差异）

**运行时偏差检测：** 不同容器运行时版本

**分布统计：** 运行时类型分布（containerd/docker/cri-o）、OS 镜像分布

**健康评分 (0-100)：** 不健康节点(-50)、版本偏差(-15)、运行时偏差(-10)、心跳陈旧(-15)

---

### 98. GET /api/security/mac-audit — AppArmor 与 SELinux 强制访问控制审计

审计 Pod 的 AppArmor 和 SELinux 强制访问控制（MAC）配置。

**AppArmor 审计：** 检测 unconfined profile、缺失 profile 的用户命名空间 Pod

**SELinux 审计：** 检测 permissive/unconfined 类型、缺失 SELinux 上下文的 Pod

**节点能力检测：** 检查节点是否支持 AppArmor/SELinux

**合规评分 (0-100)：** AppArmor 覆盖率(+40)、SELinux 覆盖率(+20)、unconfined(-20)、缺失(-25)

---

### 99. GET /api/product/service-connectivity — 服务端点与连通性健康审计

审计 Service 端点健康和连通性。

**检测项：**
- 零端点服务（无后端 Pod）
- 无就绪端点（Pod 存在但未就绪）
- 选择器间隙（selector 不匹配任何 Pod）

**类型分布：** ClusterIP、NodePort、LoadBalancer、Headless、ExternalName

**每命名空间健康统计**

**健康评分 (0-100)**

---

### 100. GET /api/scalability/ip-cidr-utilization — IP 地址与 Pod CIDR 利用率监控

分析集群节点的 IP 地址和 Pod CIDR 利用率。

**每节点分析：** Pod CIDR、CIDR 容量（地址数）、已用 Pod IP 数、利用率百分比、剩余容量

**容量预警：** 节点接近耗尽（>80%）或已耗尽（100%）

**双栈检测：** IPv4/IPv6 双栈节点识别

**集群范围：** 总体 IP 利用率、服务 IP 范围检测

**健康评分 (0-100)：** 利用率>80%(-)、耗尽节点(-10/per)、接近耗尽(-5/per)

---

### 101. GET /api/deployment/sidecar-audit — Sidecar 容器开销与注入审计

审计 Pod 中的 sidecar 容器及其资源开销。

**Sidecar 识别：** Istio Proxy、Linkerd、Vault Agent、Fluentd、Datadog 等已知模式

**资源开销分析：** 每 Pod 和每命名空间的 sidecar CPU/内存请求占比

**高开销检测：** sidecar 资源占比 >30% 的 Pod

**注入方式检测：** 自动注入（Istio/Linkerd 注解）vs 手动添加

**异常检测：** 所有容器都是 sidecar 的 Pod（无应用容器）

**健康评分 (0-100)：** CPU 开销>30%(-25)、内存开销>30%(-20)、仅注入(-15)

---

### 102. GET /api/operations/dns-health — DNS 解析健康与 CoreDNS 监控

监控集群 DNS 解析健康和 CoreDNS 性能。

**CoreDNS Pod 状态：** 就绪状态、版本、重启次数

**CoreDNS ConfigMap 分析：** cache、health、ready、prometheus 插件缺失检测

**Pod DNS 策略检测：** Default（继承节点）vs ClusterFirst（集群内解析）

**每命名空间 DNS 策略统计**

**健康评分 (0-100)：** 无 CoreDNS(0)、CoreDNS 未就绪(-40)、无 ConfigMap(-20)、错误策略(-20)

---

### 103. GET /api/security/forensics — Pod 安全取证与事件证据收集

收集 Pod 安全取证信息和事件证据。

**容器退出码分析：** OOMKilled(137)、SIGKILL(137)、SIGTERM(143)、SIGSEGV(139) 等，附带人类可读解释

**检测项：**
- OOMKill 检测与统计
- SIGKILL 终止追踪
- 特权容器逃逸检测（特权容器 + 终止历史）
- 容器/镜像哈希不匹配检测（可能的镜像篡改）
- 高重启次数 Pod 标记

**最近终止记录：** 容器名、退出码、原因、信号、完成时间、运行时长

**取证评分 (0-100)：** OOMKill(-5/per)、SIGKILL(-3/per)、退出错误(-4/per)、特权逃逸(-10/per)、哈希不匹配(-3/per)

---

### 104. GET /api/product/topology-spread — Pod 拓扑分布约束验证

验证工作负载的拓扑分布约束配置和实际 Pod 分布情况。

**覆盖工作负载：** Deployment、StatefulSet、DaemonSet

**验证项：**
- 检测多副本工作负载缺少分布约束
- 验证 maxSkew、topologyKey、whenUnsatisfiable 设置
- ScheduleAnyway vs DoNotSchedule 策略检查

**实际分布分析：** 按 topology.kubernetes.io/zone 和 kubernetes.io/hostname 域计算 Pod 分布和域偏差

**健康评分 (0-100)：** 无约束比例(-40)、违规数量(-5/per)

---

### 105. GET /api/deployment/restart-policy — 容器重启策略与生命周期钩子审计

审计容器的重启策略和生命周期钩子配置。

**重启策略验证：**
- Deployment/StatefulSet/DaemonSet 应使用 `Always`
- Job/CronJob 应使用 `OnFailure` 或 `Never`
- 检测策略与工作负载类型不匹配

**生命周期钩子：**
- postStart 钩子覆盖率
- preStop 钩子覆盖率（影响滚动更新时的优雅退出）

**每命名空间统计**

**健康评分 (0-100)：** 策略不匹配(-10/per)、无生命周期钩子(-20%)

---

### 106. GET /api/operations/csr-monitor — 证书签名请求与节点引导证书监控

监控 Kubernetes 集群中的 Certificate Signing Requests (CSR)。

**状态追踪：** Pending、Approved、Denied、Expired

**陈旧 CSR 检测：** Pending 超过 1 小时的 CSR（可能阻塞节点引导或服务 TLS）

**统计：** 按 signerName 分组、按 requester 分组

**健康评分 (0-100)：** Pending(-10/per)、陈旧 Pending(-20/per)、大量 Denied(-)

---

### 107. GET /api/scalability/node-topology — 节点拓扑分布与多可用区容错分析

分析集群节点跨可用区和区域的分布情况。

**每可用区统计：** 节点数、可分配 CPU/内存、Pod 数、CPU 占比

**风险检测：**
- 单可用区集群（关键风险，无区域故障容错）
- 可用区不平衡检测
- 缺少区域标签的节点检测
- 单区域集群检测

**健康评分 (0-100)：** 单可用区(-40)、无区域标签(-5/per)、不平衡(-)、单区域(-5)

---

### 108. GET /api/security/rbac-audit — RBAC 权限过大与通配符审计

审计 RBAC 角色和绑定的权限过大问题。

**检测项：**
- 通配符动词（`*`）— 授予所有操作权限
- 通配符资源（`*`）— 授予所有资源访问权限
- 非系统 cluster-admin 绑定
- 指向通配符角色的 RoleBinding

**角色分级：** critical（通配符动词+资源）、high（单一通配符）、medium

**系统角色过滤：** 自动排除 system:*、cluster-admin、admin、edit、view 等内置角色

**健康评分 (0-100)：** cluster-admin 绑定(-15/per)、通配符动词(-5/per)、通配符资源(-4/per)

---

### 109. GET /api/product/backup-compliance — 卷快照与 PVC 备份合规审计

审计 PVC 备份和快照的合规性。

**检测项：**
- 正在使用但缺少备份保护的 PVC
- 关键大型 PVC（>=1Gi）无备份告警
- Velero 安装状态检查

**每命名空间和每存储类别合规统计**

**健康评分 (0-100)：** 未保护比例(-50%)、关键未保护(-5/per)、无 Velero(-10)

---

### 110. GET /api/deployment/scale-readiness — 部署扩缩容就绪与自动伸缩差距检测

分析 Deployment 和 StatefulSet 的扩缩容就绪状态。

**检测项：**
- 多副本工作负载缺少 HPA（无法基于负载自动伸缩）
- 多副本工作负载缺少 PDB（自愿中断可能导致停机）
- 缺少资源请求（无法安全自动伸缩）
- 单副本工作负载（无高可用）

**完全就绪识别：** 同时拥有 HPA + PDB + 资源请求的工作负载

**健康评分 (0-100)：** 无资源(-10/per)、无 HPA(-25%)、无 PDB(-25%)、单副本(-3/per)

---

### 111. GET /api/operations/etcd-health — etcd 健康与数据库压力监控

监控 etcd Pod 健康和数据库压力。

**etcd Pod 状态：** 就绪状态、版本、重启次数

**大型对象检测：** >100KB（中等）、>500KB（高）、>1MB（严重）的 ConfigMap 和 Secret

**单实例检测：** 仅 1 个 etcd 实例（无 HA 仲裁）

**压力评分 (0-100)：** 大型对象数量(-5/per)

**健康评分 (0-100)：** 未就绪(-50%)、单实例(-20)、大型对象(-3/per)

---

### 86. GET /api/security/host-namespace — 容器主机命名空间与特权暴露审计

审计容器的宿主机命名空间暴露和特权升级风险。

**每 Pod 分析：** hostNetwork、hostPID、hostIPC、privileged、hostPath、capAdd、runAsRoot

**风险级别：** critical (privileged+hostNS)、high (privileged/hostNS)、medium (少量暴露)

**暴露安全评分 (0-100，越高越安全)：** privileged(-10)、hostNetwork(-5)、hostPID(-5)、hostIPC(-3)、hostPath(-3)、capAdd(-3)、runAsRoot(-2)

### 87. GET /api/product/api-deprecation — 已废弃 API 版本与升级就绪检查

通过 API discovery 检测集群中仍在使用的已废弃/已移除的 K8s API 版本。

**覆盖范围：** extensions/v1beta1、apps/v1beta1/v1beta2、networking v1beta1、batch v1beta1、autoscaling v2beta1/v2beta2、policy/v1beta1 (PSP)、storage v1beta1 — 共 18 种

**升级就绪评分 (0-100)：** removed(-30)、deprecated(-15)

---

### 88. GET /api/product/init-container-audit — Init Container 可靠性与启动依赖审计

审计 Init Container 的可靠性和启动依赖。

**检测项：**
- 缺少资源请求（CPU/内存）的 init container
- 缺少资源限制的 init container
- 过多 init container（>5 个，增加启动延迟和故障面）
- RestartPolicy=Always 的 init container（sidecar 行为，可能延迟启动）

**按命名空间和工作负载分组分析**

**健康评分 (0-100)：** 缺少请求(-10/per)、缺少限制(-3/per)、过多重试(-2/per)、高风险(-8/per)

---

### 89. GET /api/deployment/replica-availability — 部署副本可用性与 Ready Pod 比率监控

监控 Deployments、StatefulSets、DaemonSets 的副本可用性。

**检测项：**
- Ready/desired 副本差距
- 零 Ready 工作负载（完全不可用）
- Rollout 中的陈旧副本（updatedReplicas < desiredReplicas）
- 按命名空间分组分析

**健康评分 (0-100)：** 可用率×100 - 零Ready×10 - 差距×3

---

### 90. GET /api/scalability/tenant-pressure — 多租户资源压力与 Quota 竞争审计

审计多租户场景下的资源压力和 Quota 竞争。

**检测项：**
- Quota 饱和（>80% 利用率）
- 临界 Quota（>95% 利用率）
- 无界命名空间（无 ResourceQuota + 无 LimitRange）
- 资源热点（命名空间消耗 >30%/50% 集群容量）

**按命名空间风险分级：** critical / high / medium / low
**健康评分 (0-100)：** 100 - 无界×10 - 临界×8 - 饱和×4 - 无LimitRange×2

---

### 91. GET /api/operations/api-load — API Server 请求吞吐与负载压力监控

通过 Pod 密度、Controller 数量、Event 体量分析 API Server 负载。

**检测项：**
- 密集命名空间（>100 pods，高 API watch 压力）
- 高活跃命名空间（>80 activity score 或 >10 warnings）
- 空命名空间（浪费 API watch 资源）
- Warning event 比率分析

**健康评分 (0-100)：** 100 - 密集×10 - 高活跃×5 - 空NS×2 - warning比率惩罚

---

### 92. GET /api/product/external-secret-health — External Secrets 与 Secret Store CSI 健康审计 (v16.44)

审计 External Secrets Operator 和 Secret Store CSI 驱动健康状态。

**检测项：**
- External Secrets Operator 安装状态
- ExternalSecret CRD 同步状态（NotSynced/Error/循环检测）
- SecretStore/ClusterSecretStore 配置验证
- Secret Store CSI 驱动安装检测
- 过期/停滞同步检测

**健康评分 (0-100)，5 个单元测试**

---

### 93. GET /api/deployment/progressive-delivery — 渐进式交付与金丝雀发布健康审计 (v16.45)

审计 Argo Rollouts / Flagger 渐进式交付健康状态。

**检测项：**
- Argo Rollouts / Flagger CRD 检测（dynamic client）
- 金丝雀/蓝绿/滚动发布策略分析
- 流量权重配置验证
- 分析步骤配置验证
- 卡住的发布检测（长时间无进展）

**健康评分 (0-100)，4 个单元测试**

---

### 94. GET /api/operations/metrics-pipeline — Metrics 管道与 kube-state-metrics 健康审计 (v16.46, 盲区5: 观测性深化)

审计 Kubernetes metrics 管道完整性。

**检测项：**
- metrics-server 安装检测与可用性
- kube-state-metrics 部署健康
- Prometheus 指标管道完整性验证
- 缺失 metrics 源检测（API server、kubelet、容器）

**健康评分 (0-100)，4 个单元测试**

---

### 95. GET /api/security/pss-scorecard — Pod Security Standards 合规评分卡 (v16.47, 盲区2: 合规深化)

按 Pod Security Standards 三个级别审计集群合规状态。

**检测项：**
- Privileged/Baseline/Restricted 三个级别合规审计
- 按 namespace 检查 PSA 模式绑定
- 违规容器检测（privileged、hostPath、hostPort、capabilities 等）
- 维度评分汇总，namespace 合规率

**健康评分 (0-100)，4 个单元测试**

---

### 96. GET /api/scalability/hpa-performance — HPA 自动伸缩性能与扩缩容事件审计 (v16.48)

分析 HPA 自动伸缩性能和扩缩容事件历史。

**检测项：**
- 扩缩容事件分析（基于 Replica 变化历史）
- 伸缩震荡检测（频繁扩缩缩循环）
- 缩容延迟分析（长时间未缩容）
- 当前与目标利用率差距

**健康评分 (0-100)，3 个单元测试**

---

### 97. GET /api/product/endpoint-dns-health — 服务端点与 DNS 解析健康审计 (v16.49)

审计服务端点健康和 DNS 解析状态。

**检测项：**
- Service 后端 Pod 就绪状态检查
- Endpoints 空服务检测（无后端 Pod）
- CoreDNS 配置验证与 Pod 健康
- ExternalName 服务解析验证

**健康评分 (0-100)，3 个单元测试**

---

### 98. GET /api/deployment/rs-staleness — ReplicaSet 陈旧度与滚动发布历史审计 (v16.50)

审计 ReplicaSet 陈旧度和滚动发布历史深度。

**检测项：**
- 陈旧 ReplicaSet 检测（非当前 RS 保留过多）
- revisionHistoryLimit 配置审计
- RS 年龄分析、孤立 RS 检测
- 滚动发布历史深度评估

**健康评分 (0-100)，4 个单元测试**

---

### 99. GET /api/operations/audit-log-health — 审计日志管道与事件导出健康审计 (v16.51, 盲区5: 观测性深化)

审计 Kubernetes 审计日志管道和事件导出健康状态。

**检测项：**
- 审计日志配置检测（audit-policy.json）
- 日志后端验证（backend: logFile/logBatch）
- 事件导出管道健康（Fluentd/Fluent Bit/Loki 检测）
- Warning 事件积压检测

**健康评分 (0-100)，4 个单元测试**

---

### 100. GET /api/security/sa-token-audit — ServiceAccount Token 轮换与访问风险审计 (v16.52)

审计 ServiceAccount Token 轮换和访问风险。

**检测项：**
- 长期未轮换 Secret token 检测（>90天）
- 无 Secret 引用的 SA 检测
- automountServiceAccountToken 配置审计
- 高权限 SA 检测（cluster-admin 绑定）

**健康评分 (0-100)，5 个单元测试**

---

### 101. GET /api/scalability/pv-reclaim — PV 回收策略与存储类浪费审计 (v16.53)

审计 PersistentVolume 回收策略和存储类浪费。

**检测项：**
- 回收策略审计（Retain/Recycle/Delete）
- Released 状态 PV 检测（可回收空间）
- Failed 状态 PV 检测
- 存储类绑定分析、默认存储类审计

**健康评分 (0-100)，3 个单元测试**

---

### 102. GET /api/product/config-mount-risk — ConfigMap 与 Secret 挂载注入风险审计 (v16.55)

审计 ConfigMap 和 Secret 挂载注入风险。

**检测项：**
- 缺失 ConfigMap 引用检测
- 大型 ConfigMap 检测（>500KB）
- 非可选挂载检测（optional: false）
- subPath 挂载检测（阻止热更新）
- envFrom Secret 注入检测

**按 namespace 统计，健康评分 (0-100)，3 个单元测试**

---

## API 端点总览

| # | 端点 | 维度 | 版本 | 说明 |
|---|------|------|------|------|
| 1 | /api/health | - | v1.0 | 健康检查 |
| 2 | /api/version | - | v1.0 | 版本信息 |
| 3 | /api/cluster/overview | Product | v1.0 | 集群概览 |
| 4 | /api/cluster/nodes | Product | v1.0 | 节点列表 |
| 5 | /api/cluster/pods | Product | v1.0 | Pod 列表 |
| 6 | /api/chat/stream | Product | v1.0 | AI 聊天 |
| ... | ... | ... | ... | ... |
| 36 | /api/product/config-audit | Product | v15.14 | ConfigMap & Secret 配置审计 |
| 37 | /api/deployment/image-hygiene | Deployment | v15.13 | 容器镜像部署规范分析 |
| 38 | /api/deployment/rollout-health | Deployment | v15.19 | 滚动更新策略与健康分析 |
| 39 | /api/security/cert-expiry | Security | v15.16 | 证书与 TLS 过期监控 |
| 40 | /api/operations/pdb-audit | Operations | v15.17 | PDB 合规与中断风险 |
| 41 | /api/scalability/ns-consumption | Scalability | v15.18 | 命名空间资源消耗与成本 |
| 42 | /api/product/network-policy | Product | v15.20 | Network Policy 合规审计 |
| 43 | /api/security/volume-mounts | Security | v15.22 | 卷安全与挂载风险审计 |
| 44 | /api/operations/topology-distribution | Operations | v15.23 | 拓扑分布与 Pod 分配审计 |
| 45 | /api/scalability/capacity-headroom | Scalability | v15.24 | 集群容量余量与扩容就绪 |
| 46 | /api/deployment/probe-compliance | Deployment | v15.25 | 健康探针合规审计 |
| 47 | /api/product/label-hygiene | Product | v15.26 | 标签与注解卫生审计 |
| 48 | /api/security/endpoint-exposure | Security | v15.28 | 服务端点暴露与攻击面审计 |
| 49 | /api/operations/image-pull-failures | Operations | v15.29 | ImagePullBackOff 与启动失败追踪 |
| 50 | /api/scalability/quota-utilization | Scalability | v15.30 | 资源配额使用率与限制合规 |
| 51 | /api/deployment/resource-limits | Deployment | v15.32 | 资源限制与强制差距审计 |
| 52 | /api/product/orphaned-resources | Product | v15.33 | 孤立资源检测器 |
| 53 | /api/security/seccomp-audit | Security | v15.34 | Seccomp 与 PSS 合规审计 |
| 54 | /api/operations/restart-reasons | Operations | v15.35 | Pod 重启原因分析器 |
| 55 | /api/scalability/ha-audit | Scalability | v15.36 | 高可用与单点故障检测器 |
| 56 | /api/deployment/graceful-shutdown | Deployment | v15.38 | 优雅终止与终止合规审计 |
| 57 | /api/product/pvc-health | Product | v15.39 | PV/PVC 存储健康与容量审计 |
| 58 | /api/security/batch-audit | Security | v15.40 | CronJob 与批处理作业安全审计 |
| 59 | /api/operations/scheduling-latency | Operations | v15.41 | Pod 调度延迟分析器 |
| 60 | /api/scalability/node-failure-sim | Scalability | v15.42 | 节点故障影响模拟器 |
| 61 | /api/deployment/update-strategy | Deployment | v15.44 | 部署更新策略与回滚就绪审计 |
| 62 | /api/product/statefulset-audit | Product | v15.45 | StatefulSet 健康与有序滚动更新审计 |
| 63 | /api/operations/resource-contention | Operations | v15.46 | 资源争用与限流检测器 |
| 64 | /api/scalability/crd-explosion | Scalability | v15.48 | API 对象计数与 CRD 爆炸风险检测器 |
| 65 | /api/deployment/ref-integrity | Deployment | v15.49 | Secret/ConfigMap 引用完整性检查 |
| 66 | /api/product/affinity-conflict | Product | v15.50 | Pod 亲和性/反亲和性冲突检测 |
| 67 | /api/operations/node-lease | Operations | v15.52 | 节点心跳与健康租约监控 |
| 68 | /api/scalability/bottleneck-predictor | Scalability | v15.53 | K8s 可扩展性瓶颈预测器 |
| 69 | /api/deployment/image-drift | Deployment | v15.54 | 部署镜像漂移与版本一致性检测 |
| 70 | /api/product/taint-toleration | Product | v15.56 | 节点污点与 Pod 容忍度影响分析 |
| 71 | /api/operations/control-plane | Operations | v15.57 | 控制平面健康检查 |
| 72 | /api/scalability/namespace-isolation | Scalability | v15.59 | 命名空间隔离与多租户审计 |
| 73 | /api/deployment/revision-history | Deployment | v15.60 | 部署版本历史与回滚就绪 |
| 74 | /api/product/configmap-size | Product | v15.61 | ConfigMap/Secret 大小与内存压力审计 |
| 75 | /api/operations/pod-evictions | Operations | v15.63 | Pod 驱逐与节点压力历史追踪 |
| 76 | /api/security/audit-policy | Security | v15.65 | API Server 审计日志配置检查 |
| 77 | /api/scalability/csi-audit | Scalability | v15.67 | CSI 驱动与存储能力审计 |
| 78 | /api/deployment/disruption-impact | Deployment | v15.69 | 部署中断与维护影响分析 |
| 79 | /api/product/job-health | Product | v15.70 | 批处理 Job 执行健康分析 |
| 80 | /api/operations/api-latency | Operations | v15.72 | API 服务器响应速度与 Pod 启动延迟监控 |
| 81 | /api/security/encryption-at-rest | Security | v15.74 | Secret 静态加密配置检查 |
| 82 | /api/scalability/scale-limits | Scalability | v15.75 | 集群扩展性与阈值监控 |
| 83 | /api/product/hpa-health | Product | v15.77 | HPA 健康与缩放活动分析 |
| 84 | /api/deployment/workload-maturity | Deployment | v15.79 | 工作负载成熟度与最佳实践评分 |
| 85 | /api/operations/volume-mount-errors | Operations | v15.81 | 卷挂载与附加错误追踪 |
| 86 | /api/security/host-namespace | Security | v15.83 | 容器主机命名空间与特权暴露审计 |
| 87 | /api/product/api-deprecation | Product | v15.84 | 已废弃 API 版本与升级就绪检查 |
| 88 | /api/scalability/dr-readiness | Scalability | v15.86 | 灾难恢复就绪与备份合规审计 |
| 89 | /api/deployment/ephemeral-storage | Deployment | v15.88 | 容器临时存储与 emptyDir 限制合规 |
| 90 | /api/operations/pod-startup | Operations | v15.89 | Pod 启动生命周期与瓶颈分析 |
| 91 | /api/security/psa-audit | Security | v15.91 | Pod Security Admission 强制执行审计 |
| 92 | /api/product/qos-priority | Product | v15.92 | Pod QoS 与 PriorityClass 分布审计 |
| 93 | /api/scalability/fragmentation | Scalability | v15.93 | 资源碎片化与装箱效率分析 |
| 94 | /api/deployment/config-sync | Deployment | v15.95 | ConfigMap/Secret 配置同步与陈旧检测 |
| 95 | /api/operations/kubelet-health | Operations | v15.96 | Kubelet 与容器运行时健康监控 |
| 96 | /api/security/mac-audit | Security | v15.98 | AppArmor 与 SELinux MAC 合规审计 |
| 97 | /api/product/service-connectivity | Product | v15.99 | 服务端点与连通性健康审计 |
| 98 | /api/scalability/ip-cidr-utilization | Scalability | v16.01 | IP 地址与 Pod CIDR 利用率监控 |
| 99 | /api/deployment/sidecar-audit | Deployment | v16.02 | Sidecar 容器开销与注入审计 |
| 100 | /api/operations/dns-health | Operations | v16.03 | DNS 解析健康与 CoreDNS 监控 |
| 101 | /api/security/forensics | Security | v16.05 | Pod 安全取证与事件证据收集 |
| 102 | /api/product/topology-spread | Product | v16.06 | Pod 拓扑分布约束验证 |
| 103 | /api/deployment/restart-policy | Deployment | v16.08 | 容器重启策略与生命周期钩子审计 |
| 104 | /api/operations/csr-monitor | Operations | v16.09 | 证书签名请求与节点引导证书监控 |
| 105 | /api/scalability/node-topology | Scalability | v16.11 | 节点拓扑分布与多可用区容错分析 |
| 106 | /api/security/rbac-audit | Security | v16.12 | RBAC 权限过大与通配符审计 |
| 107 | /api/product/backup-compliance | Product | v16.13 | 卷快照与 PVC 备份合规审计 |
| 108 | /api/deployment/scale-readiness | Deployment | v16.15 | 部署扩缩容就绪与自动伸缩差距检测 |
| 109 | /api/operations/etcd-health | Operations | v16.16 | etcd 健康与数据库压力监控 |
| 110 | /api/security/secret-scan | Security | v16.18 | Secret 数据暴露与凭证泄漏扫描 |
| 111 | /api/product/init-container-audit | Product | v16.19 | Init Container 可靠性与启动依赖审计 |
| 112 | /api/deployment/replica-availability | Deployment | v16.21 | 部署副本可用性与 Ready Pod 比率监控 |
| 113 | /api/scalability/tenant-pressure | Scalability | v16.22 | 多租户资源压力与 Quota 竞争审计 |
| 114 | /api/operations/api-load | Operations | v16.23 | API Server 请求吞吐与负载压力监控 |
| 115 | /api/security/sec-drift | Security | v16.25 | 安全上下文漂移与运行时策略合规审计 |
| 116 | /api/product/hpa-gap | Product | v16.26 | HPA 目标利用率差距与扩缩容行为审计 |
| 117 | /api/scalability/node-pool-health | Scalability | v16.27 | 节点池与集群自动伸缩健康监控 |
| 118 | /api/deployment/helm-health | Deployment | v16.28 | Helm Release 健康与 GitOps 漂移检测 |
| 119 | /api/operations/prom-health | Operations | v16.29 | Prometheus 规则健康与告警覆盖率审计 |
| 120 | /api/security/opa-compliance | Security | v16.30 | OPA/Gatekeeper 策略合规与约束违规审计 |
| 121 | /api/product/mesh-health | Product | v16.31 | Service Mesh Sidecar 健康与 mTLS 覆盖率审计 |
| 122 | /api/scalability/cost-waste | Scalability | v16.32 | 闲置资源成本浪费与命名空间成本分摊审计 |
| 123 | /api/scalability/node-lifecycle | Scalability | v16.34 | 节点 OS 补丁、内核版本漂移、GPU 资源与节点轮换审计 |
| 124 | /api/deployment/surge-risk | Deployment | v16.35 | 滚动更新风险与 Surge 配置分析 |
| 125 | /api/operations/alertmanager-health | Operations | v16.36 | Alertmanager 配置与告警路由健康审计 |
| 126 | /api/security/image-vuln | Security | v16.37 | 容器镜像漏洞与补丁滞后审计 |
| 127 | /api/product/cronjob-schedule | Product | v16.38 | CronJob 调度冲突与资源配置审计 |
| 128 | /api/deployment/startup-latency | Deployment | v16.39 | Pod 启动延迟与就绪性能审计 |
| 129 | /api/operations/grafana-health | Operations | v16.40 | Grafana Dashboard 可用性与数据源健康审计 |
| 130 | /api/security/kyverno-compliance | Security | v16.41 | Kyverno 策略合规与集群策略审计 |
| 131 | /api/scalability/alloc-efficiency | Scalability | v16.42 | 资源请求与限制分配效率审计 |
| 132 | /api/product/external-secret-health | Product | v16.44 | External Secrets 与 Secret Store CSI 健康审计 |
| 133 | /api/deployment/progressive-delivery | Deployment | v16.45 | 渐进式交付与金丝雀发布健康审计 |
| 134 | /api/operations/metrics-pipeline | Operations | v16.46 | Metrics 管道与 kube-state-metrics 健康审计 |
| 135 | /api/security/pss-scorecard | Security | v16.47 | Pod Security Standards 合规评分卡 |
| 136 | /api/scalability/hpa-performance | Scalability | v16.48 | HPA 自动伸缩性能与扩缩容事件审计 |
| 137 | /api/product/endpoint-dns-health | Product | v16.49 | 服务端点与 DNS 解析健康审计 |
| 138 | /api/deployment/rs-staleness | Deployment | v16.50 | ReplicaSet 陈旧度与滚动发布历史审计 |
| 139 | /api/operations/audit-log-health | Operations | v16.51 | 审计日志管道与事件导出健康审计 |
| 140 | /api/security/sa-token-audit | Security | v16.52 | ServiceAccount Token 轮换与访问风险审计 |
| 141 | /api/scalability/pv-reclaim | Scalability | v16.53 | PV 回收策略与存储类浪费审计 |
| 142 | /api/product/config-mount-risk | Product | v16.55 | ConfigMap 与 Secret 挂载注入风险审计 |
| 143 | /api/scalability/capacity-plan | Scalability | v16.65 | 容量规划与增长趋势预测器 |
| 144 | /api/security/quota-security | Security | v16.66 | 资源配额与 LimitRange 安全审计 |
| 145 | /api/product/pv-access | Product | v16.67 | PV 访问模式与多重挂载风险审计 |
| 146 | /api/deployment/dora-metrics | Deployment | v16.68 | DORA 指标：部署频率、前置时间、MTTR、变更失败率 |
| 147 | /api/operations/apf-audit | Operations | v16.69 | API 优先级与公平性配置审计 |
| 148 | /api/scalability/spot-readiness | Scalability | v16.71 | Spot/抢占式实例就绪与成本优化审计 |
| 149 | /api/product/traffic-policy | Product | v16.72 | 服务流量策略与路由配置审计 |
| 150 | /api/deployment/daemonset-audit | Deployment | v16.73 | DaemonSet 滚动发布与节点覆盖审计 |
| 151 | /api/security/policy-drift | Security | v16.74 | 安全策略漂移与基线配置审计 |
| 152 | /api/operations/log-pipeline | Operations | v16.75 | 日志聚合与转发管道健康审计 |
| 153 | /api/product/runtime-class | Product | v16.76 | 容器运行时类与 OCI 镜像合规审计 |
| 154 | /api/deployment/image-pull-audit | Deployment | v16.77 | 镜像拉取策略与密钥管理审计 |
| 155 | /api/scalability/vpa-audit | Scalability | v16.79 | VPA 配置与资源建议质量审计 |
| 156 | /api/product/mesh-traffic | Product | v16.80 | 服务网格流量管理与熔断器健康审计 |
| 157 | /api/deployment/rollout-blocker | Deployment | v16.81 | 部署滚动更新阻塞与 Pod 条件审计 |
| 158 | /api/security/pss-hardening | Security | v16.82 | PSS 强制执行差距与工作负载加固审计 |
| 159 | /api/operations/node-trend | Operations | v16.83 | 节点状况趋势与硬件故障预测 |
| 160 | /api/product/endpoint-slice | Product | v16.84 | Endpoint Slice 健康与拓扑感知路由审计 |
| 161 | /api/deployment/surge-risk | Deployment | v16.85 | 滚动更新风险与 Surge 配置分析器 |
| 162 | /api/scalability/saturation | Scalability | v16.87 | 资源饱和与 CPU/内存节流风险预测 |
| 163 | /api/operations/registry-rate-limit | Operations | v16.88 | 容器镜像仓库限速与拉取可靠性审计 |
| 164 | /api/product/cert-manager | Product | v16.90 | Cert-Manager 健康与证书续期管道审计 |
| 165 | /api/deployment/quota-impact | Deployment | v16.91 | 部署资源配额影响与命名空间容量审计 |
| 166 | /api/security/runtime-threat | Security | v16.92 | 运行时威胁检测与容器异常审计 |
| 167 | /api/operations/cni-health | Operations | v16.93 | CNI 插件健康与网络栈配置审计 |
| 168 | /api/scalability/budget-alert | Scalability | v16.94 | 成本预算告警与命名空间支出限额审计 |
| 169 | /api/product/ingress-tls | Product | v16.95 | Ingress TLS 证书与 HTTPS 强制审计 |
| 170 | /api/deployment/env-config-drift | Deployment | v16.96 | 部署环境配置漂移与 ConfigMap/Secret 引用审计 |
| 171 | /api/operations/observability-stack | Operations | v16.98 | 可观测性栈集成健康审计 |
| 172 | /api/security/secret-posture | Security | v16.99 | 密钥管理态势与外部密钥集成审计 |
| 173 | /api/scalability/node-drain-readiness | Scalability | v17.00 | 节点排空与轮换就绪审计 |
| 174 | /api/product/east-west-traffic | Product | v17.01 | 东西向流量与服务间连通性审计 |
| 175 | /api/deployment/traceability | Deployment | v17.02 | 部署可复现性与 CI/CD 可追溯性审计 |
| 176 | /api/operations/operator-health | Operations | v17.04 | 集群 Operator 与 OLM 健康审计 |
| 177 | /api/security/namespace-posture | Security | v17.05 | 命名空间安全态势与信任边界审计 |
| 178 | /api/scalability/scaling-history | Scalability | v17.06 | 集群扩展历史与自动伸缩事件时间线审计 |
| 179 | /api/product/port-exposure | Product | v17.07 | 容器端口暴露与命名端口一致性审计 |
| 180 | /api/deployment/termination-audit | Deployment | v17.08 | Pod 终止消息与退出码模式审计 |
| 181 | /api/operations/restart-storm | Operations | v17.10 | Pod 重启模式与 CrashLoop 聚类审计 |
| 182 | /api/security/image-provenance | Security | v17.11 | 容器镜像来源与注册表信任审计 |
| 183 | /api/scalability/scheduling-fit | Scalability | v17.12 | Pod 资源请求密度与调度适配审计 |
| 184 | /api/product/endpoint-mismatch | Product | v17.13 | 服务端点与 Pod 就绪状态不匹配审计 |
| 185 | /api/deployment/readiness-gate | Deployment | v17.14 | Pod 就绪门控合规与自定义条件审计 |
| 186 | /api/operations/webhook-health | Operations | v17.16 | 准入 Webhook 配置健康与性能风险审计 |
| 187 | /api/security/threat-timeline | Security | v17.17 | 安全事件时间线与威胁检测模式审计 |
| 188 | /api/scalability/quota-saturation | Scalability | v17.18 | 命名空间资源配额饱和度与限制耗尽预测器 |
| 189 | /api/product/priority-preemption | Product | v17.19 | Pod 优先级抢占与调度饥饿风险分析器 |
| 190 | /api/deployment/concurrency-guard | Deployment | v17.20 | 部署并发防护与滚动更新碰撞检测器 |
| 191 | /api/operations/kube-proxy-health | Operations | v17.22 | Kube-Proxy 健康与网络路由稳定性审计 |
| 192 | /api/security/secret-age | Security | v17.23 | 密钥年龄与过期凭证追踪器 |
| 193 | /api/scalability/ext-resource-health | Scalability | v17.24 | 扩展资源与设备插件健康审计 |
| 194 | /api/product/mesh-injection | Product | v17.25 | 服务网格注入覆盖与命名空间采纳分析器 |
| 195 | /api/deployment/revision-diff | Deployment | v17.26 | 部署修订差异与 Pod 模板变更影响分析器 |
| 196 | /api/operations/coredns-health | Operations | v17.28 | CoreDNS 配置与解析健康审计 |
| 197 | /api/security/blast-radius | Security | v17.29 | 工作负载攻击面与爆炸半径分析器 |
| 198 | /api/scalability/reservation-audit | Scalability | v17.30 | 节点资源预留与可分配差距分析器 |
| 199 | /api/product/replica-distribution | Product | v17.31 | 工作负载副本分布与反亲和覆盖分析器 |
| 200 | /api/operations/incident-correlation | Operations | v17.32 | 多信号事件关联与根因建议引擎 |
| 201 | /api/product/service-topology | Product | v17.33 | 集群级服务依赖拓扑与级联故障风险分析器 |
| 202 | /api/deployment/chaos-readiness | Deployment | v17.34 | 混沌工程就绪度评估与实验推荐引擎 |
| 203 | /api/scalability/carbon-footprint | Scalability | v17.35 | 集群碳足迹与可持续性度量分析器 |
| 204 | /api/security/admission-policy-audit | Security | v17.36 | 准入控制策略差距与 CEL 表达式审计器 |
| 205 | /api/operations/pod-anomaly | Operations | v17.38 | Pod 性能异常与嘈杂邻居检测器 |
| 206 | /api/product/exposure-map | Product | v17.39 | 集群外部暴露面风险地图 |
| 207 | /api/scalability/scale-simulator | Scalability | v17.40 | 工作负载扩缩容影响模拟器（What-If 分析） |
| 208 | /api/deployment/rollback-risk | Deployment | v17.41 | 回滚风险与修订完整性评估器 |
| 209 | /api/operations/pod-lifecycle | Operations | v17.42 | Pod 生命周期阶段分析与停留时间追踪器 |
| 210 | /api/security/rbac-graph | Security | v17.43 | RBAC 权限图与提权路径分析器 |
| 211 | /api/product/gateway-audit | Product | v17.44 | Gateway API 与 Ingress 控制器健康审计 |
| 212 | /api/scalability/cost-allocation | Scalability | v17.45 | 命名空间成本分摊与计费报告 |
| 213 | /api/deployment/gitops-audit | Deployment | v17.46 | GitOps/CD 管道健康与配置漂移审计器 |
| 214 | /api/operations/metrics-pipeline-audit | Operations | v17.47 | Metrics 管道完整性审计 |
| 215 | /api/security/compliance-map | Security | v17.48 | SOC2/PCI-DSS/HIPAA 合规框架映射 |
| 216 | /api/product/probe-effectiveness | Product | v17.49 | 健康探针有效性与故障检测分析器 |
| 217 | /api/scalability/node-upgrade-audit | Scalability | v17.50 | 节点升级就绪度与 K8s 版本兼容性审计 |
| 218 | /api/operations/predictive-health | Operations | v17.52 | 集群预测性健康与风险预报引擎 |
| 219 | /api/deployment/change-readiness | Deployment | v17.53 | 部署变更就绪预检门禁 |
| 220 | /api/scalability/request-intelligence | Scalability | v17.54 | 资源请求智能分析与 Right-Sizing 引擎 |
| 221 | /api/product/reliability-scorecard | Product | v17.55 | 服务可靠性综合评分卡（A-F 等级） |
| 222 | /api/security/posture-scorecard | Security | v17.56 | 集群安全态势综合评分卡（A-F 等级） |
| 223 | /api/operations/triage | Operations | v17.58 | AIOps 事件分诊与修复行动计划引擎 |
| 224 | /api/deployment/impact-simulator | Deployment | v17.59 | 部署影响模拟器与爆炸半径预测 |
| 225 | /api/scalability/cost-intelligence | Scalability | v17.60 | 成本智能分析与支出预测引擎（FinOps 成熟度评分） |
| 226 | /api/product/golden-signals | Product | v17.61 | SRE 四大黄金信号统一健康引擎 |
| 227 | /api/security/remediation-matrix | Security | v17.62 | 安全修复优先级矩阵与风险-工作量分析 |
| 228 | /api/operations/mttr | Operations | v17.63 | 平均恢复时间（MTTR）与事件生命周期分析引擎 |
| 229 | /api/deployment/rollout-forensics | Deployment | v17.64 | 部署发布故障取证与部署反模式检测引擎 |
| 230 | /api/scalability/autoscaling-intel | Scalability | v17.65 | 自动扩缩容智能分析与扩缩容行为画像引擎 |
| 231 | /api/product/ownership-map | Product | v17.66 | 工作负载归属与问责治理引擎 |

**总计：307 个 OpenAPI 端点，315 个 Dashboard API 端点**
