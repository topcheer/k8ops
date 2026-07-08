# 变更日志

所有重要变更记录在此文件中。版本号遵循语义化版本规范。

---

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
| OpenAPI 端点 | 145 |
| 单元测试 | 938 |
| 文档 | 12 篇 (7 种语言) |
| i18n 文件 | 76 个 |
| Release Assets | 17 个 |
| 镜像大小 | 28.6MB (distroless) |
| Go 版本 | 1.26+ |
