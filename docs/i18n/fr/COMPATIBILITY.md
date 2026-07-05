# Matrice de compatibilité des versions et distributions Kubernetes

## Version minimale requise

| Exigence | Version | Raison |
|----------|---------|--------|
| **Version Kubernetes minimale** | **1.25** | k8ops utilise l'API PDB `policy/v1` (GA dans 1.25, `v1beta1` supprimé en 1.25) |

## Dépendances de version d'API

| Ressource API | Version utilisée | Version GA | Version de suppression beta |
|---------------|-----------------|------------|-----------------------------|
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
| PodMetrics | `metrics.k8s.io/v1beta1` | Toujours en beta | — |

## Distributions prises en charge

### Cloud géré (Managed Cloud)

| Distribution | Fournisseur cloud | Format ProviderID | Statut |
|--------------|-------------------|-------------------|--------|
| **EKS** | AWS | `aws://i-xxx` | Prise en charge complète |
| **GKE** | Google Cloud | `gce://projects/...` | Prise en charge complète |
| **AKS** | Azure | `azure:///subscriptions/...` | Prise en charge complète |
| **OKE** | Oracle Cloud | `oci://...` | Prise en charge complète |
| **ACK** | Alibaba Cloud | `alicloud://...` | Prise en charge complète |
| **TKE** | Tencent Cloud | `tencent://...` | Prise en charge complète |
| **DOKS** | DigitalOcean | `digitalocean://...` | Prise en charge complète |
| **LKE** | Linode/Akamai | `linode://...` | Prise en charge complète |

### Auto-hébergé et privé (Self-Hosted & Private)

| Distribution | Type | Caractéristique | Statut |
|--------------|------|-----------------|--------|
| **Vanilla k8s** | kubeadm | Pas de suffixe de version | Prise en charge complète |
| **k3s** | Léger | `v1.x+k3sN` | Prise en charge complète |
| **RKE2** | Niveau entreprise | `v1.x+rke2rN` | Prise en charge complète |
| **RKE1** | (obsolète) | `v1.x+rkeN` | Prise en charge complète |
| **OpenShift (OCP)** | Niveau entreprise | Nœuds avec label `openshift` | Prise en charge complète |
| **Talos Linux** | OS immuable | Nœuds avec label `talos.dev/version` | Prise en charge complète |
| **MicroK8s** | Léger | Version contenant `microk8s` | Prise en charge complète |
| **Minikube** | Développement local | Nom/label de nœud contenant `minikube` | Prise en charge complète |
| **Kind** | Développement local | Nom/label de nœud contenant `kind` | Prise en charge complète |
| **KK8s** | Auto-construit | Version contenant `kk8s` | Prise en charge complète |

### Fournisseurs d'infrastructure

| Infrastructure | Format ProviderID | Statut |
|----------------|-------------------|--------|
| **vSphere** | `vsphere://...` | Pris en charge |
| **OpenStack** | `openstack://...` | Pris en charge |
| **Bare Metal** | Pas de ProviderID | Pris en charge (détection automatique) |
| **HuaweiCloud** | `huaweicloud://...` | Pris en charge |
| **Baidu Cloud** | `bcc://...` | Pris en charge |
| **CloudStack** | `cloudstack://...` | Pris en charge |
| **Scaleway** | `scaleway://...` | Pris en charge |

## Capacités de détection automatique

k8ops détecte automatiquement les informations suivantes au démarrage et pendant l'exécution :

### Via l'API `/api/compatibility`

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

### Via l'API `/api/cluster/overview`

Les informations de compatibilité sont intégrées dans la réponse de vue d'ensemble du cluster (champ `compatibility`).

## Méthodes de détection

| Élément détecté | Méthode |
|-----------------|---------|
| Fournisseur cloud | Analyse du préfixe `Node.Spec.ProviderID` (`aws://`, `gce://`, `azure://`...) |
| Distribution | Suffixe de la chaîne de version du serveur + labels de nœud |
| Bare metal | Nœud sans ProviderID → classé comme bare-metal |
| Nœud de plan de contrôle | Label `node-role.kubernetes.io/control-plane` ou `master` |
| Nœuds ARM64 | `Node.Status.NodeInfo.Architecture == "arm64"` |
| Nœuds Windows | `Node.Status.NodeInfo.OperatingSystem == "windows"` |
| Nœuds GPU | Label `nvidia.com/gpu.present` |
| Compatibilité de version | Analyse de `major.minor` et comparaison avec le minimum requis 1.25 |

## Configuration de l'estimation des coûts

k8ops prend en charge l'estimation des coûts basée sur le fournisseur cloud. Configurable manuellement via les variables d'environnement ou par détection automatique :

```bash
# Spécifier manuellement la tarification du fournisseur cloud
K8OPS_CLOUD_PROVIDER=aws  # aws | azure | gcp | default

# Personnaliser le prix unitaire CPU/mémoire
K8OPS_CPU_PRICE=31.0      # USD/core/mois
K8OPS_RAM_PRICE=4.0       # USD/GB/mois
```

## Contraintes du Helm Chart

À partir du Helm Chart v1.1.0, le fichier `Chart.yaml` contient la contrainte `kubeVersion: ">=1.25.0-0"`.
Helm valide la version du cluster cible au moment du déploiement et refuse l'installation si la version est inférieure à 1.25.

## Limitations connues

1. **Clusters multi-cloud hybrides** : Actuellement, seul le premier fournisseur cloud détecté est signalé
2. **OpenShift SCC** : Les Security Context Constraints (SCC) d'OpenShift nécessitent une configuration supplémentaire
3. **Pod Security Admission** : Les opérations exec/patch de k8ops peuvent être restreintes par les stratégies Pod Security Admission
4. **metrics-server** : metrics-server doit être déployé pour utiliser les fonctionnalités top/metrics
5. **node-role.kubernetes.io/master** : k8s 1.24+ utilise le label `control-plane` au lieu de `master`, k8ops prend en charge les deux
