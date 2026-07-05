# Guide de l'Utilisateur k8ops

> De l'installation à la maîtrise, un guide détaillé couvrant toutes les fonctionnalités.

---

## Table des matières

1. [Démarrage Rapide](#1-démarrage-rapide)
2. [Aperçu du Cluster](#2-aperçu-du-cluster)
3. [Chat IA — Assistant intelligent](#3-chat-ia--assistant-intelligent)
4. [Diagnostic et Réparation](#4-diagnostic-et-réparation)
5. [Recommandations d'Optimisation](#5-recommandations-doptymisation)
6. [Analyse des Coûts (FinOps)](#6-analyse-des-coûts-finops)
7. [Visualisation de la Topologie du Cluster](#7-visualisation-de-la-topologie-du-cluster)
8. [Gestion des Nœuds et Pods](#8-gestion-des-nœuds-et-pods)
9. [Flux d'Événements et Notifications](#9-flux-dévénements-et-notifications)
10. [Explorateur de Ressources et Éditeur YAML](#10-explorateur-de-ressources-et-éditeur-yaml)
11. [Contrôle d'Accès RBAC](#11-contrôle-daccès-rbac)
12. [Journal d'Audit](#12-journal-daudit)
13. [Paramètres et Configuration](#13-paramètres-et-configuration)
14. [Raccourcis Clavier](#14-raccourcis-clavier)
15. [Changement de Thème](#15-changement-de-thème)
16. [Planification de la Capacité](#16-planification-de-la-capacité)
17. [Visualisation HPA](#17-visualisation-hpa)
18. [Inventaire des Images de Conteneurs](#18-inventaire-des-images-de-conteneurs)
19. [Classement des Ressources par Namespace](#19-classement-des-ressources-par-namespace)
20. [Conformité de Sécurité](#20-conformité-de-sécurité)
21. [Gestion du Système](#21-gestion-du-système)
22. [API de Diagnostic des Opérations](#22-api-de-diagnostic-des-opérations-v1461)

---

## 1. Démarrage rapide

### Première connexion

1. Ouvrez votre navigateur et accédez à l'adresse de k8ops (par exemple, `https://k8ops.iot2.win` ou `http://localhost:9090`)
2. Compte par défaut : `admin` / `admin`
3. Lors de la première connexion, vous serez invité à changer le mot de passe

### Disposition de la page

```
┌─────────┬───────────────────────────────┐
│         │  [Namespace ▼]  [🔔]  [☀/☽]  │  ← Barre supérieure
│ Sidebar ├───────────────────────────────┤
│         │                                │
│ Overview│       Content Area             │  ← Zone de contenu
│ Diagnose│                                │
│ Nodes   │                                │
│ Pods    │                                │
│ ...     │                                │
└─────────┴───────────────────────────────┘
```

### Palette de commandes Ctrl+K

Appuyez sur `Ctrl+K` (Mac : `Cmd+K`) à tout moment pour ouvrir la palette de commandes globale :

- Tapez `nodes` → aller à la page des nœuds
- Tapez `chat` → ouvrir le chat IA
- Tapez `cost` → voir l'analyse des coûts
- Utilisez les flèches pour sélectionner, Entrée pour confirmer, Échap pour fermer

---

## 2. Aperçu du cluster

La page Overview affiche l'état global du cluster.

### Cartes de statistiques

| Carte | Signification |
|-------|---------------|
| Nodes | Nombre total de nœuds du cluster / nombre Ready |
| Pods | Pods en cours d'exécution / total |
| CPU | Utilisation CPU de l'ensemble du cluster |
| Memory | Utilisation mémoire de l'ensemble du cluster |
| Warnings | Nombre d'événements Warning actuels |

### Graphiques de tendance sparkline

Sous chaque carte se trouve un mini graphique linéaire SVG montrant l'évolution des tendances des 30 dernières minutes.

### Changement de namespace

Le sélecteur déroulant en haut à gauche permet de changer le périmètre du namespace. Ce changement affecte les pages Pods, Events, Nodes, etc. La sélection est conservée dans localStorage.

---

## 3. Chat IA — Assistant intelligent

Cliquez sur le bouton Chat en bas de la barre latérale ou appuyez sur `Ctrl+K` et tapez `chat` pour l'ouvrir.

### Utilisation de base

Saisissez votre question dans le champ de saisie, l'IA :

1. Comprendra l'intention en langage naturel
2. Invoquera automatiquement l'outil K8s approprié
3. Renvoie les résultats d'analyse en streaming

### Exemples de requêtes

```
# Voir les ressources
voir les pods du namespace default
quels nœuds ont une utilisation CPU élevée ?

# Diagnostic d'incidents
pourquoi les pods de nginx-deployment sont en CrashLoopBackOff ?
quelles anomalies y a-t-il dans le cluster ?

# Recommandations d'optimisation
aide-moi à analyser l'utilisation des ressources
quels pods peuvent réduire leur nombre de répliques ?
```

### Transparence des appels d'outils

Lorsque l'IA exécute des appels d'outils, un panneau Thinking repliable s'affiche :

- Cliquez pour développer et voir les paramètres et résultats de chaque appel d'outil
- Affichage au format JSON, avec prise en charge de la recherche

### Cartes de recommandations de diagnostic

Lorsque l'IA suggère d'exécuter des commandes kubectl, sous le bloc de code s'affiche :

- **▶ Run in Chat** — charge la commande dans le champ de saisie pour un envoi facile
- **📋 Copy** — copie la commande dans le presse-papiers

### Gestion des sessions

- **New** — créer une nouvelle session
- **Liste des sessions à gauche** — cliquez pour basculer entre les sessions historiques
- Les sessions sont automatiquement résumées et compressées (déclenché automatiquement au-delà de 20k tokens)

### Rendu Markdown

Chat prend en charge :
- Les blocs de code (avec coloration syntaxique et bouton de copie)
- Les tableaux
- Les listes, le gras, l'italique
- Les liens (uniquement les protocoles http/https/mailto)

---

## 4. Diagnostic et réparation

### Déclencher un diagnostic

**Méthode 1 : Interface Web**

1. Allez sur la page Diagnostics
2. Cliquez sur "New Diagnostic"
3. Remplissez la description du problème (par exemple, "la réponse de l'API du namespace production est devenue lente")
4. Après soumission, l'IA analyse automatiquement

**Méthode 2 : Chat IA**

Décrivez le problème directement dans Chat, l'IA exécutera automatiquement le flux de diagnostic.

**Méthode 3 : CRD**

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

**Méthode 4 : CLI**

```bash
k8ops diagnose --problem "pods in production keep CrashLoopBackOff"
```

### Résultats du diagnostic

Chaque rapport de diagnostic contient :

- **Cause racine** — la cause racine analysée par l'IA
- **Preuves** — journaux, événements et données de métriques étayant l'analyse
- **Recommandations** — actions de réparation recommandées
- **Gravité** — niveau de gravité (Info / Avertissement / Critique)

### Réparation automatique (remédiation)

Les plans de réparation générés par l'IA nécessitent une approbation manuelle :

1. Allez sur la page Remediations
2. Examinez les plans de réparation en attente d'approbation
3. Cliquez sur **Approve** pour exécuter ou **Reject** pour refuser
4. Toutes les opérations sont enregistrées dans le journal d'audit

---

## 5. Recommandations d'optimisation

La page Optimizations affiche les recommandations de l'IA pour l'optimisation des ressources du cluster.

### Types de recommandations

| Type | Description |
|------|-------------|
| Resource Rightsizing | Suggestions d'ajustement des requests et limits CPU/Memory |
| HPA Gap | Deployments sans configuration de mise à l'échelle horizontale |
| PDB Gap | Charges de travail sans PodDisruptionBudget |
| Cost Saving | Coûts pouvant être économisés (ressources inactives, répliques excessives, etc.) |

### Opérations

- Cliquez sur une recommandation pour voir les détails
- Vous pouvez l'appliquer directement ou l'ignorer

---

## 6. Analyse des coûts (FinOps)

La page Cost offre une visibilité sur les coûts du cluster.

### Fonctionnalités

- **Résumé des coûts par namespace** — affiche la consommation de ressources et le coût estimé par namespace
- **Taux d'utilisation des ressources** — utilisation réelle vs allocation CPU/Memory
- **Recommandations de Rightsizing** — suggestions d'ajustement pour les ressources surallouées
- **Ressources inactives** — PV, LoadBalancer, IP élastique, etc. inutilisés depuis longtemps

---

## 7. Visualisation de la topologie du cluster

La page Topology affiche la relation entre les nœuds et les Pods sous forme de graphiques SVG.

### Éléments visuels

| Élément | Signification |
|---------|---------------|
| Cadre vert | Nœud Ready |
| Cadre rouge | Nœud NotReady |
| Barre de progression dans le nœud | Utilisation CPU (haut) / MEM (bas) |
| Point vert de Pod | Running |
| Point jaune de Pod | Pending |
| Point rouge de Pod | Failed |
| Bordure clignotante du Pod | CrashLoop (restarts > 3) |

### Interaction

- **Clic sur Pod** — ouvre la visionneuse de logs de ce Pod
- **Statistiques en bas** — nombre de nœuds Ready/NotReady, résumé de l'état des Pods

---

## 8. Gestion des nœuds et pods

### Page Nœuds

- Tableau de liste des nœuds : nom, rôle, état, CPU, mémoire, nombre de Pods
- Chaque colonne prend en charge la recherche et le filtrage
- Cliquez sur le nom du nœud pour voir les informations détaillées et tous les Pods de ce nœud

### Page Pods

- Tableau de liste des Pods : nom, namespace, état, nombre de redémarrages, nœud, âge
- Prend en charge le filtrage par namespace et la recherche en temps réel

### Visionneuse de journaux de pod

Cliquez sur une ligne de Pod pour ouvrir la visionneuse de logs :

- **Streaming en temps réel** — via SSE, les logs sont mis à jour en temps réel
- **Coloration par niveau de log** — ERROR (rouge), WARN (jaune), DEBUG (gris)
- **Recherche et filtrage** — tapez des mots-clés pour filtrer les lignes de log
- **Défilement automatique** — à l'arrivée de nouveaux logs, défilement automatique vers le bas (peut être mis en pause)
- **Téléchargement** — exporte les logs actuels dans un fichier

---

## 9. Flux d'événements et notifications

### Page Événements

Affiche les événements du cluster K8s et prend en charge :

- La recherche et le filtrage en temps réel
- La mise en évidence en rouge des événements Warning
- Le filtrage par namespace

### Flux d'événements en temps réel

Le côté droit de la page Events possède un panneau Live Events :

- Cliquez sur **Go Live** pour activer le streaming en temps réel via SSE
- Les nouveaux événements ont une animation avec un badge bleu NEW
- Les événements supprimés ont un badge rouge DEL
- Les événements Warning sont automatiquement mis en évidence en rouge

### Centre de notifications

L'icône de cloche en haut à droite de la barre supérieure :

- Affiche un badge numérique rouge + animation de pulsation en cas d'alerte
- Cliquez pour développer le panneau déroulant
- Affiche les événements Warning récents et les nœuds NotReady
- Actualisation automatique toutes les 60 secondes

---

## 10. Explorateur de ressources et éditeur YAML

### Page Ressources

Parcourez toutes les ressources K8s du cluster :

- Groupées par API Group / Resource Type
- Cliquez sur le nom de la ressource pour voir la définition YAML
- Prend en charge le filtrage multi-sélection par namespace

### Visionneuse YAML

Cliquez sur n'importe quelle ressource pour ouvrir la vue YAML en superposition :

- Affichage formaté du YAML complet
- Bouton **Copy** pour copier en un clic

### Éditeur YAML

Cliquez sur le bouton **Edit** dans la visionneuse YAML pour passer en mode édition :

1. Le contenu YAML devient un textarea modifiable
2. Après modification, cliquez sur **Apply** pour soumettre
3. Le backend utilise le server-side apply (sémantique de kubectl apply)
4. Affiche un message vert en cas de succès, ou un message d'erreur rouge en cas d'échec

---

## 11. Contrôle d'accès RBAC

La page RBAC (nécessite des droits d'administrateur) gère les utilisateurs et les rôles.

### Gestion des utilisateurs

- **Créer un utilisateur** — nom d'utilisateur, mot de passe, rôle, périmètre de namespace
- **Modifier un utilisateur** — changer le rôle, activer/désactiver
- **Supprimer un utilisateur**

### Rôles

| Rôle | Permissions |
|------|-------------|
| admin | Lecture/écriture sur tout le cluster, peut gérer les utilisateurs |
| operator | Lecture/écriture sur la plupart des ressources, ne peut pas gérer RBAC/Secrets |
| viewer | Accès en lecture seule |

### Périmètre de namespace

Chaque utilisateur peut être lié à un namespace spécifique et ne peut accéder qu'aux ressources dans ce périmètre (implémenté via K8s impersonation).

---

## 12. Journal d'audit

La page Audit affiche les enregistrements d'audit de toutes les opérations IA.

### Fonctionnalités

- **Filtrage par gravité** — liste déroulante pour sélectionner Info / Avertissement / Erreur / Critique
- **Recherche en temps réel** — tapez des mots-clés pour filtrer
- **Cartes de statistiques** — Total / Successful / Failed / Critical / Warnings
- **Tableau** — heure, niveau de gravité, action, ressource cible, opérateur, succès/échec, durée

### Périmètre d'Audit

Toutes les opérations suivantes sont enregistrées :

- Appels d'outils IA (kubectl get/describe/logs, etc.)
- Opérations de réparation initiées par l'IA
- Appels à l'API LLM
- Connexion/déconnexion des utilisateurs
- Modifications de ressources

---

## 13. Paramètres et configuration

La page Paramètres configure le fournisseur d'IA et l'authentification.

### Configuration du fournisseur d'IA

| Champ | Description |
|-------|-------------|
| Provider Type | openai / deepseek / zai / anthropic |
| Model | gpt-4o / deepseek-chat / glm-4-plus, etc. |
| Endpoint | Adresse de l'API LLM (laisser vide pour utiliser la valeur par défaut) |
| Clé API | Clé de l'API LLM |

### Configuration de l'Authentification

- **Local** — système d'utilisateurs intégré (par défaut)
- **LDAP** — intégration LDAP/AD d'entreprise
- **OIDC** — GitHub / Google / Keycloak, etc.

---

## 14. Raccourcis clavier

| Raccourci | Fonction |
|-----------|----------|
| `Ctrl+K` / `Cmd+K` | Ouvrir la palette de commandes |
| `Esc` | Fermer la palette de commandes / fenêtres contextuelles |
| `↓` / `↑` | Sélectionner dans la palette de commandes |
| `Enter` | Confirmer dans la palette de commandes |

---

## 15. Changement de thème

Cliquez sur le bouton lune/soleil en haut à droite de la barre latérale pour basculer entre le thème sombre/clair. La sélection est conservée dans localStorage et persiste après le rafraîchissement de la page.

---

## Annexe

### Documentation connexe

- [Conception de l'Architecture](ARCHITECTURE.md)
- [Guide de Déploiement](DEPLOYMENT.md)
- [Exécution Locale](LOCAL_RUN.md)
- [Référence de l'API](API.md)
- [Politique de Sécurité](SECURITY.md)

### Questions fréquentes

**Q : Chat ne répond pas ?**
R : Vérifiez que la configuration dans Paramètres → Fournisseur est correcte et que la clé API est valide.

**Q : Je ne vois pas certains namespaces ?**
R : Le rôle RBAC de l'utilisateur actuel peut limiter le périmètre d'accès aux namespaces. Contactez l'administrateur pour ajuster.

**Q : La visionneuse de logs du Pod est vide ?**
R : Le Pod vient peut-être de démarrer et n'a pas encore de logs, ou il n'a pas les permissions de logs. Vérifiez la configuration RBAC.

**Q : Les commandes suggérées par l'IA sont-elles sûres ?**
R : Toutes les opérations suggérées par l'IA passent d'abord par une validation dry-run du Safety Checker. Les opérations de réparation nécessitent une approbation manuelle avant exécution.

---

## 16. Planification de la capacité

### Surveillance de la capacité de stockage

**Chemin :** Dashboard → onglet Capacity

Affiche l'état de stockage de tous les PVC (PersistentVolumeClaim) du cluster :

| Métrique | Description |
|----------|-------------|
| Total PVCs | Nombre total de PVC dans le cluster |
| Bound | Nombre de PVC liés à un PV |
| Pending | PVC en attente de liaison |
| Total Capacity | Capacité totale de tous les PVC |
| Requested | Quantité totale demandée par tous les PVC |

### Analyse de la capacité des nœuds

La page Capacity affiche également l'utilisation des ressources de chaque nœud :

- **Utilisation CPU** : CPU demandé / CPU allouable (code couleur : <60% vert, 60-80% jaune, >80% rouge)
- **Utilisation mémoire** : mémoire demandée / mémoire allouable
- **Densité de Pods** : nombre de Pods en cours d'exécution / limite maximale de Pods
- **Recommandations d'expansion** : lorsque les ressources du nœud dépassent 80%, des recommandations d'expansion sont générées automatiquement

### Résumé au niveau du cluster

| Métrique | Description |
|----------|-------------|
| Cluster CPU Utilization | Ratio CPU demandé/allouable pour l'ensemble du cluster |
| Cluster Mem Utilization | Ratio mémoire demandée/allouable pour l'ensemble du cluster |
| Total CPU Allocatable | Quantité totale de CPU allouable dans le cluster |
| Total CPU Requested | Quantité totale de CPU demandée dans le cluster |

---

## 17. Visualisation HPA

**Chemin :** Dashboard → onglet HPA

Affiche l'état de mise à l'échelle automatique de tous les HorizontalPodAutoscaler :

### Fonctionnalités

- **Barre d'échelle des répliques** : visualise le nombre actuel de répliques, le nombre souhaité et la plage min/max
- **Barre d'utilisation des métriques** : utilisation actuelle CPU/mémoire vs valeur cible (vert/jaune/rouge)
- **Indicateur d'état de mise à l'échelle** : affiche un badge "SCALING" lorsque les répliques actuelles ≠ souhaitées
- **Cartes de résumé** : nombre total de HPA, nombre en cours de mise à l'échelle, total des répliques actuelles/souhaitées

### Types de métriques prises en charge

| Type | Description |
|------|-------------|
| Resource | Pourcentage d'utilisation CPU/mémoire |
| Pods | Métriques personnalisées de Pod (comme QPS) |
| External | Métriques externes (comme la longueur de file SQS) |
| ContainerResource | Métriques de ressources au niveau du conteneur |

---

## 18. Inventaire des images de conteneurs

**Chemin :** Dashboard → onglet Images

Affiche toutes les images de conteneurs utilisées dans le cluster :

| Métrique | Description |
|----------|-------------|
| Unique Images | Nombre total d'images uniques (dédupliquées) |
| Using :latest | Nombre d'images avec le tag `:latest` (non recommandé pour la production) |
| No Limits | Nombre d'images sans resource limits |
| No Requests | Nombre d'images sans resource requests |
| Registries | Nombre de registries d'images utilisés |

### Meilleures pratiques de sécurité

- Évitez d'utiliser le tag `:latest` — utilisez des numéros de version fixes pour garantir des déploiements reproductibles
- Tous les conteneurs devraient avoir des limits CPU/mémoire — pour éviter l'épuisement des ressources
- Tous les conteneurs devraient avoir des requests CPU/mémoire — pour garantir une allocation correcte par le planificateur

---

## 19. Classement des ressources par namespace

**Chemin :** Dashboard → onglet Namespaces

Liste l'utilisation des ressources de tous les namespaces classés par consommation CPU :

### Fonctionnalités

- **Résumé des ressources** : requests + limits CPU/mémoire, nombre de Pods, stockage PVC pour chaque namespace
- **Part du cluster** : pourcentage de CPU/mémoire demandé par rapport au total allouable du cluster (avec barre de progression visuelle)
- **Recherche et filtrage** : localisez rapidement un namespace spécifique
- **Exploration détaillée** : cliquez sur n'importe quel namespace pour voir l'utilisation de ResourceQuota, la configuration de LimitRange et les événements Warning récents

---

## 20. Conformité de sécurité

### Analyse de conformité CIS Benchmark

**Chemin :** Dashboard → onglet Compliance

Exécute les vérifications CIS Kubernetes Benchmark, couvrant les catégories suivantes :

| Catégorie | Éléments de Vérification |
|-----------|--------------------------|
| RBAC | Portée des liaisons cluster-admin, ClusterRole avec caractères génériques, utilisation du SA par défaut |
| Pod Security | Conteneurs privilégiés, hostNetwork/hostPID/hostIPC, volumes hostPath, utilisateur root, limits de ressources |
| Network | Couverture NetworkPolicy |
| Secrets | État de gestion des Secrets |

### Téléchargement du rapport de conformité

Cliquez sur le bouton "Download Report" pour télécharger le rapport de conformité complet (format .txt), qui comprend :

- Score de conformité (pourcentage)
- Statut de chaque vérification (PASS/WARN/FAIL)
- Recommandations de correction (pour les éléments WARN/FAIL)

### Recherche d'Événements d'Audit

**Chemin :** API → `GET /api/audit/events`

Prend en charge le filtrage du journal d'audit selon plusieurs dimensions :

| Paramètre | Description |
|-----------|-------------|
| `actor` | Filtrer par nom d'utilisateur |
| `action` | Filtrer par type d'opération (comme delete, scale, exec) |
| `q` | Recherche en texte intégral |
| `severity` | Filtrer par niveau de gravité |
| `from`/`to` | Plage temporelle (format RFC3339) |

### Exportation CSV

`GET /api/audit/export` — exporte les logs d'audit au format CSV, importable dans un système SIEM pour analyse de conformité.

---

## 21. Gestion du système

### Informations système

`GET /api/system/info` fournit des informations d'exécution :

- Numéro de version, version de Go, plateforme d'exécution
- Utilisation de la mémoire (Alloc/Sys/GC cycles/Heap objects)
- Nombre de goroutines
- Temps de fonctionnement du service
- Taille et nombre d'événements du journal d'audit

### Gestion des journaux

| API | Fonction |
|-----|----------|
| `POST /api/system/log/rotate` | Déclencher manuellement la rotation du journal d'audit (admin) |
| `POST /api/system/log/cleanup` | Nettoyer les fichiers de rotation de plus de 30 jours (admin) |

### Configuration du niveau de journalisation

Configurer via la variable d'environnement `LOG_LEVEL` (debug/info/warn/error) :

```bash
kubectl set env daemonset/k8ops -n k8ops-system LOG_LEVEL=debug
kubectl rollout restart daemonset/k8ops -n k8ops-system
```

### Gestion des Sauvegardes

| API | Fonction |
|-----|----------|
| `GET /api/system/backup` | Lister tous les fichiers de sauvegarde |
| `POST /api/system/backup` | Créer une sauvegarde de la base de données |
| `DELETE /api/system/backup?name=X` | Supprimer une sauvegarde spécifique |
| `POST /api/system/backup/restore?name=X` | Restaurer la base de données depuis une sauvegarde |

### Surveillance des Performances de l'API

`GET /api/system/performance` fournit des statistiques de latence pour chaque endpoint de l'API :

- Latence aux percentiles **p50/p95/p99**
- Latence moyenne et maximale
- Taux d'erreur et nombre total de requêtes

---

## 22. API de diagnostic des opérations (v14.61+)

### Audit des Network Policies

`GET /api/security/network-policies` audite la couverture NetworkPolicy du cluster :

- Détection des namespaces sans NetworkPolicy (totalement ouverts par défaut)
- Identification des politiques permissives (0.0.0.0/0 entrant/sortant)
- Classification par niveau de gravité : critical / warning / info
- Chaque constatation inclut une description détaillée et des recommandations de correction

### Diagnostic des Redémarrages de Pod

`GET /api/diagnostics/restarts` diagnostique les motifs et causes racines des redémarrages de Pod :

- Classification des motifs de redémarrage : crash-loop / occasional / post-deploy
- Extraction de la cause d'arrêt : OOMKilled / Error / code de sortie
- Identification des états d'attente : CrashLoopBackOff / ImagePullBackOff
- Informations de diagnostic indépendantes pour chaque conteneur

### État du Rollout des Déploiements

`GET /api/deployments/rollout` suit l'état de santé du rollout de toutes les charges de travail :

- Couvre Deployment / StatefulSet / DaemonSet
- 7 états : complete / in-progress / stalled / degraded / paused / failed / scaled-to-zero
- Détection de ProgressDeadlineExceeded et ReplicaFailure
- Prend en charge le filtrage par état : `?status=failed`

### Détection du Gaspillage de Ressources

`GET /api/resources/waste` analyse les ressources gaspillées et orphelines pour réduire les coûts :

- 6 catégories de gaspillage : services morts, PVC inutilisés, ConfigMap/Secret orphelins, namespaces vides, PV non liés
- Évaluation du risque de coût : low / moderate / high
- Chaque élément comprend la gravité, l'âge et des recommandations de nettoyage
- Filtrage intelligent des ressources système (kube-system, SA token, Helm release)

### Détection des Goulots d'Étranglement de Mise à l'Échelle

`GET /api/scaling/bottlenecks` identifie les facteurs limitant la mise à l'échelle horizontale :

- 7 catégories de goulots d'étranglement : planification de nœuds, pression des nœuds, limites de quota, HPA bloqué, PDB bloquant, épuisement du stockage
- Résumé de la capacité du cluster : nombre de nœuds, CPU/mémoire, capacité de Pods, marge d'expansion
- Chaque élément comprend le niveau d'impact et des recommandations de correction

### Analyse des Risques de Permissions RBAC

`GET /api/security/rbac-risk` analyse les risques de sécurité de toutes les liaisons RBAC du cluster :

- Système de score 0-100, identifie automatiquement les liaisons à haut risque
- 5 niveaux de risque : critical / high / elevated / moderate / low
- Éléments de détection : liaisons cluster-admin, élévation de privilèges (escalate/bind/impersonate), permissions génériques (verbs/resources : *), écriture à l'échelle du cluster, accès aux ressources sensibles (secrets/pods/exec)
- Chaque élément comprend une ventilation détaillée du score et des recommandations de correction (principe du moindre privilège)
- Prend en charge le filtrage par namespace : `?namespace=default`

### Surveillance de la Santé d'Exécution des CronJobs

`GET /api/operations/cronjobs/health` surveille la santé d'exécution de tous les CronJobs :

- 5 niveaux d'état de santé : healthy / warning / failing / suspended / no-runs
- Détection des échecs consécutifs (3 ou plus = failing), taux de réussite inférieur à 50%, planifications suspendues, jamais exécutés
- Association des CronJobs et de leurs Jobs enfants via OwnerReferences
- Calcul du prochain temps d'exécution prévu
- Prend en charge le filtrage par namespace : `?namespace=production`

### Surveillance de la Santé Réseau des Services et Endpoints

`GET /api/networking/health` analyse la connectivité réseau de tous les Services et Ingress :

- 5 niveaux d'état de santé des Services : healthy / degraded / no-endpoints / misconfigured / external
- Détection des sélecteurs non correspondants (label mismatch), tous les endpoints indisponibles, dégradation partielle, LoadBalancer en attente d'IP
- Validation du backend Ingress : existence du Service backend et disponibilité des endpoints
- Référence croisée de la correspondance des sélecteurs de Pod, fournissant une analyse de cause racine
- Prend en charge le filtrage par namespace : `?namespace=default`

### Surveillance de la Santé du Stockage PV/PVC

`GET /api/storage/health` analyse la santé du stockage de tous les PVC/PV :

- 6 niveaux d'état de santé des PVC : bound / pending / lost / failed / orphaned / near-capacity
- Diagnostic des PVC Pending : aucune classe de stockage, mode de liaison WaitForFirstConsumer, vérification des logs du provisioner
- Détection des PVC orphelins : liés mais non utilisés par aucun Pod depuis plus de 1 jour (gaspillage de capacité)
- Problèmes de PV : Released (nécessite un nettoyage manuel), Failed (échec du recyclage), Available obsolète (>7 jours)
- Distribution des StorageClass : classe par défaut, provisioner, reclaim policy, prise en charge de volume expansion
- Prend en charge le filtrage par namespace : `?namespace=default`

### Audit de Sécurité des ServiceAccounts

`GET /api/security/service-accounts` audite de manière exhaustive les risques de sécurité de toutes les ServiceAccounts du cluster :

- Système de score de risque 0-100, identifie automatiquement les SA à haut risque
- 5 niveaux de gravité : critical / high / elevated / moderate / low
- Éléments de détection : SA inutilisés (>7 jours), liaison cluster-admin (critical), SA par défaut utilisés par des Pods, montage automatique de token inutile, SA obsolètes (>30 jours avec permissions mais sans utilisation), secret de token hérité de longue durée
- Chaque élément comprend une explication détaillée du risque de sécurité et des recommandations de correction
- Prend en charge le filtrage par namespace : `?namespace=default`

### Suivi du Budget d'Erreurs SLO/SLA

`GET /api/operations/slo` suit la conformité SLO/SLA basée sur un algorithme multi-fenêtres et multi-taux de combustion :

- 5 fenêtres temporelles : 5 minutes, 1 heure, 6 heures, 24 heures, 7 jours
- Pourcentage de disponibilité et quantité restante/taux de consommation du budget d'erreurs
- Détection du taux de combustion multi-fenêtres (fast : 5m+1h, slow : 6h+24h)
- Percentiles de latence P50/P95/P99 et objectif SLO
- 3 niveaux d'état : meeting (conforme) / at-risk (à risque) / violated (violé)
- Prend en charge le filtrage par namespace : `?namespace=production`

### Surveillance des ResourceQuota et LimitRange

`GET /api/resources/quota` analyse l'utilisation des quotas et les contraintes LimitRange de tous les namespaces :

- 4 niveaux d'état de quota : ok (<70%) / warning (70-85%) / critical (85-100%) / exceeded (>100%)
- Utilisation des quotas CPU/mémoire/Pod/ConfigMap/Secret/stockage par namespace
- Identification des namespaces sans protection de quota
- Analyse des contraintes par défaut/minimum/maximum de LimitRange
- Classement des principaux consommateurs
- Prend en charge le filtrage par namespace : `?namespace=default`

### Audit de la Configuration des Déploiements

`GET /api/deployments/audit` audite les violations des meilleures pratiques de configuration de toutes les charges de travail :

- 8 catégories de vérification : revision-history / image-policy / resources / probes / security-context / update-strategy / lifecycle / config-drift
- Chaque élément comprend la gravité (critical/warning/info), une description spécifique du problème et des recommandations de correction exploitables
- Score de santé de 0 (parfait) à 100 (pire)
- Principales constatations agrégées montrant les problèmes les plus courants à l'échelle du cluster
- Prend en charge le filtrage par namespace et par gravité : `?namespace=default&severity=critical`

### Santé de la Planification et Analyse de la Fragmentation des Ressources

`GET /api/scheduling/health` analyse la santé de la planification du cluster et l'utilisation des ressources :

- Planifiabilité par nœud (isolation/taint/conditions de pression) et disponibilité des ressources
- Diagnostic des Pods Pending : analyse des causes des événements FailedScheduling (CPU/mémoire insuffisants, taint non correspondant, conflit de nodeSelector, échec de liaison de volume, etc.)
- Calcul du Pod planifiable maximum (le plus grand Pod déployable actuellement)
- Capacité effective vs capacité théorique (perte de capacité due aux nœuds non planifiables)
- Analyse de la fragmentation des ressources (capacité libre dispersée)
- Détection des Pods surdimensionnés (demandes dépassant la capacité de tout nœud individuel)
- Historique des évictions sur 24h (avec causes)
- Score de santé 0-100 (pénalisation pondérée)
- Recommandations de correction exploitables
- Prend en charge le filtrage par namespace : `?namespace=default`

### Analyse de la Posture de Sécurité des Pods

`GET /api/security/pods?namespace=xxx&severity=critical` audite la posture de sécurité en temps réel de tous les Pods en cours d'exécution :

- 15 catégories de vérification couvrant les conteneurs privilégiés, l'accès hôte (network/PID/IPC), les montages HostPath, les capabilities dangereuses, l'exécution en root, l'élévation de privilèges, etc.
- Score de risque par Pod de 0-100 (critical=25 points/warning=8 points/info=2 points)
- Statistiques agrégées par type de vérification et par namespace
- Prend en charge le filtrage par namespace et par gravité

### Détection des Tempêtes d'Événements et des Défaillances en Cascade

`GET /api/operations/event-storm?namespace=xxx` analyse les événements Warning du cluster :

- 4 niveaux de gravité de tempête : critical (>50) / high (>20) / medium (>10) / low (>5)
- Détection des ressources fluctuantes (même ressource, même cause répétée 3+ fois, avec fréquence de fluctuation)
- Agrégation par namespace et par cause d'événement
- Évaluation du rayon d'impact (nombre de ressources affectées)
- Recommandations d'investigation exploitables
- Prend en charge le filtrage par namespace : `?namespace=kube-system`

### Graphe des Dépendances de Ressources et Analyse d'Impact

`GET /api/dependencies?kind=Deployment&name=xxx&namespace=xxx` suit le graphe complet des dépendances des charges de travail :

- Dépendances directes : ConfigMap, Secret, PVC, ServiceAccount
- Dépendances inverses : Service (label selector), Ingress, NetworkPolicy, HPA, autres Pods partageant la configuration
- Évaluation de la portée de l'impact : score blastRadius et niveau de risque
- Utile pour l'évaluation d'impact avant changement, afin d'éviter les défaillances en cascade

### Vérification de la Conformité de la Distribution Topologique

`GET /api/topology/spread?namespace=xxx&domain=topology.kubernetes.io/zone` vérifie la conformité de la distribution topologique des Pods :

- 4 niveaux d'état de charge de travail : balanced / skewed / no-constraint / single-replica
- Analyse de la distribution et de l'écart par domaine topologique pour chaque charge de travail
- Détection des charges de travail multi-répliques sans contraintes topologiques
- Identification des nœuds sans étiquettes topologiques
- Suggestions pour les clusters à domaine unique
- Prend en charge le filtrage par namespace et par clé de domaine topologique

### Audit de la Rotation et du Cycle de Vie des Secrets

`GET /api/security/secrets/rotation?namespace=xxx` audite le cycle de vie de tous les Secrets :

- Suivi de l'âge : stale (>90j) / very stale (>180j)
- Détection des Secrets inutilisés (non référencés par aucun Pod)
- Détection de l'expiration des certificats TLS (analyse des certificats, détection des expirés et <30j)
- Suivi des Secrets de registre Docker, des tokens SA hérités
- Détection des noms sensibles (password/key/token/credential)
- Niveau de risque par Secret, score de rotation du cluster 0-100
- Prend en charge le filtrage par namespace

### Audit de l'Efficacité des Sondes de Santé

`GET /api/operations/probes?namespace=xxx` audite la configuration des sondes :

- 8 catégories de vérification : sondes manquantes, trop agressives, timeout trop court, seuils inadéquats, etc.
- Score de risque par charge de travail, score d'efficacité du cluster (0-100)
- Statistiques agrégées des principaux problèmes
- Recommandations exploitables

### Suivi de l'Obsolescence des Charges de Travail

`GET /api/product/staleness?namespace=xxx` suit l'obsolescence des déploiements :

- 5 niveaux de classification de l'obsolescence : fresh/recent/stale/very-stale/ancient
- Analyse des tags d'images : :latest, digest, sans tag
- Intervalles de distribution de l'âge, statistiques par namespace
- Score de fraîcheur du cluster (0-100)

### Analyse de la Survente et de la Pression des Ressources

`GET /api/scalability/overcommit?namespace=xxx` analyse la survente des ressources :

- Ratios de survente de request et limit CPU/mémoire par nœud
- Score de pression 0-100 et niveau de risque
- Détection des Pods sans limits/requests
- Score de sécurité du cluster 0-100
- Ventilation de la consommation de ressources par namespace

### Analyse de la Sécurité et de la Chaîne d'Approvisionnement des Images

`GET /api/security/images?namespace=xxx` analyse la sécurité de la chaîne d'approvisionnement de toutes les images de conteneurs :

- Détection du verrouillage par Digest (@sha256: référence immuable)
- Détection du tag :latest (mutable, non reproductible)
- Détection des images sans tag (:latest par défaut)
- Détection des tags de version anciens (v1, 1.0 — peuvent contenir des CVE connus)
- Analyse des registries d'images publics vs privés
- Niveau de risque par image, statistiques par registry
- Score de sécurité des images du cluster 0-100

### Planification de la Capacité

`GET /api/capacity/planning` planification de la capacité des nœuds :

- Requests CPU/mémoire vs allouable par nœud
- Capacité restante et recommandations d'expansion
- Détection de la fragmentation des ressources

### Prévision de la Capacité

`GET /api/capacity/forecast` prévision des tendances de capacité :

- Tendances de croissance des ressources basées sur les données historiques
- Temps d'épuisement estimé
- Recommandations d'expansion

### Analyse de l'Efficacité des Ressources

`GET /api/efficiency` efficacité de l'utilisation des ressources :

- Détection de la surallocation des ressources
- Identification du gaspillage de ressources
- Recommandations d'optimisation

### État des PDB

`GET /api/pdbs` état des Pod Disruption Budget :

- Vérification de la configuration des PDB
- Disruptions autorisées vs disponibles actuels
- Détection des PDB bloquants

### Compatibilité des Versions

`GET /api/compatibility` compatibilité des versions K8s :

- Vérification des API obsolètes
- Compatibilité des versions de ressources
- Évaluation de l'impact de la mise à niveau

### Expiration des Certificats

`GET /api/certificates/expiry` analyse de l'expiration des certificats TLS :

- Délai d'expiration des certificats du cluster
- Avertissement pour les certificats arrivant à expiration
- Recommandations de renouvellement

### Santé des Addons

`GET /api/addons/health` vérification de la santé des addons du cluster :

- État d'exécution des addons principaux
- Détection des addons anormaux
- Recommandations de correction

### Score de Santé du Cluster

`GET /api/operations/health-score` agrège tous les signaux de santé du cluster en un score global :

- 5 dimensions pondérées : Node(25%) + Pod(25%) + Workload(20%) + Events(15%) + API Server(15%)
- Score total 0-100, note alphabétique A-F
- État : healthy / warning / critical
- Score, poids et détails par dimension
- Résumé du cluster : nombre de nœuds/Pods/charges de travail
- Principaux problèmes triés par gravité

### Recommandations de Configuration Raisonnée des Ressources HPA/VPA

`GET /api/scalability/autoscale-recommendations?namespace=xxx` analyse la mise à l'échelle automatique et le right-sizing des ressources :

- Détection des charges de travail multi-répliques sans HPA
- Requests CPU excessifs (>1 core/conteneur)
- Requests mémoire excessifs (>2GB/conteneur)
- Analyse de l'efficacité des HPA (atteint la limite supérieure/inférieure/inactif)
- Valeurs de ressources actuelles vs recommandées par charge de travail
- Économies potentielles de cœurs CPU et de mémoire
- Score de mise à l'échelle automatique du cluster 0-100

### Surveillance de la Santé des Ingress et du Routage du Trafic

`GET /api/product/ingress-health?namespace=xxx` vérifie la santé du routage du trafic de tous les Ingress :

- Vérification de l'existence du Service backend et de la disponibilité des endpoints
- Détection de la configuration TLS
- Validation de l'IngressClass
- Détection des conflits host+path
- Détection de l'absence de règles de routage
- État par Ingress et score de santé du cluster 0-100

### Conditions des Nœuds et Pression des Ressources

`GET /api/operations/node-pressure` analyse les conditions et la pression des ressources de tous les nœuds :

- Détection de DiskPressure / MemoryPressure / PIDPressure / NetworkUnavailable
- Utilisation CPU/mémoire/Pods vs allouable
- Niveau de risque par nœud (critical/high/medium/low)
- Score de pression du cluster 0-100

### Liaison des PVC et Performance du Stockage

`GET /api/scalability/pvc-analysis?namespace=xxx` analyse la santé de la liaison du stockage :

- Détection de la cause racine des PVC bloqués (>5min pending)
- Mesure du temps de liaison et détection des liaisons lentes (>30s)
- Détection des PVC Lost
- Statistiques par StorageClass et analyse du provisioner
- Score de santé du stockage du cluster 0-100

### Gouvernance des Namespaces et Cycle de Vie

`GET /api/product/namespaces/lifecycle` audite la gouvernance des namespaces :

- Couverture des ResourceQuota / LimitRange / NetworkPolicy
- Détection des ServiceAccounts dédiés (moindres privilèges)
- Vérification des étiquettes requises (app, team, env, owner)
- Cycle de vie du namespace (active / stale / terminating)
- Score de gouvernance du cluster 0-100

### Analyse des Permissions Effectives RBAC et de l'Élévation de Privilèges

`GET /api/security/rbac-effective` analyse les permissions effectives RBAC de tous les sujets :

- Agrégation des ClusterRoleBindings + RoleBindings pour calculer les permissions réelles
- Détection de l'équivalence cluster-admin
- Détection des chemins d'élévation de privilèges (sujets pouvant modifier RBAC)
- Détection des permissions génériques (*)
- Analyse de l'accès en lecture des Secrets et de Pod exec
- Score de sécurité RBAC du cluster 0-100

### Suivi des OOM Kill de Conteneurs

`GET /api/operations/oom-tracker?namespace=xxx` suit les événements OOM des conteneurs :

- Détection des conteneurs OOMKilled et analyse de la cause racine
- Détection d'un nombre élevé de redémarrages (>=5)
- Détection des limites de mémoire manquantes ou trop basses
- Risque de pression des nœuds pour les limites très supérieures aux requests (10x+)
- Classement des principaux OOM et statistiques par namespace
- Score de risque OOM du cluster 0-100

### Prévision de l'Épuisement de la Capacité de Stockage

`GET /api/scalability/storage-forecast` prévoit la capacité de stockage :

- Utilisation par PV, taux de croissance, prédiction des jours avant épuisement
- Prise en charge de l'annotation Longhorn actual-size
- Estimation des jours avant remplissage du cluster
- Statistiques par StorageClass et analyse du provisioner
- Classement des namespaces à haut risque
- Score de santé du stockage 0-100

### Vérification de la Santé de la Résolution DNS

`GET /api/product/dns-health` analyse la santé de la résolution DNS :

- Vérification de la santé des Pods CoreDNS (exécution/prêt/redémarrages/version)
- Analyse de la configuration Corefile (forwarders, plugins)
- Couverture des endpoints des Headless Services et risque NXDOMAIN
- Détection du cache NodeLocal DNS
- Détection de la couverture dnsConfig ndots des Pods
- Découverte de services gérée par External-DNS
- Score de santé DNS du cluster 0-100
