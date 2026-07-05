# Guía de Despliegue de k8ops

## Instalación y Desinstalación con Un Solo Comando

### Requisitos Previos

- Kubernetes 1.24+ (k3s / k8s / EKS / GKE / AKS son todos compatibles)
- kubectl configurado y capaz de conectar al clúster
- Repositorio de imágenes de contenedor local o remoto (por defecto se usa `registry.iot2.win`)
- Opcional: LLM API Key (OpenAI / DeepSeek / ZAI u otras interfaces compatibles)

---

## Método 1: Modo Deployment (Recomendado)

Deployment de réplica única, adecuado para la mayoría de los escenarios. Incluye Ingress, Service, ConfigMap, Secret y RBAC; un solo comando completa todo el despliegue.

### Instalación

```bash
# Red local (ya incluye todas las configuraciones: dominio, imagen, CORS, etc.)
kubectl apply -k config/deploy/overlays/local

# O un overlay personalizado
cp -r config/deploy/overlays/local config/deploy/overlays/myorg
# Editar myorg/kustomization.yaml: reemplazar dirección de imagen, dominio, CORS, etc.
kubectl apply -k config/deploy/overlays/myorg
```

### Verificación

```bash
# Verificar estado del Pod
kubectl get pods -n k8ops-system

# Verificar Ingress
kubectl get ingress -n k8ops-system

# Acceder al Dashboard
# Abrir en el navegador https://<su-dominio>  (p. ej. https://k8ops.iot2.win)
# Inicio de sesión predeterminado: admin / admin (se solicitará cambiar la contraseña en el primer inicio de sesión)
```

### Desinstalación

```bash
kubectl delete -k config/deploy/overlays/local
```

---

## Método 2: Modo DaemonSet

Un Pod se ejecuta en cada nodo, soportando diagnóstico a nivel de nodo (hostPID, hostPath). Adecuado para escenarios que requieren monitorización profunda del nodo.

### Instalación

```bash
kubectl apply -f config/daemonset-local.yaml
```

### Verificación

```bash
# Verificar DaemonSet (un Pod por nodo)
kubectl get ds -n k8ops-system
kubectl get pods -n k8ops-system -o wide

# Acceder al Dashboard (vía Service ClusterIP o Ingress)
kubectl get svc k8ops-dashboard -n k8ops-system
```

### Desinstalación

```bash
kubectl delete -f config/daemonset-local.yaml
```

---

## Método 3: Script install.sh

```bash
# Instalar (detección automática del entorno, selección interactiva Deployment / DaemonSet)
./install.sh install

# Desinstalar
./install.sh uninstall

# Ver estado
./install.sh status
```

---

## Construcción y Publicación de Imágenes

```bash
# Construcción local (amd64, para nodos del clúster)
docker buildx build --platform linux/amd64 \
  --build-arg VERSION=v1.0.0 \
  -t registry.iot2.win/k8ops:v1.0.0 \
  -t registry.iot2.win/k8ops:latest \
  --load .

# Publicar al registry
docker push registry.iot2.win/k8ops:v1.0.0
docker push registry.iot2.win/k8ops:latest
```

### Construcción Multi-arquitectura

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  --build-arg VERSION=v1.0.0 \
  -t registry.iot2.win/k8ops:v1.0.0 \
  --push .
```

---

## Configuración del Proveedor LLM

### Método 1: Configuración desde el Dashboard (Recomendado)

1. Inicie sesión en el Dashboard → pestaña **Settings**
2. Complete el tipo de Provider, API Key, Endpoint y Model
3. Haga clic en **Save**, se persiste automáticamente en K8s ConfigMap/Secret

### Método 2: Variables de Entorno

Establezca en el ConfigMap del overlay:

```yaml
configMapGenerator:
- name: k8ops-config
  literals:
  - PROVIDER_TYPE=openai          # openai / deepseek / zai / anthropic
  - PROVIDER_MODEL=gpt-4o
  - PROVIDER_ENDPOINT=https://api.openai.com/v1
```

API Key mediante Secret:

```yaml
secretGenerator:
- name: k8ops-credentials
  literals:
  - api-key=sk-your-api-key-here
```

### Proveedores Soportados

| Proveedor | Endpoint | Modelo de Ejemplo |
|----------|----------|------------|
| OpenAI | `https://api.openai.com/v1` | gpt-4o, gpt-4o-mini |
| DeepSeek | `https://api.deepseek.com/v1` | deepseek-chat |
| ZAI (Zhipu) | `https://open.bigmodel.cn/api/paas/v4` | glm-4-flash, glm-4-plus |
| Anthropic | `https://api.anthropic.com/v1` | claude-3-5-sonnet |
| Local | `http://localhost:11434/v1` | llama3, qwen2 |

---

## Configuración de Autenticación

### Autenticación Local (Predeterminada)

Lista para usar, los usuarios se almacenan en SQLite. Primer inicio de sesión: `admin / admin`.

### LDAP

```yaml
# Establecer en ConfigMap o configuración del Provider
LDAP_SERVER=ldap://your-ldap:389
LDAP_BIND_DN=cn=admin,dc=example,dc=com
LDAP_BIND_PASSWORD=secret
LDAP_USER_BASE=ou=users,dc=example,dc=com
LDAP_SKIP_TLS_VERIFY=false   # En producción, debe ser false
```

### OIDC (GitHub / Google / Keycloak, etc.)

```yaml
# Configuración del Provider (página Dashboard Settings o CRD)
OIDC_ISSUER=https://your-keycloak/realms/myrealm
OIDC_CLIENT_ID=k8ops
OIDC_CLIENT_SECRET=your-secret
OIDC_REDIRECT_URL=https://k8ops.iot2.win/auth/oidc/callback
```

---

## Ingress y TLS

### TLS Automático (cert-manager + Let's Encrypt)

Asegúrese de que cert-manager esté instalado en el clúster y añada la anotación al Ingress:

```yaml
annotations:
  cert-manager.io/cluster-issuer: letsencrypt-prod
```

### Usar un Certificado TLS Existente

```bash
kubectl create secret tls k8ops-dashboard-tls \
  --cert=fullchain.pem \
  --key=privkey.pem \
  -n k8ops-system
```

---

## Preguntas Frecuentes

### Pod en Estado Pending Permanente

```bash
# Verificar la causa del fallo de programación
kubectl describe pod <pod-name> -n k8ops-system | tail -10

# Causas comunes:
# - Conflicto de puerto hostNetwork → eliminar hostNetwork: true o evitar conflictos de puertos
# - Recursos insuficientes → ajustar resources.requests/limits
# - Taints de nodo → verificar tolerations
```

### Dashboard Devuelve 502

```bash
# 1. Verificar si el Pod está Ready
kubectl get pods -n k8ops-system

# 2. Verificar los endpoints del Service
kubectl get endpoints k8ops-dashboard -n k8ops-system

# 3. Verificar el backend del Ingress
kubectl describe ingress -n k8ops-system

# 4. Esperar a que el Pod esté completamente listo y reintentar
```

### Fallo al Descargar Imagen

```bash
# Solución 1: establecer imagePullPolicy: Always (recomendado cuando se usa tag específico)
# Solución 2: asegurar que los nodos tienen la confianza TLS del registry configurada
# Solución 3: si usa un registry privado, crear imagePullSecrets
```

### LLM API 401

```bash
# Verificar si la API Key está correctamente configurada
kubectl get secret k8ops-credentials -n k8ops-system -o jsonpath='{.data.api-key}' | base64 -d

# O reconfigurar el Provider en Dashboard → Settings
```

---

## Actualización

```bash
# Construir y publicar la nueva imagen
docker buildx build --platform linux/amd64 \
  --build-arg VERSION=v2.0.0 \
  -t registry.iot2.win/k8ops:v2.0.0 \
  -t registry.iot2.win/k8ops:latest \
  --push .

# Actualización continua (modo Deployment)
kubectl set image deployment/k8ops \
  k8ops=registry.iot2.win/k8ops:v2.0.0 \
  -n k8ops-system

# O modificar newTag en el overlay y reaplicar
kubectl apply -k config/deploy/overlays/local

# Modo DaemonSet
kubectl set image daemonset/k8ops \
  k8ops=registry.iot2.win/k8ops:v2.0.0 \
  -n k8ops-system
```

---

## Copia de Seguridad y Restauración de Datos

### Copia de Seguridad Automática de SQLite (CronJob)

k8ops utiliza SQLite para almacenar usuarios, logs de auditoría y datos de sesiones. Se recomienda una copia de seguridad automática cada hora:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: k8ops-backup
  namespace: k8ops-system
spec:
  schedule: "0 * * * *"  # Cada hora en punto
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: backup
            image: busybox
            command:
            - sh
            - -c
            - |
              TIMESTAMP=$(date +%Y%m%d-%H%M%S)
              cp /data/k8ops.db /backup/k8ops-${TIMESTAMP}.db
              # Conservar los últimos 24 respaldos
              ls -t /backup/k8ops-*.db | tail -n +25 | xargs rm -f
            volumeMounts:
            - name: data
              mountPath: /data
              readOnly: true
            - name: backup
              mountPath: /backup
          volumes:
          - name: data
            persistentVolumeClaim:
              claimName: k8ops-data
          - name: backup
            hostPath:
              path: /var/lib/k8ops-backup
              type: DirectoryOrCreate
          restartPolicy: OnFailure
```

### Copia de Seguridad Manual

```bash
# Copiar la base de datos desde el Pod
kubectl cp k8ops-system/<pod-name>:/data/k8ops.db ./k8ops-backup-$(date +%Y%m%d).db

# O usar la copia de seguridad en línea de sqlite3 (sin interrumpir escrituras)
kubectl exec -n k8ops-system <pod-name> -- sqlite3 /data/k8ops.db ".backup /data/k8ops-backup.db"
kubectl cp k8ops-system/<pod-name>:/data/k8ops-backup.db ./k8ops-backup.db
```

### Restauración

```bash
# Detener k8ops
kubectl scale deployment k8ops -n k8ops-system --replicas=0

# Restaurar la base de datos
kubectl cp ./k8ops-backup.db k8ops-system/<pod-name>:/data/k8ops.db

# Reiniciar
kubectl scale deployment k8ops -n k8ops-system --replicas=1
```

---

## Despliegue de Alta Disponibilidad (HA)

### Modo de Nodo Único (Predeterminado, adecuado para desarrollo/clústeres pequeños)

- 1 réplica + SQLite + PVC
- Breve interrupción del servicio al reiniciar el Pod (~10s)
- Adecuado para equipos de menos de 50 usuarios

### HA Multi-réplica (Recomendado para Producción)

Usar MySQL/PostgreSQL en lugar de SQLite para soportar múltiples réplicas:

1. **Cambiar la base de datos a MySQL**:

```yaml
# Establecer en el ConfigMap del overlay
configMapGenerator:
- name: k8ops-config
  literals:
  - DB_DRIVER=mysql
  - DB_DSN=k8ops:password@tcp(mysql:3306)/k8ops?charset=utf8mb4&parseTime=True
```

2. **Multi-réplica + leader election**:

```yaml
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: k8ops
        env:
        - name: LEADER_ELECT
          value: "true"
```

3. **Almacenamiento compartido**: MySQL usa un PVC independiente, los Pods de k8ops son stateless

### Planificación de Capacidad

| Escala | Número de Usuarios | Recursos Recomendados | Base de Datos |
|------|--------|----------|--------|
| Pequeña | < 20 | 1 pod, 500m CPU / 512Mi | SQLite |
| Mediana | 20-100 | 2 pods, 1 CPU / 1Gi cada uno | MySQL |
| Grande | 100+ | 3+ pods, 2 CPU / 2Gi cada uno | MySQL + separación lectura/escritura |

---

## Proceso CI/CD y Gestión de Versiones

### Script de Despliegue con Un Solo Comando

k8ops proporciona un script de despliegue automatizado que incluye verificación previa, construcción, publicación, comprobación de salud y retroceso automático:

```bash
# Desplegar una nueva versión (verificación automática + construcción + publicación + comprobación de salud)
./scripts/deploy.sh v14.36

# Flujo de despliegue:
# 1. Verificación previa: go build + go vet + go test + gofmt
# 2. Construcción: Docker buildx + push al registry
# 3. Publicación: kubectl set image + anotación change-cause
# 4. Validación: Pod Ready + HTTP 200 (120s timeout)
# 5. Retroceso: retroceso automático a la versión anterior si la comprobación de salud falla
```

### Retroceso Rápido

```bash
# Retroceder a la versión anterior
./scripts/rollback.sh

# Retroceder a una revisión específica
./scripts/rollback.sh 58

# Retroceder a un número de versión específico
./scripts/rollback.sh v14.30
```

### Seguimiento del Historial de Versiones

Cada despliegue registra automáticamente una anotación change-cause:

```bash
# Ver el historial de versiones
kubectl rollout history daemonset/k8ops -n k8ops-system

# Ver detalles de una revisión específica
kubectl rollout history daemonset/k8ops -n k8ops-system --revision=55
```

### Proceso CI (GitHub Actions)

| Workflow | Condición de Disparo | Contenido |
|--------|----------|------|
| `ci.yml` — push/PR a main | Envío de código | test + vet + lint + govulncheck + Docker build |
| `release.yml` — tag v* | Tag de versión | Pruebas completas + GoReleaser + Docker multi-arch + Notas de Versión automáticas |

### Gestión de Imágenes

| Tag | Descripción |
|------|------|
| `registry.iot2.win/k8ops:v14.XX` | Versión específica |
| `registry.iot2.win/k8ops:latest` | Última versión estable |
| `ghcr.io/<org>/k8ops:v14.XX` | Imagen GHCR (publicación CI) |

### Optimización de Imágenes

- Imagen base: `gcr.io/distroless/static-debian12:nonroot` (sin shell, sin gestor de paquetes)
- Construcción multi-etapa: Go builder + runtime distroless
- Caché BuildKit: `--mount=type=cache` acelera la construcción en CI
- Optimización de binario: `-trimpath -ldflags="-s -w"` reduce el tamaño

| Versión | Tamaño de Imagen |
|------|----------|
| v14.30 (alpine) | 31.8 MB |
| v14.35 (distroless) | 28.6 MB |

### Configuración de Alta Disponibilidad

#### PodDisruptionBudget (PDB)

Garantiza que al menos 1 Pod esté disponible durante el mantenimiento de nodos:

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: k8ops-pdb
  namespace: k8ops-system
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: k8ops
```

#### NetworkPolicy

Restringe el Dashboard para que solo acepte tráfico del Ingress Controller:

- Ingress: solo el namespace kube-system puede acceder al puerto 9090 (dashboard)
- Ingress: solo el namespace monitoring puede acceder al puerto 8080 (metrics)
- Egress: permite DNS (53), HTTPS (443), K8s API (6443)

#### PriorityClass

k8ops utiliza la prioridad `system-cluster-critical` para garantizar que no sea desalojado bajo presión de recursos.

#### Estrategia de Actualización Continua

| Modo | maxUnavailable | maxSurge | Descripción |
|------|---------------|----------|------|
| DaemonSet | 1 | - | Actualiza 1 nodo a la vez |
| Deployment | 0 | 1 | Inicia el nuevo Pod antes de eliminar el antiguo |

#### Cuotas de Recursos

| Modo | CPU Request | CPU Limit | Mem Request | Mem Limit |
|------|-------------|-----------|-------------|-----------|
| DaemonSet | 100m | 1 | 128Mi | 1Gi |
| Deployment | 500m | 2 | 512Mi | 2Gi |

#### Comprobaciones de Salud y Gestión del Ciclo de Vida

k8ops utiliza tres niveles de sondas para garantizar la fiabilidad:

| Sonda | Ruta | Función | Parámetros |
|------|------|------|------|
| **startupProbe** | `/healthz` | Esperar a que el inicio se complete (evita que un inicio lento sea eliminado por liveness) | failureThreshold: 30, period: 5s (espera máxima 150s) |
| **livenessProbe** | `/healthz` | Comprobación de actividad (si falla, reinicia el Pod) | period: 20s, failureThreshold: 3, timeout: 5s |
| **readinessProbe** | `/readyz` | Comprobación de disponibilidad (si falla, se elimina de los Endpoints del Service) | period: 10s, failureThreshold: 3, timeout: 5s |

**Cierre Elegante (Graceful Shutdown):**

```yaml
lifecycle:
  preStop:
    exec:
      command: ["/manager", "--pre-stop"]
# --pre-stop hace sleep 5s, esperando a que el Ingress Controller elimine este Pod del balanceador de carga
# Luego kubelet envía SIGTERM, activando el cierre elegante del dashboard (drenaje de conexiones SSE)
# terminationGracePeriodSeconds: 30 garantiza suficiente tiempo para completar
```

Proceso de cierre:
1. kubelet ejecuta `preStop` → sleep 5s (drenaje de conexiones)
2. kubelet envía SIGTERM → el manejador de señales de Go inicia el cierre elegante
3. El servidor HTTP del Dashboard deja de aceptar nuevas solicitudes
4. Drenaje de conexiones SSE (10s timeout)
5. El Controller Manager se cierra elegantemente
6. El proceso termina
