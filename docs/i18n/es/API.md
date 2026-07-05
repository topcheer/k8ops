# Referencia de la API de k8ops

Todos los endpoints se sirven en el puerto del dashboard (predeterminado `:9090`).

**Autenticación:** Cookie JWT (`k8ops_token`) o cabecera `Authorization: Bearer <token>`.
**Content-Type:** `application/json` para todas las solicitudes POST/PUT.

## Especificación OpenAPI 3.0

k8ops genera automáticamente la especificación OpenAPI 3.0, que se puede utilizar para generar SDKs automáticamente, integrar puertas de enlace API o navegar en Swagger UI.

| Endpoint | Descripción |
|------|------|
| `GET /api/openapi.json` | Devuelve la especificación JSON completa de OpenAPI 3.0 |
| `GET /api/docs` | Devuelve metadatos de documentación API agrupados por etiquetas (incluye spec + tagGroups) |

**Obtener la especificación:**
```bash
curl -sk https://k8ops.iot2.win/api/openapi.json -o k8ops-openapi.json
```

**Importar a Swagger Editor:**
1. Abrir https://editor.swagger.io
2. Archivo → Importar archivo → Seleccionar `k8ops-openapi.json`

**Navegar en el Dashboard:** La barra lateral → la página API Docs ofrece un navegador API interactivo con búsqueda, filtrado y prueba en línea.

---

## Salud y Sistema

| Método | Ruta | Auth | Descripción |
|--------|------|------|-------------|
| GET | `/api/health` | Ninguna | Sonda de actividad — devuelve `{"status":"ok"}` |
| GET | `/api/version` | Ninguna | Versión de compilación, commit de git, versión de Go |

## Clúster

| Método | Ruta | Auth | Descripción |
|--------|------|------|-------------|
| GET | `/api/cluster/overview` | Requerida | Resumen del clúster: número de nodos, número de pods, uso de CPU/memoria, advertencias (caché 30s) |
| GET | `/api/nodes` | Requerida | Lista de todos los nodos con uso de recursos y condiciones (caché 30s) |
| GET | `/api/nodes/{node}/pods` | Requerida | Pods en ejecución en un nodo específico |
| GET | `/api/pods` | Requerida | Lista de todos los pods en todos los namespaces (caché 30s) |
| GET | `/api/pods/{namespace}/{name}/containers` | Requerida | Lista de contenedores de un pod |
| GET | `/api/pods/{namespace}/{name}/logs?container=&follow=&tailLines=` | Requerida | Logs del pod (soporta streaming SSE con `follow=true`) |
| GET | `/api/events?namespace=&warning=` | Requerida | Eventos de Kubernetes, filtrados opcionalmente por namespace/advertencia |
| GET | `/api/resources?kind=&namespace=` | Requerida | Listador genérico de recursos (Deployments, Services, etc.) (caché 60s) |
| GET | `/api/crds?with_counts=true` | Requerida | Custom Resource Definitions (caché 10min con conteos) |
| GET | `/api/crd-resources?group=&version=&resource=&namespace=` | Requerida | Instancias de CRD (caché 60s) |
| GET | `/api/yaml?namespace=&name=&group=&version=&resource=&kind=` | Requerida | Vista YAML de cualquier recurso de Kubernetes |

## Diagnóstico y Remediación

| Método | Ruta | Auth | Descripción |
|--------|------|------|-------------|
| GET | `/api/diagnostics` | Requerida | Lista de CRs DiagnosticReport, filtro opcional `?namespace=` |
| GET | `/api/diagnostics/{namespace}/{name}` | Requerida | Detalle de diagnóstico con análisis de IA |
| GET | `/api/remediations` | Requerida | Lista de CRs de Remediación, filtro opcional `?namespace=` |
| GET | `/api/optimizations` | Requerida | Lista de CRs de Optimización, filtro opcional `?namespace=` |

## Chat de IA

| Método | Ruta | Auth | Descripción |
|--------|------|------|-------------|
| POST | `/api/chat` | Requerida | Enviar mensaje al asistente de IA (respuesta en streaming SSE) |
| GET | `/api/chat/conversations?id=` | Requerida | Listar conversaciones u obtener una por ID |

### POST /api/chat

**Solicitud:**
```json
{
  "message": "Why is my pod crashing?",
  "conversation_id": "optional-existing-id",
  "stream": true
}
```

**Respuesta:** Stream SSE de análisis de IA con llamadas de herramientas y resultados.

### GET /api/chat/conversations

Devuelve el historial de conversaciones. Pase `?id=<uuid>` para una sola conversación.

## Gestión de Proveedores

| Método | Ruta | Auth | Descripción |
|--------|------|------|-------------|
| GET | `/api/provider/status` | Requerida | Configuración actual del proveedor de IA (API key enmascarada) |
| POST | `/api/provider/update` | Requerida | Actualizar tipo/modelo/endpoint del proveedor en tiempo de ejecución |
| POST | `/api/provider/reload` | Requerida | Recargar configuración del proveedor desde el CRD K8opsConfig |
| GET | `/api/tools` | Requerida | Listar herramientas de diagnóstico registradas |

## Autenticación

| Método | Ruta | Auth | Descripción |
|--------|------|------|-------------|
| POST | `/api/auth/login` | Pública | Inicio de sesión local (con limitación de tasa) |
| POST | `/api/auth/logout` | Requerida | Borrar cookie de autenticación |
| GET | `/api/auth/me` | Requerida | Información del usuario actual |
| POST | `/api/auth/change-password` | Requerida | Cambiar la propia contraseña |
| GET | `/api/auth/status` | Pública | Estado de la configuración de autenticación (auth_enabled, user_count, flags ldap/oidc) |
| GET | `/api/auth/provider-presets` | Pública | Plantillas de proveedores OIDC/LDAP disponibles |

### POST /api/auth/login

**Solicitud:**
```json
{
  "username": "admin",
  "password": "admin"
}
```

**Respuesta (200):**
```json
{
  "user": {"id": 1, "username": "admin", "role": "admin", "display_name": "Administrator"},
  "must_change": true,
  "redirect_url": "/"
}
```

Establece la cookie `k8ops_token` (HttpOnly, SameSite=Lax, 24h).

**Error (401):**
```json
{"error": "invalid username or password"}
```

## OIDC

| Método | Ruta | Auth | Descripción |
|--------|------|------|-------------|
| GET | `/api/auth/oidc/{provider}/login` | Pública | Redirigir al proveedor OIDC (establece cookie de estado CSRF) |
| GET | `/api/auth/oidc/{provider}/callback` | Pública | Callback OIDC (valida el estado, crea sesión de usuario) |

## Gestión de Proveedores de Autenticación (Admin)

| Método | Ruta | Auth | Descripción |
|--------|------|------|-------------|
| GET | `/api/auth/providers` | Admin | Listar proveedores de autenticación configurados |
| POST | `/api/auth/providers` | Admin | Crear proveedor de autenticación (LDAP/OIDC) |
| GET | `/api/auth/providers/{id}` | Admin | Obtener proveedor por ID |
| PUT | `/api/auth/providers/{id}` | Admin | Actualizar configuración del proveedor |
| DELETE | `/api/auth/providers/{id}` | Admin | Eliminar proveedor |

## Gestión de Usuarios (Admin)

| Método | Ruta | Auth | Descripción |
|--------|------|------|-------------|
| GET | `/api/admin/users` | Admin | Listar todos los usuarios |
| POST | `/api/admin/users` | Admin | Crear usuario (rol predeterminado: viewer, MustChangePwd=true) |
| GET | `/api/admin/users/{id}` | Admin | Obtener usuario por ID |
| PUT | `/api/admin/users/{id}` | Admin | Actualizar usuario (rol, namespaces, etc.) |
| DELETE | `/api/admin/users/{id}` | Admin | Eliminar usuario |
| POST | `/api/admin/users/{id}/reset-password` | Admin | Restablecer contraseña (establece MustChangePwd=true) |
| GET | `/api/admin/auth-config` | Admin | Obtener configuración de autenticación |
| PUT | `/api/admin/auth-config` | Admin | Actualizar configuración de autenticación |

## API Keys

| Método | Ruta | Auth | Descripción |
|--------|------|------|-------------|
| GET | `/api/auth/api-keys` | Requerida | Listar las propias API keys |
| POST | `/api/auth/api-keys` | Requerida | Crear API key |
| DELETE | `/api/auth/api-keys/{id}` | Requerida | Revocar API key |

## Gestión RBAC (Admin)

| Método | Ruta | Auth | Descripción |
|--------|------|------|-------------|
| GET | `/api/rbac/clusterroles` | Admin | Listar cluster roles |
| GET | `/api/rbac/clusterroles/{name}` | Admin | Obtener cluster role por nombre |
| DELETE | `/api/rbac/clusterroles/{name}` | Admin | Eliminar cluster role |
| GET | `/api/rbac/roles?namespace=` | Admin | Listar roles con ámbito de namespace |
| GET | `/api/rbac/roles/{namespace}/{name}` | Admin | Obtener rol con ámbito de namespace |
| DELETE | `/api/rbac/roles/{namespace}/{name}` | Admin | Eliminar rol con ámbito de namespace |
| GET | `/api/rbac/rolebindings?namespace=` | Admin | Listar role bindings |
| GET | `/api/rbac/rolebindings/{namespace}/{name}` | Admin | Obtener role binding |
| DELETE | `/api/rbac/rolebindings/{namespace}/{name}` | Admin | Eliminar role binding |
| GET | `/api/rbac/api-resources` | Admin | Listar tipos de recursos de la API de Kubernetes |
| GET | `/api/rbac/namespaces` | Admin | Listar todos los namespaces |
| GET | `/api/rbac/role-mapping?role=&kind=&name=&namespace=` | Admin | Ver mapeo de rol a sujeto |
| GET | `/api/rbac/role-defs` | Admin | Listar definiciones de roles personalizados de k8ops |
| GET | `/api/rbac/subjects?kind=&namespace=` | Admin | Listar sujetos (usuarios/grupos/service accounts) |

## Auditoría

| Método | Ruta | Auth | Descripción |
|--------|------|------|-------------|
| GET | `/api/audit?namespace=&limit=` | Requerida | Entradas del log de auditoría (paginadas) |
| GET | `/api/audit/stats` | Requerida | Resumen de estadísticas de auditoría |

## Configuración

| Método | Ruta | Auth | Descripción |
|--------|------|------|-------------|
| GET | `/api/config` | Requerida | Configuración del controlador k8ops (tipo/modelo del proveedor, funciones) |

## Auditoría de Seguridad

| Método | Ruta | Auth | Descripción |
|--------|------|------|-------------|
| GET | `/api/security/audit` | Requerida | Escaneo de seguridad del clúster — verifica Pod Security Standards, RBAC, cobertura de políticas de red, seguridad de secretos |
| GET | `/api/security/health` | Requerida | Comprobación de salud de seguridad de la plataforma — autenticación/TLS/conectividad con la API de K8s |

### GET /api/security/audit

Escanea todo el clúster y devuelve una lista de hallazgos de seguridad, ordenados por gravedad (critical > high > medium > low > info).

**Elementos de verificación:**
- **Pod Security:** Contenedores privilegiados, ejecución como root, escalada de privilegios, capabilities peligrosas, hostPath/hostNetwork
- **RBAC:** Bindings de cluster-admin, uso del SA predeterminado
- **Red:** Namespaces sin NetworkPolicy
- **Secrets:** Recomendaciones de rotación de claves del registry de Docker
- **Recursos:** Contenedores sin resource limits

**Ejemplo de respuesta:**
```json
{
  "summary": {"critical": 0, "high": 2, "medium": 5, "low": 8, "info": 3, "total": 18},
  "findings": [
    {
      "severity": "high",
      "category": "Pod Security",
      "resource": "default/pod/nginx/container/app",
      "namespace": "default",
      "detail": "Container \"app\" allows privilege escalation",
      "fix": "Set allowPrivilegeEscalation: false in securityContext"
    }
  ],
  "scannedAt": "2025-01-15T10:30:00Z"
}
```

## Operaciones de Escritura

| Método | Ruta | Auth | Descripción |
|--------|------|------|-------------|
| POST | `/api/scale` | Requerida | Escalar/desescalar deployment/statefulset |
| POST | `/api/pod/delete` | Requerida | Eliminar un solo Pod |
| POST | `/api/rollout/restart` | Requerida | Reinicio continuo de deployment/daemonset/statefulset |
| POST | `/api/node/cordon` | Requerida | Aislar/restaurar nodo |
| POST | `/api/yaml/apply` | Requerida | Aplicar YAML (kubectl apply) |

Todas las operaciones de escritura se registran en el log de auditoría.

---

## Respuestas de Error

Todos los errores devuelven JSON:

```json
{"error": "descriptive error message"}
```

| Código | Significado |
|------|---------|
| 400 | Solicitud incorrecta (parámetros faltantes/inválidos) |
| 401 | No autorizado (token faltante/expirado/inválido) |
| 403 | Prohibido (rol insuficiente) |
| 404 | Recurso no encontrado |
| 500 | Error interno del servidor |
| 503 | Servicio no disponible (proveedor de IA no configurado) |

## Roles

| Rol | Permisos |
|------|-------------|
| `admin` | Acceso completo incluyendo gestión de usuarios/RBAC/proveedores |
| `operator` | Dashboard + diagnósticos + chat (sin gestión de usuarios) |
| `viewer` | Dashboard de solo lectura + chat |
| `ns-admin` | Admin solo dentro de los namespaces asignados |
| `ns-viewer` | Viewer solo dentro de los namespaces asignados |

## Endpoints Nuevos (v14.48-v14.53)

Los siguientes endpoints se añadieron entre v14.48 y v14.53 y se han incorporado a la especificación OpenAPI 3.0.

### Inventario de Imágenes de Contenedor

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/images` | Inventario de todas las imágenes de contenedor en el clúster, con auditoría de resource limits y detección de etiquetas `:latest` |

**Campos del resumen de respuesta:** `totalImages`, `withoutLimits`, `withoutRequests`, `usingLatestTag`, `uniqueRegistries`

### Resumen de Eventos de Advertencia

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/events/summary` | Agregación de todos los eventos Warning por Reason, con clasificación de gravedad y estadísticas de namespaces afectados |

### Análisis de Eficiencia del Clúster

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/efficiency` | Análisis de eficiencia de recursos del clúster: Pods sin límites, contenedores sobreaprovisionados, nodos infrautilizados, puntuación de eficiencia 0-100 |

### Seguridad: Escaneo de Exposición de Secrets

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/security/secrets` | Detección de credenciales embebidas en código, seguimiento de rotación de Secrets (90 días), Secrets no utilizados, nombres de claves sensibles |

### Búsqueda y Exportación de Auditoría

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/audit/events` | Búsqueda de eventos de auditoría: soporta `actor`, `action`, `q` (búsqueda de texto completo), `severity`, filtrado por rango de fechas |
| GET | `/api/audit/export` | Exportar eventos de auditoría en formato CSV (importable a sistemas SIEM) |

### Gestión de Copias de Seguridad

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/system/backup` | Listar todos los archivos de respaldo (tamaño, antigüedad, tipo) |
| POST | `/api/system/backup` | Crear respaldo de base de datos (nombre con marca de tiempo) |
| DELETE | `/api/system/backup?name=X` | Eliminar un respaldo específico (protección contra path traversal) |
| POST | `/api/system/backup/restore?name=X` | Restaurar base de datos desde un respaldo |

### Webhook de Alertmanager

| Método | Ruta | Descripción |
|--------|------|-------------|
| POST | `/api/webhooks/alertmanager` | Recibir alertas v4 de Prometheus Alertmanager, generar automáticamente sugerencias de investigación |
| POST | `/api/webhooks/alertmanager/test` | Enviar una alerta de prueba para verificar el receptor |

**Ejemplo de configuración de Alertmanager:**
```yaml
receivers:
  - name: k8ops
    webhook_configs:
      - url: http://k8ops.k8ops-system.svc:9090/api/webhooks/alertmanager
        send_resolved: true
```

### Registro de Cambios

| Versión | Endpoint | Dimensión |
|------|------|------|
| v14.49 | `GET /api/events/summary` | Producto |
| v14.50 | Sonda de inicio + preStop | Despliegue |
| v14.51 | `POST /api/webhooks/alertmanager` | Operaciones |
| v14.52 | `GET /api/efficiency` | Escalabilidad |
| v14.53 | `GET /api/security/secrets` | Seguridad |
| v14.54 | OpenAPI 3.0 spec + API.md | Documentación |
| v14.55 | `GET /api/pdbs` `GET /api/compatibility` | Producto |
| v14.56 | `GET /api/certificates/expiry` | Operaciones |
| v14.57 | Drenaje de cierre elegante | Despliegue |
| v14.58 | `GET /api/addons/health` | Producto |
| v14.59 | `GET /api/capacity/forecast` | Escalabilidad |
| v14.60 | OpenAPI spec completada + actualización API.md | Documentación |
| v14.61 | `GET /api/security/network-policies` | Seguridad |
| v14.62 | `GET /api/diagnostics/restarts` | Operaciones |
| v14.63 | `GET /api/deployments/rollout` | Despliegue |
| v14.64 | `GET /api/resources/waste` | Producto |
| v14.65 | `GET /api/scaling/bottlenecks` | Escalabilidad |

### Estado de Pod Disruption Budget (v14.55+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/pdbs` | Lista todos los PDB, con estado de interrupción, workloads coincidentes, evaluación de salud (healthy/at-risk/blocked), para verificación segura antes de drain |

### Detección de Compatibilidad de Distribución K8s (v14.55+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/compatibility` | Detección automática de distribución del clúster (vanilla/k3s/RKE2/EKS/GKE/AKS/OpenShift/Talos), compatibilidad de versión, características de nodos ARM/Windows/GPU |

### Escaneo de Expiración de Certificados TLS (v14.56+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/certificates/expiry` | Escanea certificados X.509 en todos los Secrets TLS/Opaque, clasificados por fecha de expiración (expired/critical/warning/ok), correlacionados con recursos Ingress |

### Estado de Drain del Servidor (v14.57+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/system/drain-status` | Reporta el estado de cierre elegante del servidor: draining, shutdownInitiated, activeConnections, uptime |

### Detección de Salud de Complementos (v14.58+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/addons/health` | Detección no intrusiva de 39 complementos comunes de K8s (12 categorías: CNI/DNS/Ingress/CertManager/LB/Mesh/Backup/Monitoring/Policy/Storage/GitOps/VM) |

### Predicción de Agotamiento de Capacidad (v14.59+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/capacity/forecast` | Predice cuándo se agotará la capacidad de CPU/memoria/Pod/almacenamiento, basado en estimaciones de tasa de crecimiento proporciona days-to-exhaustion y recomendaciones de expansión |

### Auditoría de Network Policy (v14.61+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/security/network-policies` | Audita la cobertura de NetworkPolicy: detecta namespaces sin NetworkPolicy, políticas permisivas (0.0.0.0/0 entrada/salida), cobertura parcial, clasificados por gravedad (critical/warning/info) |

**Parámetros de consulta:** `namespace` (opcional, filtrar namespace)

**Ejemplo de respuesta:**
```json
{
  "summary": {
    "totalNamespaces": 27,
    "withoutNetPol": 25,
    "findings": 18,
    "critical": 10,
    "warning": 8
  },
  "namespaces": [...]
}
```

### Diagnóstico de Reinicios de Pod (v14.62+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/diagnostics/restarts` | Diagnostica patrones de reinicio de Pods y causa raíz: clasifica comportamientos de reinicio (crash-loop/occasional/post-deploy), extrae motivos de terminación (OOMKilled/Error/código de salida), estados de espera (CrashLoopBackOff/ImagePullBackOff) |

**Parámetros de consulta:** `namespace` (opcional)

**Modos de diagnóstico:**
- **crash-loop**: Muchos reinicios en poco tiempo
- **occasional**: Pocos reinicios durante mucho tiempo
- **post-deploy**: Reinicios inmediatamente después del despliegue

### Seguimiento de Estado de Rollout de Despliegue (v14.63+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/deployments/rollout` | Escanea el estado de rollout de todos los Deployment/StatefulSet/DaemonSet: 7 estados (complete/in-progress/stalled/degraded/paused/failed/scaled-to-zero), detecta ProgressDeadlineExceeded, ReplicaFailure, mismatch de generación |

**Parámetros de consulta:**
- `namespace` (opcional) — filtrar namespace
- `status` (opcional) — filtrar estado de rollout: `failed`, `degraded`, `stalled`, `in-progress`, `paused`, `scaled-to-zero`, `complete`

**Descripción de estados:**
| Estado | Significado |
|------|------|
| `complete` | Todas las réplicas actualizadas y listas |
| `in-progress` | Actualización continua en progreso |
| `stalled` | El controlador no ha observado el spec más reciente (generación no coincide) |
| `degraded` | Algunas réplicas no disponibles |
| `paused` | El Deployment está explícitamente pausado |
| `failed` | ProgressDeadlineExceeded, despliegue fallido por timeout |
| `scaled-to-zero` | Réplicas en 0 |

### Detección de Desperdicio de Recursos (v14.64+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/resources/waste` | Escanea recursos desperdiciados y huérfanos en el clúster para reducir costos: 6 categorías de desperdicio (dead-service/unused-pvc/orphaned-configmap/orphaned-secret/empty-namespace/unattached-pv), 4 niveles de gravedad (critical/high/medium/low), evaluación de riesgo de costo |

**Parámetros de consulta:** `namespace` (opcional)

**Tipos de desperdicio:**
| Categoría | Detección | Gravedad predeterminada |
|------|---------|-----------|
| `dead-service` | Service sin endpoints backend (LoadBalancer es critical) | medium/critical |
| `unused-pvc` | PVC no montado por ningún Pod | high |
| `orphaned-configmap` | ConfigMap no referenciado por ningún Pod | low/medium |
| `orphaned-secret` | Secret no referenciado por ningún Pod (riesgo de seguridad) | high |
| `empty-namespace` | Namespace sin Pods en ejecución | medium |
| `unattached-pv` | PV en estado Available (no vinculado a ningún PVC) | critical |

**Filtrado inteligente:** Omite automáticamente el namespace kube-system, Secret de token de ServiceAccount, Secret de release de Helm, ConfigMaps generados automáticamente

### Detección de Cuellos de Botella de Escalabilidad (v14.65+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/scaling/bottlenecks` | Escanea factores que limitan la escalabilidad horizontal: 7 tipos de cuellos de botella (node-schedulable/node-pressure/resource-quota/hpa-stuck/pdb-blocking/storage-exhaust/image-pull-limit), 4 niveles de impacto (critical/high/moderate/low), resumen de capacidad del clúster |

**Parámetros de consulta:** `namespace` (opcional)

**Tipos de cuellos de botella:**
| Categoría | Detección |
|------|---------|
| `node-schedulable` | Nodos aislados, capacidad de Pod del clúster excedida (>75% advertencia / >90% crítico) |
| `node-pressure` | Estado de presión de memoria, disco, PID |
| `resource-quota` | Cuota de namespace supera 75%/90% |
| `hpa-stuck` | HPA alcanzó el máximo de réplicas o faltan métricas |
| `pdb-blocking` | PDB permite 0 interrupciones voluntarias |
| `storage-exhaust` | Solicitudes de PVC de namespace superan 500Gi |

**Resumen de capacidad del clúster:** Proporciona número de nodos, capacidad y cantidad asignable de CPU/memoria, capacidad de Pods y cantidad asignada, margen de escalado

### Análisis de Riesgo de Permisos RBAC (v14.67+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/security/rbac-risk` | Analiza el riesgo de permisos de todos los RoleBinding/ClusterRoleBinding, sistema de puntuación 0-100, 5 niveles de riesgo (critical/high/elevated/moderate/low), detecta bindings de cluster-admin, escalada de privilegios, permisos wildcard, acceso a recursos sensibles |

**Parámetros de consulta:** `namespace` (opcional)

**Reglas de puntuación de riesgo:**
| Elemento detectado | Puntaje base | Puntaje adicional |
|--------|--------|--------|
| ClusterRoleBinding + cluster-admin | 100 | - |
| Escalada de privilegios (escalate/bind/impersonate) | - | +25 |
| Verbo wildcard (verbs: *) | - | +25 |
| Recurso wildcard (resources: *) | - | +20 |
| Escritura a nivel de clúster | - | +30 |
| Acceso a recursos sensibles (secrets/pods/exec) | - | +15 |

### Monitorización de Salud de Ejecución de CronJob (v14.68+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/operations/cronjobs/health` | Monitoriza la salud de ejecución de todos los CronJobs: tasa de éxito, fallos consecutivos, programación suspendida/estancada, nunca ejecutados, 5 estados de salud (healthy/warning/failing/suspended/no-runs) |

**Parámetros de consulta:** `namespace` (opcional)

**Estados de salud:**
| Estado | Condición de activación |
|------|---------|
| `failing` | Más de 3 fallos consecutivos |
| `warning` | 1-2 fallos consecutivos, o tasa de éxito < 50% |
| `suspended` | CronJob suspendido |
| `no-runs` | Nunca se ha ejecutado |
| `healthy` | Todos exitosos recientemente |

### Monitorización de Salud de Service y Endpoint (v14.69+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/networking/health` | Escanea la salud de red de todos los Services e Ingress: servicios sin endpoints, selectores sin coincidencia, endpoints degradados, LoadBalancer en espera, Ingress con servicio backend faltante/sin endpoints, 5 estados de salud |

**Parámetros de consulta:** `namespace` (opcional)

**Estados de salud de Service:**
| Estado | Significado |
|------|------|
| `misconfigured` | Selector sin coincidencia — ningún Pod coincide con el label |
| `no-endpoints` | Todos los endpoints no disponibles |
| `degraded` | Algunos endpoints no disponibles |
| `external` | ExternalName/LoadBalancer (informativo) |
| `healthy` | Todos los endpoints normales |

**Comprobación de salud de Ingress:** Detecta si el Service backend existe, si tiene endpoints disponibles, verifica el backend predeterminado y las rutas de reglas

### Monitorización de Salud de Almacenamiento PV/PVC (v14.70+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/storage/health` | Escanea la salud de almacenamiento de todos los PVC/PV: diagnóstico de PVC Pending, PVC huérfanos (vinculados pero sin uso por Pod > 1 día), PVC Lost/Failed, PV Released/Failed que requieren limpieza manual, PV Available obsoletos que desperdician capacidad, 6 estados de salud + análisis de distribución de StorageClass |

**Parámetros de consulta:** `namespace` (opcional)

**Estados de salud de PVC:**
| Estado | Significado |
|------|------|
| `failed` | Fallo en el aprovisionamiento del PVC |
| `lost` | El PV subyacente ha sido eliminado |
| `pending` | Esperando aprovisionamiento (sin StorageClass, WaitForFirstConsumer) |
| `near-capacity` | Cerca del límite de capacidad |
| `orphaned` | Vinculado pero sin uso por Pod durante más de 1 día |
| `bound` | Vinculado normalmente |

**Detección de problemas de PV:** PV Released (requiere limpieza manual), PV Failed (recuperación fallida), PV Available obsoleto (> 7 días desperdiciando capacidad)

**Análisis de StorageClass:** Marca de clase predeterminada, provisioner, reclaim policy, binding mode, soporte de volume expansion, distribución de conteo de PVC

### Auditoría de Seguridad de ServiceAccount (v14.72+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/security/service-accounts` | Auditoría integral de la postura de seguridad de todos los ServiceAccounts: SA no utilizados, SA predeterminado usado por Pods, montaje automático de token innecesario, bindings de cluster-admin, permisos a nivel de clúster, SA obsoletos, Secrets de token de larga duración heredados |

**Parámetros de consulta:** `namespace` (opcional)

**Puntuación de riesgo:** 0-100 (mayor = más peligroso), 5 niveles de gravedad: critical / high / elevated / moderate / low

**Elementos detectados:**
| Elemento detectado | Gravedad | Descripción |
|--------|--------|------|
| SA no utilizado (> 7 días sin referencia de Pod) | moderate | Superficie de ataque ampliada |
| SA predeterminado usado por Pods | elevated | Violación del principio de mínimo privilegio |
| Binding de cluster-admin | critical | Superpermisos a nivel de clúster |
| Montaje automático de token innecesario | moderate | Los SA que no necesitan token no deben montarlo |
| SA obsoleto (> 30 días sin uso pero con permisos) | high | Permisos zombi |
| Secret de token de larga duración heredado (K8s <1.24) | high | Token de larga duración no recomendado |

### Seguimiento de Presupuesto de Errores SLO/SLA (v14.73+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/operations/slo` | Seguimiento de disponibilidad y presupuesto de errores SLO/SLA basado en algoritmo multi-ventana multi-tasa-de-consumo |

**Parámetros de consulta:** `namespace` (opcional)

**Configuración de ventanas:** 5m / 1h / 6h / 24h / 7d

**Contenido de retorno:**
| Campo | Descripción |
|------|------|
| `availability` | Porcentaje de disponibilidad por ventana |
| `errorBudget` | Cantidad restante y tasa de consumo del presupuesto de errores |
| `burnRate` | Tasa de consumo multi-ventana (fast: 5m/1h, slow: 6h/24h) |
| `latencySLO` | Percentiles de latencia P50/P95/P99 y objetivos |
| `status` | meeting (cumple) / at-risk (en riesgo) / violated (violado) |

### Monitorización de ResourceQuota y LimitRange (v14.74+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/resources/quota` | Escanea la utilización de ResourceQuota y las restricciones predeterminadas de LimitRange en todos los namespaces |

**Parámetros de consulta:** `namespace` (opcional)

**Niveles de estado de cuota:**
| Estado | Utilización | Descripción |
|------|--------|------|
| `ok` | <70% | Normal |
| `warning` | 70-85% | Cerca del límite |
| `critical` | 85-100% | Peligro |
| `exceeded` | >100% | Excedido |
| `no-limit` | — | Sin cuota configurada |

**Elementos detectados:** Utilización de cuota de CPU/memoria/Pod/ConfigMap/Secret/almacenamiento por namespace, namespaces sin protección de cuota, análisis de restricciones predeterminadas/mínimas/máximas de LimitRange, ranking de Top consumidores

### Auditoría de Configuración de Despliegue (v14.75+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/deployments/audit` | Audita las violaciones de mejores prácticas de configuración de todos los Deployment/StatefulSet/DaemonSet, 8 categorías de verificación, cada una con gravedad y recomendación de corrección |

**Parámetros de consulta:** `namespace` (opcional), `severity` (opcional: critical / warning / info)

**Categorías de verificación:**
| Categoría | Elementos de verificación |
|------|--------|
| `revision-history` | Historial de revisiones demasiado pequeño (< 2, no se puede retroceder) o demasiado grande (> 20, desperdicio de recursos) |
| `image-policy` | Etiqueta `:latest` pero pullPolicy no es Always; etiqueta fija pero pullPolicy es Always |
| `resources` | Faltan resource limits/requests |
| `probes` | Faltan sondas liveness/readiness/startup |
| `security-context` | Contenedores privilegiados, ejecución como root, root filesystem escribible, permite escalada de privilegios |
| `update-strategy` | Estrategia Recreate (tiempo de inactividad), OnDelete (requiere eliminación manual de Pod), rollout particionado |
| `lifecycle` | terminationGracePeriod demasiado corto (< 10s) o demasiado largo (> 300s), falta hook preStop |
| `config-drift` | Falta seccomp profile |

**Puntuación de salud:** 0 (perfecto) a 100 (peor), critical=20 puntos/warning=8 puntos/info=2 puntos

### Análisis de Salud de Programación y Fragmentación de Recursos (v14.76+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/scheduling/health` | Analiza la salud de programación del clúster, programabilidad de nodos, fragmentación de recursos y diagnóstico de Pods Pending |

**Parámetros de consulta:** `namespace` (opcional)

**Contenido de retorno:**
| Campo | Descripción |
|------|------|
| `summary` | Estadísticas de nodos (programables/no programables/aislados/bajo presión), conteo de Pods Pending, conteo de FailedScheduling, desalojos en 24h, puntuación de salud 0-100 |
| `nodes` | Estado de programabilidad por nodo, tipo de presión, taints, cantidad y porcentaje disponible de CPU/memoria/Pod |
| `pendingPods` | Lista de Pods Pending, con solicitudes de CPU/memoria, nodeSelector, causa de fallo de programación analizada |
| `largestFittablePod` | El Pod más grande que se puede programar actualmente (CPU/memoria/conteo de Pods), mejor nodo |
| `effective_capacity` | Capacidad teórica vs capacidad efectiva (porcentaje de pérdida de capacidad debido a nodos no programables) |
| `fragmentation` | Métricas de fragmentación de recursos (tasa promedio de fragmentación de CPU/memoria, peor nodo fragmentado, detección de Pods extragrandes) |
| `evictions` | Registros de desalojo en 24h (Pod, nodo, motivo) |
| `recommendations` | Recomendaciones de corrección accionables |

**Análisis de causas de fallo de programación:** insufficient-cpu / insufficient-memory / untolerated-taint / node-selector-mismatch / node-affinity-mismatch / pod-affinity-conflict / pod-limit-reached / volume-binding-failure / no-nodes-available

### Escaneo de Postura de Seguridad de Pods (v14.79+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/security/pods` | Audita la postura de seguridad de todos los Pods en ejecución: contenedores privilegiados, hostNetwork/hostPID/hostIPC, montajes HostPath, capabilities peligrosas de Linux, ejecución como root, permite escalada de privilegios, root filesystem escribible, falta securityContext, imágenes :latest/sin etiqueta, no bloqueadas por digest, inyección de Secret como variable de entorno, sin resource limits, vinculación de puerto host |

**Parámetros de consulta:** `namespace` (opcional), `severity` (opcional: critical / warning / info)

**Puntuación de riesgo:** 0 (seguro) a 100 (riesgo extremo), critical=25 puntos/warning=8 puntos/info=2 puntos

**Categorías de verificación:**
| Categoría | Gravedad | Descripción |
|------|--------|------|
| `privileged` | critical | Contenedor privilegiado — acceso total al host |
| `host-network` | critical | Comparte el namespace de red del nodo |
| `host-pid` | critical | Visibles todos los procesos del nodo |
| `host-ipc` | critical | Comparte el namespace IPC |
| `host-path` | critical | Monta volumen HostPath desde el nodo |
| `dangerous-capabilities` | critical | SYS_ADMIN/NET_ADMIN/NET_RAW/SYS_PTRACE/SYS_MODULE/DAC_OVERRIDE/SETUID/SETGID |
| `runs-as-root` | warning | Se ejecuta con UID 0 |
| `privilege-escalation` | warning | Permite escalada de privilegios |
| `missing-security-context` | warning | Falta securityContext |
| `image-latest` | warning | Usa etiqueta :latest |
| `image-no-tag` | warning | Sin etiqueta (predeterminado :latest) |
| `host-port` | warning | Vincula puerto del host |
| `image-no-digest` | info | No bloqueada por digest |
| `writable-rootfs` | info | Root filesystem escribible |
| `secret-env-vars` | info | Secret inyectado como variable de entorno |
| `no-resource-limits` | info | Sin resource limits |

### Detección de Tormentas de Eventos y Fallos en Cascada (v14.80+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/operations/event-storm` | Analiza eventos Warning del clúster, detecta tormentas de eventos, fallos en cascada y fluctuación de recursos. Estadísticas de eventos de alerta en ventanas de 15min/1h/24h, clasifica gravedad de tormenta, identifica recursos fluctuantes (mismo recurso misma causa repetida 3+ veces), agrega por namespace y causa, proporciona recomendaciones accionables |

**Parámetros de consulta:** `namespace` (opcional)

**Gravedad de tormenta:**
| Gravedad | Condición | Descripción |
|--------|------|------|
| `critical` | >50 eventos/15min | Investigación urgente |
| `high` | >20 eventos/15min | Requiere atención |
| `medium` | >10 eventos/15min | Monitorizar tendencia |
| `low` | >5 eventos/15min | Informativo |

**Contenido de retorno:** Resultados de detección de tormenta, ranking de alertas por namespace, Top causas de eventos, lista de recursos fluctuantes (con frecuencia de fluctuación), línea de tiempo de eventos de los últimos 15 minutos, número de recursos afectados (radio de explosión), recomendaciones accionables

### Grafo de Dependencias de Recursos y Análisis de Impacto (v14.81+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/dependencies` | Rastrea el grafo completo de dependencias de cualquier workload (Deployment/StatefulSet/DaemonSet/Pod), evalúa el alcance de impacto de cambios |

**Parámetros de consulta:**

| Parámetro | Obligatorio | Descripción |
|------|------|------|
| `kind` | Sí | Tipo de recurso: Deployment / StatefulSet / DaemonSet / Pod |
| `name` | Sí | Nombre del recurso |
| `namespace` | No | Namespace (predeterminado: default) |

**Dependencias directas (de qué depende este workload):** ConfigMap, Secret, PVC, ServiceAccount

**Dependencias inversas (qué depende de este workload):**
- Service (coincide con Pods vía label selector)
- Ingress (enruta al Service coincidente)
- NetworkPolicy (se aplica a ese Pod)
- HPA (tiene este workload como objetivo)
- Otros Pods que comparten ConfigMap/Secret

**Evaluación de alcance de impacto:** blastRadius = número de dependencias directas + número de dependencias inversas, nivel de riesgo low(<6) / medium(6-10) / high(11-20) / critical(>20)

### Verificación de Cumplimiento de Distribución Topológica (v14.82+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/topology/spread` | Analiza la distribución de Pods en dominios topológicos (zone/region/node), verifica el cumplimiento de topologySpreadConstraints |

**Parámetros de consulta:** `namespace` (opcional), `domain` (opcional, clave del dominio topológico, predeterminado `kubernetes.io/hostname`, se puede establecer en `topology.kubernetes.io/zone`)

**Estado de workloads:**
| Estado | Significado |
|------|------|
| `balanced` | Distribución uniforme (actualSkew ≤ maxSkew) |
| `skewed` | Distribución desigual (actualSkew > maxSkew) |
| `no-constraint` | Multi-réplica pero sin restricciones topológicas |
| `single-replica` | Réplica única (la distribución topológica no aplica) |

**Contenido de retorno:** Estadísticas de dominios topológicos, distribución por dominio por workload (conteo de Pods/valor esperado), desviación real vs desviación máxima, etiquetas de dominio y conteo de Pods por nodo, recomendaciones (añadir restricciones, etiquetar nodos, sugerencias para clústeres de dominio único)

### Auditoría de Rotación y Ciclo de Vida de Secrets (v14.85+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/security/secrets/rotation` | Audita el cumplimiento de rotación y la gestión del ciclo de vida de todos los Secrets: seguimiento de antigüedad (stale >90d / very stale >180d), detección de Secrets no utilizados (no referenciados por ningún Pod), detección de expiración de certificados TLS (analiza certificados), seguimiento de Secrets del registry de Docker, detección de ServiceAccount Token heredados, detección de nombres sensibles |

**Parámetros de consulta:** `namespace` (opcional)

**Puntuación de riesgo:** Nivel de riesgo por Secret (critical / high / medium / low), puntuación de rotación del clúster 0-100

**Categorías de verificación:**
| Elemento verificado | Gravedad | Descripción |
|---------|--------|------|
| Certificado TLS expirado | critical | Actualizar inmediatamente |
| Docker Secret expirado >180d | critical | Puede contener credenciales de registry expiradas |
| Certificado TLS expira en <30d | high | Programar renovación lo antes posible |
| Stale + no utilizado + nombre sensible | high | Riesgo de seguridad |
| Docker Secret stale | medium | Se recomienda rotar |
| Stale pero en uso | low | Planificar rotación |

### Auditoría de Eficacia de Sondas de Salud (v14.86+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/operations/probes` | Audita la configuración de sondas liveness/readiness/startup de todos los workloads, detecta reinicios en cascada por configuración inadecuada, tráfico a Pods no listos, fallos de inicio |

**Parámetros de consulta:** `namespace` (opcional)

**Categorías de verificación:**
| Elemento verificado | Gravedad | Descripción |
|---------|--------|------|
| Falta liveness | warning | Contenedores colgados no se reiniciarán |
| Falta readiness | warning | El tráfico puede llegar a Pods no listos |
| Sonda demasiado agresiva (period <5s) | warning | Carga excesiva en el API server |
| Timeout demasiado corto (<2s) | warning | Puede fallar bajo picos de latencia |
| Umbral de fallo demasiado bajo (<3) | warning | Demasiado sensible a errores transitorios |
| Intervalo de readiness demasiado largo (>60s) | info | Detección de readiness lenta |
| Umbral de fallo de liveness demasiado alto (>10) | info | Recuperación por reinicio lenta |
| Mismo liveness+readiness | info | Deberían configurarse de manera diferenciada |

**Contenido de retorno:** Puntuación de riesgo por workload, puntuación de eficacia del clúster (0-100), agregación de Top problemas, recomendaciones accionables

### Seguimiento de Obsolescencia de Workloads (v14.87+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/product/staleness` | Rastrea la obsolescencia de despliegue de todos los workloads, detecta workloads no actualizados durante mucho tiempo, imágenes con etiqueta :latest, imágenes no bloqueadas por digest |

**Parámetros de consulta:** `namespace` (opcional)

**Clasificación de obsolescencia:**
| Estado | Condición | Descripción |
|------|------|------|
| `fresh` | <7d | Actualizado recientemente |
| `recent` | <30d | Relativamente nuevo |
| `stale` | <90d | Requiere atención |
| `very-stale` | <180d | Se recomienda actualizar |
| `ancient` | >180d | Riesgo de seguridad |

**Contenido de retorno:** Nivel de riesgo por workload, análisis de etiquetas de imagen (:latest / digest / sin etiqueta), buckets de distribución por antigüedad, estadísticas por namespace, puntuación de frescura del clúster (0-100), recomendaciones accionables

### Análisis de Sobrealocación y Presión de Recursos (v14.88+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/scalability/overcommit` | Analiza la tasa de sobrealocación de CPU y memoria de todos los nodos, detecta over-commit peligroso, Pods sin limits, puntuación de presión de recursos |

**Parámetros de consulta:** `namespace` (opcional)

**Análisis por nodo:**
| Métrica | Descripción |
|------|------|
| CPU request commit | sum(requests) / allocatable |
| CPU limit commit | sum(limits) / allocatable |
| Mem request/limit commit | Igual que arriba |
| Puntuación de presión | 0-100 (cálculo ponderado) |
| Nivel de riesgo | safe / moderate / high / critical (>3x) |

**Métricas del clúster:** Tasas totales de sobrealocación de CPU/memoria, número de nodos en riesgo, número de Pods sin limits, puntuación de seguridad (0-100), desglose de consumo de recursos por namespace, recomendaciones accionables

### Análisis de Seguridad de Imágenes y Cadena de Suministro (v14.92+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/security/images` | Escanea los riesgos de seguridad de la cadena de suministro de todas las imágenes de contenedor en ejecución: bloqueo por digest, etiqueta :latest, imágenes sin etiqueta, etiquetas de versión antiguas, repositorios públicos vs privados, repositorios desconocidos |

**Parámetros de consulta:** `namespace` (opcional)

**Categorías de verificación:**
| Elemento verificado | Puntaje de riesgo | Descripción |
|---------|--------|------|
| Sin etiqueta | +25 | Predeterminado :latest, versión incierta |
| Usa :latest | +15 | Etiqueta mutable, no reproducible |
| No bloqueada por digest | +10 | El contenido de la imagen puede ser reemplazado silenciosamente |
| Repositorio desconocido | +10 | Sin prefijo de repositorio, predeterminado Docker Hub |
| Etiqueta de versión antigua | +15 | Puede contener vulnerabilidades conocidas |
| Repositorio público + no bloqueado | +5 | Sin garantía de procedencia |

**Contenido de retorno:** Nivel de riesgo por imagen (critical/high/medium/low), estadísticas por repositorio, Top imágenes de riesgo, puntuación de seguridad de imágenes del clúster (0-100), recomendaciones accionables

### Planificación de Capacidad (v14.50+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/capacity/planning` | Análisis de planificación de capacidad de nodos: solicitudes de CPU/memoria vs cantidad asignable por nodo, capacidad restante, momento recomendado para expansión, detección de fragmentación de recursos |

### Agregación de Puntuación de Salud del Clúster (v14.93+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/operations/health-score` | Agrega todas las señales de salud del clúster en una puntuación integral (0-100, grado A-F), combinando 5 dimensiones ponderadas |

**5 dimensiones ponderadas:**
| Dimensión | Peso | Verificaciones |
|------|------|----------|
| Node Health | 25% | Estado de readiness de nodos |
| Pod Health | 25% | CrashLoop, Pending, Failed, alto número de reinicios |
| Workload Health | 20% | Réplicas listas de Deployment/StatefulSet/DaemonSet |
| Event Activity | 15% | Número de eventos Warning en la última hora |
| API Server | 15% | Medición de latencia en tiempo real del API server |

**Contenido de retorno:** Puntuación total 0-100, calificación de letra A-F, estado (healthy/warning/critical), detalles de puntuación por dimensión, resumen del clúster (conteo de nodos/Pods/workloads), Top problemas ordenados por gravedad

### Recomendaciones de Configuración Razonable de Recursos HPA/VPA (v14.94+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/scalability/autoscale-recommendations` | Analiza la cobertura HPA y la configuración razonable de recursos de todos los workloads, detecta sobreaprovisionamiento, workloads multi-réplica sin HPA, eficiencia del HPA |

**Parámetros de consulta:** `namespace` (opcional)

**Categorías de detección:**
| Elemento detectado | Descripción |
|---------|------|
| Workload multi-réplica sin HPA | Se recomienda añadir autoescalado |
| Solicitud de CPU demasiado alta (>1 núcleo/contenedor) | Alta confianza, se recomienda reducir a la mitad |
| Solicitud de memoria demasiado alta (>2GB/contenedor) | Se recomienda right-sizing |
| HPA alcanzó maxReplicas | Necesita aumentar capacidad |
| HPA inactivo (<20% utilización) | Se recomienda reducir maxReplicas |

**Contenido de retorno:** Valores actuales vs recomendados de CPU/memoria por workload, porcentaje de cambio, confianza, ahorros potenciales de núcleos de CPU y memoria, análisis de eficiencia del HPA, puntuación de autoescalado del clúster (0-100)

### Monitorización de Salud de Ingress y Enrutamiento de Tráfico (v14.96+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/product/ingress-health` | Analiza la salud de enrutamiento de tráfico y los problemas de configuración de todos los recursos Ingress |

**Parámetros de consulta:** `namespace` (opcional)

**Categorías de verificación:**
| Elemento verificado | Gravedad | Descripción |
|---------|--------|------|
| El servicio backend no existe | critical | El Service referenciado no existe |
| El backend no tiene endpoints listos | warning | El Service no tiene endpoints ready |
| Sin configuración TLS | warning | Tiene host pero sin cifrar |
| La IngressClass no existe | critical | El class especificado no está desplegado |
| Conflicto host+path | warning | Múltiples Ingress compiten por la misma ruta |
| Sin reglas de enrutamiento | warning | El Ingress no tiene efecto |

**Contenido de retorno:** Estado por Ingress (healthy/warning/critical), estadísticas por namespace, puntuación de salud del clúster (0-100), recomendaciones accionables

### Análisis de Condiciones de Nodo y Presión de Recursos (v14.99+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/operations/node-pressure` | Analiza el estado de condiciones y la saturación de recursos de todos los nodos |

**Categorías de detección:**
| Condición | Puntaje de riesgo | Descripción |
|------|--------|------|
| NetworkUnavailable | +30 | CNI/red no lista |
| DiskPressure | +25 | Disco lleno o casi lleno |
| MemoryPressure | +25 | Memoria del nodo agotada |
| PIDPressure | +20 | Demasiados procesos |
| NotReady | →critical | Problema de kubelet/runtime |
| CPU >90% | +20 | Saturación de solicitud de CPU |
| Memoria >95% | +20 | Saturación de solicitud de memoria |
| Cordoned | — | No programable |

**Contenido de retorno:** Nivel de riesgo por nodo (critical/high/medium/low), uso de CPU/memoria/Pod, detalles de condiciones (motivo, mensaje, duración), puntuación de presión del clúster (0-100), recomendaciones accionables

### Análisis de Vinculación y Rendimiento de PVC (v15.00+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/scalability/pvc-analysis` | Analiza la salud de vinculación y el rendimiento de almacenamiento de todos los PVC |

**Parámetros de consulta:** `namespace` (opcional)

**Categorías de detección:**
| Elemento detectado | Gravedad | Descripción |
|---------|--------|------|
| PVC atascado (>5min) | critical | PVC bloqueado + análisis de causa raíz |
| PVC Lost | critical | El PV subyacente puede haber sido eliminado |
| Vinculación lenta (>30s) | warning | Retraso en el aprovisionamiento de almacenamiento |
| PVC Pending | warning | Esperando vinculación |
| Falta StorageClass predeterminado | info | No se ha establecido SC predeterminado |

**Contenido de retorno:** Estado por PVC (healthy/warning/critical), tiempo de vinculación, estadísticas por StorageClass, causa raíz de PVC atascado, puntuación de salud de almacenamiento del clúster (0-100)

### Auditoría de Gobernanza y Ciclo de Vida de Namespaces (v15.02+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/product/namespaces/lifecycle` | Audita el cumplimiento de gobernanza y el ciclo de vida de todos los namespaces |

**Verificaciones de gobernanza:**
| Elemento verificado | Puntaje de riesgo | Descripción |
|---------|--------|------|
| Sin ResourceQuota | +15 | Consumo ilimitado de recursos |
| Sin NetworkPolicy | +15 | Tráfico sin restricciones |
| Sin LimitRange | +5 | Sin resource limits predeterminados |
| Namespace expirado | +10 | Sin Pods en ejecución, candidato a limpieza |
| Faltan etiquetas requeridas | +5 | Faltan app/team/env/owner |
| Solo SA predeterminado | 0 | Falta SA de mínimo privilegio |

**Contenido de retorno:** Nivel de riesgo por namespace (critical/high/medium/low), flags de cumplimiento, estado del ciclo de vida (active/stale/terminating), puntuación de gobernanza del clúster (0-100), recomendaciones accionables

### Análisis de Permisos Efectivos RBAC y Escalada de Privilegios (v15.04+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/security/rbac-effective` | Analiza los permisos efectivos de RBAC y los riesgos de escalada de privilegios de todos los sujetos |

Agrega ClusterRoleBindings + RoleBindings, calcula los permisos reales de cada sujeto (User/Group/ServiceAccount).

**Categorías de detección:**

| Elemento detectado | Puntaje de riesgo | Descripción |
|---------|--------|------|
| Equivalente a cluster-admin | →critical | verbs + resources wildcard |
| Puede crear/modificar RBAC | +25 | Ruta de auto-escalada de privilegios |
| Permisos wildcard (*) | +20 | Sobreactoración |
| Puede leer Secrets | +10 | Fuga de datos sensibles |
| Puede hacer exec en Pods | +10 | Punto de entrada de escape de contenedor |

**Contenido de retorno:** Nivel de riesgo por sujeto, detalles de rutas de escalada de privilegios, puntuación de seguridad RBAC del clúster (0-100), recomendaciones accionables

### Rastreador de OOM Kill de Contenedores (v15.05+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/operations/oom-tracker` | Rastrea eventos OOMKill de contenedores y análisis de configuración de memoria |

**Parámetros de consulta:** `namespace` (opcional)

**Categorías de detección:**

| Elemento detectado | Puntaje de riesgo | Descripción |
|---------|--------|------|
| Contenedor OOMKilled | +15/cada uno | Memoria insuficiente, terminado |
| Alto número de reinicios (>=10) | +20 | Indicador de CrashLoop |
| Alto número de reinicios (>=5) | +10 | Reinicios frecuentes |
| Sin límite de memoria | +5 | Comportamiento OOM impredecible |
| Límite de memoria bajo (<256MB) | — | Puede causar OOM innecesarios |
| Límite >> solicitud (10x+) | — | Riesgo de presión de memoria del nodo |

**Contenido de retorno:** Nivel de riesgo OOM por Pod, ranking Top OOM, estadísticas por namespace, puntuación de riesgo OOM del clúster (0-100)

### Predictor de Agotamiento de Capacidad de Almacenamiento (v15.06+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/scalability/storage-forecast` | Predice cuándo se agotará la capacidad de almacenamiento |

Basado en tendencias de uso de PV y estimaciones de tasa de crecimiento, predice el momento de agotamiento del espacio de almacenamiento.

**Dimensiones de análisis:**

| Métrica | Descripción |
|------|------|
| Capacidad vs usada | Soporta anotación actual-size de Longhorn para uso real |
| Tasa de crecimiento diaria | Estimación heurística basada en uso y antigüedad del PV |
| Días hasta el agotamiento | Espacio restante / tasa de crecimiento diaria |
| Fecha de agotamiento prevista | Fecha o ">10 años" o "sin crecimiento" |
| Nivel de riesgo | critical(>95%) / high(>85% o <14d) / medium(<30d) / low |

**Contenido de retorno:** Predicción por PV, estimación de días hasta lleno del clúster, estadísticas por StorageClass, ranking de namespaces de alto riesgo, puntuación de salud de almacenamiento (0-100)

### Comprobador de Salud de Resolución DNS (v15.08+)

| Método | Ruta | Descripción |
|--------|------|-------------|
| GET | `/api/product/dns-health` | Analiza el estado de salud de resolución DNS del clúster |

**Análisis de CoreDNS:**

| Elemento verificado | Descripción |
|---------|------|
| Salud de Pods | running/ready/restarts/versión por pod |
| Corefile | forwarders, plugins, detección de Corefile faltante |
| Número de réplicas | Se recomienda >= 2 para alta disponibilidad |

**Otras deteiones:**
- Cobertura de endpoints de Headless Service (riesgo NXDOMAIN)
- Detección de caché DNS NodeLocal
- Detección de cobertura de ndots en dnsConfig de Pods (>5 = demasiadas consultas DNS)
- Descubrimiento de servicios gestionado por External-DNS

**Contenido de retorno:** Estado de Pods de CoreDNS, cobertura de Headless Service, análisis de configuración DNS, puntuación de salud DNS del clúster (0-100), recomendaciones accionables
