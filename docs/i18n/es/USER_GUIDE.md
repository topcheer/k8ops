# Guía de Usuario de k8ops

> Desde la instalación hasta el dominio, una guía detallada que cubre todas las funciones.

---

## Tabla de Contenidos

1. [Inicio Rápido](#1-inicio-rápido)
2. [Resumen del Clúster](#2-resumen-del-clúster)
3. [AI Chat — Asistente Inteligente](#3-ai-chat--asistente-inteligente)
4. [Diagnóstico y Reparación](#4-diagnóstico-y-reparación)
5. [Recomendaciones de Optimización](#5-recomendaciones-de-optimización)
6. [Análisis de Costos (FinOps)](#6-análisis-de-costos-finops)
7. [Visualización de Topología del Clúster](#7-visualización-de-topología-del-clúster)
8. [Gestión de Nodos y Pods](#8-gestión-de-nodos-y-pods)
9. [Flujo de Eventos y Notificaciones](#9-flujo-de-eventos-y-notificaciones)
10. [Explorador de Recursos y Editor YAML](#10-explorador-de-recursos-y-editor-yaml)
11. [Control de Acceso RBAC](#11-control-de-acceso-rbac)
12. [Registro de Auditoría](#12-registro-de-auditoría)
13. [Configuración y Ajustes](#13-configuración-y-ajustes)
14. [Atajos de Teclado](#14-atajos-de-teclado)
15. [Cambio de Tema](#15-cambio-de-tema)
16. [Planificación de Capacidad](#16-planificación-de-capacidad)
17. [Visualización de HPA](#17-visualización-de-hpa)
18. [Inventario de Imágenes de Contenedor](#18-inventario-de-imágenes-de-contenedor)
19. [Ranking de Recursos por Namespace](#19-ranking-de-recursos-por-namespace)
20. [Cumplimiento de Seguridad](#20-cumplimiento-de-seguridad)
21. [Gestión del Sistema](#21-gestión-del-sistema)
22. [API de Diagnóstico de Operaciones](#22-api-de-diagnóstico-de-operaciones-v1461)

---

## 1. Inicio Rápido

### Primer Inicio de Sesión

1. Abra el navegador y acceda a la dirección de k8ops (por ejemplo, `https://k8ops.iot2.win` o `http://localhost:9090`)
2. Cuenta predeterminada: `admin` / `admin`
3. En el primer inicio de sesión se le pedirá que cambie la contraseña

### Diseño de la Página

```
┌─────────┬───────────────────────────────┐
│         │  [Namespace ▼]  [🔔]  [☀/☽]  │  ← Barra superior
│ Sidebar ├───────────────────────────────┤
│         │                                │
│ Overview│       Content Area             │  ← Área de contenido
│ Diagnose│                                │
│ Nodes   │                                │
│ Pods    │                                │
│ ...     │                                │
└─────────┴───────────────────────────────┘
```

### Paleta de Comandos Ctrl+K

Presione `Ctrl+K` (Mac: `Cmd+K`) en cualquier momento para abrir la paleta de comandos global:

- Escriba `nodes` → ir a la página de nodos
- Escriba `chat` → abrir AI Chat
- Escriba `cost` → ver análisis de costos
- Use las teclas de flecha para seleccionar, Enter para confirmar, Esc para cerrar

---

## 2. Resumen del Clúster

La página Overview muestra el estado general del clúster.

### Tarjetas de Estadísticas

| Tarjeta | Significado |
|---------|-------------|
| Nodes | Número total de nodos del clúster / número Ready |
| Pods | Pods en ejecución / total |
| CPU | Utilización de CPU de todo el clúster |
| Memory | Utilización de memoria de todo el clúster |
| Warnings | Número de eventos Warning actuales |

### Gráficos de Tendencia Sparkline

Debajo de cada tarjeta hay un mini gráfico de líneas SVG que muestra los cambios de tendencia de los últimos 30 minutos.

### Cambio de Namespace

El selector desplegable en la parte izquierda de la barra superior permite cambiar el ámbito del namespace. Al cambiar, afecta a las páginas de Pods, Events, Nodes, etc. La selección se guarda en localStorage.

---

## 3. AI Chat — Asistente Inteligente

Haga clic en el botón Chat en la parte inferior de la barra lateral o presione `Ctrl+K` y escriba `chat` para abrirlo.

### Uso Básico

Escriba su pregunta en el cuadro de entrada, la IA:

1. Comprenderá la intención del lenguaje natural
2. Invocará automáticamente la herramienta K8s adecuada
3. Devolverá los resultados del análisis en streaming

### Consultas de Ejemplo

```
# Ver recursos
ver pods del namespace default
¿qué nodos tienen alta utilización de CPU?

# Diagnóstico de fallos
¿por qué los pods de nginx-deployment están en CrashLoopBackOff?
¿qué anomalías hay en el clúster?

# Recomendaciones de optimización
ayúdame a analizar el uso de recursos
¿qué pods pueden reducir su número de réplicas?
```

### Transparencia en Llamadas a Herramientas

Cuando la IA ejecuta llamadas a herramientas, se muestra un panel Thinking plegable:

- Haga clic para expandir y ver los parámetros y resultados de cada llamada a herramienta
- Visualización en formato JSON, con soporte para búsqueda

### Tarjetas de Recomendaciones de Diagnóstico

Cuando la IA sugiere ejecutar comandos kubectl, debajo del bloque de código se mostrará:

- **▶ Run in Chat** — carga el comando en el cuadro de entrada para enviarlo fácilmente
- **📋 Copy** — copia el comando al portapapeles

### Gestión de Sesiones

- **New** — crear una nueva sesión
- **Lista de sesiones a la izquierda** — haga clic para cambiar entre sesiones históricas
- Las sesiones se resumen y comprimen automáticamente (se activa automáticamente al superar 20k tokens)

### Renderizado de Markdown

Chat admite:
- Bloques de código (con resaltado de sintaxis y botón de copia)
- Tablas
- Listas, negrita, cursiva
- Enlaces (solo protocolos http/https/mailto)

---

## 4. Diagnóstico y Reparación

### Iniciar Diagnóstico

**Método 1: Interfaz Web**

1. Vaya a la página Diagnostics
2. Haga clic en "New Diagnostic"
3. Complete la descripción del problema (por ejemplo, "la respuesta de la API del namespace production se ha vuelto lenta")
4. Tras enviar, la IA analiza automáticamente

**Método 2: AI Chat**

Describa el problema directamente en Chat, la IA ejecutará automáticamente el flujo de diagnóstico.

**Método 3: CRD**

```bash
kubectl apply -f - <<EOF
apiVersion: aiops.ggai.dev/v1alpha1
kind: DiagnosticReport
metadata:
  name: check-nginx
  namespace: k8ops-system
spec:
  problem: "nginx pods keep restarting"
EOF
```

**Método 4: CLI**

```bash
k8ops diagnose --problem "pods in production keep CrashLoopBackOff"
```

### Resultados del Diagnóstico

Cada informe de diagnóstico incluye:

- **Root Cause** — la causa raíz analizada por la IA
- **Evidence** — logs, eventos y datos de métricas que respaldan el análisis
- **Recommendations** — acciones de reparación recomendadas
- **Severity** — nivel de gravedad (Info / Warning / Critical)

### Reparación Automática (Remediation)

Los planes de reparación generados por la IA requieren aprobación manual:

1. Vaya a la página Remediations
2. Revise los planes de reparación pendientes de aprobación
3. Haga clic en **Approve** para ejecutar o **Reject** para rechazar
4. Todas las operaciones se registran en el registro de auditoría

---

## 5. Recomendaciones de Optimización

La página Optimizations muestra las recomendaciones de la IA para la optimización de recursos del clúster.

### Tipos de Recomendaciones

| Tipo | Descripción |
|------|-------------|
| Resource Rightsizing | Sugerencias de ajuste para requests y limits de CPU/Memory |
| HPA Gap | Deployments sin configuración de autoescalado horizontal |
| PDB Gap | Cargas de trabajo sin PodDisruptionBudget |
| Cost Saving | Costos que se pueden ahorrar (recursos inactivos, réplicas excesivas, etc.) |

### Operaciones

- Haga clic en una recomendación para ver los detalles
- Puede aplicar directamente o ignorar

---

## 6. Análisis de Costos (FinOps)

La página Cost proporciona visibilidad de los costos del clúster.

### Funciones

- **Resumen de costos por namespace** — muestra el consumo de recursos y el costo estimado por namespace
- **Tasa de utilización de recursos** — uso real vs asignación de CPU/Memory
- **Recomendaciones de Rightsizing** — sugerencias de ajuste para recursos sobreasignados
- **Recursos inactivos** — PV, LoadBalancer, IP elástica, etc. sin uso prolongado

---

## 7. Visualización de Topología del Clúster

La página Topology muestra la relación entre nodos y Pods mediante gráficos SVG.

### Elementos Visuales

| Elemento | Significado |
|----------|-------------|
| Marco verde | Nodo Ready |
| Marco rojo | Nodo NotReady |
| Barra de progreso dentro del nodo | Utilización de CPU (arriba) / MEM (abajo) |
| Punto verde de Pod | Running |
| Punto amarillo de Pod | Pending |
| Punto rojo de Pod | Failed |
| Borde parpadeante del Pod | CrashLoop (restarts > 3) |

### Interacción

- **Clic en Pod** — abre el visor de logs de ese Pod
- **Estadísticas inferiores** — número de nodos Ready/NotReady, resumen de estado de Pods

---

## 8. Gestión de Nodos y Pods

### Página de Nodes

- Tabla de lista de nodos: nombre, rol, estado, CPU, memoria, número de Pods
- Cada columna admite búsqueda y filtrado
- Haga clic en el nombre del nodo para ver información detallada y todos los Pods de ese nodo

### Página de Pods

- Tabla de lista de Pods: nombre, namespace, estado, número de reinicios, nodo, antigüedad
- Admite filtrado por namespace y búsqueda en tiempo real

### Visor de Logs de Pod

Haga clic en una fila de Pod para abrir el visor de logs:

- **Streaming en tiempo real** — mediante SSE, los logs se actualizan en tiempo real
- **Resaltado por nivel de log** — ERROR (rojo), WARN (amarillo), DEBUG (gris)
- **Búsqueda y filtrado** — escriba palabras clave para filtrar líneas de log
- **Desplazamiento automático** — al llegar nuevos logs, se desplaza automáticamente al final (se puede pausar)
- **Descarga** — exporta los logs actuales a un archivo

---

## 9. Flujo de Eventos y Notificaciones

### Página de Events

Muestra los eventos del clúster K8s y admite:

- Búsqueda y filtrado en tiempo real
- Resaltado en rojo de eventos Warning
- Filtrado por namespace

### Flujo de Eventos en Tiempo Real

El lado derecho de la página Events tiene un panel Live Events:

- Haga clic en **Go Live** para activar el streaming en tiempo real mediante SSE
- Los nuevos eventos tienen una animación con insignia azul NEW
- Los eventos eliminados tienen una insignia roja DEL
- Los eventos Warning se resaltan automáticamente en rojo

### Centro de Notificaciones

El icono de campana en la parte derecha de la barra superior:

- Muestra un badge numérico rojo + animación de pulso cuando hay alertas
- Haga clic para expandir el panel desplegable
- Muestra los eventos Warning recientes y nodos NotReady
- Actualización automática cada 60 segundos

---

## 10. Explorador de Recursos y Editor YAML

### Página de Resources

Explore todos los recursos K8s en el clúster:

- Agrupados por API Group / Resource Type
- Haga clic en el nombre del recurso para ver la definición YAML
- Admite filtrado de selección múltiple por namespace

### Visor YAML

Haga clic en cualquier recurso para abrir la vista superpuesta YAML:

- Visualización formateada del YAML completo
- Botón **Copy** para copiar con un clic

### Editor YAML

Haga clic en el botón **Edit** en el visor YAML para entrar en modo de edición:

1. El contenido YAML se convierte en un textarea editable
2. Tras modificar, haga clic en **Apply** para enviar
3. El backend utiliza server-side apply (semántica de kubectl apply)
4. Muestra un mensaje verde si tiene éxito, o un mensaje de error rojo si falla

---

## 11. Control de Acceso RBAC

La página RBAC (requiere permisos de administrador) gestiona usuarios y roles.

### Gestión de Usuarios

- **Crear usuario** — nombre de usuario, contraseña, rol, ámbito de namespace
- **Editar usuario** — cambiar rol, habilitar/deshabilitar
- **Eliminar usuario**

### Roles

| Rol | Permisos |
|------|----------|
| admin | Lectura/escritura en todo el clúster, puede gestionar usuarios |
| operator | Lectura/escritura en la mayoría de recursos, no puede gestionar RBAC/Secrets |
| viewer | Acceso de solo lectura |

### Ámbito de Namespace

Cada usuario puede vincularse a un namespace específico y solo puede acceder a los recursos dentro de ese ámbito (implementado mediante K8s impersonation).

---

## 12. Registro de Auditoría

La página Audit muestra los registros de auditoría de todas las operaciones de IA.

### Funciones

- **Filtrado por Severity** — desplegable para seleccionar Info / Warning / Error / Critical
- **Búsqueda en tiempo real** — escriba palabras clave para filtrar
- **Tarjetas de estadísticas** — Total / Successful / Failed / Critical / Warnings
- **Tabla** — hora, nivel de gravedad, acción, recurso de destino, operador, éxito/fallo, duración

### Ámbito de Auditoría

Todas las siguientes operaciones se registran:

- Llamadas a herramientas de IA (kubectl get/describe/logs, etc.)
- Operaciones de reparación iniciadas por IA
- Llamadas a la API LLM
- Inicio/cierre de sesión de usuario
- Modificación de recursos

---

## 13. Configuración y Ajustes

La página Settings configura el proveedor de IA y la autenticación.

### Configuración del Proveedor de IA

| Campo | Descripción |
|-------|-------------|
| Provider Type | openai / deepseek / zai / anthropic |
| Model | gpt-4o / deepseek-chat / glm-4-plus, etc. |
| Endpoint | Dirección de la API LLM (dejar vacío para usar el valor predeterminado) |
| API Key | Clave de la API LLM |

### Configuración de Autenticación

- **Local** — sistema de usuarios integrado (predeterminado)
- **LDAP** — integración LDAP/AD empresarial
- **OIDC** — GitHub / Google / Keycloak, etc.

---

## 14. Atajos de Teclado

| Atajo | Función |
|-------|---------|
| `Ctrl+K` / `Cmd+K` | Abrir paleta de comandos |
| `Esc` | Cerrar paleta de comandos / ventanas emergentes |
| `↓` / `↑` | Seleccionar en la paleta de comandos |
| `Enter` | Confirmar en la paleta de comandos |

---

## 15. Cambio de Tema

Haga clic en el botón de luna/sol en la esquina superior derecha de la barra lateral para alternar entre el tema oscuro/claro. La selección se guarda en localStorage y se mantiene después de actualizar la página.

---

## Apéndice

### Documentación Relacionada

- [Diseño de Arquitectura](ARCHITECTURE.md)
- [Guía de Despliegue](DEPLOYMENT.md)
- [Ejecución Local](LOCAL_RUN.md)
- [Referencia de API](API.md)
- [Política de Seguridad](SECURITY.md)

### Preguntas Frecuentes

**P: ¿Chat no responde?**
R: Verifique que la configuración en Settings → Provider sea correcta y que la API Key sea válida.

**P: ¿No puedo ver ciertos namespaces?**
R: El rol RBAC del usuario actual puede limitar el ámbito de acceso a namespaces. Póngase en contacto con el administrador para ajustar.

**P: ¿El visor de logs del Pod está en blanco?**
R: El Pod puede haberse iniciado recientemente y aún no tener logs, o puede no tener permisos de logs. Verifique la configuración de RBAC.

**P: ¿Son seguros los comandos sugeridos por la IA?**
R: Todas las operaciones sugeridas por la IA primero pasan por una validación dry-run del Safety Checker. Las operaciones de reparación requieren aprobación manual antes de ejecutarse.

---

## 16. Planificación de Capacidad

### Monitoreo de Capacidad de Almacenamiento

**Ruta:** Dashboard → pestaña Capacity

Muestra el estado de almacenamiento de todos los PVC (PersistentVolumeClaim) del clúster:

| Métrica | Descripción |
|---------|-------------|
| Total PVCs | Número total de PVC en el clúster |
| Bound | Número de PVC vinculados a un PV |
| Pending | PVC pendientes de vinculación |
| Total Capacity | Capacidad total de todos los PVC |
| Requested | Cantidad total solicitada por todos los PVC |

### Análisis de Capacidad de Nodos

La página Capacity también muestra la utilización de recursos de cada nodo:

- **Utilización de CPU**: CPU solicitada / CPU asignable (código de color: <60% verde, 60-80% amarillo, >80% rojo)
- **Utilización de memoria**: memoria solicitada / memoria asignable
- **Densidad de Pods**: número de Pods en ejecución / límite máximo de Pods
- **Recomendaciones de expansión**: cuando los recursos del nodo superan el 80%, se generan automáticamente recomendaciones de expansión

### Resumen a Nivel de Clúster

| Métrica | Descripción |
|---------|-------------|
| Cluster CPU Utilization | Proporción de CPU solicitada/asignable en todo el clúster |
| Cluster Mem Utilization | Proporción de memoria solicitada/asignable en todo el clúster |
| Total CPU Allocatable | Cantidad total de CPU asignable en todo el clúster |
| Total CPU Requested | Cantidad total de CPU solicitada en todo el clúster |

---

## 17. Visualización de HPA

**Ruta:** Dashboard → pestaña HPA

Muestra el estado de autoescalado de todos los HorizontalPodAutoscaler:

### Funciones

- **Barra de escala de réplicas**: visualiza el número actual de réplicas, el deseado y el rango mínimo/máximo
- **Barra de utilización de métricas**: utilización actual de CPU/memoria vs valor objetivo (verde/amarillo/rojo)
- **Indicador de estado de escalado**: muestra una insignia "SCALING" cuando las réplicas actuales ≠ deseadas
- **Tarjetas de resumen**: número total de HPA, número en escalado, total de réplicas actuales/deseadas

### Tipos de Métricas Admitidos

| Tipo | Descripción |
|------|-------------|
| Resource | Porcentaje de utilización de CPU/memoria |
| Pods | Métricas personalizadas de Pod (como QPS) |
| External | Métricas externas (como longitud de cola SQS) |
| ContainerResource | Métricas de recursos a nivel de contenedor |

---

## 18. Inventario de Imágenes de Contenedor

**Ruta:** Dashboard → pestaña Images

Muestra todas las imágenes de contenedor en uso en el clúster:

| Métrica | Descripción |
|---------|-------------|
| Unique Images | Número total de imágenes únicas (deduplicadas) |
| Using :latest | Número de imágenes con etiqueta `:latest` (no recomendado para producción) |
| No Limits | Número de imágenes sin resource limits |
| No Requests | Número de imágenes sin resource requests |
| Registries | Número de registros de imágenes utilizados |

### Mejores Prácticas de Seguridad

- Evite usar la etiqueta `:latest` — use números de versión fijos para garantizar despliegues reproducibles
- Todos los contenedores deben tener limits de CPU/memoria — para prevenir el agotamiento de recursos
- Todos los contenedores deben tener requests de CPU/memoria — para garantizar una correcta asignación del planificador

---

## 19. Ranking de Recursos por Namespace

**Ruta:** Dashboard → pestaña Namespaces

Lista el uso de recursos de todos los namespaces ordenados por consumo de CPU:

### Funciones

- **Resumen de recursos**: requests + limits de CPU/memoria, número de Pods, almacenamiento PVC por cada namespace
- **Proporción del clúster**: porcentaje de CPU/memoria solicitada respecto al total asignable del clúster (con barra de progreso visual)
- **Búsqueda y filtrado**: localice rápidamente un namespace específico
- **Detalle drill-down**: haga clic en cualquier namespace para ver el uso de ResourceQuota, la configuración de LimitRange y los eventos Warning recientes

---

## 20. Cumplimiento de Seguridad

### Escaneo de Cumplimiento CIS Benchmark

**Ruta:** Dashboard → pestaña Compliance

Ejecuta comprobaciones del CIS Kubernetes Benchmark, cubriendo las siguientes categorías:

| Categoría | Elementos de Comprobación |
|-----------|--------------------------|
| RBAC | Alcance de enlace cluster-admin, ClusterRole con comodines, uso de SA predeterminado |
| Pod Security | Contenedores privilegiados, hostNetwork/hostPID/hostIPC, volúmenes hostPath, usuario root, limits de recursos |
| Network | Cobertura de NetworkPolicy |
| Secrets | Estado de gestión de Secrets |

### Descarga de Informe de Cumplimiento

Haga clic en el botón "Download Report" para descargar el informe de cumplimiento completo (formato .txt), que incluye:

- Puntuación de cumplimiento (porcentaje)
- Estado de cada comprobación (PASS/WARN/FAIL)
- Recomendaciones de corrección (para elementos WARN/FAIL)

### Búsqueda de Eventos de Auditoría

**Ruta:** API → `GET /api/audit/events`

Admite el filtrado del registro de auditoría por múltiples dimensiones:

| Parámetro | Descripción |
|-----------|-------------|
| `actor` | Filtrar por nombre de usuario |
| `action` | Filtrar por tipo de operación (como delete, scale, exec) |
| `q` | Búsqueda de texto completo |
| `severity` | Filtrar por nivel de gravedad |
| `from`/`to` | Rango de tiempo (formato RFC3339) |

### Exportación CSV

`GET /api/audit/export` — exporta los logs de auditoría en formato CSV, que puede importarse a un sistema SIEM para análisis de cumplimiento.

---

## 21. Gestión del Sistema

### Información del Sistema

`GET /api/system/info` proporciona información en tiempo de ejecución:

- Número de versión, versión de Go, plataforma de ejecución
- Uso de memoria (Alloc/Sys/GC cycles/Heap objects)
- Número de goroutines
- Tiempo de actividad del servicio
- Tamaño y número de eventos del log de auditoría

### Gestión de Logs

| API | Función |
|-----|---------|
| `POST /api/system/log/rotate` | Activar manualmente la rotación del log de auditoría (admin) |
| `POST /api/system/log/cleanup` | Limpiar archivos de rotación con más de 30 días (admin) |

### Configuración del Nivel de Log

Configurar mediante la variable de entorno `LOG_LEVEL` (debug/info/warn/error):

```bash
kubectl set env daemonset/k8ops -n k8ops-system LOG_LEVEL=debug
kubectl rollout restart daemonset/k8ops -n k8ops-system
```

### Gestión de Copias de Seguridad

| API | Función |
|-----|---------|
| `GET /api/system/backup` | Listar todos los archivos de copia de seguridad |
| `POST /api/system/backup` | Crear una copia de seguridad de la base de datos |
| `DELETE /api/system/backup?name=X` | Eliminar una copia de seguridad específica |
| `POST /api/system/backup/restore?name=X` | Restaurar la base de datos desde una copia de seguridad |

### Monitoreo de Rendimiento de la API

`GET /api/system/performance` proporciona estadísticas de latencia para cada endpoint de la API:

- Latencia en percentiles **p50/p95/p99**
- Latencia media y máxima
- Tasa de error y número total de solicitudes

---

## 22. API de Diagnóstico de Operaciones (v14.61+)

### Auditoría de Network Policy

`GET /api/security/network-policies` audita la cobertura de NetworkPolicy del clúster:

- Detección de namespaces sin NetworkPolicy (totalmente abiertos por defecto)
- Identificación de políticas permisivas (0.0.0.0/0 entrada/salida)
- Clasificación por nivel de gravedad: critical / warning / info
- Cada hallazgo incluye descripción detallada y recomendaciones de corrección

### Diagnóstico de Reinicios de Pod

`GET /api/diagnostics/restarts` diagnostica los patrones y causas raíz de los reinicios de Pod:

- Clasificación de patrones de reinicio: crash-loop / occasional / post-deploy
- Extracción de causa de terminación: OOMKilled / Error / código de salida
- Identificación de estados de espera: CrashLoopBackOff / ImagePullBackOff
- Información de diagnóstico independiente para cada contenedor

### Estado de Rollout de Despliegues

`GET /api/deployments/rollout` rastrea el estado de salud del rollout de todas las cargas de trabajo:

- Cubre Deployment / StatefulSet / DaemonSet
- 7 estados: complete / in-progress / stalled / degraded / paused / failed / scaled-to-zero
- Detección de ProgressDeadlineExceeded y ReplicaFailure
- Admite filtrado por estado: `?status=failed`

### Detección de Desperdicio de Recursos

`GET /api/resources/waste` escanea recursos desperdiciados y huérfanos para reducir costos:

- 6 categorías de desperdicio: servicios muertos, PVC sin usar, ConfigMap/Secret huérfanos, namespaces vacíos, PV sin vincular
- Evaluación de riesgo de costo: low / moderate / high
- Cada elemento incluye gravedad, antigüedad y recomendaciones de limpieza
- Filtrado inteligente de recursos del sistema (kube-system, SA token, Helm release)

### Detección de Cuellos de Botella de Escalado

`GET /api/scaling/bottlenecks` identifica factores que limitan la escalabilidad horizontal:

- 7 categorías de cuellos de botella: programación de nodos, presión de nodos, límites de cuota, HPA atascado, PDB bloqueante, agotamiento de almacenamiento
- Resumen de capacidad del clúster: número de nodos, CPU/memoria, capacidad de Pods, margen de expansión
- Cada elemento incluye nivel de impacto y recomendaciones de corrección

### Análisis de Riesgos de Permisos RBAC

`GET /api/security/rbac-risk` analiza los riesgos de seguridad de todos los enlaces RBAC del clúster:

- Sistema de puntuación 0-100, identifica automáticamente enlaces de alto riesgo
- 5 niveles de riesgo: critical / high / elevated / moderate / low
- Elementos de detección: enlaces cluster-admin, escalada de privilegios (escalate/bind/impersonate), permisos con comodines (verbs/resources: *), escritura a nivel de clúster, acceso a recursos sensibles (secrets/pods/exec)
- Cada elemento incluye un desglose detallado de la puntuación y recomendaciones de corrección (principio de mínimos privilegios)
- Admite filtrado por namespace: `?namespace=default`

### Monitoreo de Salud de Ejecución de CronJob

`GET /api/operations/cronjobs/health` monitorea la salud de ejecución de todos los CronJobs:

- 5 niveles de estado de salud: healthy / warning / failing / suspended / no-runs
- Detección de fallos consecutivos (3 o más = failing), tasa de éxito inferior al 50%, programaciones suspendidas, nunca ejecutados
- Asociación de CronJob y sus Jobs secundarios mediante OwnerReferences
- Cálculo del próximo tiempo de ejecución esperado
- Admite filtrado por namespace: `?namespace=production`

### Monitoreo de Salud de Red de Service y Endpoint

`GET /api/networking/health` escanea la conectividad de red de todos los Services e Ingress:

- 5 niveles de estado de salud de Service: healthy / degraded / no-endpoints / misconfigured / external
- Detección de selectores no coincidentes (label mismatch), todos los endpoints no disponibles, degradación parcial, LoadBalancer esperando IP
- Validación de backend de Ingress: si el Service backend existe y tiene endpoints disponibles
- Referencia cruzada de coincidencia de selectores de Pod, proporcionando análisis de causa raíz
- Admite filtrado por namespace: `?namespace=default`

### Monitoreo de Salud de Almacenamiento PV/PVC

`GET /api/storage/health` escanea la salud de almacenamiento de todos los PVC/PV:

- 6 niveles de estado de salud de PVC: bound / pending / lost / failed / orphaned / near-capacity
- Diagnóstico de Pending: sin clase de almacenamiento, modo de enlace WaitForFirstConsumer, comprobación de logs del provisioner
- Detección de PVC huérfanos: vinculados pero sin uso por ningún Pod durante más de 1 día (desperdicio de capacidad)
- Problemas de PV: Released (requiere limpieza manual), Failed (reciclaje fallido), Available obsoleto (>7 días)
- Distribución de StorageClass: clase predeterminada, provisioner, reclaim policy, soporte de volume expansion
- Admite filtrado por namespace: `?namespace=default`

### Auditoría de Seguridad de ServiceAccount

`GET /api/security/service-accounts` audita exhaustivamente los riesgos de seguridad de todas las ServiceAccounts del clúster:

- Sistema de puntuación de riesgo 0-100, identifica automáticamente SA de alto riesgo
- 5 niveles de gravedad: critical / high / elevated / moderate / low
- Elementos de detección: SA sin uso (>7 días), enlace cluster-admin (critical), SA predeterminado usado por Pods, montaje automático innecesario de token, SA obsoleto (>30 días con permisos pero sin uso), secret de token heredado de larga duración
- Cada elemento incluye una explicación detallada del riesgo de seguridad y recomendaciones de corrección
- Admite filtrado por namespace: `?namespace=default`

### Seguimiento de Presupuesto de Errores SLO/SLA

`GET /api/operations/slo` rastrea el cumplimiento de SLO/SLA basado en un algoritmo de múltiples ventanas y múltiples tasas de combustión:

- 5 ventanas de tiempo: 5 minutos, 1 hora, 6 horas, 24 horas, 7 días
- Porcentaje de disponibilidad y cantidad restante/tasa de consumo del presupuesto de errores
- Detección de tasa de combustión multi-ventana (fast: 5m+1h, slow: 6h+24h)
- Percentiles de latencia P50/P95/P99 y objetivo SLO
- 3 niveles de estado: meeting (cumple) / at-risk (en riesgo) / violated (violado)
- Admite filtrado por namespace: `?namespace=production`

### Monitoreo de ResourceQuota y LimitRange

`GET /api/resources/quota` escanea la utilización de cuotas y las restricciones de LimitRange de todos los namespaces:

- 4 niveles de estado de cuota: ok (<70%) / warning (70-85%) / critical (85-100%) / exceeded (>100%)
- Utilización de cuota de CPU/memoria/Pod/ConfigMap/Secret/almacenamiento por namespace
- Identificación de namespaces sin protección de cuota
- Análisis de restricciones predeterminadas/mínimas/máximas de LimitRange
- Ranking de los principales consumidores
- Admite filtrado por namespace: `?namespace=default`

### Auditoría de Configuración de Despliegues

`GET /api/deployments/audit` audita las violaciones de mejores prácticas de configuración de todas las cargas de trabajo:

- 8 categorías de comprobación: revision-history / image-policy / resources / probes / security-context / update-strategy / lifecycle / config-drift
- Cada elemento incluye gravedad (critical/warning/info), descripción específica del problema y recomendaciones de corrección accionables
- Puntuación de salud de 0 (perfecto) a 100 (peor)
- Hallazgos principales agregados que muestran los problemas más comunes en todo el clúster
- Admite filtrado por namespace y gravedad: `?namespace=default&severity=critical`

### Salud de Programación y Análisis de Fragmentación de Recursos

`GET /api/scheduling/health` analiza la salud de programación del clúster y la utilización de recursos:

- Programabilidad por nodo (aislamiento/taint/condiciones de presión) y disponibilidad de recursos
- Diagnóstico de Pods Pending: análisis de causas de eventos FailedScheduling (CPU/memoria insuficiente, taint no coincidente, conflicto de nodeSelector, fallo de enlace de volumen, etc.)
- Cálculo del Pod programable máximo (el Pod más grande que se puede desplegar actualmente)
- Capacidad efectiva vs capacidad teórica (pérdida de capacidad debido a nodos no programables)
- Análisis de fragmentación de recursos (capacidad libre dispersa)
- Detección de Pods extragrandes (solicitudes que exceden la capacidad de cualquier nodo individual)
- Historial de desalojos de 24h (con causas)
- Puntuación de salud 0-100 (penalización ponderada)
- Recomendaciones de corrección accionables
- Admite filtrado por namespace: `?namespace=default`

### Escaneo de Postura de Seguridad de Pods

`GET /api/security/pods?namespace=xxx&severity=critical` audita la postura de seguridad en tiempo real de todos los Pods en ejecución:

- 15 categorías de comprobación que cubren contenedores privilegiados, acceso al host (network/PID/IPC), montajes HostPath, capabilities peligrosas, ejecución como root, escalada de privilegios, etc.
- Puntuación de riesgo por Pod de 0-100 (critical=25 puntos/warning=8 puntos/info=2 puntos)
- Estadísticas agregadas por tipo de comprobación y namespace
- Admite filtrado por namespace y gravedad

### Detección de Tormentas de Eventos y Fallos en Cascada

`GET /api/operations/event-storm?namespace=xxx` analiza los eventos Warning del clúster:

- 4 niveles de gravedad de tormenta: critical (>50) / high (>20) / medium (>10) / low (>5)
- Detección de recursos fluctuantes (mismo recurso, misma causa repetida 3+ veces, con frecuencia de fluctuación)
- Agregación por namespace y causa de evento
- Evaluación del radio de explosión (número de recursos afectados)
- Recomendaciones de investigación accionables
- Admite filtrado por namespace: `?namespace=kube-system`

### Grafo de Dependencias de Recursos y Análisis de Impacto

`GET /api/dependencies?kind=Deployment&name=xxx&namespace=xxx` rastrea el grafo completo de dependencias de las cargas de trabajo:

- Dependencias directas: ConfigMap, Secret, PVC, ServiceAccount
- Dependencias inversas: Service (label selector), Ingress, NetworkPolicy, HPA, otros Pods que comparten configuración
- Evaluación del alcance del impacto: puntuación blastRadius y nivel de riesgo
- Útil para la evaluación de impacto previa a cambios, para evitar fallos en cascada

### Comprobación de Cumplimiento de Distribución Topológica

`GET /api/topology/spread?namespace=xxx&domain=topology.kubernetes.io/zone` verifica el cumplimiento de la distribución topológica de los Pods:

- 4 niveles de estado de carga de trabajo: balanced / skewed / no-constraint / single-replica
- Análisis de distribución y desviación por dominio topológico por carga de trabajo
- Detección de cargas de trabajo multi-réplica sin restricciones topológicas
- Identificación de nodos sin etiquetas topológicas
- Sugerencias para clústeres de dominio único
- Admite filtrado por namespace y por clave de dominio topológico

### Auditoría de Rotación y Ciclo de Vida de Secrets

`GET /api/security/secrets/rotation?namespace=xxx` audita el ciclo de vida de todos los Secrets:

- Seguimiento de antigüedad: stale (>90d) / very stale (>180d)
- Detección de Secrets sin uso (no referenciados por ningún Pod)
- Detección de expiración de certificados TLS (análisis de certificados, detección de expirados y <30d)
- Seguimiento de Secret de registro Docker, tokens de SA heredados
- Detección de nombres sensibles (password/key/token/credential)
- Nivel de riesgo por Secret, puntuación de rotación del clúster 0-100
- Admite filtrado por namespace

### Auditoría de Eficacia de Sondas de Salud

`GET /api/operations/probes?namespace=xxx` audita la configuración de sondas:

- 8 categorías de comprobación: sondas faltantes, demasiado agresivas, timeout demasiado corto, umbrales inadecuados, etc.
- Puntuación de riesgo por carga de trabajo, puntuación de eficacia del clúster (0-100)
- Estadísticas agregadas de los principales problemas
- Recomendaciones accionables

### Seguimiento de Obsolescencia de Cargas de Trabajo

`GET /api/product/staleness?namespace=xxx` rastrea la obsolescencia de los despliegues:

- 5 niveles de clasificación de obsolescencia: fresh/recent/stale/very-stale/ancient
- Análisis de etiquetas de imagen: :latest, digest, sin etiqueta
- Intervalos de distribución de antigüedad, estadísticas por namespace
- Puntuación de frescura del clúster (0-100)

### Análisis de Sobreventa y Presión de Recursos

`GET /api/scalability/overcommit?namespace=xxx` analiza la sobreventa de recursos:

- Ratios de sobreventa de request y limit de CPU/memoria por nodo
- Puntuación de presión 0-100 y nivel de riesgo
- Detección de Pods sin limits/requests
- Puntuación de seguridad del clúster 0-100
- Desglose de consumo de recursos por namespace

### Análisis de Seguridad y Cadena de Suministro de Imágenes

`GET /api/security/images?namespace=xxx` escanea la seguridad de la cadena de suministro de todas las imágenes de contenedor:

- Detección de bloqueo por Digest (@sha256: referencia inmutable)
- Detección de etiqueta :latest (mutable, no reproducible)
- Detección de imágenes sin etiqueta (predeterminado :latest)
- Detección de etiquetas de versión antiguas (v1, 1.0 — pueden contener CVEs conocidos)
- Análisis de registros de imágenes públicos vs privados
- Nivel de riesgo por imagen, estadísticas por registro
- Puntuación de seguridad de imágenes del clúster 0-100

### Planificación de Capacidad

`GET /api/capacity/planning` planificación de capacidad de nodos:

- Requests de CPU/memoria vs asignable por nodo
- Capacidad restante y recomendaciones de expansión
- Detección de fragmentación de recursos

### Previsión de Capacidad

`GET /api/capacity/forecast` previsión de tendencias de capacidad:

- Tendencias de crecimiento de recursos basadas en datos históricos
- Tiempo estimado de agotamiento
- Recomendaciones de expansión

### Análisis de Eficiencia de Recursos

`GET /api/efficiency` eficiencia de uso de recursos:

- Detección de asignación de recursos excesiva
- Identificación de desperdicio de recursos
- Recomendaciones de optimización

### Estado de PDB

`GET /api/pdbs` estado de Pod Disruption Budget:

- Comprobación de configuración de PDB
- Disrupciones permitidas vs disponibles actuales
- Detección de PDBs bloqueantes

### Compatibilidad de Versiones

`GET /api/compatibility` compatibilidad de versiones de K8s:

- Comprobación de APIs obsoletas
- Compatibilidad de versiones de recursos
- Evaluación de impacto de actualización

### Expiración de Certificados

`GET /api/certificates/expiry` escaneo de expiración de certificados TLS:

- Tiempo de expiración de certificados del clúster
- Advertencia de certificados próximos a expirar
- Recomendaciones de renovación

### Salud de Addons

`GET /api/addons/health` comprobación de salud de addons del clúster:

- Estado de ejecución de addons principales
- Detección de addons anómalos
- Recomendaciones de corrección

### Puntuación de Salud del Clúster

`GET /api/operations/health-score` agrega todas las señales de salud del clúster en una puntuación integral:

- 5 dimensiones ponderadas: Node(25%) + Pod(25%) + Workload(20%) + Events(15%) + API Server(15%)
- Puntuación total 0-100, calificación alfabética A-F
- Estado: healthy / warning / critical
- Puntuación, peso y detalles por dimensión
- Resumen del clúster: recuento de nodos/Pods/cargas de trabajo
- Principales problemas ordenados por gravedad

### Recomendaciones de Configuración Racional de Recursos HPA/VPA

`GET /api/scalability/autoscale-recommendations?namespace=xxx` analiza el autoescalado y el right-sizing de recursos:

- Detección de cargas de trabajo multi-réplica sin HPA
- Requests de CPU excesivos (>1 core/contenedor)
- Requests de memoria excesivos (>2GB/contenedor)
- Análisis de eficiencia de HPA (alcanza límite superior/inferior/inactivo)
- Valores de recursos actuales vs recomendados por carga de trabajo
- Ahorro potencial de núcleos de CPU y memoria
- Puntuación de autoescalado del clúster 0-100

### Monitoreo de Salud de Ingress y Enrutamiento de Tráfico

`GET /api/product/ingress-health?namespace=xxx` verifica la salud del enrutamiento de tráfico de todos los Ingress:

- Verificación de existencia del Service backend y disponibilidad de endpoints
- Detección de configuración TLS
- Validación de IngressClass
- Detección de conflictos host+path
- Detección de reglas de enrutamiento ausentes
- Estado por Ingress y puntuación de salud del clúster 0-100

### Condiciones del Nodo y Presión de Recursos

`GET /api/operations/node-pressure` analiza las condiciones y presión de recursos de todos los nodos:

- Detección de DiskPressure / MemoryPressure / PIDPressure / NetworkUnavailable
- Utilización de CPU/memoria/Pods vs asignable
- Nivel de riesgo por nodo (critical/high/medium/low)
- Puntuación de presión del clúster 0-100

### Enlace de PVC y Rendimiento de Almacenamiento

`GET /api/scalability/pvc-analysis?namespace=xxx` analiza la salud del enlace de almacenamiento:

- Detección de causa raíz de PVC atascados (>5min pending)
- Medición del tiempo de enlace y detección de enlaces lentos (>30s)
- Detección de PVC Lost
- Estadísticas por StorageClass y análisis de provisioner
- Puntuación de salud de almacenamiento del clúster 0-100

### Gobernanza de Namespace y Ciclo de Vida

`GET /api/product/namespaces/lifecycle` audita la gobernanza de namespaces:

- Cobertura de ResourceQuota / LimitRange / NetworkPolicy
- Detección de ServiceAccount dedicado (mínimos privilegios)
- Comprobación de etiquetas requeridas (app, team, env, owner)
- Ciclo de vida del namespace (active / stale / terminating)
- Puntuación de gobernanza del clúster 0-100

### Análisis de Permisos Efectivos RBAC y Escalada de Privilegios

`GET /api/security/rbac-effective` analiza los permisos efectivos RBAC de todos los sujetos:

- Agregación de ClusterRoleBindings + RoleBindings para calcular permisos reales
- Detección de equivalencia cluster-admin
- Detección de rutas de escalada de privilegios (sujetos que pueden modificar RBAC)
- Detección de permisos con comodines (*)
- Análisis de acceso de lectura de Secrets y Pod exec
- Puntuación de seguridad RBAC del clúster 0-100

### Seguimiento de OOM Kill de Contenedores

`GET /api/operations/oom-tracker?namespace=xxx` rastrea eventos OOM de contenedores:

- Detección de contenedores OOMKilled y análisis de causa raíz
- Detección de alto número de reinicios (>=5)
- Detección de límites de memoria faltantes o demasiado bajos
- Riesgo de presión de nodos por límites muy superiores a requests (10x+)
- Ranking de los principales OOM y estadísticas por namespace
- Puntuación de riesgo OOM del clúster 0-100

### Previsión de Agotamiento de Capacidad de Almacenamiento

`GET /api/scalability/storage-forecast` prevé la capacidad de almacenamiento:

- Utilización por PV, tasa de crecimiento, predicción de días hasta agotamiento
- Soporte de anotación Longhorn actual-size
- Estimación de días hasta llenado del clúster
- Estadísticas por StorageClass y análisis de provisioner
- Ranking de namespaces de alto riesgo
- Puntuación de salud de almacenamiento 0-100

### Comprobación de Salud de Resolución DNS

`GET /api/product/dns-health` analiza la salud de resolución DNS:

- Comprobación de salud de Pods CoreDNS (ejecución/preparados/reinicios/versión)
- Análisis de configuración Corefile (forwarders, plugins)
- Cobertura de endpoints de Headless Service y riesgo NXDOMAIN
- Detección de caché NodeLocal DNS
- Detección de cobertura dnsConfig ndots de Pods
- Descubrimiento de servicios gestionado por External-DNS
- Puntuación de salud DNS del clúster 0-100
