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
