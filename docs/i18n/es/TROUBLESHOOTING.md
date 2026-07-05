# Guía de Solución de Problemas de k8ops

> Este documento recopila métodos de diagnóstico y soluciones para problemas comunes de k8ops, clasificados por nivel de gravedad para facilitar la localización rápida.

---

## Tabla de Contenidos

1. [Problemas de Instalación e Inicio](#1-problemas-de-instalación-e-inicio)
2. [Problemas de Autenticación e Inicio de Sesión](#2-problemas-de-autenticación-e-inicio-de-sesión)
3. [Problemas de Funcionalidad de IA](#3-problemas-de-funcionalidad-de-ia)
4. [Problemas de Pods y Clúster](#4-problemas-de-pods-y-clúster)
5. [Problemas de Red e Ingress](#5-problemas-de-red-e-ingress)
6. [Problemas de Datos y Almacenamiento](#6-problemas-de-datos-y-almacenamiento)
7. [Problemas de Rendimiento](#7-problemas-de-rendimiento)
8. [Problemas de Monitorización y Alertas](#8-problemas-de-monitorización-y-alertas)

---

## 1. Problemas de Instalación e Inicio

### 1.1 Pod en Estado Pending Permanente

**Síntoma:** `kubectl get pods -n k8ops-system` muestra Pending

**Pasos de diagnóstico:**
```bash
# Ver la causa de Pending
kubectl describe pod -n k8ops-system -l app.kubernetes.io/name=k8ops

# Causas comunes:
# - PVC no enlazado (verificar StorageClass)
# - Recursos insuficientes (verificar capacidad del nodo)
# - Node Selector sin coincidencia
```

**Soluciones:**
- **PVC no enlazado:** Verificar si el clúster tiene un StorageClass predeterminado
  ```bash
  kubectl get storageclass
  # Si no hay SC predeterminado, marcar uno:
  kubectl patch storageclass local-path -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
  ```
- **Recursos insuficientes:** Usar el modo DaemonSet (sin dependencia de PVC)
  ```bash
  kubectl apply -k config/daemonset
  ```

### 1.2 Pod CrashLoopBackOff

**Síntoma:** El Pod se reinicia repetidamente

**Pasos de diagnóstico:**
```bash
# Ver logs del contenedor
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --tail=50

# Ver eventos
kubectl get events -n k8ops-system --sort-by='.lastTimestamp' | tail -20
```

**Causas Comunes y Soluciones:**

| Causa | Característica en Logs | Solución |
|------|----------|----------|
| Problema de permisos SQLite | `unable to open database file` | `mkdir -p /data && chown 65532:65532 /data` |
| Falta JWT Secret | `JWT secret not configured` | Establecer variable de entorno `AUTH_JWT_SECRET` |
| Fallo de conexión a K8s API | `failed to get Kubernetes config` | Verificar ServiceAccount y RBAC |
| Conflicto de puerto | `bind: address already in use` | Modificar `--dashboard-address` |

### 1.3 Fallo al Descargar Imagen (ImagePullBackOff)

**Síntoma:** `Failed to pull image`

**Solución:**
```bash
# Verificar si la imagen es accesible
docker pull registry.iot2.win/k8ops:latest

# Si usa un repositorio privado, configurar imagePullSecrets
kubectl create secret docker-registry regcred \
  --docker-server=registry.iot2.win \
  --docker-username=<user> \
  --docker-password=<pass> \
  -n k8ops-system

# O usar el modo DaemonSet + hostPath (sin necesidad de descargar imagen externa)
```

---

## 2. Problemas de Autenticación e Inicio de Sesión

### 2.1 El Inicio de Sesión Devuelve 401 Unauthorized

**Diagnóstico:**
```bash
# Verificar configuración de auth
kubectl exec -n k8ops-system deploy/k8ops -- /manager --help | grep auth

# Ver logs relacionados con auth
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -i auth
```

**Solución:**
- Confirmar que `AUTH_JWT_SECRET` esté establecido y sea consistente
- Restablecer la contraseña del administrador:
  ```bash
  kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops admin reset-password
  ```
- Credenciales predeterminadas: `admin` / `changeme` (cambiar después del primer inicio de sesión)

### 2.2 Fallo de Inicio de Sesión OIDC

**Diagnóstico:**
- Confirmar que la URL del Proveedor OIDC es alcanzable (desde dentro del Pod)
- Verificar que la URL de redirección coincida con el dominio del Ingress
- Ver errores de callback: `kubectl logs ... | grep oidc`

---

## 3. Problemas de Funcionalidad de IA

### 3.1 Chat Sin Respuesta o Tiempo de Espera Agotado

**Síntoma:** Sin respuesta tras enviar un mensaje, o tiempo de espera agotado

**Pasos de diagnóstico:**
```bash
# Verificar configuración del Provider
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops config show

# Ver logs relacionados con IA
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -E "provider|llm|agent"

# Probar conectividad LLM
kubectl exec -n k8ops-system deploy/k8ops -- curl -s https://api.openai.com/v1/models -H "Authorization: Bearer $AIOPS_API_KEY"
```

**Causas Comunes:**

| Causa | Característica en Logs | Solución |
|------|----------|----------|
| API Key inválida | `401 Unauthorized` | Actualizar la variable de entorno `AIOPS_API_KEY` |
| Red inalcanzable | `context deadline exceeded` | Configurar la salida (egress) hacia la API del LLM |
| Modelo inexistente | `model not found` | Actualizar `--provider-model` |
| Límite de tasa | `429 Too Many Requests` | Esperar o cambiar de Provider |
| Circuit Breaker abierto | `circuit breaker open` | Esperar el periodo de enfriamiento de 60s |

### 3.2 El Diagnóstico de IA No Se Activa

**Diagnóstico:**
```bash
# Verificar el estado del recolector de eventos
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -i "collector\|event"

# Confirmar que no esté deshabilitado
kubectl get deploy k8ops -n k8ops-system -o jsonpath='{.spec.template.spec.containers[0].command}'
# No debe contener --disable-event-collector
```

---

## 4. Problemas de Pods y Clúster

### 4.1 El Dashboard Muestra "kubernetes client not available"

**Síntoma:** La API devuelve 503, la UI muestra error de conexión

**Causa:** Permisos insuficientes del ServiceAccount de K8s dentro del Pod o fallo al cargar la configuración

**Solución:**
```bash
# Re-aplicar RBAC
kubectl apply -k config/rbac

# Verificar ServiceAccount
kubectl auth can-i list pods --as=system:serviceaccount:k8ops-system:k8ops -n k8ops-system
```

### 4.2 Las Operaciones (Scale/Delete/Restart) Devuelven 403 Forbidden

**Causa:** Permisos RBAC insuficientes del rol de usuario

**Solución:**
```bash
# Verificar el rol del usuario
kubectl get rolebindings -n k8ops-system | grep <username>

# Actualizar al rol admin
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops admin set-role <username> admin
```

---

## 5. Problemas de Red e Ingress

### 5.1 Dashboard Inaccesible (502/503)

**Diagnóstico:**
```bash
# Verificar si el Service tiene Endpoints
kubectl get endpoints -n k8ops-system

# Verificar configuración de Ingress
kubectl get ingress -n k8ops-system
kubectl describe ingress -n k8ops-system

# Acceder directamente al puerto del Pod
kubectl port-forward -n k8ops-system deploy/k8ops 9090:9090
# Luego acceder a http://localhost:9090
```

### 5.2 Traefik Tiempo de Espera Agotado (499/504)

**Síntoma:** Tiempo de espera agotado en push al registry o carga de archivos grandes

**Solución (específica de Traefik):**
```bash
# Desactivar los límites de tiempo de espera de Traefik
kubectl patch deployment -n kube-system traefik \
  --type='json' \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.readtimeout=0s"}]'

# O establecer timeout en IngressRoute
```

### 5.3 SSE (Server-Sent Events) No Funciona

**Síntoma:** La interfaz de chat no tiene respuesta en tiempo real

**Diagnóstico:**
- Verificar si el proxy inverso soporta conexiones persistentes (long-lived)
- La configuración de Nginx requiere: `proxy_buffering off; proxy_cache off;`
- Traefik no requiere configuración adicional

---

## 6. Problemas de Datos y Almacenamiento

### 6.1 Base de Datos SQLite Corrupta

**Síntoma:** `database disk image is malformed`

**Solución:**
```bash
# Entrar al Pod
kubectl exec -it -n k8ops-system deploy/k8ops -- sh

# Reparar la base de datos (si distroless sin shell, usar la herramienta CLI)
# Solución 1: respaldo y reconstrucción
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops db backup /data/k8ops-backup.db
kubectl exec -n k8ops-system deploy/k8ops -- rm /data/k8ops.db
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops db restore /data/k8ops-backup.db

# Solución 2: eliminar PVC y reconstruir (se perderán datos de usuario)
kubectl delete pvc -n k8ops-system k8ops-data
kubectl delete pod -n k8ops-system -l app.kubernetes.io/name=k8ops
```

### 6.2 Espacio Insuficiente en PVC

**Diagnóstico:**
```bash
kubectl exec -n k8ops-system deploy/k8ops -- df -h /data
# O ver a través de Dashboard → página Capacity
```

**Solución:**
- Expandir PVC:
  ```bash
  kubectl patch pvc -n k8ops-system k8ops-data -p '{"spec":{"resources":{"requests":{"storage":"5Gi"}}}}'
  ```
- Limpiar logs de auditoría antiguos:
  ```bash
  kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops audit cleanup --max-age 30d
  ```

---

## 7. Problemas de Rendimiento

### 7.1 Respuesta Lenta de la API

**Diagnóstico:**
```bash
# Verificar tiempo de respuesta (cabecera X-Response-Time)
curl -sk -o /dev/null -w '%{http_code} %{time_total}s\n' \
  -D - https://k8ops.iot2.win/api/cluster/overview 2>&1 | grep -i "x-response-time"

# Ver métricas de Prometheus
curl -sk https://k8ops.iot2.win/metrics | grep k8ops_http_request_duration
```

**Soluciones de optimización:**
- La caché de API ya está habilitada (overview: 30s, resources: 60s, CRDs: 10min)
- Verificar si `k8ops_http_requests_in_flight` es demasiado alto
- Los logs de solicitudes lentas (>500ms) se registran automáticamente en los logs del Pod

### 7.2 Uso Elevado de Memoria

**Diagnóstico:**
```bash
kubectl top pods -n k8ops-system
```

**Optimización:**
- Gestión automática de memoria de conversaciones: resumen automático tras umbral de 20k tokens
- Las conversaciones inactivas se limpian después de 30min
- Si el alto consumo de memoria persiste, considere reiniciar el Pod (el modo DaemonSet se reinicia automáticamente)

---

## 8. Problemas de Monitorización y Alertas

### 8.1 Prometheus No Puede Recopilar Métricas

**Diagnóstico:**
```bash
# Confirmar que el endpoint de métricas funciona
kubectl exec -it <prometheus-pod> -n monitoring -- curl -s http://k8ops.k8ops-system.svc:8080/metrics | head -5

# Verificar ServiceMonitor
kubectl get servicemonitor -n k8ops-system
```

**Nota:** El endpoint `/metrics` solo permite acceso desde localhost. Prometheus necesita recopilar desde dentro del clúster (mismo Pod o Service).

### 8.2 Las Reglas de Alerta No Funcionan

**Diagnóstico:**
```bash
# Verificar PrometheusRule
kubectl get prometheusrule -n k8ops-system

# Aplicar reglas de alerta
kubectl apply -f config/alerting-rules.yaml
```

---

## Apéndice: Comandos de Diagnóstico Comunes

```bash
# Verificación de estado con un solo comando
kubectl get pods -n k8ops-system
kubectl get events -n k8ops-system --sort-by='.lastTimestamp' | tail -20
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --tail=50

# Comprobación de salud
curl -sk https://k8ops.iot2.win/api/health
curl -sk https://k8ops.iot2.win/api/version

# Escaneo de seguridad
curl -sk https://k8ops.iot2.win/api/security/audit | jq .summary
curl -sk https://k8ops.iot2.win/api/security/compliance | jq .score

# Planificación de capacidad
curl -sk https://k8ops.iot2.win/api/capacity/planning | jq .summary
```

## Apéndice: Niveles de Log

k8ops utiliza logs estructurados JSON (slog) y soporta los siguientes niveles:

| Nivel | Uso | Ejemplo |
|------|------|------|
| `INFO` | Operación normal | Inicio del servidor, autenticación exitosa |
| `WARN` | Solicitudes lentas, problemas de configuración | Solicitudes >500ms, PVC casi lleno |
| `ERROR` | Fallo de operaciones | Error de K8s API, fallo de llamada LLM |

Correlación de logs mediante Request ID:
```bash
# Obtener el Request ID (de la cabecera HTTP X-Request-ID)
# Luego buscar en los logs
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep "a1b2c3d4"
```

---

*Última actualización: 2026-07-03*
*Mantenedores: Equipo de k8ops*
