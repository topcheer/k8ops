# k8ops Installations-Assistent Anleitung

Der interaktive Installations-Assistent (`wizard.sh`) führt Sie durch die Konfiguration
aller wichtigen k8ops-Komponenten vor der Bereitstellung: Datenbank-Backend, SSO-Integration
und KI-Provider.

## Schnellstart

### Interaktiver Modus

```bash
git clone https://github.com/topcheer/k8ops.git
cd k8ops
./wizard.sh
```

### Nicht-interaktiver Modus

```bash
# Bearbeiten Sie config/wizard-values.yaml mit Ihren Einstellungen, dann:
./wizard.sh --values config/wizard-values.yaml
```

### Dry-Run (nur Manifeste generieren)

```bash
./wizard.sh --dry-run
# Überprüfen Sie die generierten Dateien: .wizard-*.yaml
# Manuelles Bereitstellen mit kubectl apply -f ...
```

## Assistenten-Schritte

### Schritt 1: Bereitstellungsmodus

| Modus | Beschreibung | Optimal für |
|------|-------------|-------------|
| **DaemonSet** | Läuft auf jedem Knoten | K3s/Bare-Metal-Cluster, knotenbezogene Überwachung |
| **Deployment** | Einzelne Replika mit PVC | Verwaltetes K8s (EKS/GKE/AKS), kostenbewusste Setups |

### Schritt 2: Datenbank-Backend

k8ops verwendet eine Datenbank für Benutzerkonten, Rollen und Auth-Provider.

| Backend | Anwendungsfall | HA | Einrichtung |
|---------|---------------|-----|-------------|
| **SQLite** | Kleine Cluster, Einzelknoten | Nein | Keine Konfiguration (eingebettet) |
| **MySQL** | Multi-Replica, gemeinsame Auth | Ja | Internes StatefulSet oder externe Verbindung |
| **PostgreSQL** | Multi-Replica, gemeinsame Auth | Ja | Internes StatefulSet oder externe Verbindung |

#### Intern vs. Externe Datenbank

- **Intern**: Der Assistent stellt ein MySQL/PostgreSQL-StatefulSet im `k8ops-system`-
  Namespace mit einem PVC bereit. Vollständig verwaltet — keine externen Abhängigkeiten.
- **Extern**: Verbinden Sie sich mit Ihrer bestehenden Datenbank. Sie geben die DSN-Verbindungszeichenfolge an.

#### DSN-Formate

**MySQL:**
```
k8ops:password@tcp(mysql-host:3306)/k8ops?charset=utf8mb4&parseTime=True
```

**PostgreSQL:**
```
host=postgres-host user=k8ops password=secret dbname=k8ops sslmode=disable
```

### Schritt 3: SSO / Identity Provider

k8ops unterstützt mehrere SSO-Provider mit integrierten Voreinstellungen:

| Provider | Typ | Voreinstellung |
|----------|------|----------------|
| **GitHub** | OIDC | Vorkonfigurierter Issuer |
| **Google** | OIDC | Vorkonfigurierter Issuer |
| **Microsoft** (Entra ID) | OIDC | Vorkonfigurierter Issuer |
| **GitLab** | OIDC | Vorkonfigurierter Issuer |
| **Keycloak** | OIDC | Benutzerdefinierter Issuer (Ihr Realm) |
| **Okta** | OIDC | Benutzerdefinierter Issuer |
| **Auth0** | OIDC | Benutzerdefinierter Issuer |
| **LDAP / AD** | LDAP | Server + Bind-DN |
| **Benutzerdefiniertes OIDC** | OIDC | Manuelle Issuer-URL |

#### OIDC-Redirect-URL

Verwenden Sie beim Registrieren Ihrer Anwendung beim Identity Provider folgende Redirect-URL:

```
https://<your-dashboard-host>/api/auth/oidc/<provider-name>/callback
```

Beispiel für GitHub:
```
https://k8ops.example.com/api/auth/oidc/github/callback
```

#### LDAP-Konfiguration

Geben Sie an:
- **Server-URL**: `ldap://host:389` oder `ldaps://host:636`
- **Suchbasis**: z.B. `ou=users,dc=example,dc=com`
- **Bind-DN**: Service-Account-DN, z.B. `cn=admin,dc=example,dc=com`
- **Bind-Passwort**: Service-Account-Passwort

SSO kann während der Installation übersprungen und später über **Settings > Auth Providers** im Dashboard konfiguriert werden.

### Schritt 4: KI-Provider

| Provider | Modelle | Hinweise |
|----------|---------|---------|
| **OpenAI** | gpt-4o, gpt-4o-mini | Standard |
| **Anthropic** | claude-sonnet-4-20250514 | Claude-Familie |
| **Gemini** | gemini-1.5-flash | Google AI |
| **Benutzerdefiniert** | Beliebig | OpenAI-kompatibler Endpunkt |

Die Konfiguration des KI-Providers kann bis nach der Installation verschoben und dann über **Einstellungen** im Dashboard vorgenommen werden.

### Schritt 5: Bestätigen und Bereitstellen

Der Assistent zeigt eine Zusammenfassung aller Auswahlmöglichkeiten. Nach Bestätigung:

1. Generiert Kubernetes-Manifeste (Secrets, optionales DB-StatefulSet)
2. Wendet diese im Cluster an
3. Stellt k8ops bereit (DaemonSet oder Deployment)
4. Wartet, bis Pods bereit sind
5. Zeigt Zugriffs-URL und Anmeldedaten an

## Nach der Installation

### Standard-Login

- Benutzername: `admin`
- Passwort: `admin`
- **Sofort nach erstem Login ändern**

### SSO nach der Installation konfigurieren

Wenn Sie SSO während der Installation übersprungen haben:

1. Navigieren Sie zu **Settings > Auth Providers**
2. Klicken Sie **Add Provider**
3. Wählen Sie eine Voreinstellung (GitHub, Google, etc.)
4. Geben Sie Client-ID und Client-Secret ein
5. Speichern und aktivieren

### Umgebungsvariablen-Referenz

Der Assistent setzt diese Umgebungsvariablen (können auch manuell gesetzt werden):

| Variable | Beschreibung | Standardwert |
|----------|-------------|-------------|
| `AUTH_DB_DRIVER` | Datenbanktreiber | `sqlite` |
| `AUTH_DB_DSN` | Datenbankverbindungszeichenfolge | (leer) |
| `AUTH_DB_PATH` | SQLite-Dateipfad | `/data/k8ops.db` |
| `AUTH_JWT_SECRET` | JWT-Signiergeheimnis | (automatisch generiert) |
| `AUTH_DEFAULT_ROLE` | Standardrolle für SSO-Benutzer | `viewer` |
| `AIOPS_API_KEY` | KI-Provider-API-Schlüssel | (leer) |

## CLI-Flags

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

## Fehlerbehebung

### SQLite "out of memory"-Fehler

Dies tritt auf, wenn der SQLite-Datenbankpfad nicht beschreibbar ist (z.B. schreibgeschütztes Container-Dateisystem).
Stellen Sie sicher, dass `/data` durch ein `emptyDir`- oder PVC-Volume gestützt wird.

### MySQL/PostgreSQL-Verbindung fehlgeschlagen

1. Vergewissern Sie sich, dass das DSN-Format zu Ihrem Datenbanktyp passt
2. Überprüfen Sie die Netzwerkverbindung von k8ops-Pods zur Datenbank
3. Stellen Sie sicher, dass der Datenbankbenutzer CREATE/ALTER-Berechtigungen hat (für Auto-Migration)

### SSO-Redirect funktioniert nicht

1. Vergewissern Sie sich, dass die Redirect-URL exakt übereinstimmt (einschließlich abschließendem Schrägstrich)
2. Überprüfen Sie, ob HTTPS korrekt konfiguriert ist (OIDC erfordert HTTPS)
3. Stellen Sie sicher, dass der Identity Provider die korrekte Redirect-URL registriert hat
