# Guide d'exécution locale k8ops

> Aucun déploiement sur cluster Kubernetes requis : exécutez directement le binaire k8ops sur votre ordinateur portable/poste de travail.

---

## Cas d'usage

- **Développement et débogage local** — itération rapide du code, sans reconstruire d'image à chaque fois
- **Outil de gestion hors ligne** — comme alternative intelligente à kubectl
- **Démonstration et essai** — profitez de toutes les fonctionnalités sans déploiement dans le cluster
- **Intégration CI/CD** — exécuté comme outil de diagnostic dans les pipelines

---

## Prérequis

- Go 1.26+ (ou télécharger directement un binaire précompilé)
- kubectl configuré et capable de se connecter au cluster
- Clé API LLM (OpenAI / DeepSeek / ZAI, etc.)

---

## Méthode 1 : Compilation depuis les sources

```bash
cd k8ops

# Compiler manager (serveur du tableau de bord)
go build -o k8ops-manager ./cmd/manager/

# Compiler l'outil CLI
go build -o k8ops ./cmd/k8ops/
```

## Méthode 2 : Télécharger le binaire précompilé

Téléchargez le binaire correspondant à votre plateforme depuis [GitHub Releases](https://github.com/topcheer/k8ops/releases).

---

## Démarrer le tableau de bord

```bash
AIOPS_API_KEY=your-api-key \
  ./k8ops-manager \
  --leader-elect=false \
  --dashboard-address=:9090 \
  --auth-db-path=/tmp/k8ops.db
```

Après le démarrage, accédez à `http://localhost:9090`, connexion par défaut `admin / admin`.

### Description des paramètres

| Paramètre | Défaut | Description |
|-----------|--------|-------------|
| `--dashboard-address` | `:9090` | Adresse d'écoute du tableau de bord |
| `--leader-elect` | `false` | Leader Election (à désactiver en instance unique) |
| `--metrics-bind-address` | `:8080` | Port des métriques Prometheus |
| `--health-probe-bind-address` | `:8081` | Port des sondes de santé |
| `--auth-db-path` | `/data/k8ops.db` | Chemin de la base SQLite |
| `--auth-jwt-secret` | (généré aléatoirement) | Clé de signature JWT |
| `--provider-type` | `openai` | Fournisseur LLM |
| `--provider-model` | `gpt-4o` | Nom du modèle |
| `--provider-api-key` | (obligatoire) | Clé API LLM |
| `--provider-endpoint` | (par défaut) | Endpoint API personnalisé |

### Variables d'environnement

Tous les paramètres peuvent également être définis via des variables d'environnement :

```bash
export AIOPS_API_KEY=sk-your-key
export PROVIDER_TYPE=deepseek
export PROVIDER_MODEL=deepseek-chat
export AUTH_DB_PATH=$HOME/.k8ops/k8ops.db
export AUTH_JWT_SECRET=your-secret

./k8ops-manager --leader-elect=false
```

---

## Mécanisme de découverte kubeconfig

k8ops utilise `ctrl.GetConfigOrDie()` de controller-runtime pour découvrir automatiquement le kubeconfig, ordre de recherche :

1. Variable d'environnement `KUBECONFIG`
2. `~/.kube/config` (chemin par défaut)
3. Configuration in-cluster (`/var/run/secrets/kubernetes.io/serviceaccount/`)

En exécution locale, `~/.kube/config` est automatiquement utilisé, aucune configuration supplémentaire nécessaire.

### Spécifier un cluster

```bash
KUBECONFIG=~/.kube/prod-config ./k8ops-manager --leader-elect=false
```

### Bascule multi-cluster

```bash
# Basculer avec kubectx
kubectx prod-cluster
./k8ops-manager --leader-elect=false
```

---

## Différences de flux de données

### Exécution dans le cluster vs exécution locale

| Dimension | Dans le cluster (DaemonSet/Deployment) | Exécution locale |
|-----------|---------------------------------------|------------------|
| Authentification API K8s | Jeton ServiceAccount | kubeconfig |
| Outils Host | `nsenter` pour accéder à l'hôte | Exécution directe sur la machine locale |
| Données d'authentification | Persistance PVC | Fichier SQLite local |
| Leader Election | Nécessaire en multi-répliques | Désactivé en instance unique |
| Emprunt d'identité RBAC | Utilisateur → ServiceAccount | Utilisateur → utilisateur kubeconfig |
| Autorisations réseau | Réseau du Pod | Réseau local |
| Sortie des journaux | stdout → kubectl logs | Sortie directe dans le terminal |

### Comportement des outils hôte

Dans un conteneur, les outils Host accèdent à l'espace de noms de l'hôte via `nsenter -m -u -i -n -p --`. En exécution locale, ils s'exécutent directement via `/bin/sh -c`, accédant au système d'exploitation local.

Cela signifie que :
- `host_disk_check` vérifie le disque local
- `host_process_list` liste les processus locaux
- `host_exec` exécute des commandes localement

---

## Utilisation de l'outil CLI

```bash
# Diagnostic
./k8ops diagnose --problem "pods in production keep CrashLoopBackOff"

# Voir les suggestions d'optimisation
./k8ops optimize --namespace production

# Déclencher une remédiation
./k8ops remediate --plan <plan-name> --approve
```

---

## Exécution en arrière-plan

### macOS (launchd)

```bash
cat > ~/Library/LaunchAgents/dev.ggai.k8ops.plist << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>dev.ggai.k8ops</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/k8ops-manager</string>
        <string>--leader-elect=false</string>
        <string>--dashboard-address=:9090</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>AIOPS_API_KEY</key>
        <string>your-api-key</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
EOF

launchctl load ~/Library/LaunchAgents/dev.ggai.k8ops.plist
```

### Linux (systemd)

```bash
sudo tee /etc/systemd/system/k8ops.service << 'EOF'
[Unit]
Description=k8ops AI Operations
After=network.target

[Service]
ExecStart=/usr/local/bin/k8ops-manager --leader-elect=false --dashboard-address=:9090
Environment=AIOPS_API_KEY=your-api-key
Environment=AUTH_DB_PATH=/var/lib/k8ops/k8ops.db
Restart=always
User=k8ops

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl enable --now k8ops
```

---

## Mode développement

### Rechargement à chaud

```bash
# Installer air
go install github.com/air-verse/air@latest

# À la racine du projet k8ops
air --build.cmd "go build ./cmd/manager/" --build.bin "./manager"
```

### Débogage

```bash
# Activer les journaux DEBUG
DEBUG=true ./k8ops-manager --leader-elect=false

# Voir les journaux structurés JSON
tail -f /tmp/k8ops.log
```

---

## Dépannage

### « unable to get kubeconfig »

Assurez-vous que `~/.kube/config` existe et est valide :
```bash
kubectl cluster-info  # Tester le kubeconfig
```

### « address already in use :9090 »

```bash
# Voir le processus utilisant le port 9090
lsof -i :9090
# Ou changer de port
./k8ops-manager --dashboard-address=:9091
```

### Base d'authentification verrouillée

Supprimer le fichier DB et réinitialiser :
```bash
rm /tmp/k8ops.db
./k8ops-manager --auth-db-path=/tmp/k8ops.db
```

### Délai d'expiration du fournisseur

Définir un timeout plus long ou vérifier le réseau :
```bash
export PROVIDER_ENDPOINT=https://api.openai.com/v1
# Confirmer l'accessibilité réseau
curl -I https://api.openai.com/v1/models
```
