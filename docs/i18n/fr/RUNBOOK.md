# Manuel d'Exploitation de k8ops (Runbook)

> Ce document est destiné au personnel d'exploitation et couvre les opérations de maintenance quotidiennes, les procédures de traitement des incidents, les contacts d'urgence et les procédures opérationnelles standard.

---

## Table des matières

1. [Aperçu du service](#1-aperçu-du-service)
2. [Opérations Quotidiennes](#2-opérations-quotidiennes)
3. [Traitement des Incidents](#3-traitement-des-incidents)
4. [Opérations d'Urgence](#4-opérations-durgence)
5. [Sauvegarde et Restauration](#5-sauvegarde-et-restauration)
6. [Optimisation des Performances](#6-optimisation-des-performances)
7. [Contacts d'Urgence](#7-contacts-durgence)
8. [Définition SLO/SLA](#8-définition-slosla)

---

## 1. Aperçu du service

### Description de l'Architecture

```
┌─────────────────────────────────────────────────┐
│               Navigateur utilisateur               │
│              https://k8ops.iot2.win               │
└───────────────────┬─────────────────────────────┘
                    │ HTTPS (Traefik Ingress)
┌───────────────────▼─────────────────────────────┐
│              Traefik (kube-system)               │
│         websecure: 8443 → 8000                   │
└───────────────────┬─────────────────────────────┘
                    │
┌───────────────────▼─────────────────────────────┐
│            k8ops DaemonSet (k8ops-system)         │
│  ┌─────────────────────────────────────────────┐ │
│  │  Go Binary (frontend statique intégré)         │ │
│  │  :8080 Dashboard                             │ │
│  │  /metrics  Prometheus                        │ │
│  │  /api/chat  SSE → LLM Provider               │ │
│  │  nsenter → host kubectl                      │ │
│  └─────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────┘
```

### Composants clés

| Composant | Emplacement | Fonction |
|-----------|-------------|----------|
| k8ops DaemonSet | k8ops-system | Service principal, un Pod par nœud |
| Traefik | kube-system | Contrôleur Ingress, terminaison TLS |
| Registry | registry.iot2.win | Registre d'images privé |
| LLM Provider | API externe | Moteur Chat IA / diagnostic / optimisation |

### Endpoints de vérification de santé

| Endpoint | Réponse attendue | Notes |
|----------|-----------------|-------|
| `https://k8ops.iot2.win/` | 200/303 | Page frontale |
| `https://k8ops.iot2.win/readyz` | 200 | Sonde de préparation K8s |
| `https://k8ops.iot2.win/api/version` | 200 JSON | Informations de version |
| `https://k8ops.iot2.win/metrics` | 200 (local uniquement) | Métriques Prometheus |

---

## 2. Opérations quotidiennes

### 2.1 Vérifier l'état du service

```bash
# État des Pods
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops

# Journaux du service (100 dernières lignes)
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --tail=100

# Informations de version
curl -sk https://k8ops.iot2.win/api/version | jq .

# Vue d'ensemble du cluster
curl -sk https://k8ops.iot2.win/api/cluster/overview | jq .
```

### 2.2 Mettre à jour le déploiement

```bash
# Construire une nouvelle version
cd /Volumes/new/ggai/k8ops
VERSION=v14XX
docker buildx build --platform linux/amd64 \
  --build-arg VERSION=$VERSION \
  -t registry.iot2.win/k8ops:$VERSION \
  -t registry.iot2.win/k8ops:latest \
  --push .

# Mise à jour progressive
kubectl set image daemonset/k8ops \
  k8ops=registry.iot2.win/k8ops:$VERSION -n k8ops-system

# Vérification
sleep 15
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops
curl -sk -o /dev/null -w '%{http_code}' https://k8ops.iot2.win/
```

### 2.3 Gestion des journaux

k8ops utilise des journaux structurés `log/slog`, le niveau de journalisation est contrôlé par la variable d'environnement `LOG_LEVEL` :

| Niveau | Utilisation |
|--------|-------------|
| `DEBUG` | Débogage de développement, affiche tous les journaux |
| `INFO` (par défaut) | Exécution en production, enregistre les opérations clés |
| `WARN` | Uniquement les avertissements et erreurs |

```bash
# Changer le niveau de journalisation
kubectl set env daemonset/k8ops LOG_LEVEL=DEBUG -n k8ops-system
```

### 2.4 Configuration du fournisseur

Les fonctionnalités d'IA nécessitent la configuration d'un fournisseur LLM :

1. Visitez la page Paramètres → Fournisseur
2. Sélectionnez un fournisseur (OpenAI / Zhipu / DeepSeek, etc.)
3. Saisissez la clé API
4. Testez la connexion

Si non configuré, le tableau de bord affichera une bannière d'avertissement de fournisseur non configuré.

---

## 3. Traitement des incidents

### 3.1 Le pod ne démarre pas (CrashLoopBackOff)

**Symptôme** : Le Pod k8ops redémarre en boucle

**Étapes de diagnostic** :
```bash
# 1. Voir les événements du Pod
kubectl describe pod -n k8ops-system -l app.kubernetes.io/name=k8ops

# 2. Consulter les journaux du conteneur
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --previous

# 3. Vérifier les permissions RBAC
kubectl auth can-i --list --as=system:serviceaccount:k8ops-system:k8ops

# 4. Vérifier le montage ConfigMap/Secret
kubectl exec -n k8ops-system -it deploy/k8ops -- ls -la /etc/k8ops/
```

**Causes courantes** :
- Permissions RBAC insuffisantes → vérifiez `config/rbac/`
- kubeconfig invalide → vérifiez le kubeconfig monté
- Conflit de port → vérifiez si le port 8080 est occupé
- Mémoire insuffisante → vérifiez les ressources du nœud `kubectl describe nodes`

### 3.2 Le tableau de bord est inaccessible (502/503)

**Symptôme** : https://k8ops.iot2.win renvoie 502 ou 503

**Étapes de diagnostic** :
```bash
# 1. Vérifier l'Ingress
kubectl get ingress -A | grep k8ops

# 2. Vérifier Traefik
kubectl get pods -n kube-system -l app.kubernetes.io/name=traefik
kubectl logs -n kube-system -l app.kubernetes.io/name=traefik --tail=50

# 3. Vérifier le Service k8ops
kubectl get svc -n k8ops-system
kubectl get endpoints -n k8ops-system

# 4. Tester le Pod directement
kubectl exec -n k8ops-system -it deploy/k8ops -- curl -s localhost:8080/api/version
```

**Causes courantes** :
- Traefik ne route pas correctement → vérifiez les règles Ingress
- k8ops n'est pas prêt → vérifiez la sonde readyz
- Certificat TLS expiré → vérifiez cert-manager

### 3.3 Le chat IA ne répond pas

**Symptôme** : Après envoi d'un message dans Chat, pas de réponse ou timeout

**Étapes de diagnostic** :
```bash
# 1. Vérifier l'état du Provider
curl -sk https://k8ops.iot2.win/api/provider/status | jq .

# 2. Consulter les journaux du moteur
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -i 'llm\|provider\|chat'

# 3. Tester la connexion du Provider
curl -sk https://k8ops.iot2.win/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"message":"hello","conversationId":"test"}' --max-time 30
```

**Causes courantes** :
- Clé API non configurée ou expirée
- Limitation de débit (rate limit) de l'API du Provider (429)
- Réseau inaccessible (DNS/firewall)
- Tokens dépassés → l'Agent compresse automatiquement le contexte, mais peut échouer dans des cas extrêmes

### 3.4 Échec du push vers le registre (499)

**Symptôme** : `docker push` renvoie 499 Client Closed Request

**Solution** :
```bash
# Vérifier la configuration de timeout de Traefik
kubectl get deploy -n kube-system traefik -o jsonpath='{.spec.template.spec.containers[0].args}'

# S'il manque des paramètres de timeout, ajouter :
kubectl patch deploy -n kube-system traefik --type='json' -p='[
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.readtimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.writetimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.idletimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.keepalivemaxtime=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.keepalivemaxrequests=0"}
]'
```

### 3.5 Échec des opérations d'écriture (Scale/Delete/Restart)

**Symptôme** : Les opérations Scale/Delete/Restart échouent après clic sur les boutons

**Étapes de diagnostic** :
```bash
# Vérifier les permissions RBAC
kubectl auth can-i patch deployments --as=system:serviceaccount:k8ops-system:k8ops -n default
kubectl auth can-i delete pods --as=system:serviceaccount:k8ops-system:k8ops -n default

# Voir le journal d'audit
curl -sk https://k8ops.iot2.win/api/audit?severity=critical | jq .

# Vérifier les politiques de sécurité
kubectl get psp,podsecurity --all-namespaces 2>/dev/null
```

---

## 4. Opérations d'urgence

### 4.1 Retour arrière rapide

```bash
# Voir l'historique des versions
kubectl rollout history daemonset/k8ops -n k8ops-system

# Retour arrière à la version précédente
kubectl rollout undo daemonset/k8ops -n k8ops-system

# Retour arrière à une version spécifique
kubectl rollout undo daemonset/k8ops -n k8ops-system --to-revision=3
```

### 4.2 Réduction d'urgence (mise à l'échelle à zéro réplique)

```bash
# Note : DaemonSet ne supporte pas scale 0, suppression directe nécessaire
kubectl delete pod -n k8ops-system -l app.kubernetes.io/name=k8ops --grace-period=0 --force

# Pour arrêter complètement, modifier temporairement le nodeSelector
kubectl patch daemonset k8ops -n k8ops-system -p='{"spec":{"template":{"spec":{"nodeSelector":{"non-existent":"true"}}}}}'
```

### 4.3 Nettoyage des données

```bash
# Nettoyer les CRD d'historique de diagnostic
kubectl delete diagnostics --all --all-namespaces

# Nettoyer les journaux d'audit (conserver les 7 derniers jours)
kubectl get auditlogs -A -o json | jq '.items[] | select(.metadata.creationTimestamp < "'$(date -d '7 days ago' -Iseconds)'")' | kubectl delete -f -

# Nettoyer les rapports d'optimisation
kubectl delete optimizations --all --all-namespaces
```

---

## 5. Sauvegarde et restauration

### 5.1 Sauvegarde de la configuration

```bash
# Sauvegarder la configuration de k8ops
kubectl get cm,secret,daemonset -n k8ops-system -o yaml > k8ops-backup-$(date +%Y%m%d).yaml

# Sauvegarder les données CRD
kubectl get diagnostics,remediations,optimizations -A -o yaml > k8ops-crd-backup-$(date +%Y%m%d).yaml

# Sauvegarder RBAC
kubectl get clusterrole,clusterrolebinding -o yaml | grep -A5 k8ops > k8ops-rbac-backup-$(date +%Y%m%d).yaml
```

### 5.2 Processus de restauration

```bash
# Restaurer la configuration
kubectl apply -f k8ops-backup-YYYYMMDD.yaml

# Restaurer les données CRD
kubectl apply -f k8ops-crd-backup-YYYYMMDD.yaml

# Vérification
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops
curl -sk https://k8ops.iot2.win/api/version | jq .
```

### 5.3 Recommandation de sauvegarde régulière

Utilisez Velero ou un cron job pour une sauvegarde quotidienne :
```bash
# Sauvegarde Velero (recommandé)
velero backup create k8ops-daily-$(date +%Y%m%d) \
  --include-namespaces k8ops-system \
  --include-cluster-resources=true
```

---

## 6. Optimisation des performances

### 6.1 Métriques clés

| Métrique | Métrique Prometheus | Seuil d'alerte |
|---------|-------------------|----------------|
| Latence API | `k8ops_tool_call_duration_seconds` | P99 > 10s |
| Latence d'appel LLM | `k8ops_llm_call_duration_seconds` | P99 > 60s |
| Diagnostics actifs | `k8ops_active_diagnostics` | > 10 |
| Blocages de sécurité | `k8ops_safety_blocks_total` | rate > 10/min |
| Consommation de tokens | `k8ops_llm_tokens_total` | Croissance anormale quotidienne |
| Score de santé du cluster | `k8ops_cluster_health_score` | < 60 |

### 6.2 Recommandations de ressources

| Taille des nœuds | Requêtes de ressources k8ops | Limites de ressources |
|-------------------|-----------------------------|----------------------|
| <= 5 nœuds | 100m CPU / 128Mi | 500m CPU / 512Mi |
| 5-20 nœuds | 200m CPU / 256Mi | 1 CPU / 1Gi |
| 20-50 nœuds | 500m CPU / 512Mi | 2 CPU / 2Gi |

### 6.3 Optimisation du niveau de journalisation

Il est recommandé de maintenir le niveau `INFO` en production. Basculez temporairement vers `DEBUG` uniquement pour le diagnostic de problèmes :
```bash
# Activer temporairement DEBUG
kubectl set env daemonset/k8ops LOG_LEVEL=DEBUG -n k8ops-system
# Restaurer après diagnostic
kubectl set env daemonset/k8ops LOG_LEVEL=INFO -n k8ops-system
```

---

## 7. Contacts d'urgence

### 7.1 Processus d'escalade

```
Détection d'incident → Opérateur de garde (L1)
    ├── Non résolu en 5 minutes → Responsable des opérations (L2)
    │     ├── Non résolu en 15 minutes → Architecte (L3)
    │     │     ├── Impact en production → Notification au CTO
```

### 7.2 Tableau des contacts

> À compléter selon la situation réelle

| Rôle | Nom | Téléphone | Périmètre de Responsabilité |
|------|-----|-----------|------------------------------|
| L1 Opérateur de garde | ____ | ____ | Première réponse, traitement des incidents de base |
| L2 Responsable des opérations | ____ | ____ | Incidents complexes, impact sur plusieurs services |
| L3 Architecte | ____ | ____ | Problèmes d'architecture, récupération de données |
| Administrateur du cluster | ____ | ____ | Pannes du cluster K8s lui-même |
| Réseau/Sécurité | ____ | ____ | Politiques réseau, certificats, incidents de sécurité |

### 7.3 Contacts des fournisseurs

| Fournisseur | Utilisation | Contact |
|-------------|-------------|---------|
| LLM Provider | Chat IA/Diagnostic | ____ |
| Registry | Registre d'images | ____ |
| DNS/CDN | Résolution de noms | ____ |

---

## Annexe : Liste des métriques Prometheus

k8ops expose les métriques personnalisées suivantes (endpoint `/metrics`) :

| Métrique | Type | Étiquettes | Description |
|--------|------|------------|-------------|
| `k8ops_diagnostics_total` | Counter | phase, trigger | Nombre total de rapports de diagnostic |
| `k8ops_remediation_actions_total` | Counter | type, result, risk | Nombre total d'opérations de réparation |
| `k8ops_llm_call_duration_seconds` | Histogram | provider, model, status | Latence des appels LLM |
| `k8ops_llm_tokens_total` | Counter | provider, model, type | Consommation de tokens |
| `k8ops_agent_steps` | Histogram | - | Étapes d'exécution de l'Agent |
| `k8ops_tool_call_duration_seconds` | Histogram | tool, success | Latence des appels d'outils |
| `k8ops_safety_blocks_total` | Counter | reason | Nombre de blocages de sécurité |
| `k8ops_active_diagnostics` | Gauge | - | Nombre de diagnostics actuels |
| `k8ops_active_remediations` | Gauge | - | Réparations en cours d'exécution |
| `k8ops_audit_events_total` | Counter | type, severity | Nombre total d'événements d'audit |
| `k8ops_cluster_health_score` | Gauge | - | Score de santé du cluster (0-100) |
| `k8ops_conversation_count` | Gauge | - | Nombre de conversations actives |
| `k8ops_tool_executions_total` | Counter | tool, success | Nombre total d'exécutions d'outils |
| `k8ops_http_requests_total` | Counter | method, path, status | Nombre total de requêtes HTTP |
| `k8ops_http_request_duration_seconds` | Histogram | method, path | Latence des requêtes HTTP |
| `k8ops_http_requests_in_flight` | Gauge | - | Nombre de requêtes en cours de traitement |
| `k8ops_api_errors_total` | Counter | method, path, status | Nombre d'erreurs API (4xx+5xx) |

---

## 8. Définition SLO/SLA

### 8.1 Objectifs de niveau de service (SLO)

| Métrique | Objectif | Fenêtre de Mesure | Budget d'Erreurs |
|---------|----------|-------------------|------------------|
| Disponibilité du tableau de bord | 99,9 % | 30 jours glissants | 43,2 minutes/mois |
| Taux de réussite API (non 429) | 99.5% | 30 jours glissants | 3.6 heures/mois |
| Latence P99 de l'API | < 2s | Temps réel | - |
| Temps de réponse du chat IA | < 30s (premier jeton) | Temps réel | - |
| Fin de l'analyse d'audit de sécurité | < 60s | Temps réel | - |

### 8.2 Gestion du budget d'erreurs

Objectif de disponibilité mensuelle de 99.9% = **43.2 minutes de budget d'erreurs** :

- **Dans le budget (<30min)** : Rythme de publication normal, aucune approbation supplémentaire nécessaire
- **Avertissement de budget (30-43min)** : Gel des changements non urgents, prioriser la correction des problèmes de fiabilité
- **Budget épuisé (>43min)** : Gel total des publications, réaliser une analyse post-mortem

### 8.3 Requêtes de surveillance SLO (Prometheus PromQL)

**Taux d'erreurs API (5 minutes) :**
```promql
sum(rate(k8ops_api_errors_total{status=~"5.."}[5m])) by (path)
/ sum(rate(k8ops_http_requests_total[5m])) by (path)
```

**Latence P99 de l'API :**
```promql
histogram_quantile(0.99,
  sum(rate(k8ops_http_request_duration_seconds_bucket[5m])) by (le, path)
)
```

**Taux de consommation du budget d'erreurs :**
```promql
1 - (
  sum(rate(k8ops_http_requests_total{status!~"5.."}[30d]))
  / sum(rate(k8ops_http_requests_total[30d]))
)
```

### 8.4 Stratégie de dégradation

Lorsque le SLO est sur le point d'être violé, dégrader par ordre de priorité :

1. **Désactiver le chat IA** — la fonctionnalité la plus gourmande en ressources, sa dégradation n'affecte pas la gestion centrale de K8s
2. **Augmenter le TTL du cache** — augmenter le cache overview/nodes/pods de 30s à 120s
3. **Limiter les diagnostics concurrents** — réduire la limite maximale de `k8ops_active_diagnostics`
4. **Désactiver le collecteur d'événements** — flag `--disable-event-collector`

### 8.5 Traçabilité des requêtes

Toutes les réponses HTTP incluent l'en-tête `X-Request-ID`, utilisé pour :
- Corrélation des journaux — toutes les lignes de journal d'une même requête partagent le request_id
- Traçabilité d'audit — le request_id dans le journal d'audit peut être relié à la requête HTTP spécifique
- Diagnostic d'incidents — lorsqu'un utilisateur signale un problème, fournir le request_id permet de localiser rapidement les journaux

Exemple de consultation des journaux :
```bash
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep "a1b2c3d4e5f6"
```

### 8.6 Configuration du niveau de journalisation

k8ops utilise des journaux structurés JSON (slog), avec prise en charge de la configuration du niveau via la variable d'environnement `LOG_LEVEL` ou l'argument de ligne de commande `--log-level` :

| Niveau | Utilisation | Description |
|--------|-------------|-------------|
| `debug` | Diagnostic de problèmes | Inclut source file:line, journaux très détaillés (non recommandé en production) |
| `info` | Par défaut | Logs d'opérations normales (recommandé pour la production) |
| `warn` | Uniquement les avertissements | Requêtes lentes, problèmes de configuration, près du seuil |
| `error` | Uniquement les erreurs | Enregistrer uniquement les échecs d'opération |

Méthodes de configuration :
```bash
# Via variable d'environnement (recommandé)
kubectl set env daemonset/k8ops -n k8ops-system LOG_LEVEL=debug

# Via ConfigMap
kubectl patch configmap k8ops-config -n k8ops-system \
  --type='json' -p='[{"op":"add","path":"/data/log-level","value":"debug"}]'

# Via argument de ligne de commande (uniquement applicable en mode Deployment)
# args:
# - --log-level=debug
```

Redémarrer le Pod après changement de niveau :
```bash
kubectl rollout restart daemonset/k8ops -n k8ops-system
```

### 8.7 Rotation des journaux

Le fichier journal d'audit (`/data/k8ops-audit.jsonl`) fait l'objet d'une rotation automatique :
- **Rotation automatique** : le fichier est divisé automatiquement lorsqu'il dépasse 100MB
- **Rotation manuelle** : `POST /api/system/log/rotate` (permissions admin)
- **Nettoyage des anciens fichiers** : `POST /api/system/log/cleanup` (supprime les fichiers de rotation de plus de 30 jours)

Les journaux stdout des conteneurs sont gérés par Kubelet, avec une limite par défaut de 10 Mo x 3 fichiers = 30 Mo par conteneur.
Dans k3s, ils peuvent être ajustés via `--container-log-max-size` et `--container-log-max-files`.

---

*Dernière mise à jour : 2026-07-02*
*Mainteneurs : Équipe k8ops*
