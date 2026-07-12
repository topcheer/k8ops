# 变更日志

所有重要变更记录在此文件中。版本号遵循语义化版本规范。

---

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

## 统计信息

| 指标 | 数值 |
|------|------|
| OpenAPI 端点 | 154 |
| 单元测试 | 990 |
| 文档 | 12 篇 (7 种语言) |
| i18n 文件 | 76 个 |
| Release Assets | 17 个 |
| 镜像大小 | 28.6MB (distroless) |
| Go 版本 | 1.26+ |
