# Guía de Ejecución Local de k8ops

> No requiere despliegue en un clúster de Kubernetes; ejecute el binario de k8ops directamente en su portátil/estación de trabajo.

---

## Casos de Uso

- **Desarrollo y depuración local** — iteración rápida de código, sin necesidad de construir imágenes cada vez
- **Herramienta de gestión sin conexión** — como alternativa inteligente a kubectl
- **Demostraciones y pruebas** — experimente todas las funciones sin necesidad de despliegue en el clúster
- **Integración CI/CD** — ejecutar como herramienta de diagnóstico en pipelines

---

## Requisitos Previos

- Go 1.26+ (o descargar directamente el binario precompilado)
- kubectl configurado y capaz de conectar al clúster
- API Key de LLM (OpenAI / DeepSeek / ZAI, etc.)

---

## Método 1: Compilar desde el Código Fuente

```bash
cd k8ops

# Compilar manager (servidor del dashboard)
go build -o k8ops-manager ./cmd/manager/

# Compilar herramienta CLI
go build -o k8ops ./cmd/k8ops/
```

## Método 2: Descargar Binario Precompilado

Descargue el binario para su plataforma desde [GitHub Releases](https://github.com/topcheer/k8ops/releases).

---

## Iniciar el Dashboard

```bash
AIOPS_API_KEY=your-api-key \
  ./k8ops-manager \
  --leader-elect=false \
  --dashboard-address=:9090 \
  --auth-db-path=/tmp/k8ops.db
```

Después de iniciar, acceda a `http://localhost:9090`, inicio de sesión predeterminado `admin / admin`.

### Descripción de Parámetros

| Parámetro | Valor Predeterminado | Descripción |
|------|--------|------|
| `--dashboard-address` | `:9090` | Dirección de escucha del Dashboard |
| `--leader-elect` | `false` | Leader Election (desactivar para ejecución de instancia única) |
| `--metrics-bind-address` | `:8080` | Puerto de métricas de Prometheus |
| `--health-probe-bind-address` | `:8081` | Puerto de comprobación de salud |
| `--auth-db-path` | `/data/k8ops.db` | Ruta de la base de datos SQLite |
| `--auth-jwt-secret` | (generado aleatoriamente) | Clave de firma JWT |
| `--provider-type` | `openai` | Proveedor LLM |
| `--provider-model` | `gpt-4o` | Nombre del modelo |
| `--provider-api-key` | (obligatorio) | LLM API Key |
| `--provider-endpoint` | (predeterminado) | Endpoint de API personalizado |

### Variables de Entorno

Todos los parámetros también se pueden configurar mediante variables de entorno:

```bash
export AIOPS_API_KEY=sk-your-key
export PROVIDER_TYPE=deepseek
export PROVIDER_MODEL=deepseek-chat
export AUTH_DB_PATH=$HOME/.k8ops/k8ops.db
export AUTH_JWT_SECRET=your-secret

./k8ops-manager --leader-elect=false
```

---

## Mecanismo de Descubrimiento de kubeconfig

k8ops utiliza `ctrl.GetConfigOrDie()` de controller-runtime para descubrir automáticamente kubeconfig, en este orden:

1. Variable de entorno `KUBECONFIG`
2. `~/.kube/config` (ruta predeterminada)
3. Configuración dentro del clúster (`/var/run/secrets/kubernetes.io/serviceaccount/`)

En ejecución local, se usa automáticamente `~/.kube/config` sin necesidad de configuración adicional.

### Especificar Clúster

```bash
KUBECONFIG=~/.kube/prod-config ./k8ops-manager --leader-elect=false
```

### Cambio Multi-clúster

```bash
# Cambiar con kubectx
kubectx prod-cluster
./k8ops-manager --leader-elect=false
```

---

## Diferencias en el Flujo de Datos

### Ejecución en Clúster vs Ejecución Local

| Dimensión | En Clúster (DaemonSet/Deployment) | Ejecución Local |
|------|------|------|
| Autenticación K8s API | Token de ServiceAccount | kubeconfig |
| Herramientas de Host | `nsenter` para acceder al host | Ejecución directa en la máquina local |
| Datos de Auth | Persistencia con PVC | Archivo SQLite local |
| Leader Election | Necesario para multi-réplica | Desactivado para instancia única |
| Suplantación RBAC | Usuario → ServiceAccount | Usuario → usuario de kubeconfig |
| Permisos de Red | Red del Pod | Red de la máquina local |
| Salida de Logs | stdout → kubectl logs | Salida directa a terminal |

### Comportamiento de Herramientas de Host

En un contenedor, las herramientas de Host acceden al namespace del host mediante `nsenter -m -u -i -n -p --`. En ejecución local se ejecutan directamente mediante `/bin/sh -c`, accediendo al sistema operativo local.

Esto significa que:
- `host_disk_check` verifica el disco local
- `host_process_list` lista los procesos locales
- `host_exec` ejecuta comandos localmente

---

## Uso de la Herramienta CLI

```bash
# Diagnóstico
./k8ops diagnose --problem "pods in production keep CrashLoopBackOff"

# Ver sugerencias de optimización
./k8ops optimize --namespace production

# Desencadenar remediación
./k8ops remediate --plan <plan-name> --approve
```

---

## Ejecución Permanente en Segundo Plano

### macOS (launchd)

```bash
cat > ~/Library/LaunchAgents/dev.ggai.k8ops.plist << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>dev.ggai.k8ops</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/k8ops-manager</string>
        <string>--leader-elect=false</string>
        <string>--dashboard-address=:9090</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>AIOPS_API_KEY</key>
        <string>your-api-key</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
EOF

launchctl load ~/Library/LaunchAgents/dev.ggai.k8ops.plist
```

### Linux (systemd)

```bash
sudo tee /etc/systemd/system/k8ops.service << 'EOF'
[Unit]
Description=k8ops AI Operations
After=network.target

[Service]
ExecStart=/usr/local/bin/k8ops-manager --leader-elect=false --dashboard-address=:9090
Environment=AIOPS_API_KEY=your-api-key
Environment=AUTH_DB_PATH=/var/lib/k8ops/k8ops.db
Restart=always
User=k8ops

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl enable --now k8ops
```

---

## Modo de Desarrollo

### Recarga en Caliente

```bash
# Instalar air
go install github.com/air-verse/air@latest

# En el directorio raíz del proyecto k8ops
air --build.cmd "go build ./cmd/manager/" --build.bin "./manager"
```

### Depuración

```bash
# Habilitar logs DEBUG
DEBUG=true ./k8ops-manager --leader-elect=false

# Ver logs estructurados JSON
tail -f /tmp/k8ops.log
```

---

## Solución de Problemas

### "unable to get kubeconfig"

Asegúrese de que `~/.kube/config` exista y sea válido:
```bash
kubectl cluster-info  # probar kubeconfig
```

### "address already in use :9090"

```bash
# Ver qué proceso ocupa el puerto 9090
lsof -i :9090
# O cambiar de puerto
./k8ops-manager --dashboard-address=:9091
```

### Bloqueo de Auth DB

Elimine el archivo DB y reinicialice:
```bash
rm /tmp/k8ops.db
./k8ops-manager --auth-db-path=/tmp/k8ops.db
```

### Tiempo de Espera Agotado del Proveedor

Establezca un tiempo de espera más largo o verifique la red:
```bash
export PROVIDER_ENDPOINT=https://api.openai.com/v1
# Confirmar que la red es accesible
curl -I https://api.openai.com/v1/models
```
