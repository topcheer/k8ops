# k8ops 安全

## 认证

k8ops 支持三种认证方式，可按部署配置：

### 本地认证

- 用户名/密码存储在 SQLite 中
- 密码使用 bcrypt 哈希
- 通过 `AUTH_DEFAULT_ROLE` 环境变量引导管理员账户

### LDAP / Active Directory

- 可配置的服务器 URL、Bind DN、搜索基准
- `SkipTLSVerify` 选项（默认：`false`）用于自签名证书
- 多 Provider 支持：可同时配置多个 LDAP 服务器

### OIDC (OpenID Connect)

- 支持任何兼容 OIDC 的身份提供者（Google、GitHub、Keycloak 等）
- **CSRF 防护**：state 参数使用 `crypto/subtle.ConstantTimeCompare` 验证
- **每 Provider 独立 cookie**：`oidc_state_{provider}` 防止多 Provider 冲突
- **Secure 标志**：通过 TLS 或 `X-Forwarded-Proto` 头自动检测
- **HttpOnly + SameSite**：state cookie 无法通过 JavaScript 访问

## RBAC 模型

### 角色

| 角色 | 范围 | 权限 |
|------|------|------|
| `admin` | 集群 | 完全访问：管理用户、Provider、所有命名空间 |
| `operator` | 集群 | 读取全部 + 对话 + 执行诊断 |
| `viewer` | 集群 | 只读：查看仪表板、报告 |
| `ns-admin` | 命名空间 | 在分配的命名空间内拥有管理员权限 |
| `ns-viewer` | 命名空间 | 在分配的命名空间内拥有只读权限 |

### 命名空间范围

具有命名空间范围角色的用户通过以下方式限制在分配的命名空间内：

1. **K8s RBAC 同步**：每个命名空间创建 `RoleBinding` 资源
2. **API 身份模拟**：Dashboard API 调用在与 K8s API 通信时使用用户身份
3. **命名空间过滤**：API 响应过滤为允许的命名空间

### 内置角色保护

内置角色（`admin`、`operator`、`viewer`）标记为 `Builtin: true`，无法通过 API 删除。

## 安全功能

### CORS 白名单

- 通过 `CORS_ALLOWED_ORIGINS` 环境变量配置（逗号分隔）
- 涉及凭据时不使用通配符（`*`）
- 未配置时仅允许同源

### OIDC CSRF 防护

- State 参数：每次认证尝试使用随机 nonce
- 使用 `subtle.ConstantTimeCompare` 验证（时序安全）
- 存储在带 Secure + SameSite 标志的 HttpOnly cookie 中

### JWT 持久化

- JWT 签名密钥持久化在 K8s Secret `k8ops-auth` 中（key：`jwt-secret`）
- 如果 Secret 不存在，则回退到临时随机密钥并输出警告日志
- 防止 Pod 重启时会话失效

### 审计日志

所有敏感操作均记录日志：

- 用户登录/登出
- Provider 配置变更
- 诊断执行
- 修复操作
- 用户角色变更

### 速率限制

- `resilience.RateLimiter` 可用（尚未接入 HTTP 层 — 未来工作）

### 优雅关闭

- `SIGTERM`/`SIGINT` → 排空 SSE 连接 → 刷新 SQLite WAL → 停止管理器
- 防止 Pod 驱逐时数据损坏

## 安全配置

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `AUTH_DB_DRIVER` | `sqlite` | 数据库驱动 |
| `AUTH_DB_DSN` | — | 数据库连接字符串 |
| `AUTH_DB_PATH` | `/data/k8ops.db` | SQLite 数据库路径 |
| `AUTH_JWT_SECRET` | （随机） | JWT 签名密钥（通过 K8s Secret 持久化） |
| `AUTH_DEFAULT_ROLE` | `viewer` | 新用户角色 |
| `CORS_ALLOWED_ORIGINS` | — | 逗号分隔的允许源 |
| `AIOPS_API_KEY` | — | LLM Provider API 密钥 |

### K8s Secret 管理

```yaml
# JWT 持久化的 K8s Secret
apiVersion: v1
kind: Secret
metadata:
  name: k8ops-auth
  namespace: k8ops-system
type: Opaque
stringData:
  jwt-secret: "<openssl rand -base64 32>"
```

部署通过以下方式读取：
```yaml
env:
- name: AUTH_JWT_SECRET
  valueFrom:
    secretKeyRef:
      name: k8ops-auth
      key: jwt-secret
      optional: true  # 不存在时回退到随机密钥
```

### LDAP TLS 配置

LDAP Provider 支持 `skip_tls_verify`（默认：`false`）：

```json
{
  "ldap": {
    "server": "ldaps://ldap.corp.com",
    "skip_tls_verify": false
  }
}
```

仅在开发环境使用自签名证书时设置 `skip_tls_verify: true`。

## 已知限制

1. **登录无速率限制** — `resilience.RateLimiter` 已存在但未接入 HTTP 处理器
2. **无 HTTPS 终止** — k8ops 提供 HTTP 服务；TLS 必须由 Ingress 控制器处理
3. **SQLite 单节点** — 无高可用数据库；适用于单副本部署
4. **无会话撤销** — JWT token 有效期至过期（24h）；无服务端撤销列表

## 安全报告

如需报告安全漏洞：

1. **请勿创建公开的 GitHub issue**
2. 发送邮件至 security@ggai.dev，附上详情和复现步骤
3. 我们将在 48 小时内确认并提供修复时间表
4. 感谢负责任的披露

## 未来安全改进

- [ ] 将速率限制接入登录 API
- [ ] 添加会话撤销（黑名单）
- [ ] 支持外部 OAuth Provider 用于 RBAC
- [ ] 添加 mTLS 用于服务间通信
- [ ] 实现静态加密密钥（超越 PVC）
