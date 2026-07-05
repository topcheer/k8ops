# k8ops 本地运行指南

> 不需要 Kubernetes 集群部署，在笔记本/工作站上直接运行 k8ops binary。

---

## 适用场景

- **本地开发调试** — 快速迭代代码，无需每次构建镜像
- **离线管理工具** — 作为智能 kubectl 替代品
- **演示和试用** — 不需要集群内部署即可体验全部功能
- **CI/CD 集成** — 在流水线中作为诊断工具运行

---

## 前提条件

- Go 1.26+（或直接下载预编译 binary）
- kubectl 已配置并可连接集群
- LLM API Key（OpenAI / DeepSeek / ZAI 等）

---

## 方式一：从源码编译

```bash
cd k8ops

# 编译 manager（dashboard 服务端）
go build -o k8ops-manager ./cmd/manager/

# 编译 CLI 工具
go build -o k8ops ./cmd/k8ops/
```

## 方式二：下载预编译 binary

从 [GitHub Releases](https://github.com/topcheer/k8ops/releases) 下载对应平台的二进制文件。

---

## 启动 Dashboard

```bash
AIOPS_API_KEY=your-api-key \
  ./k8ops-manager \
  𔒬leader-elect=false \
  𔒬dashboard-address=:9090 \
  𔒬auth-db-path=/tmp/k8ops.db
```

启动后访问 `http://localhost:9090`，默认登录 `admin / admin`。

### 参数说明

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--dashboard-address` | `:9090` | Dashboard 监听地址 |
| `--leader-elect` | `false` | Leader Election（单实例运行需关闭） |
| `--metrics-bind-address` | `:8080` | Prometheus metrics 端口 |
| `--health-probe-bind-address` | `:8081` | 健康检查端口 |
| `--auth-db-path` | `/data/k8ops.db` | SQLite 数据库路径 |
| `--auth-jwt-secret` | （随机生成） | JWT 签名密钥 |
| `--provider-type` | `openai` | LLM provider |
| `--provider-model` | `gpt-4o` | 模型名 |
| `--provider-api-key` | （必填） | LLM API Key |
| `--provider-endpoint` | （默认） | 自定义 API 端点 |

### 环境变量

所有参数也可通过环境变量设置：

```bash
export AIOPS_API_KEY=sk-your-key
export PROVIDER_TYPE=deepseek
export PROVIDER_MODEL=deepseek-chat
export AUTH_DB_PATH=$HOME/.k8ops/k8ops.db
export AUTH_JWT_SECRET=your-secret

./k8ops-manager --leader-elect=false
```

---

## kubeconfig 发现机制

k8ops 使用 controller-runtime 的 `ctrl.GetConfigOrDie()` 自动发现 kubeconfig，查找顺序：

1. `KUBECONFIG` 环境变量
2. `~/.kube/config`（默认路径）
3. In-cluster config（`/var/run/secrets/kubernetes.io/serviceaccount/`）

本地运行时自动使用 `~/.kube/config`，无需额外配置。

### 指定集群

```bash
KUBECONFIG=~/.kube/prod-config ./k8ops-manager --leader-elect=false
```

### 多集群切换

```bash
# 使用 kubectx 切换
kubectx prod-cluster
./k8ops-manager --leader-elect=false
```

---

## 数据流差异

### 集群内运行 vs 本地运行

| 维度 | 集群内 (DaemonSet/Deployment) | 本地运行 |
|------|------|------|
| K8s API 认证 | ServiceAccount token | kubeconfig |
| Host 工具 | `nsenter` 访问宿主机 | 直接在本机执行 |
| Auth 数据 | PVC 持久化 | 本地 SQLite 文件 |
| Leader Election | 多副本需要 | 单实例关闭 |
| RBAC impersonation | 用户 → ServiceAccount | 用户 → kubeconfig 用户 |
| 网络权限 | Pod 网络 | 本机网络 |
| 日志输出 | stdout → kubectl logs | 直接终端输出 |

### Host 工具行为

在容器中，Host 工具通过 `nsenter -m -u -i -n -p --` 访问宿主机命名空间。本地运行时直接通过 `/bin/sh -c` 执行，访问的是本地操作系统。

这意味着：
- `host_disk_check` 检查的是本地磁盘
- `host_process_list` 列出的是本地进程
- `host_exec` 在本地执行命令

---

## 使用 CLI 工具

```bash
# 诊断
./k8ops diagnose --problem "pods in production keep CrashLoopBackOff"

# 查看优化建议
./k8ops optimize --namespace production

# 触发修复
./k8ops remediate --plan <plan-name> --approve
```

---

## 后台常驻运行

### macOS (launchd)

```bash
cat > ~/Library/LaunchAgents/dev.ggai.k8ops.plist << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>dev.ggai.k8ops</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/k8ops-manager</string>
        <string>--leader-elect=false</string>
        <string>--dashboard-address=:9090</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>AIOPS_API_KEY</key>
        <string>your-api-key</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
EOF

launchctl load ~/Library/LaunchAgents/dev.ggai.k8ops.plist
```

### Linux (systemd)

```bash
sudo tee /etc/systemd/system/k8ops.service << 'EOF'
[Unit]
Description=k8ops AI Operations
After=network.target

[Service]
ExecStart=/usr/local/bin/k8ops-manager --leader-elect=false --dashboard-address=:9090
Environment=AIOPS_API_KEY=your-api-key
Environment=AUTH_DB_PATH=/var/lib/k8ops/k8ops.db
Restart=always
User=k8ops

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl enable --now k8ops
```

---

## 开发模式

### 热重载

```bash
# 安装 air
go install github.com/air-verse/air@latest

# 在 k8ops 项目根目录
air --build.cmd "go build ./cmd/manager/" --build.bin "./manager"
```

### 调试

```bash
# 启用 DEBUG 日志
DEBUG=true ./k8ops-manager --leader-elect=false

# 查看 JSON 结构化日志
tail -f /tmp/k8ops.log
```

---

## 故障排查

### “unable to get kubeconfig”

确保 `~/.kube/config` 存在且有效：
```bash
kubectl cluster-info  # 测试 kubeconfig
```

### “address already in use :9090”

```bash
# 查看占用 9090 的进程
lsof -i :9090
# 或换个端口
./k8ops-manager --dashboard-address=:9091
```

### Auth DB 锁定

删除 DB 文件重新初始化：
```bash
rm /tmp/k8ops.db
./k8ops-manager --auth-db-path=/tmp/k8ops.db
```

### Provider 超时

设置更长的超时或检查网络：
```bash
export PROVIDER_ENDPOINT=https://api.openai.com/v1
# 确认网络可达
curl -I https://api.openai.com/v1/models
```
