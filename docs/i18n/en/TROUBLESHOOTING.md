# k8ops 故障排查指南

> 本文汇总 k8ops 常见问题的诊断方法和解决方案。按严重程度分类，便于快速定位。

---

## 目录

1. [安装与启动问题](#1-安装与启动问题)
2. [认证与登录问题](#2-认证与登录问题)
3. [AI 功能问题](#3-ai-功能问题)
4. [Pod 与集群问题](#4-pod-与集群问题)
5. [网络与 Ingress 问题](#5-网络与-ingress-问题)
6. [数据与存储问题](#6-数据与存储问题)
7. [性能问题](#7-性能问题)
8. [监控与告警问题](#8-监控与告警问题)

---

## 1. 安装与启动问题

### 1.1 Pod 一直处于 Pending 状态

**现象：** `kubectl get pods -n k8ops-system` 显示 Pending

**排查步骤：**
```bash
# 查看 Pending 原因
kubectl describe pod -n k8ops-system -l app.kubernetes.io/name=k8ops

# 常见原因：
# - PVC 未绑定（检查 StorageClass）
# - 资源不足（检查节点容量）
# - Node Selector 不匹配
```

**解决方案：**
- **PVC 未绑定：** 检查集群是否有默认 StorageClass
  ```bash
  kubectl get storageclass
  # 如果没有默认 SC，标记一个：
  kubectl patch storageclass local-path -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
  ```
- **资源不足：** 使用 DaemonSet 模式（无 PVC 依赖）
  ```bash
  kubectl apply -k config/daemonset
  ```

### 1.2 Pod CrashLoopBackOff

**现象：** Pod 反复重启

**排查步骤：**
```bash
# 查看容器日志
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --tail=50

# 查看事件
kubectl get events -n k8ops-system --sort-by='.lastTimestamp' | tail -20
```

**常见原因与解决方案：**

| 原因 | 日志特征 | 解决方案 |
|------|----------|----------|
| SQLite 权限问题 | `unable to open database file` | `mkdir -p /data && chown 65532:65532 /data` |
| JWT Secret 缺失 | `JWT secret not configured` | 设置 `AUTH_JWT_SECRET` 环境变量 |
| K8s API 连接失败 | `failed to get Kubernetes config` | 检查 ServiceAccount 和 RBAC |
| 端口冲突 | `bind: address already in use` | 修改 `--dashboard-address` |

### 1.3 镜像拉取失败 (ImagePullBackOff)

**现象：** `Failed to pull image`

**解决方案：**
```bash
# 检查镜像是否可访问
docker pull registry.iot2.win/k8ops:latest

# 如果使用私有仓库，配置 imagePullSecrets
kubectl create secret docker-registry regcred \
  --docker-server=registry.iot2.win \
  --docker-username=<user> \
  --docker-password=<pass> \
  -n k8ops-system

# 或使用 DaemonSet 模式 + hostPath（无需拉取外部镜像）
```

---

## 2. 认证与登录问题

### 2.1 登录返回 401 Unauthorized

**排查：**
```bash
# 检查 auth 配置
kubectl exec -n k8ops-system deploy/k8ops -- /manager --help | grep auth

# 查看 auth 相关日志
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -i auth
```

**解决方案：**
- 确认 `AUTH_JWT_SECRET` 已设置且一致
- 重置管理员密码：
  ```bash
  kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops admin reset-password
  ```
- 默认凭据：`admin` / `changeme`（首次登录后请修改）

### 2.2 OIDC 登录失败

**排查：**
- 确认 OIDC Provider URL 可达（从 Pod 内部）
- 检查 redirect URL 是否匹配 Ingress 域名
- 查看 callback 错误：`kubectl logs ... | grep oidc`

---

## 3. AI 功能问题

### 3.1 Chat 无响应或超时

**现象：** 发送消息后无响应，或返回超时

**排查步骤：**
```bash
# 检查 Provider 配置
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops config show

# 查看 AI 相关日志
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -E "provider|llm|agent"

# 测试 LLM 连通性
kubectl exec -n k8ops-system deploy/k8ops -- curl -s https://api.openai.com/v1/models -H "Authorization: Bearer $AIOPS_API_KEY"
```

**常见原因：**

| 原因 | 日志特征 | 解决方案 |
|------|----------|----------|
| API Key 无效 | `401 Unauthorized` | 更新 `AIOPS_API_KEY` 环境变量 |
| 网络不通 | `context deadline exceeded` | 配置 LLM API egress |
| 模型不存在 | `model not found` | 更新 `--provider-model` |
| 速率限制 | `429 Too Many Requests` | 等待或切换 Provider |
| Circuit Breaker 打开 | `circuit breaker open` | 等待 60s 冷却期 |

### 3.2 AI 诊断不触发

**排查：**
```bash
# 检查事件收集器状态
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -i "collector\|event"

# 确认未被禁用
kubectl get deploy k8ops -n k8ops-system -o jsonpath='{.spec.template.spec.containers[0].command}'
# 不应包含 --disable-event-collector
```

---

## 4. Pod 与集群问题

### 4.1 Dashboard 显示 "kubernetes client not available"

**现象：** API 返回 503，UI 显示连接错误

**原因：** Pod 内 K8s ServiceAccount 权限不足或 config 加载失败

**解决方案：**
```bash
# 重新应用 RBAC
kubectl apply -k config/rbac

# 验证 ServiceAccount
kubectl auth can-i list pods --as=system:serviceaccount:k8ops-system:k8ops -n k8ops-system
```

### 4.2 操作（Scale/Delete/Restart）返回 403 Forbidden

**原因：** 用户 RBAC 角色权限不足

**解决方案：**
```bash
# 检查用户角色
kubectl get rolebindings -n k8ops-system | grep <username>

# 升级为 admin 角色
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops admin set-role <username> admin
```

---

## 5. 网络与 Ingress 问题

### 5.1 Dashboard 无法访问 (502/503)

**排查：**
```bash
# 检查 Service 是否有 Endpoints
kubectl get endpoints -n k8ops-system

# 检查 Ingress 配置
kubectl get ingress -n k8ops-system
kubectl describe ingress -n k8ops-system

# 直接访问 Pod 端口
kubectl port-forward -n k8ops-system deploy/k8ops 9090:9090
# 然后访问 http://localhost:9090
```

### 5.2 Traefik 超时 (499/504)

**现象：** Registry push 或大文件上传超时

**解决方案（Traefik 特有）：**
```bash
# 关闭 Traefik 超时限制
kubectl patch deployment -n kube-system traefik \
  --type='json' \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.readtimeout=0s"}]'

# 或在 IngressRoute 中设置 timeout
```

### 5.3 SSE (Server-Sent Events) 不工作

**现象：** Chat 界面无实时响应

**排查：**
- 检查反向代理是否支持长连接
- Nginx 配置需要：`proxy_buffering off; proxy_cache off;`
- Traefik 不需要额外配置

---

## 6. 数据与存储问题

### 6.1 SQLite 数据库损坏

**现象：** `database disk image is malformed`

**解决方案：**
```bash
# 进入 Pod
kubectl exec -it -n k8ops-system deploy/k8ops -- sh

# 修复数据库（如果 distroless 无 shell，使用 CLI 工具）
# 方案 1：备份重建
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops db backup /data/k8ops-backup.db
kubectl exec -n k8ops-system deploy/k8ops -- rm /data/k8ops.db
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops db restore /data/k8ops-backup.db

# 方案 2：删除 PVC 重建（会丢失用户数据）
kubectl delete pvc -n k8ops-system k8ops-data
kubectl delete pod -n k8ops-system -l app.kubernetes.io/name=k8ops
```

### 6.2 PVC 磁盘空间不足

**排查：**
```bash
kubectl exec -n k8ops-system deploy/k8ops -- df -h /data
# 或通过 Dashboard → Capacity 页面查看
```

**解决方案：**
- 扩容 PVC：
  ```bash
  kubectl patch pvc -n k8ops-system k8ops-data -p '{"spec":{"resources":{"requests":{"storage":"5Gi"}}}}'
  ```
- 清理旧审计日志：
  ```bash
  kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops audit cleanup --max-age 30d
  ```

---

## 7. 性能问题

### 7.1 API 响应慢

**排查：**
```bash
# 检查响应时间（X-Response-Time 头）
curl -sk -o /dev/null -w '%{http_code} %{time_total}s\n' \
  -D - https://k8ops.iot2.win/api/cluster/overview 2>&1 | grep -i "x-response-time"

# 查看 Prometheus 指标
curl -sk https://k8ops.iot2.win/metrics | grep k8ops_http_request_duration
```

**优化方案：**
- API 缓存已启用（overview: 30s, resources: 60s, CRDs: 10min）
- 检查 `k8ops_http_requests_in_flight` 是否过高
- 慢请求日志（>500ms）会自动记录到 Pod 日志

### 7.2 内存使用高

**排查：**
```bash
kubectl top pods -n k8ops-system
```

**优化：**
- 对话内存自动管理：20k token 阈值后自动摘要
- 空闲对话 30min 后清理
- 如持续高内存，考虑重启 Pod（DaemonSet 模式会自动重启）

---

## 8. 监控与告警问题

### 8.1 Prometheus 无法抓取 Metrics

**排查：**
```bash
# 确认 metrics 端点正常
kubectl exec -it <prometheus-pod> -n monitoring -- curl -s http://k8ops.k8ops-system.svc:8080/metrics | head -5

# 检查 ServiceMonitor
kubectl get servicemonitor -n k8ops-system
```

**注意：** `/metrics` 端点仅允许 localhost 访问。Prometheus 需要从集群内（同 Pod 或 Service）抓取。

### 8.2 告警规则不生效

**排查：**
```bash
# 检查 PrometheusRule
kubectl get prometheusrule -n k8ops-system

# 应用告警规则
kubectl apply -f config/alerting-rules.yaml
```

---

## 附录：常用诊断命令

```bash
# 一键状态检查
kubectl get pods -n k8ops-system
kubectl get events -n k8ops-system --sort-by='.lastTimestamp' | tail -20
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --tail=50

# 健康检查
curl -sk https://k8ops.iot2.win/api/health
curl -sk https://k8ops.iot2.win/api/version

# 安全扫描
curl -sk https://k8ops.iot2.win/api/security/audit | jq .summary
curl -sk https://k8ops.iot2.win/api/security/compliance | jq .score

# 容量规划
curl -sk https://k8ops.iot2.win/api/capacity/planning | jq .summary
```

## 附录：日志级别

k8ops 使用结构化 JSON 日志 (slog)，支持以下级别：

| 级别 | 用途 | 示例 |
|------|------|------|
| `INFO` | 正常操作 | 服务器启动、认证成功 |
| `WARN` | 慢请求、配置问题 | 请求 >500ms、PVC 接近满 |
| `ERROR` | 操作失败 | K8s API 错误、LLM 调用失败 |

通过 Request ID 关联日志：
```bash
# 获取 Request ID（从 HTTP 响应头 X-Request-ID）
# 然后在日志中搜索
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep "a1b2c3d4"
```

---

*最后更新: 2026-07-03*
*维护者: k8ops Team*
