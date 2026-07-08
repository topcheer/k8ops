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

### 36. ConfigMap & Secret Configuration Audit (v15.14)

**Path:** `GET /api/product/config-audit?namespace=xxx`

Audits ConfigMaps and Secrets for configuration best practices. ConfigMap: large size detection (>1MB), unreferenced detection, empty data, immutability. Secret: stale credential detection (>180d), unreferenced, plaintext credential keys, rotation recommendations. Cross-reference engine scans pod volumes, env vars, envFrom, and projected sources. Returns config audit health score (0-100).

### 37. Container Image Deployment Hygiene Analyzer (v15.13)

**Path:** `GET /api/deployment/image-hygiene?namespace=xxx`

Analyzes all running container images for deployment hygiene. Per-image: tag strategy (versioned/latest/digest-pinned), registry trust level, risk classification. Duplicate detection (same base image with multiple tags). Registry distribution. Cluster-wide image hygiene score (0-100).

### 38. Deployment Rollout Strategy & Health Analyzer (v15.19)

**Path:** `GET /api/deployment/rollout-health?namespace=xxx`

Analyzes deployment rollout strategies and health. Per-deployment: strategy type, maxSurge/maxUnavailable, revisionHistoryLimit (rollback readiness), progressDeadline, replica status. Status classification: healthy/progressing/stuck/paused. Risk detection: Recreate downtime, no-rollback, aggressive deadline. Cluster-wide rollout health score (0-100).

### 39. Certificate & TLS Expiry Monitor (v15.16)

**Path:** `GET /api/security/cert-expiry?namespace=xxx`

Monitors all TLS certificates (kubernetes.io/tls Secrets) for expiry. Parses PEM to extract CN, SANs, Issuer, validity period, key size, self-signed status. Risk: critical (expired/<30d), high (<60d), medium (<90d), low (>90d). Pod reference tracking. Cluster-wide certificate health score (0-100).

### 40. PDB Compliance & Voluntary Disruption Risk (v15.17)

**Path:** `GET /api/operations/pdb-audit?namespace=xxx`

Audits PodDisruptionBudget compliance and voluntary disruption risk. PDB status: healthy/blocked/impossible. Unprotected multi-replica deployment detection. Node drain simulation (per-node PDB blocking analysis). Cluster-wide PDB coverage score (0-100).

### 41. Namespace Resource Consumption & Cost Attribution (v15.18)

**Path:** `GET /api/scalability/ns-consumption`

Analyzes per-namespace resource consumption and estimates cost attribution. Aggregates CPU/memory requests and limits plus PVC storage. Cost estimation ($28/core CPU, $3.8/GB memory, $0.10/GB storage). Waste analysis: over-provisioned, idle namespaces, wasted capacity. Top 10 consumers.

### 42. Network Policy Compliance & Traffic Isolation (v15.20)

**Path:** `GET /api/product/network-policy?namespace=xxx`

Audits NetworkPolicy coverage and traffic isolation. Per-namespace: pod count, policy count, protected pods, default-deny status, isolation score (0-100). Unprotected pod detection via label selector matching. Permissive egress detection (0.0.0.0/0). Cluster isolation score (0-100).

### 43. Volume Security & Mount Risk Auditor (v15.22)

**Path:** `GET /api/security/volume-mounts?namespace=xxx`

Audits all pod volume mounts for container escape risks. Scans 14 known dangerous paths (docker.sock, /proc, /sys, /, kubelet, etcd). HostPath risk classification (critical/high/medium/low). Privileged container and host namespace detection. SA token projected volume tracking. Cluster security score (0-100, higher = safer).

### 44. Topology Spread & Pod Distribution Auditor (v15.23)

**Path:** `GET /api/operations/topology-distribution?namespace=xxx`

Audits pod distribution across nodes. Per-workload: node distribution map, max pods per node, spread ratio, TSC/anti-affinity status. Risk: critical (>70% on 1 node) / high (>50%) / medium (>34%) / low (<34%). Node load imbalance detection. Distribution score (0-100).

### 45. Cluster Capacity Headroom & Scale-Out Readiness (v15.24)

**Path:** `GET /api/scalability/capacity-headroom`

Analyzes cluster capacity headroom. Per-node: allocatable vs used CPU/memory, headroom %, bottleneck resource. Pod scheduling profiles (small/medium/large/xlarge). Scale-out readiness: Cluster Autoscaler/Karpenter detection, urgency level. Headroom score (0-100).

### 46. Health Probe Compliance Auditor (v15.25)

**Path:** `GET /api/deployment/probe-compliance?namespace=xxx`

Audits liveness, readiness, and startup probe configuration. Per-container: probe type, path, port, timing thresholds. Issue detection: zero probes, missing readiness (critical), missing liveness, TCP probes. Misconfiguration: excessive delay, slow period, high threshold. Probe compliance health score (0-100).

### 47. Label & Annotation Hygiene Auditor (v15.26)

**Path:** `GET /api/product/label-hygiene?namespace=xxx`

Audits label hygiene across all workloads. Zero-label detection (breaks Service selectors), missing standard labels (app.kubernetes.io/name), missing team/owner labels, malformed key detection (DNS-1123), excessive labels (>20). Per-namespace scoring. Cluster compliance health score (0-100).

### 48. Service Endpoint Exposure & Attack Surface (v15.28)

**Path:** `GET /api/security/endpoint-exposure?namespace=xxx`

Maps all externally-accessible services and ingress routes. Per-service: type, exposure level (public/node/internal), port analysis (HTTP/HTTPS), NetworkPolicy coverage. Per-ingress: hosts, TLS status, backend, route counts. Attack surface score (0-100, higher = safer).

### 49. ImagePullBackOff & Container Start Failure Tracker (v15.29)

**Path:** `GET /api/operations/image-pull-failures?namespace=xxx`

Tracks image pull failures and container start failures. Per-container: image, reason, message, restart count. Failure types: ImagePullBackOff, ErrImagePull, CreateContainerError, CrashLoopBackOff. Root cause: registry auth, rate limiting, invalid images. Per-image aggregation. Health score (0-100).

### 50. Resource Quota Utilization & Limit Compliance (v15.30)

**Path:** `GET /api/scalability/quota-utilization?namespace=xxx`

Audits ResourceQuota utilization and LimitRange compliance. Per-quota: hard/used/utilization%, risk level. LimitRange: default request/limit, max enforcement. Container governance: missing requests/limits. Compliance score (0-100).

### 51. Resource Limit & Enforcement Gap Auditor (v15.32)

**Path:** `GET /api/deployment/resource-limits?namespace=xxx`

Audits resource limits and enforcement gaps. Per-container: CPU/memory requests/limits, request-to-limit ratio. Unbounded containers (no limits, critical), missing memory limits (OOM risk), under-provisioned (<1.2x ratio), over-provisioned (>4x ratio). Compliance score (0-100).

### 52. Orphaned Resource Detector (v15.33)

**Path:** `GET /api/product/orphaned-resources?namespace=xxx`

Detects orphaned resources across 5 types: Services (no backing pods), ConfigMaps (unreferenced), Secrets (stale credentials), PVCs (unmounted), Ingresses (missing backend service). Pod reference tracking via volumes, env vars, envFrom, imagePullSecrets. Hygiene score (0-100).

### 53. Seccomp & PSS Restricted Compliance Auditor (v15.34)

**Path:** `GET /api/security/seccomp-audit?namespace=xxx`

Audits seccomp profiles and Pod Security Standards restricted-level compliance. Per-container: seccomp type, capabilities drop/add, allowPrivilegeEscalation, runAsNonRoot, readOnlyRootFilesystem. PSS level: restricted/baseline/privileged. Dangerous capability detection (11 caps). Hardening score (0-100).

### 54. Pod Restart Reason Analyzer (v15.35)

**Path:** `GET /api/operations/restart-reasons?namespace=xxx`

Comprehensively categorizes pod restart reasons. Per-container: last termination reason, exit code, restart count. Categories: OOMKilled (exit 137), application errors (exit != 0), config errors, DeadlineExceeded, Completed. Top 20 restarters. Stability score (0-100).

### 55. HA & Single-Point-of-Failure Detector (v15.36)

**Path:** `GET /api/scalability/ha-audit?namespace=xxx`

Detects single points of failure. SPOF detection: single-replica deployments, multi-replica without PDB, no pod anti-affinity, single-node spread, missing readiness probes. Risk: critical (single replica/node), high (no PDB), medium (no anti-affinity/readiness). HA score (0-100).

### 56. Graceful Shutdown & Termination Compliance (v15.38)

**Path:** `GET /api/deployment/graceful-shutdown?namespace=xxx`

Audits graceful shutdown config for zero-downtime deployments. Per-container: preStop hook (httpGet/exec), readiness probe, terminationGracePeriodSeconds classification. Risk: critical (no preStop + no readiness = dropped requests), high (no preStop). Shutdown score (0-100).

### 57. PV/PVC Storage Health & Capacity (v15.39)

**Path:** `GET /api/product/pvc-health?namespace=xxx`

Audits PV/PVC health. Per-PVC: phase (Bound/Pending/Lost), SC, capacity. Per-PV: phase (Bound/Available/Released/Failed), reclaim policy. StorageClass: expansion support, default detection. Storage health score (0-100).

### 58. CronJob & Batch Job Security Audit (v15.40)

**Path:** `GET /api/security/batch-audit?namespace=xxx`

Audits CronJobs and Jobs for security risks. Detection: privileged (critical), hostPath (critical), hostNetwork/PID (high), default SA (medium), suspicious every-minute schedules (persistence), no concurrency limit (fork-bomb). Batch security score (0-100).

### 59. Pod Scheduling Latency Analyzer (v15.41)

**Path:** `GET /api/operations/scheduling-latency?namespace=xxx`

Tracks pod scheduling latency. Per-pod: creation-to-scheduled time, pending reason. Detection: unschedulable, resource shortage (Insufficient cpu/memory), slow scheduling (>60s), very slow (>300s). Scheduling efficiency score (0-100).

### 60. Node Failure Impact Simulator (v15.42)

**Path:** `GET /api/scalability/node-failure-sim`

Simulates each node failing. Per-node: affected pods, can reschedule (resource/selector/taint check), unschedulable, single-replica workloads. Excludes DaemonSet/completed/system pods. Resilience score (0-100).

### 61. Deployment Update Strategy & Rollback Readiness (v15.44)

**Path:** `GET /api/deployment/update-strategy?namespace=xxx`

Audits deployment update strategies for safe rollouts. Per-deployment: strategy type (RollingUpdate/Recreate), maxSurge/maxUnavailable, revisionHistoryLimit, progressDeadlineSeconds. Detection: Recreate (critical), maxUnavailable=100% (high), maxSurge=0, low rev history, no progress deadline. Readiness score (0-100).

### 62. StatefulSet Health & Ordered Rollout Audit (v15.45)

**Path:** `GET /api/product/statefulset-audit?namespace=xxx`

Audits StatefulSet health. Per-STS: pod management policy (OrderedReady/Parallel), PVC retention (Retain/Delete), headless service existence, volumeClaimTemplates, partition canary. Detection: no headless svc (critical), stuck rollout (high), PVC Delete (high), paused canary (warning). Health score (0-100).

### 63. Resource Contention & Throttling Detector (v15.46)

**Path:** `GET /api/operations/resource-contention?namespace=xxx`

Detects CPU throttling, memory pressure, and resource contention. Detection: node MemoryPressure/DiskPressure (critical), high-restart pods (CPU throttled), no CPU/mem limits, CPU limit <100m, mem limit <128Mi. Contention score (0-100).

---

## API Summary

### 64. API Object Count & CRD Explosion Risk (v15.48)

**Path:** `GET /api/scalability/crd-explosion`

Counts API objects per resource type. Per-resource: object count, risk level. Per-namespace: ConfigMap/Secret/Service/Pod counts. Detection: very high counts, excessive secrets, excessive CRDs. Scalability score (0-100).

### 65. Secret/ConfigMap Reference Integrity Checker (v15.49)

**Path:** `GET /api/deployment/ref-integrity?namespace=xxx`

Verifies every Secret/ConfigMap reference exists. Checks volumes, envFrom, env valueFrom. Detection: broken refs (critical), optional missing refs. Integrity score (0-100).

### 66. Affinity & Anti-Affinity Conflict Detector (v15.50)

**Path:** `GET /api/product/affinity-conflict?namespace=xxx`

Detects pods stuck due to unsatisfiable affinity rules. Topology domain map from node labels. Detection: unsatisfiable anti-affinity (critical), pending due to affinity (high). Health score (0-100).

---

## API Summary

### 67. Node Lease & Heartbeat Health Monitor (v15.52)

**Path:** `GET /api/operations/node-lease`

Monitors kubelet heartbeat freshness via Lease objects. Per-node: lease existence, heartbeat age, holder identity, kubelet version, active conditions. Detection: no lease (critical), very stale >2min (critical), stale >40s (high), NotReady (warning). Health score (0-100).

### 68. K8s Scalability Bottleneck Predictor (v15.53)

**Path:** `GET /api/scalability/bottleneck-predictor`

Predicts which K8s resource hits its limit first. Compares 7 resources against K8s limits: pods/node (110), total pods (150k), services (5k), services/node (20), nodes (5k), namespaces (10k). Status: healthy/warning/critical/bottleneck. Risk score (0-100).

### 69. Deployment Image Drift & Version Consistency Detector (v15.54)

**Path:** `GET /api/deployment/image-drift?namespace=xxx`

Detects image version drift within workloads. Per-workload: distinct image variants with pod counts, drift detection, latest tag usage, digest presence. Detection: image drift (high), latest tag (medium), no digest (low). Consistency score (0-100).

---

## API Summary

### 70. Node Taint & Pod Toleration Impact Analyzer (v15.56)

**Path:** `GET /api/product/taint-toleration`

Analyzes node taints and pod tolerations for maintenance planning. Per-node: taint list (NoSchedule/NoExecute), cordon status. Cluster-wide taint summary. Detection: NoExecute (critical, evicting pods), cordoned nodes (warning), broad tolerations key=Exists (warning). Impact score (0-100).

### 71. Control Plane Health Checker (v15.57)

**Path:** `GET /api/operations/control-plane`

Verifies control plane component health (kube-apiserver, kube-scheduler, kube-controller-manager, etcd). Per-component: ready, restarts, uptime, risk. Detection: unhealthy (critical), excessive restarts (high), missing components (critical). k3s/microk8s/kind detection. Health score (0-100).

---

### 75. Pod Eviction & Node Pressure History Tracker (v15.63)

**Path:** `GET /api/operations/pod-evictions?namespace=xxx`

Tracks evicted pods with cause classification (memory/disk/pid/unknown). Per-node eviction counts and risk levels. Recent evictions (24h). Health score (0-100).

---

### 76. API Server Audit Logging Configuration Checker (v15.65)

**Path:** `GET /api/security/audit-policy`

Verifies audit logging configuration for compliance. Checks: audit enabled, policy file, log retention, sensitive resource coverage. Detects k3s/microk8s. Compliance score (0-100) for PCI-DSS/SOC2/HIPAA.

### 77. CSI Driver & Storage Capability Auditor (v15.67)

**Path:** `GET /api/scalability/csi-audit`

Audits CSI drivers and StorageClass capabilities. Per-SC: provisioner, default, binding mode, expansion, reclaim policy. Per-CSIDriver: attach required, fsGroup policy. Detection: no default SC, missing CSI driver, no expansion, Delete policy. Health score (0-100).

---

## API Summary

### 78. Deployment Disruption & Maintenance Impact Analyzer (v15.69)

**Path:** `GET /api/deployment/disruption-impact`

Analyzes Deployment/StatefulSet + PDB interaction for maintenance readiness. Per-workload: PDB presence, minAvailable/maxUnavailable, evictable pods, block-drain flag. Maintenance readiness score (0-100).

### 79. Batch Job Execution Health & Completion Analyzer (v15.70)

**Path:** `GET /api/product/job-health?namespace=xxx`

Analyzes batch Job execution health. Per-job: status, duration, completions, backoffLimit, parent CronJob. Detection: failed jobs, long-running >24h, suspended. Health score (0-100).

---

## API Summary

**Total: 145 OpenAPI endpoints** across 6 dimensions:
- **Product**: Cluster resources, DNS health, config audit, network policy, label hygiene, orphaned resources, PVC health, StatefulSet audit, affinity conflict, taint toleration, configmap size, job health
- **Deployment**: Image hygiene, rollout health, probe compliance, resource limits, graceful shutdown, update strategy, ref integrity, image drift, revision history, disruption impact
- **Operations**: CrashLoopBackOff, PDB compliance, topology distribution, image pull failures, restart reasons, scheduling latency, resource contention, node lease, control plane, pod evictions
- **Security**: Admission webhook, certificate expiry, volume security, endpoint exposure, seccomp & PSS, batch security, audit policy
- **Scalability**: Overcommit, storage forecast, pod density, NS consumption, capacity headroom, quota utilization, HA & SPOF, node failure sim, CRD explosion, bottleneck predictor, namespace isolation, CSI audit
- **Infrastructure**: Auth, RBAC, health, version
