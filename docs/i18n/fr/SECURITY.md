# Sécurité k8ops

## Authentification

k8ops prend en charge trois méthodes d'authentification, configurables par déploiement :

### Authentification locale

- Nom d'utilisateur/mot de passe stockés dans SQLite
- Mots de passe hachés avec bcrypt
- Amorçage administrateur via la variable d'environnement `AUTH_DEFAULT_ROLE`

### LDAP/Active Directory

- URL du serveur, DN de liaison, base de recherche configurables
- Option `SkipTLSVerify` (par défaut : `false`) pour les certificats auto-signés
- Prise en charge multi-providers : plusieurs serveurs LDAP peuvent être configurés simultanément

### OIDC (OpenID Connect)

- Prise en charge de tout IdP compatible OIDC (Google, GitHub, Keycloak, etc.)
- **Protection CSRF** : paramètre d'état validé avec `crypto/subtle.ConstantTimeCompare`
- **Cookie par fournisseur** : `oidc_state_{provider}` empêche les collisions multi-fournisseurs
- **Flag Secure** : détecté automatiquement via TLS ou l'en-tête `X-Forwarded-Proto`
- **HttpOnly + SameSite** : le cookie d'état n'est pas accessible via JavaScript

## Modèle RBAC

### Rôles

| Rôle | Portée | Permissions |
|------|--------|-------------|
| `admin` | cluster | Accès complet : gestion des utilisateurs, providers, tous les namespaces |
| `operator` | cluster | Lecture globale + chat + exécution des diagnostics |
| `viewer` | cluster | Lecture seule : consultation des tableaux de bord, rapports |
| `ns-admin` | namespace | Administrateur dans les namespaces assignés |
| `ns-viewer` | namespace | Lecture seule dans les namespaces assignés |

### Portée par namespace

Les utilisateurs avec des rôles à portée de namespace sont restreints à leurs namespaces assignés via :

1. **Synchronisation RBAC K8s** : ressources `RoleBinding` créées par namespace
2. **Emprunt d'identité API** : les appels API du tableau de bord utilisent l'identité de l'utilisateur pour communiquer avec l'API K8s
3. **Filtrage par namespace** : les réponses API sont filtrées selon les namespaces autorisés

### Protection des rôles intégrés

Les rôles intégrés (`admin`, `operator`, `viewer`) sont marqués `Builtin: true` et ne peuvent pas être supprimés via l'API.

## Fonctionnalités de sécurité

### Liste d'autorisation CORS

- Configurée via la variable d'environnement `CORS_ALLOWED_ORIGINS` (séparée par des virgules)
- Pas de caractère générique (`*`) lorsque des identifiants sont impliqués
- Même origine uniquement si non configuré

### Protection CSRF OIDC

- Paramètre d'état : nonce aléatoire par tentative d'authentification
- Validé avec `subtle.ConstantTimeCompare` (résistant aux attaques temporelles)
- Stocké dans un cookie HttpOnly avec les flags Secure + SameSite

### Persistance JWT

- Le secret de signature JWT est persisté dans le Secret K8s `k8ops-auth` (clé : `jwt-secret`)
- Repli sur un secret aléatoire éphémère avec un avertissement dans les journaux si le Secret est absent
- Empêche l'invalidation des sessions au redémarrage du pod

### Journalisation d'audit

Toutes les opérations sensibles sont journalisées :

- Connexion/déconnexion utilisateur
- Modifications de configuration du fournisseur
- Exécution de diagnostics
- Actions de remédiation
- Modifications de rôle utilisateur

### Limitation de débit

- `resilience.RateLimiter` disponible (pas encore connecté à la couche HTTP — travail futur)

### Arrêt en douceur

- `SIGTERM`/`SIGINT` → drainage des connexions SSE → vidange du WAL SQLite → arrêt du manager
- Empêche la corruption des données lors de l'éviction du pod

## Configuration de sécurité

### Variables d'environnement

| Variable | Défaut | Description |
|----------|--------|-------------|
| `AUTH_DB_DRIVER` | `sqlite` | Pilote de base de données |
| `AUTH_DB_DSN` | — | Chaîne de connexion à la base de données |
| `AUTH_DB_PATH` | `/data/k8ops.db` | Chemin de la base SQLite |
| `AUTH_JWT_SECRET` | (aléatoire) | Secret de signature JWT (persister via Secret K8s) |
| `AUTH_DEFAULT_ROLE` | `viewer` | Rôle pour les nouveaux utilisateurs |
| `CORS_ALLOWED_ORIGINS` | — | Origines autorisées, séparées par des virgules |
| `AIOPS_API_KEY` | — | Clé API du fournisseur LLM |

### Gestion des secrets K8s

```yaml
# Secret K8s pour la persistance JWT
apiVersion: v1
kind: Secret
metadata:
  name: k8ops-auth
  namespace: k8ops-system
type: Opaque
stringData:
  jwt-secret: "<openssl rand -base64 32>"
```

Le déploiement lit cela via :
```yaml
env:
- name: AUTH_JWT_SECRET
  valueFrom:
    secretKeyRef:
      name: k8ops-auth
      key: jwt-secret
      optional: true  # repli sur aléatoire si absent
```

### Configuration TLS LDAP

Les providers LDAP prennent en charge `skip_tls_verify` (par défaut : `false`) :

```json
{
  "ldap": {
    "server": "ldaps://ldap.corp.com",
    "skip_tls_verify": false
  }
}
```

Ne définissez `skip_tls_verify: true` que pour le développement avec des certificats auto-signés.

## Limitations connues

1. **Pas de limitation de débit sur la connexion** — `resilience.RateLimiter` existe mais n'est pas connecté aux handlers HTTP
2. **Pas de terminaison HTTPS** — k8ops sert en HTTP ; le TLS doit être géré par le contrôleur Ingress
3. **SQLite mono-nœud** — pas de base de données HA ; adapté aux déploiements à réplique unique
4. **Pas de révocation de session** — les jetons JWT sont valides jusqu'à expiration (24h) ; pas de liste de révocation côté serveur

## Signalement de sécurité

Pour signaler une vulnérabilité de sécurité :

1. **NE PAS ouvrir un ticket GitHub public**
2. Envoyez un email à security@ggai.dev avec les détails et les étapes de reproduction
3. Nous accuserons réception dans les 48 heures et fournirons un calendrier de correction
4. La divulgation responsable est appréciée

## Améliorations de sécurité futures

- [ ] Connecter la limitation de débit à l'API de connexion
- [ ] Ajouter la révocation de session (liste de blocage)
- [ ] Prise en charge des providers OAuth externes pour le RBAC
- [ ] Ajouter mTLS pour la communication service à service
- [ ] Implémenter le chiffrement des secrets au repos (au-delà du PVC)
