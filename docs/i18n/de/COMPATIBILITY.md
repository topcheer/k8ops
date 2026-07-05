# Kubernetes-Versions- und Distributions-Kompatibilitätsmatrix

## Mindestversionsanforderungen

| Anforderung | Version | Grund |
|-------------|---------|-------|
| **Mindest Kubernetes-Version** | **1.25** | k8ops verwendet die `policy/v1` PDB-API (GA in 1.25, `v1beta1` in 1.25 entfernt) |

## API-Versionsabhängigkeiten

| API-Ressource | Verwendete Version | GA-Version | Beta-Entfernung |
|---------------|-------------------|------------|-----------------|
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
| PodMetrics | `metrics.k8s.io/v1beta1` | noch beta | — |

## Unterstützte Distributionen

### Cloud-verwaltet (Managed Cloud)

| Distribution | Cloud-Anbieter | ProviderID-Format | Status |
|--------------|---------------|-------------------|--------|
| **EKS** | AWS | `aws://i-xxx` | Voll unterstützt |
| **GKE** | Google Cloud | `gce://projects/...` | Voll unterstützt |
| **AKS** | Azure | `azure:///subscriptions/...` | Voll unterstützt |
| **OKE** | Oracle Cloud | `oci://...` | Voll unterstützt |
| **ACK** | Alibaba Cloud | `alicloud://...` | Voll unterstützt |
| **TKE** | Tencent Cloud | `tencent://...` | Voll unterstützt |
| **DOKS** | DigitalOcean | `digitalocean://...` | Voll unterstützt |
| **LKE** | Linode/Akamai | `linode://...` | Voll unterstützt |

### Self-Hosted und Privat

| Distribution | Typ | Merkmal | Status |
|--------------|-----|---------|--------|
| **Vanilla k8s** | kubeadm | Kein Versionssuffix | Voll unterstützt |
| **k3s** | Leichtgewichtig | `v1.x+k3sN` | Voll unterstützt |
| **RKE2** | Enterprise | `v1.x+rke2rN` | Voll unterstützt |
| **RKE1** | (veraltet) | `v1.x+rkeN` | Voll unterstützt |
| **OpenShift (OCP)** | Enterprise | Knoten haben `openshift`-Label | Voll unterstützt |
| **Talos Linux** | Unveränderliches OS | Knoten haben `talos.dev/version`-Label | Voll unterstützt |
| **MicroK8s** | Leichtgewichtig | Version enthält `microk8s` | Voll unterstützt |
| **Minikube** | Lokale Entwicklung | Knotenname/Label enthält `minikube` | Voll unterstützt |
| **Kind** | Lokale Entwicklung | Knotenname/Label enthält `kind` | Voll unterstützt |
| **KK8s** | Self-Hosted | Version enthält `kk8s` | Voll unterstützt |

### Infrastruktur-Anbieter

| Infrastruktur | ProviderID-Format | Status |
|---------------|-------------------|--------|
| **vSphere** | `vsphere://...` | Unterstützt |
| **OpenStack** | `openstack://...` | Unterstützt |
| **Bare Metal** | Keine ProviderID | Unterstützt (automatische Erkennung) |
| **HuaweiCloud** | `huaweicloud://...` | Unterstützt |
| **Baidu Cloud** | `bcc://...` | Unterstützt |
| **CloudStack** | `cloudstack://...` | Unterstützt |
| **Scaleway** | `scaleway://...` | Unterstützt |

## Automatische Erkennungsfähigkeiten

k8ops erkennt beim Start und zur Laufzeit automatisch folgende Informationen:

### Über die `/api/compatibility`-API

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

### Über die `/api/cluster/overview`-API

Kompatibilitätsinformationen sind in die Cluster-Übersichtsantwort eingebettet (`compatibility`-Feld).

## Erkennungsmethoden

| Erkennungspunkt | Methode |
|-----------------|---------|
| Cloud-Anbieter | Aus `Node.Spec.ProviderID`-Präfix geparst (`aws://`, `gce://`, `azure://`...) |
| Distribution | Aus Server-Versionszeichenfolgen-Suffix + Knoten-Labels |
| Bare Metal | Knoten ohne ProviderID als Bare-Metal eingestuft |
| Control-Plane-Knoten | `node-role.kubernetes.io/control-plane` oder `master`-Label |
| ARM64-Knoten | `Node.Status.NodeInfo.Architecture == "arm64"` |
| Windows-Knoten | `Node.Status.NodeInfo.OperatingSystem == "windows"` |
| GPU-Knoten | `nvidia.com/gpu.present`-Label |
| Versionskompatibilität | `major.minor` geparst und mit Mindestanforderung 1.25 verglichen |

## Kostenschätzungs-Konfiguration

k8ops unterstützt cloud-anbieterbasierte Kostenschätzungen. Kann über Umgebungsvariablen manuell konfiguriert oder auf automatische Erkennung vertraut werden:

```bash
# Cloud-Anbieter-Preise manuell angeben
K8OPS_CLOUD_PROVIDER=aws  # aws | azure | gcp | default

# Benutzerdefinierte CPU/Speicher-Preise
K8OPS_CPU_PRICE=31.0      # USD/Kern/Monat
K8OPS_RAM_PRICE=4.0       # USD/GB/Monat
```

## Helm-Chart-Einschränkungen

Ab Helm-Chart v1.1.0 enthält `Chart.yaml` die Einschränkung `kubeVersion: ">=1.25.0-0"`.
Helm validiert die Cluster-Version bei der Bereitstellung und lehnt die Installation bei Versionen unter 1.25 ab.

## Bekannte Einschränkungen

1. **Multi-Cloud-Cluster**: Aktuell wird nur der erste erkannte Cloud-Anbieter gemeldet
2. **OpenShift SCC**: Die Security Context Constraints (SCC) von OpenShift erfordern zusätzliche Konfiguration
3. **Pod Security Admission**: k8ops-Exec-/Patch-Operationen können durch Pod-Security-Admission-Richtlinien eingeschränkt sein
4. **metrics-server**: metrics-server muss bereitgestellt sein, um Top-/Metrik-Funktionen zu nutzen
5. **node-role.kubernetes.io/master**: K8s 1.24+ verwendet `control-plane`-Label statt `master`, k8ops unterstützt beide
