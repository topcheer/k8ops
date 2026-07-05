# Guide de dépannage k8ops

> Ce document résume les méthodes de diagnostic et les solutions pour les problèmes courants de k8ops, classés par niveau de gravité pour un repérage rapide.

---

## Table des matières

1. [Problèmes d'installation et de démarrage](#1-problèmes-dinstallation-et-de-démarrage)
2. [Problèmes d'authentification et de connexion](#2-problèmes-dauthentification-et-de-connexion)
3. [Problèmes de fonctionnalités IA](#3-problèmes-de-fonctionnalités-ia)
4. [Problèmes de Pod et de cluster](#4-problèmes-de-pod-et-de-cluster)
5. [Problèmes réseau et Ingress](#5-problèmes-réseau-et-ingress)
6. [Problèmes de données et de stockage](#6-problèmes-de-données-et-de-stockage)
7. [Problèmes de performance](#7-problèmes-de-performance)
8. [Problèmes de surveillance et d'alerte](#8-problèmes-de-surveillance-et-dalerte)

---

## 1. Problèmes d'installation et de démarrage

### 1.1 Le Pod reste à l'état Pending

**Symptôme :** `kubectl get pods -n k8ops-system` affiche Pending

**Étapes de diagnostic :**
```bash
# Voir la raison du Pending
kubectl describe pod -n k8ops-system -l app.kubernetes.io/name=k8ops

# Causes courantes :
# - PVC non lié (vérifier le StorageClass)
# - Ressources insuffisantes (vérifier la capacité des nœuds)
# - Node Selector ne correspond pas
```

**Solutions :**
- **PVC non lié :** Vérifier que le cluster possède un StorageClass par défaut
  ```bash
  kubectl get storageclass
  # S'il n'y a pas de SC par défaut, en marquer un :
  kubectl patch storageclass local-path -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
  ```
- **Ressources insuffisantes :** Utiliser le mode DaemonSet (sans dépendance PVC)
  ```bash
  kubectl apply -k config/daemonset
  ```

### 1.2 Pod en CrashLoopBackOff

**Symptôme :** Le Pod redémarre en boucle

**Étapes de diagnostic :**
```bash
# Voir les journaux du conteneur
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --tail=50

# Voir les événements
kubectl get events -n k8ops-system --sort-by='.lastTimestamp' | tail -20
```

**Causes courantes et solutions :**

| Cause | Signature dans les journaux | Solution |
|-------|----------------------------|----------|
| Problème de permissions SQLite | `unable to open database file` | `mkdir -p /data && chown 65532:65532 /data` |
| JWT Secret manquant | `JWT secret not configured` | Définir la variable d'environnement `AUTH_JWT_SECRET` |
| Échec de connexion à l'API K8s | `failed to get Kubernetes config` | Vérifier le ServiceAccount et le RBAC |
| Conflit de port | `bind: address already in use` | Modifier `--dashboard-address` |

### 1.3 Échec de pull d'image (ImagePullBackOff)

**Symptôme :** `Failed to pull image`

**Solution :**
```bash
# Vérifier si l'image est accessible
docker pull registry.iot2.win/k8ops:latest

# Si vous utilisez un registre privé, configurez les imagePullSecrets
kubectl create secret docker-registry regcred \
  --docker-server=registry.iot2.win \
  --docker-username=<user> \
  --docker-password=<pass> \
  -n k8ops-system

# Ou utilisez le mode DaemonSet + hostPath (pas besoin de pull d'image externe)
```

---

## 2. Problèmes d'authentification et de connexion

### 2.1 La connexion renvoie 401 Unauthorized

**Diagnostic :**
```bash
# Vérifier la configuration d'authentification
kubectl exec -n k8ops-system deploy/k8ops -- /manager --help | grep auth

# Voir les journaux liés à l'authentification
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -i auth
```

**Solution :**
- Confirmer que `AUTH_JWT_SECRET` est défini et cohérent
- Réinitialiser le mot de passe administrateur :
  ```bash
  kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops admin reset-password
  ```
- Identifiants par défaut : `admin` / `changeme` (à modifier après la première connexion)

### 2.2 Échec de connexion OIDC

**Diagnostic :**
- Confirmer que l'URL du provider OIDC est accessible (depuis le Pod)
- Vérifier que l'URL de redirection correspond au domaine Ingress
- Consulter les erreurs de callback : `kubectl logs ... | grep oidc`

---

## 3. Problèmes de fonctionnalités IA

### 3.1 Pas de réponse du chat ou timeout

**Symptôme :** Aucune réponse après l'envoi d'un message, ou timeout

**Étapes de diagnostic :**
```bash
# Vérifier la configuration du provider
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops config show

# Voir les journaux liés à l'IA
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -E "provider|llm|agent"

# Tester la connectivité LLM
kubectl exec -n k8ops-system deploy/k8ops -- curl -s https://api.openai.com/v1/models -H "Authorization: Bearer $AIOPS_API_KEY"
```

**Causes courantes :**

| Cause | Signature dans les journaux | Solution |
|-------|----------------------------|----------|
| Clé API invalide | `401 Unauthorized` | Mettre à jour la variable d'environnement `AIOPS_API_KEY` |
| Réseau inaccessible | `context deadline exceeded` | Configurer l'egress vers l'API LLM |
| Modèle inexistant | `model not found` | Mettre à jour `--provider-model` |
| Limite de débit | `429 Too Many Requests` | Attendre ou changer de provider |
| Circuit Breaker ouvert | `circuit breaker open` | Attendre 60s de récupération |

### 3.2 Les diagnostics IA ne se déclenchent pas

**Diagnostic :**
```bash
# Vérifier le statut du collecteur d'événements
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -i "collector\|event"

# Confirmer qu'il n'est pas désactivé
kubectl get deploy k8ops -n k8ops-system -o jsonpath='{.spec.template.spec.containers[0].command}'
# Ne doit pas contenir --disable-event-collector
```

---

## 4. Problèmes de Pod et de cluster

### 4.1 Le tableau de bord affiche « kubernetes client not available »

**Symptôme :** L'API renvoie 503, l'interface affiche une erreur de connexion

**Cause :** Permissions insuffisantes du ServiceAccount K8s dans le Pod ou échec de chargement de la configuration

**Solution :**
```bash
# Réappliquer le RBAC
kubectl apply -k config/rbac

# Vérifier le ServiceAccount
kubectl auth can-i list pods --as=system:serviceaccount:k8ops-system:k8ops -n k8ops-system
```

### 4.2 Les opérations (Scale/Delete/Restart) renvoient 403 Forbidden

**Cause :** Permissions RBAC utilisateur insuffisantes

**Solution :**
```bash
# Vérifier le rôle de l'utilisateur
kubectl get rolebindings -n k8ops-system | grep <username>

# Promouvoir au rôle admin
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops admin set-role <username> admin
```

---

## 5. Problèmes réseau et Ingress

### 5.1 Tableau de bord inaccessible (502/503)

**Diagnostic :**
```bash
# Vérifier si le Service a des Endpoints
kubectl get endpoints -n k8ops-system

# Vérifier la configuration Ingress
kubectl get ingress -n k8ops-system
kubectl describe ingress -n k8ops-system

# Accéder directement au port du Pod
kubectl port-forward -n k8ops-system deploy/k8ops 9090:9090
# Puis accéder à http://localhost:9090
```

### 5.2 Délai d'expiration Traefik (499/504)

**Symptôme :** Push vers le registre ou timeout lors du transfert de gros fichiers

**Solution (spécifique à Traefik) :**
```bash
# Désactiver la limite de timeout Traefik
kubectl patch deployment -n kube-system traefik \
  --type='json' \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.readtimeout=0s"}]'

# Ou définir le timeout dans l'IngressRoute
```

### 5.3 SSE (Server-Sent Events) ne fonctionne pas

**Symptôme :** Pas de réponse en temps réel dans l'interface de chat

**Diagnostic :**
- Vérifier que le proxy inverse prend en charge les connexions persistantes
- La configuration Nginx nécessite : `proxy_buffering off; proxy_cache off;`
- Traefik ne nécessite pas de configuration supplémentaire

---

## 6. Problèmes de données et de stockage

### 6.1 Base de données SQLite corrompue

**Symptôme :** `database disk image is malformed`

**Solution :**
```bash
# Entrer dans le Pod
kubectl exec -it -n k8ops-system deploy/k8ops -- sh

# Réparer la base de données (si distroless sans shell, utiliser l'outil CLI)
# Solution 1 : sauvegarde et restauration
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops db backup /data/k8ops-backup.db
kubectl exec -n k8ops-system deploy/k8ops -- rm /data/k8ops.db
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops db restore /data/k8ops-backup.db

# Solution 2 : supprimer et recréer le PVC (entraîne la perte des données utilisateur)
kubectl delete pvc -n k8ops-system k8ops-data
kubectl delete pod -n k8ops-system -l app.kubernetes.io/name=k8ops
```

### 6.2 Espace disque PVC insuffisant

**Diagnostic :**
```bash
kubectl exec -n k8ops-system deploy/k8ops -- df -h /data
# Ou consulter via Dashboard → page Capacity
```

**Solution :**
- Étendre le PVC :
  ```bash
  kubectl patch pvc -n k8ops-system k8ops-data -p '{"spec":{"resources":{"requests":{"storage":"5Gi"}}}}'
  ```
- Nettoyer les anciens journaux d'audit :
  ```bash
  kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops audit cleanup --max-age 30d
  ```

---

## 7. Problèmes de performance

### 7.1 Réponses API lentes

**Diagnostic :**
```bash
# Vérifier le temps de réponse (en-tête X-Response-Time)
curl -sk -o /dev/null -w '%{http_code} %{time_total}s\n' \
  -D - https://k8ops.iot2.win/api/cluster/overview 2>&1 | grep -i "x-response-time"

# Voir les métriques Prometheus
curl -sk https://k8ops.iot2.win/metrics | grep k8ops_http_request_duration
```

**Optimisations :**
- Le cache API est activé (overview : 30s, resources : 60s, CRDs : 10min)
- Vérifier si `k8ops_http_requests_in_flight` est trop élevé
- Les requêtes lentes (>500ms) sont automatiquement journalisées dans les logs du Pod

### 7.2 Utilisation mémoire élevée

**Diagnostic :**
```bash
kubectl top pods -n k8ops-system
```

**Optimisation :**
- Gestion automatique de la mémoire des conversations : résumé automatique après un seuil de 20k tokens
- Nettoyage des conversations inactives après 30min
- En cas de mémoire élevée persistante, envisager de redémarrer le Pod (le mode DaemonSet redémarre automatiquement)

---

## 8. Problèmes de surveillance et d'alerte

### 8.1 Prometheus ne peut pas récupérer les métriques

**Diagnostic :**
```bash
# Confirmer que l'endpoint metrics fonctionne
kubectl exec -it <prometheus-pod> -n monitoring -- curl -s http://k8ops.k8ops-system.svc:8080/metrics | head -5

# Vérifier le ServiceMonitor
kubectl get servicemonitor -n k8ops-system
```

**Remarque :** L'endpoint `/metrics` n'autorise que l'accès depuis localhost. Prometheus doit récupérer depuis le cluster (même Pod ou Service).

### 8.2 Les règles d'alerte ne fonctionnent pas

**Diagnostic :**
```bash
# Vérifier le PrometheusRule
kubectl get prometheusrule -n k8ops-system

# Appliquer les règles d'alerte
kubectl apply -f config/alerting-rules.yaml
```

---

## Annexe : Commandes de diagnostic courantes

```bash
# Vérification du statut en une commande
kubectl get pods -n k8ops-system
kubectl get events -n k8ops-system --sort-by='.lastTimestamp' | tail -20
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --tail=50

# Vérification de santé
curl -sk https://k8ops.iot2.win/api/health
curl -sk https://k8ops.iot2.win/api/version

# Analyse de sécurité
curl -sk https://k8ops.iot2.win/api/security/audit | jq .summary
curl -sk https://k8ops.iot2.win/api/security/compliance | jq .score

# Planification de capacité
curl -sk https://k8ops.iot2.win/api/capacity/planning | jq .summary
```

## Annexe : Niveaux de journalisation

k8ops utilise une journalisation structurée JSON (slog) avec les niveaux suivants :

| Niveau | Rôle | Exemple |
|--------|------|---------|
| `INFO` | Opérations normales | Démarrage du serveur, authentification réussie |
| `WARN` | Requêtes lentes, problèmes de configuration | Requête >500ms, PVC presque plein |
| `ERROR` | Échecs d'opérations | Erreur API K8s, échec d'appel LLM |

Corrélation des journaux via le Request ID :
```bash
# Obtenir le Request ID (de l'en-tête de réponse HTTP X-Request-ID)
# Puis rechercher dans les journaux
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep "a1b2c3d4"
```

---

*Dernière mise à jour : 2026-07-03*
*Mainteneur : k8ops Team*
