# Manual de Operaciones de k8ops (Runbook)

> Este documento está dirigido al personal de operaciones y cubre las operaciones de mantenimiento diarias, los procesos de manejo de fallos, los contactos de emergencia y los procedimientos operativos estándar.

---

## Tabla de Contenidos

1. [Resumen del Servicio](#1-resumen-del-servicio)
2. [Operaciones Diarias](#2-operaciones-diarias)
3. [Manejo de Fallos](#3-manejo-de-fallos)
4. [Operaciones de Emergencia](#4-operaciones-de-emergencia)
5. [Copia de Seguridad y Recuperación](#5-copia-de-seguridad-y-recuperación)
6. [Ajuste de Rendimiento](#6-ajuste-de-rendimiento)
7. [Contactos de Emergencia](#7-contactos-de-emergencia)
8. [Definición de SLO/SLA](#8-definición-de-slosla)

---

## 1. Resumen del Servicio

### Descripción de la Arquitectura

```
┌─────────────────────────────────────────────────┐
│                Navegador del usuario             │
│              https://k8ops.iot2.win               │
└───────────────────┬─────────────────────────────┘
                    │ HTTPS (Traefik Ingress)
┌───────────────────▼─────────────────────────────┐
│              Traefik (kube-system)               │
│         websecure: 8443 → 8000                   │
└───────────────────┬─────────────────────────────┘
                    │
┌───────────────────▼─────────────────────────────┐
│            k8ops DaemonSet (k8ops-system)         │
│  ┌─────────────────────────────────────────────┐ │
│  │  Go Binary (frontend integrado)                │ │
│  │  :8080 Dashboard                             │ │
│  │  /metrics  Prometheus                        │ │
│  │  /api/chat  SSE → LLM Provider               │ │
│  │  nsenter → host kubectl                      │ │
│  └─────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────┘
```

### Componentes Clave

| Componente | Ubicación | Función |
|------------|-----------|---------|
| k8ops DaemonSet | k8ops-system | Servicio principal, un Pod por nodo |
| Traefik | kube-system | Controlador Ingress, terminación TLS |
| Registry | registry.iot2.win | Registro de imágenes privado |
| LLM Provider | API externa | Motor de Chat IA / diagnóstico / optimización |

### Endpoints de Comprobación de Salud

| Endpoint | Respuesta Esperada | Notas |
|----------|-------------------|-------|
| `https://k8ops.iot2.win/` | 200/303 | Página frontal |
| `https://k8ops.iot2.win/readyz` | 200 | Sonda de preparación K8s |
| `https://k8ops.iot2.win/api/version` | 200 JSON | Información de versión |
| `https://k8ops.iot2.win/metrics` | 200 (solo local) | Métricas Prometheus |

---

## 2. Operaciones Diarias

### 2.1 Verificar el Estado del Servicio

```bash
# Estado de Pods
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops

# Logs del servicio (últimas 100 líneas)
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --tail=100

# Información de versión
curl -sk https://k8ops.iot2.win/api/version | jq .

# Resumen del clúster
curl -sk https://k8ops.iot2.win/api/cluster/overview | jq .
```

### 2.2 Actualizar el Despliegue

```bash
# Construir nueva versión
cd /Volumes/new/ggai/k8ops
VERSION=v14XX
docker buildx build --platform linux/amd64 \
  --build-arg VERSION=$VERSION \
  -t registry.iot2.win/k8ops:$VERSION \
  -t registry.iot2.win/k8ops:latest \
  --push .

# Actualización continua
kubectl set image daemonset/k8ops \
  k8ops=registry.iot2.win/k8ops:$VERSION -n k8ops-system

# Verificación
sleep 15
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops
curl -sk -o /dev/null -w '%{http_code}' https://k8ops.iot2.win/
```

### 2.3 Gestión de Logs

k8ops utiliza logs estructurados `log/slog`, el nivel de log se controla mediante la variable de entorno `LOG_LEVEL`:

| Nivel | Uso |
|-------|-----|
| `DEBUG` | Depuración de desarrollo, muestra todos los logs |
| `INFO` (predeterminado) | Ejecución en producción, registra operaciones clave |
| `WARN` | Solo advertencias y errores |

```bash
# Cambiar nivel de log
kubectl set env daemonset/k8ops LOG_LEVEL=DEBUG -n k8ops-system
```

### 2.4 Configuración del Proveedor

Las funciones de IA requieren configurar un proveedor LLM:

1. Visite la página Settings → Provider
2. Seleccione un proveedor (OpenAI / Zhipu / DeepSeek, etc.)
3. Introduzca la API Key
4. Pruebe la conexión

Si no está configurado, el Dashboard mostrará una advertencia de Provider no configurado.

---

## 3. Manejo de Fallos

### 3.1 El Pod No Se Inicia (CrashLoopBackOff)

**Síntoma**: El Pod de k8ops se reinicia repetidamente

**Pasos de investigación**:
```bash
# 1. Ver eventos del Pod
kubectl describe pod -n k8ops-system -l app.kubernetes.io/name=k8ops

# 2. Ver logs del contenedor
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --previous

# 3. Verificar permisos RBAC
kubectl auth can-i --list --as=system:serviceaccount:k8ops-system:k8ops

# 4. Comprobar montaje de ConfigMap/Secret
kubectl exec -n k8ops-system -it deploy/k8ops -- ls -la /etc/k8ops/
```

**Causas comunes**:
- Permisos RBAC insuficientes → verifique `config/rbac/`
- kubeconfig no válido → verifique el kubeconfig montado
- Conflicto de puertos → verifique si el puerto 8080 está ocupado
- Memoria insuficiente → verifique los recursos del nodo `kubectl describe nodes`

### 3.2 El Dashboard No Es Accesible (502/503)

**Síntoma**: https://k8ops.iot2.win devuelve 502 o 503

**Pasos de investigación**:
```bash
# 1. Verificar Ingress
kubectl get ingress -A | grep k8ops

# 2. Verificar Traefik
kubectl get pods -n kube-system -l app.kubernetes.io/name=traefik
kubectl logs -n kube-system -l app.kubernetes.io/name=traefik --tail=50

# 3. Verificar el Service de k8ops
kubectl get svc -n k8ops-system
kubectl get endpoints -n k8ops-system

# 4. Probar el Pod directamente
kubectl exec -n k8ops-system -it deploy/k8ops -- curl -s localhost:8080/api/version
```

**Causas comunes**:
- Traefik no enruta correctamente → verifique las reglas de Ingress
- k8ops no está listo → verifique la sonda readyz
- Certificado TLS expirado → verifique cert-manager

### 3.3 AI Chat No Responde

**Síntoma**: Después de enviar un mensaje en Chat, no hay respuesta o se agota el tiempo

**Pasos de investigación**:
```bash
# 1. Verificar estado del Provider
curl -sk https://k8ops.iot2.win/api/provider/status | jq .

# 2. Ver logs del motor
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -i 'llm\|provider\|chat'

# 3. Probar la conexión del Provider
curl -sk https://k8ops.iot2.win/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"message":"hello","conversationId":"test"}' --max-time 30
```

**Causas comunes**:
- API Key no configurada o expirada
- Limitación de tasa (rate limit) de la API del Provider (429)
- Red no accesible (DNS/firewall)
- Tokens excedidos → el Agent comprime automáticamente el contexto, pero puede fallar en casos extremos

### 3.4 Fallo de Push al Registry (499)

**Síntoma**: `docker push` devuelve 499 Client Closed Request

**Solución**:
```bash
# Verificar configuración de timeout de Traefik
kubectl get deploy -n kube-system traefik -o jsonpath='{.spec.template.spec.containers[0].args}'

# Si faltan parámetros de timeout, añadir:
kubectl patch deploy -n kube-system traefik --type='json' -p='[
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.readtimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.writetimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.idletimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.keepalivemaxtime=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.keepalivemaxrequests=0"}
]'
```

### 3.5 Fallo en Operaciones de Escritura (Scale/Delete/Restart)

**Síntoma**: Las operaciones de Scale/Delete/Restart fallan al hacer clic en los botones

**Pasos de investigación**:
```bash
# Verificar permisos RBAC
kubectl auth can-i patch deployments --as=system:serviceaccount:k8ops-system:k8ops -n default
kubectl auth can-i delete pods --as=system:serviceaccount:k8ops-system:k8ops -n default

# Ver log de auditoría
curl -sk https://k8ops.iot2.win/api/audit?severity=critical | jq .

# Verificar políticas de seguridad
kubectl get psp,podsecurity --all-namespaces 2>/dev/null
```

---

## 4. Operaciones de Emergencia

### 4.1 Rollback Rápido

```bash
# Ver historial de versiones
kubectl rollout history daemonset/k8ops -n k8ops-system

# Rollback a la versión anterior
kubectl rollout undo daemonset/k8ops -n k8ops-system

# Rollback a una versión específica
kubectl rollout undo daemonset/k8ops -n k8ops-system --to-revision=3
```

### 4.2 Reducción de Emergencia (Escalar a 0 Réplicas)

```bash
# Nota: DaemonSet no soporta scale 0, es necesario eliminar directamente
kubectl delete pod -n k8ops-system -l app.kubernetes.io/name=k8ops --grace-period=0 --force

# Para detener completamente, modificar temporalmente el nodeSelector
kubectl patch daemonset k8ops -n k8ops-system -p='{"spec":{"template":{"spec":{"nodeSelector":{"non-existent":"true"}}}}}'
```

### 4.3 Limpieza de Datos

```bash
# Limpiar CRD de historial de diagnóstico
kubectl delete diagnostics --all --all-namespaces

# Limpiar logs de auditoría (conservar los últimos 7 días)
kubectl get auditlogs -A -o json | jq '.items[] | select(.metadata.creationTimestamp < "'$(date -d '7 days ago' -Iseconds)'")' | kubectl delete -f -

# Limpiar informes de optimización
kubectl delete optimizations --all --all-namespaces
```

---

## 5. Copia de Seguridad y Recuperación

### 5.1 Copia de Seguridad de Configuración

```bash
# Copia de seguridad de la configuración de k8ops
kubectl get cm,secret,daemonset -n k8ops-system -o yaml > k8ops-backup-$(date +%Y%m%d).yaml

# Copia de seguridad de datos CRD
kubectl get diagnostics,remediations,optimizations -A -o yaml > k8ops-crd-backup-$(date +%Y%m%d).yaml

# Copia de seguridad de RBAC
kubectl get clusterrole,clusterrolebinding -o yaml | grep -A5 k8ops > k8ops-rbac-backup-$(date +%Y%m%d).yaml
```

### 5.2 Proceso de Recuperación

```bash
# Restaurar configuración
kubectl apply -f k8ops-backup-YYYYMMDD.yaml

# Restaurar datos CRD
kubectl apply -f k8ops-crd-backup-YYYYMMDD.yaml

# Verificación
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops
curl -sk https://k8ops.iot2.win/api/version | jq .
```

### 5.3 Recomendación de Copia de Seguridad Periódica

Use Velero o un cron job para hacer una copia de seguridad diaria:
```bash
# Copia de seguridad con Velero (recomendado)
velero backup create k8ops-daily-$(date +%Y%m%d) \
  --include-namespaces k8ops-system \
  --include-cluster-resources=true
```

---

## 6. Ajuste de Rendimiento

### 6.1 Métricas Clave

| Métrica | Prometheus Metric | Umbral de Alerta |
|---------|-------------------|------------------|
| Latencia de API | `k8ops_tool_call_duration_seconds` | P99 > 10s |
| Latencia de llamada LLM | `k8ops_llm_call_duration_seconds` | P99 > 60s |
| Diagnósticos activos | `k8ops_active_diagnostics` | > 10 |
| Bloqueos de seguridad | `k8ops_safety_blocks_total` | rate > 10/min |
| Consumo de tokens | `k8ops_llm_tokens_total` | Crecimiento anormal diario |
| Puntuación de salud del clúster | `k8ops_cluster_health_score` | < 60 |

### 6.2 Recomendaciones de Recursos

| Tamaño del Nodo | Request de Recursos k8ops | Límite de Recursos |
|-----------------|---------------------------|-------------------|
| <= 5 nodos | 100m CPU / 128Mi | 500m CPU / 512Mi |
| 5-20 nodos | 200m CPU / 256Mi | 1 CPU / 1Gi |
| 20-50 nodos | 500m CPU / 512Mi | 2 CPU / 2Gi |

### 6.3 Optimización del Nivel de Log

Se recomienda mantener el nivel `INFO` en producción. Cambie temporalmente a `DEBUG` solo para investigación de problemas:
```bash
# Activar DEBUG temporalmente
kubectl set env daemonset/k8ops LOG_LEVEL=DEBUG -n k8ops-system
# Restaurar después de la investigación
kubectl set env daemonset/k8ops LOG_LEVEL=INFO -n k8ops-system
```

---

## 7. Contactos de Emergencia

### 7.1 Proceso de Escalado

```
Detección de fallos → Operador de guardia (L1)
    ├── No resuelto en 5 minutos → Responsable de operaciones (L2)
    │     ├── No resuelto en 15 minutos → Arquitecto (L3)
    │     │     ├── Impacto en producción → Notificación al CTO
```

### 7.2 Tabla de Contactos

> Complete según su situación real

| Rol | Nombre | Teléfono | Ámbito de Responsabilidad |
|-----|--------|----------|---------------------------|
| L1 Operador de guardia | ____ | ____ | Primera respuesta, manejo de fallos básicos |
| L2 Responsable de operaciones | ____ | ____ | Fallos complejos, impacto en múltiples servicios |
| L3 Arquitecto | ____ | ____ | Problemas a nivel de arquitectura, recuperación de datos |
| Administrador del clúster | ____ | ____ | Fallos del clúster K8s en sí |
| Red/Seguridad | ____ | ____ | Políticas de red, certificados, incidentes de seguridad |

### 7.3 Contacto de Proveedores

| Proveedor | Uso | Contacto |
|-----------|-----|----------|
| LLM Provider | Chat IA/Diagnóstico | ____ |
| Registry | Registro de imágenes | ____ |
| DNS/CDN | Resolución de nombres | ____ |

---

## Apéndice: Lista de Métricas de Prometheus

k8ops expone las siguientes métricas personalizadas (endpoint `/metrics`):

| Metric | Tipo | Etiquetas | Descripción |
|--------|------|-----------|-------------|
| `k8ops_diagnostics_total` | Counter | phase, trigger | Número total de informes de diagnóstico |
| `k8ops_remediation_actions_total` | Counter | type, result, risk | Número total de operaciones de reparación |
| `k8ops_llm_call_duration_seconds` | Histogram | provider, model, status | Latencia de llamadas LLM |
| `k8ops_llm_tokens_total` | Counter | provider, model, type | Consumo de tokens |
| `k8ops_agent_steps` | Histogram | - | Pasos de ejecución del Agent |
| `k8ops_tool_call_duration_seconds` | Histogram | tool, success | Latencia de llamadas a herramientas |
| `k8ops_safety_blocks_total` | Counter | reason | Número de bloqueos de seguridad |
| `k8ops_active_diagnostics` | Gauge | - | Número de diagnósticos activos actuales |
| `k8ops_active_remediations` | Gauge | - | Reparaciones en ejecución actualmente |
| `k8ops_audit_events_total` | Counter | type, severity | Número total de eventos de auditoría |
| `k8ops_cluster_health_score` | Gauge | - | Puntuación de salud del clúster (0-100) |
| `k8ops_conversation_count` | Gauge | - | Número de conversaciones activas |
| `k8ops_tool_executions_total` | Counter | tool, success | Número total de ejecuciones de herramientas |
| `k8ops_http_requests_total` | Counter | method, path, status | Número total de solicitudes HTTP |
| `k8ops_http_request_duration_seconds` | Histogram | method, path | Latencia de solicitudes HTTP |
| `k8ops_http_requests_in_flight` | Gauge | - | Número de solicitudes en proceso actualmente |
| `k8ops_api_errors_total` | Counter | method, path, status | Número de errores de API (4xx+5xx) |

---

## 8. Definición de SLO/SLA

### 8.1 Objetivos a Nivel de Servicio (SLO)

| Métrica | Objetivo | Ventana de Medición | Presupuesto de Errores |
|---------|----------|---------------------|------------------------|
| Disponibilidad del Dashboard | 99.9% | 30 días móvil | 43.2 minutos/mes |
| Tasa de éxito de API (no 429) | 99.5% | 30 días móvil | 3.6 horas/mes |
| Latencia P99 de API | < 2s | En tiempo real | - |
| Tiempo de respuesta de AI Chat | < 30s (primer token) | En tiempo real | - |
| Finalización del escaneo de auditoría de seguridad | < 60s | En tiempo real | - |

### 8.2 Gestión del Presupuesto de Errores

Objetivo de disponibilidad mensual del 99.9% = **43.2 minutos de presupuesto de errores**:

- **Dentro del presupuesto (<30min)**: Ritmo de publicación normal, sin aprobación adicional necesaria
- **Advertencia de presupuesto (30-43min)**: Congelar cambios no urgentes, priorizar la corrección de problemas de fiabilidad
- **Presupuesto agotado (>43min)**: Congelación total de publicaciones, realizar análisis post-mortem

### 8.3 Consultas de Monitoreo SLO (Prometheus PromQL)

**Tasa de errores de API (5 minutos):**
```promql
sum(rate(k8ops_api_errors_total{status=~"5.."}[5m])) by (path)
/ sum(rate(k8ops_http_requests_total[5m])) by (path)
```

**Latencia P99 de API:**
```promql
histogram_quantile(0.99,
  sum(rate(k8ops_http_request_duration_seconds_bucket[5m])) by (le, path)
)
```

**Tasa de consumo del presupuesto de errores:**
```promql
1 - (
  sum(rate(k8ops_http_requests_total{status!~"5.."}[30d]))
  / sum(rate(k8ops_http_requests_total[30d]))
)
```

### 8.4 Estrategia de Degradación

Cuando el SLO está a punto de incumplirse, degradar por orden de prioridad:

1. **Desactivar AI Chat** — la función con mayor consumo de recursos, su degradación no afecta la gestión central de K8s
2. **Aumentar el TTL de caché** — aumentar la caché de overview/nodes/pods de 30s a 120s
3. **Limitar diagnósticos concurrentes** — reducir el límite máximo de `k8ops_active_diagnostics`
4. **Cerrar el recolector de eventos** — flag `--disable-event-collector`

### 8.5 Seguimiento de Solicitudes

Todas las respuestas HTTP incluyen la cabecera `X-Request-ID`, utilizada para:
- Correlación de logs — todas las líneas de log de la misma solicitud comparten el request_id
- Seguimiento de auditoría — el request_id en el log de auditoría puede vincularse a la solicitud HTTP específica
- Investigación de fallos — cuando un usuario reporta un problema, proporcionar el request_id permite localizar rápidamente los logs

Ejemplo de consulta de logs:
```bash
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep "a1b2c3d4e5f6"
```

### 8.6 Configuración del Nivel de Log

k8ops utiliza logs estructurados JSON (slog), con soporte para configurar el nivel mediante la variable de entorno `LOG_LEVEL` o el argumento de línea de comandos `--log-level`:

| Nivel | Uso | Descripción |
|-------|-----|-------------|
| `debug` | Investigación de problemas | Incluye source file:line, logs muy detallados (no recomendado para producción) |
| `info` | Predeterminado | Logs de operación normal (recomendado para producción) |
| `warn` | Solo advertencias | Solicitudes lentas, problemas de configuración, cerca del umbral |
| `error` | Solo errores | Registrar solo fallos de operación |

Métodos de configuración:
```bash
# Mediante variable de entorno (recomendado)
kubectl set env daemonset/k8ops -n k8ops-system LOG_LEVEL=debug

# Mediante ConfigMap
kubectl patch configmap k8ops-config -n k8ops-system \
  --type='json' -p='[{"op":"add","path":"/data/log-level","value":"debug"}]'

# Mediante argumento de línea de comandos (solo aplicable al modo Deployment)
# args:
# - --log-level=debug
```

Reiniciar el Pod después de cambiar el nivel:
```bash
kubectl rollout restart daemonset/k8ops -n k8ops-system
```

### 8.7 Rotación de Logs

El archivo de log de auditoría (`/data/k8ops-audit.jsonl`) se rota automáticamente:
- **Rotación automática**: el archivo se divide automáticamente cuando supera 100MB
- **Rotación manual**: `POST /api/system/log/rotate` (permisos de admin)
- **Limpieza de archivos antiguos**: `POST /api/system/log/cleanup` (elimina archivos de rotación con más de 30 días)

Los logs stdout del contenedor son gestionados por Kubelet, con un límite predeterminado de 10MB x 3 archivos = 30MB por contenedor.
En k3s se pueden ajustar mediante `--container-log-max-size` y `--container-log-max-files`.

---

*Última actualización: 2026-07-02*
*Mantenedores: Equipo de k8ops*
