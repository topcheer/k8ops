# Kubernetes 버전 및 배포판 호환성 매트릭스

## 최소 버전 요구사항

| 요구사항 | 버전 | 이유 |
|------|------|------|
| **최소 Kubernetes 버전** | **1.25** | k8ops는 `policy/v1` PDB API를 사용함 (1.25에 GA, `v1beta1`은 1.25에서 제거됨) |

## API 버전 종속성

| API 리소스 | 사용 버전 | GA 버전 | beta 제거 버전 |
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
| PodMetrics | `metrics.k8s.io/v1beta1` | 여전히 beta | — |

## 지원 배포판

### 클라우드 관리형 (Managed Cloud)

| 배포판 | 클라우드 공급자 | ProviderID 형식 | 상태 |
|--------|--------|----------------|------|
| **EKS** | AWS | `aws://i-xxx` | 완전 지원 |
| **GKE** | Google Cloud | `gce://projects/...` | 완전 지원 |
| **AKS** | Azure | `azure:///subscriptions/...` | 완전 지원 |
| **OKE** | Oracle Cloud | `oci://...` | 완전 지원 |
| **ACK** | 알리바바 클라우드 | `alicloud://...` | 완전 지원 |
| **TKE** | 텐센트 클라우드 | `tencent://...` | 완전 지원 |
| **DOKS** | DigitalOcean | `digitalocean://...` | 완전 지원 |
| **LKE** | Linode/Akamai | `linode://...` | 완전 지원 |

### 자체 구축/프라이빗 (Self-Hosted & Private)

| 배포판 | 유형 | 특징 | 상태 |
|--------|------|------|------|
| **Vanilla k8s** | kubeadm | 버전 접미사 없음 | 완전 지원 |
| **k3s** | 경량 | `v1.x+k3sN` | 완전 지원 |
| **RKE2** | 엔터프라이즈 | `v1.x+rke2rN` | 완전 지원 |
| **RKE1** | (구버전) | `v1.x+rkeN` | 완전 지원 |
| **OpenShift (OCP)** | 엔터프라이즈 | 노드에 `openshift` 레이블 포함 | 완전 지원 |
| **Talos Linux** | 불변 OS | 노드에 `talos.dev/version` 레이블 포함 | 완전 지원 |
| **MicroK8s** | 경량 | 버전에 `microk8s` 포함 | 완전 지원 |
| **Minikube** | 로컬 개발 | 노드 이름/레이블에 `minikube` 포함 | 완전 지원 |
| **Kind** | 로컬 개발 | 노드 이름/레이블에 `kind` 포함 | 완전 지원 |
| **KK8s** | 자체 구축 | 버전에 `kk8s` 포함 | 완전 지원 |

### 인프라 프로바이더

| 인프라 | ProviderID 형식 | 상태 |
|----------|----------------|------|
| **vSphere** | `vsphere://...` | 지원 |
| **OpenStack** | `openstack://...` | 지원 |
| **Bare Metal** | ProviderID 없음 | 지원 (자동 감지) |
| **HuaweiCloud** | `huaweicloud://...` | 지원 |
| **Baidu Cloud** | `bcc://...` | 지원 |
| **CloudStack** | `cloudstack://...` | 지원 |
| **Scaleway** | `scaleway://...` | 지원 |

## 자동 감지 기능

k8ops는 시작 및 런타임 시 다음 정보를 자동으로 감지합니다:

### `/api/compatibility` API를 통해

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

### `/api/cluster/overview` API를 통해

호환성 정보는 클러스터 개요 응답에 내장되어 있습니다 (`compatibility` 필드).

## 감지 방법

| 감지 항목 | 방법 |
|--------|------|
| 클라우드 공급자 | `Node.Spec.ProviderID` 접두사에서 파싱 (`aws://`, `gce://`, `azure://`...) |
| 배포판 | 서버 버전 문자열 접미사 + 노드 레이블 |
| 베어메탈 | 노드에 ProviderID가 없으면 bare-metal로 판정 |
| 컨트롤 플레인 노드 | `node-role.kubernetes.io/control-plane` 또는 `master` 레이블 |
| ARM64 노드 | `Node.Status.NodeInfo.Architecture == "arm64"` |
| Windows 노드 | `Node.Status.NodeInfo.OperatingSystem == "windows"` |
| GPU 노드 | `nvidia.com/gpu.present` 레이블 |
| 버전 호환성 | `major.minor` 파싱 후 최소 요구사항 1.25와 비교 |

## 비용 추정 구성

k8ops는 클라우드 공급자 기반의 비용 추정을 지원합니다. 환경 변수로 수동 구성하거나 자동 감지에 의존할 수 있습니다:

```bash
# 수동으로 클라우드 공급자 가격 지정
K8OPS_CLOUD_PROVIDER=aws  # aws | azure | gcp | default

# CPU/메모리 단가 커스터마이즈
K8OPS_CPU_PRICE=31.0      # USD/core/month
K8OPS_RAM_PRICE=4.0       # USD/GB/month
```

## Helm Chart 제약 조건

Helm Chart v1.1.0부터 `Chart.yaml`에 `kubeVersion: ">=1.25.0-0"` 제약 조건이 포함됩니다.
Helm은 배포 시 대상 클러스터 버전을 검증하며, 1.25 미만일 경우 설치를 거부합니다.

## 알려진 제한 사항

1. **멀티 클라우드 하이브리드 클러스터**: 현재 첫 번째로 감지된 클라우드 공급자만 보고
2. **OpenShift SCC**: OpenShift의 Security Context Constraints (SCC)는 추가 구성 필요
3. **Pod Security Admission**: k8ops의 exec/patch 작업이 Pod Security Admission 정책에 의해 제한될 수 있음
4. **metrics-server**: top/metrics 기능을 사용하려면 metrics-server가 배포되어 있어야 함
5. **node-role.kubernetes.io/master**: k8s 1.24+는 `control-plane` 레이블로 `master`를 대체하며, k8ops는 둘 다 지원
