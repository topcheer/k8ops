# 变更日志

所有重要变更记录在此文件中。版本号遵循语义化版本规范。

---

## v16.89-v16.96 (2026-07-14)

### v16.89: API 文档同步 v16.78-v16.88 (维度5: 文档)
- CHANGELOG + API.md + en/API.md 同步 v16.78-v16.88

### v16.90: Cert-Manager 健康与证书续期管道审计 (维度1: 产品)
- `GET /api/product/cert-manager` — Issuer/ClusterIssuer 状态、证书过期检测、就绪状态、续期管道健康

### v16.91: 部署资源配额影响与命名空间容量审计 (维度2: 部署)
- `GET /api/deployment/quota-impact` — 命名空间配额使用率、ResourceQuota 硬限制、LimitRange 约束、部署容量影响

### v16.92: 运行时威胁检测与容器异常审计 (维度4: 安全)
- `GET /api/security/runtime-threat` — 特权逃逸风险、可疑能力、seccomp 缺失、异常进程检测、容器行为基线

### v16.93: CNI 插件健康与网络栈配置审计 (维度3: 运维)
- `GET /api/operations/cni-health` — CNI 插件状态、网络模型、节点覆盖、插件版本一致性、IPAM 模式

### v16.94: 成本预算告警与命名空间支出限额审计 (维度6: 可扩展性, 盲区1: Cost/FinOps)
- `GET /api/scalability/budget-alert` — 命名空间预算阈值、支出趋势、超额检测、成本分摊、预算合规评分

### v16.95: Ingress TLS 证书与 HTTPS 强制审计 (维度1: 产品)
- `GET /api/product/ingress-tls` — TLS 证书状态、HTTPS 强制、HSTS 配置、证书过期预测、未加密入口检测

### v16.96: 部署环境配置漂移与 ConfigMap/Secret 引用审计 (维度2: 部署)
- `GET /api/deployment/env-config-drift` — 缺失 ConfigMap/Secret 引用、硬编码密钥、引用验证、环境变量计数、健康评分

## v16.78-v16.88 (2026-07-14)

### v16.78: API 文档同步 (维度5: 文档)
- CHANGELOG + API.md + en/API.md 同步 v16.70-v16.77

### v16.79: VPA 配置与资源建议质量审计 (维度6: 可扩展性)
- `GET /api/scalability/vpa-audit` — VPA 安装检测、更新模式统计、OOM 工作负载识别、覆盖缺口

### v16.80: 服务网格流量管理与熔断器健康审计 (维度1: 产品, 盲区3: 网络网格)
- `GET /api/product/mesh-traffic` — Istio/Linkerd 检测、sidecar 注入覆盖率、VirtualService retry/timeout

### v16.81: 部署滚动更新阻塞与 Pod 条件审计 (维度2: 部署)
- `GET /api/deployment/rollout-blocker` — ProgressDeadlineExceeded、CrashLoopBackOff、ImagePullBackOff、OOMKilled

### v16.82: PSS 强制执行差距与工作负载加固审计 (维度4: 安全)
- `GET /api/security/pss-hardening` — 特权容器、allowPrivilegeEscalation、hostPID/Network/IPC、seccomp/AppArmor

### v16.83: 节点状况趋势与硬件故障预测 (维度3: 运维)
- `GET /api/operations/node-trend` — MemoryPressure、DiskPressure、PIDPressure、陈旧心跳、风险分级

### v16.84: Endpoint Slice 健康与拓扑感知路由审计 (维度1: 产品)
- `GET /api/product/endpoint-slice` — 端点就绪状态、拓扑提示、zone 分布、无端点服务

### v16.85: 滚动更新风险与 Surge 配置分析器 (维度2: 部署)
- `GET /api/deployment/surge-risk` — 高 surge/maxUnavailable、Recreate 策略风险、rollout 卡住检测

### v16.86: 修复重复路由注册 (维度2: 部署)
- 修复 surge-risk 重复路由注册导致 CrashLoopBackOff

### v16.87: 资源饱和与 CPU/内存节流风险预测 (维度6: 可扩展性)
- `GET /api/scalability/saturation` — 无限制 Pod、高 CPU limit/request 比率、节流风险、OOM 风险

### v16.88: 容器镜像仓库限速与拉取可靠性审计 (维度3: 运维)
- `GET /api/operations/registry-rate-limit` — Docker Hub 匿名限速、私有仓库认证、重复镜像、无 pull secrets

## v16.70-v16.77 (2026-07-14)

### v16.70: API 文档更新 v16.66-v16.69 (维度5: 文档)

**更新：**
- CHANGELOG 补充 v16.25-v16.32 条目
- API.md 补充至 147 个端点
- en/API.md 同步英文版本

### v16.71: Spot/抢占式实例就绪与成本优化审计 (维度6: 可扩展性, 盲区1: Cost/FinOps)

**新增 API：**
- `GET /api/scalability/spot-readiness` — Spot 实例就绪审计
  - 检测节点 preemptible 标签、Spot 实例分布
  - 检测无 PDB 保护的 Spot 工作负载
  - 检测 Spot 节点上的有状态工作负载（StatefulSet）
  - 健康评分（0-100），5 个单元测试

### v16.72: 服务流量策略与路由配置审计 (维度1: 产品, 盲区3: Network/Mesh)

**新增 API：**
- `GET /api/product/traffic-policy` — 服务流量策略审计
  - 检测 externalTrafficPolicy 配置（Cluster/Local）
  - 检测 session affinity、over-exposed LB、ExternalName 服务
  - 检测 publishNotReady、external IPs
  - 健康评分（0-100），4 个单元测试

### v16.73: DaemonSet 滚动发布与节点覆盖审计 (维度2: 部署)

**新增 API：**
- `GET /api/deployment/daemonset-audit` — DaemonSet 发布审计
  - 检测 desired vs scheduled vs updated vs ready 节点数
  - 检测缺失节点覆盖、stale 修订版本
  - 检测 toleration 覆盖率
  - 健康评分（0-100），5 个单元测试

### v16.74: 安全策略漂移与基线配置审计 (维度4: 安全, 盲区2: 合规/治理)

**新增 API：**
- `GET /api/security/policy-drift` — 安全策略漂移审计
  - 检测 PSA enforce 标签缺失
  - 检测 PSA 级别不一致（privileged enforce）
  - 检测危险默认角色绑定（cluster-admin → default SA）
  - 检测网络策略基线（default deny 缺失）
  - 检测 API server 安全标志漂移
  - 健康评分（0-100），5 个单元测试

### v16.75: 日志聚合与转发管道健康审计 (维度3: 运维, 盲区5: 可观测性)

**新增 API：**
- `GET /api/operations/log-pipeline` — 日志管道健康审计
  - 检测 Fluent Bit/Fluentd/Vector/Promtail/Filebeat 收集器
  - 检测收集器就绪状态、转发 ConfigMap 配置
  - 检测存储后端（Elasticsearch/Loki/Kafka）
  - 检测命名空间覆盖缺口
  - 健康评分（0-100），5 个单元测试

### v16.76: 容器运行时类与 OCI 镜像合规审计 (维度1: 产品)

**新增 API：**
- `GET /api/product/runtime-class` — 运行时类与镜像合规审计
  - 检测 RuntimeClass 定义（kata）、节点容器运行时（containerd/cri-o）
  - 检测 Pod runtimeClassName 使用情况
  - 检测 :latest 镜像标签、缺失 digest 引用
  - 检测不受信任的镜像仓库
  - 健康评分（0-100），5 个单元测试

### v16.77: 镜像拉取策略与密钥管理审计 (维度2: 部署)

**新增 API：**
- `GET /api/deployment/image-pull-audit` — 镜像拉取策略审计
  - 检测 imagePullPolicy 分布（Always/IfNotPresent/Never）
  - 检测缺失策略、私有镜像无 imagePullSecrets
  - 检测陈旧 dockerconfigjson 密钥、重复密钥
  - 检测 pinned 镜像上的 Always 拉取浪费
  - 健康评分（0-100），5 个单元测试
  - 修复 log pipeline flaky 测试（map 迭代顺序导致检测不一致）

## v16.25-v16.32 (2026-07-13)

### v16.25: 安全上下文漂移与运行时策略合规审计 (维度4: 安全, 盲区2: 合规/治理)

**新增 API：**
- `GET /api/security/sec-drift` — 安全上下文漂移审计
  - 检测缺失 runAsNonRoot、readOnlyRootFilesystem、allowPrivilegeEscalation
  - 检测无 capability drops、ADD ALL caps、privileged containers
  - 检测危险 capabilities（SYS_ADMIN、NET_ADMIN 等）
  - 健康评分（0-100），3 个单元测试

### v16.26: HPA 目标利用率差距与扩缩容行为审计 (维度1: 产品)

**新增 API：**
- `GET /api/product/hpa-gap` — HPA 目标利用率差距审计
  - 检测目标利用率过高（>90%）或过低（<30%）
  - 检测缺失 metrics、minReplicas==maxReplicas（无扩缩容空间）
  - 检测缺失 scaleDown 行为/稳定窗口
  - 健康评分（0-100），3 个单元测试

### v16.27: 节点池与集群自动伸缩健康监控 (维度6: 可扩展性)

**新增 API：**
- `GET /api/scalability/node-pool-health` — 节点池健康监控
  - 检测节点就绪状态、陈旧心跳（>5min）、cordon 节点
  - 检测不平衡池（>30% NotReady）
  - 检测 cluster autoscaler 是否安装
  - 按池和可用区分组分析，健康评分（0-100），3 个单元测试

### v16.28: Helm Release 健康与 GitOps 漂移检测 (维度2: 部署, 盲区4: GitOps/CD)

**新增 API：**
- `GET /api/deployment/helm-health` — Helm Release 健康审计
  - 扫描 Helm release secrets，检测 failed/pending/stale releases
  - 识别卡住的安装/升级、异常 release 状态
  - 健康评分（0-100），3 个单元测试
  - **盲区4 (GitOps/CD) 首次覆盖**

### v16.29: Prometheus 规则健康与告警覆盖率审计 (维度3: 运维, 盲区5: 可观测性栈)

**新增 API：**
- `GET /api/operations/prom-health` — 可观测性栈健康审计
  - 检测 Prometheus、Alertmanager、Grafana、metrics-server、kube-state-metrics
  - 扫描 PrometheusRule ConfigMaps 中的告警/记录规则
  - 识别无告警覆盖的命名空间
  - 健康评分（0-100），3 个单元测试
  - **盲区5 (Observability Stack) 首次覆盖**

### v16.30: OPA/Gatekeeper 策略合规与约束违规审计 (维度4: 安全, 盲区2: 合规/治理)

**新增 API：**
- `GET /api/security/opa-compliance` — OPA/Gatekeeper 策略合规审计
  - 检测 Gatekeeper 和 Kyverno 安装状态
  - 扫描 Constraint CRD，识别 enforce/audit 模式
  - 统计每个 constraint 和命名空间的违规数
  - 健康评分（0-100），3 个单元测试
  - **盲区2 (Compliance/Governance) 首次覆盖**

### v16.31: Service Mesh Sidecar 健康与 mTLS 覆盖率审计 (维度1: 产品, 盲区3: 网络/服务网格)

**新增 API：**
- `GET /api/product/mesh-health` — Service Mesh 健康审计
  - 检测 Istio、Linkerd、Consul Connect 控制面
  - 扫描 pod 的 sidecar 注入状态
  - 检查每个 pod 的 mTLS 状态
  - 检测 sidecar 高重启次数
  - 健康评分（0-100），3 个单元测试
  - **盲区3 (Network/Service Mesh) 首次覆盖**

### v16.32: 闲置资源成本浪费与命名空间成本分摊审计 (维度6: 可扩展性, 盲区1: 成本/FinOps)

**新增 API：**
- `GET /api/scalability/cost-waste` — 成本浪费审计
  - 检测闲置 pods（<100m CPU / <128Mi memory 请求）
  - 检测过度配置 pods（>4 CPU 或 >8Gi memory 请求）
  - 识别闲置 namespaces
  - 计算浪费百分比和按命名空间成本分布
  - 健康评分（0-100），3 个单元测试
  - **盲区1 (Cost/FinOps) 首次覆盖**

**盲区覆盖进度：5/6 完成**
- #1 Cost/FinOps: DONE (v16.32)
- #2 Compliance/Governance: DONE (v16.30)
- #3 Network/Service Mesh: DONE (v16.31)
- #4 GitOps/CD: DONE (v16.28)
- #5 Observability Stack: DONE (v16.29)
- #6 Node Lifecycle: TODO (partial, node-pool-health at v16.27)

**Agent Tools 总数：** 39 base + 2 audit = 41 LLM tools
**Dashboard APIs 总数：** 195 endpoints
**测试总数：** ~1139
**OpenAPI 端点：** 188

---

## v16.20-v16.23 (2026-07-12)

### v16.20: Agent Tool Bridge — 100+ Dashboard API 暴露为 LLM Agent Tool (跨维度基础设施)

**新增 Agent Tools：**
- `k8s_run_audit` — Agent 可运行任意已注册的 100+ 集群审计/分析端点
- `k8s_list_audits` — 列出所有可用审计及描述
- 覆盖全 6 大维度 + 集群概览 + 基础设施
- 注册到 5 个入口点：CLI、manager、diagnostic controller、chat engine、provider tool list
- Agent system prompt 更新，告知 LLM 审计能力
- 8 个单元测试

### v16.21: 部署副本可用性与 Ready Pod 比率监控 (维度2: 部署与发布)

**新增 API：**
- `GET /api/deployment/replica-availability` — 副本可用性监控
  - 监控 Deployments、StatefulSets、DaemonSets 三类工作负载
  - 检测 ready/desired 副本差距、零 Ready 工作负载
  - 检测 rollout 中的陈旧副本
  - 按命名空间分组分析，健康评分（0-100）
  - 4 个单元测试

### v16.22: 多租户资源压力与 Quota 竞争审计 (维度6: 可扩展性)

**新增 API：**
- `GET /api/scalability/tenant-pressure` — 多租户资源压力审计
  - 检测 quota 饱和（>80%）、临界 quota（>95%）
  - 识别无界命名空间（无 quota + 无 LimitRange）
  - 识别消耗不成比例集群资源的命名空间热点
  - 按命名空间风险分级（critical/high/medium/low）
  - 健康评分（0-100）
  - 4 个单元测试

### v16.23: API Server 请求吞吐与负载压力监控 (维度3: 运维与可观测性)

**新增 API：**
- `GET /api/operations/api-load` — API Server 负载监控
  - 分析 pod 密度、controller 数量、event 体量、warning 比率
  - 识别密集命名空间（>100 pods）、高活跃命名空间、空命名空间
  - 健康评分（0-100）
  - 3 个单元测试

**Agent Tools 总数：** 39 base + 2 audit = 41 LLM tools
**Dashboard APIs 总数：** 186 endpoints
**测试总数：** ~1112
**OpenAPI 端点：** 179

---

## v16.19 (2026-07-12)

### v16.19: Init Container 可靠性与启动依赖审计 (维度1: 产品)

**新增 API：**
- `GET /api/product/init-container-audit` — Init Container 可靠性与启动依赖审计
  - 检测缺少资源请求（CPU/内存）的 init container
  - 检测缺少资源限制的 init container
  - 检测过多的 init container（>5 个，增加启动延迟和故障面）
  - 检测 RestartPolicy=Always 的 init container（sidecar 行为，可能延迟启动）
  - 按命名空间和工作负载分组分析
  - 健康评分（0-100）
  - 5 个单元测试

**测试总数：** ~1093

---

## v16.15-v16.16 (2026-07-12)

### v16.15: 部署扩缩容就绪与自动伸缩差距检测 (维度2: 部署与发布)

**新增 API：**
- `GET /api/deployment/scale-readiness` — 部署扩缩容就绪检测
  - 检测缺少 HPA、PDB、资源请求的多副本工作负载
  - 单副本检测（无高可用）
  - 识别完全就绪可扩缩容的工作负载
  - 3 个单元测试

### v16.16: etcd 健康与数据库压力监控 (维度3: 运维与可观测性)

**新增 API：**
- `GET /api/operations/etcd-health` — etcd 健康监控
  - etcd Pod 就绪状态、版本、重启追踪
  - 大型 ConfigMap/Secret 检测（>100KB/500KB/1MB）
  - 单 etcd 实例检测（无 HA 仲裁）
  - 6 个单元测试

### 统计
- OpenAPI 端点：173 → 175
- 单元测试：1074 → 1083

## v16.11-v16.13 (2026-07-12)

### v16.11: 节点拓扑分布与多可用区容错分析 (维度6: 可扩展性与高可用)

**新增 API：**
- `GET /api/scalability/node-topology` — 节点拓扑分布分析
  - 每可用区节点/CPU/内存/Pod 统计
  - 单可用区集群检测（关键风险）
  - 可用区不平衡和缺少区域标签检测
  - 2 个单元测试

### v16.12: RBAC 权限过大与通配符审计 (维度4: 安全与合规)

**新增 API：**
- `GET /api/security/rbac-audit` — RBAC 权限审计
  - 通配符动词 (*) 和资源 (*) 检测
  - 非系统 cluster-admin 绑定检测
  - 每角色严重性分级
  - 5 个单元测试

### v16.13: 卷快照与 PVC 备份合规审计 (维度1: 产品功能)

**新增 API：**
- `GET /api/product/backup-compliance` — PVC 备份合规审计
  - 检测正在使用但缺少备份的 PVC
  - 关键大型 PVC (>=1Gi) 无备份告警
  - Velero 安装状态检查
  - 2 个单元测试

### 统计
- OpenAPI 端点：170 → 173
- 单元测试：1065 → 1074

## v16.08-v16.09 (2026-07-12)

### v16.08: 容器重启策略与生命周期钩子审计 (维度2: 部署与发布)

**新增 API：**
- `GET /api/deployment/restart-policy` — 容器重启策略与生命周期钩子审计
  - 检测策略不匹配（Job 使用 Always，Deployment 使用 Never）
  - 追踪 postStart/preStop 钩子覆盖率
  - 每命名空间统计
  - 2 个单元测试

### v16.09: 证书签名请求与节点引导证书监控 (维度3: 运维与可观测性)

**新增 API：**
- `GET /api/operations/csr-monitor` — 证书签名请求监控
  - 追踪 pending/approved/denied/expired CSR
  - 陈旧 pending 检测（>1h 阻塞节点引导）
  - 按 signer/requester 分组统计
  - 2 个单元测试

### 统计
- OpenAPI 端点：168 → 170
- 单元测试：1061 → 1065

## v16.05-v16.06 (2026-07-11)

### v16.05: Pod 安全取证与事件证据收集 (维度4: 安全与合规)

**新增 API：**
- `GET /api/security/forensics` — Pod 安全取证与事件证据收集
  - 容器退出码分析（OOMKilled/SIGKILL/SIGSEGV 等）
  - 特权容器逃逸检测
  - 容器/镜像哈希不匹配检测（篡改证据）
  - 最近终止记录
  - 6 个单元测试

### v16.06: Pod 拓扑分布约束验证 (维度1: 产品功能)

**新增 API：**
- `GET /api/product/topology-spread` — Pod 拓扑分布约束验证
  - 检测多副本工作负载缺少分布约束
  - 验证 maxSkew/topologyKey/whenUnsatisfiable
  - 实际 Pod 分布分析和域偏差计算
  - 4 个单元测试

### 统计
- OpenAPI 端点：166 → 168
- 单元测试：1051 → 1061

## v16.01-v16.03 (2026-07-11)

### v16.01: IP 地址与 Pod CIDR 利用率监控 (维度6: 可扩展性与高可用)

**新增 API：**
- `GET /api/scalability/ip-cidr-utilization` — IP 地址与 Pod CIDR 利用率监控
  - 每节点 CIDR 容量、利用率、剩余容量
  - 双栈检测、节点接近/已耗尽预警
  - 集群范围 IP 利用率、服务 IP 范围检测
  - 3 个单元测试

### v16.02: Sidecar 容器开销与注入审计 (维度2: 部署与发布)

**新增 API：**
- `GET /api/deployment/sidecar-audit` — Sidecar 容器开销与注入审计
  - 识别已知 sidecar：Istio、Linkerd、Vault、Fluentd、Datadog 等
  - CPU/内存开销计算（每 Pod 和命名空间）
  - 高开销检测 (>30%)、仅注入 Pod 检测
  - 4 个单元测试

### v16.03: DNS 解析健康与 CoreDNS 监控 (维度3: 运维与可观测性)

**新增 API：**
- `GET /api/operations/dns-health` — DNS 解析健康与 CoreDNS 监控
  - CoreDNS Pod 就绪状态、版本、重启追踪
  - ConfigMap 分析（cache/health/ready/prometheus 插件缺失）
  - Pod DNS 策略检测
  - 4 个单元测试

### 统计
- OpenAPI 端点：163 → 166
- 单元测试：1040 → 1051

## v15.98-v15.99 (2026-07-11)

### v15.98: AppArmor & SELinux MAC 合规审计 (维度4: 安全与合规)

**新增 API：**
- `GET /api/security/mac-audit` — AppArmor 与 SELinux 强制访问控制审计
  - AppArmor: unconfined 检测、缺失 profile 检测
  - SELinux: permissive/unconfined 类型检测、缺失上下文检测
  - 节点能力检测
  - 合规评分 (0-100)
  - 2 个单元测试

### v15.99: 服务端点与连通性健康审计 (维度1: 产品功能)

**新增 API：**
- `GET /api/product/service-connectivity` — 服务端点与连通性健康审计
  - 零端点检测、无就绪端点检测
  - 选择器间隙检测（selector 不匹配任何 Pod）
  - 服务类型分布
  - 每命名空间健康统计
  - 健康评分 (0-100)
  - 4 个单元测试

### 统计
- OpenAPI 端点：161 → 163
- 单元测试：1034 → 1040

## v15.95-v15.96 (2026-07-09)

### v15.95: ConfigMap/Secret 配置同步与陈旧检测 (维度2: 部署与发布)

**新增 API：**
- `GET /api/deployment/config-sync` — ConfigMap/Secret 配置同步与陈旧检测
  - 检测使用陈旧配置的 Pod（env/envFrom 引用不自动更新）
  - subPath 挂载检测（不自动更新）
  - 工作负载 Reloader 注解缺失检测
  - 不可变 ConfigMap/Secret 检测
  - 陈旧评分 (0-100)
  - 5 个单元测试

### v15.96: Kubelet 与容器运行时健康监控 (维度3: 运维与可观测性)

**新增 API：**
- `GET /api/operations/kubelet-health` — Kubelet 与容器运行时健康监控
  - 每节点 kubelet 版本、运行时版本、OS 镜像、心跳新旧度
  - 版本偏差和运行时偏差检测
  - 条件追踪：NotReady、DiskPressure、MemoryPressure、PIDPressure
  - 运行时分布统计（containerd/docker/cri-o）
  - 健康评分 (0-100)
  - 7 个单元测试

### 统计
- OpenAPI 端点：159 → 161
- 单元测试：1022 → 1034

## v15.91-v15.93 (2026-07-09)

### v15.91: Pod Security Admission (PSA) 强制执行审计 (维度4: 安全与合规)

**新增 API：**
- `GET /api/security/psa-audit` — Pod Security Admission 强制执行审计
  - 检查 pod-security.kubernetes.io/enforce/audit/warn 标签
  - Baseline 违规：privileged、hostNetwork/PID/IPC、hostPath、危险 caps
  - Restricted 违规：以 root 运行、特权升级、未丢弃 caps、缺少 seccomp
  - 强制执行评分 (0-100)
  - 8 个单元测试

### v15.92: Pod QoS 与 PriorityClass 分布审计 (维度1: 产品功能)

**新增 API：**
- `GET /api/product/qos-priority` — Pod QoS 与 PriorityClass 分布审计
  - QoS 分布：Guaranteed、Burstable、BestEffort
  - PriorityClass 使用分析：system-critical、high、medium、low
  - 配置错误检测与驱逐风险分析
  - QoS 健康评分 (0-100)
  - 6 个单元测试

### v15.93: 资源碎片化与装箱效率分析 (维度6: 可扩展性与高可用)

**新增 API：**
- `GET /api/scalability/fragmentation` — 资源碎片化与装箱效率分析
  - 每节点 CPU/内存/Pod 槽位利用率和效率
  - 碎片化评分和滞留资源检测
  - Pod 大小模拟（small/medium/large/xlarge）
  - Bin-packing 评分 (0-100) 和碎片化评分 (0-100)
  - 5 个单元测试

### 统计
- OpenAPI 端点：156 → 159
- 单元测试：1003 → 1022

## v15.89 (2026-07-09)

### Pod 启动生命周期与瓶颈分析 (维度3: 运维与可观测性)

**新增 API：**
- `GET /api/operations/pod-startup` — Pod 启动生命周期与瓶颈分析
  - 启动阶段分解：调度延迟、init 容器耗时、镜像拉取、就绪探针延迟
  - 慢启动 Pod 识别 (>120s)，按阶段拆分耗时
  - 卡住 Pod 追踪（Pending/ContainerCreating）
  - 瓶颈分类：scheduling、image_pull、init_container、probe、volume
  - 按工作负载类型统计启动时间
  - 集群启动健康评分 (0-100)
  - 8 个单元测试

## v15.88 (2026-07-09)

### 容器临时存储与 emptyDir 限制合规 (维度2: 部署与发布)

**新增 API：**
- `GET /api/deployment/ephemeral-storage` — 容器临时存储与 emptyDir 限制合规检查
  - 检查项：ephemeral-storage 限制、emptyDir 卷数量及大小限制、无限制 emptyDir 检测
  - 合规评分 (0-100)
  - 5 个单元测试

### 统计
- OpenAPI 端点：154 → 156
- 单元测试：990 → 1003

### 灾难恢复就绪与备份合规审计 (维度6: 可扩展性与高可用)

**新增 API：**
- `GET /api/scalability/dr-readiness` — 灾难恢复就绪与备份合规审计
  - 检查项：Velero/备份控制器、命名空间备份标签、CSI 快照控制器、多可用区拓扑、PVC 数据保护
  - DR 就绪评分 (0-100)
  - 5 个单元测试

### 统计
- OpenAPI 端点：153 → 154
- 单元测试：985 → 990

## v15.84 (2026-07-08)

### 新功能
- **Product**: 已废弃 API 版本与升级就绪检查 (`GET /api/product/api-deprecation`)
  - 通过 API discovery 检测集群中仍在使用的已废弃/已移除的 K8s API 版本
  - 覆盖 18 种废弃 API（extensions、apps/v1beta1/v1beta2、networking、batch、autoscaling、PSP）
  - 升级就绪评分 (0-100)
  - 5 个单元测试

### v15.83
- **Security**: 容器主机命名空间与特权暴露审计 (`GET /api/security/host-namespace`)
  - 审计 hostNetwork、hostPID、hostIPC、privileged、hostPath、capAdd、runAsRoot
  - 暴露安全评分 (0-100)
  - 6 个单元测试

## v15.81 (2026-07-08)

### 新增
- **卷挂载与附加错误追踪** (`GET /api/operations/volume-mount-errors`)
  - 追踪因卷挂载/附加失败而卡住的 Pod
  - 错误分类：mount_fail、attach_fail、provisioning、timeout
  - 健康评分 (0-100)
  - 7 个单元测试

## v15.80 (2026-07-08)

### 文档
- API.md 新增 1 个端点文档 (v15.79)
- CHANGELOG.md 更新 v15.78-v15.79 发布日志

## v15.79 (2026-07-08)

### 新增
- **工作负载成熟度与最佳实践评分** (`GET /api/deployment/workload-maturity`)
  - 8 项最佳实践检查（权重总和=100）
  - 每工作负载：成熟度评分 0-100
  - 集群平均成熟度评分
  - 5 个单元测试

## v15.78 (2026-07-08)

### 文档
- API.md 新增 1 个端点文档 (v15.77)
- CHANGELOG.md 更新 v15.77 发布日志

## v15.77 (2026-07-08)

### 新增
- **HPA 健康与缩放活动分析** (`GET /api/product/hpa-health`)
  - 每 HPA：副本数、缩放活跃状态、指标数量、条件状态
  - 检测：达到最大副本、无指标、缩放未激活
  - 健康评分 (0-100)
  - 6 个单元测试

## v15.76 (2026-07-08)

### 文档
- API.md 新增 2 个端点文档 (v15.74-v15.75)
- CHANGELOG.md 更新 v15.74-v15.75 发布日志

## v15.75 (2026-07-08)

### 新增
- **集群扩展性与阈值监控** (`GET /api/scalability/scale-limits`)
  - 检查集群与 K8s 官方限制的接近程度
  - 限制：Nodes(5000)、Pods(150000)、Services(5000)、Namespaces(10000)
  - Pod 容量利用率跟踪
  - 扩展评分 (0-100)
  - 6 个单元测试

## v15.74 (2026-07-08)

### 新增
- **Secret 静态加密配置检查** (`GET /api/security/encryption-at-rest`)
  - 检查 Secret 是否在 etcd 中加密
  - 检测 k3s 环境
  - 安全评分 (0-100)
  - 6 个单元测试

## v15.73 (2026-07-08)

### 文档
- API.md 新增 1 个端点文档 (v15.72)
- CHANGELOG.md 更新 v15.71-v15.72 发布日志

## v15.72 (2026-07-08)

### 新增
- **API 服务器响应速度与 Pod 启动延迟监控** (`GET /api/operations/api-latency`)
  - 监控 API 响应性、等待 Pod、容器启动延迟
  - 检测：调度慢 >2min、不就绪 Pod、容器启动慢
  - 响应评分 (0-100)
  - 6 个单元测试

## v15.71 (2026-07-07)

### 文档
- API.md 新增 2 个端点文档 (v15.69-v15.70)
- CHANGELOG.md 更新 v15.69-v15.70 发布日志

## v15.70 (2026-07-07)

### 新增
- **批处理 Job 执行健康分析** (`GET /api/product/job-health`)
  - 每Job：状态、运行时长、完成数、backoffLimit
  - 检测：失败Job(warning)、运行>24h(warning)、暂停(info)
  - 健康评分 (0-100)
  - 6 个单元测试

## v15.69 (2026-07-07)

### 新增
- **部署中断与维护影响分析** (`GET /api/deployment/disruption-impact`)
  - 分析 Deployment/StatefulSet + PDB 交互
  - 检测：阻塞 drain(critical)、无 PDB(warning)、危险 PDB
  - 维护就绪评分 (0-100)
  - 6 个单元测试

## v15.68 (2026-07-07)

### 文档
- API.md 新增 1 个端点文档 (v15.66-v15.67)
- CHANGELOG.md 更新

## v15.67 (2026-07-07)

### 新增
- **CSI 驱动与存储能力审计** (`GET /api/scalability/csi-audit`)
  - 每 StorageClass：provisioner、默认、绑定模式、扩展、回收策略
  - 每 CSIDriver：attach required、fsGroup policy
  - 检测：无默认 SC、缺失 CSI 驱动、无扩展支持
  - 健康评分 (0-100)
  - 6 个单元测试

## v15.66 (2026-07-07)

### 文档
- API.md 新增 v15.64-v15.65 端点文档
- CHANGELOG.md 更新 v15.64-v15.65 发布日志

## v15.65 (2026-07-07)

### 新增
- **API Server 审计日志配置检查** (`GET /api/security/audit-policy`)
  - 检查审计日志启用状态、策略文件、保留配置
  - 检测 k3s/microk8s 环境
  - 合规评分 (0-100) for PCI-DSS/SOC2/HIPAA
  - 6 个单元测试

## v15.64 (2026-07-07)

### 文档
- API.md 新增 1 个端点文档 (v15.63)
- CHANGELOG.md 更新 v15.62-v15.63 发布日志

## v15.63 (2026-07-07)

### 新增
- **Pod 驱逐与节点压力历史追踪** (`GET /api/operations/pod-evictions`)
  - 追踪被驱逐的 Pod，按原因分类（内存/磁盘/PID/未知）
  - 每节点驱逐计数和风险级别
  - 最近 24 小时驱逐详情
  - 健康评分 (0-100)
  - 5 个单元测试

## v15.62 (2026-07-07)

### 文档
- API.md 新增 3 个端点文档 (v15.59-v15.61)
- CHANGELOG.md 更新 v15.59-v15.61 发布日志

## v15.61 (2026-07-07)

### 新增
- **ConfigMap/Secret 大小与内存压力审计** (`GET /api/product/configmap-size`)
  - 6 个单元测试

## v15.60 (2026-07-07)

### 新增
- **部署版本历史与回滚就绪分析** (`GET /api/deployment/revision-history`)
  - 5 个单元测试

## v15.59 (2026-07-07)

### 新增
- **命名空间隔离与多租户审计** (`GET /api/scalability/namespace-isolation`)
  - 5 个单元测试

## v15.58 (2026-07-07)

### 文档
- API.md 新增 2 个端点文档 (v15.56-v15.57)

## v15.57 (2026-07-07)

### 新增
- **控制平面健康检查** (`GET /api/operations/control-plane`)
  - 检查 kube-apiserver, kube-scheduler, kube-controller-manager, etcd
  - 每组件：就绪状态、重启次数、运行时长
  - k3s/microk8s/kind 检测
  - 健康评分 (0-100)
  - 7 个单元测试

## v15.56 (2026-07-07)

### 新增
- **节点污点与 Pod 容忍度影响分析** (`GET /api/product/taint-toleration`)
  - 每节点：污点列表、cordon 状态、风险级别
  - 集群级污点摘要
  - 检测：NoExecute(critical)、cordoned(warning)、宽泛容忍(warning)
  - 影响评分 (0-100)
  - 6 个单元测试

## v15.55 (2026-07-07)

### 文档
- API.md 新增 3 个端点文档 (v15.52-v15.54)
- CHANGELOG.md 更新 v15.52-v15.54 发布日志

## v15.54 (2026-07-07)

### 新增
- **部署镜像漂移与版本一致性检测** (`GET /api/deployment/image-drift`)
  - 每工作负载：不同镜像版本列表 + Pod 数
  - 检测：镜像漂移(high)、:latest(medium)、无摘要(low)
  - 一致性评分 (0-100)
  - 5 个单元测试

## v15.53 (2026-07-07)

### 新增
- **K8s 可扩展性瓶颈预测器** (`GET /api/scalability/bottleneck-predictor`)
  - 比较 7 种资源 vs K8s 限制（pods/node, total pods, services 等）
  - 状态：healthy/warning/critical/bottleneck
  - 风险评分 (0-100)
  - 4 个单元测试

## v15.52 (2026-07-07)

### 新增
- **节点心跳与健康租约监控** (`GET /api/operations/node-lease`)
  - 通过 Lease 对象监控 kubelet 心跳新鲜度
  - 检测：无 Lease(critical)、心跳 >2min(critical)、心跳 >40s(high)
  - 健康评分 (0-100)
  - 5 个单元测试

## v15.51 (2026-07-07)

### 文档
- API.md 新增 3 个端点文档 (v15.48-v15.50)
- CHANGELOG.md 更新 v15.48-v15.50 发布日志

## v15.50 (2026-07-07)

### 新增
- **Pod 亲和性/反亲和性冲突检测** (`GET /api/product/affinity-conflict`)
  - 拓扑域分析：hostname/zone/region 域映射
  - 检测：不可满足反亲和性(critical)、因亲和性Pending(high)
  - 7 个单元测试

## v15.49 (2026-07-07)

### 新增
- **Secret/ConfigMap 引用完整性检查** (`GET /api/deployment/ref-integrity`)
  - 检查 Deployment/StatefulSet/DaemonSet 的所有 Secret/ConfigMap 引用
  - 来源：volume、envFrom、env valueFrom
  - 5 个单元测试

## v15.48 (2026-07-07)

### 新增
- **API 对象计数与 CRD 爆炸风险检测器** (`GET /api/scalability/crd-explosion`)
  - 每资源类型：对象计数、风险级别
  - 每命名空间：ConfigMap/Secret/Service/Pod 计数
  - 6 个单元测试

## v15.47 (2026-07-07)

### 文档
- API.md 新增 3 个端点文档 (v15.44-v15.46)
- CHANGELOG.md 更新 v15.44-v15.46 发布日志

## v15.46 (2026-07-07)

### 新增
- **资源争用与限流检测器** (`GET /api/operations/resource-contention`)
  - 检测 CPU 限流模式、内存压力、资源争用
  - 检测项：节点压力、高重启 Pod、无限制 Pod、低限制
  - 争用评分 (0-100)
  - 6 个单元测试

## v15.45 (2026-07-06)

### 新增
- **StatefulSet 健康与有序滚动更新审计** (`GET /api/product/statefulset-audit`)
  - 每STS：Pod管理策略、PVC保留策略、Headless Service、VolumeClaimTemplates、分区金丝雀
  - 检测：无headless(critical)、卡住(high)、PVC Delete(high)、暂停金丝雀(warning)
  - 健康评分 (0-100)
  - 6 个单元测试

## v15.44 (2026-07-06)

### 新增
- **部署更新策略与回滚就绪审计** (`GET /api/deployment/update-strategy`)
  - 每部署：策略类型、maxSurge/maxUnavailable、revisionHistoryLimit、progressDeadlineSeconds
  - 检测：Recreate(critical)、maxUnavailable=100%(high)、maxSurge=0、低版本历史、无进度截止
  - 就绪评分 (0-100)
  - 6 个单元测试

## v15.43 (2026-07-06)

### 文档
- API.md 新增 5 个端点文档 (v15.38-v15.42)
- CHANGELOG.md 更新 v15.38-v15.42 发布日志

## v15.42 (2026-07-06)

### 新增
- **节点故障影响模拟器** (`GET /api/scalability/node-failure-sim`)
  - 模拟每个节点故障后的影响：受影响 Pod 数、可重调度/不可调度
  - 重调度可行性检查：资源容量、Node Selector、Taint/Toleration
  - 弹性评分 (0-100)
  - 6 个单元测试

## v15.41 (2026-07-06)

### 新增
- **Pod 调度延迟分析器** (`GET /api/operations/scheduling-latency`)
  - 每 Pod：创建→调度时间、Pending 原因
  - 检测：Unschedulable、资源短缺、慢调度 (>60s/>300s)
  - 每节点平均调度时间
  - 调度效率评分 (0-100)
  - 7 个单元测试

## v15.40 (2026-07-06)

### 新增
- **CronJob 与批处理作业安全审计** (`GET /api/security/batch-audit`)
  - Privileged 检测、HostPath、HostNetwork/PID
  - 默认 SA 检测、可疑调度（每分钟=持久化）检测
  - 批处理安全评分 (0-100)
  - 7 个单元测试

## v15.39 (2026-07-06)

### 新增
- **PV/PVC 存储健康与容量审计** (`GET /api/product/pvc-health`)
  - 每 PVC：Phase（Bound/Pending/Lost）、SC、容量
  - 每 PV：Phase（Bound/Available/Released/Failed）、Reclaim Policy
  - StorageClass 分析：扩容支持、默认检测
  - 存储健康评分 (0-100)
  - 7 个单元测试

## v15.38 (2026-07-06)

### 新增
- **优雅终止与终止合规审计** (`GET /api/deployment/graceful-shutdown`)
  - preStop Hook 检测、Readiness Probe 检测
  - Grace Period 分类（short/default/long/custom）
  - 丢弃请求风险检测（无 preStop + 无 readiness = critical）
  - 优雅终止评分 (0-100)
  - 8 个单元测试

## v15.37 (2026-07-06)

### 文档
- API.md 新增 5 个端点文档 (v15.32-v15.36)
- CHANGELOG.md 更新 v15.32-v15.36 发布日志
- 更新 en/API.md 英文端点文档

## v15.36 (2026-07-06)

### 新增
- **高可用与单点故障检测器** (`GET /api/scalability/ha-audit`)
  - 5 种 SPOF 检测：单副本、单节点分布、无 PDB、无反亲和、无 Readiness
  - HA 评分 (0-100)
  - 8 个单元测试

## v15.35 (2026-07-06)

### 新增
- **Pod 重启原因分析器** (`GET /api/operations/restart-reasons`)
  - 原因分类：OOMKilled、应用错误、配置错误、DeadlineExceeded、Completed
  - Top 20 重启最多容器，每命名空间分析
  - 集群稳定性评分 (0-100)
  - 8 个单元测试

## v15.34 (2026-07-06)

### 新增
- **Seccomp 与 PSS Restricted 合规审计器** (`GET /api/security/seccomp-audit`)
  - Seccomp 配置文件检测、Capabilities drop/add 追踪
  - PSS 级别分类：restricted/baseline/privileged
  - 危险 Capability 检测（11 个：SYS_ADMIN 等）
  - 容器加固评分 (0-100)
  - 6 个单元测试

## v15.33 (2026-07-06)

### 新增
- **孤立资源检测器** (`GET /api/product/orphaned-resources`)
  - 5 种资源：Services（无 Pod）、ConfigMaps（未引用）、Secrets（过期凭证）、PVCs（未挂载）、Ingresses（后端缺失）
  - Pod 引用追踪：卷、环境变量、envFrom、ImagePullSecrets
  - 集群卫生评分 (0-100)
  - 5 个单元测试

## v15.32 (2026-07-05)

### 新增
- **资源限制与强制差距审计器** (`GET /api/deployment/resource-limits`)
  - 无限制容器检测（critical）、无内存限制（critical）
  - 供应不足 (<1.2x) / 供应过度 (>4x) 检测
  - 过度请求检测 (>2000m CPU, >4Gi 内存)
  - 合规评分 (0-100)
  - 8 个单元测试

## v15.31 (2026-07-05)

### 文档
- API.md 新增 3 个端点文档 (v15.28-v15.30)
- CHANGELOG.md 更新 v15.28-v15.30 发布日志
- 更新 en/API.md 英文端点文档

## v15.30 (2026-07-05)

### 新增
- **资源配额使用率与限制合规审计器** (`GET /api/scalability/quota-utilization`)
  - ResourceQuota 使用率分析 (hard/used/utilization%)
  - LimitRange 合规检查 (default request/limit, max enforcement)
  - 容器资源治理：缺失 requests/limits 检测
  - 配额合规评分 (0-100)
  - 7 个单元测试

## v15.29 (2026-07-05)

### 新增
- **ImagePullBackOff 与容器启动失败追踪器** (`GET /api/operations/image-pull-failures`)
  - 失败类型：ImagePullBackOff, ErrImagePull, CreateContainerError, CrashLoopBackOff
  - 根因分类：Registry 认证失败、Docker Hub 限速、无效镜像名
  - 按镜像聚合、按命名空间统计
  - 镜像拉取健康评分 (0-100)
  - 10 个单元测试

## v15.28 (2026-07-05)

### 新增
- **服务端点暴露与攻击面审计器** (`GET /api/security/endpoint-exposure`)
  - 每服务：类型、暴露级别 (public/node/internal)、端口分析
  - 每 Ingress：域名、TLS 状态、后端、路由计数
  - 风险检测：公开暴露+无NP、无TLS Ingress、NodePort、ExternalIP
  - 攻击面评分 (0-100，越高越安全)
  - 10 个单元测试

## v15.27 (2026-07-05)

### 文档
- API.md 新增 5 个端点文档 (v15.22-v15.26)
- CHANGELOG.md 更新 v15.22-v15.26 发布日志
- 更新 en/API.md 英文端点文档

## v15.26 (2026-07-05)

### 新增
- **标签与注解卫生审计器** (`GET /api/product/label-hygiene`)
  - 零标签检测、缺失标准/团队/版本标签检测
  - 畸形标签键检测 (DNS-1123 合规)
  - 每命名空间评分，集群合规评分 (0-100)
  - 10 个单元测试

## v15.25 (2026-07-05)

### 新增
- **健康探针合规审计器** (`GET /api/deployment/probe-compliance`)
  - Liveness/Readiness/Startup 探针状态检测
  - 零探针、缺失 readiness (critical)、缺失 liveness 检测
  - 配置错误检测（过长延迟、慢周期、高阈值）
  - 探针合规评分 (0-100)
  - 8 个单元测试

## v15.24 (2026-07-05)

### 新增
- **集群容量余量与扩容就绪分析器** (`GET /api/scalability/capacity-headroom`)
  - 每节点 CPU/内存/Pod 槽位余量分析
  - Pod 调度容量估算 (small/medium/large/xlarge)
  - Cluster Autoscaler/Karpenter 检测
  - 余量评分 (0-100)
  - 8 个单元测试

## v15.23 (2026-07-05)

### 新增
- **拓扑分布与 Pod 分配审计器** (`GET /api/operations/topology-distribution`)
  - 节点分布图、集中检测、分布比率
  - topologySpreadConstraints / podAntiAffinity 状态
  - 节点负载不均衡检测
  - 分布评分 (0-100)
  - 6 个单元测试

## v15.22 (2026-07-05)

### 新增
- **卷安全与挂载风险审计器** (`GET /api/security/volume-mounts`)
  - 14 个危险路径检测 (docker.sock, /proc, /sys, /, kubelet, etcd)
  - HostPath 风险分级 (critical/high/medium/low)
  - 特权容器 + Host namespace 共享检测
  - SA Token 投射卷追踪
  - 安全评分 (0-100，越高越安全)
  - 9 个单元测试

## v15.21 (2026-07-05)

### 文档
- API.md 新增 7 个端点文档 (v15.13-v15.20)
- 创建 CHANGELOG.md (v15.13-v15.20 发布日志)
- 更新 en/API.md 英文端点文档

## v15.20 (2026-07-05)

### 新增
- **Network Policy 合规与流量隔离审计器** (`GET /api/product/network-policy`)
  - 按命名空间分析 NetworkPolicy 覆盖率和流量隔离
  - 未保护 Pod 检测、宽松出站检测 (0.0.0.0/0)
  - 集群隔离评分 (0-100)
  - 7 个单元测试

## v15.19 (2026-07-05)

### 新增
- **Deployment 滚动更新策略与健康分析器** (`GET /api/deployment/rollout-health`)
  - 滚动更新策略分析 (RollingUpdate vs Recreate)
  - 卡住部署检测 (Progressing=False / ReplicaFailure / 超时)
  - 回滚就绪评估 (revisionHistoryLimit)
  - 集群滚动更新健康评分 (0-100)
  - 7 个单元测试

## v15.18 (2026-07-05)

### 新增
- **命名空间资源消耗与成本归属** (`GET /api/scalability/ns-consumption`)
  - 按命名空间聚合 CPU/内存/存储消耗
  - 月成本估算 ($28/核 CPU, $3.8/GB 内存, $0.10/GB 存储)
  - 浪费分析：过度配置、空闲命名空间、浪费评分
  - Top 10 消费者排行
  - 5 个单元测试

## v15.17 (2026-07-05)

### 新增
- **PDB 合规与自愿中断风险分析器** (`GET /api/operations/pdb-audit`)
  - PDB 状态分类：healthy / blocked / impossible
  - 未保护多副本部署检测
  - 节点排空模拟（逐节点 PDB 阻塞分析）
  - 集群 PDB 覆盖评分 (0-100)
  - 8 个单元测试

## v15.16 (2026-07-05)

### 新增
- **证书与 TLS 过期监控器** (`GET /api/security/cert-expiry`)
  - PEM 证书解析 (CN, SANs, 颁发者, 有效期, 密钥大小)
  - 自签名检测、Pod 引用追踪
  - 过期分级：critical (<30d) / high (<60d) / medium (<90d) / low (>90d)
  - 集群证书健康评分 (0-100)
  - 9 个单元测试

## v15.15 (2026-07-05)

### 新增
- **多语言文档** — 7 种语言，76 个文件
  - English, 中文, 日本語, 한국어, Español, Français, Deutsch
  - 每语言 10 篇文档 + README
  - 母语级 Review 完成

## v15.14 (2026-07-05)

### 新增
- **ConfigMap & Secret 配置审计器** (`GET /api/product/config-audit`)
  - ConfigMap：超大检测 (>1MB)、未引用检测、空数据键、不可变标志
  - Secret：过期凭证 (>180d)、未引用、明文凭证键检测、轮换建议
  - 交叉引用引擎 (Pod volumes, env, envFrom, projected sources)
  - 集群配置审计健康评分 (0-100)
  - 7 个单元测试

## v15.13 (2026-07-05)

### 新增
- **容器镜像部署规范分析器** (`GET /api/deployment/image-hygiene`)
  - 镜像标签策略分析 (版本号 / :latest / @sha256 摘要锁定)
  - 重复镜像检测、仓库信任分级
  - 集群镜像规范评分 (0-100)
  - 8 个单元测试

---

## v16.34-v16.42 (2026-07-13)

### v16.34: 节点 OS 补丁与内核版本漂移审计 (维度6: 可扩展性, 盲区6: 节点生命周期)

**新增 API：**
- `GET /api/scalability/node-lifecycle` — 节点生命周期审计
  - 内核版本漂移检测、OS 镜像差异检测
  - 架构多样性分析、GPU 资源可用性检测
  - 节点年龄与轮换需求（90/180天阈值）
  - 健康评分（0-100），3 个单元测试

### v16.35: 滚动更新风险与 Surge 配置分析 (维度2: 部署)

**新增 API：**
- `GET /api/deployment/surge-risk` — 滚动更新风险与 Surge 配置分析
  - Max Surge/Max Unavailable 配置审计
  - 不可用副本风险评估、Surge 资源影响分析
  - 健康评分（0-100），3 个单元测试

### v16.36: Alertmanager 配置与告警路由健康审计 (维度3: 运维, 盲区5: 观测性深化)

**新增 API：**
- `GET /api/operations/alertmanager-health` — Alertmanager 配置与告警路由审计
  - Alertmanager 实例检测、告警路由配置分析
  - 告警接收器健康、静默规则检测
  - 健康评分（0-100），3 个单元测试

### v16.37: 容器镜像漏洞与补丁滞后审计 (维度4: 安全)

**新增 API：**
- `GET /api/security/image-vuln` — 容器镜像漏洞与补丁滞后审计
  - latest 标签检测、digest 引用检测
  - 镜像新鲜度评估、补丁滞后风险
  - 健康评分（0-100），3 个单元测试

### v16.38: CronJob 调度冲突与资源配置审计 (维度1: 产品)

**新增 API：**
- `GET /api/product/cronjob-schedule` — CronJob 调度冲突与资源配置审计
  - 调度时间冲突检测（3+ 个任务在同一时间槽）
  - ConcurrencyPolicy 审计、资源限制检查
  - Job 历史限制检查、挂起任务检测
  - 健康评分（0-100），5 个单元测试

### v16.39: Pod 启动延迟与就绪性能审计 (维度2: 部署)

**新增 API：**
- `GET /api/deployment/startup-latency` — Pod 启动延迟与就绪性能审计
  - p50/p90/p99 启动百分位计算
  - 慢启动 Pod 检测（>60s warning, >120s critical）
  - 缺失 readiness/liveness 探针检测
  - CrashLoopBackOff 追踪、init container 影响分析
  - 健康评分（0-100），6 个单元测试

### v16.40: Grafana Dashboard 可用性与数据源健康审计 (维度3: 运维, 盲区5: 观测性深化)

**新增 API：**
- `GET /api/operations/grafana-health` — Grafana Dashboard 可用性与数据源健康审计
  - Grafana 安装检测、Dashboard ConfigMap 分析
  - 过期 Dashboard 检测（无/超长刷新间隔）
  - 损坏 Dashboard 检测（有面板但无数据源引用）
  - Grafana Pod 健康检查
  - 健康评分（0-100），6 个单元测试

### v16.41: Kyverno 策略合规与集群策略审计 (维度4: 安全, 盲区2: 合规深化)

**新增 API：**
- `GET /api/security/kyverno-compliance` — Kyverno 策略合规与集群策略审计
  - ClusterPolicy/Policy CRD 扫描（dynamic client）
  - 规则分类：validate/mutate/generate
  - 强制模式审计（Enforce vs Audit）、后台扫描状态
  - Audit-only 策略强制执行就绪识别
  - 健康评分（0-100），5 个单元测试

### v16.42: 资源请求与限制分配效率审计 (维度6: 可扩展性, 盲区1: 成本深化)

**新增 API：**
- `GET /api/scalability/alloc-efficiency` — 资源请求与限制分配效率审计
  - CPU/内存 request vs limit 比率分析
  - 过度分配检测（request ≈ limit，浪费调度）
  - 分配不足检测（request << limit，限流风险）
  - 无请求/无限制容器检测
  - 整体 CPU 分配效率比率
  - 健康评分（0-100），3 个单元测试

---

## v16.44-v16.55 (2026-07-14)

### v16.44: External Secrets 与 Secret Store CSI 健康审计 (维度1: 产品)

**新增 API：**
- `GET /api/product/external-secret-health` — External Secrets 与 Secret Store CSI 健康审计
  - External Secrets Operator 安装检测
  - ExternalSecret/SecretStore CRD 扫描（dynamic client）
  - 同步状态审计（NotSynced/Error/循环检测）
  - SecretStore 配置验证、缺失 CSI 驱动检测
  - 健康评分（0-100），5 个单元测试

### v16.45: 渐进式交付与金丝雀发布健康审计 (维度2: 部署)

**新增 API：**
- `GET /api/deployment/progressive-delivery` — 渐进式交付与金丝雀发布健康审计
  - Argo Rollouts / Flagger CRD 检测（dynamic client）
  - 金丝雀/蓝绿/滚动发布策略分析
  - 流量权重分析、分析步骤验证
  - 卡住的发布检测（长时间无进展）
  - 健康评分（0-100），4 个单元测试

### v16.46: Metrics Pipeline 与 kube-state-metrics 健康审计 (维度3: 运维, 盲区5: 观测性深化)

**新增 API：**
- `GET /api/operations/metrics-pipeline` — Metrics 管道与 kube-state-metrics 健康审计
  - metrics-server 安装检测与可用性
  - kube-state-metrics 部署健康
  - Prometheus 指标管道完整性验证
  - 缺失 metrics 源检测（API server、kubelet、容器）
  - 健康评分（0-100），4 个单元测试

### v16.47: Pod Security Standards 合规评分卡 (维度4: 安全, 盲区2: 合规深化)

**新增 API：**
- `GET /api/security/pss-scorecard` — Pod Security Standards 合规评分卡
  - Privileged/Baseline/Restricted 三个级别合规审计
  - 按 namespace 检查 PSA 模式绑定
  - 违规容器检测（privileged、hostPath、hostPort、capabilities 等）
  - 维度评分汇总，namespace 合规率
  - 健康评分（0-100），4 个单元测试

### v16.48: HPA 自动伸缩性能与扩缩容事件审计 (维度6: 可扩展性)

**新增 API：**
- `GET /api/scalability/hpa-performance` — HPA 自动伸缩性能与扩缩容事件审计
  - 扩缩容事件分析（基于 Replica 变化历史）
  - 伸缩震荡检测（频繁扩缩缩循环）
  - 缩容延迟分析（长时间未缩容）
  - 当前与目标利用率差距
  - 健康评分（0-100），3 个单元测试

### v16.49: 服务端点与 DNS 解析健康审计 (维度1: 产品)

**新增 API：**
- `GET /api/product/endpoint-dns-health` — 服务端点与 DNS 解析健康审计
  - Service 后端 Pod 就绪状态检查
  - Endpoints 空服务检测（无后端 Pod）
  - CoreDNS 配置验证与 Pod 健康
  - DNS 解析测试（基于 CoreDNS Pod 状态）
  - ExternalName 服务解析验证
  - 健康评分（0-100），3 个单元测试

### v16.50: ReplicaSet 陈旧度与滚动发布历史审计 (维度2: 部署)

**新增 API：**
- `GET /api/deployment/rs-staleness` — ReplicaSet 陈旧度与滚动发布历史审计
  - 陈旧 ReplicaSet 检测（非当前 RS 保留过多）
  - revisionHistoryLimit 配置审计
  - RS 年龄分析、孤立 RS 检测
  - 滚动发布历史深度评估
  - 健康评分（0-100），4 个单元测试

### v16.51: 审计日志管道与事件导出健康审计 (维度3: 运维, 盲区5: 观测性深化)

**新增 API：**
- `GET /api/operations/audit-log-health` — 审计日志管道与事件导出健康审计
  - 审计日志配置检测（audit-policy.json）
  - 日志后端验证（backend: logFile/logBatch）
  - 事件导出管道健康（Fluentd/Fluent Bit/Loki 检测）
  - Warning 事件积压检测
  - 健康评分（0-100），4 个单元测试

### v16.52: ServiceAccount Token 轮换与访问风险审计 (维度4: 安全)

**新增 API：**
- `GET /api/security/sa-token-audit` — ServiceAccount Token 轮换与访问风险审计
  - 长期未轮换 Secret token 检测（>90天）
  - 无 Secret 引用的 SA 检测
  - automountServiceAccountToken 配置审计
  - 高权限 SA 检测（cluster-admin 绑定）
  - 健康评分（0-100），5 个单元测试

### v16.53: PV 回收策略与存储类浪费审计 (维度6: 可扩展性)

**新增 API：**
- `GET /api/scalability/pv-reclaim` — PV 回收策略与存储类浪费审计
  - 回收策略审计（Retain/Recycle/Delete）
  - Released 状态 PV 检测（可回收空间）
  - Failed 状态 PV 检测
  - 存储类绑定分析、默认存储类审计
  - 健康评分（0-100），3 个单元测试

### v16.54: 前端审计仪表盘 — 统一审计端点视图 (维度3: 运维)

**前端增强：**
- 新增 "Audit Dashboard" 标签页，展示所有 40+ 审计端点
- 健康评分卡片（0-100），颜色编码：绿/黄/橙/红
- 按六大维度组织，点击查看详细问题、建议和统计
- 并行 API 加载，快速响应
- 新文件: `audit-dashboard.js`（248 行）
- 接入 `index.html`、`core.js`、`main.js`

### v16.55: ConfigMap 与 Secret 挂载注入风险审计 (维度1: 产品)

**新增 API：**
- `GET /api/product/config-mount-risk` — ConfigMap 与 Secret 挂载注入风险审计
  - 缺失 ConfigMap 引用检测
  - 大型 ConfigMap 检测（>500KB）
  - 非可选挂载检测（optional: false）
  - subPath 挂载检测（阻止热更新）
  - envFrom Secret 注入检测
  - 按 namespace 统计，健康评分（0-100），3 个单元测试

---

## v16.66-v16.69 (2026-07-14)

### v16.66: 资源配额与 LimitRange 安全审计 (维度4: 安全)

**新增 API：**
- `GET /api/security/quota-security` — 资源配额与 LimitRange 安全审计
  - 检测无 ResourceQuota 的 namespace（DoS 资源耗尽攻击风险）
  - 检测无 LimitRange 的 namespace（无限制 pod 资源请求）
  - 检测配额压力（CPU/内存/Pod 使用率 >80%）
  - 按 namespace 统计，健康评分（0-100），8 个单元测试

### v16.67: PV 访问模式与多重挂载风险审计 (维度1: 产品)

**新增 API：**
- `GET /api/product/pv-access` — PV 访问模式与多重挂载风险审计
  - PV 访问模式分布（RWO/RWX/ROX）
  - 未绑定 PVC 检测（pod 卡在 Pending）
  - RWX PVC 多 pod 使用检测（数据损坏风险）
  - Delete vs Retain 回收策略检测（数据丢失风险）
  - 按 StorageClass 统计，健康评分（0-100），7 个单元测试

### v16.68: DORA 指标分析器 (维度2: 部署)

**新增 API：**
- `GET /api/deployment/dora-metrics` — DORA 指标分析
  - 部署频率（deploys/day）
  - 变更前置时间
  - 平均恢复时间（MTTR）
  - 变更失败率
  - 交付成熟度分级（elite/high/medium/low）
  - 按 namespace 成功率统计，健康评分（0-100），6 个单元测试

### v16.69: API 优先级与公平性配置审计 (维度3: 运维)

**新增 API：**
- `GET /api/operations/apf-audit` — API 优先级与公平性配置审计
  - FlowSchema 资源审计
  - PriorityLevelConfiguration 资源审计
  - 检测缺失 PriorityLevel 引用
  - 检测缺失关键 PL（global-default、leader-election、node-high）
  - 使用动态客户端访问 flowcontrol.apiserver.k8s.io/v1 CRD
  - 健康评分（0-100），2 个单元测试

---

## 统计信息

| 指标 | 数值 |
|------|------|
| Dashboard API 端点 | 221 |
| OpenAPI 端点 | 215 |
| 单元测试 | 1830 |
| Agent LLM Tools | 45 |
| 文档 | 12 篇 (7 种语言) |
| i18n 文件 | 76 个 |
| Release Assets | 17 个 |
| 镜像大小 | 28.6MB (distroless) |
| Go 版本 | 1.26+ |
