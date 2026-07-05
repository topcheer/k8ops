# Référence de l'API k8ops

Tous les endpoints sont servis sur le port du tableau de bord (par défaut `:9090`).

**Authentification :** Cookie JWT (`k8ops_token`) ou en-tête `Authorization: Bearer <token>`.
**Content-Type :** `application/json` pour toutes les requêtes POST/PUT.

## Spécification OpenAPI 3.0

k8ops génère automatiquement une spécification OpenAPI 3.0, utilisable pour générer des SDK, intégrer des passerelles API ou naviguer dans Swagger UI.

| Endpoint | Description |
|----------|-------------|
| `GET /api/openapi.json` | Renvoie la spécification JSON OpenAPI 3.0 complète |
| `GET /api/docs` | Renvoie les métadonnées de documentation API groupées par tag (spec + tagGroups) |

**Obtenir la spécification :**
```bash
curl -sk https://k8ops.iot2.win/api/openapi.json -o k8ops-openapi.json
```

**Importer dans Swagger Editor :**
1. Ouvrir https://editor.swagger.io
2. Fichier → Importer un fichier → Sélectionner `k8ops-openapi.json`

**Navigation dans le tableau de bord :** La barre latérale → page API Docs offre un navigateur API interactif avec recherche, filtrage et test en ligne.

---

## Santé et système

| Méthode | Chemin | Auth | Description |
|---------|--------|------|-------------|
| GET | `/api/health` | Aucune | Sonde de survie — renvoie `{"status":"ok"}` |
| GET | `/api/version` | Aucune | Version de build, commit git, version Go |

## Cluster

| Méthode | Chemin | Auth | Description |
|---------|--------|------|-------------|
| GET | `/api/cluster/overview` | Requise | Résumé du cluster : nombre de nœuds, nombre de pods, utilisation CPU/mémoire, avertissements (cache 30s) |
| GET | `/api/nodes` | Requise | Liste de tous les nœuds avec utilisation des ressources et conditions (cache 30s) |
| GET | `/api/nodes/{node}/pods` | Requise | Pods en cours d'exécution sur un nœud spécifique |
| GET | `/api/pods` | Requise | Liste de tous les pods dans tous les namespaces (cache 30s) |
| GET | `/api/pods/{namespace}/{name}/containers` | Requise | Liste des conteneurs d'un pod |
| GET | `/api/pods/{namespace}/{name}/logs?container=&follow=&tailLines=` | Requise | Journaux de pod (prend en charge le streaming SSE avec `follow=true`) |
| GET | `/api/events?namespace=&warning=` | Requise | Événements Kubernetes, filtrage optionnel par namespace/avertissement |
| GET | `/api/resources?kind=&namespace=` | Requise | Lister de ressources générique (Deployments, Services, etc.) (cache 60s) |
| GET | `/api/crds?with_counts=true` | Requise | Custom Resource Definitions (cache 10min avec compteurs) |
| GET | `/api/crd-resources?group=&version=&resource=&namespace=` | Requise | Instances de CRD (cache 60s) |
| GET | `/api/yaml?namespace=&name=&group=&version=&resource=&kind=` | Requise | Vue YAML de toute ressource Kubernetes |

## Diagnostics et remédiation

| Méthode | Chemin | Auth | Description |
|---------|--------|------|-------------|
| GET | `/api/diagnostics` | Requise | Liste des CR DiagnosticReport, filtre optionnel `?namespace=` |
| GET | `/api/diagnostics/{namespace}/{name}` | Requise | Détail du diagnostic avec analyse IA |
| GET | `/api/remediations` | Requise | Liste des CR Remediation, filtre optionnel `?namespace=` |
| GET | `/api/optimizations` | Requise | Liste des CR Optimization, filtre optionnel `?namespace=` |

## Chat IA

| Méthode | Chemin | Auth | Description |
|---------|--------|------|-------------|
| POST | `/api/chat` | Requise | Envoyer un message à l'assistant IA (réponse en streaming SSE) |
| GET | `/api/chat/conversations?id=` | Requise | Liste des conversations ou récupération par ID |

### POST /api/chat

**Requête :**
```json
{
  "message": "Why is my pod crashing?",
  "conversation_id": "optional-existing-id",
  "stream": true
}
```

**Réponse :** Flux SSE de l'analyse IA avec appels d'outils et résultats.

### GET /api/chat/conversations

Renvoie l'historique des conversations. Passer `?id=<uuid>` pour une conversation unique.

## Gestion des providers

| Méthode | Chemin | Auth | Description |
|---------|--------|------|-------------|
| GET | `/api/provider/status` | Requise | Configuration actuelle du provider IA (clé API masquée) |
| POST | `/api/provider/update` | Requise | Mettre à jour le type/modèle/endpoint du provider à l'exécution |
| POST | `/api/provider/reload` | Requise | Recharger la configuration du provider depuis le CRD K8opsConfig |
| GET | `/api/tools` | Requise | Lister les outils de diagnostic enregistrés |

## Authentification

| Méthode | Chemin | Auth | Description |
|---------|--------|------|-------------|
| POST | `/api/auth/login` | Publique | Connexion locale (avec limitation de débit) |
| POST | `/api/auth/logout` | Requise | Effacer le cookie d'authentification |
| GET | `/api/auth/me` | Requise | Informations de l'utilisateur courant |
| POST | `/api/auth/change-password` | Requise | Changer son propre mot de passe |
| GET | `/api/auth/status` | Publique | Statut de la configuration d'authentification (auth_enabled, user_count, indicateurs ldap/oidc) |
| GET | `/api/auth/provider-presets` | Publique | Modèles de providers OIDC/LDAP disponibles |

### POST /api/auth/login

**Requête :**
```json
{
  "username": "admin",
  "password": "admin"
}
```

**Réponse (200) :**
```json
{
  "user": {"id": 1, "username": "admin", "role": "admin", "display_name": "Administrator"},
  "must_change": true,
  "redirect_url": "/"
}
```

Définit le cookie `k8ops_token` (HttpOnly, SameSite=Lax, 24h).

**Erreur (401) :**
```json
{"error": "invalid username or password"}
```

## OIDC

| Méthode | Chemin | Auth | Description |
|---------|--------|------|-------------|
| GET | `/api/auth/oidc/{provider}/login` | Publique | Rediriger vers le provider OIDC (définit le cookie d'état CSRF) |
| GET | `/api/auth/oidc/{provider}/callback` | Publique | Callback OIDC (valide l'état, crée la session utilisateur) |

## Gestion des providers d'authentification (Admin)

| Méthode | Chemin | Auth | Description |
|---------|--------|------|-------------|
| GET | `/api/auth/providers` | Admin | Lister les providers d'authentification configurés |
| POST | `/api/auth/providers` | Admin | Créer un provider d'authentification (LDAP/OIDC) |
| GET | `/api/auth/providers/{id}` | Admin | Obtenir un provider par ID |
| PUT | `/api/auth/providers/{id}` | Admin | Mettre à jour la configuration du provider |
| DELETE | `/api/auth/providers/{id}` | Admin | Supprimer un provider |

## Gestion des utilisateurs (Admin)

| Méthode | Chemin | Auth | Description |
|---------|--------|------|-------------|
| GET | `/api/admin/users` | Admin | Lister tous les utilisateurs |
| POST | `/api/admin/users` | Admin | Créer un utilisateur (rôle par défaut : viewer, MustChangePwd=true) |
| GET | `/api/admin/users/{id}` | Admin | Obtenir un utilisateur par ID |
| PUT | `/api/admin/users/{id}` | Admin | Mettre à jour un utilisateur (rôle, namespaces, etc.) |
| DELETE | `/api/admin/users/{id}` | Admin | Supprimer un utilisateur |
| POST | `/api/admin/users/{id}/reset-password` | Admin | Réinitialiser le mot de passe (définit MustChangePwd=true) |
| GET | `/api/admin/auth-config` | Admin | Obtenir la configuration d'authentification |
| PUT | `/api/admin/auth-config` | Admin | Mettre à jour la configuration d'authentification |

## Clés API

| Méthode | Chemin | Auth | Description |
|---------|--------|------|-------------|
| GET | `/api/auth/api-keys` | Requise | Lister ses propres clés API |
| POST | `/api/auth/api-keys` | Requise | Créer une clé API |
| DELETE | `/api/auth/api-keys/{id}` | Requise | Révoquer une clé API |

## Gestion RBAC (Admin)

| Méthode | Chemin | Auth | Description |
|---------|--------|------|-------------|
| GET | `/api/rbac/clusterroles` | Admin | Lister les rôles de cluster |
| GET | `/api/rbac/clusterroles/{name}` | Admin | Obtenir un rôle de cluster par nom |
| DELETE | `/api/rbac/clusterroles/{name}` | Admin | Supprimer un rôle de cluster |
| GET | `/api/rbac/roles?namespace=` | Admin | Lister les rôles à portée de namespace |
| GET | `/api/rbac/roles/{namespace}/{name}` | Admin | Obtenir un rôle à portée de namespace |
| DELETE | `/api/rbac/roles/{namespace}/{name}` | Admin | Supprimer un rôle à portée de namespace |
| GET | `/api/rbac/rolebindings?namespace=` | Admin | Lister les liaisons de rôles |
| GET | `/api/rbac/rolebindings/{namespace}/{name}` | Admin | Obtenir une liaison de rôle |
| DELETE | `/api/rbac/rolebindings/{namespace}/{name}` | Admin | Supprimer une liaison de rôle |
| GET | `/api/rbac/api-resources` | Admin | Lister les types de ressources de l'API Kubernetes |
| GET | `/api/rbac/namespaces` | Admin | Lister tous les namespaces |
| GET | `/api/rbac/role-mapping?role=&kind=&name=&namespace=` | Admin | Voir le mappage rôle vers sujet |
| GET | `/api/rbac/role-defs` | Admin | Lister les définitions de rôles personnalisés k8ops |
| GET | `/api/rbac/subjects?kind=&namespace=` | Admin | Lister les sujets (utilisateurs/groupes/comptes de service) |

## Audit

| Méthode | Chemin | Auth | Description |
|---------|--------|------|-------------|
| GET | `/api/audit?namespace=&limit=` | Requise | Entrées du journal d'audit (paginées) |
| GET | `/api/audit/stats` | Requise | Résumé des statistiques d'audit |

## Configuration

| Méthode | Chemin | Auth | Description |
|---------|--------|------|-------------|
| GET | `/api/config` | Requise | Configuration du contrôleur k8ops (type/modèle de provider, fonctionnalités) |

## Audit de sécurité

| Méthode | Chemin | Auth | Description |
|---------|--------|------|-------------|
| GET | `/api/security/audit` | Requise | Analyse de sécurité du cluster — vérifie les Pod Security Standards, le RBAC, la couverture des NetworkPolicies, la sécurité des secrets |
| GET | `/api/security/health` | Requise | Vérification de santé sécurité de la plateforme — authentification/TLS/connectivité API K8s |

### GET /api/security/audit

Analyse tout le cluster, renvoie une liste de résultats de sécurité triés par gravité (critical > high > medium > low > info).

**Éléments vérifiés :**
- **Sécurité des Pods :** conteneurs privilégiés, exécution en root, escalation de privilèges, capabilities dangereuses, hostPath/hostNetwork
- **RBAC :** liaisons cluster-admin, utilisation du SA par défaut
- **Réseau :** namespaces sans NetworkPolicy
- **Secrets :** recommandations de rotation des clés de registre Docker
- **Ressources :** conteneurs sans resource limits

**Exemple de réponse :**
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

## Opérations d'écriture

| Méthode | Chemin | Auth | Description |
|---------|--------|------|-------------|
| POST | `/api/scale` | Requise | Mettre à l'échelle un deployment/statefulset |
| POST | `/api/pod/delete` | Requise | Supprimer un pod individuel |
| POST | `/api/rollout/restart` | Requise | Redémarrage en continu d'un deployment/daemonset/statefulset |
| POST | `/api/node/cordon` | Requise | Isoler/restaurer un nœud |
| POST | `/api/yaml/apply` | Requise | Appliquer un YAML (kubectl apply) |

Toutes les opérations d'écriture sont enregistrées dans le journal d'audit.

---

## Réponses d'erreur

Toutes les erreurs renvoient du JSON :

```json
{"error": "descriptive error message"}
```

| Code | Signification |
|------|---------------|
| 400 | Requête incorrecte (paramètres manquants/invalides) |
| 401 | Non autorisé (jeton manquant/expiré/invalide) |
| 403 | Interdit (rôle insuffisant) |
| 404 | Ressource introuvable |
| 500 | Erreur interne du serveur |
| 503 | Service indisponible (provider IA non configuré) |

## Rôles

| Rôle | Permissions |
|------|-------------|
| `admin` | Accès complet incluant gestion des utilisateurs/RBAC/providers |
| `operator` | Tableau de bord + diagnostics + chat (sans gestion des utilisateurs) |
| `viewer` | Tableau de bord en lecture seule + chat |
| `ns-admin` | Admin dans les namespaces assignés uniquement |
| `ns-viewer` | Viewer dans les namespaces assignés uniquement |

## Nouveaux endpoints (v14.48-v14.53)

Les endpoints suivants ont été ajoutés entre v14.48 et v14.53 et sont inclus dans la spécification OpenAPI 3.0.

### Inventaire des images de conteneurs

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/images` | Inventaire de toutes les images de conteneurs dans le cluster, avec audit des limites de ressources et détection des tags `:latest` |

**Champs du résumé de réponse :** `totalImages`, `withoutLimits`, `withoutRequests`, `usingLatestTag`, `uniqueRegistries`

### Résumé des événements d'avertissement

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/events/summary` | Agrège tous les événements Warning par Reason, avec classification de gravité et statistiques des namespaces affectés |

### Analyse d'efficacité du cluster

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/efficiency` | Analyse de l'efficacité des ressources du cluster : pods sans limites, conteneurs surprovisionnés, nœuds sous-utilisés, score d'efficacité 0-100 |

### Sécurité : scan d'exposition des Secrets

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/security/secrets` | Détecte les identifiants codés en dur, le suivi de rotation des Secrets (90 jours), les Secrets inutilisés, les noms de clés sensibles |

### Recherche et export d'audit

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/audit/events` | Recherche d'événements d'audit : prend en charge `actor`, `action`, `q` (recherche plein texte), `severity`, filtrage par plage de dates |
| GET | `/api/audit/export` | Exporter les événements d'audit au format CSV (importable dans un SIEM) |

### Gestion des sauvegardes

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/system/backup` | Lister tous les fichiers de sauvegarde (taille, âge, type) |
| POST | `/api/system/backup` | Créer une sauvegarde de base de données (nom avec horodatage) |
| DELETE | `/api/system/backup?name=X` | Supprimer une sauvegarde spécifique (protection contre la traversée de chemin) |
| POST | `/api/system/backup/restore?name=X` | Restaurer la base de données depuis une sauvegarde |

### Webhook Alertmanager

| Méthode | Chemin | Description |
|---------|--------|-------------|
| POST | `/api/webhooks/alertmanager` | Reçoit les alertes Prometheus Alertmanager v4, génère automatiquement des suggestions d'investigation |
| POST | `/api/webhooks/alertmanager/test` | Envoie une alerte de test pour vérifier le récepteur |

**Exemple de configuration Alertmanager :**
```yaml
receivers:
  - name: k8ops
    webhook_configs:
      - url: http://k8ops.k8ops-system.svc:9090/api/webhooks/alertmanager
        send_resolved: true
```

### Journal des modifications

| Version | Endpoint | Dimension |
|---------|----------|-----------|
| v14.49 | `GET /api/events/summary` | Product |
| v14.50 | Sonde de démarrage + preStop | Deployment |
| v14.51 | `POST /api/webhooks/alertmanager` | Operations |
| v14.52 | `GET /api/efficiency` | Scalability |
| v14.53 | `GET /api/security/secrets` | Security |
| v14.54 | Spec OpenAPI 3.0 + API.md | Documentation |
| v14.55 | `GET /api/pdbs` `GET /api/compatibility` | Product |
| v14.56 | `GET /api/certificates/expiry` | Operations |
| v14.57 | Portail de drainage d'arrêt en douceur | Deployment |
| v14.58 | `GET /api/addons/health` | Product |
| v14.59 | `GET /api/capacity/forecast` | Scalability |
| v14.60 | Complétion spec OpenAPI + mise à jour API.md | Documentation |
| v14.61 | `GET /api/security/network-policies` | Security |
| v14.62 | `GET /api/diagnostics/restarts` | Operations |
| v14.63 | `GET /api/deployments/rollout` | Deployment |
| v14.64 | `GET /api/resources/waste` | Product |
| v14.65 | `GET /api/scaling/bottlenecks` | Scalability |

### Statut des Pod Disruption Budgets (v14.55+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/pdbs` | Liste tous les PDB, avec statut de disruption, workloads correspondants, évaluation de santé (healthy/at-risk/blocked), pour vérification de sécurité avant drain |

### Détection de compatibilité des distributions K8s (v14.55+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/compatibility` | Détecte automatiquement la distribution du cluster (vanilla/k3s/RKE2/EKS/GKE/AKS/OpenShift/Talos), la compatibilité de version, les caractéristiques des nœuds ARM/Windows/GPU |

### Scan d'expiration des certificats TLS (v14.56+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/certificates/expiry` | Scan tous les certificats X.509 dans les Secrets TLS/Opaque, classification par date d'expiration (expired/critical/warning/ok), corrélation avec les ressources Ingress |

### Statut de drainage du serveur (v14.57+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/system/drain-status` | Rapporte le statut d'arrêt en douceur du serveur : draining, shutdownInitiated, activeConnections, uptime |

### Détection de santé des plugins (v14.58+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/addons/health` | Détection non intrusive de 39 plugins K8s courants (12 catégories : CNI/DNS/Ingress/CertManager/LB/Mesh/Backup/Monitoring/Policy/Storage/GitOps/VM) et de leur état de santé |

### Prédiction d'épuisement de capacité (v14.59+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/capacity/forecast` | Prédit quand la capacité CPU/mémoire/Pod/stockage sera épuisée, avec estimation en jours et recommandations de mise à l'échelle |

### Scan d'audit des NetworkPolicies (v14.61+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/security/network-policies` | Audit de la couverture NetworkPolicy : détecte les namespaces sans NetworkPolicy, les politiques permissives (0.0.0.0/0 en entrée/sortie), la couverture partielle, classés par gravité (critical/warning/info) |

**Paramètres de requête :** `namespace` (optionnel, filtrer par namespace)

**Exemple de réponse :**
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

### Diagnostic des redémarrages de Pods (v14.62+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/diagnostics/restarts` | Diagnostic des modèles de redémarrage de Pods et des causes racines : classification des comportements (crash-loop/occasional/post-deploy), extraction des raisons de terminaison (OOMKilled/Error/code de sortie), des états d'attente (CrashLoopBackOff/ImagePullBackOff) |

**Paramètres de requête :** `namespace` (optionnel)

**Modèles de diagnostic :**
- **crash-loop** : nombreux redémarrages en peu de temps
- **occasional** : quelques redémarrages sur une longue période
- **post-deploy** : redémarrage immédiat après le déploiement

### Suivi du statut de rollout des déploiements (v14.63+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/deployments/rollout` | Scan de l'état de santé du rollout de tous les Deployment/StatefulSet/DaemonSet : 7 états (complete/in-progress/stalled/degraded/paused/failed/scaled-to-zero), détecte ProgressDeadlineExceeded, ReplicaFailure, mismatch de génération |

**Paramètres de requête :**
- `namespace` (optionnel) — filtrer par namespace
- `status` (optionnel) — filtrer par statut de rollout : `failed`, `degraded`, `stalled`, `in-progress`, `paused`, `scaled-to-zero`, `complete`

**Description des états :**
| État | Signification |
|------|---------------|
| `complete` | Toutes les répliques sont mises à jour et prêtes |
| `in-progress` | Mise à jour en continu en cours |
| `stalled` | Le contrôleur n'a pas observé le dernier spec (mismatch de génération) |
| `degraded` | Certaines répliques ne sont pas disponibles |
| `paused` | Deployment explicitement suspendu |
| `failed` | ProgressDeadlineExceeded, le déploiement a échoué par timeout |
| `scaled-to-zero` | Le nombre de répliques est de 0 |

### Détection des ressources gaspillées (v14.64+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/resources/waste` | Scan des ressources gaspillées et orphelines dans le cluster pour réduire les coûts : 6 catégories (dead-service/unused-pvc/orphaned-configmap/orphaned-secret/empty-namespace/unattached-pv), 4 niveaux de gravité (critical/high/medium/low), évaluation du risque de coûts |

**Paramètres de requête :** `namespace` (optionnel)

**Types de gaspillage :**
| Catégorie | Élément détecté | Gravité par défaut |
|-----------|-----------------|-------------------|
| `dead-service` | Service sans endpoint backend (LoadBalancer = critical) | medium/critical |
| `unused-pvc` | PVC non monté par aucun Pod | high |
| `orphaned-configmap` | ConfigMap non référencé par aucun Pod | low/medium |
| `orphaned-secret` | Secret non référencé par aucun Pod (risque de sécurité) | high |
| `empty-namespace` | Namespace sans Pod en cours d'exécution | medium |
| `unattached-pv` | PV en état Available (non lié à un PVC) | critical |

**Filtrage intelligent :** Ignore automatiquement le namespace kube-system, les Secrets de jetons ServiceAccount, les Secrets de release Helm, les ConfigMaps générés automatiquement

### Détection des goulots d'étranglement de mise à l'échelle (v14.65+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/scaling/bottlenecks` | Scan des facteurs limitant la mise à l'échelle horizontale : 7 types de goulots (node-schedulable/node-pressure/resource-quota/hpa-stuck/pdb-blocking/storage-exhaust/image-pull-limit), 4 niveaux d'impact (critical/high/moderate/low), résumé de capacité du cluster |

**Paramètres de requête :** `namespace` (optionnel)

**Types de goulots :**
| Catégorie | Élément détecté |
|-----------|-----------------|
| `node-schedulable` | Nœuds isolés, capacité de Pods du cluster dépassée (>75% avertissement / >90% critique) |
| `node-pressure` | États de pression mémoire, disque, PID |
| `resource-quota` | Quota de namespace dépassé à 75%/90% |
| `hpa-stuck` | HPA au nombre max de répliques ou métriques manquantes |
| `pdb-blocking` | PDB autorisant 0 interruption volontaire |
| `storage-exhaust` | Requêtes PVC de namespace dépassant 500Gi |

**Résumé de capacité du cluster :** Fournit le nombre de nœuds, la capacité CPU/mémoire et les quantités allouables, la capacité de Pods et la quantité allouée, la marge de mise à l'échelle

### Analyse des risques RBAC (v14.67+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/security/rbac-risk` | Analyse des risques de permissions de tous les RoleBinding/ClusterRoleBinding, système de score 0-100, 5 niveaux de risque (critical/high/elevated/moderate/low), détecte les liaisons cluster-admin, l'escalade de privilèges, les permissions génériques, l'accès aux ressources sensibles |

**Paramètres de requête :** `namespace` (optionnel)

**Règles de score de risque :**
| Élément détecté | Score de base | Score supplémentaire |
|------------------|---------------|---------------------|
| ClusterRoleBinding + cluster-admin | 100 | — |
| Escalade de privilèges (escalate/bind/impersonate) | — | +25 |
| Verbe générique (verbs: *) | — | +25 |
| Ressource générique (resources: *) | — | +20 |
| Opération d'écriture à l'échelle du cluster | — | +30 |
| Accès aux ressources sensibles (secrets/pods/exec) | — | +15 |

### Surveillance de santé d'exécution des CronJobs (v14.68+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/operations/cronjobs/health` | Surveillance de la santé d'exécution de tous les CronJobs : taux de succès, échecs consécutifs, planification suspendue/stagnante, jamais exécutés, 5 états de santé (healthy/warning/failing/suspended/no-runs) |

**Paramètres de requête :** `namespace` (optionnel)

**États de santé :**
| État | Condition de déclenchement |
|------|----------------------------|
| `failing` | 3 échecs consécutifs ou plus |
| `warning` | 1-2 échecs consécutifs, ou taux de succès < 50% |
| `suspended` | CronJob suspendu |
| `no-runs` | Jamais exécuté |
| `healthy` | Succès total récent |

### Surveillance de santé des Services et Endpoints (v14.69+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/networking/health` | Scan de la santé réseau de tous les Services et Ingress : services sans endpoints, sélecteur ne correspondant pas, endpoints dégradés, attente LoadBalancer, backend Ingress manquant/sans endpoints, 5 états de santé |

**Paramètres de requête :** `namespace` (optionnel)

**États de santé des Services :**
| État | Signification |
|------|---------------|
| `misconfigured` | Sélecteur ne correspondant pas — aucun Pod ne correspond au label |
| `no-endpoints` | Tous les endpoints indisponibles |
| `degraded` | Partie des endpoints indisponibles |
| `external` | ExternalName/LoadBalancer (informatif) |
| `healthy` | Tous les endpoints normaux |

**Vérification de santé Ingress :** Détecte si le Service backend existe, s'il a des endpoints disponibles, valide le backend par défaut et les chemins de règles

### Surveillance de santé de stockage PV/PVC (v14.70+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/storage/health` | Scan de la santé de stockage de tous les PVC/PV : diagnostic PVC en attente, PVC orphelins (liés mais sans Pod depuis > 1 jour), PVC Lost/Failed, PV Released/Failed nécessitant un nettoyage manuel, vieux PV Available gaspillant de la capacité, 6 états de santé + analyse de distribution des StorageClass |

**Paramètres de requête :** `namespace` (optionnel)

**États de santé PVC :**
| État | Signification |
|------|---------------|
| `failed` | Échec de configuration du PVC |
| `lost` | Le PV sous-jacent a été supprimé |
| `pending` | En attente de provisionnement (pas de StorageClass, WaitForFirstConsumer) |
| `near-capacity` | Proche de la limite de capacité |
| `orphaned` | Lié mais sans utilisation par Pod depuis plus de 1 jour |
| `bound` | Liaison normale |

**Détection de problèmes PV :** PV Released (nécessite un nettoyage manuel), PV Failed (échec de récupération), vieux PV Available (>7 jours gaspillant de la capacité)

**Analyse des StorageClass :** Marqueur de classe par défaut, provisioner, reclaim policy, mode de liaison, support de l'expansion de volume, distribution du nombre de PVC

### Audit de sécurité des ServiceAccounts (v14.72+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/security/service-accounts` | Audit complet de la sécurité de tous les ServiceAccounts : SA inutilisés, SA par défaut utilisé par des Pods, montage automatique de jeton inutile, liaisons cluster-admin, permissions à l'échelle du cluster, SA obsolètes, anciens Secrets de jetons longue durée |

**Paramètres de requête :** `namespace` (optionnel)

**Score de risque :** 0-100 (plus élevé = plus dangereux), 5 niveaux de gravité : critical / high / elevated / moderate / low

**Éléments détectés :**
| Élément détecté | Gravité | Description |
|------------------|---------|-------------|
| SA inutilisé (>7 jours sans référence par Pod) | moderate | Surface d'attaque élargie |
| SA par défaut utilisé par des Pods | elevated | Violation du principe de moindre privilège |
| Liaison cluster-admin | critical | Super-permissions au niveau du cluster |
| Montage automatique de jeton inutile | moderate | Les SA sans jeton ne devraient pas être montés |
| SA obsolète (>30 jours sans utilisation mais toujours avec permissions) | high | Permissions zombies |
| Ancien Secret de jeton longue durée (K8s <1.24) | high | Jeton longue durée non recommandé |

### Suivi du budget d'erreur SLO/SLA (v14.73+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/operations/slo` | Suivi de disponibilité SLO/SLA et du budget d'erreur basé sur un algorithme multi-fenêtre multi-taux de combustion |

**Paramètres de requête :** `namespace` (optionnel)

**Configuration des fenêtres :** 5m / 1h / 6h / 24h / 7d

**Contenu renvoyé :**
| Champ | Description |
|-------|-------------|
| `availability` | Pourcentage de disponibilité par fenêtre |
| `errorBudget` | Quantité restante du budget d'erreur et taux de consommation |
| `burnRate` | Taux de combustion multi-fenêtre (fast: 5m/1h, slow: 6h/24h) |
| `latencySLO` | Percentiles de latence P50/P95/P99 et objectifs |
| `status` | meeting (atteint) / at-risk (à risque) / violated (violation) |

### Surveillance des ResourceQuotas et LimitRanges (v14.74+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/resources/quota` | Scan de l'utilisation des ResourceQuotas et des contraintes par défaut des LimitRanges dans tous les namespaces |

**Paramètres de requête :** `namespace` (optionnel)

**Niveaux de statut des quotas :**
| Statut | Utilisation | Description |
|--------|-------------|-------------|
| `ok` | <70% | Normal |
| `warning` | 70-85% | Proche de la limite |
| `critical` | 85-100% | Dangereux |
| `exceeded` | >100% | Dépassement |
| `no-limit` | — | Aucun quota défini |

**Éléments détectés :** Utilisation des quotas CPU/mémoire/Pod/ConfigMap/Secret/stockage par namespace, namespaces sans protection de quota, analyse des contraintes par défaut/min/max des LimitRanges, classement des plus gros consommateurs

### Audit de configuration des déploiements (v14.75+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/deployments/audit` | Audit des violations des bonnes pratiques de configuration de tous les Deployment/StatefulSet/DaemonSet, 8 catégories de vérification, chacune avec gravité et recommandation de correction |

**Paramètres de requête :** `namespace` (optionnel), `severity` (optionnel : critical / warning / info)

**Catégories de vérification :**
| Catégorie | Élément vérifié |
|-----------|-----------------|
| `revision-history` | Historique des révisions trop court (< 2, pas de retour arrière possible) ou trop long (> 20, gaspillage de ressources) |
| `image-policy` | Tag `:latest` mais pullPolicy n'est pas Always ; tag fixe mais pullPolicy est Always |
| `resources` | Limits/requests de ressources manquants |
| `probes` | Sondes liveness/readiness/startup manquantes |
| `security-context` | Conteneurs privilégiés, exécution en root, système de fichiers racine inscriptible, escalation autorisée |
| `update-strategy` | Stratégie Recreate (indisponibilité), OnDelete (suppression manuelle de Pod requise), mise à jour en continu partitionnée |
| `lifecycle` | terminationGracePeriod trop court (< 10s) ou trop long (> 300s), crochet preStop manquant |
| `config-drift` | Profil seccomp manquant |

**Score de santé :** 0 (parfait) à 100 (pire), critical=20 points/warning=8 points/info=2 points

### Analyse de santé du scheduling et de fragmentation des ressources (v14.76+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/scheduling/health` | Analyse de la santé du scheduling du cluster, de la planifiabilité des nœuds, de la fragmentation des ressources et du diagnostic des Pods en attente |

**Paramètres de requête :** `namespace` (optionnel)

**Contenu renvoyé :**
| Champ | Description |
|-------|-------------|
| `summary` | Statistiques des nœuds (planifiables/non planifiables/isolés/sous pression), nombre de Pods en attente, nombre de FailedScheduling, expulsés en 24h, score de santé 0-100 |
| `nodes` | Statut de planifiabilité par nœud, types de pression, taints, CPU/mémoire/Pod disponibles et pourcentages |
| `pendingPods` | Liste des Pods en attente, avec requêtes CPU/mémoire, nodeSelector, cause d'échec de scheduling résolue |
| `largestFittablePod` | Plus grand Pod actuellement planifiable (CPU/mémoire/nombre de Pods), meilleur nœud |
| `effectiveCapacity` | Capacité théorique vs capacité effective (pourcentage de perte de capacité dû aux nœuds non planifiables) |
| `fragmentation` | Indicateurs de fragmentation des ressources (taux moyen de fragmentation CPU/mémoire, pire nœud fragmenté, détection de très gros Pods) |
| `evictions` | Enregistrements d'expulsion dans les 24h (Pod, nœud, raison) |
| `recommendations` | Recommandations de correction exploitables |

**Analyse des causes d'échec de scheduling :** insufficient-cpu / insufficient-memory / untolerated-taint / node-selector-mismatch / node-affinity-mismatch / pod-affinity-conflict / pod-limit-reached / volume-binding-failure / no-nodes-available

### Scan de posture de sécurité des Pods (v14.79+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/security/pods` | Audit de la posture de sécurité de tous les Pods en cours d'exécution : conteneurs privilégiés, hostNetwork/hostPID/hostIPC, montages HostPath, capabilities Linux dangereuses, exécution en root, escalation autorisée, système de fichiers racine inscriptible, contexte de sécurité manquant, images :latest/sans tag, non verrouillées par digest, injection de variables d'environnement Secret, sans limites de ressources, liaison de port hôte |

**Paramètres de requête :** `namespace` (optionnel), `severity` (optionnel : critical / warning / info)

**Score de risque :** 0 (sûr) à 100 (très risqué), critical=25 points/warning=8 points/info=2 points

**Catégories de vérification :**
| Catégorie | Gravité | Description |
|-----------|---------|-------------|
| `privileged` | critical | Conteneur privilégié — accès hôte complet |
| `host-network` | critical | Partage de l'espace de noms réseau du nœud |
| `host-pid` | critical | Visibilité de tous les processus du nœud |
| `host-ipc` | critical | Partage de l'espace de noms IPC |
| `host-path` | critical | Montage de volume HostPath depuis le nœud |
| `dangerous-capabilities` | critical | SYS_ADMIN/NET_ADMIN/NET_RAW/SYS_PTRACE/SYS_MODULE/DAC_OVERRIDE/SETUID/SETGID |
| `runs-as-root` | warning | Exécution en UID 0 |
| `privilege-escalation` | warning | Escalade de privilèges autorisée |
| `missing-security-context` | warning | Contexte de sécurité manquant |
| `image-latest` | warning | Utilisation du tag :latest |
| `image-no-tag` | warning | Sans tag (:latest par défaut) |
| `host-port` | warning | Liaison de port hôte |
| `image-no-digest` | info | Non verrouillé par digest |
| `writable-rootfs` | info | Système de fichiers racine inscriptible |
| `secret-env-vars` | info | Secret injecté en variable d'environnement |
| `no-resource-limits` | info | Sans limites de ressources |

### Détection de tempêtes d'événements et de défaillances en cascade (v14.80+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/operations/event-storm` | Analyse les événements Warning du cluster, détecte les tempêtes d'événements, les défaillances en cascade et le jitter des ressources. Statistiques des événements d'alerte sur les fenêtres 15min/1h/24h, classification de la gravité des tempêtes, identification des ressources en jitter (même ressource, même raison répétée 3+ fois), agrégation par namespace et raison, fournit des recommandations exploitables |

**Paramètres de requête :** `namespace` (optionnel)

**Gravité des tempêtes :**
| Gravité | Condition | Description |
|---------|-----------|-------------|
| `critical` | >50 events/15min | Investigation urgente |
| `high` | >20 events/15min | Attention requise |
| `medium` | >10 events/15min | Surveiller la tendance |
| `low` | >5 events/15min | Informatif |

**Contenu renvoyé :** Résultats de détection de tempête, classement des alertes par namespace, Top raisons d'événements, liste des ressources en jitter (avec fréquence), chronologie des événements des 15 dernières minutes, nombre de ressources affectées (rayon d'impact), recommandations exploitables

### Graphe de dépendances de ressources et analyse de portée d'impact (v14.81+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/dependencies` | Trace le grappe complet de dépendances d'un workload quelconque (Deployment/StatefulSet/DaemonSet/Pod), évalue la portée d'impact des changements |

**Paramètres de requête :**

| Paramètre | Obligatoire | Description |
|-----------|-------------|-------------|
| `kind` | Oui | Type de ressource : Deployment / StatefulSet / DaemonSet / Pod |
| `name` | Oui | Nom de la ressource |
| `namespace` | Non | Namespace (défaut : default) |

**Dépendances directes (ce que ce workload dépend de) :** ConfigMap, Secret, PVC, ServiceAccount

**Dépendances inverses (ce qui dépend de ce workload) :**
- Service (via label selector correspondant aux Pods)
- Ingress (routant vers le Service correspondant)
- NetworkPolicy (appliqué à ces Pods)
- HPA (ciblant ce workload)
- Autres Pods partageant le même ConfigMap/Secret

**Évaluation de la portée d'impact :** blastRadius = nombre de dépendances directes + nombre de dépendances inverses, niveau de risque low(<6) / medium(6-10) / high(11-20) / critical(>20)

### Vérification de conformité de la distribution topologique (v14.82+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/topology/spread` | Analyse la distribution des Pods dans les domaines topologiques (zone/region/node), vérifie la conformité aux topologySpreadConstraints |

**Paramètres de requête :** `namespace` (optionnel), `domain` (optionnel, clé du domaine topologique, défaut `kubernetes.io/hostname`, peut être défini sur `topology.kubernetes.io/zone`)

**Statut des workloads :**
| Statut | Signification |
|--------|---------------|
| `balanced` | Distribution uniforme (actualSkew ≤ maxSkew) |
| `skewed` | Distribution inégale (actualSkew > maxSkew) |
| `no-constraint` | Multi-répliques sans contrainte topologique |
| `single-replica` | Réplique unique (distribution topologique non applicable) |

**Contenu renvoyé :** Statistiques des domaines topologiques, distribution par domaine par workload (nombre de Pods / valeur attendue), écart réel vs écart maximum, labels de domaine et nombre de Pods par nœud, recommandations (ajouter des contraintes, labelliser les nœuds, indice de cluster à domaine unique)

### Audit de rotation et de cycle de vie des Secrets (v14.85+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/security/secrets/rotation` | Audit de la conformité de rotation et de la gestion du cycle de vie de tous les Secrets : suivi de l'âge (stale >90j / very stale >180j), détection des Secrets inutilisés (non référencés par aucun Pod), détection d'expiration des certificats TLS (analyse des certificats), suivi des Secrets de registre Docker, détection des anciens jetons ServiceAccount, détection de noms sensibles |

**Paramètres de requête :** `namespace` (optionnel)

**Score de risque :** Niveau de risque par Secret (critical / high / medium / low), score de rotation du cluster 0-100

**Catégories de vérification :**
| Élément vérifié | Gravité | Description |
|------------------|---------|-------------|
| Certificat TLS expiré | critical | Mise à jour immédiate |
| Secret Docker expiré >180j | critical | Peut contenir des identifiants de registre expirés |
| Certificat TLS expirant <30j | high | Planifier le renouvellement dès que possible |
| Stale + inutilisé + nom sensible | high | Risque de sécurité |
| Secret Docker stale | medium | Rotation recommandée |
| Stale mais en cours d'utilisation | low | Planifier la rotation |

### Audit d'efficacité des sondes de santé (v14.86+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/operations/probes` | Audit de la configuration des sondes liveness/readiness/startup de tous les workloads, détecte les redémarrages en cascade, le trafic vers des Pods non prêts, les échecs de démarrage causés par une configuration inadéquate |

**Paramètres de requête :** `namespace` (optionnel)

**Catégories de vérification :**
| Élément vérifié | Gravité | Description |
|------------------|---------|-------------|
| liveness manquante | warning | Conteneur bloqué non redémarré |
| readiness manquante | warning | Le trafic peut atteindre des Pods non prêts |
| Sonde trop agressive (period <5s) | warning | Charge excessive sur l'API server |
| Timeout trop court (<2s) | warning | Faux positif possible sous pics de latence |
| Seuil d'échec trop bas (<3) | warning | Trop sensible aux erreurs transitoires |
| Intervalle readiness trop long (>60s) | info | Détection de disponibilité lente |
| Seuil d'échec liveness trop élevé (>10) | info | Récupération lente après redémarrage |
| liveness+readiness identiques | info | Devraient être différenciées |

**Contenu renvolté :** Score de risque par workload, score d'efficacité du cluster (0-100), Top problèmes agrégés, recommandations exploitables

### Suivi de l'obsolescence des workloads (v14.87+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/product/staleness` | Suit l'obsolescence de déploiement de tous les workloads, détecte les workloads non mis à jour depuis longtemps, les images utilisant le tag :latest, les images non verrouillées par digest |

**Paramètres de requête :** `namespace` (optionnel)

**Classification de l'obsolescence :**
| Statut | Condition | Description |
|--------|-----------|-------------|
| `fresh` | <7j | Récemment mis à jour |
| `recent` | <30j | Assez récent |
| `stale` | <90j | Attention requise |
| `very-stale` | <180j | Mise à jour recommandée |
| `ancient` | >180j | Risque de sécurité |

**Contenu renvolté :** Niveau de risque par workload, analyse des tags d'image (:latest / digest / no-tag), buckets de distribution d'âge, statistiques par namespace, score de fraîcheur du cluster (0-100), recommandations exploitables

### Analyse du sur-engagement et de la pression des ressources (v14.88+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/scalability/overcommit` | Analyse des ratios de sur-engagement CPU et mémoire de tous les nœuds, détecte les over-commits dangereux, les Pods sans limits, score de pression des ressources |

**Paramètres de requête :** `namespace` (optionnel)

**Analyse par nœud :**
| Métrique | Description |
|----------|-------------|
| CPU request commit | sum(requests) / allocatable |
| CPU limit commit | sum(limits) / allocatable |
| Mem request/limit commit | Idem |
| Score de pression | 0-100 (calcul pondéré) |
| Niveau de risque | safe / moderate / high / critical (>3x) |

**Métriques du cluster :** Ratios totaux de sur-engagement CPU/mémoire, nombre de nœuds à risque, nombre de Pods sans limits, score de sécurité (0-100), détail de consommation par namespace, recommandations exploitables

### Analyse de sécurité et de chaîne d'approvisionnement des images (v14.92+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/security/images` | Scan des risques de sécurité de la chaîne d'approvisionnement de toutes les images de conteneurs en cours d'exécution : verrouillage par digest, tag :latest, images sans tag, anciens tags de version, registres publics vs privés, registres inconnus |

**Paramètres de requête :** `namespace` (optionnel)

**Catégories de vérification :**
| Élément vérifié | Score de risque | Description |
|------------------|-----------------|-------------|
| Sans tag | +25 | Utilise :latest par défaut, version incertaine |
| Utilise :latest | +15 | Tag mutable, non reproductible |
| Non verrouillé par digest | +10 | Le contenu de l'image peut être remplacé silencieusement |
| Registre inconnu | +10 | Sans préfixe de registre, Docker Hub par défaut |
| Ancien tag de version | +15 | Peut contenir des vulnérabilités connues |
| Registre public + non verrouillé | +5 | Aucune garantie de provenance |

**Contenu renvolté :** Niveau de risque par image (critical/high/medium/low), statistiques par registre, Top images à risque, score de sécurité des images du cluster (0-100), recommandations exploitables

### Planification de capacité (v14.50+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/capacity/planning` | Analyse de planification de capacité des nœuds : requêtes CPU/mémoire par nœud vs quantités allouables, capacité restante, calendrier de mise à l'échelle recommandé, détection de fragmentation des ressources |

### Score de santé agrégé du cluster (v14.93+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/operations/health-score` | Agrège tous les signaux de santé du cluster en un score composite (0-100, niveau A-F), combinant 5 dimensions pondérées |

**5 dimensions pondérées :**
| Dimension | Poids | Éléments vérifiés |
|-----------|-------|-------------------|
| Node Health | 25% | État de préparation des nœuds |
| Pod Health | 25% | CrashLoop, Pending, Failed, nombre élev de redémarrages |
| Workload Health | 20% | Répliques prêtes des Deployment/StatefulSet/DaemonSet |
| Event Activity | 15% | Nombre d'événements Warning dans la dernière heure |
| API Server | 15% | Mesure de latence en temps réel de l'API server |

**Contenu renvolté :** Score total 0-100, note A-F, statut (healthy/warning/critical), détails du score par dimension, résumé du cluster (nombre de nœuds/Pods/workloads), Top problèmes triés par gravité

### Recommandations de configuration rationnelle des ressources HPA/VPA (v14.94+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/scalability/autoscale-recommendations` | Analyse de la couverture HPA et de la configuration rationnelle des ressources de tous les workloads, détecte le surprovisionnement, les workloads multi-répliques sans HPA, l'efficacité des HPA |

**Paramètres de requête :** `namespace` (optionnel)

**Catégories de détection :**
| Élément détecté | Description |
|------------------|-------------|
| Workload multi-répliques sans HPA | Ajout d'autoscaling recommandé |
| Requête CPU excessive (>1 core/container) | Haute confiance, réduction de moitié recommandée |
| Requête mémoire excessive (>2GB/container) | Right-sizing recommandé |
| HPA à maxReplicas | Augmentation de capacité requise |
| HPA inactif (<20% utilisation) | Réduction de maxReplicas recommandée |

**Contenu renvolté :** Valeurs CPU/mémoire actuelles vs recommandées par workload, pourcentage de changement, niveau de confiance, économies potentielles en coeurs CPU et mémoire, analyse d'efficacité HPA, score d'autoscaling du cluster (0-100)

### Surveillance de santé des Ingress et du routage de trafic (v14.96+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/product/ingress-health` | Analyse de la santé du routage de trafic et des problèmes de configuration de toutes les ressources Ingress |

**Paramètres de requête :** `namespace` (optionnel)

**Catégories de vérification :**
| Élément vérifié | Gravité | Description |
|------------------|---------|-------------|
| Service backend inexistant | critical | Le Service référencé n'existe pas |
| Backend sans endpoints prêts | warning | Le Service n'a pas d'endpoints prêts |
| Pas de configuration TLS | warning | Hôte présent mais non chiffré |
| IngressClass inexistante | critical | La classe spécifiée n'est pas déployée |
| Conflit host+path | warning | Plusieurs Ingress se disputent la même route |
| Pas de règle de routage | warning | L'Ingress n'a aucun effet |

**Contenu renvolté :** Statut par Ingress (healthy/warning/critical), statistiques par namespace, score de santé du cluster (0-100), recommandations exploitables

### Analyse des conditions de nœud et de la pression des ressources (v14.99+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/operations/node-pressure` | Analyse de l'état des conditions et de la saturation des ressources de tous les nœuds |

**Catégories de détection :**
| Condition | Score de risque | Description |
|-----------|-----------------|-------------|
| NetworkUnavailable | +30 | CNI/réseau non prêt |
| DiskPressure | +25 | Disque plein ou presque |
| MemoryPressure | +25 | Mémoire du nœud épuisée |
| PIDPressure | +20 | Trop de processus |
| NotReady | →critical | Problème kubelet/runtime |
| CPU >90% | +20 | Saturation des requêtes CPU |
| Mémoire >95% | +20 | Saturation des requêtes mémoire |
| Cordoned | — | Non planifiable |

**Contenu renvolté :** Niveau de risque par nœud (critical/high/medium/low), utilisation CPU/mémoire/Pod, détails des conditions (raison, message, durée), score de pression du cluster (0-100), recommandations exploitables

### Analyse de liaison PVC et de performance de stockage (v15.00+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/scalability/pvc-analysis` | Analyse de la santé de liaison et de la performance de stockage de tous les PVC |

**Paramètres de requête :** `namespace` (optionnel)

**Catégories de détection :**
| Élément détecté | Gravité | Description |
|------------------|---------|-------------|
| PVC bloqué (>5min) | critical | PVC coincé + analyse de la cause racine |
| PVC Lost | critical | Le PV sous-jacent a peut-être été supprimé |
| Liaison lente (>30s) | warning | Latence de provisionnement de stockage |
| PVC Pending | warning | En attente de liaison |
| StorageClass par défaut manquant | info | Aucun SC par défaut défini |

**Contenu renvolté :** Statut par PVC (healthy/warning/critical), temps de liaison, statistiques par StorageClass, cause racine des PVC bloqués, score de santé de stockage du cluster (0-100)

### Gouvernance des namespaces et audit de cycle de vie (v15.02+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/product/namespaces/lifecycle` | Audit de la conformité à la gouvernance et du cycle de vie de tous les namespaces |

**Vérifications de gouvernance :**
| Élément vérifié | Score de risque | Description |
|------------------|-----------------|-------------|
| Pas de ResourceQuota | +15 | Consommation de ressources illimitée |
| Pas de NetworkPolicy | +15 | Trafic non restreint |
| Pas de LimitRange | +5 | Pas de limites de ressources par défaut |
| Namespace expiré | +10 | Aucun Pod en cours, candidat au nettoyage |
| Labels requis manquants | +5 | Manque app/team/env/owner |
| SA par défaut uniquement | 0 | Manque de SA à moindre privilège |

**Contenu renvolté :** Niveau de risque par namespace (critical/high/medium/low), indicateurs de conformité, statut de cycle de vie (active/stale/terminating), score de gouvernance du cluster (0-100), recommandations exploitables

### Analyse des permissions effectives RBAC et de l'escalade (v15.04+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/security/rbac-effective` | Analyse des permissions RBAC effectives et des risques d'escalade de tous les sujets |

Agrège les ClusterRoleBindings + RoleBindings, calcule les permissions réelles de chaque sujet (User/Group/ServiceAccount).

**Catégories de détection :**

| Élément détecté | Score de risque | Description |
|------------------|-----------------|-------------|
| Équivalent cluster-admin | →critical | Verbes génériques + ressources génériques |
| Peut créer/modifier le RBAC | +25 | Chemin d'auto-escalade |
| Permissions génériques (*) | +20 | Sur-dépendance |
| Peut lire les Secrets | +10 | Fuite de données sensibles |
| Peut exec dans les Pods | +10 | Point d'entrée d'évasion de conteneur |

**Contenu renvolté :** Niveau de risque par sujet, détails des chemins d'escalade, score de sécurité RBAC du cluster (0-100), recommandations exploitables

### Traqueur de OOM Kill de conteneurs (v15.05+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/operations/oom-tracker` | Traque les événements OOMKill de conteneurs et l'analyse de configuration mémoire |

**Paramètres de requête :** `namespace` (optionnel)

**Catégories de détection :**

| Élément détecté | Score de risque | Description |
|------------------|-----------------|-------------|
| Conteneur OOMKilled | +15/conteneur | Tué par manque de mémoire |
| Nombre élevé de redémarrages (>=10) | +20 | Indicateur CrashLoop |
| Nombre élevé de redémarrages (>=5) | +10 | Redémarrages fréquents |
| Pas de limite mémoire | +5 | Comportement OOM imprévisible |
| Limite mémoire faible (<256MB) | — | Peut causer des OOM inutiles |
| Limite>>requête (10x+) | — | Risque de pression mémoire sur le nœud |

**Contenu renvolté :** Niveau de risque OOM par Pod, classement Top OOM, statistiques par namespace, score de risque OOM du cluster (0-100)

### Prédicteur d'épuisement de capacité de stockage (v15.06+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/scalability/storage-forecast` | Prédit quand la capacité de stockage sera épuisée |

Basé sur les tendances d'utilisation des PV et l'estimation du taux de croissance, prédit le moment d'épuisement de l'espace de stockage.

**Dimensions d'analyse :**

| Métrique | Description |
|----------|-------------|
| Capacité vs utilisée | Prise en charge de l'annotation actual-size de Longhorn pour l'utilisation réelle |
| Taux de croissance quotidien | Estimation heuristique basée sur l'utilisation et l'âge du PV |
| Jours avant épuisement | Espace restant / taux de croissance quotidien |
| Date d'épuisement prévue | Date ou ">10 ans" ou "pas de croissance" |
| Niveau de risque | critical(>95%) / high(>85% ou <14j) / medium(<30j) / low |

**Contenu renvolté :** Prévision par PV, estimation en jours avant épuisement complet du cluster, statistiques par StorageClass, classement des namespaces à haut risque, score de santé de stockage (0-100)

### Vérificateur de santé de résolution DNS (v15.08+)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| GET | `/api/product/dns-health` | Analyse de l'état de santé de la résolution DNS du cluster |

**Analyse CoreDNS :**

| Élément vérifié | Description |
|------------------|-------------|
| Santé des Pods | running/ready/redémarrages/version par pod |
| Corefile | forwarders, plugins, détection de Corefile manquant |
| Nombre de répliques | Recommandation >= 2 pour la haute disponibilité |

**Autres détections :**
- Couverture des endpoints des Services Headless (risque NXDOMAIN)
- Détection du cache DNS NodeLocal
- Détection de couverture ndots dnsConfig des Pods (>5 = trop de requêtes DNS)
- Découverte de services gérés par External-DNS

**Contenu renvolté :** Statut des Pods CoreDNS, couverture des Services Headless, analyse de configuration DNS, score de santé DNS du cluster (0-100), recommandations exploitables
