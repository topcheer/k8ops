# k8ops Bereitstellungsanleitung

## Ein-Klick-Installation und Deinstallation

### Voraussetzungen

- Kubernetes 1.24+ (k3s / k8s / EKS / GKE / AKS jeweils geeignet)
- kubectl konfiguriert und mit Cluster verbunden
- Lokale oder entfernte Container-Image-Registry (standardmäßig `registry.iot2.win`)
- Optional: LLM-API-Schlüssel (OpenAI / DeepSeek / ZAI oder kompatible Schnittstellen)

---

## Methode 1: Deployment-Modus (empfohlen)

Single-Replica-Deployment, geeignet für die meisten Szenarien. Enthält Ingress, Service, ConfigMap, Secret, RBAC — alles in einem Befehl bereitgestellt.

### Installation

```bash
# Lokales Netzwerk (enthält bereits alle Konfigurationen wie Domäne, Image, CORS)
kubectl apply -k config/deploy/overlays/local

# Oder benutzerdefiniertes Overlay
cp -r config/deploy/overlays/local config/deploy/overlays/myorg
# Bearbeiten Sie myorg/kustomization.yaml: Image-Adresse, Domäne, CORS etc. ersetzen
kubectl apply -k config/deploy/overlays/myorg
```

### Verifizierung

```bash
# Pod-Status überprüfen
kubectl get pods -n k8ops-system

# Ingress überprüfen
kubectl get ingress -n k8ops-system

# Dashboard aufrufen
# Browser öffnen: https://<ihre-domäne>  (z.B. https://k8ops.iot2.win)
# Standard-Login: admin / admin (bei erster Anmeldung wird Passwortänderung verlangt)
```

### Deinstallation

```bash
kubectl delete -k config/deploy/overlays/local
```

---

## Methode 2: DaemonSet-Modus

Ein Pod pro Knoten, unterstützt knotenbezogene Diagnose (hostPID, hostPath). Geeignet für Szenarien mit tiefgehender Knotenüberwachung.

### Installation

```bash
kubectl apply -f config/daemonset-local.yaml
```

### Verifizierung

```bash
# DaemonSet überprüfen (ein Pod pro Knoten)
kubectl get ds -n k8ops-system
kubectl get pods -n k8ops-system -o wide

# Dashboard aufrufen (über Service ClusterIP oder Ingress)
kubectl get svc k8ops-dashboard -n k8ops-system
```

### Deinstallation

```bash
kubectl delete -f config/daemonset-local.yaml
```

---

## Methode 3: install.sh-Skript

```bash
# Installation (automatische Umgebungserkennung, interaktive Auswahl Deployment / DaemonSet)
./install.sh install

# Deinstallation
./install.sh uninstall

# Status anzeigen
./install.sh status
```

---

## Image-Erstellung und -Push

```bash
# Lokaler Build (amd64, für Cluster-Knoten geeignet)
docker buildx build --platform linux/amd64 \
  --build-arg VERSION=v1.0.0 \
  -t registry.iot2.win/k8ops:v1.0.0 \
  -t registry.iot2.win/k8ops:latest \
  --load .

# In Registry pushen
docker push registry.iot2.win/k8ops:v1.0.0
docker push registry.iot2.win/k8ops:latest
```

### Multi-Architektur-Build

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  --build-arg VERSION=v1.0.0 \
  -t registry.iot2.win/k8ops:v1.0.0 \
  --push .
```

---

## LLM-Provider-Konfiguration

### Methode 1: Dashboard-Oberfläche (empfohlen)

1. Dashboard-Anmeldung → **Settings**-Registerkarte
2. Provider-Typ, API-Schlüssel, Endpunkt, Modell ausfüllen
3. **Save** klicken, automatisch in K8s ConfigMap/Secret persistent gespeichert

### Methode 2: Umgebungsvariablen

In der ConfigMap des Overlays festlegen:

```yaml
configMapGenerator:
- name: k8ops-config
  literals:
  - PROVIDER_TYPE=openai          # openai / deepseek / zai / anthropic
  - PROVIDER_MODEL=gpt-4o
  - PROVIDER_ENDPOINT=https://api.openai.com/v1
```

API-Schlüssel über Secret:

```yaml
secretGenerator:
- name: k8ops-credentials
  literals:
  - api-key=sk-your-api-key-here
```

### Unterstützte Provider

| Provider | Endpunkt | Beispiel-Modell |
|----------|----------|-----------------|
| OpenAI | `https://api.openai.com/v1` | gpt-4o, gpt-4o-mini |
| DeepSeek | `https://api.deepseek.com/v1` | deepseek-chat |
| ZAI | `https://open.bigmodel.cn/api/paas/v4` | glm-4-flash, glm-4-plus |
| Anthropic | `https://api.anthropic.com/v1` | claude-3-5-sonnet |
| Lokal | `http://localhost:11434/v1` | llama3, qwen2 |

---

## Authentifizierungskonfiguration

### Lokale Authentifizierung (Standard)

Out-of-the-box, Benutzer in SQLite gespeichert. Erste Anmeldung: `admin / admin`.

### LDAP

```yaml
# In ConfigMap oder Provider-Konfiguration festlegen
LDAP_SERVER=ldap://your-ldap:389
LDAP_BIND_DN=cn=admin,dc=example,dc=com
LDAP_BIND_PASSWORD=secret
LDAP_USER_BASE=ou=users,dc=example,dc=com
LDAP_SKIP_TLS_VERIFY=false   # In Produktion zwingend false
```

### OIDC (GitHub / Google / Keycloak etc.)

```yaml
# Provider-Konfiguration (Dashboard Settings-Seite oder CRD)
OIDC_ISSUER=https://your-keycloak/realms/myrealm
OIDC_CLIENT_ID=k8ops
OIDC_CLIENT_SECRET=your-secret
OIDC_REDIRECT_URL=https://k8ops.iot2.win/auth/oidc/callback
```

---

## Ingress und TLS

### Automatisches TLS (cert-manager + Let's Encrypt)

Stellen Sie sicher, dass cert-manager im Cluster installiert ist, und fügen Sie in Ingress folgende Annotation hinzu:

```yaml
annotations:
  cert-manager.io/cluster-issuer: letsencrypt-prod
```

### Vorhandenes TLS-Zertifikat verwenden

```bash
kubectl create secret tls k8ops-dashboard-tls \
  --cert=fullchain.pem \
  --key=privkey.pem \
  -n k8ops-system
```

---

## Häufige Probleme

### Pod bleibt dauerhaft im Pending-Zustand

```bash
# Grund für fehlgeschlagene Schedulierung überprüfen
kubectl describe pod <pod-name> -n k8ops-system | tail -10

# Häufige Ursachen:
# - hostNetwork-Portkonflikt → hostNetwork: true entfernen oder Portkonflikte vermeiden
# - Ressourcenmangel → resources.requests/limits anpassen
# - Knoten-Taints → tolerations überprüfen
```

### Dashboard gibt 502 zurück

```bash
# 1. Überprüfen, ob Pod Ready ist
kubectl get pods -n k8ops-system

# 2. Service-Endpoints überprüfen
kubectl get endpoints k8ops-dashboard -n k8ops-system

# 3. Ingress-Backend überprüfen
kubectl describe ingress -n k8ops-system

# 4. Nach vollständiger Pod-Bereitschaft erneut versuchen
```

### Image-Pull fehlgeschlagen

```bash
# Lösung 1: imagePullPolicy: Always festlegen (bei spezifischem Tag empfohlen)
# Lösung 2: Sicherstellen, dass Knoten TLS-Vertrauen zur Registry konfiguriert haben
# Lösung 3: Bei privater Registry imagePullSecrets erstellen
```

### LLM-API 401

```bash
# Überprüfen, ob API-Schlüssel korrekt konfiguriert ist
kubectl get secret k8ops-credentials -n k8ops-system -o jsonpath='{.data.api-key}' | base64 -d

# Oder im Dashboard → Settings Provider neu konfigurieren
```

---

## Upgrade

```bash
# Neues Image erstellen und pushen
docker buildx build --platform linux/amd64 \
  --build-arg VERSION=v2.0.0 \
  -t registry.iot2.win/k8ops:v2.0.0 \
  -t registry.iot2.win/k8ops:latest \
  --push .

# Rolling Update (Deployment-Modus)
kubectl set image deployment/k8ops \
  k8ops=registry.iot2.win/k8ops:v2.0.0 \
  -n k8ops-system

# Oder newTag im Overlay ändern und erneut apply
kubectl apply -k config/deploy/overlays/local

# DaemonSet-Modus
kubectl set image daemonset/k8ops \
  k8ops=registry.iot2.win/k8ops:v2.0.0 \
  -n k8ops-system
```

---

## Datenbanksicherung und -wiederherstellung

### Automatische SQLite-Sicherung (CronJob)

k8ops verwendet SQLite für Benutzer, Audit-Logs und Sitzungsdaten. Stündliche automatische Sicherung empfohlen:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: k8ops-backup
  namespace: k8ops-system
spec:
  schedule: "0 * * * *"  # Jede Stunde zur vollen Stunde
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
              # Letzte 24 Sicherungen behalten
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

### Manuelle Sicherung

```bash
# Datenbank aus Pod kopieren
kubectl cp k8ops-system/<pod-name>:/data/k8ops.db ./k8ops-backup-$(date +%Y%m%d).db

# Oder Online-Sicherung mit sqlite3 (ohne Schreibunterbrechung)
kubectl exec -n k8ops-system <pod-name> -- sqlite3 /data/k8ops.db ".backup /data/k8ops-backup.db"
kubectl cp k8ops-system/<pod-name>:/data/k8ops-backup.db ./k8ops-backup.db
```

### Wiederherstellung

```bash
# k8ops stoppen
kubectl scale deployment k8ops -n k8ops-system --replicas=0

# Datenbank wiederherstellen
kubectl cp ./k8ops-backup.db k8ops-system/<pod-name>:/data/k8ops.db

# Neustart
kubectl scale deployment k8ops -n k8ops-system --replicas=1
```

---

## Hochverfügbarkeit (HA)-Bereitstellung

### Einzelknoten-Modus (Standard, für Entwicklung/kleine Cluster)

- 1 Replika + SQLite + PVC
- Kurzzeitige Service-Unterbrechung bei Pod-Neustart (~10s)
- Geeignet für Teams < 50 Benutzer

### Multi-Replica-HA (Produktion empfohlen)

MySQL/PostgreSQL statt SQLite verwenden, unterstützt Multi-Replica:

1. **Datenbank auf MySQL umstellen**:

```yaml
# In overlay ConfigMap festlegen
configMapGenerator:
- name: k8ops-config
  literals:
  - DB_DRIVER=mysql
  - DB_DSN=k8ops:password@tcp(mysql:3306)/k8ops?charset=utf8mb4&parseTime=True
```

2. **Multi-Replica + Leader Election**:

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

3. **Gemeinsamer Speicher**: MySQL verwendet unabhängiges PVC, k8ops-Pod zustandslos

### Kapazitätsplanung

| Größe | Benutzerzahl | Ressourcenvorschlag | Datenbank |
|-------|-------------|---------------------|-----------|
| Klein | < 20 | 1 Pod, 500m CPU / 512Mi | SQLite |
| Mittel | 20-100 | 2 Pods, 1 CPU / 1Gi jeweils | MySQL |
| Groß | 100+ | 3+ Pods, 2 CPU / 2Gi jeweils | MySQL + Read/Write-Splitting |

---

## CI/CD-Prozess und Release-Verwaltung

### Ein-Klick-Bereitstellungsskript

k8ops bietet ein automatisiertes Bereitstellungsskript mit Vorabprüfung, Build, Release, Gesundheitsprüfung und automatischem Rollback:

```bash
# Neue Version bereitstellen (automatische Vorabprüfung + Build + Release + Gesundheitsprüfung)
./scripts/deploy.sh v14.36

# Bereitstellungsablauf:
# 1. Vorabprüfung: go build + go vet + go test + gofmt
# 2. Build: Docker buildx + Push zur Registry
# 3. Release: kubectl set image + change-cause Annotation
# 4. Verifizierung: Pod Ready + HTTP 200 (120s Timeout)
# 5. Rollback: automatischer Rollback bei fehlgeschlagener Gesundheitsprüfung
```

### Schnelles Rollback

```bash
# Zur vorherigen Version zurückkehren
./scripts/rollback.sh

# Zur angegebenen Revision zurückkehren
./scripts/rollback.sh 58

# Zur angegebenen Versionsnummer zurückkehren
./scripts/rollback.sh v14.30
```

### Release-Verlaufsnachverfolgung

Jede Bereitstellung zeichnet automatisch change-cause-Annotation auf:

```bash
# Release-Verlauf anzeigen
kubectl rollout history daemonset/k8ops -n k8ops-system

# Details einer bestimmten Revision anzeigen
kubectl rollout history daemonset/k8ops -n k8ops-system --revision=55
```

### CI-Prozess (GitHub Actions)

| Workflow | Auslöser | Inhalt |
|----------|----------|--------|
| `ci.yml` — Push/PR nach main | Code-Einreichung | test + vet + lint + govulncheck + Docker build |
| `release.yml` — Tag v* | Versions-Tag | Vollständige Tests + GoReleaser + Docker Multi-Arch + automatische Release Notes |

### Image-Verwaltung

| Tag | Beschreibung |
|-----|-------------|
| `registry.iot2.win/k8ops:v14.XX` | Spezifische Version |
| `registry.iot2.win/k8ops:latest` | Neueste stabile Version |
| `ghcr.io/<org>/k8ops:v14.XX` | GHCR-Image (CI-Release) |

### Image-Optimierung

- Basis-Image: `gcr.io/distroless/static-debian12:nonroot` (keine Shell, kein Paketmanager)
- Multi-Stage-Build: Go-Builder + Distroless-Runtime
- BuildKit-Cache: `--mount=type=cache` beschleunigt CI-Build
- Binär-Optimierung: `-trimpath -ldflags="-s -w"` reduziert Größe

| Version | Image-Größe |
|---------|------------|
| v14.30 (alpine) | 31.8 MB |
| v14.35 (distroless) | 28.6 MB |

### Hochverfügbarkeitskonfiguration

#### PodDisruptionBudget (PDB)

Stellen Sie sicher, dass während der Knoten-Wartung mindestens 1 Pod verfügbar ist:

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

Schränkt das Dashboard so ein, dass es nur Traffic vom Ingress-Controller akzeptiert:

- Ingress: Nur kube-system-Namespace kann 9090 (Dashboard) erreichen
- Ingress: Nur monitoring-Namespace kann 8080 (Metriken) erreichen
- Egress: DNS (53), HTTPS (443), K8s-API (6443) erlaubt

#### PriorityClass

k8ops verwendet `system-cluster-critical`-Priorität, um sicherzustellen, dass es bei Ressourcenknappheit nicht verdrängt wird.

#### Rolling-Update-Strategie

| Modus | maxUnavailable | maxSurge | Beschreibung |
|-------|---------------|----------|-------------|
| DaemonSet | 1 | - | Jeweils 1 Knoten aktualisieren |
| Deployment | 0 | 1 | Neuen Pod starten, dann alten löschen |

#### Ressourcenkontingente

| Modus | CPU-Request | CPU-Limit | Mem-Request | Mem-Limit |
|-------|------------|-----------|------------|-----------|
| DaemonSet | 100m | 1 | 128Mi | 1Gi |
| Deployment | 500m | 2 | 512Mi | 2Gi |

#### Gesundheitsprüfung und Lebenszyklusverwaltung

k8ops verwendet drei Sonden-Ebenen zur Sicherstellung der Zuverlässigkeit:

| Sonde | Pfad | Funktion | Parameter |
|-------|------|----------|-----------|
| **startupProbe** | `/healthz` | Wartet auf Start-Abschluss (verhindert, dass langsamer Start von Liveness beendet wird) | failureThreshold: 30, period: 5s (max. 150s Wartezeit) |
| **livenessProbe** | `/healthz` | Lebendigkeitsprüfung (bei Fehlschlag Pod-Neustart) | period: 20s, failureThreshold: 3, timeout: 5s |
| **readinessProbe** | `/readyz` | Bereitschaftsprüfung (bei Fehlschlag aus Service-Endpoints entfernt) | period: 10s, failureThreshold: 3, timeout: 5s |

**Graceful Shutdown:**

```yaml
lifecycle:
  preStop:
    exec:
      command: ["/manager", "--pre-stop"]
# --pre-stop führt sleep 5s aus, wartet bis Ingress-Controller diesen Pod entfernen kann
# Dann sendet kubelet SIGTERM, löst graceful Shutdown des Dashboards aus (SSE-Verbindungen beenden)
# terminationGracePeriodSeconds: 30 stellt ausreichend Zeit zur Verfügung
```

Shutdown-Ablauf:
1. kubelet führt `preStop` aus → sleep 5s (Verbindungsbeendigung)
2. kubelet sendet SIGTERM → Go-Signalhandler beginnt Graceful-Shutdown
3. Dashboard-HTTP-Server stoppt Annahme neuer Anfragen
4. SSE-Verbindungen beenden (10s Timeout)
5. Controller Manager fährt graceful herunter
6. Prozess beendet
