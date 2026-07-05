# Matriz de Compatibilidad de Versiones y Distribuciones de Kubernetes

## Requisitos de Versión Mínima

| Requisito | Versión | Motivo |
|------|------|------|
| **Versión mínima de Kubernetes** | **1.25** | k8ops utiliza la API PDB `policy/v1` (GA en 1.25, `v1beta1` eliminada en 1.25) |

## Dependencias de Versión de API

| Recurso de API | Versión utilizada | Versión GA | Versión de eliminación beta |
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
| PodMetrics | `metrics.k8s.io/v1beta1` | Sigue siendo beta | — |

## Distribuciones Soportadas

### Nube Gestionada (Managed Cloud)

| Distribución | Proveedor Cloud | Formato ProviderID | Estado |
|--------|--------|----------------|------|
| **EKS** | AWS | `aws://i-xxx` | ✅ Totalmente soportado |
| **GKE** | Google Cloud | `gce://projects/...` | ✅ Totalmente soportado |
| **AKS** | Azure | `azure:///subscriptions/...` | ✅ Totalmente soportado |
| **OKE** | Oracle Cloud | `oci://...` | ✅ Totalmente soportado |
| **ACK** | Alibaba Cloud | `alicloud://...` | ✅ Totalmente soportado |
| **TKE** | Tencent Cloud | `tencent://...` | ✅ Totalmente soportado |
| **DOKS** | DigitalOcean | `digitalocean://...` | ✅ Totalmente soportado |
| **LKE** | Linode/Akamai | `linode://...` | ✅ Totalmente soportado |

### Autohospedado y Privado (Self-Hosted & Private)

| Distribución | Tipo | Característica | Estado |
|--------|------|------|------|
| **Vanilla k8s** | kubeadm | Sin sufijo de versión | ✅ Totalmente soportado |
| **k3s** | Ligero | `v1.x+k3sN` | ✅ Totalmente soportado |
| **RKE2** | Nivel empresarial | `v1.x+rke2rN` | ✅ Totalmente soportado |
| **RKE1** | (Versión anterior) | `v1.x+rkeN` | ✅ Totalmente soportado |
| **OpenShift (OCP)** | Nivel empresarial | Nodos con etiqueta `openshift` | ✅ Totalmente soportado |
| **Talos Linux** | SO inmutable | Nodos con etiqueta `talos.dev/version` | ✅ Totalmente soportado |
| **MicroK8s** | Ligero | Versión con `microk8s` | ✅ Totalmente soportado |
| **Minikube** | Desarrollo local | Nombre/etiqueta de nodo con `minikube` | ✅ Totalmente soportado |
| **Kind** | Desarrollo local | Nombre/etiqueta de nodo con `kind` | ✅ Totalmente soportado |
| **KK8s** | Autohospedado | Versión con `kk8s` | ✅ Totalmente soportado |

### Proveedores de Infraestructura

| Infraestructura | Formato ProviderID | Estado |
|----------|----------------|------|
| **vSphere** | `vsphere://...` | ✅ Soportado |
| **OpenStack** | `openstack://...` | ✅ Soportado |
| **Bare Metal** | Sin ProviderID | ✅ Soportado (detección automática) |
| **HuaweiCloud** | `huaweicloud://...` | ✅ Soportado |
| **Baidu Cloud** | `bcc://...` | ✅ Soportado |
| **CloudStack** | `cloudstack://...` | ✅ Soportado |
| **Scaleway** | `scaleway://...` | ✅ Soportado |

## Capacidades de Detección Automática

k8ops detecta automáticamente la siguiente información durante el inicio y en tiempo de ejecución:

### Mediante la API `/api/compatibility`

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

### Mediante la API `/api/cluster/overview`

La información de compatibilidad está incrustada en la respuesta del resumen del clúster (campo `compatibility`).

## Métodos de Detección

| Elemento detectado | Método |
|--------|------|
| Proveedor Cloud | Análisis del prefijo de `Node.Spec.ProviderID` (`aws://`, `gce://`, `azure://`...) |
| Distribución | Sufijo de la cadena de versión del servidor + etiquetas de nodo |
| Bare Metal | Nodos sin ProviderID se clasifican como bare-metal |
| Nodos del plano de control | Etiqueta `node-role.kubernetes.io/control-plane` o `master` |
| Nodos ARM64 | `Node.Status.NodeInfo.Architecture == "arm64"` |
| Nodos Windows | `Node.Status.NodeInfo.OperatingSystem == "windows"` |
| Nodos GPU | Etiqueta `nvidia.com/gpu.present` |
| Compatibilidad de versión | Análisis de `major.minor` y comparación con el requisito mínimo de 1.25 |

## Configuración de Estimación de Costos

k8ops soporta estimación de costos basada en el proveedor cloud. Se puede configurar manualmente mediante variables de entorno o depender de la detección automática:

```bash
# Especificar manualmente el proveedor cloud y precios
K8OPS_CLOUD_PROVIDER=aws  # aws | azure | gcp | default

# Personalizar precios unitarios de CPU/memoria
K8OPS_CPU_PRICE=31.0      # USD/núcleo/mes
K8OPS_RAM_PRICE=4.0       # USD/GB/mes
```

## Restricciones del Helm Chart

A partir del Helm Chart v1.1.0, `Chart.yaml` incluye la restricción `kubeVersion: ">=1.25.0-0"`.
Helm valida la versión del clúster de destino durante el despliegue y rechaza la instalación si es inferior a 1.25.

## Limitaciones Conocidas

1. **Clústeres multi-cloud híbridos**: Actualmente solo reporta el primer proveedor cloud detectado
2. **SCC de OpenShift**: Los Security Context Constraints (SCC) de OpenShift requieren configuración adicional
3. **Pod Security Admission**: Las operaciones exec/patch de k8ops pueden verse limitadas por las políticas de Pod Security Admission
4. **metrics-server**: Se requiere que metrics-server esté desplegado para utilizar las funcionalidades de top/metrics
5. **node-role.kubernetes.io/master**: k8s 1.24+ utiliza la etiqueta `control-plane` en lugar de `master`; k8ops soporta ambas
