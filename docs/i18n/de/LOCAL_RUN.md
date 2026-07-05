# k8ops Lokale Ausführung

> Keine Kubernetes-Cluster-Bereitstellung erforderlich — k8ops-Binärdatei direkt auf Laptop/Workstation ausführen.

---

## Anwendungsfälle

- **Lokale Entwicklung und Debugging** — schnelle Code-Iteration, ohne jedes Mal ein Image zu erstellen
- **Offline-Verwaltungswerkzeug** — als intelligente kubectl-Alternative
- **Demo und Test** — alle Funktionen ohne In-Cluster-Bereitstellung erleben
- **CI/CD-Integration** — als Diagnosewerkzeug in der Pipeline ausführen

---

## Voraussetzungen

- Go 1.26+ (oder vorcompilierte Binärdatei direkt herunterladen)
- kubectl konfiguriert und mit Cluster verbunden
- LLM-API-Schlüssel (OpenAI / DeepSeek / ZAI etc.)

---

## Methode 1: Aus dem Quellcode kompilieren

```bash
cd k8ops

# Manager kompilieren (Dashboard-Server)
go build -o k8ops-manager ./cmd/manager/

# CLI-Werkzeug kompilieren
go build -o k8ops ./cmd/k8ops/
```

## Methode 2: Vorcompilierte Binärdatei herunterladen

Von [GitHub Releases](https://github.com/topcheer/k8ops/releases) die Binärdatei für die entsprechende Plattform herunterladen.

---

## Dashboard starten

```bash
AIOPS_API_KEY=your-api-key \
  ./k8ops-manager \
  --leader-elect=false \
  --dashboard-address=:9090 \
  --auth-db-path=/tmp/k8ops.db
```

Nach dem Start `http://localhost:9090` aufrufen, Standard-Login `admin / admin`.

### Parameterbeschreibung

| Parameter | Standardwert | Beschreibung |
|-----------|-------------|--------------|
| `--dashboard-address` | `:9090` | Dashboard-Listen-Adresse |
| `--leader-elect` | `false` | Leader Election (für Einzelinstanz deaktivieren) |
| `--metrics-bind-address` | `:8080` | Prometheus-Metriken-Port |
| `--health-probe-bind-address` | `:8081` | Gesundheitsprüfungs-Port |
| `--auth-db-path` | `/data/k8ops.db` | SQLite-Datenbankpfad |
| `--auth-jwt-secret` | (zufällig generiert) | JWT-Signierschlüssel |
| `--provider-type` | `openai` | LLM-Provider |
| `--provider-model` | `gpt-4o` | Modellname |
| `--provider-api-key` | (erforderlich) | LLM-API-Schlüssel |
| `--provider-endpoint` | (Standard) | Benutzerdefinierter API-Endpunkt |

### Umgebungsvariablen

Alle Parameter können auch über Umgebungsvariablen gesetzt werden:

```bash
export AIOPS_API_KEY=sk-your-key
export PROVIDER_TYPE=deepseek
export PROVIDER_MODEL=deepseek-chat
export AUTH_DB_PATH=$HOME/.k8ops/k8ops.db
export AUTH_JWT_SECRET=your-secret

./k8ops-manager --leader-elect=false
```

---

## kubeconfig-Erkennungsmechanismus

k8ops verwendet die `ctrl.GetConfigOrDie()`-Funktion von controller-runtime zur automatischen kubeconfig-Erkennung, Suchreihenfolge:

1. `KUBECONFIG`-Umgebungsvariable
2. `~/.kube/config` (Standardpfad)
3. In-Cluster-Config (`/var/run/secrets/kubernetes.io/serviceaccount/`)

Bei lokaler Ausführung wird automatisch `~/.kube/config` verwendet, keine zusätzliche Konfiguration erforderlich.

### Bestimmten Cluster angeben

```bash
KUBECONFIG=~/.kube/prod-config ./k8ops-manager --leader-elect=false
```

### Multi-Cluster-Wechsel

```bash
# Mit kubectx wechseln
kubectx prod-cluster
./k8ops-manager --leader-elect=false
```

---

## Datenflussunterschiede

### In-Cluster vs. Lokale Ausführung

| Dimension | In-Cluster (DaemonSet/Deployment) | Lokale Ausführung |
|-----------|-----------------------------------|-------------------|
| K8s-API-Authentifizierung | ServiceAccount-Token | kubeconfig |
| Host-Tools | `nsenter` für Host-Namespace | Direkt auf lokalem Rechner |
| Auth-Daten | PVC-persistent | Lokale SQLite-Datei |
| Leader Election | Für Multi-Replica erforderlich | Für Einzelinstanz deaktiviert |
| RBAC-Impersonation | Benutzer → ServiceAccount | Benutzer → kubeconfig-Benutzer |
| Netzwerkberechtigung | Pod-Netzwerk | Lokales Netzwerk |
| Log-Ausgabe | stdout → kubectl logs | Direkte Terminal-Ausgabe |

### Host-Tool-Verhalten

Im Container greifen Host-Tools über `nsenter -m -u -i -n -p --` auf den Host-Namespace zu. Bei lokaler Ausführung werden sie direkt über `/bin/sh -c` ausgeführt und greifen auf das lokale Betriebssystem zu.

Das bedeutet:
- `host_disk_check` prüft die lokale Festplatte
- `host_process_list` listet lokale Prozesse auf
- `host_exec` führt Befehle lokal aus

---

## CLI-Werkzeug verwenden

```bash
# Diagnose
./k8ops diagnose --problem "pods in production keep CrashLoopBackOff"

# Optimierungsvorschläge anzeigen
./k8ops optimize --namespace production

# Behebung auslösen
./k8ops remediate --plan <plan-name> --approve
```

---

## Hintergrund-Dauerbetrieb

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

## Entwicklungsmodus

### Hot-Reload

```bash
# air installieren
go install github.com/air-verse/air@latest

# Im k8ops-Projekt-Stammverzeichnis
air --build.cmd "go build ./cmd/manager/" --build.bin "./manager"
```

### Debugging

```bash
# DEBUG-Logs aktivieren
DEBUG=true ./k8ops-manager --leader-elect=false

# Strukturierte JSON-Logs anzeigen
tail -f /tmp/k8ops.log
```

---

## Fehlerbehebung

### "unable to get kubeconfig"

Stellen Sie sicher, dass `~/.kube/config` existiert und gültig ist:
```bash
kubectl cluster-info  # kubeconfig testen
```

### "address already in use :9090"

```bash
# Prozess auf Port 9090 anzeigen
lsof -i :9090
# Oder anderen Port verwenden
./k8ops-manager --dashboard-address=:9091
```

### Auth-DB gesperrt

Löschen Sie die DB-Datei und initialisieren Sie sie neu:
```bash
rm /tmp/k8ops.db
./k8ops-manager --auth-db-path=/tmp/k8ops.db
```

### Provider-Timeout

Setzen Sie ein längeres Timeout oder überprüfen Sie das Netzwerk:
```bash
export PROVIDER_ENDPOINT=https://api.openai.com/v1
# Netzwerk-Erreichbarkeit bestätigen
curl -I https://api.openai.com/v1/models
```
