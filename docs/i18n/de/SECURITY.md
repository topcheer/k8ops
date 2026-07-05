# k8ops Sicherheit

## Authentifizierung

k8ops unterstützt drei Authentifizierungsmethoden, die pro Bereitstellung konfigurierbar sind:

### Lokale Authentifizierung

- Benutzername/Passwort in SQLite gespeichert
- Passwörter mit bcrypt gehasht
- Admin-Bootstrap über `AUTH_DEFAULT_ROLE`-Umgebungsvariable

### LDAP / Active Directory

- Konfigurierbare Server-URL, Bind-DN, Suchbasis
- `SkipTLSVerify`-Option (Standard: `false`) für selbstsignierte Zertifikate
- Multi-Provider-Unterstützung: mehrere LDAP-Server können gleichzeitig konfiguriert werden

### OIDC (OpenID Connect)

- Unterstützt jeden OIDC-kompatiblen IdP (Google, GitHub, Keycloak, etc.)
- **CSRF-Schutz**: State-Parameter validiert mit `crypto/subtle.ConstantTimeCompare`
- **Pro-Provider-Cookie**: `oidc_state_{provider}` verhindert Multi-Provider-Kollision
- **Secure-Flag**: automatische Erkennung über TLS oder `X-Forwarded-Proto`-Header
- **HttpOnly + SameSite**: State-Cookie nicht über JavaScript zugänglich

## RBAC-Modell

### Rollen

| Rolle | Bereich | Berechtigungen |
|-------|---------|---------------|
| `admin` | Cluster | Vollzugriff: Benutzerverwaltung, Provider, alle Namespaces |
| `operator` | Cluster | Lesen + Chat + Diagnosen ausführen |
| `viewer` | Cluster | Schreibgeschützt: Dashboards und Berichte anzeigen |
| `ns-admin` | Namespace | Administrator innerhalb zugewiesener Namespaces |
| `ns-viewer` | Namespace | Schreibgeschützt innerhalb zugewiesener Namespaces |

### Namespace-Eingrenzung

Benutzer mit Namespace-bezogenen Rollen sind über folgende Mechanismen auf ihre zugewiesenen Namespaces beschränkt:

1. **K8s-RBAC-Synchronisierung**: `RoleBinding`-Ressourcen werden pro Namespace erstellt
2. **API-Impersonation**: Dashboard-API-Aufrufe verwenden die Benutzeridentität bei Kommunikation mit der K8s-API
3. **Namespace-Filterung**: API-Antworten werden auf erlaubte Namespaces gefiltert

### Integrierte Rollen-Schutz

Integrierte Rollen (`admin`, `operator`, `viewer`) sind als `Builtin: true` markiert und können nicht über die API gelöscht werden.

## Sicherheitsfunktionen

### CORS-Allowlist

- Konfiguriert über `CORS_ALLOWED_ORIGINS`-Umgebungsvariable (kommagetrennt)
- Kein Wildcard-(`*`)-Eintrag, wenn Anmeldedaten involviert sind
- Same-Origin nur, wenn nicht konfiguriert

### OIDC-CSRF-Schutz

- State-Parameter: zufällige Nonce pro Authentifizierungsversuch
- Validiert mit `subtle.ConstantTimeCompare` (Timing-sicher)
- In HttpOnly-Cookie mit Secure- + SameSite-Flags gespeichert

### JWT-Persistenz

- JWT-Signiergeheimnis in K8s-Secret `k8ops-auth` gespeichert (Schlüssel: `jwt-secret`)
- Fällt auf temporäres zufälliges Geheimnis mit Warnungslog zurück, wenn Secret fehlt
- Verhindert Sitzungsinvalidierung bei Pod-Neustart

### Audit-Protokollierung

Alle sensiblen Operationen werden protokolliert:

- Benutzer-Login/Logout
- Provider-Konfigurationsänderungen
- Diagnoseausführung
- Behebungsaktionen
- Benutzerrollenänderungen

### Ratenbegrenzung

- `resilience.RateLimiter` verfügbar (noch nicht an HTTP-Ebene angebunden — zukünftige Arbeit)

### Graceful Shutdown

- `SIGTERM`/`SIGINT` → SSE-Verbindungen beenden → SQLite-WAL flushen → Manager stoppen
- Verhindert Datenkorruption bei Pod-Eviction

## Sicherheitskonfiguration

### Umgebungsvariablen

| Variable | Standardwert | Beschreibung |
|----------|-------------|--------------|
| `AUTH_DB_DRIVER` | `sqlite` | Datenbanktreiber |
| `AUTH_DB_DSN` | — | Datenbankverbindungszeichenfolge |
| `AUTH_DB_PATH` | `/data/k8ops.db` | SQLite-Datenbankpfad |
| `AUTH_JWT_SECRET` | (zufällig) | JWT-Signiergeheimnis (über K8s-Secret persistieren) |
| `AUTH_DEFAULT_ROLE` | `viewer` | Rolle für neue Benutzer |
| `CORS_ALLOWED_ORIGINS` | — | Kommagetrennte erlaubte Ursprünge |
| `AIOPS_API_KEY` | — | LLM-Provider-API-Schlüssel |

### K8s-Secret-Verwaltung

```yaml
# K8s-Secret für JWT-Persistenz
apiVersion: v1
kind: Secret
metadata:
  name: k8ops-auth
  namespace: k8ops-system
type: Opaque
stringData:
  jwt-secret: "<openssl rand -base64 32>"
```

Das Deployment liest dies über:
```yaml
env:
- name: AUTH_JWT_SECRET
  valueFrom:
    secretKeyRef:
      name: k8ops-auth
      key: jwt-secret
      optional: true  # fällt auf zufälligen Wert zurück, wenn nicht vorhanden
```

### LDAP-TLS-Konfiguration

LDAP-Provider unterstützen `skip_tls_verify` (Standard: `false`):

```json
{
  "ldap": {
    "server": "ldaps://ldap.corp.com",
    "skip_tls_verify": false
  }
}
```

Setzen Sie `skip_tls_verify: true` nur für die Entwicklung mit selbstsignierten Zertifikaten.

## Bekannte Einschränkungen

1. **Keine Ratenbegrenzung beim Login** — `resilience.RateLimiter` existiert, ist aber nicht an HTTP-Handler angebunden
2. **Keine HTTPS-Terminierung** — k8ops bedient HTTP; TLS muss vom Ingress-Controller behandelt werden
3. **SQLite-Einzelknoten** — keine HA-Datenbank; geeignet für Single-Replica-Bereitstellungen
4. **Keine Sitzungssperrung** — JWT-Tokens gültig bis Ablauf (24h); keine serverseitige Sperrliste

## Sicherheitsmeldungen

Um eine Sicherheitslücke zu melden:

1. **KEIN öffentliches GitHub-Issue eröffnen**
2. E-Mail an security@ggai.dev mit Details und Reproduktionsschritten
3. Wir bestätigen innerhalb von 48 Stunden und geben einen Zeitplan für die Behebung
4. Verantwortungsvolle Offenlegung wird geschätzt

## Zukünftige Sicherheitsverbesserungen

- [ ] Ratenbegrenzung an Login-API anbinden
- [ ] Sitzungssperrung (Denylist) hinzufügen
- [ ] Externe OAuth-Provider für RBAC unterstützen
- [ ] mTLS für Service-to-Service-Kommunikation hinzufügen
- [ ] Verschlüsselung von Secrets at rest implementieren (über PVC hinaus)
