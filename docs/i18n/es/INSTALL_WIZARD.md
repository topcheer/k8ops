# Guía del Asistente de Instalación de k8ops

El asistente de instalación interactivo (`wizard.sh`) le guía en la configuración de
todos los componentes principales de k8ops antes del despliegue: motor de base de datos,
integración SSO y proveedor de IA.

## Inicio Rápido

### Modo Interactivo

```bash
git clone https://github.com/topcheer/k8ops.git
cd k8ops
./wizard.sh
```

### Modo No Interactivo

```bash
# Editar config/wizard-values.yaml con su configuración, luego:
./wizard.sh --values config/wizard-values.yaml
```

### Simulación (Solo Generar Manifiestos)

```bash
./wizard.sh --dry-run
# Revisar los archivos generados: .wizard-*.yaml
# Desplegar manualmente con kubectl apply -f ...
```

## Pasos del Asistente

### Paso 1: Modo de Despliegue

| Modo | Descripción | Ideal Para |
|------|-------------|----------|
| **DaemonSet** | Se ejecuta en cada nodo | Clústeres K3s/bare-metal, monitorización a nivel de nodo |
| **Deployment** | Réplica única con PVC | K8s gestionado (EKS/GKE/AKS), configuraciones sensibles a costos |

### Paso 2: Motor de Base de Datos

k8ops utiliza una base de datos para cuentas de usuario, roles y proveedores de autenticación.

| Motor | Caso de Uso | HA | Configuración |
|---------|----------|----|-------|
| **SQLite** | Clústeres pequeños, nodo único | No | Sin configuración (integrado) |
| **MySQL** | Multi-réplica, auth compartida | Sí | StatefulSet interno o conexión externa |
| **PostgreSQL** | Multi-réplica, auth compartida | Sí | StatefulSet interno o conexión externa |

#### Base de Datos Interna vs Externa

- **Interna**: El asistente despliega un StatefulSet de MySQL/PostgreSQL en el namespace
  `k8ops-system` con un PVC. Totalmente gestionado, sin dependencias externas.
- **Externa**: Se conecta a su base de datos existente. Usted proporciona la cadena de conexión DSN.

#### Formatos DSN

**MySQL:**
```
k8ops:password@tcp(mysql-host:3306)/k8ops?charset=utf8mb4&parseTime=True
```

**PostgreSQL:**
```
host=postgres-host user=k8ops password=secret dbname=k8ops sslmode=disable
```

### Paso 3: SSO / Proveedor de Identidad

k8ops soporta múltiples proveedores SSO con preajustes integrados:

| Proveedor | Tipo | Preajuste |
|----------|------|--------|
| **GitHub** | OIDC | Emisor preconfigurado |
| **Google** | OIDC | Emisor preconfigurado |
| **Microsoft** (Entra ID) | OIDC | Emisor preconfigurado |
| **GitLab** | OIDC | Emisor preconfigurado |
| **Keycloak** | OIDC | Emisor personalizado (su realm) |
| **Okta** | OIDC | Emisor personalizado |
| **Auth0** | OIDC | Emisor personalizado |
| **LDAP / AD** | LDAP | Servidor + bind DN |
| **OIDC Personalizado** | OIDC | URL de emisor manual |

#### URL de Redirección OIDC

Al registrar su aplicación con el proveedor de identidad, utilice esta URL de redirección:

```
https://<su-host-dashboard>/api/auth/oidc/<nombre-proveedor>/callback
```

Ejemplo para GitHub:
```
https://k8ops.example.com/api/auth/oidc/github/callback
```

#### Configuración LDAP

Proporcione:
- **URL del servidor**: `ldap://host:389` o `ldaps://host:636`
- **Base de búsqueda**: p. ej. `ou=users,dc=example,dc=com`
- **Bind DN**: DN de la cuenta de servicio, p. ej. `cn=admin,dc=example,dc=com`
- **Contraseña de enlace**: Contraseña de la cuenta de servicio

El SSO se puede omitir durante la instalación y configurarse posteriormente mediante **Settings > Auth Providers** en el dashboard.

### Paso 4: Proveedor de IA

| Proveedor | Modelos | Notas |
|----------|--------|-------|
| **OpenAI** | gpt-4o, gpt-4o-mini | Predeterminado |
| **Anthropic** | claude-sonnet-4-20250514 | Familia Claude |
| **Gemini** | gemini-1.5-flash | Google AI |
| **Personalizado** | Cualquiera | Endpoint compatible con OpenAI |

El proveedor de IA se puede aplazar hasta después de la instalación mediante **Settings** en el dashboard.

### Paso 5: Confirmar y Desplegar

El asistente muestra un resumen de todas las selecciones. Tras la confirmación:

1. Genera los manifiestos de Kubernetes (secrets, StatefulSet de base de datos opcional)
2. Los aplica al clúster
3. Despliega k8ops (DaemonSet o Deployment)
4. Espera a que los pods estén listos
5. Muestra la URL de acceso y las credenciales de inicio de sesión

## Posterior a la Instalación

### Inicio de Sesión Predeterminado

- Usuario: `admin`
- Contraseña: `admin`
- **Cambiar inmediatamente después del primer inicio de sesión**

### Configurar SSO Después de la Instalación

Si omitió SSO durante la instalación:

1. Navegue a **Settings > Auth Providers**
2. Haga clic en **Add Provider**
3. Seleccione un preajuste (GitHub, Google, etc.)
4. Ingrese Client ID y Client Secret
5. Guarde y habilite

### Referencia de Variables de Entorno

El asistente establece estas variables de entorno (también se pueden configurar manualmente):

| Variable | Descripción | Predeterminado |
|----------|-------------|---------|
| `AUTH_DB_DRIVER` | Driver de base de datos | `sqlite` |
| `AUTH_DB_DSN` | Cadena de conexión a la base de datos | (vacío) |
| `AUTH_DB_PATH` | Ruta del archivo SQLite | `/data/k8ops.db` |
| `AUTH_JWT_SECRET` | Secreto de firma JWT | (auto-generado) |
| `AUTH_DEFAULT_ROLE` | Rol predeterminado para usuarios SSO | `viewer` |
| `AIOPS_API_KEY` | API key del proveedor de IA | (vacío) |

## Flags de CLI

```bash
./manager \
  --auth-db-driver=postgres \
  --auth-db-dsn="host=localhost user=k8ops password=secret dbname=k8ops sslmode=disable" \
  --auth-jwt-secret=my-secret \
  --provider-type=openai \
  --provider-model=gpt-4o \
  --provider-api-key=sk-... \
  --dashboard-address=:9090
```

## Solución de Problemas

### Error "out of memory" de SQLite

Esto ocurre cuando la ruta de la base de datos SQLite no es escribible (p. ej. sistema de archivos de contenedor de solo lectura).
Asegúrese de que `/data` esté respaldado por un volumen `emptyDir` o PVC.

### Falló la conexión MySQL/PostgreSQL

1. Verifique que el formato DSN coincida con el tipo de su base de datos
2. Compruebe la conectividad de red desde los pods de k8ops a la base de datos
3. Asegúrese de que el usuario de la base de datos tenga permisos CREATE/ALTER (para auto-migración)

### La redirección SSO no funciona

1. Verifique que la URL de redirección coincida exactamente (incluyendo la barra diagonal final)
2. Compruebe que HTTPS esté configurado correctamente (OIDC requiere HTTPS)
3. Asegúrese de que el proveedor de identidad tenga la URL de redirección correcta registrada
