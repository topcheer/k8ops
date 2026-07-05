# k8ops Betriebsleitfaden (Runbook)

> Dieses Dokument richtet sich an das Betriebspersonal und deckt tägliche Wartungsarbeiten, Incident-Management-Prozesse, Notfallkontakte und Standardarbeitsanweisungen ab.

---

## Inhaltsverzeichnis

1. [Serviceübersicht](#1-serviceübersicht)
2. [Tägliche Wartung](#2-tägliche-wartung)
3. [Incident-Management](#3-incident-management)
4. [Notfalloperationen](#4-notfalloperationen)
5. [Backup und Wiederherstellung](#5-backup-und-wiederherstellung)
6. [Leistungsoptimierung](#6-leistungsoptimierung)
7. [Notfallkontakte](#7-notfallkontakte)
8. [SLO/SLA-Definition](#8-slosla-definition)

---

## 1. Serviceübersicht

### Architekturbeschreibung

```
┌─────────────────────────────────────────────────┐
│                   Benutzerbrowser                  │
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
│  │  Go Binary (eing. stat. Frontend-Ressourcen)       │ │
│  │  :8080 Dashboard                             │ │
│  │  /metrics  Prometheus                        │ │
│  │  /api/chat  SSE → LLM Provider               │ │
│  │  nsenter → host kubectl                      │ │
│  └─────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────┘
```

### Schlüsselkomponenten

| Komponente | Standort | Funktion |
|------------|----------|----------|
| k8ops DaemonSet | k8ops-system | Hauptservice, ein Pod pro Knoten |
| Traefik | kube-system | Ingress-Controller, TLS-Terminierung |
| Registry | registry.iot2.win | Private Image-Registry |
| LLM Provider | Externe API | KI-Chat- / Diagnose- / Optimierungs-Engine |

### Health-Check-Endpunkte

| Endpunkt | Erwartete Antwort | Hinweise |
|----------|-------------------|----------|
| `https://k8ops.iot2.win/` | 200/303 | Frontend-Seite |
| `https://k8ops.iot2.win/readyz` | 200 | K8s Readiness-Sonde |
| `https://k8ops.iot2.win/api/version` | 200 JSON | Versionsinformationen |
| `https://k8ops.iot2.win/metrics` | 200 (nur lokal) | Prometheus-Metriken |

---

## 2. Tägliche Wartung

### 2.1 Servicestatus prüfen

```bash
# Pod-Status
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops

# Service-Logs (letzte 100 Zeilen)
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --tail=100

# Versionsinformationen
curl -sk https://k8ops.iot2.win/api/version | jq .

# Cluster-Übersicht
curl -sk https://k8ops.iot2.win/api/cluster/overview | jq .
```

### 2.2 Deployment aktualisieren

```bash
# Neue Version erstellen
cd /Volumes/new/ggai/k8ops
VERSION=v14XX
docker buildx build --platform linux/amd64 \
  --build-arg VERSION=$VERSION \
  -t registry.iot2.win/k8ops:$VERSION \
  -t registry.iot2.win/k8ops:latest \
  --push .

# Rolling Update
kubectl set image daemonset/k8ops \
  k8ops=registry.iot2.win/k8ops:$VERSION -n k8ops-system

# Verifikation
sleep 15
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops
curl -sk -o /dev/null -w '%{http_code}' https://k8ops.iot2.win/
```

### 2.3 Log-Verwaltung

k8ops verwendet strukturierte `log/slog`-Logs, die Log-Ebene wird über die Umgebungsvariable `LOG_LEVEL` gesteuert:

| Ebene | Verwendung |
|-------|------------|
| `DEBUG` | Entwicklung/Debugging, gibt alle Logs aus |
| `INFO` (Standard) | Produktionsbetrieb, protokolliert Schlüsselvorgänge |
| `WARN` | Nur Warnungen und Fehler |

```bash
# Log-Ebene ändern
kubectl set env daemonset/k8ops LOG_LEVEL=DEBUG -n k8ops-system
```

### 2.4 Provider-Konfiguration

KI-Funktionen erfordern die Konfiguration eines LLM-Providers:

1. Besuchen Sie die Seite Settings → Provider
2. Wählen Sie einen Provider (OpenAI / Zhipu / DeepSeek usw.)
3. Geben Sie den API Key ein
4. Testen Sie die Verbindung

Wenn nicht konfiguriert, zeigt das Dashboard eine Warnbanner, dass der Provider nicht konfiguriert ist.

---

## 3. Incident-Management

### 3.1 Pod startet nicht (CrashLoopBackOff)

**Symptom**: Der k8ops-Pod startet wiederholt neu

**Diagnoseschritte**:
```bash
# 1. Pod-Ereignisse anzeigen
kubectl describe pod -n k8ops-system -l app.kubernetes.io/name=k8ops

# 2. Container-Logs anzeigen
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --previous

# 3. RBAC-Berechtigungen prüfen
kubectl auth can-i --list --as=system:serviceaccount:k8ops-system:k8ops

# 4. ConfigMap/Secret-Einhängung prüfen
kubectl exec -n k8ops-system -it deploy/k8ops -- ls -la /etc/k8ops/
```

**Häufige Ursachen**:
- Unzureichende RBAC-Berechtigungen → prüfen Sie `config/rbac/`
- Ungültige kubeconfig → prüfen Sie die eingehängte kubeconfig
- Portkonflikt → prüfen Sie, ob Port 8080 belegt ist
- Unzureichender Speicher → prüfen Sie die Knotenressourcen `kubectl describe nodes`

### 3.2 Dashboard nicht erreichbar (502/503)

**Symptom**: https://k8ops.iot2.win gibt 502 oder 503 zurück

**Diagnoseschritte**:
```bash
# 1. Ingress prüfen
kubectl get ingress -A | grep k8ops

# 2. Traefik prüfen
kubectl get pods -n kube-system -l app.kubernetes.io/name=traefik
kubectl logs -n kube-system -l app.kubernetes.io/name=traefik --tail=50

# 3. k8ops-Service prüfen
kubectl get svc -n k8ops-system
kubectl get endpoints -n k8ops-system

# 4. Pod direkt testen
kubectl exec -n k8ops-system -it deploy/k8ops -- curl -s localhost:8080/api/version
```

**Häufige Ursachen**:
- Traefik routet nicht korrekt → prüfen Sie die Ingress-Regeln
- k8ops nicht bereit → prüfen Sie die readyz-Sonde
- TLS-Zertifikat abgelaufen → prüfen Sie cert-manager

### 3.3 AI Chat reagiert nicht

**Symptom**: Nach dem Senden einer Chat-Nachricht keine Antwort oder Timeout

**Diagnoseschritte**:
```bash
# 1. Provider-Status prüfen
curl -sk https://k8ops.iot2.win/api/provider/status | jq .

# 2. Engine-Logs anzeigen
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -i 'llm\|provider\|chat'

# 3. Provider-Verbindung testen
curl -sk https://k8ops.iot2.win/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"message":"hello","conversationId":"test"}' --max-time 30
```

**Häufige Ursachen**:
- API Key nicht konfiguriert oder abgelaufen
- Provider-API-Ratenbegrenzung (429)
- Netzwerk nicht erreichbar (DNS/Firewall)
- Token-Limit überschritten → der Agent komprimiert automatisch den Kontext, kann aber in Extremfällen fehlschlagen

### 3.4 Registry-Push fehlgeschlagen (499)

**Symptom**: `docker push` gibt 499 Client Closed Request zurück

**Lösung**:
```bash
# Traefik-Timeout-Konfiguration prüfen
kubectl get deploy -n kube-system traefik -o jsonpath='{.spec.template.spec.containers[0].args}'

# Falls Timeout-Parameter fehlen, hinzufügen:
kubectl patch deploy -n kube-system traefik --type='json' -p='[
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.readtimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.writetimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.idletimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.keepalivemaxtime=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.keepalivemaxrequests=0"}
]'
```

### 3.5 Schreibvorgänge fehlgeschlagen (Scale/Delete/Restart)

**Symptom**: Scale/Delete/Restart-Operationen schlagen nach Klick auf die Schaltflächen fehl

**Diagnoseschritte**:
```bash
# RBAC-Berechtigungen prüfen
kubectl auth can-i patch deployments --as=system:serviceaccount:k8ops-system:k8ops -n default
kubectl auth can-i delete pods --as=system:serviceaccount:k8ops-system:k8ops -n default

# Audit-Log anzeigen
curl -sk https://k8ops.iot2.win/api/audit?severity=critical | jq .

# Sicherheitsrichtlinien prüfen
kubectl get psp,podsecurity --all-namespaces 2>/dev/null
```

---

## 4. Notfalloperationen

### 4.1 Schnelles Rollback

```bash
# Versionsverlauf anzeigen
kubectl rollout history daemonset/k8ops -n k8ops-system

# Zur vorherigen Version zurückkehren
kubectl rollout undo daemonset/k8ops -n k8ops-system

# Zurück zu einer bestimmten Version
kubectl rollout undo daemonset/k8ops -n k8ops-system --to-revision=3
```

### 4.2 Notfall-Herunterskalierung (auf 0 Replikate)

```bash
# Hinweis: DaemonSet unterstützt kein Scale 0, direkte Löschung erforderlich
kubectl delete pod -n k8ops-system -l app.kubernetes.io/name=k8ops --grace-period=0 --force

# Zum vollständigen Stoppen temporär nodeSelector ändern
kubectl patch daemonset k8ops -n k8ops-system -p='{"spec":{"template":{"spec":{"nodeSelector":{"non-existent":"true"}}}}}'
```

### 4.3 Datenbereinigung

```bash
# CRD-Diagnoseverlauf bereinigen
kubectl delete diagnostics --all --all-namespaces

# Audit-Logs bereinigen (letzte 7 Tage behalten)
kubectl get auditlogs -A -o json | jq '.items[] | select(.metadata.creationTimestamp < "'$(date -d '7 days ago' -Iseconds)'")' | kubectl delete -f -

# Optimierungsberichte bereinigen
kubectl delete optimizations --all --all-namespaces
```

---

## 5. Backup und Wiederherstellung

### 5.1 Konfigurations-Backup

```bash
# k8ops-Konfiguration sichern
kubectl get cm,secret,daemonset -n k8ops-system -o yaml > k8ops-backup-$(date +%Y%m%d).yaml

# CRD-Daten sichern
kubectl get diagnostics,remediations,optimizations -A -o yaml > k8ops-crd-backup-$(date +%Y%m%d).yaml

# RBAC sichern
kubectl get clusterrole,clusterrolebinding -o yaml | grep -A5 k8ops > k8ops-rbac-backup-$(date +%Y%m%d).yaml
```

### 5.2 Wiederherstellungsprozess

```bash
# Konfiguration wiederherstellen
kubectl apply -f k8ops-backup-YYYYMMDD.yaml

# CRD-Daten wiederherstellen
kubectl apply -f k8ops-crd-backup-YYYYMMDD.yaml

# Verifikation
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops
curl -sk https://k8ops.iot2.win/api/version | jq .
```

### 5.3 Empfehlung für regelmäßige Backups

Verwenden Sie Velero oder einen Cron-Job für tägliche Backups:
```bash
# Velero-Backup (empfohlen)
velero backup create k8ops-daily-$(date +%Y%m%d) \
  --include-namespaces k8ops-system \
  --include-cluster-resources=true
```

---

## 6. Leistungsoptimierung

### 6.1 Schlüsselmetriken

| Metrik | Prometheus Metric | Alarm-Schwelle |
|--------|-------------------|-----------------|
| API-Latenz | `k8ops_tool_call_duration_seconds` | P99 > 10s |
| LLM-Aufruflatenz | `k8ops_llm_call_duration_seconds` | P99 > 60s |
| Aktive Diagnosen | `k8ops_active_diagnostics` | > 10 |
| Sicherheitsblockierungen | `k8ops_safety_blocks_total` | rate > 10/min |
| Token-Verbrauch | `k8ops_llm_tokens_total` | Anormaler Tagesanstieg |
| Cluster-Integritätsbewertung | `k8ops_cluster_health_score` | < 60 |

### 6.2 Ressourcenempfehlungen

| Knotengröße | k8ops Resource Request | Resource Limit |
|-------------|------------------------|----------------|
| <= 5 Knoten | 100m CPU / 128Mi | 500m CPU / 512Mi |
| 5-20 Knoten | 200m CPU / 256Mi | 1 CPU / 1Gi |
| 20-50 Knoten | 500m CPU / 512Mi | 2 CPU / 2Gi |

### 6.3 Log-Ebenen-Optimierung

In der Produktion wird empfohlen, die Ebene `INFO` beizubehalten. Wechseln Sie nur zur Problemdiagnose temporär auf `DEBUG`:
```bash
# DEBUG temporär aktivieren
kubectl set env daemonset/k8ops LOG_LEVEL=DEBUG -n k8ops-system
# Nach Diagnose wiederherstellen
kubectl set env daemonset/k8ops LOG_LEVEL=INFO -n k8ops-system
```

---

## 7. Notfallkontakte

### 7.1 Eskalationsprozess

```
Incident-Erkennung → Bereitschafts-Operator (L1)
    ├── Nicht gelöst in 5 Minuten → Betriebsverantwortlicher (L2)
    │     ├── Nicht gelöst in 15 Minuten → Architekt (L3)
    │     │     ├── Auswirkung auf Produktion → CTO-Benachrichtigung
```

### 7.2 Kontakttabelle

> Entsprechend der tatsächlichen Situation ausfüllen

| Rolle | Name | Telefon | Verantwortungsbereich |
|-------|------|---------|----------------------|
| L1 Bereitschafts-Operator | ____ | ____ | Erstantwort, grundlegende Incident-Behandlung |
| L2 Betriebsverantwortlicher | ____ | ____ | Komplexe Incidents, Auswirkungen auf mehrere Services |
| L3 Architekt | ____ | ____ | Architektur-Ebene-Probleme, Datenwiederherstellung |
| Cluster-Administrator | ____ | ____ | Ausfälle des K8s-Clusters selbst |
| Netzwerk/Sicherheit | ____ | ____ | Netzwerkrichtlinien, Zertifikate, Sicherheitsvorfälle |

### 7.3 Anbieter-Kontakte

| Anbieter | Verwendung | Kontakt |
|----------|-----------|---------|
| LLM Provider | KI-Chat/Diagnose | ____ |
| Registry | Image-Registry | ____ |
| DNS/CDN | Namensauflösung | ____ |

---

## Anhang: Prometheus-Metrikenliste

k8ops stellt die folgenden benutzerdefinierten Metriken bereit (Endpunkt `/metrics`):

| Metric | Typ | Labels | Beschreibung |
|--------|------|--------|--------------|
| `k8ops_diagnostics_total` | Counter | phase, trigger | Gesamtzahl der Diagnoseberichte |
| `k8ops_remediation_actions_total` | Counter | type, result, risk | Gesamtzahl der Behebungsvorgänge |
| `k8ops_llm_call_duration_seconds` | Histogram | provider, model, status | LLM-Aufruflatenz |
| `k8ops_llm_tokens_total` | Counter | provider, model, type | Token-Verbrauch |
| `k8ops_agent_steps` | Histogram | - | Agent-Ausführungsschritte |
| `k8ops_tool_call_duration_seconds` | Histogram | tool, success | Tool-Aufruflatenz |
| `k8ops_safety_blocks_total` | Counter | reason | Anzahl Sicherheitsblockierungen |
| `k8ops_active_diagnostics` | Gauge | - | Aktuell aktive Diagnosen |
| `k8ops_active_remediations` | Gauge | - | Aktuell ausgeführte Behebungen |
| `k8ops_audit_events_total` | Counter | type, severity | Gesamtzahl der Audit-Ereignisse |
| `k8ops_cluster_health_score` | Gauge | - | Cluster-Integritätsbewertung (0-100) |
| `k8ops_conversation_count` | Gauge | - | Anzahl aktiver Konversationen |
| `k8ops_tool_executions_total` | Counter | tool, success | Gesamtzahl der Tool-Ausführungen |
| `k8ops_http_requests_total` | Counter | method, path, status | Gesamtzahl der HTTP-Anfragen |
| `k8ops_http_request_duration_seconds` | Histogram | method, path | HTTP-Anfragenlatenz |
| `k8ops_http_requests_in_flight` | Gauge | - | Aktuell verarbeitete Anfragen |
| `k8ops_api_errors_total` | Counter | method, path, status | API-Fehleranzahl (4xx+5xx) |

---

## 8. SLO/SLA-Definition

### 8.1 Service Level Objectives (SLO)

| Metrik | Ziel | Messfenster | Fehlerbudget |
|--------|------|-------------|--------------|
| Dashboard-Verfügbarkeit | 99.9% | 30 Tage rollierend | 43.2 Minuten/Monat |
| API-Erfolgsrate (nicht 429) | 99.5% | 30 Tage rollierend | 3.6 Stunden/Monat |
| API P99-Latenz | < 2s | Echtzeit | - |
| AI Chat-Antwortzeit | < 30s (erstes Token) | Echtzeit | - |
| Sicherheits-Audit-Scan abgeschlossen | < 60s | Echtzeit | - |

### 8.2 Fehlerbudget-Verwaltung

Monatliches Verfügbarkeitsziel von 99.9% = **43.2 Minuten Fehlerbudget**:

- **Im Budget (<30min)**: Normales Veröffentlichungstempo, keine zusätzliche Genehmigung erforderlich
- **Budget-Warnung (30-43min)**: Nicht-dringende Änderungen einfrieren, Zuverlässigkeitsprobleme priorisieren
- **Budget erschöpft (>43min)**: Vollständiger Veröffentlichungsstopp, Post-Mortem-Analyse durchführen

### 8.3 SLO-Überwachungsabfragen (Prometheus PromQL)

**API-Fehlerrate (5 Minuten):**
```promql
sum(rate(k8ops_api_errors_total{status=~"5.."}[5m])) by (path)
/ sum(rate(k8ops_http_requests_total[5m])) by (path)
```

**API P99-Latenz:**
```promql
histogram_quantile(0.99,
  sum(rate(k8ops_http_request_duration_seconds_bucket[5m])) by (le, path)
)
```

**Fehlerbudget-Verbrauchsrate:**
```promql
1 - (
  sum(rate(k8ops_http_requests_total{status!~"5.."}[30d]))
  / sum(rate(k8ops_http_requests_total[30d]))
)
```

### 8.4 Degradationsstrategie

Wenn das SLO kurz davor steht, verletzt zu werden, nach Priorität degradieren:

1. **AI Chat deaktivieren** — die ressourcenintensivste Funktion, deren Degradierung die zentrale K8s-Verwaltung nicht beeinträchtigt
2. **Cache-TTL erhöhen** — den Cache für Overview/Nodes/Pods von 30s auf 120s erhöhen
3. **Gleichzeitige Diagnosen begrenzen** — das Limit für `k8ops_active_diagnostics` senken
4. **Ereignis-Sammler deaktivieren** — Flag `--disable-event-collector`

### 8.5 Anfrageverfolgung

Alle HTTP-Antworten enthalten den `X-Request-ID`-Header, verwendet für:
- Log-Korrelation — alle Log-Zeilen derselben Anfrage teilen sich die request_id
- Audit-Verfolgung — die request_id im Audit-Log kann mit der spezifischen HTTP-Anfrage verknüpft werden
- Incident-Diagnose — wenn ein Benutzer ein Problem meldet, ermöglicht die Angabe der request_id eine schnelle Lokalisierung der Logs

Beispiel für Log-Abfrage:
```bash
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep "a1b2c3d4e5f6"
```

### 8.6 Log-Ebenen-Konfiguration

k8ops verwendet strukturierte JSON-Logs (slog) mit Unterstützung für die Konfiguration der Ebene über die Umgebungsvariable `LOG_LEVEL` oder das Kommandozeilenargument `--log-level`:

| Ebene | Verwendung | Beschreibung |
|-------|-----------|--------------|
| `debug` | Problemdiagnose | Enthält source file:line, sehr detaillierte Logs (nicht für Produktion empfohlen) |
| `info` | Standard | Normale Betriebs-Logs (für Produktion empfohlen) |
| `warn` | Nur Warnungen | Langsame Anfragen, Konfigurationsprobleme, nahe am Schwellenwert |
| `error` | Nur Fehler | Nur Betriebsfehler aufzeichnen |

Konfigurationsmethoden:
```bash
# Über Umgebungsvariable (empfohlen)
kubectl set env daemonset/k8ops -n k8ops-system LOG_LEVEL=debug

# Über ConfigMap
kubectl patch configmap k8ops-config -n k8ops-system \
  --type='json' -p='[{"op":"add","path":"/data/log-level","value":"debug"}]'

# Über Kommandozeilenargument (nur im Deployment-Modus anwendbar)
# args:
# - --log-level=debug
```

Pod nach Ebenenwechsel neu starten:
```bash
kubectl rollout restart daemonset/k8ops -n k8ops-system
```

### 8.7 Log-Rotation

Die Audit-Log-Datei (`/data/k8ops-audit.jsonl`) wird automatisch rotiert:
- **Automatische Rotation**: Die Datei wird automatisch geteilt, wenn sie 100MB überschreitet
- **Manuelle Rotation**: `POST /api/system/log/rotate` (Admin-Berechtigung)
- **Alte Dateien bereinigen**: `POST /api/system/log/cleanup` (löscht Rotationsdateien älter als 30 Tage)

Container-stdout-Logs werden von Kubelet verwaltet, mit einem Standardlimit von 10MB x 3 Dateien = 30MB pro Container.
In k3s können sie über `--container-log-max-size` und `--container-log-max-files` angepasst werden.

---

*Letzte Aktualisierung: 2026-07-02*
*Betreuer: k8ops-Team*
