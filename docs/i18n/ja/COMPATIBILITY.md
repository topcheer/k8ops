# Kubernetes バージョンとディストリビューション互換性マトリクス

## 最低バージョン要件

| 要件 | バージョン | 理由 |
|------|------|------|
| **最低 Kubernetes バージョン** | **1.25** | k8ops は `policy/v1` PDB API を使用（1.25 で GA、`v1beta1` は 1.25 で削除） |

## API バージョン依存関係

| API リソース | 使用バージョン | GA バージョン | beta 削除バージョン |
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
| PodMetrics | `metrics.k8s.io/v1beta1` | まだ beta | — |

## サポートされるディストリビューション

### マネージドクラウド (Managed Cloud)

| ディストリビューション | クラウドプロバイダー | ProviderID 形式 | 状態 |
|--------|--------|----------------|------|
| **EKS** | AWS | `aws://i-xxx` | ✅ 完全サポート |
| **GKE** | Google Cloud | `gce://projects/...` | ✅ 完全サポート |
| **AKS** | Azure | `azure:///subscriptions/...` | ✅ 完全サポート |
| **OKE** | Oracle Cloud | `oci://...` | ✅ 完全サポート |
| **ACK** | Alibaba Cloud | `alicloud://...` | ✅ 完全サポート |
| **TKE** | Tencent Cloud | `tencent://...` | ✅ 完全サポート |
| **DOKS** | DigitalOcean | `digitalocean://...` | ✅ 完全サポート |
| **LKE** | Linode/Akamai | `linode://...` | ✅ 完全サポート |

### セルフホスト / プライベート (Self-Hosted & Private)

| ディストリビューション | タイプ | 特徴 | 状態 |
|--------|------|------|------|
| **Vanilla k8s** | kubeadm | バージョンサフィックスなし | ✅ 完全サポート |
| **k3s** | 軽量 | `v1.x+k3sN` | ✅ 完全サポート |
| **RKE2** | エンタープライズ | `v1.x+rke2rN` | ✅ 完全サポート |
| **RKE1** | (旧版) | `v1.x+rkeN` | ✅ 完全サポート |
| **OpenShift (OCP)** | エンタープライズ | ノードに `openshift` ラベルあり | ✅ 完全サポート |
| **Talos Linux** | イミュータブル OS | ノードに `talos.dev/version` ラベルあり | ✅ 完全サポート |
| **MicroK8s** | 軽量 | バージョンに `microk8s` を含む | ✅ 完全サポート |
| **Minikube** | ローカル開発 | ノード名/ラベルに `minikube` を含む | ✅ 完全サポート |
| **Kind** | ローカル開発 | ノード名/ラベルに `kind` を含む | ✅ 完全サポート |
| **KK8s** | セルフビルド | バージョンに `kk8s` を含む | ✅ 完全サポート |

### インフラストラクチャプロバイダー

| インフラ | ProviderID 形式 | 状態 |
|----------|----------------|------|
| **vSphere** | `vsphere://...` | ✅ サポート |
| **OpenStack** | `openstack://...` | ✅ サポート |
| **Bare Metal** | ProviderID なし | ✅ サポート（自動検出） |
| **HuaweiCloud** | `huaweicloud://...` | ✅ サポート |
| **Baidu Cloud** | `bcc://...` | ✅ サポート |
| **CloudStack** | `cloudstack://...` | ✅ サポート |
| **Scaleway** | `scaleway://...` | ✅ サポート |

## 自動検出機能

k8ops は起動時と実行時に以下の情報を自動検出します：

### `/api/compatibility` API 経由

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

### `/api/cluster/overview` API 経由

互換性情報はクラスターオーバービューのレスポンス（`compatibility` フィールド）に組み込まれています。

## 検出方法

| 検出項目 | 方法 |
|--------|------|
| クラウドプロバイダー | `Node.Spec.ProviderID` プレフィックスから解析 (`aws://`, `gce://`, `azure://`...) |
| ディストリビューション | サーバーバージョン文字列のサフィックス + ノードラベルから判定 |
| ベアメタル | ノードに ProviderID がない場合、ベアメタルと判定 |
| コントロールプレーンノード | `node-role.kubernetes.io/control-plane` または `master` ラベル |
| ARM64 ノード | `Node.Status.NodeInfo.Architecture == "arm64"` |
| Windows ノード | `Node.Status.NodeInfo.OperatingSystem == "windows"` |
| GPU ノード | `nvidia.com/gpu.present` ラベル |
| バージョン互換性 | `major.minor` を解析し、最低要件 1.25 と比較 |

## コスト見積もり設定

k8ops はクラウドプロバイダーに基づくコスト見積もりをサポートします。環境変数で手動設定するか、自動検出に依存できます：

```bash
# クラウドプロバイダーの価格を手動指定
K8OPS_CLOUD_PROVIDER=aws  # aws | azure | gcp | default

# CPU/メモリ単価のカスタマイズ
K8OPS_CPU_PRICE=31.0      # USD/core/month
K8OPS_RAM_PRICE=4.0       # USD/GB/month
```

## Helm Chart の制約

Helm Chart v1.1.0 以降、`Chart.yaml` に `kubeVersion: ">=1.25.0-0"` 制約が含まれています。
Helm はデプロイ時にターゲットクラスターバージョンを検証し、1.25 未満の場合はインストールを拒否します。

## 既知の制限事項

1. **マルチクラウド混在クラスター**: 現在、最初に検出されたクラウドプロバイダーのみを報告します
2. **OpenShift SCC**: OpenShift の Security Context Constraints (SCC) は追加設定が必要です
3. **Pod Security Admission**: k8ops の exec/patch 操作は Pod Security Admission ポリシーにより制限される場合があります
4. **metrics-server**: top/metrics 機能を使用するには metrics-server がデプロイされている必要があります
5. **node-role.kubernetes.io/master**: k8s 1.24+ では `control-plane` ラベルが `master` を置き換えますが、k8ops は両方をサポートします
