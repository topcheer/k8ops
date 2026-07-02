# k8ops 运行手册 (Runbook)

> 本文档面向运维人员，涵盖日常运维操作、故障处理流程、紧急联系人和标准操作规程。

---

## 目录

1. [服务概述](#1-服务概述)
2. [日常运维](#2-日常运维)
3. [故障处理](#3-故障处理)
4. [紧急操作](#4-紧急操作)
5. [备份与恢复](#5-备份与恢复)
6. [性能调优](#6-性能调优)
7. [紧急联系人](#7-紧急联系人)

---

## 1. 服务概述

### 架构简介

```
┌─────────────────────────────────────────────────┐
│                   用户浏览器                      │
│              https://k8ops.iot2.win               │
└───────────────────┬─────────────────────────────┘
                    │ HTTPS (Traefik Ingress)
┌───────────────────▼─────────────────────────────┐
│              Traefik (kube-system)               │
│         websecure: 8443 → 8000                   │
└───────────────────┬─────────────────────────────┘
                    │
┌───────────────────▼─────────────────────────────┐
│            k8ops DaemonSet (k8ops-system)         │
│  ┌─────────────────────────────────────────────┐ │
│  │  Go Binary (嵌入前端静态资源)                  │ │
│  │  :8080 Dashboard                             │ │
│  │  /metrics  Prometheus                        │ │
│  │  /api/chat  SSE → LLM Provider               │ │
│  │  nsenter → host kubectl                      │ │
│  └─────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────┘
```

### 关键组件

| 组件 | 位置 | 作用 |
|------|------|------|
| k8ops DaemonSet | k8ops-system | 主服务，每个节点一个 Pod |
| Traefik | kube-system | Ingress 控制器，TLS 终结 |
| Registry | registry.iot2.win | 私有镜像仓库 |
| LLM Provider | 外部 API | AI Chat / 诊断 / 优化引擎 |

### 健康检查端点

| 端点 | 期望响应 | 说明 |
|------|---------|------|
| `https://k8ops.iot2.win/` | 200/303 | 前端页面 |
| `https://k8ops.iot2.win/readyz` | 200 | K8s 就绪探针 |
| `https://k8ops.iot2.win/api/version` | 200 JSON | 版本信息 |
| `https://k8ops.iot2.win/metrics` | 200 (仅本地) | Prometheus 指标 |

---

## 2. 日常运维

### 2.1 查看服务状态

```bash
# Pod 状态
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops

# 服务日志（最近 100 行）
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --tail=100

# 版本信息
curl -sk https://k8ops.iot2.win/api/version | jq .

# 集群概览
curl -sk https://k8ops.iot2.win/api/cluster/overview | jq .
```

### 2.2 更新部署

```bash
# 构建新版本
cd /Volumes/new/ggai/k8ops
VERSION=v14XX
docker buildx build --platform linux/amd64 \
  --build-arg VERSION=$VERSION \
  -t registry.iot2.win/k8ops:$VERSION \
  -t registry.iot2.win/k8ops:latest \
  --push .

# 滚动更新
kubectl set image daemonset/k8ops \
  k8ops=registry.iot2.win/k8ops:$VERSION -n k8ops-system

# 验证
sleep 15
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops
curl -sk -o /dev/null -w '%{http_code}' https://k8ops.iot2.win/
```

### 2.3 日志管理

k8ops 使用 `log/slog` 结构化日志，日志级别通过环境变量 `LOG_LEVEL` 控制：

| 级别 | 用途 |
|------|------|
| `DEBUG` | 开发调试，输出所有日志 |
| `INFO` (默认) | 生产运行，记录关键操作 |
| `WARN` | 仅警告和错误 |

```bash
# 修改日志级别
kubectl set env daemonset/k8ops LOG_LEVEL=DEBUG -n k8ops-system
```

### 2.4 Provider 配置

AI 功能需要配置 LLM Provider：

1. 访问 Settings → Provider 配置页面
2. 选择 Provider（OpenAI / Zhipu / DeepSeek 等）
3. 输入 API Key
4. 测试连接

如未配置，Dashboard 会显示 Provider 未配置警告横幅。

---

## 3. 故障处理

### 3.1 Pod 无法启动 (CrashLoopBackOff)

**症状**: k8ops Pod 反复重启

**排查步骤**:
```bash
# 1. 查看 Pod 事件
kubectl describe pod -n k8ops-system -l app.kubernetes.io/name=k8ops

# 2. 查看容器日志
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --previous

# 3. 检查 RBAC 权限
kubectl auth can-i --list --as=system:serviceaccount:k8ops-system:k8ops

# 4. 检查 ConfigMap/Secret 挂载
kubectl exec -n k8ops-system -it deploy/k8ops -- ls -la /etc/k8ops/
```

**常见原因**:
- RBAC 权限不足 → 检查 `config/rbac/`
- kubeconfig 无效 → 检查挂载的 kubeconfig
- 端口冲突 → 检查 8080 端口是否被占用
- 内存不足 → 检查节点资源 `kubectl describe nodes`

### 3.2 Dashboard 无法访问 (502/503)

**症状**: https://k8ops.iot2.win 返回 502 或 503

**排查步骤**:
```bash
# 1. 检查 Ingress
kubectl get ingress -A | grep k8ops

# 2. 检查 Traefik
kubectl get pods -n kube-system -l app.kubernetes.io/name=traefik
kubectl logs -n kube-system -l app.kubernetes.io/name=traefik --tail=50

# 3. 检查 k8ops Service
kubectl get svc -n k8ops-system
kubectl get endpoints -n k8ops-system

# 4. 直接测试 Pod
kubectl exec -n k8ops-system -it deploy/k8ops -- curl -s localhost:8080/api/version
```

**常见原因**:
- Traefik 未正确路由 → 检查 Ingress 规则
- k8ops 未就绪 → 检查 readyz 探针
- TLS 证书过期 → 检查 cert-manager

### 3.3 AI Chat 无响应

**症状**: Chat 发送消息后无回复或超时

**排查步骤**:
```bash
# 1. 检查 Provider 状态
curl -sk https://k8ops.iot2.win/api/provider/status | jq .

# 2. 查看引擎日志
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -i 'llm\|provider\|chat'

# 3. 测试 Provider 连接
curl -sk https://k8ops.iot2.win/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"message":"hello","conversationId":"test"}' --max-time 30
```

**常见原因**:
- API Key 未配置或已过期
- Provider API 限流 (429)
- 网络不通 (DNS/防火墙)
- Token 超限 → Agent 自动压缩上下文，但极端情况可能失败

### 3.4 Registry 推送失败 (499)

**症状**: `docker push` 返回 499 Client Closed Request

**解决方案**:
```bash
# 检查 Traefik 超时配置
kubectl get deploy -n kube-system traefik -o jsonpath='{.spec.template.spec.containers[0].args}'

# 如缺少超时参数，添加：
kubectl patch deploy -n kube-system traefik --type='json' -p='[
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.readtimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.writetimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.idletimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.keepalivemaxtime=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.keepalivemaxrequests=0"}
]'
```

### 3.5 写操作失败 (Scale/Delete/Restart)

**症状**: 点击 Scale/Delete/Restart 按钮后操作失败

**排查步骤**:
```bash
# 检查 RBAC 权限
kubectl auth can-i patch deployments --as=system:serviceaccount:k8ops-system:k8ops -n default
kubectl auth can-i delete pods --as=system:serviceaccount:k8ops-system:k8ops -n default

# 查看审计日志
curl -sk https://k8ops.iot2.win/api/audit?severity=critical | jq .

# 检查安全策略
kubectl get psp,podsecurity --all-namespaces 2>/dev/null
```

---

## 4. 紧急操作

### 4.1 快速回滚

```bash
# 查看历史版本
kubectl rollout history daemonset/k8ops -n k8ops-system

# 回滚到上一版本
kubectl rollout undo daemonset/k8ops -n k8ops-system

# 回滚到指定版本
kubectl rollout undo daemonset/k8ops -n k8ops-system --to-revision=3
```

### 4.2 紧急缩容（保留 0 副本）

```bash
# 注意：DaemonSet 不支持 scale 0，需要直接删除
kubectl delete pod -n k8ops-system -l app.kubernetes.io/name=k8ops --grace-period=0 --force

# 如需完全停止，临时修改 nodeSelector
kubectl patch daemonset k8ops -n k8ops-system -p='{"spec":{"template":{"spec":{"nodeSelector":{"non-existent":"true"}}}}}'
```

### 4.3 清理数据

```bash
# 清理诊断历史 CRD
kubectl delete diagnostics --all --all-namespaces

# 清理审计日志（保留最近 7 天）
kubectl get auditlogs -A -o json | jq '.items[] | select(.metadata.creationTimestamp < "'$(date -d '7 days ago' -Iseconds)'")' | kubectl delete -f -

# 清理优化报告
kubectl delete optimizations --all --all-namespaces
```

---

## 5. 备份与恢复

### 5.1 配置备份

```bash
# 备份 k8ops 配置
kubectl get cm,secret,daemonset -n k8ops-system -o yaml > k8ops-backup-$(date +%Y%m%d).yaml

# 备份 CRD 数据
kubectl get diagnostics,remediations,optimizations -A -o yaml > k8ops-crd-backup-$(date +%Y%m%d).yaml

# 备份 RBAC
kubectl get clusterrole,clusterrolebinding -o yaml | grep -A5 k8ops > k8ops-rbac-backup-$(date +%Y%m%d).yaml
```

### 5.2 恢复流程

```bash
# 恢复配置
kubectl apply -f k8ops-backup-YYYYMMDD.yaml

# 恢复 CRD 数据
kubectl apply -f k8ops-crd-backup-YYYYMMDD.yaml

# 验证
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops
curl -sk https://k8ops.iot2.win/api/version | jq .
```

### 5.3 定期备份建议

使用 Velero 或 cron job 每日备份：
```bash
# Velero 备份（推荐）
velero backup create k8ops-daily-$(date +%Y%m%d) \
  --include-namespaces k8ops-system \
  --include-cluster-resources=true
```

---

## 6. 性能调优

### 6.1 关键指标

| 指标 | Prometheus Metric | 告警阈值 |
|------|-------------------|---------|
| API 延迟 | `k8ops_tool_call_duration_seconds` | P99 > 10s |
| LLM 调用延迟 | `k8ops_llm_call_duration_seconds` | P99 > 60s |
| 活跃诊断数 | `k8ops_active_diagnostics` | > 10 |
| 安全拦截 | `k8ops_safety_blocks_total` | rate > 10/min |
| Token 消耗 | `k8ops_llm_tokens_total` | 日消耗异常增长 |
| 集群健康分 | `k8ops_cluster_health_score` | < 60 |

### 6.2 资源建议

| 节点规模 | k8ops 资源 Request | 资源 Limit |
|---------|-------------------|-----------|
| ≤ 5 节点 | 100m CPU / 128Mi | 500m CPU / 512Mi |
| 5-20 节点 | 200m CPU / 256Mi | 1 CPU / 1Gi |
| 20-50 节点 | 500m CPU / 512Mi | 2 CPU / 2Gi |

### 6.3 日志级别优化

生产环境建议保持 `INFO` 级别。仅在排查问题时临时切换为 `DEBUG`：
```bash
# 临时开启 DEBUG
kubectl set env daemonset/k8ops LOG_LEVEL=DEBUG -n k8ops-system
# 排查后恢复
kubectl set env daemonset/k8ops LOG_LEVEL=INFO -n k8ops-system
```

---

## 7. 紧急联系人

### 7.1 升级流程

```
故障发现 → 值班运维 (L1)
    ├── 5分钟内未解决 → 运维负责人 (L2)
    │     ├── 15分钟内未解决 → 架构师 (L3)
    │     │     ├── 影响生产 → CTO 通报
```

### 7.2 联系人表

> 根据实际情况填写

| 角色 | 姓名 | 电话 | 职责范围 |
|------|------|------|---------|
| L1 值班运维 | ____ | ____ | 首响应，基础故障处理 |
| L2 运维负责人 | ____ | ____ | 复杂故障，影响多个服务 |
| L3 架构师 | ____ | ____ | 架构级问题，数据恢复 |
| 集群管理员 | ____ | ____ | K8s 集群本身故障 |
| 网络/安全 | ____ | ____ | 网络策略，证书，安全事件 |

### 7.3 供应商联系

| 供应商 | 用途 | 联系方式 |
|--------|------|---------|
| LLM Provider | AI Chat/诊断 | ____ |
| Registry | 镜像仓库 | ____ |
| DNS/CDN | 域名解析 | ____ |

---

## 附录: Prometheus 指标列表

k8ops 暴露以下自定义指标（`/metrics` 端点）：

| Metric | 类型 | 标签 | 说明 |
|--------|------|------|------|
| `k8ops_diagnostics_total` | Counter | phase, trigger | 诊断报告总数 |
| `k8ops_remediation_actions_total` | Counter | type, result, risk | 修复操作总数 |
| `k8ops_llm_call_duration_seconds` | Histogram | provider, model, status | LLM 调用延迟 |
| `k8ops_llm_tokens_total` | Counter | provider, model, type | Token 消耗 |
| `k8ops_agent_steps` | Histogram | - | Agent 执行步数 |
| `k8ops_tool_call_duration_seconds` | Histogram | tool, success | 工具调用延迟 |
| `k8ops_safety_blocks_total` | Counter | reason | 安全拦截次数 |
| `k8ops_active_diagnostics` | Gauge | - | 当前活跃诊断数 |
| `k8ops_active_remediations` | Gauge | - | 当前执行中的修复 |
| `k8ops_audit_events_total` | Counter | type, severity | 审计事件总数 |
| `k8ops_cluster_health_score` | Gauge | - | 集群健康分 (0-100) |
| `k8ops_conversation_count` | Gauge | - | 活跃对话数 |
| `k8ops_tool_executions_total` | Counter | tool, success | 工具执行总数 |

---

*最后更新: 2026-07-02*
*维护者: k8ops Team*
