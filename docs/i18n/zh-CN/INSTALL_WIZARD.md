# k8ops 安装向导指南

交互式安装向导（`wizard.sh`）引导您在部署前配置所有主要的 k8ops 组件：数据库后端、SSO 集成和 AI Provider。

## 快速开始

### 交互模式

```bash
git clone https://github.com/topcheer/k8ops.git
cd k8ops
./wizard.sh
```

### 非交互模式

```bash
# 编辑 config/wizard-values.yaml 填入您的配置，然后：
./wizard.sh --values config/wizard-values.yaml
```

### 试运行（仅生成清单）

```bash
./wizard.sh --dry-run
# 查看生成的文件：.wizard-*.yaml
# 手动部署：kubectl apply -f ...
```

## 向导步骤

### 第 1 步：部署模式

| 模式 | 说明 | 适用场景 |
|------|------|---------|
| **DaemonSet** | 在每个节点运行 | K3s/裸金属集群、节点级监控 |
| **Deployment** | 单副本 + PVC | 托管 K8s（EKS/GKE/AKS）、成本敏感的部署 |

### 第 2 步：数据库后端

k8ops 使用数据库存储用户账户、角色和认证 Provider。

| 后端 | 使用场景 | 高可用 | 配置 |
|------|---------|--------|------|
| **SQLite** | 小型集群、单节点 | 否 | 零配置（内嵌） |
| **MySQL** | 多副本、共享认证 | 是 | 内部 StatefulSet 或外部连接 |
| **PostgreSQL** | 多副本、共享认证 | 是 | 内部 StatefulSet 或外部连接 |

#### 内部 vs 外部数据库

- **内部**：向导在 `k8ops-system` 命名空间中部署 MySQL/PostgreSQL StatefulSet 和 PVC。完全托管 — 无外部依赖。
- **外部**：连接到您现有的数据库。您需要提供 DSN 连接字符串。

#### DSN 格式

**MySQL:**
```
k8ops:password@tcp(mysql-host:3306)/k8ops?charset=utf8mb4&parseTime=True
```

**PostgreSQL:**
```
host=postgres-host user=k8ops password=secret dbname=k8ops sslmode=disable
```

### 第 3 步：SSO / 身份提供者

k8ops 支持多种 SSO Provider，内置预设模板：

| Provider | 类型 | 预设 |
|----------|------|------|
| **GitHub** | OIDC | 预配置 issuer |
| **Google** | OIDC | 预配置 issuer |
| **Microsoft** (Entra ID) | OIDC | 预配置 issuer |
| **GitLab** | OIDC | 预配置 issuer |
| **Keycloak** | OIDC | 自定义 issuer（您的 realm） |
| **Okta** | OIDC | 自定义 issuer |
| **Auth0** | OIDC | 自定义 issuer |
| **LDAP / AD** | LDAP | 服务器 + Bind DN |
| **自定义 OIDC** | OIDC | 手动输入 issuer URL |

#### OIDC 回调 URL

在身份提供者处注册应用时，使用以下回调 URL：

```
https://<your-dashboard-host>/api/auth/oidc/<provider-name>/callback
```

GitHub 示例：
```
https://k8ops.example.com/api/auth/oidc/github/callback
```

#### LDAP 配置

需提供：
- **服务器 URL**：`ldap://host:389` 或 `ldaps://host:636`
- **搜索基准**：例如 `ou=users,dc=example,dc=com`
- **Bind DN**：服务账户 DN，例如 `cn=admin,dc=example,dc=com`
- **Bind 密码**：服务账户密码

SSO 可以在安装时跳过，之后通过 Dashboard 的 **Settings > Auth Providers** 配置。

### 第 4 步：AI Provider

| Provider | 模型 | 备注 |
|----------|------|------|
| **OpenAI** | gpt-4o, gpt-4o-mini | 默认 |
| **Anthropic** | claude-sonnet-4-20250514 | Claude 系列 |
| **Gemini** | gemini-1.5-flash | Google AI |
| **自定义** | 任意 | OpenAI 兼容端点 |

AI Provider 可以推迟到安装后通过 Dashboard 的 **Settings** 配置。

### 第 5 步：确认与部署

向导显示所有选择的摘要。确认后将：

1. 生成 Kubernetes 清单（Secret、可选的数据库 StatefulSet）
2. 应用到集群
3. 部署 k8ops（DaemonSet 或 Deployment）
4. 等待 Pod 就绪
5. 显示访问 URL 和登录凭据

## 安装后

### 默认登录

- 用户名：`admin`
- 密码：`admin`
- **首次登录后请立即修改**

### 安装后配置 SSO

如果安装时跳过了 SSO：

1. 导航到 **Settings > Auth Providers**
2. 点击 **Add Provider**
3. 选择预设模板（GitHub、Google 等）
4. 输入 Client ID 和 Client Secret
5. 保存并启用

### 环境变量参考

向导设置以下环境变量（也可手动设置）：

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `AUTH_DB_DRIVER` | 数据库驱动 | `sqlite` |
| `AUTH_DB_DSN` | 数据库连接字符串 | （空） |
| `AUTH_DB_PATH` | SQLite 文件路径 | `/data/k8ops.db` |
| `AUTH_JWT_SECRET` | JWT 签名密钥 | （自动生成） |
| `AUTH_DEFAULT_ROLE` | SSO 用户默认角色 | `viewer` |
| `AIOPS_API_KEY` | AI Provider API 密钥 | （空） |

## CLI 参数

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

## 故障排查

### SQLite "out of memory" 错误

当 SQLite 数据库路径不可写时出现（例如只读容器文件系统）。确保 `/data` 由 `emptyDir` 或 PVC 卷提供。

### MySQL/PostgreSQL 连接失败

1. 验证 DSN 格式与您的数据库类型匹配
2. 检查从 k8ops Pod 到数据库的网络连通性
3. 确保数据库用户拥有 CREATE/ALTER 权限（用于自动迁移）

### SSO 回调不生效

1. 验证回调 URL 完全匹配（包括末尾斜杠）
2. 检查 HTTPS 是否正确配置（OIDC 要求 HTTPS）
3. 确保身份提供者已注册正确的回调 URL
