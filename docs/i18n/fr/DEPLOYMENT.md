# Guide de déploiement k8ops

## Installation et désinstallation en une commande

### Prérequis

- Kubernetes 1.24+ (k3s / k8s / EKS / GKE / AKS pris en charge)
- kubectl configuré et capable de se connecter au cluster
- Registre d'images conteneurs local ou distant (utilise par défaut `registry.iot2.win`)
- Optionnel : Clé API LLM (OpenAI / DeepSeek / ZAI ou interface compatible)

---

## Méthode 1 : Mode Déploiement (recommandé)

Déploiement à réplique unique, adapté à la plupart des scénarios. Inclut Ingress, Service, ConfigMap, Secret, RBAC — une seule commande pour un déploiement complet.

### Installation

```bash
# Réseau local (inclut déjà domaine, image, CORS et toute la configuration)
kubectl apply -k config/deploy/overlays/local

# Ou un overlay personnalisé
cp -r config/deploy/overlays/local config/deploy/overlays/myorg
# Modifiez myorg/kustomization.yaml : remplacez l'adresse de l'image, le domaine, CORS, etc.
kubectl apply -k config/deploy/overlays/myorg
```

### Vérification

```bash
# Vérifier le statut des Pods
kubectl get pods -n k8ops-system

# Vérifier l'Ingress
kubectl get ingress -n k8ops-system

# Accéder au tableau de bord
# Ouvrez https://<votre-domaine> dans le navigateur (ex. https://k8ops.iot2.win)
# Connexion par défaut : admin / admin (modification du mot de passe demandée à la première connexion)
```

### Désinstallation

```bash
kubectl delete -k config/deploy/overlays/local
```

---

## Méthode 2 : Mode DaemonSet

Un Pod par nœud, prenant en charge les diagnostics au niveau du nœud (hostPID, hostPath). Adapté aux scénarios nécessitant une surveillance approfondie des nœuds.

### Installation

```bash
kubectl apply -f config/daemonset-local.yaml
```

### Vérification

```bash
# Vérifier le DaemonSet (un Pod par nœud)
kubectl get ds -n k8ops-system
kubectl get pods -n k8ops-system -o wide

# Accéder au tableau de bord (via le ClusterIP du Service ou l'Ingress)
kubectl get svc k8ops-dashboard -n k8ops-system
```

### Désinstallation

```bash
kubectl delete -f config/daemonset-local.yaml
```

---

## Méthode 3 : Script install.sh

```bash
# Installation (détection automatique de l'environnement, choix interactif Deployment / DaemonSet)
./install.sh install

# Désinstallation
./install.sh uninstall

# Voir le statut
./install.sh status
```

---

## Construction et publication d'image

```bash
# Construction locale (amd64, pour les nœuds de cluster)
docker buildx build --platform linux/amd64 \
  --build-arg VERSION=v1.0.0 \
  -t registry.iot2.win/k8ops:v1.0.0 \
  -t registry.iot2.win/k8ops:latest \
  --load .

# Pousser vers le registre
docker push registry.iot2.win/k8ops:v1.0.0
docker push registry.iot2.win/k8ops:latest
```

### Construction multi-architecture

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  --build-arg VERSION=v1.0.0 \
  -t registry.iot2.win/k8ops:v1.0.0 \
  --push .
```

---

## Configuration du fournisseur LLM

### Méthode 1 : Configuration via le tableau de bord (recommandé)

1. Connectez-vous au tableau de bord → onglet **Paramètres**
2. Renseignez le type de fournisseur, la clé API, l'endpoint et le modèle
3. Cliquez sur **Enregistrer**, sauvegarde automatique dans le ConfigMap/Secret K8s

### Méthode 2 : Variables d'environnement

Définir dans le ConfigMap de l'overlay :

```yaml
configMapGenerator:
- name: k8ops-config
  literals:
  - PROVIDER_TYPE=openai          # openai / deepseek / zai / anthropic
  - PROVIDER_MODEL=gpt-4o
  - PROVIDER_ENDPOINT=https://api.openai.com/v1
```

Clé API via Secret :

```yaml
secretGenerator:
- name: k8ops-credentials
  literals:
  - api-key=sk-your-api-key-here
```

### Providers pris en charge

| Provider | Endpoint | Exemple de modèle |
|----------|----------|-------------------|
| OpenAI | `https://api.openai.com/v1` | gpt-4o, gpt-4o-mini |
| DeepSeek | `https://api.deepseek.com/v1` | deepseek-chat |
| ZAI (智谱) | `https://open.bigmodel.cn/api/paas/v4` | glm-4-flash, glm-4-plus |
| Anthropic | `https://api.anthropic.com/v1` | claude-3-5-sonnet |
| Local | `http://localhost:11434/v1` | llama3, qwen2 |

---

## Configuration de l'authentification

### Authentification locale (par défaut)

Fonctionnement immédiat, utilisateurs stockés dans SQLite. Première connexion : `admin / admin`.

### LDAP

```yaml
# Définir dans le ConfigMap ou la configuration du fournisseur
LDAP_SERVER=ldap://your-ldap:389
LDAP_BIND_DN=cn=admin,dc=example,dc=com
LDAP_BIND_PASSWORD=secret
LDAP_USER_BASE=ou=users,dc=example,dc=com
LDAP_SKIP_TLS_VERIFY=false   # En production, doit absolument être false
```

### OIDC (GitHub / Google / Keycloak, etc.)

```yaml
# Configuration du fournisseur (page Paramètres du tableau de bord ou CRD)
OIDC_ISSUER=https://your-keycloak/realms/myrealm
OIDC_CLIENT_ID=k8ops
OIDC_CLIENT_SECRET=your-secret
OIDC_REDIRECT_URL=https://k8ops.iot2.win/auth/oidc/callback
```

---

## Ingress et TLS

### TLS automatique (cert-manager + Let's Encrypt)

Assurez-vous que cert-manager est installé sur le cluster, ajoutez l'annotation dans l'Ingress :

```yaml
annotations:
  cert-manager.io/cluster-issuer: letsencrypt-prod
```

### Utiliser un certificat TLS existant

```bash
kubectl create secret tls k8ops-dashboard-tls \
  --cert=fullchain.pem \
  --key=privkey.pem \
  -n k8ops-system
```

---

## Problèmes courants

### Pod reste en Pending

```bash
# Vérifier la cause de l'échec de planification
kubectl describe pod <pod-name> -n k8ops-system | tail -10

# Causes courantes :
# - Conflit de port hostNetwork → supprimer hostNetwork: true ou éviter les conflits de déclaration de port
# - Ressources insuffisantes → ajuster resources.requests/limits
# - Taints de nœud → vérifier les tolerations
```

### Le tableau de bord renvoie 502

```bash
# 1. Vérifier si le Pod est Ready
kubectl get pods -n k8ops-system

# 2. Vérifier les endpoints du Service
kubectl get endpoints k8ops-dashboard -n k8ops-system

# 3. Vérifier le backend Ingress
kubectl describe ingress -n k8ops-system

# 4. Attendre que le Pod soit complètement prêt puis réessayer
```

### Échec de pull d'image

```bash
# Solution 1 : Définir imagePullPolicy: Always (recommandé avec un tag spécifique)
# Solution 2 : S'assurer que le nœud est configuré pour la confiance TLS du registre
# Solution 3 : Si registre privé, créer des imagePullSecrets
```

### Erreur 401 de l'API LLM

```bash
# Vérifier si la clé API est correctement configurée
kubectl get secret k8ops-credentials -n k8ops-system -o jsonpath='{.data.api-key}' | base64 -d

# Ou reconfigurer le fournisseur dans Tableau de bord → Paramètres
```

---

## Mise à niveau

```bash
# Construire et pousser la nouvelle image
docker buildx build --platform linux/amd64 \
  --build-arg VERSION=v2.0.0 \
  -t registry.iot2.win/k8ops:v2.0.0 \
  -t registry.iot2.win/k8ops:latest \
  --push .

# Mise à jour en continu (mode Deployment)
kubectl set image deployment/k8ops \
  k8ops=registry.iot2.win/k8ops:v2.0.0 \
  -n k8ops-system

# Ou modifier le newTag dans l'overlay puis réappliquer
kubectl apply -k config/deploy/overlays/local

# Mode DaemonSet
kubectl set image daemonset/k8ops \
  k8ops=registry.iot2.win/k8ops:v2.0.0 \
  -n k8ops-system
```

---

## Sauvegarde et restauration des données

### Sauvegarde automatique SQLite (CronJob)

k8ops utilise SQLite pour stocker les utilisateurs, les journaux d'audit et les données de session. Sauvegarde automatique horaire recommandée :

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: k8ops-backup
  namespace: k8ops-system
spec:
  schedule: "0 * * * *"  # Chaque heure pile
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
              # Conserver les 24 derniers sauvegardes
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

### Sauvegarde manuelle

```bash
# Copier la base de données depuis le Pod
kubectl cp k8ops-system/<pod-name>:/data/k8ops.db ./k8ops-backup-$(date +%Y%m%d).db

# Ou utiliser la sauvegarde en ligne sqlite3 (sans interrompre l'écriture)
kubectl exec -n k8ops-system <pod-name> -- sqlite3 /data/k8ops.db ".backup /data/k8ops-backup.db"
kubectl cp k8ops-system/<pod-name>:/data/k8ops-backup.db ./k8ops-backup.db
```

### Restauration

```bash
# Arrêter k8ops
kubectl scale deployment k8ops -n k8ops-system --replicas=0

# Restaurer la base de données
kubectl cp ./k8ops-backup.db k8ops-system/<pod-name>:/data/k8ops.db

# Redémarrer
kubectl scale deployment k8ops -n k8ops-system --replicas=1
```

---

## Déploiement haute disponibilité (HA)

### Mode nœud unique (par défaut, adapté au développement/petits clusters)

- 1 réplique + SQLite + PVC
- Brève interruption de service au redémarrage du Pod (~10s)
- Adapté aux équipes de moins de 50 utilisateurs

### HA multi-répliques (recommandé pour la production)

Utiliser MySQL/PostgreSQL à la place de SQLite, avec prise en charge multi-répliques :

1. **Basculer la base de données vers MySQL** :

```yaml
# Définir dans le ConfigMap de l'overlay
configMapGenerator:
- name: k8ops-config
  literals:
  - DB_DRIVER=mysql
  - DB_DSN=k8ops:password@tcp(mysql:3306)/k8ops?charset=utf8mb4&parseTime=True
```

2. **Multi-répliques + leader election** :

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

3. **Stockage partagé** : MySQL utilise un PVC indépendant, les Pods k8ops sont sans état

### Planification de capacité

| Taille | Utilisateurs | Ressources recommandées | Base de données |
|--------|-------------|------------------------|-----------------|
| Petite | < 20 | 1 pod, 500m CPU / 512Mi | SQLite |
| Moyenne | 20-100 | 2 pods, 1 CPU / 1Gi chacun | MySQL |
| Grande | 100+ | 3+ pods, 2 CPU / 2Gi chacun | MySQL + séparation lecture/écriture |

---

## Processus CI/CD et gestion des versions

### Script de déploiement en une commande

k8ops fournit un script de déploiement automatisé avec pré-vérification, construction, publication, vérification de santé et restauration automatique :

```bash
# Déployer une nouvelle version (pré-vérification auto + construction + publication + vérification de santé)
./scripts/deploy.sh v14.36

# Processus de déploiement :
# 1. Pré-vérification : go build + go vet + go test + gofmt
# 2. Construction : Docker buildx + push vers le registre
# 3. Publication : kubectl set image + annotation change-cause
# 4. Vérification : Pod Ready + HTTP 200 (délai 120s)
# 5. Restauration : retour automatique à la version précédente si la vérification de santé échoue
```

### Retour arrière rapide

```bash
# Revenir à la version précédente
./scripts/rollback.sh

# Revenir à une révision spécifique
./scripts/rollback.sh 58

# Revenir à un numéro de version spécifique
./scripts/rollback.sh v14.30
```

### Suivi de l'historique des versions

Chaque déploiement enregistre automatiquement l'annotation change-cause :

```bash
# Voir l'historique des versions
kubectl rollout history daemonset/k8ops -n k8ops-system

# Voir les détails d'une révision spécifique
kubectl rollout history daemonset/k8ops -n k8ops-system --revision=55
```

### Processus CI (GitHub Actions)

| Workflow | Déclencheur | Contenu |
|----------|-------------|---------|
| `ci.yml` — push/PR vers main | Soumission de code | test + vet + lint + govulncheck + Docker build |
| `release.yml` — tag v* | Tag de version | Tests complets + GoReleaser + Docker multi-arch + Release Notes automatiques |

### Gestion des images

| Tag | Description |
|-----|-------------|
| `registry.iot2.win/k8ops:v14.XX` | Version spécifique |
| `registry.iot2.win/k8ops:latest` | Dernière version stable |
| `ghcr.io/<org>/k8ops:v14.XX` | Image GHCR (publication CI) |

### Optimisation des images

- Image de base : `gcr.io/distroless/static-debian12:nonroot` (sans shell, sans gestionnaire de paquets)
- Construction multi-étapes : Go builder + runtime distroless
- Cache BuildKit : `--mount=type=cache` accélère la construction CI
- Optimisation binaire : `-trimpath -ldflags="-s -w"` réduit la taille

| Version | Taille de l'image |
|---------|-------------------|
| v14.30 (alpine) | 31.8 MB |
| v14.35 (distroless) | 28.6 MB |

### Configuration haute disponibilité

#### PodDisruptionBudget (PDB)

Garantit qu'au moins 1 Pod reste disponible pendant la maintenance des nœuds :

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

Limite le tableau de bord à n'accepter que le trafic provenant du contrôleur Ingress :

- Ingress : seul le namespace kube-system peut accéder au port 9090 (dashboard)
- Ingress : seul le namespace monitoring peut accéder au port 8080 (metrics)
- Egress : autorise DNS (53), HTTPS (443), K8s API (6443)

#### PriorityClass

k8ops utilise la priorité `system-cluster-critical` pour ne pas être expulsé en cas de pression sur les ressources.

#### Stratégie de mise à jour en continu

| Mode | maxUnavailable | maxSurge | Description |
|------|----------------|----------|-------------|
| DaemonSet | 1 | - | 1 nœud mis à jour à la fois |
| Deployment | 0 | 1 | Nouveau Pod démarré avant de supprimer l'ancien |

#### Quotas de ressources

| Mode | CPU Request | CPU Limit | Mem Request | Mem Limit |
|------|-------------|-----------|-------------|-----------|
| DaemonSet | 100m | 1 | 128Mi | 1Gi |
| Deployment | 500m | 2 | 512Mi | 2Gi |

#### Gestion des sondes de santé et du cycle de vie

k8ops utilise des sondes à trois niveaux pour assurer la fiabilité :

| Sonde | Chemin | Rôle | Paramètres |
|-------|--------|------|------------|
| **startupProbe** | `/healthz` | Attend la fin du démarrage (évite qu'un démarrage lent soit tué par la liveness) | failureThreshold: 30, period: 5s (attente max 150s) |
| **livenessProbe** | `/healthz` | Vérification de survie (redémarre le Pod en cas d'échec) | period: 20s, failureThreshold: 3, timeout: 5s |
| **readinessProbe** | `/readyz` | Vérification de disponibilité (retire des Endpoints du Service en cas d'échec) | period: 10s, failureThreshold: 3, timeout: 5s |

**Arrêt en douceur (Graceful Shutdown) :**

```yaml
lifecycle:
  preStop:
    exec:
      command: ["/manager", "--pre-stop"]
# --pre-stop dort 5s, attendant que le contrôleur Ingress retire ce Pod de l'équilibreur de charge
# Puis kubelet envoie SIGTERM, déclenchant l'arrêt en douceur du tableau de bord (drainage des connexions SSE)
# terminationGracePeriodSeconds: 30 garantit suffisamment de temps pour terminer
```

Processus d'arrêt :
1. kubelet exécute `preStop` → sleep 5s (drainage des connexions)
2. kubelet envoie SIGTERM → le gestionnaire de signaux Go commence l'arrêt en douceur
3. Le serveur HTTP du tableau de bord cesse d'accepter de nouvelles requêtes
4. Drainage des connexions SSE (délai 10s)
5. Arrêt en douceur du Controller Manager
6. Fin du processus
