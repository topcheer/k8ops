# Kubernetes 版本与发行版兼容性矩阵

## 最低版本要求

| 要求 | 版本 | 原因 |
|------|------|------|
| **最低 Kubernetes 版本** | **1.25** | k8ops 使用 `policy/v1` PDB API（GA 于 1.25，`v1beta1` 在 1.25 移除） |

## API 版本依赖

| API 资源 | 使用的版本 | GA 版本 | beta 移除版本 |
|----------|-----------|---------|--------------|
| Pods, Services, Nodes, Secrets, ConfigMaps | `core/v1` | 1.0 | — |
| Deployments, StatefulSets, DaemonSets, ReplicaSets | `apps/v1` | 1.9 | — |
| HPA | `autoscaling/v2` | 1.23 | 1.26 |
| Ingress | `networking.k8s.io/v1` | 1.19 | 1.22 |
| **PDB** | `policy/v1` | **1.25** | **1.25** |
| CronJob | `batch/v1` | 1.21 | 1.25 |
| EndpointSlice | `discovery.k8s.io/v1` | 1.21 | 1.25 |
| NetworkPolicy | `networking.k8s.io/v1` | 1.19 | — |
| StorageClass | `storage.k8s.io/v1` | 1.6 | — |
| RBAC | `rbac.authorization.k8s.io/v1` | 1.8 | — |
| PodMetrics | `metrics.k8s.io/v1beta1` | 仍是 beta | — |

## 支持的发行版

### 云托管 (Managed Cloud)

| 发行版 | 云厂商 | ProviderID 格式 | 状态 |
|--------|--------|----------------|------|
| **EKS** | AWS | `aws://i-xxx` | ✅ 完全支持 |
| **GKE** | Google Cloud | `gce://projects/...` | ✅ 完全支持 |
| **AKS** | Azure | `azure:///subscriptions/...` | ✅ 完全支持 |
| **OKE** | Oracle Cloud | `oci://...` | ✅ 完全支持 |
| **ACK** | 阿里云 | `alicloud://...` | ✅ 完全支持 |
| **TKE** | 腾讯云 | `tencent://...` | ✅ 完全支持 |
| **DOKS** | DigitalOcean | `digitalocean://...` | ✅ 完全支持 |
| **LKE** | Linode/Akamai | `linode://...` | ✅ 完全支持 |

### 自建/私有化 (Self-Hosted & Private)

| 发行版 | 类型 | 特征 | 状态 |
|--------|------|------|------|
| **Vanilla k8s** | kubeadm | 无版本后缀 | ✅ 完全支持 |
| **k3s** | 轻量级 | `v1.x+k3sN` | ✅ 完全支持 |
| **RKE2** | 企业级 | `v1.x+rke2rN` | ✅ 完全支持 |
| **RKE1** | (旧版) | `v1.x+rkeN` | ✅ 完全支持 |
| **OpenShift (OCP)** | 企业级 | 节点含 `openshift` 标签 | ✅ 完全支持 |
| **Talos Linux** | 不可变 OS | 节点含 `talos.dev/version` 标签 | ✅ 完全支持 |
| **MicroK8s** | 轻量级 | 版本含 `microk8s` | ✅ 完全支持 |
| **Minikube** | 本地开发 | 节点名/标签含 `minikube` | ✅ 完全支持 |
| **Kind** | 本地开发 | 节点名/标签含 `kind` | ✅ 完全支持 |
| **KK8s** | 自建 | 版本含 `kk8s` | ✅ 完全支持 |

### 基础设施提供商

| 基础设施 | ProviderID 格式 | 状态 |
|----------|----------------|------|
| **vSphere** | `vsphere://...` | ✅ 支持 |
| **OpenStack** | `openstack://...` | ✅ 支持 |
| **Bare Metal** | 无 ProviderID | ✅ 支持（自动检测） |
| **HuaweiCloud** | `huaweicloud://...` | ✅ 支持 |
| **Baidu Cloud** | `bcc://...` | ✅ 支持 |
| **CloudStack** | `cloudstack://...` | ✅ 支持 |
| **Scaleway** | `scaleway://...` | ✅ 支持 |

## 自动检测能力

k8ops 在启动和运行时自动检测以下信息：

### 通过 `/api/compatibility` API

```json
{
  "cloudProvider": "aws",
  "distribution": "eks",
  "managed": true,
  "k8sVersion": "v1.28.4-eks-123456",
  "minVersion": "1.25",
  "compatible": true,
  "features": ["multi-zone", "cloud-provider-managed", "arm64-nodes"],
  "nodeInfo": {
    "totalNodes": 5,
    "controlPlane": 1,
    "worker": 4,
    "bareMetalNodes": 0,
    "virtualNodes": 5
  }
}
```

### 通过 `/api/cluster/overview` API

兼容性信息已内嵌在集群概览响应中（`compatibility` 字段）。

## 检测方法

| 检测项 | 方法 |
|--------|------|
| 云厂商 | 从 `Node.Spec.ProviderID` 前缀解析 (`aws://`, `gce://`, `azure://`...) |
| 发行版 | 从 server version 字符串后缀 + 节点标签 |
| 裸金属 | 节点无 ProviderID 时判定为 bare-metal |
| 控制面节点 | `node-role.kubernetes.io/control-plane` 或 `master` 标签 |
| ARM64 节点 | `Node.Status.NodeInfo.Architecture == "arm64"` |
| Windows 节点 | `Node.Status.NodeInfo.OperatingSystem == "windows"` |
| GPU 节点 | `nvidia.com/gpu.present` 标签 |
| 版本兼容性 | 解析 `major.minor` 并与最低要求 1.25 对比 |

## 成本估算配置

k8ops 支持基于云厂商的成本估算。可通过环境变量手动配置或依赖自动检测：

```bash
# 手动指定云厂商定价
K8OPS_CLOUD_PROVIDER=aws  # aws | azure | gcp | default

# 自定义 CPU/内存单价
K8OPS_CPU_PRICE=31.0      # USD/core/month
K8OPS_RAM_PRICE=4.0       # USD/GB/month
```

## Helm Chart 约束

从 Helm Chart v1.1.0 起，`Chart.yaml` 包含 `kubeVersion: ">=1.25.0-0"` 约束。
Helm 会在部署时校验目标集群版本，低于 1.25 时拒绝安装。

## 已知限制

1. **多云混合集群**: 当前仅报告第一个检测到的云厂商
2. **OpenShift SCC**: OpenShift 的 Security Context Constraints (SCC) 需要额外配置
3. **Pod Security Admission**: k8ops 的 exec/patch 操作可能受 Pod Security Admission 策略限制
4. **metrics-server**: 需要 metrics-server 已部署才能使用 top/metrics 功能
5. **node-role.kubernetes.io/master**: k8s 1.24+ 使用 `control-plane` 标签替代 `master`，k8ops 两者都支持
