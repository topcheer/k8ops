# k8ops — Kubernetes AI 运维 Operator

<div align="center">

**一个使用 AI 诊断问题、自动修复并优化集群的 Kubernetes AIOps Operator。**

[![GitHub release](https://img.shields.io/github/v/release/topcheer/k8ops?style=flat-square)](https://github.com/topcheer/k8ops/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/topcheer/k8ops/ci.yml?branch=main&style=flat-square&label=CI)](https://github.com/topcheer/k8ops/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/topcheer/k8ops?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![Docker](https://img.shields.io/badge/Docker-ghcr.io-2496ED?style=flat-square&logo=docker)](https://github.com/topcheer/k8ops/pkgs/container/k8ops)
[![Built with ggcode](https://img.shields.io/badge/Built%20with-ggcode-6C43BC?style=flat-square)](https://github.com/topcheer/ggcode)

</div>

---

**语言：** [English](../../README.md) | [中文](README.md) | [日本語](../ja/README.md) | [한국어](../ko/README.md) | [Español](../es/README.md) | [Français](../fr/README.md) | [Deutsch](../de/README.md)

---

## 功能特性

### AI 驱动运维
- **智能诊断** — 提交问题描述，获取基于工具增强推理（kubectl describe、日志、事件、指标）的 AI 根因分析
- **自动修复** — AI 提出并（经审批后）执行安全的修复操作：重启 Pod、扩缩容、清理资源
- **优化建议** — 持续分析资源使用、HPA/PDB 缺口和成本优化机会
- **流式对话** — 实时 SSE 流式传输，支持思维块、工具调用透明度和差异对比结果渲染

### 企业级安全
- **多供应商认证** — 本地（bcrypt）、LDAP（可配置 TLS 验证）、OIDC（GitHub、Google、GitLab、Keycloak、Okta、Auth0、Microsoft）
- **RBAC** — 基于角色的访问控制，支持 admin/operator/viewer 角色和命名空间范围权限
- **OIDC CSRF 防护** — 每供应商状态 Cookie，使用 `ConstantTimeCompare` 验证
- **CORS 白名单** — 基于来源的白名单（凭证模式下不允许通配符），`Vary: Origin` 头
- **审计日志** — 每个 AI 操作、工具执行和 LLM 调用都记录为结构化审计事件
- **JWT 持久化** — 签名的 JWT 密钥存储在 K8s Secret 中，支持可选回退
- **速率限制** — 登录端点上的令牌桶速率限制器，防止暴力破解
- **安全头** — X-Content-Type-Options、X-Frame-Options、HSTS、CSP

### 运维与可靠性
- **优雅关闭** — SIGTERM/SIGINT 处理，包括 SSE 排空、SQLite WAL 刷新和控制器停止
- **会话 TTL** — 自动清理空闲聊天会话（30 分钟超时，最多 1000 个会话）
- **熔断器** — 可配置重试、退避和熔断的弹性 LLM 调用
- **Prometheus 指标** — 集群健康指标、会话计数器、工具执行指标

### 部署
- **Kustomize** — 基础 + 覆盖层部署，提供生产就绪默认值
- **嵌入式 Web UI** — 单一二进制文件，无外部前端依赖
- **SQLite + K8s CRD** — 轻量级持久化，无需外部数据库
- **PVC 持久化** — 数据在 Pod 重启后依然存在

---

## 架构

```
┌─────────────────────────────────────────────────────────┐
│                    仪表板 / Web UI                        │
│  (嵌入式 SPA + REST API + SSE 流式传输)                    │
├─────────────────────────────────────────────────────────┤
│            认证 (本地/LDAP/OIDC) + RBAC                   │
├─────────────────────────────────────────────────────────┤
│                      AI Agent                            │
│  (LLM 推理 + 工具调用 + 流式传输)                          │
├──────────┬──────────┬──────────┬────────────────────────┤
│  聊天    │  安全    │  审计    │  弹性                   │
│  引擎    │  检查器  │  记录器  │  (熔断器)               │
├──────────┴──────────┴──────────┴────────────────────────┤
│                    工具注册表                             │
│  (kubectl get/describe/logs, exec, events, metrics)      │
├─────────────────────────────────────────────────────────┤
│              控制器运行时 + CRD                           │
│  (DiagnosticReport, RemediationPlan, OptimizationSuggestion) │
├─────────────────────────────────────────────────────────┤
│                   Kubernetes API                         │
│  (身份模拟：用户范围 RBAC)                                │
└─────────────────────────────────────────────────────────┘
```

详见 [架构文档](../../ARCHITECTURE.md)。

---

## 快速开始

### 前置条件
- Kubernetes 1.24+（k3s / k8s / EKS / GKE / AKS）
- 已配置 kubectl
- LLM API 密钥（OpenAI、DeepSeek、ZAI 或任何 OpenAI 兼容供应商）

### 1. 部署到 Kubernetes

**选项 A：Deployment 模式（推荐）**

```bash
# 一条命令 — 包括命名空间、RBAC、密钥、Ingress、TLS
kubectl apply -k config/deploy/overlays/local

# 或创建自己的覆盖层
cp -r config/deploy/overlays/local config/deploy/overlays/myorg
# 编辑 myorg/kustomization.yaml：设置您的域名、镜像仓库、CORS
kubectl apply -k config/deploy/overlays/myorg
```

**选项 B：DaemonSet 模式（每节点诊断）**

```bash
kubectl apply -f config/daemonset-local.yaml
```

**选项 C：install.sh（交互式）**

```bash
./install.sh install    # 部署
./install.sh status     # 检查状态
./install.sh uninstall  # 卸载
```

详见 [部署指南](../../DEPLOYMENT.md)。

### 2. 配置 LLM 供应商

```bash
# 通过仪表板：设置选项卡 → 填写供应商类型、API 密钥、模型
# 或通过覆盖层 ConfigMap 中的环境变量：

configMapGenerator:
- name: k8ops-config
  literals:
  - PROVIDER_TYPE=openai
  - PROVIDER_MODEL=gpt-4o
  - PROVIDER_ENDPOINT=https://api.openai.com/v1

# API 密钥通过 Secret：
secretGenerator:
- name: k8ops-credentials
  literals:
  - api-key=sk-your-key-here
```

### 3. 访问仪表板

```bash
# 通过 Ingress（如果已配置）
# 打开 https://<您的域名>  (例如 https://k8ops.iot2.win)

# 或端口转发
kubectl port-forward svc/k8ops-dashboard 9090:9090 -n k8ops-system
# 打开 http://localhost:9090
# 默认登录：admin / admin（会提示修改密码）
```

### 4. 触发诊断

```bash
# 通过 kubectl (CRD)
kubectl apply -f examples/diagnostic.yaml

# 通过 CLI
go run ./cmd/k8ops diagnose --problem "production 中的 Pod 持续 CrashLoopBackOff"

# 通过 Web 仪表板聊天界面
```

---

## 配置

所有配置通过 ConfigMap/Secret（由 Kustomize 覆盖层管理）。详见 [config/deploy/overlays/local/kustomization.yaml](config/deploy/overlays/local/kustomization.yaml)。

### 核心
| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PROVIDER_TYPE` | `openai` | LLM 供应商类型 |
| `PROVIDER_MODEL` | `gpt-4o` | 模型名称 |
| `PROVIDER_ENDPOINT` | `https://api.openai.com/v1` | LLM 供应商基础 URL |
| `AIOPS_API_KEY` | （必填）| LLM API 密钥（来自 Secret）|

### 安全
| 变量 | 默认值 | 说明 |
|------|--------|------|
| `AUTH_JWT_SECRET` | （自动生成）| JWT 签名密钥（持久化在 K8s Secret 中）|
| `CORS_ALLOWED_ORIGINS` | （空）| 逗号分隔的允许来源 |
| `LDAP_SERVER` | （空）| LDAP 服务器 URL |
| `LDAP_SKIP_TLS_VERIFY` | `false` | 跳过 LDAP TLS 证书验证 |
| `OIDC_ISSUER` | （空）| OIDC 签发者 URL |

### 通知
| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SLACK_WEBHOOK_URL` | （空）| Slack 通知 Webhook |

### AI / 聊天
| 变量 | 默认值 | 说明 |
|------|--------|------|
| `MAX_STEPS` | `15` | 每次请求最大 Agent 推理步数 |
| `CONVERSATION_TTL` | `30m` | 空闲会话超时 |
| `MAX_CONVERSATIONS` | `1000` | 最大并发会话数 |

---

## API

仪表板在 `http://<host>:9090/api/` 暴露 REST API。主要端点：

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/health` | 健康检查 | 公开 |
| GET | `/api/version` | 构建版本 | 公开 |
| GET | `/api/cluster/overview` | 集群摘要 | Viewer+ |
| GET | `/api/cluster/nodes` | 节点列表 + 健康 | Viewer+ |
| GET | `/api/cluster/pods` | Pod 列表及状态 | Viewer+ |
| POST | `/api/chat/stream` | AI 聊天（SSE 流式）| Viewer+ |
| GET | `/api/resources/{type}` | K8s 资源查询 | Viewer+ |
| POST | `/api/auth/login` | 本地/LDAP 登录 | 公开 |
| GET | `/api/auth/status` | 认证配置 + 供应商 | 公开 |
| GET | `/api/auth/providers` | 列出认证供应商 | Admin |
| GET/POST | `/api/rbac/users` | 用户管理 | Admin |
| GET/POST | `/api/rbac/roles` | 角色管理 | Admin |

完整 API 参考详见 [API 文档](../../API.md)。

---

## 开发

### 前置条件
- Go 1.22+
- kubectl（用于集成测试）
- 可访问的 Kubernetes 集群（用于控制器测试）

### 构建与测试

```bash
# 构建 manager 二进制文件
make build

# 运行所有测试
make test

# 使用竞态检测器运行测试
go test -race -count=1 ./internal/...

# 生成 CRD
make manifests

# 构建 Docker 镜像
make docker-build IMG=ghcr.io/topcheer/k8ops:latest
```

---

## 本地开发

直接在工作站上运行 k8ops，无需 Kubernetes 部署：

```bash
# 构建
go build -o k8ops-manager ./cmd/manager/

# 运行
AIOPS_API_KEY=your-key ./k8ops-manager \
  --leader-elect=false \
  --dashboard-address=:9090 \
  --auth-db-path=/tmp/k8ops.db
```

二进制文件会自动发现 kubeconfig（`~/.kube/config`），因此所有 K8s 数据来自您连接的集群。详见 [本地运行文档](../../LOCAL_RUN.md)。

---

## 文档

| 文档 | 说明 |
|------|------|
| [用户手册](../../USER_GUIDE.md) | 全面的用户指南（所有功能）|
| [架构文档](../../ARCHITECTURE.md) | 系统架构和组件设计 |
| [部署指南](../../DEPLOYMENT.md) | 部署指南（Deployment / DaemonSet / Helm）|
| [本地运行](../../LOCAL_RUN.md) | 在本地运行 k8ops 二进制（无需 K8s 部署）|
| [API 文档](../../API.md) | REST API 参考 |
| [安全文档](../../SECURITY.md) | 安全策略和 RBAC 模型 |
| [变更日志](../../CHANGELOG.md) | 发布历史 |

---

## 许可证

GNU Affero General Public License v3.0 (AGPL-3.0)。详见 [LICENSE](LICENSE)。
