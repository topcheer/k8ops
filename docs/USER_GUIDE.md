# k8ops 用户手册

> 从安装到精通，覆盖所有功能的详细使用指南。

---

## 目录

1. [快速入门](#1-快速入门)
2. [集群总览](#2-集群总览)
3. [AI Chat — 智能助手](#3-ai-chat--智能助手)
4. [诊断与修复](#4-诊断与修复)
5. [优化建议](#5-优化建议)
6. [成本分析 (FinOps)](#6-成本分析-finops)
7. [集群拓扑可视化](#7-集群拓扑可视化)
8. [节点与 Pod 管理](#8-节点与-pod-管理)
9. [事件流与通知](#9-事件流与通知)
10. [资源浏览器与 YAML 编辑器](#10-资源浏览器与-yaml-编辑器)
11. [RBAC 访问控制](#11-rbac-访问控制)
12. [审计日志](#12-审计日志)
13. [设置与配置](#13-设置与配置)
14. [键盘快捷键](#14-键盘快捷键)
15. [主题切换](#15-主题切换)
16. [容量规划](#16-容量规划)
17. [HPA 可视化](#17-hpa-可视化)
18. [容器镜像清单](#18-容器镜像清单)
19. [命名空间资源排行](#19-命名空间资源排行)
20. [安全合规](#20-安全合规)
21. [系统管理](#21-系统管理)
22. [运维诊断 API](#22-运维诊断-apiv1461)

---

## 1. 快速入门

### 首次登录

1. 打开浏览器访问 k8ops 地址（如 `https://k8ops.iot2.win` 或 `http://localhost:9090`）
2. 默认账号：`admin` / `admin`
3. 首次登录会提示修改密码

### 页面布局

```
┌─────────┬───────────────────────────────┐
│         │  [Namespace ▼]  [🔔]  [☀/☽]  │  ← 顶栏
│ Sidebar ├───────────────────────────────┤
│         │                                │
│ Overview│       Content Area             │  ← 内容区
│ Diagnose│                                │
│ Nodes   │                                │
│ Pods    │                                │
│ ...     │                                │
└─────────┴───────────────────────────────┘
```

### Ctrl+K 命令面板

随时按 `Ctrl+K`（Mac: `Cmd+K`）打开全局命令面板：

- 输入 `nodes` → 跳转到节点页
- 输入 `chat` → 打开 AI Chat
- 输入 `cost` → 查看成本分析
- 方向键选择，Enter 确认，Esc 关闭

---

## 2. 集群总览

Overview 页面展示集群整体状态。

### 统计卡片

| 卡片 | 含义 |
|------|------|
| Nodes | 集群节点总数 / Ready 数 |
| Pods | 运行中 Pod 数 / 总数 |
| CPU | 集群整体 CPU 利用率 |
| Memory | 集群整体内存利用率 |
| Warnings | 当前 Warning 事件数 |

### Sparkline 趋势图

每个卡片下方有 SVG 迷你折线图，展示最近 30 分钟的趋势变化。

### Namespace 切换

顶栏左侧的下拉选择器可以切换 namespace scope。切换后影响 Pods、Events、Nodes 等页面。选择会持久化到 localStorage。

---

## 3. AI Chat — 智能助手

点击侧边栏底部的 Chat 按钮或 `Ctrl+K` 输入 `chat` 打开。

### 基本用法

在输入框输入问题，AI 会：

1. 理解自然语言意图
2. 自动调用合适的 K8s 工具
3. 流式返回分析结果

### 示例查询

```
# 查看资源
查看 default 命名空间的 pod
有哪些节点 CPU 使用率高？

# 故障诊断
为什么 nginx-deployment 的 pod 在 CrashLoopBackOff？
集群有什么异常？

# 优化建议
帮我分析资源使用情况
有哪些 pod 可以缩小副本数？
```

### 工具调用透明度

AI 执行工具调用时，会显示折叠式的 Thinking 面板：

- 点击展开可以看到每个工具调用的参数和返回结果
- JSON 格式化展示，支持搜索

### 诊断建议卡片

当 AI 建议执行 kubectl 命令时，代码块下方会显示：

- **▶ Run in Chat** — 将命令载入输入框，方便发送执行
- **📋 Copy** — 复制命令到剪贴板

### 会话管理

- **New** — 新建会话
- **左侧会话列表** — 点击切换历史会话
- 会话自动总结压缩（超过 20k token 时自动触发）

### Markdown 渲染

Chat 支持：
- 代码块（带语法高亮和复制按钮）
- 表格
- 列表、粗体、斜体
- 链接（仅 http/https/mailto 协议）

---

## 4. 诊断与修复

### 触发诊断

**方式一：Web 界面**

1. 进入 Diagnostics 页面
2. 点击 "New Diagnostic"
3. 填写问题描述（如 "production 命名空间的 API 响应变慢"）
4. 提交后 AI 自动分析

**方式二：AI Chat**

在 Chat 中直接描述问题，AI 会自动执行诊断流程。

**方式三：CRD**

```bash
kubectl apply -f - <<EOF
apiVersion: aiops.ggai.dev/v1alpha1
kind: DiagnosticReport
metadata:
  name: check-nginx
  namespace: k8ops-system
spec:
  problem: "nginx pods keep restarting"
EOF
```

**方式四：CLI**

```bash
k8ops diagnose --problem "pods in production keep CrashLoopBackOff"
```

### 诊断结果

每份诊断报告包含：

- **Root Cause** — AI 分析的根本原因
- **Evidence** — 支持分析的日志、事件、指标数据
- **Recommendations** — 建议的修复操作
- **Severity** — 严重等级（Info / Warning / Critical）

### 自动修复 (Remediation)

AI 生成的修复计划需要人工审批：

1. 进入 Remediations 页面
2. 查看待审批的修复计划
3. 点击 **Approve** 执行，或 **Reject** 拒绝
4. 所有操作记录在审计日志中

---

## 5. 优化建议

Optimizations 页面展示 AI 对集群资源的优化建议。

### 建议类型

| 类型 | 说明 |
|------|------|
| Resource Rightsizing | CPU/Memory requests 和 limits 建议调整 |
| HPA Gap | 缺少水平自动扩缩配置的 Deployment |
| PDB Gap | 缺少 PodDisruptionBudget 的工作负载 |
| Cost Saving | 可节省的成本（闲置资源、过剩副本等） |

### 操作

- 点击建议查看详情
- 可直接 Apply 或忽略

---

## 6. 成本分析 (FinOps)

Cost 页面提供集群成本可见性。

### 功能

- **命名空间成本汇总** — 按 namespace 展示资源消耗和预估成本
- **资源利用率** — CPU/Memory 实际使用 vs 分配
- **Rightsizing 建议** — 过度分配的资源调整建议
- **闲置资源** — 长期未使用的 PV、LoadBalancer、弹性 IP 等

---

## 7. 集群拓扑可视化

Topology 页面以 SVG 图形展示节点和 Pod 的关系。

### 视觉元素

| 元素 | 含义 |
|------|------|
| 绿色框 | Ready 节点 |
| 红色框 | NotReady 节点 |
| 节点框内进度条 | CPU（上）/ MEM（下）利用率 |
| Pod 绿色圆点 | Running |
| Pod 黄色圆点 | Pending |
| Pod 红色圆点 | Failed |
| Pod 闪烁边框 | CrashLoop（restarts > 3） |

### 交互

- **点击 Pod** — 打开该 Pod 的日志查看器
- **底部统计** — Ready/NotReady 节点数、Pod 状态汇总

---

## 8. 节点与 Pod 管理

### Nodes 页面

- 节点列表表格：名称、角色、状态、CPU、内存、Pod 数量
- 每列支持搜索过滤
- 点击节点名查看详细信息和该节点上的所有 Pod

### Pods 页面

- Pod 列表表格：名称、命名空间、状态、重启次数、节点、年龄
- 支持命名空间过滤和实时搜索

### Pod 日志查看器

点击 Pod 行打开日志查看器：

- **实时流式** — SSE 推送，日志实时更新
- **日志级别高亮** — ERROR（红）、WARN（黄）、DEBUG（灰）
- **搜索过滤** — 输入关键词过滤日志行
- **自动滚动** — 新日志到达时自动滚到底部（可暂停）
- **下载** — 导出当前日志为文件

---

## 9. 事件流与通知

### Events 页面

展示 K8s 集群事件，支持：

- 实时搜索过滤
- Warning 事件红色高亮
- 按命名空间过滤

### 实时事件流

Events 页面右侧有 Live Events 面板：

- 点击 **Go Live** 开启 SSE 实时推送
- 新事件带蓝色 NEW 徽章动画
- 删除事件带红色 DEL 徽章
- Warning 事件自动红色高亮

### 通知中心

顶栏右侧的铃铛图标：

- 有告警时显示红色数字徽章 + 脉冲动画
- 点击展开下拉面板
- 展示最近的 Warning 事件和 NotReady 节点
- 60 秒自动刷新

---

## 10. 资源浏览器与 YAML 编辑器

### Resources 页面

浏览集群中的所有 K8s 资源：

- 按 API Group / Resource Type 分组
- 点击资源名查看 YAML 定义
- 支持命名空间多选过滤

### YAML 查看器

点击任意资源可打开 YAML overlay：

- 格式化展示完整 YAML
- **Copy** 按钮一键复制

### YAML 编辑器

在 YAML 查看器中点击 **Edit** 按钮进入编辑模式：

1. YAML 内容变为可编辑 textarea
2. 修改后点击 **Apply** 提交
3. 后端使用 server-side apply（kubectl apply 语义）
4. 成功显示绿色提示，失败显示红色错误信息

---

## 11. RBAC 访问控制

RBAC 页面（需要 admin 权限）管理用户和角色。

### 用户管理

- **创建用户** — 用户名、密码、角色、命名空间 scope
- **编辑用户** — 修改角色、启用/禁用
- **删除用户**

### 角色

| 角色 | 权限 |
|------|------|
| admin | 全集群读写，可管理用户 |
| operator | 大部分资源读写，不能管理 RBAC/Secrets |
| viewer | 只读访问 |

### 命名空间 Scope

每个用户可以绑定特定的命名空间，只能访问该范围内的资源（通过 K8s impersonation 实现）。

---

## 12. 审计日志

Audit 页面展示所有 AI 操作的审计记录。

### 功能

- **Severity 过滤** — 下拉选择 Info / Warning / Error / Critical
- **实时搜索** — 输入关键词过滤
- **统计卡片** — Total / Successful / Failed / Critical / Warnings
- **表格** — 时间、严重级别、动作、目标资源、操作者、成功/失败、耗时

### 审计范围

所有以下操作都会被记录：

- AI 工具调用（kubectl get/describe/logs 等）
- AI 发起的修复操作
- LLM API 调用
- 用户登录/登出
- 资源修改

---

## 13. 设置与配置

Settings 页面配置 AI Provider 和认证。

### AI Provider 配置

| 字段 | 说明 |
|------|------|
| Provider Type | openai / deepseek / zai / anthropic |
| Model | gpt-4o / deepseek-chat / glm-4-plus 等 |
| Endpoint | LLM API 地址（留空使用默认） |
| API Key | LLM API 密钥 |

### 认证配置

- **Local** — 内置用户系统（默认）
- **LDAP** — 企业 LDAP/AD 集成
- **OIDC** — GitHub / Google / Keycloak 等

---

## 14. 键盘快捷键

| 快捷键 | 功能 |
|--------|------|
| `Ctrl+K` / `Cmd+K` | 打开命令面板 |
| `Esc` | 关闭命令面板 / 弹窗 |
| `↓` / `↑` | 命令面板中选择 |
| `Enter` | 命令面板中确认 |

---

## 15. 主题切换

点击侧边栏右上角的月亮/太阳按钮切换暗色/亮色主题。选择会持久化到 localStorage，刷新页面后保持。

---

## 附录

### 相关文档

- [架构设计](ARCHITECTURE.md)
- [部署指南](DEPLOYMENT.md)
- [本地运行](LOCAL_RUN.md)
- [API 参考](API.md)
- [安全策略](SECURITY.md)

### 常见问题

**Q: Chat 没有响应？**
A: 检查 Settings → Provider 配置是否正确，API Key 是否有效。

**Q: 看不到某些命名空间？**
A: 当前用户的 RBAC 角色可能限制了命名空间访问范围，联系管理员调整。

**Q: Pod 日志查看器空白？**
A: Pod 可能刚启动还没有日志，或者没有日志权限。检查 RBAC 配置。

**Q: AI 建议的命令安全吗？**
A: 所有 AI 建议的操作都会先通过 Safety Checker 的 dry-run 验证，修复操作需要人工审批才能执行。

---

## 16. 容量规划

### 存储容量监控

**路径：** Dashboard → Capacity 标签页

展示集群中所有 PVC（PersistentVolumeClaim）的存储状态：

| 指标 | 说明 |
|------|------|
| Total PVCs | 集群中 PVC 总数 |
| Bound | 已绑定 PV 的 PVC 数 |
| Pending | 等待绑定的 PVC |
| Total Capacity | 所有 PVC 的总容量 |
| Requested | 所有 PVC 请求的总量 |

### 节点容量分析

Capacity 页面同时展示每个节点的资源利用率：

- **CPU 利用率**：已请求 CPU / 可分配 CPU（颜色编码：<60% 绿色，60-80% 黄色，>80% 红色）
- **内存利用率**：已请求内存 / 可分配内存
- **Pod 密度**：已运行 Pod 数 / 最大 Pod 数限制
- **扩容建议**：当节点资源超过 80% 时自动生成扩容建议

### 集群级汇总

| 指标 | 说明 |
|------|------|
| Cluster CPU Utilization | 全集群 CPU 请求/可分配比率 |
| Cluster Mem Utilization | 全集群内存请求/可分配比率 |
| Total CPU Allocatable | 全集群可分配 CPU 总量 |
| Total CPU Requested | 全集群已请求 CPU 总量 |

---

## 17. HPA 可视化

**路径：** Dashboard → HPA 标签页

展示所有 HorizontalPodAutoscaler 的自动扩缩状态：

### 功能

- **副本缩放条**：可视化当前副本数、期望副本数、最小/最大范围
- **指标利用率条**：CPU/内存当前利用率 vs 目标值（绿/黄/红）
- **扩缩状态标识**：当当前副本 ≠ 期望副本时显示 "SCALING" 徽章
- **摘要卡片**：HPA 总数、正在扩缩数、当前/期望副本总数

### 支持的指标类型

| 类型 | 说明 |
|------|------|
| Resource | CPU/内存利用率百分比 |
| Pods | 自定义 Pod 指标（如 QPS） |
| External | 外部指标（如 SQS 队列长度） |
| ContainerResource | 容器级资源指标 |

---

## 18. 容器镜像清单

**路径：** Dashboard → Images 标签页

展示集群中所有正在使用的容器镜像：

| 指标 | 说明 |
|------|------|
| Unique Images | 去重后的镜像总数 |
| Using :latest | 使用 `:latest` 标签的镜像数（不推荐用于生产） |
| No Limits | 没有设置资源 limits 的镜像数 |
| No Requests | 没有设置资源 requests 的镜像数 |
| Registries | 使用的镜像仓库数量 |

### 安全最佳实践

- 避免使用 `:latest` 标签 — 使用固定版本号确保可重现部署
- 所有容器应设置 CPU/内存 limits — 防止资源耗尽
- 所有容器应设置 CPU/内存 requests — 确保调度器正确分配

---

## 19. 命名空间资源排行

**路径：** Dashboard → Namespaces 标签页

按 CPU 消耗排序列出所有命名空间的资源使用情况：

### 功能

- **资源汇总**：每个 namespace 的 CPU/内存 requests + limits、Pod 数、PVC 存储量
- **集群占比**：CPU/内存请求占集群可分配总量的百分比（带可视化进度条）
- **搜索过滤**：快速定位特定 namespace
- **详情钻取**：点击任意 namespace 查看 ResourceQuota 使用情况、LimitRange 配置、近期警告事件

---

## 20. 安全合规

### CIS Benchmark 合规扫描

**路径：** Dashboard → Compliance 标签页

运行 CIS Kubernetes Benchmark 检查，覆盖以下类别：

| 类别 | 检查项 |
|------|--------|
| RBAC | cluster-admin 绑定范围、通配符 ClusterRole、默认 SA 使用 |
| Pod Security | 特权容器、hostNetwork/hostPID/hostIPC、hostPath 卷、root 用户、资源 limits |
| Network | NetworkPolicy 覆盖率 |
| Secrets | Secret 管理健康度 |

### 合规报告下载

点击 "Download Report" 按钮可下载完整合规报告（.txt 格式），包含：

- 合规分数（百分比）
- 每项检查的状态（PASS/WARN/FAIL）
- 修复建议（针对 WARN/FAIL 项）

### 审计事件搜索

**路径：** API → `GET /api/audit/events`

支持多维度过滤审计日志：

| 参数 | 说明 |
|------|------|
| `actor` | 按用户名过滤 |
| `action` | 按操作类型过滤（如 delete, scale, exec） |
| `q` | 全文搜索 |
| `severity` | 按严重级别过滤 |
| `from`/`to` | 时间范围（RFC3339 格式） |

### CSV 导出

`GET /api/audit/export` — 导出审计日志为 CSV 格式，可导入 SIEM 系统进行合规分析。

---

## 21. 系统管理

### 系统信息

`GET /api/system/info` 提供运行时信息：

- 版本号、Go 版本、运行平台
- 内存使用（Alloc/Sys/GC cycles/Heap objects）
- Goroutine 数量
- 服务运行时间
- 审计日志大小和事件数

### 日志管理

| API | 功能 |
|-----|------|
| `POST /api/system/log/rotate` | 手动触发审计日志轮转（admin） |
| `POST /api/system/log/cleanup` | 清理 30 天以上的轮转文件（admin） |

### 日志级别配置

通过环境变量 `LOG_LEVEL` 配置（debug/info/warn/error）：

```bash
kubectl set env daemonset/k8ops -n k8ops-system LOG_LEVEL=debug
kubectl rollout restart daemonset/k8ops -n k8ops-system
```

### 备份管理

| API | 功能 |
|-----|------|
| `GET /api/system/backup` | 列出所有备份文件 |
| `POST /api/system/backup` | 创建数据库备份 |
| `DELETE /api/system/backup?name=X` | 删除指定备份 |
| `POST /api/system/backup/restore?name=X` | 从备份恢复数据库 |

### API 性能监控

`GET /api/system/performance` 提供每个 API 端点的延迟统计：

- **p50/p95/p99** 百分位延迟
- 平均和最大延迟
- 错误率和请求总数

---

## 22. 运维诊断 API（v14.61+）

### Network Policy 审计

`GET /api/security/network-policies` 审计集群的 NetworkPolicy 覆盖情况：

- 检测无 NetworkPolicy 的命名空间（默认全开放）
- 识别宽松策略（0.0.0.0/0 入站/出站）
- 按严重级别分类：critical / warning / info
- 每个发现包含详细描述和修复建议

### Pod 重启诊断

`GET /api/diagnostics/restarts` 诊断 Pod 重启模式和根因：

- 分类重启模式：crash-loop / occasional / post-deploy
- 提取终止原因：OOMKilled / Error / 退出码
- 识别等待状态：CrashLoopBackOff / ImagePullBackOff
- 每个容器独立的诊断信息

### 部署 Rollout 状态

`GET /api/deployments/rollout` 追踪所有工作负载的 rollout 健康状态：

- 覆盖 Deployment / StatefulSet / DaemonSet
- 7 种状态：complete / in-progress / stalled / degraded / paused / failed / scaled-to-zero
- 检测 ProgressDeadlineExceeded 和 ReplicaFailure
- 支持按状态过滤：`?status=failed`

### 资源浪费检测

`GET /api/resources/waste` 扫描浪费和孤立资源以降低成本：

- 6 类浪费：死服务、未用 PVC、孤立 ConfigMap/Secret、空命名空间、未绑定 PV
- 成本风险评估：low / moderate / high
- 每项包含严重度、年龄、清理建议
- 智能过滤系统资源（kube-system、SA token、Helm release）

### 扩展瓶颈检测

`GET /api/scaling/bottlenecks` 识别限制水平扩展的因素：

- 7 类瓶颈：节点调度、节点压力、配额限制、HPA 卡住、PDB 阻塞、存储耗尽
- 集群容量摘要：节点数、CPU/内存、Pod 容量、扩展余量
- 每项包含影响级别和修复建议

### RBAC 权限风险分析

`GET /api/security/rbac-risk` 分析集群中所有 RBAC 绑定的安全风险：

- 0-100 评分系统，自动识别高危绑定
- 5 级风险等级：critical / high / elevated / moderate / low
- 检测项：cluster-admin 绑定、权限提升（escalate/bind/impersonate）、通配符权限（verbs/resources: *）、集群范围写操作、敏感资源访问（secrets/pods/exec）
- 每项包含详细评分明细和修复建议（最小权限原则）
- 支持按命名空间过滤：`?namespace=default`

### CronJob 执行健康监控

`GET /api/operations/cronjobs/health` 监控所有 CronJob 的执行健康状况：

- 5 级健康状态：healthy / warning / failing / suspended / no-runs
- 检测连续失败（3 次以上 = failing）、成功率低于 50%、暂停的调度、从未执行
- 通过 OwnerReferences 关联 CronJob 及其子 Job
- 计算下次预期运行时间
- 支持按命名空间过滤：`?namespace=production`

### Service & Endpoint 网络健康监控

`GET /api/networking/health` 扫描所有 Service 和 Ingress 的网络连通性：

- 5 级 Service 健康状态：healthy / degraded / no-endpoints / misconfigured / external
- 检测选择器不匹配（label mismatch）、所有端点不可用、部分降级、LoadBalancer 等待 IP
- Ingress 后端验证：后端 Service 是否存在、是否有可用端点
- 交叉引用 Pod 选择器匹配，提供根因分析
- 支持按命名空间过滤：`?namespace=default`

### PV/PVC 存储健康监控

`GET /api/storage/health` 扫描所有 PVC/PV 的存储健康状况：

- 6 级 PVC 健康状态：bound / pending / lost / failed / orphaned / near-capacity
- Pending 诊断：无存储类、WaitForFirstConsumer 绑定模式、provisioner 日志检查
- 孤立 PVC 检测：已绑定但超过 1 天无 Pod 使用（容量浪费）
- PV 问题：Released（需手动清理）、Failed（回收失败）、陈旧 Available（>7 天）
- 存储类分布：默认类标记、provisioner、reclaim policy、volume expansion 支持
- 支持按命名空间过滤：`?namespace=default`

### ServiceAccount 安全审计

`GET /api/security/service-accounts` 全面审计集群中所有 ServiceAccount 的安全风险：

- 0-100 风险评分系统，自动识别高危 SA
- 5 级严重度：critical / high / elevated / moderate / low
- 检测项：未使用 SA（>7 天）、cluster-admin 绑定（critical）、默认 SA 被 Pod 使用、不必要的 token 自动挂载、陈旧 SA（>30 天有权限但无使用）、遗留长效 token secret
- 每项包含详细安全风险说明和修复建议
- 支持按命名空间过滤：`?namespace=default`

### SLO/SLA 错误预算追踪

`GET /api/operations/slo` 基于多窗口多燃烧率算法追踪 SLO/SLA 达标情况：

- 5 个时间窗口：5 分钟、1 小时、6 小时、24 小时、7 天
- 可用性百分比和错误预算剩余量/消耗率
- 多窗口燃烧率检测（fast: 5m+1h，slow: 6h+24h）
- P50/P95/P99 延迟百分位及 SLO 目标
- 3 级状态：meeting（达标）/ at-risk（风险）/ violated（违反）
- 支持按命名空间过滤：`?namespace=production`

### ResourceQuota 与 LimitRange 监控

`GET /api/resources/quota` 扫描所有命名空间的配额利用率和 LimitRange 约束：

- 4 级配额状态：ok（<70%）/ warning（70-85%）/ critical（85-100%）/ exceeded（>100%）
- 每命名空间的 CPU/内存/Pod/ConfigMap/Secret/存储配额利用率
- 识别无配额保护的命名空间
- LimitRange 默认/最小/最大约束分析
- Top 消费者排名
- 支持按命名空间过滤：`?namespace=default`

### 部署配置审计

`GET /api/deployments/audit` 审计所有工作负载的配置最佳实践违规：

- 8 个检查类别：revision-history / image-policy / resources / probes / security-context / update-strategy / lifecycle / config-drift
- 每项包含严重度（critical/warning/info）、具体问题描述和可操作修复建议
- 健康评分 0（完美）到 100（最差）
- 聚合 Top Findings 显示全集群最常见问题
- 支持按命名空间和严重度过滤：`?namespace=default&severity=critical`

### 调度健康与资源碎片分析

`GET /api/scheduling/health` 分析集群调度健康和资源利用率：

- 每节点可调度性（隔离/taint/压力条件）和资源可用量
- Pending Pod 诊断：解析 FailedScheduling 事件原因（CPU/内存不足、taint 不匹配、nodeSelector 冲突、卷绑定失败等）
- 最大可调度 Pod 计算（当前能部署多大的 Pod）
- 有效容量 vs 理论容量（不可调度节点导致的容量损失）
- 资源碎片化分析（散落的空闲容量）
- 超大 Pod 检测（请求超过任何单节点容量）
- 24h 驱逐历史（含原因）
- 健康评分 0-100（加权惩罚）
- 可操作的修复建议
- 支持按命名空间过滤：`?namespace=default`

### Pod 安全态势扫描

`GET /api/security/pods?namespace=xxx&severity=critical` 审计所有运行中 Pod 的实时安全态势：

- 15 个检查类别覆盖特权容器、主机访问（network/PID/IPC）、HostPath 挂载、危险 capabilities、root 运行、提权等
- 每 Pod 风险评分 0-100（critical=25分/warning=8分/info=2分）
- 按检查类型和命名空间聚合统计
- 支持按命名空间和严重度过滤

### 事件风暴与级联故障检测

`GET /api/operations/event-storm?namespace=xxx` 分析集群 Warning 事件：

- 4 级风暴严重度：critical (>50) / high (>20) / medium (>10) / low (>5)
- 抖动资源检测（同资源同原因重复 3+ 次，含抖动频率）
- 按命名空间和事件原因聚合
- 爆炸半径评估（受影响资源数）
- 可操作的排查建议
- 支持按命名空间过滤：`?namespace=kube-system`

### 资源依赖图与影响范围分析

`GET /api/dependencies?kind=Deployment&name=xxx&namespace=xxx` 追踪工作负载的完整依赖图：

- 正向依赖：ConfigMap、Secret、PVC、ServiceAccount
- 反向依赖：Service（label selector）、Ingress、NetworkPolicy、HPA、共享配置的其他 Pod
- 影响范围评估：blastRadius 评分和风险等级
- 用于变更前影响评估，避免级联故障

### 拓扑分布合规检查

`GET /api/topology/spread?namespace=xxx&domain=topology.kubernetes.io/zone` 检查 Pod 的拓扑分布合规性：

- 4 级工作负载状态：balanced / skewed / no-constraint / single-replica
- 每工作负载的拓扑域分布和偏差分析
- 检测缺少拓扑约束的多副本工作负载
- 识别缺少拓扑标签的节点
- 单域集群提示
- 支持按命名空间和拓扑域 key 过滤

### Secret 轮转与生命周期审计

`GET /api/security/secrets/rotation?namespace=xxx` 审计所有 Secret 的生命周期：

- 年龄追踪：stale (>90d) / very stale (>180d)
- 未使用 Secret 检测（不被任何 Pod 引用）
- TLS 证书过期检测（解析证书，检测已过期和 <30d 过期）
- Docker registry Secret、遗留 SA token 追踪
- 敏感名称检测（password/key/token/credential）
- 每 Secret 风险等级、集群轮转评分 0-100
- 支持按命名空间过滤

### 健康探针有效性审计

`GET /api/operations/probes?namespace=xxx` 审计探针配置：

- 8 个检查类别：缺少探针、过于激进、超时过短、阈值不当等
- 每工作负载风险评分，集群有效性评分 (0-100)
- 聚合 Top 问题统计
- 可操作建议

### 工作负载陈旧度追踪

`GET /api/product/staleness?namespace=xxx` 追踪部署陈旧度：

- 5 级陈旧度分类：fresh/recent/stale/very-stale/ancient
- 镜像标签分析：:latest、digest、no-tag
- 年龄分布桶、命名空间统计
- 集群新鲜度评分 (0-100)

### 资源超卖与压力分析

`GET /api/scalability/overcommit?namespace=xxx` 分析资源超卖：

- 每节点 CPU/内存 request 和 limit 超卖比率
- 压力评分 0-100 和风险等级
- 无 limits/requests 的 Pod 检测
- 集群安全评分 0-100
- 命名空间资源消耗明细

### 镜像安全与供应链分析

`GET /api/security/images?namespace=xxx` 扫描所有容器镜像的供应链安全：

- Digest 锁定检测（@sha256: 不可变引用）
- :latest 标签检测（可变，不可复现）
- 无标签镜像检测（默认 :latest）
- 旧版本标签检测（v1, 1.0 — 可能含已知 CVE）
- 公共 vs 私有镜像仓库分析
- 每镜像风险等级、每仓库统计
- 集群镜像安全评分 0-100

### 容量规划

`GET /api/capacity/planning` 节点容量规划：

- 每节点 CPU/内存请求 vs 可分配量
- 剩余容量和扩容建议
- 资源碎片化检测

### 容量预测

`GET /api/capacity/forecast` 容量趋势预测：

- 基于历史数据的资源增长趋势
- 预计耗尽时间
- 扩容建议

### 资源效率分析

`GET /api/efficiency` 资源使用效率：

- 过大资源分配检测
- 资源浪费识别
- 优化建议

### PDB 状态

`GET /api/pdbs` Pod Disruption Budget 状态：

- PDB 配置检查
- 允许中断数 vs 当前可用数
- PDB 阻塞检测

### 版本兼容性

`GET /api/compatibility` K8s 版本兼容性：

- API 弃用检查
- 资源版本兼容性
- 升级影响评估

### 证书过期

`GET /api/certificates/expiry` TLS 证书过期扫描：

- 集群证书过期时间
- 即将过期证书警告
- 续期建议

### Addon 健康

`GET /api/addons/health` 集群 addon 健康检查：

- 核心 addon 运行状态
- 异常 addon 检测
- 修复建议

### 集群健康评分

`GET /api/operations/health-score` 聚合所有集群健康信号为一个综合评分：

- 5 个加权维度：Node(25%) + Pod(25%) + Workload(20%) + Events(15%) + API Server(15%)
- 总分 0-100，字母评级 A-F
- 状态：healthy / warning / critical
- 每维度评分、权重、详情
- 集群摘要：节点/Pod/工作负载计数
- 按严重度排序的 Top 问题

### HPA/VPA 资源合理配置建议

`GET /api/scalability/autoscale-recommendations?namespace=xxx` 分析自动缩放和资源右-sizing：

- 检测缺少 HPA 的多副本工作负载
- CPU 请求过高 (>1 core/container)
- 内存请求过高 (>2GB/container)
- HPA 效率分析（达到上限/下限/闲置）
- 每工作负载当前 vs 建议资源值
- 潜在 CPU 核心和内存节省
- 集群自动缩放评分 0-100

### Ingress 与流量路由健康监控

`GET /api/product/ingress-health?namespace=xxx` 检查所有 Ingress 的流量路由健康：

- 后端服务存在性和端点就绪检查
- TLS 配置检测
- IngressClass 有效性验证
- host+path 冲突检测
- 无路由规则检测
- 每 Ingress 状态和集群健康评分 0-100

### 节点条件与资源压力

`GET /api/operations/node-pressure` 分析所有节点的条件和资源压力：

- DiskPressure / MemoryPressure / PIDPressure / NetworkUnavailable 检测
- CPU/内存/Pod 使用率 vs 可分配量
- 每节点风险等级 (critical/high/medium/low)
- 集群压力评分 0-100

### PVC 绑定与存储性能

`GET /api/scalability/pvc-analysis?namespace=xxx` 分析存储绑定健康：

- Stuck PVC 根因检测（>5min pending）
- 绑定时间测量和慢绑定检测（>30s）
- Lost PVC 检测
- 每 StorageClass 统计和供应器分析
- 集群存储健康评分 0-100

### Namespace 治理与生命周期

`GET /api/product/namespaces/lifecycle` 审计命名空间治理：

- ResourceQuota / LimitRange / NetworkPolicy 覆盖率
- 专用 ServiceAccount 检测（最小权限）
- 必需标签检查（app, team, env, owner）
- 命名空间生命周期（active / stale / terminating）
- 集群治理评分 0-100

### RBAC 有效权限与提权分析

`GET /api/security/rbac-effective` 分析所有主体的 RBAC 有效权限：

- 聚合 ClusterRoleBindings + RoleBindings 计算实际权限
- cluster-admin 等效检测
- 提权路径检测（可修改 RBAC 的主体）
- 通配符 (*) 权限检测
- Secret 读取和 Pod exec 访问分析
- 集群 RBAC 安全评分 0-100

### 容器 OOM Kill 追踪

`GET /api/operations/oom-tracker?namespace=xxx` 追踪容器 OOM 事件：

- OOMKilled 容器检测和根因分析
- 高重启次数检测 (>=5)
- 缺失/过低内存限制检测
- 限制远大于请求 (10x+) 的节点压力风险
- Top OOM 排名和每命名空间统计
- 集群 OOM 风险评分 0-100

### 存储容量耗尽预测

`GET /api/scalability/storage-forecast` 预测存储容量：

- 每 PV 使用率、增长率、耗尽天数预测
- Longhorn actual-size 注解支持
- 集群存满天数估算
- 每 StorageClass 统计和供应器分析
- 高风险命名空间排名
- 存储健康评分 0-100

### DNS 解析健康检查

`GET /api/product/dns-health` 分析 DNS 解析健康：

- CoreDNS Pod 健康检查（运行/就绪/重启/版本）
- Corefile 配置分析（forwarders, plugins）
- Headless Service 端点覆盖和 NXDOMAIN 风险
- NodeLocal DNS 缓存检测
- Pod dnsConfig ndots 覆盖检测
- External-DNS 托管服务发现
- 集群 DNS 健康评分 0-100

### 多信号事件关联与根因建议引擎

`GET /api/operations/incident-correlation` 是 AIOps 核心功能，将分散的告警信号智能聚合为可操作的事件：

- **信号收集**：集群告警事件（Warning Events）、Pod 生命周期（CrashLoopBackOff、OOMKilled、高频重启）、节点压力状况
- **关联引擎**：Union-Find 算法，5 分钟时间窗口 + 命名空间/节点关联
- **每个事件集群**：严重性分级、根因猜测（置信度 0-100%）、爆炸半径、时间线重建
- **参数**：`namespace=` 过滤，`window=` 时间窗口（默认 60 分钟，最大 360 分钟）

### 集群级服务依赖拓扑与级联故障风险

`GET /api/product/service-topology` 自动发现集群中所有服务间依赖关系：

- **依赖发现**：扫描工作负载环境变量中的 Kubernetes DNS 引用（svc.ns.svc）
- **关键枢纽识别**：高扇入服务（多个工作负载依赖它）
- **单点故障检测**：无 HA 的关键服务
- **孤儿服务检测**：有 selector 但无匹配工作负载的 Service
- **跨命名空间依赖追踪**

### 混沌工程就绪度评估

`GET /api/deployment/chaos-readiness` 评估每个工作负载对混沌实验的承受能力：

- **六大评估标准**：多副本 HA、PDB 覆盖、健康探针、优雅关闭、反亲和性、多可用区分布
- **就绪分级**：ready（≥70 分）/ partial（40-69）/ fragile（<40）
- **实验推荐**：为就绪工作负载推荐安全的混沌实验（pod-kill、network-latency）

### 集群碳足迹与可持续性度量

`GET /api/scalability/carbon-footprint` 估算集群能耗和碳排放：

- **区域检测**：从节点元数据自动检测云区域，映射到电网碳强度（25+ 区域）
- **碳归因**：按命名空间和工作负载分摊碳排放
- **减碳机会**：资源整合、工作负载右 sizing、绿色时段调度、区域迁移
- **绿色评分**：0-100 可持续发展评分

### 准入控制策略差距审计

`GET /api/security/admission-policy-audit` 审计集群准入控制安全态势：

- **Webhook 健康检查**：failurePolicy、sideEffects、超时配置
- **策略引擎检测**：OPA/Gatekeeper 和 Kyverno 自动发现
- **覆盖率分析**：按资源类型计算准入保护覆盖率
- **CEL 策略推荐**：推荐使用 K8s 1.30+ ValidatingAdmissionPolicies 替代重量级 webhook

### Pod 性能异常与嘈杂邻居检测

`GET /api/operations/pod-anomaly` 通过统计方法自动发现异常 Pod：

- **异常值检测**：副本间重启次数方差分析（3+ 标准差）
- **嘈杂邻居识别**：共享节点上的干扰模式检测
- **节点热点**：异常 Pod 浓度分析（>30% 异常率）
- **年龄归一化严重性**：按运行时间计算重启率/小时

### 集群外部暴露面风险地图

`GET /api/product/exposure-map` 全面映射集群的外部攻击面：

- **入口点追踪**：Ingress/LB/NodePort/ExternalIP → 后端工作负载
- **风险评估**：TLS 缺失、认证注解、敏感路径检测（/admin、/debug）
- **孤儿端点检测**：Service 无后端 Pod

### 工作负载扩缩容影响模拟器

`GET /api/scalability/scale-simulator?workload=X&namespace=Y&replicas=N` What-If 分析：

- **节点容量**：集群级 CPU/内存容量 vs 模拟使用量
- **命名空间配额**：ResourceQuota CPU/内存/Pod 限制
- **HPA 对齐**：目标 vs minReplicas/maxReplicas
- **结论**：can-scale / risky / cannot-scale

### 回滚风险与修订完整性评估

`GET /api/deployment/rollback-risk` 评估每个工作负载的回滚就绪度：

- **修订历史**：revisionHistoryLimit=0 = 无法回滚（critical）
- **镜像稳定性**：`:latest` 标签 = 回滚不可预测
- **配置依赖漂移**：ConfigMap/Secret 可能已变更
- **副本数与成熟度**：单副本 = 停机风险，新创建 = 验证不足

### RBAC 权限图与提权路径分析

`GET /api/security/rbac-graph` 构建集群级 RBAC 权限图：

- **危险角色分类**：cluster-admin 等价（wildcard）、提权动词（escalate/bind）、pods/exec、secret 访问
- **提权路径发现**：低权限主体如何获得更高权限（exec 窃取 SA token、secret 提取 token）
- **过度授权识别**：SA 拥有 cluster-admin、通配符权限

### GitOps/CD 管道健康审计

`GET /api/deployment/gitops-audit` 检测和审计所有 GitOps/CD 工具：

- **工具发现**：ArgoCD、Flux、Argo Rollouts，含健康状态和版本
- **Helm 清单**：从 Helm v3 secrets 提取 release 信息
- **GitOps 采用率**：带有 GitOps 注解的工作负载百分比
- **配置漂移指标**：被管理但存在手动更改的工作负载

### SOC2/PCI-DSS/HIPAA 合规框架映射

`GET /api/security/compliance-map` 将集群配置映射到三大合规框架：

- **SOC2 Type II**（7 项控制）：访问控制、监控、变更管理
- **PCI-DSS 4.0**（7 项控制）：数据保护、网络安全、漏洞管理
- **HIPAA**（4 项控制）：访问控制、审计、完整性、传输安全
- 每项控制返回 pass/fail 状态和具体修复建议

### Metrics 管道完整性审计

`GET /api/operations/metrics-pipeline-audit` 评估 metrics 管道完整性：

- **组件发现**（11 种）：Prometheus、VictoriaMetrics、node-exporter、Grafana、Alertmanager 等
- **五维评分卡**：采集 / 存储 / 可视化 / 告警 / 覆盖率（0-100）
- **缺口检测**：按严重性分级，附带修复建议

### 节点升级就绪度审计

`GET /api/scalability/node-upgrade-audit` 评估集群升级就绪度：

- **版本偏斜检测**：K8s 偏斜策略检查（最大 +2/-1）
- **升级阻断检测**：节点压力、PDB 覆盖不足、弃用 API
- **升级影响评估**：受影响工作负载数、需重新调度的 Pod 数
- **就绪评分**：0-100 基于阻断因素和偏斜程度

### 集群预测性健康引擎

`GET /api/operations/predictive-health` 预测集群未来 30 天的潜在风险：

- **节点风险预测**：基于节点条件（MemoryPressure、DiskPressure、PIDPressure）计算 0-100 风险评分
- **Pod 风险分类**：识别重启循环（OOMKill 前兆）、资源饥饿（无 limit）、驱逐风险（低 QoS）
- **资源趋势分析**：Pod 密度趋势、容量消耗速率、预测资源耗尽时间
- **证书过期预测**：TLS Secret 年龄分析，提醒续期管道检查
- **风险时间线**：按 ETA 分桶（<24h、1-7d、7-30d、>30d）展示未来风险事件
- **置信度评分**：基于数据完整性计算预测置信度（50-100）

```bash
# 获取集群预测性健康报告
curl -sk https://k8ops.iot2.win/api/operations/predictive-health \
  -H "Authorization: Bearer $JWT" | jq '.overallRiskLevel, .predictions'
```

### 部署变更就绪门禁

`GET /api/deployment/change-readiness` 在部署前评估集群是否处于安全状态，适合 CI/CD 管道集成：

**8 项预检**：
1. **节点稳定性**：无 MemoryPressure/DiskPressure/PIDPressure
2. **活跃滚动更新**：并发 rollout < 3
3. **失败 Pod 检测**：CrashLoopBackOff/ImagePullBackOff/高重启 < 10
4. **PDB 覆盖率**：PDB 覆盖 > 50% 的工作负载
5. **容量余量**：Pod 容量利用率 < 85%
6. **回滚路径**：RevisionHistoryLimit > 0 的部署占比
7. **资源限制**：容器设置了 CPU/Memory limit
8. **健康探针**：容器配置了 readiness probe

**Gate 决策**：
- `proceed`：所有检查通过，可以安全部署
- `proceed-with-caution`：有警告但不阻断，建议小批量部署
- `blocked`：存在阻断因素，必须先修复

```bash
# CI/CD 管道中作为部署门禁
RESULT=$(curl -sk https://k8ops.iot2.win/api/deployment/change-readiness \
  -H "Authorization: Bearer $JWT" | jq -r '.gateDecision')

if [ "$RESULT" = "blocked" ]; then
  echo "Deployment blocked by readiness gate"
  exit 1
fi
```

### 资源请求智能分析

`GET /api/scalability/request-intelligence` 通过多信号代理分析资源请求合理性，提供量化的 Right-Sizing 建议：

**分析引擎**：
- **过度供给检测**：稳定运行的工作负载使用"圆数"请求（1000m/2Gi 等）→ 可能浪费 30%
- **供给不足检测**：OOMKill/重启循环信号 → 故障风险
- **无请求检测**：工作负载未设置任何资源请求 → 调度器盲区

**输出**：
- **分类判定**：over-provisioned / under-provisioned / optimal / no-requests
- **具体建议**：CPU/Memory 推荐值（基于 0.7x 缩减或 1.5x 增加）
- **节省估算**：月度成本节省（$30/核/月, $4/GB/月云定价）、可减少节点数
- **风险评估**：OOM 预测、CPU 节流预测、高/中/低风险分级
- **Posture Score**：0-100 资源请求健康度评分

```bash
# 获取资源请求智能分析
curl -sk https://k8ops.iot2.win/api/scalability/request-intelligence \
  -H "Authorization: Bearer $JWT" | jq '.postureScore, .savingsEstimate, .underProvisioned'
```

### 服务可靠性评分卡

`GET /api/product/reliability-scorecard` 为每个工作负载生成 A-F 可靠性等级：

**7 个评分维度**：
1. **副本高可用**：>=3 副本得满分，单副本不及格
2. **健康探针**：readiness + liveness + startup 配置
3. **资源管理**：CPU/Memory requests + limits
4. **PDB 覆盖**：PodDisruptionBudget 保护
5. **安全上下文**：runAsNonRoot + readOnlyRootFilesystem
6. **更新策略**：RollingUpdate vs Recreate
7. **反亲和性**：pod anti-affinity / topology spread

**等级标准**：A(>=90) B(80-89) C(70-79) D(60-69) F(<60)

```bash
curl -sk https://k8ops.iot2.win/api/product/reliability-scorecard \
  -H "Authorization: Bearer $JWT" | jq '.clusterGrade, .distribution, .weakestSignals'
```

### 安全态势评分卡

`GET /api/security/posture-scorecard` 评估集群整体安全态势，生成 A-F 评级：

**5 个评估维度**：
1. **Pod 安全**：特权容器、root 运行检测
2. **主机访问**：hostNetwork/hostPID/hostIPC/hostPath 暴露
3. **网络隔离**：NetworkPolicy 覆盖率
4. **资源边界**：容器 limit 覆盖
5. **攻击面**：危险能力(cap)、SA Token、无限制出口流量

**输出**：集群评级(A-F)、高风险工作负载列表、攻击面量化、修复建议

```bash
curl -sk https://k8ops.iot2.win/api/security/posture-scorecard \
  -H "Authorization: Bearer $JWT" | jq '.clusterGrade, .dimensions, .highRiskWorkloads'
```

---

### 成本智能分析与支出预测引擎

**端点**：`GET /api/scalability/cost-intelligence`

超越静态成本快照的高级 FinOps 智能层。提供成本趋势分析、异常检测、支出预测和 FinOps 成熟度评分。

**核心能力**：
1. **命名空间成本趋势**：按命名空间分析月度成本、支出速率（increasing/stable）、风险等级
2. **成本异常检测**：
   - 集中度异常（单命名空间占 >40% 总支出）
   - 过度请求（limit/request 比率 >5x）
   - 闲置浪费（大量低请求 Pod）
   - 失控增长（快速资源增长模式）
3. **支出预测**：基于当前分配模式预测月度支出、增长率（0-20%）、推荐预算（含 10% 缓冲）
4. **优化机会排名**：按预估年节省金额排序的 Top 15 优化建议
   - right-size-cpu（CPU 右调）
   - remove-idle（清理闲置 Pod）
   - consolidate（Pod 合并）
   - spot-migrate（Spot 实例迁移）
5. **FinOps 成熟度评分卡**（A-F 等级），5 个维度：
   - **可见性**：资源请求覆盖率
   - **优化**：过度配置/闲置资源比例
   - **预算**：命名空间预算标注覆盖率
   - **效率**：limit/request 比率合理性
   - **分配**：团队/部门标签覆盖率

**示例**：
```bash
# 查看成本智能报告
curl -sk https://k8ops.iot2.win/api/scalability/cost-intelligence \
  -H "Authorization: Bearer $JWT" | jq '{
    monthlySpend: .summary.monthlySpend,
    annualProjection: .summary.annualProjection,
    forecast: .forecast,
    finOpsGrade: .finOpsScore.grade,
    finOpsScore: .finOpsScore.score,
    topSavings: .topOpportunities[0:3],
    anomalies: [.anomalies[] | select(.severity == "critical")]
  }'

# 查看 FinOps 成熟度详情
curl -sk https://k8ops.iot2.win/api/scalability/cost-intelligence \
  -H "Authorization: Bearer $JWT" | jq '.finOpsScore'

# 查看成本最高的命名空间
curl -sk https://k8ops.iot2.win/api/scalability/cost-intelligence \
  -H "Authorization: Bearer $JWT" | jq '.byNamespace[0:5]'
```

---

### SRE 四大黄金信号统一健康引擎

**端点**：`GET /api/product/golden-signals`

将 SRE 四大黄金信号（Latency 延迟、Traffic 流量、Errors 错误、Saturation 饱和度）综合为统一的健康视图。

**四大信号**：
1. **延迟**：未就绪容器数量、Pod 启动时间（>2min 为慢启动）
2. **流量**：Running Pod 数量、端点就绪率、节点容量
3. **错误**：CrashLoopBackOff、高重启 Pod（>5次）、OOMKill、警告事件
4. **饱和度**：节点压力（磁盘/内存/PID）、Pending Pod、无 limit 的 Pod

**评分原则**：木桶效应（最弱信号决定总体评分）

**复合故障检测**：
- 静默故障（高延迟+高错误 = Pod 快速启动但持续崩溃）
- 级联故障风险（高饱和+高错误 = 集群可能正在级联故障）
- 服务能力不足（低流量+健康延迟 = 就绪探针配置错误）
- 命名空间热点（多信号同时降级）

**示例**：
```bash
# 查看黄金信号总览
curl -sk https://k8ops.iot2.win/api/product/golden-signals \
  -H "Authorization: Bearer $JWT" | jq '{
    overallScore: .overallScore,
    overallGrade: .overallGrade,
    signals: [.signals[] | {name, score, status, summary}],
    criticalIssues: [.topIssues[] | select(.severity == "critical")]
  }'

# 查看命名空间级信号评分
curl -sk https://k8ops.iot2.win/api/product/golden-signals \
  -H "Authorization: Bearer $JWT" | jq '.byNamespace[0:10]'
```

---

### 安全修复优先级矩阵

**端点**：`GET /api/security/remediation-matrix`

收集集群中的安全发现，使用 CVSS 类方法评分（0-100），按风险 × 修复工作量优先级排序。

**发现类别与评分**：
- **Pod 安全**：特权容器(95)、root 运行(70)、危险 capabilities(75)、host 命名空间(72)、无 limit(40)
- **网络安全**：无 NetworkPolicy(65)、外部暴露(50)
- **RBAC**：未使用 SA Token(45)
- **镜像安全**：可变标签 latest(42)
- **准入控制**：无 PSA 标签(38)

**修复分类**：
- **快速修复**（Quick Wins）：高风险 + 可在 1 小时内修复
- **战略修复**（Strategic）：高风险 + 需要较多工作量

**输出**：发现列表、分类风险聚合、Top-15 修复计划、预估总修复工时

**示例**：
```bash
# 查看安全修复矩阵
curl -sk https://k8ops.iot2.win/api/security/remediation-matrix \
  -H "Authorization: Bearer $JWT" | jq '{
    summary: .summary,
    quickWins: [.quickWins[] | {id, title, severity, riskScore, fixCommand}],
    topCategories: .byCategory[0:3]
  }'

# 查看 Top-10 修复计划
curl -sk https://k8ops.iot2.win/api/security/remediation-matrix \
  -H "Authorization: Bearer $JWT" | jq '.remediationPlan[0:10]'
```
