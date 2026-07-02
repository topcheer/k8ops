# k8ops 部署指南

## 一键安装与卸载

### 前提条件

- Kubernetes 1.24+ (k3s / k8s / EKS / GKE / AKS 均可)
- kubectl 已配置并可连接集群
- 本地或远程容器镜像仓库（默认使用 `registry.iot2.win`）
- 可选：LLM API Key（OpenAI / DeepSeek / ZAI 等兼容接口）

---

## 方式一：Deployment 模式（推荐）

单副本 Deployment，适合大多数场景。包含 Ingress、Service、ConfigMap、Secret、RBAC，一条命令完成全部部署。

### 安装

```bash
# 本地网络（已含域名、镜像、CORS 等所有配置）
kubectl apply -k config/deploy/overlays/local

# 或自定义 overlay
cp -r config/deploy/overlays/local config/deploy/overlays/myorg
# 编辑 myorg/kustomization.yaml：替换镜像地址、域名、CORS 等
kubectl apply -k config/deploy/overlays/myorg
```

### 验证

```bash
# 检查 Pod 状态
kubectl get pods -n k8ops-system

# 检查 Ingress
kubectl get ingress -n k8ops-system

# 访问 Dashboard
# 浏览器打开 https://<你的域名>  (如 https://k8ops.iot2.win)
# 默认登录: admin / admin（首次登录会提示修改密码）
```

### 卸载

```bash
kubectl delete -k config/deploy/overlays/local
```

---

## 方式二：DaemonSet 模式

每个节点运行一个 Pod，支持节点级诊断（hostPID、hostPath）。适合需要深度节点监控的场景。

### 安装

```bash
kubectl apply -f config/daemonset-local.yaml
```

### 验证

```bash
# 检查 DaemonSet（每个节点一个 Pod）
kubectl get ds -n k8ops-system
kubectl get pods -n k8ops-system -o wide

# 访问 Dashboard（通过 Service ClusterIP 或 Ingress）
kubectl get svc k8ops-dashboard -n k8ops-system
```

### 卸载

```bash
kubectl delete -f config/daemonset-local.yaml
```

---

## 方式三：install.sh 脚本

```bash
# 安装（自动检测环境，交互式选择 Deployment / DaemonSet）
./install.sh install

# 卸载
./install.sh uninstall

# 查看状态
./install.sh status
```

---

## 镜像构建与推送

```bash
# 本地构建（amd64，适用于集群节点）
docker buildx build --platform linux/amd64 \
  --build-arg VERSION=v1.0.0 \
  -t registry.iot2.win/k8ops:v1.0.0 \
  -t registry.iot2.win/k8ops:latest \
  --load .

# 推送到 registry
docker push registry.iot2.win/k8ops:v1.0.0
docker push registry.iot2.win/k8ops:latest
```

### 多架构构建

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  --build-arg VERSION=v1.0.0 \
  -t registry.iot2.win/k8ops:v1.0.0 \
  --push .
```

---

## LLM Provider 配置

### 方式一：Dashboard 界面配置（推荐）

1. 登录 Dashboard → **Settings** 标签页
2. 填写 Provider 类型、API Key、Endpoint、Model
3. 点击 **Save**，自动持久化到 K8s ConfigMap/Secret

### 方式二：环境变量

在 overlay 的 ConfigMap 中设置：

```yaml
configMapGenerator:
- name: k8ops-config
  literals:
  - PROVIDER_TYPE=openai          # openai / deepseek / zai / anthropic
  - PROVIDER_MODEL=gpt-4o
  - PROVIDER_ENDPOINT=https://api.openai.com/v1
```

API Key 通过 Secret：

```yaml
secretGenerator:
- name: k8ops-credentials
  literals:
  - api-key=sk-your-api-key-here
```

### 支持的 Provider

| Provider | Endpoint | 示例 Model |
|----------|----------|------------|
| OpenAI | `https://api.openai.com/v1` | gpt-4o, gpt-4o-mini |
| DeepSeek | `https://api.deepseek.com/v1` | deepseek-chat |
| ZAI (智谱) | `https://open.bigmodel.cn/api/paas/v4` | glm-4-flash, glm-4-plus |
| Anthropic | `https://api.anthropic.com/v1` | claude-3-5-sonnet |
| 本地 | `http://localhost:11434/v1` | llama3, qwen2 |

---

## 认证配置

### 本地认证（默认）

开箱即用，用户存储在 SQLite。首次登录：`admin / admin`。

### LDAP

```yaml
# 在 ConfigMap 或 Provider 配置中设置
LDAP_SERVER=ldap://your-ldap:389
LDAP_BIND_DN=cn=admin,dc=example,dc=com
LDAP_BIND_PASSWORD=secret
LDAP_USER_BASE=ou=users,dc=example,dc=com
LDAP_SKIP_TLS_VERIFY=false   # 生产环境务必为 false
```

### OIDC（GitHub / Google / Keycloak 等）

```yaml
# Provider 配置（Dashboard Settings 页面或 CRD）
OIDC_ISSUER=https://your-keycloak/realms/myrealm
OIDC_CLIENT_ID=k8ops
OIDC_CLIENT_SECRET=your-secret
OIDC_REDIRECT_URL=https://k8ops.iot2.win/auth/oidc/callback
```

---

## Ingress 与 TLS

### 自动 TLS（cert-manager + Let's Encrypt）

确保集群已安装 cert-manager，在 Ingress 中添加 annotation：

```yaml
annotations:
  cert-manager.io/cluster-issuer: letsencrypt-prod
```

### 使用已有 TLS 证书

```bash
kubectl create secret tls k8ops-dashboard-tls \
  --cert=fullchain.pem \
  --key=privkey.pem \
  -n k8ops-system
```

---

## 常见问题

### Pod 一直 Pending

```bash
# 检查调度失败原因
kubectl describe pod <pod-name> -n k8ops-system | tail -10

# 常见原因：
# - hostNetwork 端口冲突 → 移除 hostNetwork: true 或避免端口声明冲突
# - 资源不足 → 调整 resources.requests/limits
# - 节点污点 → 检查 tolerations
```

### Dashboard 返回 502

```bash
# 1. 检查 Pod 是否 Ready
kubectl get pods -n k8ops-system

# 2. 检查 Service endpoints
kubectl get endpoints k8ops-dashboard -n k8ops-system

# 3. 检查 Ingress backend
kubectl describe ingress -n k8ops-system

# 4. 等待 Pod 完全就绪后重试
```

### 镜像拉取失败

```bash
# 方案 1：设置 imagePullPolicy: Always（使用具体 tag 时推荐）
# 方案 2：确保节点已配置 registry 的 TLS 信任
# 方案 3：如用私有 registry，创建 imagePullSecrets
```

### LLM API 401

```bash
# 检查 API Key 是否正确配置
kubectl get secret k8ops-credentials -n k8ops-system -o jsonpath='{.data.api-key}' | base64 -d

# 或在 Dashboard → Settings 重新配置 Provider
```

---

## 升级

```bash
# 构建并推送新镜像
docker buildx build --platform linux/amd64 \
  --build-arg VERSION=v2.0.0 \
  -t registry.iot2.win/k8ops:v2.0.0 \
  -t registry.iot2.win/k8ops:latest \
  --push .

# 滚动更新（Deployment 模式）
kubectl set image deployment/k8ops \
  k8ops=registry.iot2.win/k8ops:v2.0.0 \
  -n k8ops-system

# 或修改 overlay 中的 newTag 后重新 apply
kubectl apply -k config/deploy/overlays/local

# DaemonSet 模式
kubectl set image daemonset/k8ops \
  k8ops=registry.iot2.win/k8ops:v2.0.0 \
  -n k8ops-system
```

---

## 数据备份与恢复

### SQLite 自动备份（CronJob）

k8ops 使用 SQLite 存储用户、审计日志、会话数据。建议每小时自动备份：

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: k8ops-backup
  namespace: k8ops-system
spec:
  schedule: "0 * * * *"  # 每小时整点
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: backup
            image: busybox
            command:
            - sh
            - -c
            - |
              TIMESTAMP=$(date +%Y%m%d-%H%M%S)
              cp /data/k8ops.db /backup/k8ops-${TIMESTAMP}.db
              # 保留最近 24 个备份
              ls -t /backup/k8ops-*.db | tail -n +25 | xargs rm -f
            volumeMounts:
            - name: data
              mountPath: /data
              readOnly: true
            - name: backup
              mountPath: /backup
          volumes:
          - name: data
            persistentVolumeClaim:
              claimName: k8ops-data
          - name: backup
            hostPath:
              path: /var/lib/k8ops-backup
              type: DirectoryOrCreate
          restartPolicy: OnFailure
```

### 手动备份

```bash
# 从 Pod 中拷贝数据库
kubectl cp k8ops-system/<pod-name>:/data/k8ops.db ./k8ops-backup-$(date +%Y%m%d).db

# 或使用 sqlite3 在线备份（不断写）
kubectl exec -n k8ops-system <pod-name> -- sqlite3 /data/k8ops.db ".backup /data/k8ops-backup.db"
kubectl cp k8ops-system/<pod-name>:/data/k8ops-backup.db ./k8ops-backup.db
```

### 恢复

```bash
# 停止 k8ops
kubectl scale deployment k8ops -n k8ops-system --replicas=0

# 恢复数据库
kubectl cp ./k8ops-backup.db k8ops-system/<pod-name>:/data/k8ops.db

# 重启
kubectl scale deployment k8ops -n k8ops-system --replicas=1
```

---

## 高可用 (HA) 部署

### 单节点模式（默认，适合开发/小集群）

- 1 replica + SQLite + PVC
- Pod 重启时服务短暂中断（~10s）
- 适合 < 50 用户的团队

### 多副本 HA（生产推荐）

使用 MySQL/PostgreSQL 替代 SQLite，支持多副本：

1. **切换数据库到 MySQL**：

```yaml
# 在 overlay ConfigMap 中设置
configMapGenerator:
- name: k8ops-config
  literals:
  - DB_DRIVER=mysql
  - DB_DSN=k8ops:password@tcp(mysql:3306)/k8ops?charset=utf8mb4&parseTime=True
```

2. **多副本 + leader election**：

```yaml
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: k8ops
        env:
        - name: LEADER_ELECT
          value: "true"
```

3. **共享存储**：MySQL 使用独立 PVC，k8ops Pod 无状态

### 容量规划

| 规模 | 用户数 | 资源建议 | 数据库 |
|------|--------|----------|--------|
| 小型 | < 20 | 1 pod, 500m CPU / 512Mi | SQLite |
| 中型 | 20-100 | 2 pods, 1 CPU / 1Gi each | MySQL |
| 大型 | 100+ | 3+ pods, 2 CPU / 2Gi each | MySQL + 读写分离 |

---

## CI/CD 流程与发布管理

### 一键部署脚本

k8ops 提供自动化部署脚本，包含预检、构建、发布、健康检查和自动回滚：

```bash
# 部署新版本（自动预检 + 构建 + 发布 + 健康检查）
./scripts/deploy.sh v14.36

# 部署流程：
# 1. 预检：go build + go vet + go test + gofmt
# 2. 构建：Docker buildx + push 到 registry
# 3. 发布：kubectl set image + change-cause 注解
# 4. 验证：Pod Ready + HTTP 200（120s 超时）
# 5. 回滚：健康检查失败时自动回滚到上一版本
```

### 快速回滚

```bash
# 回滚到上一版本
./scripts/rollback.sh

# 回滚到指定 revision
./scripts/rollback.sh 58

# 回滚到指定版本号
./scripts/rollback.sh v14.30
```

### 发布历史追踪

每次部署自动记录 change-cause 注解：

```bash
# 查看发布历史
kubectl rollout history daemonset/k8ops -n k8ops-system

# 查看特定 revision 详情
kubectl rollout history daemonset/k8ops -n k8ops-system --revision=55
```

### CI 流程 (GitHub Actions)

| 工作流 | 触发条件 | 内容 |
|--------|----------|------|
| `ci.yml` — push/PR to main | 代码提交 | test + vet + lint + govulncheck + Docker build |
| `release.yml` — tag v* | 版本标签 | 全量测试 + GoReleaser + Docker multi-arch + 自动 Release Notes |

### 镜像管理

| 标签 | 说明 |
|------|------|
| `registry.iot2.win/k8ops:v14.XX` | 特定版本 |
| `registry.iot2.win/k8ops:latest` | 最新稳定版 |
| `ghcr.io/<org>/k8ops:v14.XX` | GHCR 镜像（CI 发布） |

### 镜像优化

- 基础镜像：`gcr.io/distroless/static-debian12:nonroot`（无 shell、无包管理器）
- 多阶段构建：Go builder + distroless runtime
- BuildKit 缓存：`--mount=type=cache` 加速 CI 构建
- 二进制优化：`-trimpath -ldflags="-s -w"` 减小体积

| 版本 | 镜像大小 |
|------|----------|
| v14.30 (alpine) | 31.8 MB |
| v14.35 (distroless) | 28.6 MB |

