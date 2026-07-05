# Seguridad de k8ops

## Autenticación

k8ops soporta tres métodos de autenticación, configurables por despliegue:

### Autenticación Local

- Usuario/contraseña almacenados en SQLite
- Contraseñas cifradas con bcrypt
- Inicialización del administrador mediante la variable de entorno `AUTH_DEFAULT_ROLE`

### LDAP/Active Directory

- URL del servidor, DN de enlace y base de búsqueda configurables
- Opción `SkipTLSVerify` (predeterminado: `false`) para certificados autofirmados
- Soporte multi-proveedor: se pueden configurar múltiples servidores LDAP simultáneamente

### OIDC (OpenID Connect)

- Compatible con cualquier IdP compatible con OIDC (Google, GitHub, Keycloak, etc.)
- **Protección CSRF**: parámetro state validado con `crypto/subtle.ConstantTimeCompare`
- **Cookie por proveedor**: `oidc_state_{provider}` evita colisiones entre múltiples proveedores
- **Flag Secure**: detectado automáticamente mediante TLS o cabecera `X-Forwarded-Proto`
- **HttpOnly + SameSite**: la cookie de estado no es accesible mediante JavaScript

## Modelo RBAC

### Roles

| Rol | Ámbito | Permisos |
|------|-------|----------|
| `admin` | cluster | Acceso completo: gestión de usuarios, proveedores y todos los namespaces |
| `operator` | cluster | Lectura total + chat + ejecución de diagnósticos |
| `viewer` | cluster | Solo lectura: visualización de dashboards e informes |
| `ns-admin` | namespace | Administrador dentro de los namespaces asignados |
| `ns-viewer` | namespace | Solo lectura dentro de los namespaces asignados |

### Ámbito de Namespace

Los usuarios con roles de ámbito namespace están restringidos a sus namespaces asignados mediante:

1. **Sincronización RBAC de K8s**: se crean recursos `RoleBinding` por namespace
2. **Suplantación de API**: las llamadas a la API del Dashboard utilizan la identidad del usuario al comunicarse con la API de K8s
3. **Filtrado de namespaces**: las respuestas de la API se filtran a los namespaces permitidos

### Protección de Roles Integrados

Los roles integrados (`admin`, `operator`, `viewer`) están marcados como `Builtin: true` y no pueden eliminarse mediante la API.

## Funciones de Seguridad

### Lista de Permitidos CORS

- Configurada mediante la variable de entorno `CORS_ALLOWED_ORIGINS` (separada por comas)
- No se permite comodín (`*`) cuando hay credenciales involucradas
- Solo mismo origen si no está configurada

### Protección CSRF de OIDC

- Parámetro state: nonce aleatorio por cada intento de autenticación
- Validado con `subtle.ConstantTimeCompare` (resistente a ataques de temporización)
- Almacenado en cookie HttpOnly con flags Secure + SameSite

### Persistencia de JWT

- Secreto de firma JWT persistido en el Secret de K8s `k8ops-auth` (clave: `jwt-secret`)
- Recurre a un secreto aleatorio efímero con un log de advertencia si el Secret no existe
- Evita la invalidación de sesiones al reiniciar el pod

### Registro de Auditoría

Todas las operaciones sensibles se registran:

- Inicio/cierre de sesión de usuario
- Cambios en la configuración del proveedor
- Ejecución de diagnósticos
- Acciones de remediación
- Cambios de rol de usuario

### Limitación de Tasa

- `resilience.RateLimiter` disponible (aún no conectado a la capa HTTP — trabajo futuro)

### Cierre Elegante

- `SIGTERM`/`SIGINT` → drenar conexiones SSE → vaciar WAL de SQLite → detener el manager
- Previene la corrupción de datos durante el desalojo del pod

## Configuración de Seguridad

### Variables de Entorno

| Variable | Predeterminado | Descripción |
|----------|---------|-------------|
| `AUTH_DB_DRIVER` | `sqlite` | Driver de base de datos |
| `AUTH_DB_DSN` | — | Cadena de conexión a la base de datos |
| `AUTH_DB_PATH` | `/data/k8ops.db` | Ruta de la base de datos SQLite |
| `AUTH_JWT_SECRET` | (aleatorio) | Secreto de firma JWT (persistir mediante Secret de K8s) |
| `AUTH_DEFAULT_ROLE` | `viewer` | Rol para nuevos usuarios |
| `CORS_ALLOWED_ORIGINS` | — | Orígenes permitidos separados por comas |
| `AIOPS_API_KEY` | — | API key del proveedor LLM |

### Gestión de Secrets de K8s

```yaml
# K8s Secret para persistencia de JWT
apiVersion: v1
kind: Secret
metadata:
  name: k8ops-auth
  namespace: k8ops-system
type: Opaque
stringData:
  jwt-secret: "<openssl rand -base64 32>"
```

El despliegue lo lee mediante:
```yaml
env:
- name: AUTH_JWT_SECRET
  valueFrom:
    secretKeyRef:
      name: k8ops-auth
      key: jwt-secret
      optional: true  # recurre a aleatorio si no existe
```

### Configuración TLS de LDAP

Los proveedores LDAP soportan `skip_tls_verify` (predeterminado: `false`):

```json
{
  "ldap": {
    "server": "ldaps://ldap.corp.com",
    "skip_tls_verify": false
  }
}
```

Solo establezca `skip_tls_verify: true` para desarrollo con certificados autofirmados.

## Limitaciones Conocidas

1. **Sin limitación de tasa en el inicio de sesión** — `resilience.RateLimiter` existe pero no está conectado a los manejadores HTTP
2. **Sin terminación HTTPS** — k8ops sirve HTTP; el TLS debe ser gestionado por el controlador de ingress
3. **SQLite de nodo único** — sin base de datos HA; adecuado para despliegues de réplica única
4. **Sin revocación de sesiones** — los tokens JWT son válidos hasta su expiración (24h); no hay lista de revocación del lado del servidor

## Reportes de Seguridad

Para reportar una vulnerabilidad de seguridad:

1. **NO abra un issue público en GitHub**
2. Envíe un correo a security@ggai.dev con los detalles y los pasos de reproducción
3. Reconoceremos dentro de las 48 horas y proporcionaremos un cronograma de corrección
4. Se agradece la divulgación responsable

## Mejoras de Seguridad Futuras

- [ ] Conectar la limitación de tasa a la API de inicio de sesión
- [ ] Añadir revocación de sesiones (lista de denegación)
- [ ] Soportar proveedores OAuth externos para RBAC
- [ ] Añadir mTLS para comunicación entre servicios
- [ ] Implementar cifrado de secretos en reposo (más allá de PVC)
