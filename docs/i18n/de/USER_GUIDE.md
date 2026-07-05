# k8ops Benutzerhandbuch

> Von der Installation bis zur Beherrschung – ein ausführlicher Leitfaden, der alle Funktionen abdeckt.

---

## Inhaltsverzeichnis

1. [Schnellstart](#1-schnellstart)
2. [Clusterübersicht](#2-clusterübersicht)
3. [AI Chat — Intelligenter Assistent](#3-ai-chat--intelligenter-assistent)
4. [Diagnose und Behebung](#4-diagnose-und-behebung)
5. [Optimierungsempfehlungen](#5-optimierungsempfehlungen)
6. [Kostenanalyse (FinOps)](#6-kostenanalyse-finops)
7. [Cluster-Topologie-Visualisierung](#7-cluster-topologie-visualisierung)
8. [Knoten- und Pod-Verwaltung](#8-knoten-und-pod-verwaltung)
9. [Ereignisstream und Benachrichtigungen](#9-ereignisstream-und-benachrichtigungen)
10. [Ressourcenbrowser und YAML-Editor](#10-ressourcenbrowser-und-yaml-editor)
11. [RBAC-Zugriffskontrolle](#11-rbac-zugriffskontrolle)
12. [Audit-Protokoll](#12-audit-protokoll)
13. [Einstellungen und Konfiguration](#13-einstellungen-und-konfiguration)
14. [Tastenkombinationen](#14-tastenkombinationen)
15. [Designwechsel](#15-designwechsel)
16. [Kapazitätsplanung](#16-kapazitätsplanung)
17. [HPA-Visualisierung](#17-hpa-visualisierung)
18. [Container-Image-Inventar](#18-container-image-inventar)
19. [Namespace-Ressourcen-Rangliste](#19-namespace-ressourcen-rangliste)
20. [Sicherheitskonformität](#20-sicherheitskonformität)
21. [Systemverwaltung](#21-systemverwaltung)
22. [Betriebsdiagnose-API](#22-betriebsdiagnose-api-v1461)

---

## 1. Schnellstart

### Erstanmeldung

1. Öffnen Sie den Browser und rufen Sie die k8ops-Adresse auf (z. B. `https://k8ops.iot2.win` oder `http://localhost:9090`)
2. Standardkonto: `admin` / `admin`
3. Bei der ersten Anmeldung werden Sie aufgefordert, das Passwort zu ändern

### Seitenlayout

```
┌─────────┬───────────────────────────────┐
│         │  [Namespace ▼]  [🔔]  [☀/☽]  │  ← obere Leiste
│ Sidebar ├───────────────────────────────┤
│         │                                │
│ Overview│       Content Area             │  ← Inhaltsbereich
│ Diagnose│                                │
│ Nodes   │                                │
│ Pods    │                                │
│ ...     │                                │
└─────────┴───────────────────────────────┘
```

### Strg+K Befehlspalette

Drücken Sie jederzeit `Strg+K` (Mac: `Cmd+K`), um die globale Befehlspalette zu öffnen:

- `nodes` eingeben → zur Knotenseite wechseln
- `chat` eingeben → AI Chat öffnen
- `cost` eingeben → Kostenanalyse anzeigen
- Pfeiltasten zum Auswählen, Enter zum Bestätigen, Esc zum Schließen

---

## 2. Clusterübersicht

Die Overview-Seite zeigt den Gesamtstatus des Clusters.

### Statistikkarten

| Karte | Bedeutung |
|-------|-----------|
| Nodes | Gesamtzahl der Cluster-Knoten / Ready-Anzahl |
| Pods | Laufende Pods / Gesamtzahl |
| CPU | CPU-Auslastung des gesamten Clusters |
| Memory | Speicherauslastung des gesamten Clusters |
| Warnings | Aktuelle Anzahl an Warning-Ereignissen |

### Sparkline-Trenddiagramme

Unter jeder Karte befindet sich ein Mini-SVG-Liniendiagramm, das die Trendänderungen der letzten 30 Minuten anzeigt.

### Namespace-Wechsel

Die Dropdown-Auswahl oben links in der oberen Leiste ermöglicht das Wechseln des Namespace-Bereichs. Der Wechsel wirkt sich auf die Seiten Pods, Events, Nodes usw. aus. Die Auswahl wird in localStorage gespeichert.

---

## 3. AI Chat — Intelligenter Assistent

Klicken Sie auf die Chat-Schaltfläche unten in der Seitenleiste oder drücken Sie `Strg+K` und geben Sie `chat` ein, um ihn zu öffnen.

### Grundlegende Verwendung

Geben Sie Ihre Frage in das Eingabefeld ein, die KI wird:

1. Die Absicht in natürlicher Sprache verstehen
2. Automatisch das geeignete K8s-Tool aufrufen
3. Die Analyseergebnisse per Streaming zurückgeben

### Beispielabfragen

```
# Ressourcen anzeigen
Pods des Namespace default anzeigen
welche Knoten haben hohe CPU-Auslastung?

# Fehlerdiagnose
warum sind die Pods von nginx-deployment in CrashLoopBackOff?
welche Anomalien gibt es im Cluster?

# Optimierungsempfehlungen
hilf mir, die Ressourcennutzung zu analysieren
welche Pods können ihre Replikatanzahl reduzieren?
```

### Transparenz bei Tool-Aufrufen

Wenn die KI Tool-Aufrufe ausführt, wird ein einklappbares Thinking-Panel angezeigt:

- Zum Aufklappen klicken, um Parameter und Ergebnisse jedes Tool-Aufrufs zu sehen
- JSON-formatierte Darstellung, mit Suchunterstützung

### Diagnose-Empfehlungskarten

Wenn die KI vorschlägt, kubectl-Befehle auszuführen, wird unter dem Codeblock Folgendes angezeigt:

- **▶ Run in Chat** — lädt den Befehl in das Eingabefeld, um ihn einfach zu senden
- **📋 Copy** — kopiert den Befehl in die Zwischenablage

### Sitzungsverwaltung

- **New** — neue Sitzung erstellen
- **Sitzungsliste links** — Klicken zum Wechseln zwischen historischen Sitzungen
- Sitzungen werden automatisch zusammengefasst und komprimiert (wird automatisch bei über 20k Tokens ausgelöst)

### Markdown-Rendering

Chat unterstützt:
- Codeblöcke (mit Syntaxhervorhebung und Kopierbutton)
- Tabellen
- Listen, Fett, Kursiv
- Links (nur http/https/mailto-Protokolle)

---

## 4. Diagnose und Behebung

### Diagnose auslösen

**Methode 1: Web-Oberfläche**

1. Gehen Sie zur Diagnostics-Seite
2. Klicken Sie auf "New Diagnostic"
3. Füllen Sie die Problembeschreibung aus (z. B. "die API-Antwort im Namespace production ist langsam geworden")
4. Nach dem Absenden analysiert die KI automatisch

**Methode 2: AI Chat**

Beschreiben Sie das Problem direkt im Chat, die KI führt automatisch den Diagnoseablauf aus.

**Methode 3: CRD**

```bash
kubectl apply -f - <<EOF
apiVersion: aiops.ggai.dev/v1alpha1
kind: DiagnosticReport
metadata:
  name: check-nginx
  namespace: k8ops-system
spec:
  problem: "nginx pods keep restarting"
EOF
```

**Methode 4: CLI**

```bash
k8ops diagnose --problem "pods in production keep CrashLoopBackOff"
```

### Diagnoseergebnisse

Jeder Diagnosebericht enthält:

- **Root Cause** — die von der KI analysierte Grundursache
- **Evidence** — Logs, Ereignisse und Metrikdaten, die die Analyse stützen
- **Recommendations** — empfohlene Behebungsmaßnahmen
- **Severity** — Schweregrad (Info / Warning / Critical)

### Automatische Behebung (Remediation)

Die von der KI erstellten Behebungspläne erfordern eine manuelle Genehmigung:

1. Gehen Sie zur Remediations-Seite
2. Prüfen Sie die genehmigungspflichtigen Behebungspläne
3. Klicken Sie auf **Approve** zum Ausführen oder **Reject** zum Ablehnen
4. Alle Vorgänge werden im Audit-Protokoll aufgezeichnet

---

## 5. Optimierungsempfehlungen

Die Optimizations-Seite zeigt die Empfehlungen der KI zur Optimierung der Cluster-Ressourcen.

### Empfehlungstypen

| Typ | Beschreibung |
|-----|--------------|
| Resource Rightsizing | Vorschläge zur Anpassung von CPU/Memory-Requests und -Limits |
| HPA Gap | Deployments ohne Konfiguration für horizontale automatische Skalierung |
| PDB Gap | Arbeitslasten ohne PodDisruptionBudget |
| Cost Saving | Einsparbare Kosten (inaktive Ressourcen, übermäßige Replikate usw.) |

### Aktionen

- Auf eine Empfehlung klicken, um Details anzuzeigen
- Direkt anwenden oder ignorieren

---

## 6. Kostenanalyse (FinOps)

Die Cost-Seite bietet Kostentransparenz für das Cluster.

### Funktionen

- **Kostenzusammenfassung nach Namespace** — zeigt Ressourcenverbrauch und geschätzte Kosten pro Namespace
- **Ressourcenauslastung** — tatsächliche CPU/Memory-Nutzung vs. Zuweisung
- **Rightsizing-Empfehlungen** — Anpassungsvorschläge für überzogene Ressourcen
- **Inaktive Ressourcen** — lange ungenutzte PVs, LoadBalancer, elastische IPs usw.

---

## 7. Cluster-Topologie-Visualisierung

Die Topology-Seite stellt die Beziehung zwischen Knoten und Pods in SVG-Grafiken dar.

### Visuelle Elemente

| Element | Bedeutung |
|---------|-----------|
| Grüner Rahmen | Ready-Knoten |
| Roter Rahmen | NotReady-Knoten |
| Fortschrittsbalken im Knoten | CPU (oben) / MEM (unten) Auslastung |
| Grüner Pod-Punkt | Running |
| Gelber Pod-Punkt | Pending |
| Roter Pod-Punkt | Failed |
| Blinkender Pod-Rahmen | CrashLoop (restarts > 3) |

### Interaktion

- **Auf Pod klicken** — öffnet die Log-Anzeige für diesen Pod
- **Statistik unten** — Anzahl Ready/NotReady-Knoten, Zusammenfassung des Pod-Status

---

## 8. Knoten- und Pod-Verwaltung

### Nodes-Seite

- Knotenlistentabelle: Name, Rolle, Status, CPU, Speicher, Pod-Anzahl
- Jede Spalte unterstützt Suche und Filterung
- Auf Knotennamen klicken, um detaillierte Informationen und alle Pods auf diesem Knoten anzuzeigen

### Pods-Seite

- Pod-Listentabelle: Name, Namespace, Status, Neustartanzahl, Knoten, Alter
- Unterstützt Namespace-Filterung und Echtzeitsuche

### Pod-Log-Anzeige

Auf eine Pod-Zeile klicken, um die Log-Anzeige zu öffnen:

- **Echtzeit-Streaming** — über SSE, Logs werden in Echtzeit aktualisiert
- **Log-Level-Hervorhebung** — ERROR (rot), WARN (gelb), DEBUG (grau)
- **Suche und Filterung** — Schlüsselwörter eingeben, um Log-Zeilen zu filtern
- **Automatisches Scrollen** — bei neuen Logs automatisch nach unten scrollen (kann pausiert werden)
- **Download** — aktuelle Logs als Datei exportieren

---

## 9. Ereignisstream und Benachrichtigungen

### Events-Seite

Zeigt K8s-Cluster-Ereignisse an und unterstützt:

- Echtzeitsuche und Filterung
- Rote Hervorhebung von Warning-Ereignissen
- Filterung nach Namespace

### Echtzeit-Ereignisstream

Die rechte Seite der Events-Seite verfügt über ein Live-Events-Panel:

- Auf **Go Live** klicken, um SSE-Echtzeit-Push zu aktivieren
- Neue Ereignisse haben eine blaue NEW-Badge-Animation
- Gelöschte Ereignisse haben eine rote DEL-Badge
- Warning-Ereignisse werden automatisch rot hervorgehoben

### Benachrichtigungscenter

Das Glockensymbol oben rechts in der oberen Leiste:

- Zeigt eine rote Zahlen-Badge + Pulsenimation bei Alarmen
- Klicken zum Aufklappen des Dropdown-Panels
- Zeigt aktuelle Warning-Ereignisse und NotReady-Knoten
- Automatische Aktualisierung alle 60 Sekunden

---

## 10. Ressourcenbrowser und YAML-Editor

### Resources-Seite

Durchsuchen Sie alle K8s-Ressourcen im Cluster:

- Gruppiert nach API Group / Resource Type
- Auf Ressourcennamen klicken, um die YAML-Definition anzuzeigen
- Unterstützt Multi-Select-Filterung nach Namespace

### YAML-Anzeige

Auf eine beliebige Ressource klicken, um die YAML-Überlagerung zu öffnen:

- Formatierte Anzeige des vollständigen YAML
- **Copy**-Button zum Kopieren mit einem Klick

### YAML-Editor

Im YAML-Anzeiger auf die **Edit**-Schaltfläche klicken, um in den Bearbeitungsmodus zu wechseln:

1. Der YAML-Inhalt wird zu einer bearbeitbaren Textarea
2. Nach der Änderung auf **Apply** klicken, um zu senden
3. Das Backend verwendet Server-Side Apply (kubectl-apply-Semantik)
4. Bei Erfolg wird eine grüne Meldung angezeigt, bei Fehlern eine rote Fehlermeldung

---

## 11. RBAC-Zugriffskontrolle

Die RBAC-Seite (erfordert Administratorrechte) verwaltet Benutzer und Rollen.

### Benutzerverwaltung

- **Benutzer erstellen** — Benutzername, Passwort, Rolle, Namespace-Bereich
- **Benutzer bearbeiten** — Rolle ändern, aktivieren/deaktivieren
- **Benutzer löschen**

### Rollen

| Rolle | Berechtigungen |
|-------|----------------|
| admin | Lese-/Schreibzugriff auf das gesamte Cluster, kann Benutzer verwalten |
| operator | Lese-/Schreibzugriff auf die meisten Ressourcen, kann RBAC/Secrets nicht verwalten |
| viewer | Nur-Lese-Zugriff |

### Namespace-Bereich

Jeder Benutzer kann an einen bestimmten Namespace gebunden werden und nur auf Ressourcen innerhalb dieses Bereichs zugreifen (implementiert über K8s Impersonation).

---

## 12. Audit-Protokoll

Die Audit-Seite zeigt die Audit-Einträge aller KI-Vorgänge.

### Funktionen

- **Severity-Filter** — Dropdown zur Auswahl von Info / Warning / Error / Critical
- **Echtzeitsuche** — Schlüsselwörter eingeben zum Filtern
- **Statistikkarten** — Total / Successful / Failed / Critical / Warnings
- **Tabelle** — Zeit, Schweregrad, Aktion, Zielressource, Akteur, Erfolg/Fehlschlag, Dauer

### Audit-Umfang

Alle folgenden Vorgänge werden protokolliert:

- KI-Tool-Aufrufe (kubectl get/describe/logs usw.)
- Von der KI initiierte Behebungsvorgänge
- LLM-API-Aufrufe
- Benutzeranmeldung/-abmeldung
- Ressourcenänderungen

---

## 13. Einstellungen und Konfiguration

Die Settings-Seite konfiguriert den KI-Anbieter und die Authentifizierung.

### KI-Anbieterkonfiguration

| Feld | Beschreibung |
|------|--------------|
| Provider Type | openai / deepseek / zai / anthropic |
| Model | gpt-4o / deepseek-chat / glm-4-plus usw. |
| Endpoint | Adresse der LLM-API (leer lassen für Standardwert) |
| API Key | Schlüssel der LLM-API |

### Authentifizierungskonfiguration

- **Local** — integriertes Benutzersystem (Standard)
- **LDAP** — LDAP/AD-Unternehmensintegration
- **OIDC** — GitHub / Google / Keycloak usw.

---

## 14. Tastenkombinationen

| Tastenkombination | Funktion |
|-------------------|----------|
| `Strg+K` / `Cmd+K` | Befehlspalette öffnen |
| `Esc` | Befehlspalette / Popups schließen |
| `↓` / `↑` | In der Befehlspalette auswählen |
| `Enter` | In der Befehlspalette bestätigen |

---

## 15. Designwechsel

Klicken Sie auf die Mond/Sonne-Schaltfläche oben rechts in der Seitenleiste, um zwischen dunklem/hellem Design zu wechseln. Die Auswahl wird in localStorage gespeichert und nach Aktualisierung der Seite beibehalten.

---

## Anhang

### Verwandte Dokumentation

- [Architekturdesign](ARCHITECTURE.md)
- [Bereitstellungshandbuch](DEPLOYMENT.md)
- [Lokale Ausführung](LOCAL_RUN.md)
- [API-Referenz](API.md)
- [Sicherheitsrichtlinie](SECURITY.md)

### Häufig gestellte Fragen

**F: Chat reagiert nicht?**
A: Überprüfen Sie, ob die Konfiguration unter Settings → Provider korrekt ist und ob der API Key gültig ist.

**F: Ich kann bestimmte Namespaces nicht sehen?**
A: Die RBAC-Rolle des aktuellen Benutzers kann den Namespace-Zugriffsbereich einschränken. Wenden Sie sich an den Administrator zur Anpassung.

**F: Die Pod-Log-Anzeige ist leer?**
A: Der Pod wurde möglicherweise gerade gestartet und hat noch keine Logs, oder es fehlen Log-Berechtigungen. Überprüfen Sie die RBAC-Konfiguration.

**F: Sind die von der KI vorgeschlagenen Befehle sicher?**
A: Alle von der KI vorgeschlagenen Vorgänge durchlaufen zuerst eine Dry-Run-Validierung des Safety Checkers. Behebungsvorgänge erfordern eine manuelle Genehmigung vor der Ausführung.

---

## 16. Kapazitätsplanung

### Speicherkapazitätsüberwachung

**Pfad:** Dashboard → Registerkarte Capacity

Zeigt den Speicherstatus aller PVCs (PersistentVolumeClaim) im Cluster:

| Metrik | Beschreibung |
|--------|--------------|
| Total PVCs | Gesamtzahl der PVCs im Cluster |
| Bound | Anzahl der an ein PV gebundenen PVCs |
| Pending | Auf Bindung wartende PVCs |
| Total Capacity | Gesamtkapazität aller PVCs |
| Requested | Gesamt angeforderte Menge aller PVCs |

### Knoten-Kapazitätsanalyse

Die Capacity-Seite zeigt auch die Ressourcenauslastung jedes Knotens:

- **CPU-Auslastung**: angeforderte CPU / zuweisbare CPU (Farbcodierung: <60% grün, 60-80% gelb, >80% rot)
- **Speicherauslastung**: angeforderter Speicher / zuweisbarer Speicher
- **Pod-Dichte**: Anzahl laufender Pods / maximale Pod-Grenze
- **Skalierungsempfehlungen**: Wenn Knotenressourcen 80% überschreiten, werden automatisch Skalierungsempfehlungen generiert

### Clusterweite Zusammenfassung

| Metrik | Beschreibung |
|--------|--------------|
| Cluster CPU Utilization | Verhältnis angeforderte/zuweisbare CPU im gesamten Cluster |
| Cluster Mem Utilization | Verhältnis angeforderter/zuweisbarer Speicher im gesamten Cluster |
| Total CPU Allocatable | Gesamt zuweisbare CPU-Menge im Cluster |
| Total CPU Requested | Gesamt angeforderte CPU-Menge im Cluster |

---

## 17. HPA-Visualisierung

**Pfad:** Dashboard → Registerkarte HPA

Zeigt den automatischen Skalierungsstatus aller HorizontalPodAutoscaler:

### Funktionen

- **Replikatskalierungsleiste**: visualisiert aktuelle Replikatanzahl, gewünschte Replikatanzahl und min/max-Bereich
- **Metrik-Auslastungsleiste**: aktuelle CPU/Speicher-Auslastung vs. Zielwert (grün/gelb/rot)
- **Skalierungsstatus-Anzeige**: zeigt ein "SCALING"-Badge, wenn aktuelle Replikate ≠ gewünschte Replikate
- **Zusammenfassungskarten**: Gesamtzahl der HPAs, Anzahl in Skalierung, Gesamtzahl aktueller/gewünschter Replikate

### Unterstützte Metriktypen

| Typ | Beschreibung |
|-----|--------------|
| Resource | CPU/Speicher-Auslastung in Prozent |
| Pods | Benutzerdefinierte Pod-Metriken (wie QPS) |
| External | Externe Metriken (wie SQS-Warteschlangenlänge) |
| ContainerResource | Ressourcenmetriken auf Containerebene |

---

## 18. Container-Image-Inventar

**Pfad:** Dashboard → Registerkarte Images

Zeigt alle im Cluster verwendeten Container-Images:

| Metrik | Beschreibung |
|--------|--------------|
| Unique Images | Gesamtzahl der eindeutigen (deduplizierten) Images |
| Using :latest | Anzahl der Images mit `:latest`-Tag (nicht für Produktion empfohlen) |
| No Limits | Anzahl der Images ohne Resource-Limits |
| No Requests | Anzahl der Images ohne Resource-Requests |
| Registries | Anzahl der verwendeten Image-Registries |

### Sicherheits-Best-Practices

- Vermeiden Sie die Verwendung des `:latest`-Tags — verwenden Sie feste Versionsnummern für reproduzierbare Bereitstellungen
- Alle Container sollten CPU/Speicher-Limits haben — um Ressourcenerschöpfung zu verhindern
- Alle Container sollten CPU/Speicher-Requests haben — um eine korrekte Zuweisung durch den Scheduler zu gewährleisten

---

## 19. Namespace-Ressourcen-Rangliste

**Pfad:** Dashboard → Registerkarte Namespaces

Listet die Ressourcennutzung aller Namespaces nach CPU-Verbrauch sortiert auf:

### Funktionen

- **Ressourcenzusammenfassung**: CPU/Speicher-Requests + -Limits, Pod-Anzahl, PVC-Speicher für jeden Namespace
- **Cluster-Anteil**: Prozentsatz der angeforderten CPU/Speicher bezogen auf die insgesamt zuweisbare Menge (mit visueller Fortschrittsleiste)
- **Suche und Filterung**: schnelles Auffinden eines bestimmten Namespaces
- **Detail-Drilldown**: auf einen Namespace klicken, um ResourceQuota-Nutzung, LimitRange-Konfiguration und aktuelle Warning-Ereignisse anzuzeigen

---

## 20. Sicherheitskonformität

### CIS Benchmark-Konformitätsprüfung

**Pfad:** Dashboard → Registerkarte Compliance

Führt CIS Kubernetes Benchmark-Prüfungen durch, die folgende Kategorien abdecken:

| Kategorie | Prüfpunkte |
|-----------|------------|
| RBAC | cluster-admin-Bindungsumfang, Wildcard-ClusterRole, Verwendung des Standard-SA |
| Pod Security | Privilegierte Container, hostNetwork/hostPID/hostIPC, hostPath-Volumes, Root-Benutzer, Resource-Limits |
| Network | NetworkPolicy-Abdeckung |
| Secrets | Secret-Verwaltungsintegrität |

### Konformitätsbericht herunterladen

Klicken Sie auf die Schaltfläche "Download Report", um den vollständigen Konformitätsbericht (.txt-Format) herunterzuladen, der Folgendes enthält:

- Konformitätsbewertung (Prozent)
- Status jeder Prüfung (PASS/WARN/FAIL)
- Behebungsempfehlungen (für WARN/FAIL-Elemente)

### Audit-Ereignissuche

**Pfad:** API → `GET /api/audit/events`

Unterstützt mehrdimensionale Filterung des Audit-Protokolls:

| Parameter | Beschreibung |
|-----------|--------------|
| `actor` | Nach Benutzername filtern |
| `action` | Nach Vorgangstyp filtern (wie delete, scale, exec) |
| `q` | Volltextsuche |
| `severity` | Nach Schweregrad filtern |
| `from`/`to` | Zeitbereich (RFC3339-Format) |

### CSV-Export

`GET /api/audit/export` — exportiert Audit-Logs im CSV-Format, das in ein SIEM-System zur Konformitätsanalyse importiert werden kann.

---

## 21. Systemverwaltung

### Systeminformationen

`GET /api/system/info` bietet Laufzeitinformationen:

- Versionsnummer, Go-Version, Ausführungsplattform
- Speichernutzung (Alloc/Sys/GC cycles/Heap objects)
- Anzahl der Goroutines
- Service-Laufzeit
- Größe und Ereignisanzahl des Audit-Logs

### Log-Verwaltung

| API | Funktion |
|-----|----------|
| `POST /api/system/log/rotate` | Manuelles Auslösen der Audit-Log-Rotation (admin) |
| `POST /api/system/log/cleanup` | Bereinigen von Rotationsdateien älter als 30 Tage (admin) |

### Log-Level-Konfiguration

Konfiguration über die Umgebungsvariable `LOG_LEVEL` (debug/info/warn/error):

```bash
kubectl set env daemonset/k8ops -n k8ops-system LOG_LEVEL=debug
kubectl rollout restart daemonset/k8ops -n k8ops-system
```

### Backup-Verwaltung

| API | Funktion |
|-----|----------|
| `GET /api/system/backup` | Alle Backup-Dateien auflisten |
| `POST /api/system/backup` | Datenbank-Backup erstellen |
| `DELETE /api/system/backup?name=X` | Bestimmtes Backup löschen |
| `POST /api/system/backup/restore?name=X` | Datenbank aus Backup wiederherstellen |

### API-Leistungsüberwachung

`GET /api/system/performance` bietet Latenzstatistiken für jeden API-Endpunkt:

- **p50/p95/p99** Perzentil-Latenz
- Durchschnittliche und maximale Latenz
- Fehlerrate und Gesamtanforderungsanzahl

---

## 22. Betriebsdiagnose-API (v14.61+)

### Network Policy-Audit

`GET /api/security/network-policies` prüft die NetworkPolicy-Abdeckung des Clusters:

- Erkennung von Namespaces ohne NetworkPolicy (standardmäßig vollständig geöffnet)
- Identifizierung von großzügigen Richtlinien (0.0.0.0/0 ein-/ausgehend)
- Klassifizierung nach Schweregrad: critical / warning / info
- Jeder Befund enthält eine detaillierte Beschreibung und Behebungsempfehlungen

### Pod-Neustart-Diagnose

`GET /api/diagnostics/restarts` diagnostiziert Pod-Neustartmuster und Grundursachen:

- Klassifizierung der Neustartmuster: crash-loop / occasional / post-deploy
- Extraktion der Beendigungsursache: OOMKilled / Error / Exit-Code
- Identifizierung von Wartezuständen: CrashLoopBackOff / ImagePullBackOff
- Unabhängige Diagnoseinformationen für jeden Container

### Deployment-Rollout-Status

`GET /api/deployments/rollout` verfolgt den Rollout-Integritätsstatus aller Arbeitslasten:

- Umfasst Deployment / StatefulSet / DaemonSet
- 7 Zustände: complete / in-progress / stalled / degraded / paused / failed / scaled-to-zero
- Erkennung von ProgressDeadlineExceeded und ReplicaFailure
- Unterstützt Filterung nach Status: `?status=failed`

### Ressourcenverschwendung-Erkennung

`GET /api/resources/waste` scannt verschwendete und verwaiste Ressourcen zur Kostensenkung:

- 6 Verschwendungskategorien: tote Services, ungenutzte PVCs, verwaiste ConfigMaps/Secrets, leere Namespaces, ungebundene PVs
- Kostenrisikobewertung: low / moderate / high
- Jedes Element umfasst Schweregrad, Alter und Bereinigungsempfehlungen
- Intelligente Filterung von Systemressourcen (kube-system, SA-Token, Helm-Release)

### Skalierungs-Engpass-Erkennung

`GET /api/scaling/bottlenecks` identifiziert Faktoren, die die horizontale Skalierung einschränken:

- 7 Engpasskategorien: Knotenplanung, Knotendruck, Kontingentgrenzen, feststeckendes HPA, blockierendes PDB, Speicherverschwendung
- Cluster-Kapazitätszusammenfassung: Knotenanzahl, CPU/Speicher, Pod-Kapazität, Skalierungsspielraum
- Jedes Element umfasst Auswirkungsstufe und Behebungsempfehlungen

### RBAC-Berechtigungsrisikoanalyse

`GET /api/security/rbac-risk` analysiert die Sicherheitsrisiken aller RBAC-Bindungen im Cluster:

- Bewertungssystem 0-100, identifiziert automatisch Hochrisiko-Bindungen
- 5 Risikostufen: critical / high / elevated / moderate / low
- Prüfpunkte: cluster-admin-Bindungen, Privilegieneskalation (escalate/bind/impersonate), Wildcard-Berechtigungen (verbs/resources: *), clusterweiter Schreibzugriff, Zugriff auf sensible Ressourcen (secrets/pods/exec)
- Jedes Element enthält detaillierte Bewertungsaufschlüsselung und Behebungsempfehlungen (Prinzip der minimalen Rechte)
- Unterstützt Filterung nach Namespace: `?namespace=default`

### CronJob-Ausführungsintegritätsüberwachung

`GET /api/operations/cronjobs/health` überwacht die Ausführungsintegrität aller CronJobs:

- 5 Integritätsstufen: healthy / warning / failing / suspended / no-runs
- Erkennung aufeinanderfolgender Fehler (3 oder mehr = failing), Erfolgsquote unter 50%, ausgesetzte Zeitpläne, nie ausgeführt
- Verknüpfung von CronJobs und ihren untergeordneten Jobs über OwnerReferences
- Berechnung der nächsten erwarteten Ausführungszeit
- Unterstützt Filterung nach Namespace: `?namespace=production`

### Service- und Endpoint-Netzwerkintegritätsüberwachung

`GET /api/networking/health` scannt die Netzwerkverbindungen aller Services und Ingress:

- 5 Service-Integritätsstufen: healthy / degraded / no-endpoints / misconfigured / external
- Erkennung nicht übereinstimmender Selektoren (label mismatch), alle Endpunkte nicht verfügbar, teilweise Beeinträchtigung, LoadBalancer wartet auf IP
- Ingress-Backend-Validierung: Existenz des Backend-Services und Verfügbarkeit von Endpunkten
- Querverweis der Pod-Selektor-Übereinstimmung mit Ursachenanalyse
- Unterstützt Filterung nach Namespace: `?namespace=default`

### PV/PVC-Speicherintegritätsüberwachung

`GET /api/storage/health` scannt die Speicherintegrität aller PVCs/PVs:

- 6 PVC-Integritätsstufen: bound / pending / lost / failed / orphaned / near-capacity
- Pending-Diagnose: keine Storage-Klasse, WaitForFirstConsumer-Bindungsmodus, Provisioner-Log-Prüfung
- Erkennung verwaister PVCs: gebunden, aber seit über 1 Tag von keinem Pod verwendet (Kapazitätsverschwendung)
- PV-Probleme: Released (manuelle Bereinigung erforderlich), Failed (Recycling fehlgeschlagen), veraltete Available-PVs (>7 Tage)
- StorageClass-Verteilung: Standardklasse, Provisioner, Reclaim-Policy, Volume-Expansion-Unterstützung
- Unterstützt Filterung nach Namespace: `?namespace=default`

### ServiceAccount-Sicherheitsaudit

`GET /api/security/service-accounts` überwacht umfassend die Sicherheitsrisiken aller ServiceAccounts im Cluster:

- Risikobewertungssystem 0-100, identifiziert automatisch Hochrisiko-SAs
- 5 Schweregradstufen: critical / high / elevated / moderate / low
- Prüfpunkte: ungenutzte SAs (>7 Tage), cluster-admin-Bindung (critical), Standard-SA von Pods verwendet, unnötige automatische Token-Einhängung, veraltete SAs (>30 Tage mit Berechtigungen aber ungenutzt), veraltete langlebige Token-Secrets
- Jedes Element enthält eine detaillierte Sicherheitsrisikoerklärung und Behebungsempfehlungen
- Unterstützt Filterung nach Namespace: `?namespace=default`

### SLO/SLA-Fehlerbudget-Verfolgung

`GET /api/operations/slo` verfolgt die SLO/SLA-Einhaltung basierend auf einem Multi-Fenster-Multi-Burn-Rate-Algorithmus:

- 5 Zeitfenster: 5 Minuten, 1 Stunde, 6 Stunden, 24 Stunden, 7 Tage
- Verfügbarkeitsprozentsatz und verbleibende Menge/Verbrauchsrate des Fehlerbudgets
- Multi-Fenster-Burn-Rate-Erkennung (fast: 5m+1h, slow: 6h+24h)
- P50/P95/P99 Latenzperzentile und SLO-Ziel
- 3 Statusstufen: meeting (eingehalten) / at-risk (gefährdet) / violated (verletzt)
- Unterstützt Filterung nach Namespace: `?namespace=production`

### ResourceQuota- und LimitRange-Überwachung

`GET /api/resources/quota` scannt die Kontingentnutzung und LimitRange-Beschränkungen aller Namespaces:

- 4 Kontingentstatus: ok (<70%) / warning (70-85%) / critical (85-100%) / exceeded (>100%)
- CPU/Speicher/Pod/ConfigMap/Secret/Speicher-Kontingentnutzung pro Namespace
- Identifizierung von Namespaces ohne Kontingentschutz
- Analyse der Standard-/Minimum-/Maximum-Beschränkungen von LimitRange
- Rangliste der Top-Verbraucher
- Unterstützt Filterung nach Namespace: `?namespace=default`

### Deployment-Konfigurationsaudit

`GET /api/deployments/audit` überwacht Verstöße gegen Best Practices bei der Konfiguration aller Arbeitslasten:

- 8 Prüfkategorien: revision-history / image-policy / resources / probes / security-context / update-strategy / lifecycle / config-drift
- Jedes Element umfasst Schweregrad (critical/warning/info), spezifische Problembeschreibung und umsetzbare Behebungsempfehlungen
- Integritätsbewertung von 0 (perfekt) bis 100 (schlechtestes)
- Aggregierte Top-Erkenntnisse, die die häufigsten clusterweiten Probleme anzeigen
- Unterstützt Filterung nach Namespace und Schweregrad: `?namespace=default&severity=critical`

### Scheduling-Integrität und Ressourcenfragmentierungsanalyse

`GET /api/scheduling/health` analysiert die Scheduling-Integrität des Clusters und die Ressourcenauslastung:

- Schedulbarkeit pro Knoten (Isolierung/Taint/Druckbedingungen) und Ressourcenverfügbarkeit
- Pending-Pod-Diagnose: Analyse der Ursachen von FailedScheduling-Ereignissen (unzureichende CPU/Speicher, Taint-Nichtübereinstimmung, nodeSelector-Konflikt, Volume-Bindungsfehler usw.)
- Berechnung des maximal planbaren Pods (der größte derzeit bereitstellbare Pod)
- Effektive Kapazität vs. theoretische Kapazität (Kapazitätsverlust durch nicht planbare Knoten)
- Ressourcenfragmentierungsanalyse (verstreute freie Kapazität)
- Erkennung übergroßer Pods (Anfragen übersteigen die Kapazität eines einzelnen Knotens)
- 24h-Evictionsverlauf (mit Ursachen)
- Integritätsbewertung 0-100 (gewichtete Strafe)
- Umsetzbare Behebungsempfehlungen
- Unterstützt Filterung nach Namespace: `?namespace=default`

### Pod-Sicherheitshaltungs-Scan

`GET /api/security/pods?namespace=xxx&severity=critical` überwacht die Echtzeit-Sicherheitshaltung aller laufenden Pods:

- 15 Prüfkategorien, die privilegierte Container, Host-Zugriff (network/PID/IPC), HostPath-Einhängungen, gefährliche Capabilities, Root-Ausführung, Privilegieneskalation usw. abdecken
- Risikobewertung pro Pod von 0-100 (critical=25 Punkte/warning=8 Punkte/info=2 Punkte)
- Aggregierte Statistiken nach Prüftyp und Namespace
- Unterstützt Filterung nach Namespace und Schweregrad

### Ereignissturm- und Kaskadenausfall-Erkennung

`GET /api/operations/event-storm?namespace=xxx` analysiert die Warning-Ereignisse des Clusters:

- 4 Sturmschweregrade: critical (>50) / high (>20) / medium (>10) / low (>5)
- Erkennung fluktuierender Ressourcen (gleiche Ressource, gleiche Ursache 3+ Mal wiederholt, mit Fluktuationsfrequenz)
- Aggregation nach Namespace und Ereignisursache
- Bewertung des Auswirkungsradius (Anzahl betroffener Ressourcen)
- Umsetzbare Untersuchungsempfehlungen
- Unterstützt Filterung nach Namespace: `?namespace=kube-system`

### Ressourcen-Abhängigkeitsdiagramm und Auswirkungsanalyse

`GET /api/dependencies?kind=Deployment&name=xxx&namespace=xxx` verfolgt das vollständige Abhängigkeitsdiagramm von Arbeitslasten:

- Vorwärtsabhängigkeiten: ConfigMap, Secret, PVC, ServiceAccount
- Rückwärtsabhängigkeiten: Service (Label-Selektor), Ingress, NetworkPolicy, HPA, andere Pods mit gemeinsamer Konfiguration
- Bewertung des Auswirkungsbereichs: blastRadius-Bewertung und Risikostufe
- Nützlich für die Auswirkungsbewertung vor Änderungen, um Kaskadenausfälle zu vermeiden

### Topologie-Verteilungs-Konformitätsprüfung

`GET /api/topology/spread?namespace=xxx&domain=topology.kubernetes.io/zone` überprüft die Konformität der Topologieverteilung von Pods:

- 4 Arbeitslaststatus: balanced / skewed / no-constraint / single-replica
- Analyse der Topologie-Domänenverteilung und Abweichung pro Arbeitslast
- Erkennung von Arbeitslasten mit mehreren Replikaten ohne Topologie-Einschränkungen
- Identifizierung von Knoten ohne Topologie-Labels
- Hinweise für Single-Domain-Cluster
- Unterstützt Filterung nach Namespace und Topologie-Domänen-Schlüssel

### Secret-Rotation und Lebenszyklus-Audit

`GET /api/security/secrets/rotation?namespace=xxx` überwacht den Lebenszyklus aller Secrets:

- Altersverfolgung: stale (>90T) / very stale (>180T)
- Erkennung ungenutzter Secrets (von keinem Pod referenziert)
- TLS-Zertifikatsablauf-Erkennung (Zertifikatsanalyse, Erkennung abgelaufener und <30T vor Ablauf)
- Verfolgung von Docker-Registry-Secrets, veralteten SA-Tokens
- Erkennung sensibler Namen (password/key/token/credential)
- Risikostufe pro Secret, Cluster-Rotationsbewertung 0-100
- Unterstützt Filterung nach Namespace

### Health-Probe-Wirksamkeitsaudit

`GET /api/operations/probes?namespace=xxx` überwacht die Sondenkonfiguration:

- 8 Prüfkategorien: fehlende Sonden, zu aggressiv, Timeout zu kurz, unangemessene Schwellenwerte usw.
- Risikobewertung pro Arbeitslast, Cluster-Wirksamkeitsbewertung (0-100)
- Aggregierte Top-Problemstatistiken
- Umsetzbare Empfehlungen

### Workload-Überalterungsnachverfolgung

`GET /api/product/staleness?namespace=xxx` verfolgt die Überalterung von Deployments:

- 5 Überaltungsstufen: fresh/recent/stale/very-stale/ancient
- Image-Tag-Analyse: :latest, Digest, ohne Tag
- Altersverteilungs-Buckets, Namespace-Statistiken
- Cluster-Frischheitsbewertung (0-100)

### Ressourcen-Overcommit- und Druckanalyse

`GET /api/scalability/overcommit?namespace=xxx` analysiert den Ressourcen-Overcommit:

- CPU/Speicher-Request- und -Limit-Overcommit-Raten pro Knoten
- Druckbewertung 0-100 und Risikostufe
- Erkennung von Pods ohne Limits/Requests
- Cluster-Sicherheitsbewertung 0-100
- Ressourcenverbrauchsaufschlüsselung nach Namespace

### Image-Sicherheits- und Lieferkettenanalyse

`GET /api/security/images?namespace=xxx` scannt die Lieferkettensicherheit aller Container-Images:

- Digest-Lock-Erkennung (@sha256: unveränderliche Referenz)
- :latest-Tag-Erkennung (veränderlich, nicht reproduzierbar)
- Erkennung von Images ohne Tag (Standard :latest)
- Erkennung alter Versionstags (v1, 1.0 — können bekannte CVEs enthalten)
- Analyse öffentlicher vs. privater Image-Registries
- Risikostufe pro Image, Statistiken pro Registry
- Cluster-Image-Sicherheitsbewertung 0-100

### Kapazitätsplanung

`GET /api/capacity/planning` Knoten-Kapazitätsplanung:

- CPU/Speicher-Requests vs. Zuweisbares pro Knoten
- Verbleibende Kapazität und Skalierungsempfehlungen
- Ressourcenfragmentierungserkennung

### Kapazitätsprognose

`GET /api/capacity/forecast` Kapazitätstrendprognose:

- Ressourcenwachstumstrends basierend auf historischen Daten
- Geschätzte Erschöpfungszeit
- Skalierungsempfehlungen

### Ressourceneffizienzanalyse

`GET /api/efficiency` Ressourcennutzungseffizienz:

- Erkennung übermäßiger Ressourcenzuweisung
- Identifizierung von Ressourcenverschwendung
- Optimierungsempfehlungen

### PDB-Status

`GET /api/pdbs` Pod Disruption Budget-Status:

- PDB-Konfigurationsprüfung
- Erlaubte Störungen vs. aktuelle Verfügbarkeit
- Erkennung blockierender PDBs

### Versionskompatibilität

`GET /api/compatibility` K8s-Versionskompatibilität:

- Prüfung veralteter APIs
- Ressourcen-Versionskompatibilität
- Bewertung des Upgrade-Einflusses

### Zertifikatsablauf

`GET /api/certificates/expiry` TLS-Zertifikatsablauf-Scan:

- Ablaufzeit der Cluster-Zertifikate
- Warnung für bald ablaufende Zertifikate
- Verlängerungsempfehlungen

### Addon-Integrität

`GET /api/addons/health` Cluster-Addon-Integritätsprüfung:

- Ausführungsstatus der Kern-Addons
- Erkennung anormaler Addons
- Behebungsempfehlungen

### Cluster-Integritätsbewertung

`GET /api/operations/health-score` aggregiert alle Cluster-Integritätssignale zu einer Gesamtbewertung:

- 5 gewichtete Dimensionen: Node(25%) + Pod(25%) + Workload(20%) + Events(15%) + API Server(15%)
- Gesamtbewertung 0-100, Buchstabennote A-F
- Status: healthy / warning / critical
- Bewertung, Gewicht und Details pro Dimension
- Cluster-Zusammenfassung: Knoten/Pod/Arbeitslast-Zähler
- Top-Probleme sortiert nach Schweregrad

### HPA/VPA-Ressourcen-Konfigurationsempfehlungen

`GET /api/scalability/autoscale-recommendations?namespace=xxx` analysiert automatische Skalierung und Ressourcen-Right-Sizing:

- Erkennung von Arbeitslasten mit mehreren Replikaten ohne HPA
- Überhöhte CPU-Requests (>1 Core/Container)
- Überhöhte Speicher-Requests (>2GB/Container)
- HPA-Effizienzanalyse (erreicht oberes/unteres Limit/inaktiv)
- Aktuelle vs. empfohlene Ressourcenwerte pro Arbeitslast
- Potenzielle CPU-Core- und Speicherersparnis
- Cluster-Autoskalierungsbewertung 0-100

### Ingress- und Traffic-Routing-Integritätsüberwachung

`GET /api/product/ingress-health?namespace=xxx` überprüft die Traffic-Routing-Integrität aller Ingress:

- Überprüfung der Backend-Service-Existenz und Endpoint-Verfügbarkeit
- TLS-Konfigurationserkennung
- IngressClass-Validierung
- host+path-Konflikterkennung
- Erkennung fehlender Routing-Regeln
- Status pro Ingress und Cluster-Integritätsbewertung 0-100

### Knotenbedingungen und Ressourcendruck

`GET /api/operations/node-pressure` analysiert die Bedingungen und den Ressourcendruck aller Knoten:

- Erkennung von DiskPressure / MemoryPressure / PIDPressure / NetworkUnavailable
- CPU/Speicher/Pod-Nutzung vs. Zuweisbares
- Risikostufe pro Knoten (critical/high/medium/low)
- Cluster-Druckbewertung 0-100

### PVC-Bindung und Speicherleistung

`GET /api/scalability/pvc-analysis?namespace=xxx` analysiert die Speicherbindungsintegrität:

- Erkennung der Grundursache feststeckender PVCs (>5min pending)
- Messung der Bindungszeit und Erkennung langsamer Bindungen (>30s)
- Erkennung verlorener PVCs
- Statistiken pro StorageClass und Provisioner-Analyse
- Cluster-Speicherintegritätsbewertung 0-100

### Namespace-Governance und Lebenszyklus

`GET /api/product/namespaces/lifecycle` überwacht die Namespace-Governance:

- ResourceQuota / LimitRange / NetworkPolicy-Abdeckung
- Erkennung dedizierter ServiceAccounts (minimale Rechte)
- Prüfung erforderlicher Labels (app, team, env, owner)
- Namespace-Lebenszyklus (active / stale / terminating)
- Cluster-Governance-Bewertung 0-100

### RBAC effektive Berechtigungen und Eskalationsanalyse

`GET /api/security/rbac-effective` analysiert die effektiven RBAC-Berechtigungen aller Subjekte:

- Aggregation von ClusterRoleBindings + RoleBindings zur Berechnung der tatsächlichen Berechtigungen
- Erkennung von cluster-admin-Äquivalenz
- Erkennung von Eskalationspfaden (Subjekte, die RBAC ändern können)
- Erkennung von Wildcard-Berechtigungen (*)
- Analyse des Secret-Lese- und Pod-Exec-Zugriffs
- Cluster-RBAC-Sicherheitsbewertung 0-100

### Container-OOM-Kill-Verfolgung

`GET /api/operations/oom-tracker?namespace=xxx` verfolgt Container-OOM-Ereignisse:

- Erkennung von OOMKilled-Containern und Ursachenanalyse
- Erkennung hoher Neustartanzahlen (>=5)
- Erkennung fehlender oder zu niedriger Speicher-Limits
- Knotendruckrisiko durch Limits, die Requests weit übersteigen (10x+)
- Top-OOM-Rangliste und Statistiken pro Namespace
- Cluster-OOM-Risikobewertung 0-100

### Speicherkapazitäts-Erschöpfungsprognose

`GET /api/scalability/storage-forecast` prognostiziert die Speicherkapazität:

- Nutzung pro PV, Wachstumsrate, Vorhersage der Tage bis zur Erschöpfung
- Unterstützung der Longhorn actual-size-Annotation
- Schätzung der Tage bis zur Cluster-Füllung
- Statistiken pro StorageClass und Provisioner-Analyse
- Rangliste der Hochrisiko-Namespaces
- Speicherintegritätsbewertung 0-100

### DNS-Auflösungsintegritätsprüfung

`GET /api/product/dns-health` analysiert die DNS-Auflösungsintegrität:

- CoreDNS-Pod-Integritätsprüfung (Ausführung/Bereitschaft/Neustarts/Version)
- Corefile-Konfigurationsanalyse (Forwarders, Plugins)
- Headless-Service-Endpoint-Abdeckung und NXDOMAIN-Risiko
- NodeLocal-DNS-Cache-Erkennung
- Pod dnsConfig ndots-Abdeckungserkennung
- External-DNS-verwaltete Serviceerkennung
- Cluster-DNS-Integritätsbewertung 0-100
