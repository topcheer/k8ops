# k8ops API-Referenz

Alle Endpunkte werden auf dem Dashboard-Port bereitgestellt (Standard `:9090`).

**Authentifizierung:** JWT-Cookie (`k8ops_token`) oder `Authorization: Bearer <token>`-Header.
**Content-Type:** `application/json` für alle POST/PUT-Anfragen.

## OpenAPI-3.0-Spezifikation

k8ops generiert automatisch eine OpenAPI-3.0-Spezifikation, die für die automatische SDK-Generierung, die Integration in API-Gateways oder das Durchsuchen in Swagger UI verwendet werden kann.

| Endpunkt | Beschreibung |
|----------|-------------|
| `GET /api/openapi.json` | Gibt die vollständige OpenAPI 3.0 JSON-Spezifikation zurück |
| `GET /api/docs` | Gibt nach Tags gruppierte API-Dokumentations-Metadaten zurück (inkl. spec + tagGroups) |

**Spezifikation abrufen:**
```bash
curl -sk https://k8ops.iot2.win/api/openapi.json -o k8ops-openapi.json
```

**In Swagger Editor importieren:**
1. https://editor.swagger.io öffnen
2. Datei → Datei importieren → `k8ops-openapi.json` auswählen

**Im Dashboard:** Die Seitenleiste → API-Dokumentation bietet einen interaktiven API-Browser mit Suche, Filterung und Online-Testfunktion.

---

## Gesundheit und System

| Methode | Pfad | Auth | Beschreibung |
|---------|------|------|-------------|
| GET | `/api/health` | Keine | Liveness-Probe — gibt `{"status":"ok"}` zurück |
| GET | `/api/version` | Keine | Build-Version, Git-Commit, Go-Version |

## Cluster

| Methode | Pfad | Auth | Beschreibung |
|---------|------|------|-------------|
| GET | `/api/cluster/overview` | Erforderlich | Cluster-Zusammenfassung: Knotenanzahl, Pod-Anzahl, CPU/Speicher-Nutzung, Warnungen (30s Cache) |
| GET | `/api/nodes` | Erforderlich | Alle Knoten mit Ressourcennutzung und Bedingungen (30s Cache) |
| GET | `/api/nodes/{node}/pods` | Erforderlich | Pods auf einem bestimmten Knoten |
| GET | `/api/pods` | Erforderlich | Alle Pods über alle Namespaces (30s Cache) |
| GET | `/api/pods/{namespace}/{name}/containers` | Erforderlich | Container-Liste für einen Pod |
| GET | `/api/pods/{namespace}/{name}/logs?container=&follow=&tailLines=` | Erforderlich | Pod-Logs (unterstützt SSE-Streaming mit `follow=true`) |
| GET | `/api/events?namespace=&warning=` | Erforderlich | Kubernetes-Ereignisse, optional nach Namespace/Warnung gefiltert |
| GET | `/api/resources?kind=&namespace=` | Erforderlich | Generischer Ressourcen-Lister (Deployments, Services, etc.) (60s Cache) |
| GET | `/api/crds?with_counts=true` | Erforderlich | Custom Resource Definitions (10min Cache mit Zählungen) |
| GET | `/api/crd-resources?group=&version=&resource=&namespace=` | Erforderlich | CRD-Instanzen (60s Cache) |
| GET | `/api/yaml?namespace=&name=&group=&version=&resource=&kind=` | Erforderlich | YAML-Ansicht jeder Kubernetes-Ressource |

## Diagnose und Behebung

| Methode | Pfad | Auth | Beschreibung |
|---------|------|------|-------------|
| GET | `/api/diagnostics` | Erforderlich | DiagnosticReport CRs auflisten, optionaler `?namespace=`-Filter |
| GET | `/api/diagnostics/{namespace}/{name}` | Erforderlich | Diagnose-Detail mit KI-Analyse |
| GET | `/api/remediations` | Erforderlich | Remediation CRs auflisten, optionaler `?namespace=`-Filter |
| GET | `/api/optimizations` | Erforderlich | Optimization CRs auflisten, optionaler `?namespace=`-Filter |

## KI-Chat

| Methode | Pfad | Auth | Beschreibung |
|---------|------|------|-------------|
| POST | `/api/chat` | Erforderlich | Nachricht an KI-Assistenten senden (SSE-Streaming-Antwort) |
| GET | `/api/chat/conversations?id=` | Erforderlich | Konversationen auflisten oder eine nach ID abrufen |

### POST /api/chat

**Anfrage:**
```json
{
  "message": "Why is my pod crashing?",
  "conversation_id": "optional-existing-id",
  "stream": true
}
```

**Antwort:** SSE-Stream der KI-Analyse mit Tool-Aufrufen und Ergebnissen.

### GET /api/chat/conversations

Gibt Konversationsverlauf zurück. `?id=<uuid>` für eine einzelne Konversation übergeben.

## Provider-Verwaltung

| Methode | Pfad | Auth | Beschreibung |
|---------|------|------|-------------|
| GET | `/api/provider/status` | Erforderlich | Aktuelle KI-Provider-Konfiguration (maskierter API-Schlüssel) |
| POST | `/api/provider/update` | Erforderlich | Provider-Typ/Modell/Endpunkt zur Laufzeit aktualisieren |
| POST | `/api/provider/reload` | Erforderlich | Provider-Konfiguration aus K8opsConfig CRD neu laden |
| GET | `/api/tools` | Erforderlich | Registrierte Diagnose-Tools auflisten |

## Auth

| Methode | Pfad | Auth | Beschreibung |
|---------|------|------|-------------|
| POST | `/api/auth/login` | Öffentlich | Lokaler Login (ratenbegrenzt) |
| POST | `/api/auth/logout` | Erforderlich | Auth-Cookie löschen |
| GET | `/api/auth/me` | Erforderlich | Aktuelle Benutzerinformationen |
| POST | `/api/auth/change-password` | Erforderlich | Eigenes Passwort ändern |
| GET | `/api/auth/status` | Öffentlich | Auth-Konfigurationsstatus (auth_enabled, user_count, ldap/oidc-Flags) |
| GET | `/api/auth/provider-presets` | Öffentlich | Verfügbare OIDC/LDAP-Provider-Vorlagen |

### POST /api/auth/login

**Anfrage:**
```json
{
  "username": "admin",
  "password": "admin"
}
```

**Antwort (200):**
```json
{
  "user": {"id": 1, "username": "admin", "role": "admin", "display_name": "Administrator"},
  "must_change": true,
  "redirect_url": "/"
}
```

Setzt `k8ops_token`-Cookie (HttpOnly, SameSite=Lax, 24h).

**Fehler (401):**
```json
{"error": "invalid username or password"}
```

## OIDC

| Methode | Pfad | Auth | Beschreibung |
|---------|------|------|-------------|
| GET | `/api/auth/oidc/{provider}/login` | Öffentlich | Weiterleitung zum OIDC-Provider (setzt CSRF-State-Cookie) |
| GET | `/api/auth/oidc/{provider}/callback` | Öffentlich | OIDC-Callback (validiert State, erstellt Benutzersitzung) |

## Auth-Provider-Verwaltung (Admin)

| Methode | Pfad | Auth | Beschreibung |
|---------|------|------|-------------|
| GET | `/api/auth/providers` | Admin | Konfigurierte Auth-Provider auflisten |
| POST | `/api/auth/providers` | Admin | Auth-Provider erstellen (LDAP/OIDC) |
| GET | `/api/auth/providers/{id}` | Admin | Provider nach ID abrufen |
| PUT | `/api/auth/providers/{id}` | Admin | Provider-Konfiguration aktualisieren |
| DELETE | `/api/auth/providers/{id}` | Admin | Provider löschen |

## Benutzerverwaltung (Admin)

| Methode | Pfad | Auth | Beschreibung |
|---------|------|------|-------------|
| GET | `/api/admin/users` | Admin | Alle Benutzer auflisten |
| POST | `/api/admin/users` | Admin | Benutzer erstellen (Standardrolle: viewer, MustChangePwd=true) |
| GET | `/api/admin/users/{id}` | Admin | Benutzer nach ID abrufen |
| PUT | `/api/admin/users/{id}` | Admin | Benutzer aktualisieren (Rolle, Namespaces, etc.) |
| DELETE | `/api/admin/users/{id}` | Admin | Benutzer löschen |
| POST | `/api/admin/users/{id}/reset-password` | Admin | Passwort zurücksetzen (setzt MustChangePwd=true) |
| GET | `/api/admin/auth-config` | Admin | Auth-Konfiguration abrufen |
| PUT | `/api/admin/auth-config` | Admin | Auth-Konfiguration aktualisieren |

## API-Schlüssel

| Methode | Pfad | Auth | Beschreibung |
|---------|------|------|-------------|
| GET | `/api/auth/api-keys` | Erforderlich | Eigene API-Schlüssel auflisten |
| POST | `/api/auth/api-keys` | Erforderlich | API-Schlüssel erstellen |
| DELETE | `/api/auth/api-keys/{id}` | Erforderlich | API-Schlüssel widerrufen |

## RBAC-Verwaltung (Admin)

| Methode | Pfad | Auth | Beschreibung |
|---------|------|------|-------------|
| GET | `/api/rbac/clusterroles` | Admin | Cluster-Rollen auflisten |
| GET | `/api/rbac/clusterroles/{name}` | Admin | Cluster-Rolle nach Name abrufen |
| DELETE | `/api/rbac/clusterroles/{name}` | Admin | Cluster-Rolle löschen |
| GET | `/api/rbac/roles?namespace=` | Admin | Namespace-bezogene Rollen auflisten |
| GET | `/api/rbac/roles/{namespace}/{name}` | Admin | Namespace-bezogene Rolle abrufen |
| DELETE | `/api/rbac/roles/{namespace}/{name}` | Admin | Namespace-bezogene Rolle löschen |
| GET | `/api/rbac/rolebindings?namespace=` | Admin | Role Bindings auflisten |
| GET | `/api/rbac/rolebindings/{namespace}/{name}` | Admin | Role Binding abrufen |
| DELETE | `/api/rbac/rolebindings/{namespace}/{name}` | Admin | Role Binding löschen |
| GET | `/api/rbac/api-resources` | Admin | Kubernetes-API-Ressourcentypen auflisten |
| GET | `/api/rbac/namespaces` | Admin | Alle Namespaces auflisten |
| GET | `/api/rbac/role-mapping?role=&kind=&name=&namespace=` | Admin | Rollen-zu-Subjekt-Zuordnung anzeigen |
| GET | `/api/rbac/role-defs` | Admin | k8ops benutzerdefinierte Rollendefinitionen auflisten |
| GET | `/api/rbac/subjects?kind=&namespace=` | Admin | Subjekte auflisten (Benutzer/Gruppen/Service-Accounts) |

## Audit

| Methode | Pfad | Auth | Beschreibung |
|---------|------|------|-------------|
| GET | `/api/audit?namespace=&limit=` | Erforderlich | Audit-Protokolleinträge (paginiert) |
| GET | `/api/audit/stats` | Erforderlich | Audit-Statistikzusammenfassung |

## Konfiguration

| Methode | Pfad | Auth | Beschreibung |
|---------|------|------|-------------|
| GET | `/api/config` | Erforderlich | k8ops Controller-Konfiguration (Provider-Typ/Modell, Funktionen) |

## Sicherheits-Audit

| Methode | Pfad | Auth | Beschreibung |
|---------|------|------|-------------|
| GET | `/api/security/audit` | Erforderlich | Cluster-Sicherheits-Scan — prüft Pod Security Standards, RBAC, Network-Policy-Abdeckung, Secret-Sicherheit |
| GET | `/api/security/health` | Erforderlich | Plattform-Sicherheits-Health-Check — Authentifizierung/TLS/K8s-API-Konnektivität |

### GET /api/security/audit

Scannt den gesamten Cluster und gibt eine Liste von Sicherheitsbefunden zurück, nach Schweregrad sortiert (critical > high > medium > low > info).

**Prüfpunkte:**
- **Pod Security:** Privilegierte Container, Root-Ausführung, Rechteausweitung, gefährliche Capabilities, hostPath/hostNetwork
- **RBAC:** cluster-admin-Bindungen, Standard-SA-Verwendung
- **Netzwerk:** Namespaces ohne NetworkPolicy
- **Secrets:** Docker-Registry-Secret-Rotierungsempfehlungen
- **Ressourcen:** Container ohne Resource Limits

**Antwortbeispiel:**
```json
{
  "summary": {"critical": 0, "high": 2, "medium": 5, "low": 8, "info": 3, "total": 18},
  "findings": [
    {
      "severity": "high",
      "category": "Pod Security",
      "resource": "default/pod/nginx/container/app",
      "namespace": "default",
      "detail": "Container \"app\" allows privilege escalation",
      "fix": "Set allowPrivilegeEscalation: false in securityContext"
    }
  ],
  "scannedAt": "2025-01-15T10:30:00Z"
}
```

## Schreiboperationen

| Methode | Pfad | Auth | Beschreibung |
|---------|------|------|-------------|
| POST | `/api/scale` | Erforderlich | Deployment/StatefulSet skalieren |
| POST | `/api/pod/delete` | Erforderlich | Einzelnen Pod löschen |
| POST | `/api/rollout/restart` | Erforderlich | Rolling-Restart von Deployment/DaemonSet/StatefulSet |
| POST | `/api/node/cordon` | Erforderlich | Knoten isolieren/wiederherstellen |
| POST | `/api/yaml/apply` | Erforderlich | YAML anwenden (kubectl apply) |

Alle Schreiboperationen werden im Audit-Protokoll aufgezeichnet.

---

## Fehlerantworten

Alle Fehler geben JSON zurück:

```json
{"error": "descriptive error message"}
```

| Code | Bedeutung |
|------|----------|
| 400 | Bad Request (fehlende/ungültige Parameter) |
| 401 | Unauthorized (fehlendes/abgelaufenes/ungültiges Token) |
| 403 | Forbidden (unzureichende Rolle) |
| 404 | Ressource nicht gefunden |
| 500 | Interner Serverfehler |
| 503 | Service nicht verfügbar (KI-Provider nicht konfiguriert) |

## Rollen

| Rolle | Berechtigungen |
|-------|---------------|
| `admin` | Vollzugriff inkl. Benutzer/RBAC/Provider-Verwaltung |
| `operator` | Dashboard + Diagnosen + Chat (keine Benutzerverwaltung) |
| `viewer` | Schreibgeschütztes Dashboard + Chat |
| `ns-admin` | Admin nur innerhalb zugewiesener Namespaces |
| `ns-viewer` | Viewer nur innerhalb zugewiesener Namespaces |

## Neue Endpunkte (v14.48-v14.53)

Die folgenden Endpunkte wurden in v14.48 bis v14.53 hinzugefügt und in die OpenAPI-3.0-Spezifikation aufgenommen.

### Container-Image-Inventar

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/images` | Inventar aller Container-Images im Cluster, inkl. Resource-Limit-Audit und `:latest`-Tag-Erkennung |

**Zusammenfassungsfelder der Antwort:** `totalImages`, `withoutLimits`, `withoutRequests`, `usingLatestTag`, `uniqueRegistries`

### Warnungsereignis-Zusammenfassung

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/events/summary` | Aggregiert alle Warning-Ereignisse nach Reason, mit Schweregrad-Klassifizierung und betroffenen Namespace-Statistiken |

### Cluster-Effizienzanalyse

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/efficiency` | Cluster-Ressourceneffizienzanalyse: Pods ohne Limits, überprovisionierte Container, unzureichend genutzte Knoten, Effizienzbewertung 0-100 |

### Sicherheit: Secret-Expositions-Scan

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/security/secrets` | Erkennt hartcodierte Anmeldedaten, Secret-Rotierungsverfolgung (90 Tage), ungenutzte Secrets, sensible Schlüsselnamen |

### Audit-Suche und -Export

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/audit/events` | Audit-Ereignissuche: unterstützt `actor`, `action`, `q` (Volltextsuche), `severity`, Datumsbereichsfilter |
| GET | `/api/audit/export` | Export von Audit-Ereignissen im CSV-Format (für SIEM-Systeme importierbar) |

### Backup-Verwaltung

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/system/backup` | Alle Backup-Dateien auflisten (Größe, Alter, Typ) |
| POST | `/api/system/backup` | Datenbank-Backup erstellen (Zeitstempel-benannt) |
| DELETE | `/api/system/backup?name=X` | Bestimmtes Backup löschen (Pfad-Traversal-Schutz) |
| POST | `/api/system/backup/restore?name=X` | Datenbank aus Backup wiederherstellen |

### Alertmanager Webhook

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| POST | `/api/webhooks/alertmanager` | Empfängt Prometheus Alertmanager v4-Alarme, generiert automatisch Untersuchungsvorschläge |
| POST | `/api/webhooks/alertmanager/test` | Test-Alarm senden zur Empfängerverifizierung |

**Alertmanager-Konfigurationsbeispiel:**
```yaml
receivers:
  - name: k8ops
    webhook_configs:
      - url: http://k8ops.k8ops-system.svc:9090/api/webhooks/alertmanager
        send_resolved: true
```

### Änderungsprotokoll

| Version | Endpunkt | Dimension |
|---------|----------|-----------|
| v14.49 | `GET /api/events/summary` | Product |
| v14.50 | Startup-Probe + preStop | Deployment |
| v14.51 | `POST /api/webhooks/alertmanager` | Operations |
| v14.52 | `GET /api/efficiency` | Scalability |
| v14.53 | `GET /api/security/secrets` | Security |
| v14.54 | OpenAPI 3.0 spec + API.md | Documentation |
| v14.55 | `GET /api/pdbs` `GET /api/compatibility` | Product |
| v14.56 | `GET /api/certificates/expiry` | Operations |
| v14.57 | Graceful-Shutdown-Draining-Gate | Deployment |
| v14.58 | `GET /api/addons/health` | Product |
| v14.59 | `GET /api/capacity/forecast` | Scalability |
| v14.60 | OpenAPI-Spec-Vervollständigung + API.md-Update | Documentation |
| v14.61 | `GET /api/security/network-policies` | Security |
| v14.62 | `GET /api/diagnostics/restarts` | Operations |
| v14.63 | `GET /api/deployments/rollout` | Deployment |
| v14.64 | `GET /api/resources/waste` | Product |
| v14.65 | `GET /api/scaling/bottlenecks` | Scalability |

### Pod Disruption Budget Status (v14.55+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/pdbs` | Listet alle PDBs mit Disruption-Status, übereinstimmenden Workloads, Gesundheitseinschätzung (healthy/at-risk/blocked), für sichere Pre-Drain-Prüfung |

### K8s-Distributions-Kompatibilitätserkennung (v14.55+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/compatibility` | Erkennt automatisch Cluster-Distribution (vanilla/k3s/RKE2/EKS/GKE/AKS/OpenShift/Talos), Versionskompatibilität, ARM/Windows/GPU-Knoten-Features |

### TLS-Zertifikats-Ablauf-Scan (v14.56+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/certificates/expiry` | Scannt alle TLS/Opaque Secrets nach X.509-Zertifikaten, klassifiziert nach Ablaufzeit (expired/critical/warning/ok), verknüpft Ingress-Ressourcen |

### Server-Drain-Status (v14.57+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/system/drain-status` | Meldet Server-Graceful-Shutdown-Status: draining, shutdownInitiated, activeConnections, uptime |

### Add-on-Gesundheitsprüfung (v14.58+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/addons/health` | Nicht-invasive Erkennung von 39 gängigen K8s-Add-ons (12 Kategorien: CNI/DNS/Ingress/CertManager/LB/Mesh/Backup/Monitoring/Policy/Storage/GitOps/VM) |

### Kapazitätserschöpfungs-Vorhersage (v14.59+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/capacity/forecast` | Sagt voraus, wann CPU/Speicher/Pod/Speicherplatz-Kapazität erschöpft ist, basierend auf Wachstumsraten mit Tagen-bis-Erschöpfung und Skalierungsempfehlungen |

### Network-Policy-Audit-Scan (v14.61+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/security/network-policies` | Auditiert NetworkPolicy-Abdeckung: erkennt Namespaces ohne NetworkPolicy, lockerer Richtlinien (0.0.0.0/0 In-/Egress), teilweiser Abdeckung, klassifiziert nach Schweregrad (critical/warning/info) |

**Abfrageparameter:** `namespace` (optional, Namespace-Filter)

**Rückgabebeispiel:**
```json
{
  "summary": {
    "totalNamespaces": 27,
    "withoutNetPol": 25,
    "findings": 18,
    "critical": 10,
    "warning": 8
  },
  "namespaces": [...]
}
```

### Pod-Restart-Diagnose (v14.62+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/diagnostics/restarts` | Diagnostiziert Pod-Restart-Muster und Ursachen: klassifiziert Restart-Verhalten (crash-loop/occasional/post-deploy), extrahiert Terminierungsursachen (OOMKilled/Error/Exit-Code), Wait-Status (CrashLoopBackOff/ImagePullBackOff) |

**Abfrageparameter:** `namespace` (optional)

**Diagnose-Muster:**
- **crash-loop**: Viele Restarts in kurzer Zeit
- **occasional**: Wenige Restarts über lange Zeit
- **post-deploy**: Sofortige Restarts nach dem Deployment

### Deployment-Rollout-Status-Tracking (v14.63+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/deployments/rollout` | Scannt Rollout-Health-Status aller Deployment/StatefulSet/DaemonSet: 7 Zustände (complete/in-progress/stalled/degraded/paused/failed/scaled-to-zero), erkennt ProgressDeadlineExceeded, ReplicaFailure, Generation-Mismatch |

**Abfrageparameter:**
- `namespace` (optional) — Namespace-Filter
- `status` (optional) — Rollout-Status-Filter: `failed`, `degraded`, `stalled`, `in-progress`, `paused`, `scaled-to-zero`, `complete`

**Status-Bedeutungen:**
| Status | Bedeutung |
|--------|-----------|
| `complete` | Alle Repliken aktualisiert und bereit |
| `in-progress` | Rolling-Update läuft |
| `stalled` | Controller hat neueste Spec nicht beobachtet (Generation-Mismatch) |
| `degraded` | Einige Repliken nicht verfügbar |
| `paused` | Deployment explizit pausiert |
| `failed` | ProgressDeadlineExceeded, Deployment-Timeout |
| `scaled-to-zero` | Replikanzahl ist 0 |

### Ressourcenverschwendung-Erkennung (v14.64+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/resources/waste` | Scannt Verschwendung und verwaiste Ressourcen zur Kostensenkung: 6 Verschwendungstypen (dead-service/unused-pvc/orphaned-configmap/orphaned-secret/empty-namespace/unattached-pv), 4 Schweregrade (critical/high/medium/low), Kostenrisikobewertung |

**Abfrageparameter:** `namespace` (optional)

**Verschwendungstypen:**
| Kategorie | Erkennung | Standard-Schweregrad |
|-----------|----------|---------------------|
| `dead-service` | Service ohne Backend-Endpoint (LoadBalancer = critical) | medium/critical |
| `unused-pvc` | PVC, das von keinem Pod eingehängt ist | high |
| `orphaned-configmap` | ConfigMap, die von keinem Pod referenziert wird | low/medium |
| `orphaned-secret` | Secret, das von keinem Pod referenziert wird (Sicherheitsrisiko) | high |
| `empty-namespace` | Namespace ohne laufende Pods | medium |
| `unattached-pv` | PV im Available-Zustand (nicht an PVC gebunden) | critical |

**Intelligente Filterung:** Überspringt automatisch kube-system-Namespace, ServiceAccount-Token-Secrets, Helm-Release-Secrets, automatisch generierte ConfigMaps

### Skalierungs-Engpass-Erkennung (v14.65+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/scaling/bottlenecks` | Scannt Faktoren, die horizontale Skalierung einschränken: 7 Engpasstypen (node-schedulable/node-pressure/resource-quota/hpa-stuck/pdb-blocking/storage-exhaust/image-pull-limit), 4 Auswirkungsgrade (critical/high/moderate/low), Cluster-Kapazitätszusammenfassung |

**Abfrageparameter:** `namespace` (optional)

**Engpasstypen:**
| Kategorie | Erkennung |
|-----------|----------|
| `node-schedulable` | Isolierte Knoten, Cluster-Pod-Kapazität überschritten (>75% Warnung / >90% Kritisch) |
| `node-pressure` | Speicher-, Festplatten-, PID-Druck-Zustände |
| `resource-quota` | Namespace-Quote über 75%/90% |
| `hpa-stuck` | HPA erreicht maxReplicas oder fehlende Metriken |
| `pdb-blocking` | PDB erlaubt 0 freiwillige Disruptions |
| `storage-exhaust` | Namespace-PVC-Anfragen über 500Gi |

**Cluster-Kapazitätszusammenfassung:** Knotenanzahl, CPU/Speicher-Kapazität vs. Zuweisbare, Pod-Kapazität vs. zugewiesen, Skalierungsspielraum

### RBAC-Berechtigungsrisikoanalyse (v14.67+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/security/rbac-risk` | Analysiert Berechtigungsrisiken aller RoleBinding/ClusterRoleBinding, 0-100 Bewertungssystem, 5 Risikostufen (critical/high/elevated/moderate/low), erkennt cluster-admin-Bindungen, Rechteausweitung, Wildcard-Berechtigungen, sensible Ressourcenzugriffe |

**Abfrageparameter:** `namespace` (optional)

**Risikobewertungsregeln:**
| Prüfpunkt | Basispunkte | Zusätzliche Punkte |
|-----------|------------|-------------------|
| ClusterRoleBinding + cluster-admin | 100 | - |
| Rechteausweitung (escalate/bind/impersonate) | - | +25 |
| Wildcard-Verben (verbs: *) | - | +25 |
| Wildcard-Ressourcen (resources: *) | - | +20 |
| Cluster-weite Schreiboperationen | - | +30 |
| Sensible Ressourcenzugriffe (secrets/pods/exec) | - | +15 |

### CronJob-Ausführungs-Health-Monitoring (v14.68+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/operations/cronjobs/health` | Überwacht Ausführungsgesundheit aller CronJobs: Erfolgsrate, aufeinanderfolgende Fehler, pausierte/stehende Zeitpläne, nie ausgeführt, 5 Gesundheitsstufen (healthy/warning/failing/suspended/no-runs) |

**Abfrageparameter:** `namespace` (optional)

**Gesundheitsstufen:**
| Status | Auslöser |
|--------|---------|
| `failing` | 3+ aufeinanderfolgende Fehler |
| `warning` | 1-2 aufeinanderfolgende Fehler oder Erfolgsrate < 50% |
| `suspended` | CronJob ist suspendiert |
| `no-runs` | Nie ausgeführt |
| `healthy` | Alle kürzlich erfolgreich |

### Service- und Endpoint-Health-Monitoring (v14.69+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/networking/health` | Scannt Netzwerk-Health aller Services und Ingress: Services ohne Endpoints, Selector-Mismatch, Endpoint-Degradation, wartende LoadBalancer, fehlende Ingress-Backend-Services/Endpoints, 5 Gesundheitsstufen |

**Abfrageparameter:** `namespace` (optional)

**Service-Gesundheitsstufen:**
| Status | Bedeutung |
|--------|-----------|
| `misconfigured` | Selector-Mismatch — kein Pod entspricht dem Label-Selector |
| `no-endpoints` | Alle Endpoints nicht verfügbar |
| `degraded` | Einige Endpoints nicht verfügbar |
| `external` | ExternalName/LoadBalancer (informativ) |
| `healthy` | Alle Endpoints normal |

**Ingress-Gesundheitsprüfung:** Erkennt, ob Backend-Service existiert und verfügbare Endpoints hat, validiert Default-Backend und Regel-Pfade

### PV/PVC-Speicher-Health-Monitoring (v14.70+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/storage/health` | Scannt Speicher-Health aller PVC/PV: Pending-PVC-Diagnose, verwaiste PVCs (gebunden aber >1 Tag kein Pod), Lost/Failed PVCs, freigegebene/fehlgeschlagene PVs mit manueller Bereinigung, veraltete Available-PVs mit verschwendeter Kapazität, 6 Gesundheitsstufen + StorageClass-Verteilungsanalyse |

**Abfrageparameter:** `namespace` (optional)

**PVC-Gesundheitsstufen:**
| Status | Bedeutung |
|--------|-----------|
| `failed` | PVC-Provisionierung fehlgeschlagen |
| `lost` | Zugrundeliegendes PV wurde gelöscht |
| `pending` | Wartet auf Provisionierung (keine StorageClass, WaitForFirstConsumer) |
| `near-capacity` | Nahe der Kapazitätsgrenze |
| `orphaned` | Gebunden aber >1 Tag ohne Pod-Nutzung |
| `bound` | Normal gebunden |

**PV-Problem-Erkennung:** Released PVs (manuelle Bereinigung nötig), Failed PVs (Recycling fehlgeschlagen), veraltete Available PVs (>7 Tage verschwendete Kapazität)

**StorageClass-Analyse:** Default-Class-Markierung, Provisioner, Reclaim-Policy, Binding-Mode, Volume-Expansion-Unterstützung, PVC-Anzahlverteilung

### ServiceAccount-Sicherheitsaudit (v14.72+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/security/service-accounts` | Umfassender Sicherheitsaudit aller ServiceAccounts: ungenutzte SAs, Standard-SA von Pods verwendet, unnötige Token-Auto-Mounting, cluster-admin-Bindungen, Cluster-weite Berechtigungen, veraltete SAs, Legacy-langlebige Token-Secrets |

**Abfrageparameter:** `namespace` (optional)

**Risikobewertung:** 0-100 (höher = gefährlicher), 5 Schweregrade: critical / high / elevated / moderate / low

**Prüfpunkte:**
| Prüfpunkt | Schweregrad | Beschreibung |
|-----------|------------|-------------|
| Ungenutzte SA (>7 Tage kein Pod-Verweis) | moderate | Vergrößerte Angriffsfläche |
| Standard-SA von Pods verwendet | elevated | Verletzung des Least-Privilege-Prinzips |
| cluster-admin-Bindung | critical | Cluster-weite Super-Berechtigungen |
| Unnötiges Token-Auto-Mounting | moderate | SAs ohne Token-Bedarf sollten nicht mounten |
| Veraltete SA (>30 Tage ungenutzt aber noch mit Berechtigungen) | high | Zombie-Berechtigungen |
| Legacy-langlebiges Token-Secret (K8s <1.24) | high | Nicht empfohlene langlebige Tokens |

### SLO/SLA-Fehlerbudget-Tracking (v14.73+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/operations/slo` | SLO/SLA-Verfügbarkeits- und Fehlerbudget-Tracking basierend auf Multi-Window-Multi-Burn-Rate-Algorithmus |

**Abfrageparameter:** `namespace` (optional)

**Fensterkonfiguration:** 5m / 1h / 6h / 24h / 7d

**Rückgabeinhalte:**
| Feld | Beschreibung |
|------|-------------|
| `availability` | Verfügbarkeitsprozentsatz pro Fenster |
| `errorBudget` | Verbleibendes Fehlerbudget und Verbrauchsrate |
| `burnRate` | Multi-Window-Burn-Rate (fast: 5m/1h, slow: 6h/24h) |
| `latencySLO` | P50/P95/P99-Latenz-Perzentile und Ziele |
| `status` | meeting (erfüllt) / at-risk (Risiko) / violated (verletzt) |

### ResourceQuota- und LimitRange-Monitoring (v14.74+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/resources/quota` | Scannt ResourceQuota-Auslastung und LimitRange-Standardbeschränkungen aller Namespaces |

**Abfrageparameter:** `namespace` (optional)

**Quota-Status-Stufen:**
| Status | Auslastung | Beschreibung |
|--------|-----------|-------------|
| `ok` | <70% | Normal |
| `warning` | 70-85% | Nahe am Limit |
| `critical` | 85-100% | Gefährlich |
| `exceeded` | >100% | Überschritten |
| `no-limit` | — | Keine Quota gesetzt |

**Prüfpunkte:** CPU/Speicher/Pod/ConfigMap/Secret/Speicher-Quota-Auslastung pro Namespace, Namespaces ohne Quota-Schutz, LimitRange-Standard/Min/Max-Beschränkungsanalyse, Top-Verbraucher-Ranking

### Deployment-Konfigurationsaudit (v14.75+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/deployments/audit` | Auditiert Best-Practice-Verstöße aller Deployment/StatefulSet/DaemonSet-Konfigurationen, 8 Prüfkategorien, jede mit Schweregrad und Reparaturempfehlung |

**Abfrageparameter:** `namespace` (optional), `severity` (optional: critical / warning / info)

**Prüfkategorien:**
| Kategorie | Prüfpunkte |
|-----------|-----------|
| `revision-history` | Zu wenige Revisionsverläufe (<2, kein Rollback möglich) oder zu viele (>20, Ressourcenverschwendung) |
| `image-policy` | `:latest`-Tag aber pullPolicy nicht Always; festes Tag aber pullPolicy Always |
| `resources` | Fehlende Resource Limits/Requests |
| `probes` | Fehlende liveness/readiness/startup-Sonden |
| `security-context` | Privilegierte Container, Root-Ausführung, beschreibbares Root-Dateisystem, Rechteausweitung erlaubt |
| `update-strategy` | Recreate-Strategie (Ausfall), OnDelete (manuelle Pod-Löschung erforderlich), partitioniertes Rolling-Update |
| `lifecycle` | terminationGracePeriod zu kurz (<10s) oder zu lang (>300s), fehlender preStop-Hook |
| `config-drift` | Fehlendes Seccomp-Profil |

**Gesundheitsbewertung:** 0 (perfekt) bis 100 (schlechtestens), critical=20 Punkte/warning=8 Punkte/info=2 Punkte

### Scheduling-Health- und Ressourcenfragmentierung-Analyse (v14.76+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/scheduling/health` | Analysiert Cluster-Scheduling-Health, Knoten-Schedulability, Ressourcenfragmentierung und Pending-Pod-Diagnose |

**Abfrageparameter:** `namespace` (optional)

**Rückgabeinhalte:**
| Feld | Beschreibung |
|------|-------------|
| `summary` | Knotenstatistiken (schedulable/unschedulable/cordoned/under-pressure), Pending-Pod-Anzahl, FailedScheduling-Anzahl, 24h-Evictions, Health-Score 0-100 |
| `nodes` | Pro-Knoten-Schedulable-Status, Drucktyp, Taints, CPU/Speicher/Pod-Verfügbarmenge und Prozentsatz |
| `pendingPods` | Pending-Pod-Liste mit CPU/Speicher-Anfragen, NodeSelector, aufgelösten Scheduling-Fehlerursachen |
| `largestFittablePod` | Größter aktuell schedulbarer Pod (CPU/Speicher/Pod-Anzahl), bester Knoten |
| `effectiveCapacity` | Theoretische vs. effektive Kapazität (Kapazitätsverlustprozentsatz durch unschedulable Knoten) |
| `fragmentation` | Ressourcenfragmentierungsmetriken (durchschnittliche CPU/Speicher-Fragmentierungsrate, schlechtester Fragmentierungsknoten, übergroßer Pod erkannt) |
| `evictions` | 24h-Eviction-Aufzeichnungen (Pod, Knoten, Grund) |
| `recommendations` | Umsetzbare Reparaturempfehlungen |

**Scheduling-Fehlerursachen-Auflösung:** insufficient-cpu / insufficient-memory / untolerated-taint / node-selector-mismatch / node-affinity-mismatch / pod-affinity-conflict / pod-limit-reached / volume-binding-failure / no-nodes-available

### Pod-Sicherheitsstatus-Scan (v14.79+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/security/pods` | Auditiert Sicherheitsstatus aller laufenden Pods: privilegierte Container, hostNetwork/hostPID/hostIPC, HostPath-Mounts, gefährliche Linux-Capabilities, Root-Ausführung, Rechteausweitung erlaubt, beschreibbares Root-Dateisystem, fehlender Security-Context, :latest/ohne-Tag-Images, nicht per Digest gesperrt, Secret-Umgebungsvariablen-Injection, keine Resource-Limits, Host-Port-Bindung |

**Abfrageparameter:** `namespace` (optional), `severity` (optional: critical / warning / info)

**Risikobewertung:** 0 (sicher) bis 100 (sehr hohes Risiko), critical=25 Punkte/warning=8 Punkte/info=2 Punkte

**Prüfkategorien:**
| Kategorie | Schweregrad | Beschreibung |
|-----------|------------|-------------|
| `privileged` | critical | Privilegierter Container — voller Host-Zugriff |
| `host-network` | critical | Geteilter Knoten-Netzwerk-Namespace |
| `host-pid` | critical | Sichtbarkeit aller Knoten-Prozesse |
| `host-ipc` | critical | Geteilter IPC-Namespace |
| `host-path` | critical | HostPath-Volume vom Knoten gemountet |
| `dangerous-capabilities` | critical | SYS_ADMIN/NET_ADMIN/NET_RAW/SYS_PTRACE/SYS_MODULE/DAC_OVERRIDE/SETUID/SETGID |
| `runs-as-root` | warning | Läuft als UID 0 |
| `privilege-escalation` | warning | Rechteausweitung erlaubt |
| `missing-security-context` | warning | Security-Context fehlt |
| `image-latest` | warning | Verwendet :latest-Tag |
| `image-no-tag` | warning | Kein Tag (Standard :latest) |
| `host-port` | warning | Host-Port-Bindung |
| `image-no-digest` | info | Nicht per Digest gesperrt |
| `writable-rootfs` | info | Beschreibbares Root-Dateisystem |
| `secret-env-vars` | info | Secret als Umgebungsvariable injiziert |
| `no-resource-limits` | info | Keine Resource-Limits |

### Ereignissturm- und Kaskadenausfall-Erkennung (v14.80+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/operations/event-storm` | Analysiert Cluster-Warning-Ereignisse, erkennt Ereignisstürme, Kaskadenausfälle und Ressourcen-Jitter. Zählt Alarm-Ereignisse in 15min/1h/24h-Zeitfenstern, stuft Sturm-Schweregrad ein, identifiziert jitternde Ressourcen (gleiche Ressource + gleicher Grund 3+ Mal wiederholt), aggregiert nach Namespace und Grund, bietet umsetzbare Empfehlungen |

**Abfrageparameter:** `namespace` (optional)

**Sturm-Schweregrad:**
| Schweregrad | Bedingung | Beschreibung |
|-----------|-----------|-------------|
| `critical` | >50 events/15min | Sofortige Untersuchung |
| `high` | >20 events/15min | Aufmerksamkeit erforderlich |
| `medium` | >10 events/15min | Trend überwachen |
| `low` | >5 events/15min | Informativ |

**Rückgabeinhalte:** Sturmerkennungsergebnis, Namespace-Alarm-Ranking, Top-Ereignisgründe, Jitter-Ressourcen-Liste (mit Jitter-Frequenz), letzte 15min-Ereigniszeitlinie, Anzahl betroffener Ressourcen (Explosionsradius), umsetzbare Empfehlungen

### Ressourcenabhängigkeitsgraph und Auswirkungsanalyse (v14.81+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/dependencies` | Verfolgt den vollständigen Abhängigkeitsgraphen beliebiger Workloads (Deployment/StatefulSet/DaemonSet/Pod), bewertet Änderungsauswirkungen |

**Abfrageparameter:**

| Parameter | Erforderlich | Beschreibung |
|-----------|-------------|-------------|
| `kind` | Ja | Ressourcentyp: Deployment / StatefulSet / DaemonSet / Pod |
| `name` | Ja | Ressourcenname |
| `namespace` | Nein | Namespace (Standard: default) |

**Vorwärtsabhängigkeiten (wovon diese Workload abhängt):** ConfigMap, Secret, PVC, ServiceAccount

**Rückwärtsabhängigkeiten (was von dieser Workload abhängt):**
- Service (über Label-Selector passend zum Pod)
- Ingress (routet zu passendem Service)
- NetworkPolicy (angewendet auf diesen Pod)
- HPA (mit dieser Workload als Ziel)
- Andere Pods, die geteilte ConfigMap/Secret verwenden

**Auswirkungsanalyse:** blastRadius = Vorwärtsabhängigkeiten + Rückwärtsabhängigkeiten, Risikostufe low(<6) / medium(6-10) / high(11-20) / critical(>20)

### Topologie-Verteilungs-Compliance-Prüfung (v14.82+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/topology/spread` | Analysiert Pod-Verteilung in Topologie-Domains (Zone/Region/Knoten), validiert topologySpreadConstraints-Compliance |

**Abfrageparameter:** `namespace` (optional), `domain` (optional, Topologie-Domain-Key, Standard `kubernetes.io/hostname`, kann auf `topology.kubernetes.io/zone` gesetzt werden)

**Workload-Status:**
| Status | Bedeutung |
|--------|-----------|
| `balanced` | Gleichmäßig verteilt (actualSkew ≤ maxSkew) |
| `skewed` | Ungleich verteilt (actualSkew > maxSkew) |
| `no-constraint` | Multi-Replica aber ohne Topologie-Beschränkung |
| `single-replica` | Single-Replica (Topologie-Verteilung nicht anwendbar) |

**Rückgabeinhalte:** Topologie-Domain-Statistiken, pro-Workload Domain-Verteilung (Pod-Anzahl/Erwartung), tatsächliche vs. maximale Abweichung, Domain-Labels und Pod-Anzahl pro Knoten, Empfehlungen (Beschränkungen hinzufügen, Knoten labeln, Single-Domain-Cluster-Hinweis)

### Secret-Rotations- und Lebenszyklus-Audit (v14.85+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/security/secrets/rotation` | Auditiert Rotations-Compliance und Lebenszyklusverwaltung aller Secrets: Alters-Tracking (stale >90d / very stale >180d), ungenutzte Secret-Erkennung (von keinem Pod referenziert), TLS-Zertifikats-Ablauf-Erkennung (Zertifikat geparst), Docker-Registry-Secret-Tracking, Legacy-ServiceAccount-Token-Erkennung, sensible Namens-Erkennung |

**Abfrageparameter:** `namespace` (optional)

**Risikobewertung:** Risiko-Level pro Secret (critical / high / medium / low), Cluster-Rotations-Score 0-100

**Prüfkategorien:**
| Prüfpunkt | Schweregrad | Beschreibung |
|-----------|------------|-------------|
| TLS-Zertifikat abgelaufen | critical | Sofort erneuern |
| Docker-Secret >180d alt | critical | Könnte veraltete Registry-Anmeldedaten enthalten |
| TLS-Zertifikat <30d bis Ablauf | high | Erneuerung bald einplanen |
| Stale + ungenutzt + sensibler Name | high | Sicherheitsrisiko |
| Stale Docker-Secret | medium | Rotierung empfohlen |
| Stale aber in Verwendung | low | Rotierung einplanen |

### Health-Probe-Effektivitäts-Audit (v14.86+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/operations/probes` | Auditiert liveness/readiness/startup-Sonden-Konfiguration aller Workloads, erkennt falsch konfigurierte Sonden, die Kaskaden-Neustarts, Traffic zu unvorbereiteten Pods, Startfehler verursachen |

**Abfrageparameter:** `namespace` (optional)

**Prüfkategorien:**
| Prüfpunkt | Schweregrad | Beschreibung |
|-----------|------------|-------------|
| Liveness fehlt | warning | Hängende Container werden nicht neu gestartet |
| Readiness fehlt | warning | Traffic könnte unvorbereitete Pods erreichen |
| Sonde zu aggressiv (period <5s) | warning | Zu hohe Last auf API-Server |
| Timeout zu kurz (<2s) | warning | Fehleinschätzung bei Latenzspitzen |
| Failure-Threshold zu niedrig (<3) | warning | Zu empfindlich gegen transiente Fehler |
| Readiness-Intervall zu lang (>60s) | info | Langsame Bereitschaftserkennung |
| Liveness-Failure-Threshold zu hoch (>10) | info | Langsamer Neustart/Erholung |
| Gleiche liveness+readiness | info | Sollte differenziert konfiguriert werden |

**Rückgabeinhalte:** Risiko-Score pro Workload, Cluster-Effektivitäts-Score (0-100), aggregierte Top-Probleme, umsetzbare Empfehlungen

### Workload-Überalterungs-Tracking (v14.87+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/product/staleness` | Verfolgt die Deployment-Überalterung aller Workloads, erkennt lange nicht aktualisierte Workloads, Images mit :latest-Tag, nicht per Digest gesperrte Images |

**Abfrageparameter:** `namespace` (optional)

**Überalterungsklassifizierung:**
| Status | Bedingung | Beschreibung |
|--------|-----------|-------------|
| `fresh` | <7d | Kürzlich aktualisiert |
| `recent` | <30d | Ziemlich neu |
| `stale` | <90d | Aufmerksamkeit erforderlich |
| `very-stale` | <180d | Aktualisierung empfohlen |
| `ancient` | >180d | Sicherheitsrisiko |

**Rückgabeinhalte:** Risiko-Level pro Workload, Image-Tag-Analyse (:latest / digest / no-tag), Altersverteilungs-Buckets, Namespace-Statistiken, Cluster-Freshness-Score (0-100), umsetzbare Empfehlungen

### Ressourcen-Overcommit- und Druck-Analyse (v14.88+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/scalability/overcommit` | Analysiert CPU- und Speicher-Overcommit-Raten aller Knoten, erkennt gefährlichen Overcommit, Pods ohne Limits, Ressourcendruck-Bewertung |

**Abfrageparameter:** `namespace` (optional)

**Pro-Knoten-Analyse:**
| Metrik | Beschreibung |
|--------|-------------|
| CPU-Request-Commit | sum(requests) / allocatable |
| CPU-Limit-Commit | sum(limits) / allocatable |
| Speicher-Request/Limit-Commit | wie oben |
| Druck-Score | 0-100 (gewichtet berechnet) |
| Risiko-Level | safe / moderate / high / critical (>3x) |

**Cluster-Metriken:** Gesamt CPU/Speicher-Overcommit-Rate, Anzahl Risiko-Knoten, Anzahl Pods ohne Limits, Sicherheits-Score (0-100), Namespace-Ressourcenverbrauch-Details, umsetzbare Empfehlungen

### Image-Sicherheits- und Lieferketten-Analyse (v14.92+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/security/images` | Scannt Lieferketten-Sicherheitsrisiken aller laufenden Container-Images: Digest-Sperrung, :latest-Tag, Images ohne Tag, alte Versionstags, öffentliche vs. private Image-Registries, unbekannte Image-Registries |

**Abfrageparameter:** `namespace` (optional)

**Prüfkategorien:**
| Prüfpunkt | Risikopunkte | Beschreibung |
|-----------|-------------|-------------|
| Kein Tag | +25 | Standardmäßig :latest, Version unsicher |
| Verwendet :latest | +15 | Mutabler Tag, nicht reproduzierbar |
| Nicht per Digest gesperrt | +10 | Image-Inhalt kann stillschweigend ersetzt werden |
| Unbekannte Image-Registry | +10 | Kein Registry-Präfix, Standard Docker Hub |
| Alte Versionstags | +15 | Könnten bekannte Schwachstellen enthalten |
| Öffentliche Registry + nicht gesperrt | +5 | Keine Herkunftsgarantie |

**Rückgabeinhalte:** Risiko-Level pro Image (critical/high/medium/low), Statistik pro Registry, Top-Risiko-Images, Cluster-Image-Sicherheitsscore (0-100), umsetzbare Empfehlungen

### Kapazitätsplanung (v14.50+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/capacity/planning` | Knoten-Kapazitätsplanungsanalyse: CPU/Speicher-Anfragen vs. Zuweisbare pro Knoten, verbleibende Kapazität, empfohlener Skalierungszeitpunkt, Ressourcenfragmentierungserkennung |

### Cluster-Health-Score-Aggregation (v14.93+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/operations/health-score` | Aggregiert alle Cluster-Health-Signale zu einem Gesamtscore (0-100, Note A-F), kombiniert 5 gewichtete Dimensionen |

**5 gewichtete Dimensionen:**
| Dimension | Gewicht | Prüfinhalte |
|-----------|---------|------------|
| Node Health | 25% | Knoten-Bereitschaftsstatus |
| Pod Health | 25% | CrashLoop, Pending, Failed, hohe Restart-Anzahl |
| Workload Health | 20% | Deployment/StatefulSet/DaemonSet bereit-Repliken |
| Event Activity | 15% | Warning-Ereignisse der letzten Stunde |
| API Server | 15% | API-Server-Echtzeit-Latenzmessung |

**Rückgabeinhalte:** Gesamtscore 0-100, Buchstabennote A-F, Status (healthy/warning/critical), Detail-Score pro Dimension, Cluster-Zusammenfassung (Knoten/Pod/Workload-Anzahl), Top-Probleme nach Schweregrad sortiert

### HPA/VPA-Ressourcen-Rechtsizing-Empfehlungen (v14.94+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/scalability/autoscale-recommendations` | Analysiert HPA-Abdeckung und Ressourcen-Rechtsizing aller Workloads, erkennt Überprovisionierung, Multi-Replica-Workloads ohne HPA, HPA-Effizienz |

**Abfrageparameter:** `namespace` (optional)

**Prüfkategorien:**
| Prüfpunkt | Beschreibung |
|-----------|-------------|
| Multi-Replica-Workload ohne HPA | Auto-Scaling hinzufügen empfohlen |
| CPU-Request zu hoch (>1 Kern/Container) | Hohe Konfidenz, Halbierung empfohlen |
| Speicher-Request zu hoch (>2GB/Container) | Rightsizing empfohlen |
| HPA erreicht maxReplicas | Kapazität erhöhen erforderlich |
| HPA im Leerlauf (<20% Auslastung) | maxReplicas reduzieren empfohlen |

**Rückgabeinhalte:** Aktuelle vs. empfohlene CPU/Speicher-Werte pro Workload, Änderungsprozentsatz, Konfidenz, potenzielle CPU-Kern- und Speichereinsparungen, HPA-Effizienzanalyse, Cluster-Auto-Scaling-Score (0-100)

### Ingress- und Traffic-Routing-Health-Monitoring (v14.96+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/product/ingress-health` | Analysiert Traffic-Routing-Health und Konfigurationsprobleme aller Ingress-Ressourcen |

**Abfrageparameter:** `namespace` (optional)

**Prüfkategorien:**
| Prüfpunkt | Schweregrad | Beschreibung |
|-----------|------------|-------------|
| Backend-Service existiert nicht | critical | Referenzierter Service existiert nicht |
| Backend hat keine bereiten Endpoints | warning | Service hat keine ready Endpoints |
| Keine TLS-Konfiguration | warning | Host vorhanden aber unverschlüsselt |
| IngressClass existiert nicht | critical | Angegebene Klasse nicht bereitgestellt |
| host+path-Konflikt | warning | Mehrere Ingress konkurrieren um dieselbe Route |
| Keine Routing-Regeln | warning | Ingress ist wirkungslos |

**Rückgabeinhalte:** Status pro Ingress (healthy/warning/critical), Statistik pro Namespace, Cluster-Health-Score (0-100), umsetzbare Empfehlungen

### Knotenbedingungs- und Ressourcendruck-Analyse (v14.99+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/operations/node-pressure` | Analysiert Bedingungsstatus und Ressourcensättigung aller Knoten |

**Prüfkategorien:**
| Bedingung | Risikopunkte | Beschreibung |
|-----------|-------------|-------------|
| NetworkUnavailable | +30 | CNI/Netzwerk nicht bereit |
| DiskPressure | +25 | Festplatte voll oder fast voll |
| MemoryPressure | +25 | Knotenspeicher erschöpft |
| PIDPressure | +20 | Zu viele Prozesse |
| NotReady | →critical | kubelet/Runtime-Problem |
| CPU >90% | +20 | CPU-Request-Sättigung |
| Memory >95% | +20 | Speicher-Request-Sättigung |
| Cordoned | — | Nicht schedulable |

**Rückgabeinhalte:** Risiko-Level pro Knoten (critical/high/medium/low), CPU/Speicher/Pod-Auslastung, Bedingungsdetails (Grund, Nachricht, Dauer), Cluster-Druck-Score (0-100), umsetzbare Empfehlungen

### PVC-Bindungs- und Speicherleistungs-Analyse (v15.00+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/scalability/pvc-analysis` | Analysiert Bindungs-Health und Speicherleistung aller PVCs |

**Abfrageparameter:** `namespace` (optional)

**Prüfkategorien:**
| Prüfpunkt | Schweregrad | Beschreibung |
|-----------|------------|-------------|
| Festgefahrene PVC (>5min) | critical | Festgefahrene PVC + Ursachenanalyse |
| Lost PVC | critical | Zugrundeliegendes PV könnte gelöscht sein |
| Langsame Bindung (>30s) | warning | Speicher-Provisionierungsverzögerung |
| Pending PVC | warning | Wartet auf Bindung |
| Standard-StorageClass fehlt | info | Keine Standard-SC gesetzt |

**Rückgabeinhalte:** Status pro PVC (healthy/warning/critical), Bindungszeit, Statistik pro StorageClass, Festgefahrene-PVC-Ursachen, Cluster-Speicher-Health-Score (0-100)

### Namespace-Governance- und Lebenszyklus-Audit (v15.02+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/product/namespaces/lifecycle` | Auditiert Governance-Compliance und Lebenszyklus aller Namespaces |

**Governance-Prüfungen:**
| Prüfpunkt | Risikopunkte | Beschreibung |
|-----------|-------------|-------------|
| Kein ResourceQuota | +15 | Unbegrenzter Ressourcenverbrauch |
| Keine NetworkPolicy | +15 | Unbegrenzter Traffic |
| Kein LimitRange | +5 | Keine Standard-Ressourcenlimits |
| Namespace abgelaufen | +10 | Keine laufenden Pods, Bereinigungskandidat |
| Erforderliche Labels fehlen | +5 | app/team/env/owner fehlen |
| Nur Standard-SA | 0 | Least-Privilege-SA fehlt |

**Rückgabeinhalte:** Risiko-Level pro Namespace (critical/high/medium/low), Compliance-Flags, Lebenszyklusstatus (active/stale/terminating), Cluster-Governance-Score (0-100), umsetzbare Empfehlungen

### RBAC effektive Berechtigungen und Privilegieneskalations-Analyse (v15.04+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/security/rbac-effective` | Analysiert RBAC effektive Berechtigungen und Privilegieneskalationsrisiken aller Subjekte |

Aggregiert ClusterRoleBindings + RoleBindings und berechnet die tatsächlichen Berechtigungen jedes Subjekts (User/Group/ServiceAccount).

**Prüfkategorien:**

| Prüfpunkt | Risikopunkte | Beschreibung |
|-----------|-------------|-------------|
| cluster-admin-Äquivalent | →critical | Wildcard verbs + resources |
| Kann RBAC erstellen/ändern | +25 | Selbst-Eskalationspfad |
| Wildcard (*)-Berechtigungen | +20 | Übermäßige Berechtigung |
| Kann Secrets lesen | +10 | Sensible Daten-Exposition |
| Kann exec in Pods | +10 | Container-Escape-Einstiegspunkt |

**Rückgabeinhalte:** Risiko-Level pro Subjekt, Privilegieneskalationspfad-Details, Cluster-RBAC-Sicherheitsscore (0-100), umsetzbare Empfehlungen

### Container OOM-Kill-Tracker (v15.05+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/operations/oom-tracker` | Verfolgt Container-OOMKill-Ereignisse und Speicherkonfigurationsanalyse |

**Abfrageparameter:** `namespace` (optional)

**Prüfkategorien:**

| Prüfpunkt | Risikopunkte | Beschreibung |
|-----------|-------------|-------------|
| OOMKilled-Container | +15/Stk. | Wegen unzureichendem Speicher getötet |
| Hohe Restart-Anzahl (>=10) | +20 | CrashLoop-Indikator |
| Hohe Restart-Anzahl (>=5) | +10 | Häufige Neustarts |
| Kein Speicher-Limit | +5 | OOM-Verhalten unvorhersehbar |
| Niedriges Speicher-Limit (<256MB) | — | Könnte unnötige OOMs verursachen |
| Limit>>Request (10x+) | — | Knoten-Speicherdruck-Risiko |

**Rückgabeinhalte:** OOM-Risiko-Level pro Pod, Top-OOM-Ranking, Statistik pro Namespace, Cluster-OOM-Risiko-Score (0-100)

### Speicherkapazitäts-Erschöpfungs-Vorhersager (v15.06+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/scalability/storage-forecast` | Sagt voraus, wann Speicherkapazität erschöpft ist |

Basierend auf PV-Nutzungstrends und Wachstumsraten-Schätzungen wird der Speicherplatz-Erschöpfungszeitpunkt vorhergesagt.

**Analyse-Dimensionen:**

| Metrik | Beschreibung |
|--------|-------------|
| Kapazität vs. genutzt | Unterstützt Longhorn actual-size Annotation für echte Nutzung |
| Tägliche Wachstumsrate | Heuristische Schätzung basierend auf Auslastung und PV-Alter |
| Tage bis Erschöpfung | Verbleibender Speicher / tägliche Wachstumsrate |
| Vorhergesagtes Erschöpfungsdatum | Datum oder ">10 Jahre" oder "kein Wachstum" |
| Risiko-Level | critical(>95%) / high(>85% oder <14d) / medium(<30d) / low |

**Rückgabeinhalte:** Vorhersage pro PV, Cluster-Speicher-voll-Tage-Schätzung, Statistik pro StorageClass, Hochrisiko-Namespace-Ranking, Speicher-Health-Score (0-100)

### DNS-Auflösungs-Health-Checker (v15.08+)

| Methode | Pfad | Beschreibung |
|---------|------|-------------|
| GET | `/api/product/dns-health` | Analysiert Cluster-DNS-Auflösungs-Health-Status |

**CoreDNS-Analyse:**

| Prüfpunkt | Beschreibung |
|-----------|-------------|
| Pod-Health | running/ready/restarts/version pro Pod |
| Corefile | Forwarders, Plugins, fehlende Corefile-Erkennung |
| Replikanzahl | Empfohlen >= 2 für Hochverfügbarkeit |

**Weitere Erkennungen:**
- Headless-Service-Endpoint-Abdeckung (NXDOMAIN-Risiko)
- NodeLocal-DNS-Cache-Erkennung
- Pod dnsConfig ndots Überschreitungserkennung (>5 = zu viele DNS-Abfragen)
- External-DNS-verwaltete Service-Discovery

**Rückgabeinhalte:** CoreDNS-Pod-Status, Headless-Service-Abdeckung, DNS-Konfigurationsanalyse, Cluster-DNS-Health-Score (0-100), umsetzbare Empfehlungen
