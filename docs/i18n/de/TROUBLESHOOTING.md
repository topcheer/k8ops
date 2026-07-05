# k8ops Fehlerbehebungsanleitung

> Dieses Dokument fasst Diagnosemethoden und Lösungen für häufige k8ops-Probleme zusammen. Nach Schweregrad kategorisiert für schnelle Lokalisierung.

---

## Inhaltsverzeichnis

1. [Installations- und Startprobleme](#1-installations--und-startprobleme)
2. [Authentifizierungs- und Login-Probleme](#2-authentifizierungs--und-login-probleme)
3. [KI-Funktionsprobleme](#3-ki-funktionsprobleme)
4. [Pod- und Cluster-Probleme](#4-pod--und-cluster-probleme)
5. [Netzwerk- und Ingress-Probleme](#5-netzwerk--und-ingress-probleme)
6. [Daten- und Speicherprobleme](#6-daten--und-speicherprobleme)
7. [Leistungsprobleme](#7-leistungsprobleme)
8. [Überwachungs- und Alarmierungsprobleme](#8-ueberwachungs--und-alarmierungsprobleme)

---

## 1. Installations- und Startprobleme

### 1.1 Pod bleibt dauerhaft im Pending-Zustand

**Symptom:** `kubectl get pods -n k8ops-system` zeigt Pending

**Diagnoseschritte:**
```bash
# Pending-Grund anzeigen
kubectl describe pod -n k8ops-system -l app.kubernetes.io/name=k8ops

# Häufige Ursachen:
# - PVC nicht gebunden (StorageClass überprüfen)
# - Ressourcenmangel (Knotenkapazität überprüfen)
# - Node Selector stimmt nicht überein
```

**Lösungen:**
- **PVC nicht gebunden:** Überprüfen Sie, ob der Cluster eine Standard-StorageClass hat
  ```bash
  kubectl get storageclass
  # Falls keine Standard-SC vorhanden, eine markieren:
  kubectl patch storageclass local-path -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
  ```
- **Ressourcenmangel:** Verwenden Sie den DaemonSet-Modus (keine PVC-Abhängigkeit)
  ```bash
  kubectl apply -k config/daemonset
  ```

### 1.2 Pod CrashLoopBackOff

**Symptom:** Pod startet wiederholt neu

**Diagnoseschritte:**
```bash
# Container-Logs anzeigen
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --tail=50

# Ereignisse anzeigen
kubectl get events -n k8ops-system --sort-by='.lastTimestamp' | tail -20
```

**Häufige Ursachen und Lösungen:**

| Ursache | Log-Merkmal | Lösung |
|---------|-------------|--------|
| SQLite-Berechtigungsproblem | `unable to open database file` | `mkdir -p /data && chown 65532:65532 /data` |
| JWT-Secret fehlt | `JWT secret not configured` | `AUTH_JWT_SECRET`-Umgebungsvariable setzen |
| K8s-API-Verbindung fehlgeschlagen | `failed to get Kubernetes config` | ServiceAccount und RBAC überprüfen |
| Portkonflikt | `bind: address already in use` | `--dashboard-address` ändern |

### 1.3 Image-Pull fehlgeschlagen (ImagePullBackOff)

**Symptom:** `Failed to pull image`

**Lösung:**
```bash
# Überprüfen, ob Image erreichbar ist
docker pull registry.iot2.win/k8ops:latest

# Falls private Registry verwendet wird, imagePullSecrets konfigurieren
kubectl create secret docker-registry regcred \
  --docker-server=registry.iot2.win \
  --docker-username=<user> \
  --docker-password=<pass> \
  -n k8ops-system

# Oder DaemonSet-Modus + hostPath verwenden (kein externes Image-Pull nötig)
```

---

## 2. Authentifizierungs- und Login-Probleme

### 2.1 Login gibt 401 Unauthorized zurück

**Diagnose:**
```bash
# Auth-Konfiguration überprüfen
kubectl exec -n k8ops-system deploy/k8ops -- /manager --help | grep auth

# Auth-bezogene Logs anzeigen
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -i auth
```

**Lösung:**
- Bestätigen, dass `AUTH_JWT_SECRET` gesetzt und konsistent ist
- Administrator-Passwort zurücksetzen:
  ```bash
  kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops admin reset-password
  ```
- Standard-Anmeldedaten: `admin` / `changeme` (nach erstem Login ändern)

### 2.2 OIDC-Login fehlgeschlagen

**Diagnose:**
- Bestätigen, dass OIDC-Provider-URL erreichbar ist (vom Pod aus)
- Redirect-URL mit Ingress-Domäne abgleichen
- Callback-Fehler anzeigen: `kubectl logs ... | grep oidc`

---

## 3. KI-Funktionsprobleme

### 3.1 Chat ohne Antwort oder Timeout

**Symptom:** Nach dem Senden einer Nachricht keine Antwort oder Timeout

**Diagnoseschritte:**
```bash
# Provider-Konfiguration überprüfen
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops config show

# KI-bezogene Logs anzeigen
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -E "provider|llm|agent"

# LLM-Konnektivität testen
kubectl exec -n k8ops-system deploy/k8ops -- curl -s https://api.openai.com/v1/models -H "Authorization: Bearer $AIOPS_API_KEY"
```

**Häufige Ursachen:**

| Ursache | Log-Merkmal | Lösung |
|---------|-------------|--------|
| API-Schlüssel ungültig | `401 Unauthorized` | `AIOPS_API_KEY`-Umgebungsvariable aktualisieren |
| Netzwerk nicht erreichbar | `context deadline exceeded` | LLM-API-Egress konfigurieren |
| Modell existiert nicht | `model not found` | `--provider-model` aktualisieren |
| Ratenbegrenzung | `429 Too Many Requests` | Warten oder Provider wechseln |
| Circuit Breaker geöffnet | `circuit breaker open` | 60s Abkühlphase abwarten |

### 3.2 KI-Diagnose wird nicht ausgelöst

**Diagnose:**
```bash
# Event-Collector-Status überprüfen
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -i "collector\|event"

# Bestätigen, dass nicht deaktiviert
kubectl get deploy k8ops -n k8ops-system -o jsonpath='{.spec.template.spec.containers[0].command}'
# Sollte nicht --disable-event-collector enthalten
```

---

## 4. Pod- und Cluster-Probleme

### 4.1 Dashboard zeigt "kubernetes client not available"

**Symptom:** API gibt 503 zurück, UI zeigt Verbindungsfehler

**Ursache:** K8s-ServiceAccount-Berechtigungen im Pod unzureichend oder Config-Laden fehlgeschlagen

**Lösung:**
```bash
# RBAC erneut anwenden
kubectl apply -k config/rbac

# ServiceAccount verifizieren
kubectl auth can-i list pods --as=system:serviceaccount:k8ops-system:k8ops -n k8ops-system
```

### 4.2 Operationen (Scale/Delete/Restart) geben 403 Forbidden zurück

**Ursache:** Benutzer-RBAC-Rollen-Berechtigungen unzureichend

**Lösung:**
```bash
# Benutzerrolle überprüfen
kubectl get rolebindings -n k8ops-system | grep <username>

# Auf Admin-Rolle aktualisieren
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops admin set-role <username> admin
```

---

## 5. Netzwerk- und Ingress-Probleme

### 5.1 Dashboard nicht erreichbar (502/503)

**Diagnose:**
```bash
# Überprüfen, ob Service Endpoints hat
kubectl get endpoints -n k8ops-system

# Ingress-Konfiguration überprüfen
kubectl get ingress -n k8ops-system
kubectl describe ingress -n k8ops-system

# Pod-Port direkt ansprechen
kubectl port-forward -n k8ops-system deploy/k8ops 9090:9090
# Dann http://localhost:9090 aufrufen
```

### 5.2 Traefik-Timeout (499/504)

**Symptom:** Registry-Push oder großer Datei-Upload-Timeout

**Lösung (Traefik-spezifisch):**
```bash
# Traefik-Timeout-Limit deaktivieren
kubectl patch deployment -n kube-system traefik \
  --type='json' \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.readtimeout=0s"}]'

# Oder Timeout in IngressRoute festlegen
```

### 5.3 SSE (Server-Sent Events) funktioniert nicht

**Symptom:** Chat-Oberfläche ohne Echtzeit-Antwort

**Diagnose:**
- Überprüfen Sie, ob der Reverse-Proxy Long Connections unterstützt
- Nginx-Konfiguration erfordert: `proxy_buffering off; proxy_cache off;`
- Traefik benötigt keine zusätzliche Konfiguration

---

## 6. Daten- und Speicherprobleme

### 6.1 SQLite-Datenbank beschädigt

**Symptom:** `database disk image is malformed`

**Lösung:**
```bash
# In den Pod wechseln
kubectl exec -it -n k8ops-system deploy/k8ops -- sh

# Datenbank reparieren (falls Distroless keine Shell hat, CLI-Tool verwenden)
# Lösung 1: Sicherung und Wiederherstellung
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops db backup /data/k8ops-backup.db
kubectl exec -n k8ops-system deploy/k8ops -- rm /data/k8ops.db
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops db restore /data/k8ops-backup.db

# Lösung 2: PVC löschen und neu erstellen (Benutzerdaten gehen verloren)
kubectl delete pvc -n k8ops-system k8ops-data
kubectl delete pod -n k8ops-system -l app.kubernetes.io/name=k8ops
```

### 6.2 PVC-Speicherplatz knapp

**Diagnose:**
```bash
kubectl exec -n k8ops-system deploy/k8ops -- df -h /data
# Oder über Dashboard → Kapazitätsseite anzeigen
```

**Lösung:**
- PVC erweitern:
  ```bash
  kubectl patch pvc -n k8ops-system k8ops-data -p '{"spec":{"resources":{"requests":{"storage":"5Gi"}}}}'
  ```
- Alte Audit-Logs bereinigen:
  ```bash
  kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops audit cleanup --max-age 30d
  ```

---

## 7. Leistungsprobleme

### 7.1 Langsame API-Antwort

**Diagnose:**
```bash
# Antwortzeit überprüfen (X-Response-Time Header)
curl -sk -o /dev/null -w '%{http_code} %{time_total}s\n' \
  -D - https://k8ops.iot2.win/api/cluster/overview 2>&1 | grep -i "x-response-time"

# Prometheus-Metriken anzeigen
curl -sk https://k8ops.iot2.win/metrics | grep k8ops_http_request_duration
```

**Optimierungsmaßnahmen:**
- API-Caching aktiviert (Overview: 30s, Ressourcen: 60s, CRDs: 10min)
- Überprüfen Sie, ob `k8ops_http_requests_in_flight` zu hoch ist
- Langsame Anfragen (>500ms) werden automatisch in Pod-Logs aufgezeichnet

### 7.2 Hohe Speicherauslastung

**Diagnose:**
```bash
kubectl top pods -n k8ops-system
```

**Optimierung:**
- Automatische Konversations-Speicherverwaltung: Zusammenfassung nach 20k-Token-Schwelle
- Inaktive Konversationen nach 30min bereinigt
- Bei dauerhaft hohem Speicherverbrauch Pod-Neustart in Betracht ziehen (DaemonSet-Modus startet automatisch neu)

---

## 8. Überwachungs- und Alarmierungsprobleme

### 8.1 Prometheus kann Metriken nicht abrufen

**Diagnose:**
```bash
# Metriken-Endpunkt bestätigen
kubectl exec -it <prometheus-pod> -n monitoring -- curl -s http://k8ops.k8ops-system.svc:8080/metrics | head -5

# ServiceMonitor überprüfen
kubectl get servicemonitor -n k8ops-system
```

**Hinweis:** Der `/metrics`-Endpunkt erlaubt nur Localhost-Zugriff. Prometheus muss aus dem Cluster (gleicher Pod oder Service) abrufen.

### 8.2 Alarmierungsregeln nicht wirksam

**Diagnose:**
```bash
# PrometheusRule überprüfen
kubectl get prometheusrule -n k8ops-system

# Alarmierungsregeln anwenden
kubectl apply -f config/alerting-rules.yaml
```

---

## Anhang: Häufige Diagnosebefehle

```bash
# Ein-Klick-Statusprüfung
kubectl get pods -n k8ops-system
kubectl get events -n k8ops-system --sort-by='.lastTimestamp' | tail -20
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --tail=50

# Gesundheitsprüfung
curl -sk https://k8ops.iot2.win/api/health
curl -sk https://k8ops.iot2.win/api/version

# Sicherheits-Scan
curl -sk https://k8ops.iot2.win/api/security/audit | jq .summary
curl -sk https://k8ops.iot2.win/api/security/compliance | jq .score

# Kapazitätsplanung
curl -sk https://k8ops.iot2.win/api/capacity/planning | jq .summary
```

## Anhang: Log-Ebenen

k8ops verwendet strukturierte JSON-Logs (slog) mit folgenden Ebenen:

| Ebene | Verwendung | Beispiel |
|-------|-----------|----------|
| `INFO` | Normalbetrieb | Server-Start, erfolgreiche Authentifizierung |
| `WARN` | Langsame Anfragen, Konfigurationsprobleme | Anfrage >500ms, PVC fast voll |
| `ERROR` | Fehlgeschlagene Operationen | K8s-API-Fehler, LLM-Aufruf fehlgeschlagen |

Log-Korrelation über Request-ID:
```bash
# Request-ID aus HTTP-Antwort-Header X-Request-ID abrufen
# Dann in Logs suchen
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep "a1b2c3d4"
```

---

*Zuletzt aktualisiert: 2026-07-03*
*Verwalter: k8ops-Team*
