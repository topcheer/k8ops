# Guide de l'Assistant d'Installation k8ops

L'assistant d'installation interactif (`wizard.sh`) vous guide dans la configuration
de tous les principaux composants k8ops avant le déploiement : backend de base de données,
intégration SSO et fournisseur IA.

## Démarrage rapide

### Mode interactif

```bash
git clone https://github.com/topcheer/k8ops.git
cd k8ops
./wizard.sh
```

### Mode non interactif

```bash
# Modifiez config/wizard-values.yaml avec vos paramètres, puis :
./wizard.sh --values config/wizard-values.yaml
```

### Simulation (générer les manifests uniquement)

```bash
./wizard.sh --dry-run
# Vérifiez les fichiers générés : .wizard-*.yaml
# Déployez manuellement avec kubectl apply -f ...
```

## Étapes de l'assistant

### Étape 1 : Mode de déploiement

| Mode | Description | Recommandé pour |
|------|-------------|-----------------|
| **DaemonSet** | S'exécute sur chaque nœud | Clusters K3s/bare-metal, surveillance au niveau du nœud |
| **Deployment** | Réplique unique avec PVC | K8s géré (EKS/GKE/AKS), configurations sensibles aux coûts |

### Étape 2 : Backend de base de données

k8ops utilise une base de données pour les comptes utilisateurs, les rôles et les providers d'authentification.

| Backend | Cas d'usage | HA | Configuration |
|---------|-------------|----|---------------|
| **SQLite** | Petits clusters, nœud unique | Non | Configuration zéro (intégré) |
| **MySQL** | Multi-répliques, auth partagée | Oui | StatefulSet interne ou connexion externe |
| **PostgreSQL** | Multi-répliques, auth partagée | Oui | StatefulSet interne ou connexion externe |

#### Base de données interne vs externe

- **Interne** : L'assistant déploie un StatefulSet MySQL/PostgreSQL dans le namespace
  `k8ops-system` avec un PVC. Entièrement géré — aucune dépendance externe.
- **Externe** : Connectez-vous à votre base de données existante. Vous fournissez la chaîne de connexion DSN.

#### Formats DSN

**MySQL :**
```
k8ops:password@tcp(mysql-host:3306)/k8ops?charset=utf8mb4&parseTime=True
```

**PostgreSQL :**
```
host=postgres-host user=k8ops password=secret dbname=k8ops sslmode=disable
```

### Étape 3 : SSO / Fournisseur d'identité

k8ops prend en charge plusieurs providers SSO avec des préréglages intégrés :

| Provider | Type | Préréglage |
|----------|------|------------|
| **GitHub** | OIDC | Émetteur préconfiguré |
| **Google** | OIDC | Émetteur préconfiguré |
| **Microsoft** (Entra ID) | OIDC | Émetteur préconfiguré |
| **GitLab** | OIDC | Émetteur préconfiguré |
| **Keycloak** | OIDC | Émetteur personnalisé (votre realm) |
| **Okta** | OIDC | Émetteur personnalisé |
| **Auth0** | OIDC | Émetteur personnalisé |
| **LDAP / AD** | LDAP | Serveur + DN de liaison |
| **OIDC personnalisé** | OIDC | URL d'émetteur manuelle |

#### URL de redirection OIDC

Lors de l'enregistrement de votre application auprès du fournisseur d'identité, utilisez cette URL de redirection :

```
https://<votre-hote-dashboard>/api/auth/oidc/<nom-provider>/callback
```

Exemple pour GitHub :
```
https://k8ops.example.com/api/auth/oidc/github/callback
```

#### Configuration LDAP

Fournir :
- **URL du serveur** : `ldap://host:389` ou `ldaps://host:636`
- **Base de recherche** : ex. `ou=users,dc=example,dc=com`
- **DN de liaison** : DN du compte de service, ex. `cn=admin,dc=example,dc=com`
- **Mot de passe de liaison** : Mot de passe du compte de service

Le SSO peut être ignoré pendant l'installation et configuré ultérieurement via **Paramètres > Auth Providers** dans le tableau de bord.

### Étape 4 : Fournisseur IA

| Provider | Modèles | Notes |
|----------|---------|-------|
| **OpenAI** | gpt-4o, gpt-4o-mini | Par défaut |
| **Anthropic** | claude-sonnet-4-20250514 | Famille Claude |
| **Gemini** | gemini-1.5-flash | Google AI |
| **Personnalisé** | Tout | Endpoint compatible OpenAI |

Le fournisseur IA peut être configuré après l'installation via **Paramètres** dans le tableau de bord.

### Étape 5 : Confirmation et déploiement

L'assistant affiche un résumé de tous les choix. Après confirmation, il :

1. Génère les manifests Kubernetes (secrets, StatefulSet de base de données optionnel)
2. Les applique au cluster
3. Déploie k8ops (DaemonSet ou Deployment)
4. Attend que les pods soient prêts
5. Affiche l'URL d'accès et les identifiants de connexion

## Post-installation

### Connexion par défaut

- Nom d'utilisateur : `admin`
- Mot de passe : `admin`
- **À modifier immédiatement après la première connexion**

### Configurer le SSO après l'installation

Si vous avez ignoré le SSO pendant l'installation :

1. Accédez à **Paramètres > Auth Providers**
2. Cliquez sur **Ajouter un fournisseur**
3. Sélectionnez un préréglage (GitHub, Google, etc.)
4. Saisissez le Client ID et le Client Secret
5. Enregistrez et activez

### Référence des variables d'environnement

L'assistant définit ces variables d'environnement (peuvent également être définies manuellement) :

| Variable | Description | Défaut |
|----------|-------------|--------|
| `AUTH_DB_DRIVER` | Pilote de base de données | `sqlite` |
| `AUTH_DB_DSN` | Chaîne de connexion à la base de données | (vide) |
| `AUTH_DB_PATH` | Chemin du fichier SQLite | `/data/k8ops.db` |
| `AUTH_JWT_SECRET` | Secret de signature JWT | (auto-généré) |
| `AUTH_DEFAULT_ROLE` | Rôle par défaut pour les utilisateurs SSO | `viewer` |
| `AIOPS_API_KEY` | Clé API du fournisseur IA | (vide) |

## Options de la ligne de commande

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

## Dépannage

### Erreur SQLite « out of memory »

Cela se produit lorsque le chemin de la base SQLite n'est pas accessible en écriture (ex. système de fichiers conteneur en lecture seule).
Assurez-vous que `/data` est soutenu par un volume `emptyDir` ou PVC.

### Échec de connexion MySQL/PostgreSQL

1. Vérifiez que le format DSN correspond à votre type de base de données
2. Vérifiez la connectivité réseau entre les pods k8ops et la base de données
3. Assurez-vous que l'utilisateur de la base de données dispose des permissions CREATE/ALTER (pour l'auto-migration)

### Redirection SSO non fonctionnelle

1. Vérifiez que l'URL de redirection correspond exactement (y compris la barre oblique finale)
2. Vérifiez que HTTPS est correctement configuré (OIDC nécessite HTTPS)
3. Assurez-vous que le fournisseur d'identité a l'URL de redirection correcte enregistrée
