# k8ops トラブルシューティングガイド

> このドキュメントは k8ops の一般的な問題の診断方法と解決策をまとめています。重大度別に分類し、迅速な切り分けを容易にしています。

---

## 目次

1. [インストールと起動の問題](#1-インストールと起動の問題)
2. [認証とログインの問題](#2-認証とログインの問題)
3. [AI 機能の問題](#3-ai-機能の問題)
4. [Pod とクラスターの問題](#4-pod-とクラスターの問題)
5. [ネットワークと Ingress の問題](#5-ネットワークと-ingress-の問題)
6. [データとストレージの問題](#6-データとストレージの問題)
7. [パフォーマンスの問題](#7-パフォーマンスの問題)
8. [監視とアラートの問題](#8-監視とアラートの問題)

---

## 1. インストールと起動の問題

### 1.1 Pod が Pending 状態のまま

**現象：** `kubectl get pods -n k8ops-system` が Pending を表示

**調査手順：**
```bash
# Pending の原因を確認
kubectl describe pod -n k8ops-system -l app.kubernetes.io/name=k8ops

# 一般的な原因：
# - PVC が未バインド（StorageClass を確認）
# - リソース不足（ノード容量を確認）
# - Node Selector が不一致
```

**解決策：**
- **PVC が未バインド：** クラスターにデフォルト StorageClass があるか確認
  ```bash
  kubectl get storageclass
  # デフォルト SC がない場合、マークする：
  kubectl patch storageclass local-path -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
  ```
- **リソース不足：** DaemonSet モードを使用（PVC 依存なし）
  ```bash
  kubectl apply -k config/daemonset
  ```

### 1.2 Pod CrashLoopBackOff

**現象：** Pod が繰り返し再起動

**調査手順：**
```bash
# コンテナログの確認
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --tail=50

# イベントの確認
kubectl get events -n k8ops-system --sort-by='.lastTimestamp' | tail -20
```

**一般的な原因と解決策：**

| 原因 | ログの特徴 | 解決策 |
|------|----------|----------|
| SQLite 権限の問題 | `unable to open database file` | `mkdir -p /data && chown 65532:65532 /data` |
| JWT Secret が未設定 | `JWT secret not configured` | `AUTH_JWT_SECRET` 環境変数を設定 |
| K8s API 接続失敗 | `failed to get Kubernetes config` | ServiceAccount と RBAC を確認 |
| ポート競合 | `bind: address already in use` | `--dashboard-address` を変更 |

### 1.3 イメージのプル失敗 (ImagePullBackOff)

**現象：** `Failed to pull image`

**解決策：**
```bash
# イメージがアクセス可能か確認
docker pull registry.iot2.win/k8ops:latest

# プライベートレジストリを使用する場合、imagePullSecrets を設定
kubectl create secret docker-registry regcred \
  --docker-server=registry.iot2.win \
  --docker-username=<user> \
  --docker-password=<pass> \
  -n k8ops-system

# または DaemonSet モード + hostPath を使用（外部イメージのプル不要）
```

---

## 2. 認証とログインの問題

### 2.1 ログインが 401 Unauthorized を返す

**調査：**
```bash
# auth 設定の確認
kubectl exec -n k8ops-system deploy/k8ops -- /manager --help | grep auth

# auth 関連ログの確認
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -i auth
```

**解決策：**
- `AUTH_JWT_SECRET` が設定され、一致していることを確認
- 管理者パスワードのリセット：
  ```bash
  kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops admin reset-password
  ```
- デフォルト認証情報：`admin` / `changeme`（初回ログイン後に変更してください）

### 2.2 OIDC ログイン失敗

**調査：**
- OIDC プロバイダー URL が到達可能か確認（Pod 内部から）
- リダイレクト URL が Ingress ドメインと一致するか確認
- コールバックエラーの確認：`kubectl logs ... | grep oidc`

---

## 3. AI 機能の問題

### 3.1 Chat の応答なしまたはタイムアウト

**現象：** メッセージ送信後に応答がない、またはタイムアウトが返る

**調査手順：**
```bash
# プロバイダー設定の確認
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops config show

# AI 関連ログの確認
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -E "provider|llm|agent"

# LLM 接続性のテスト
kubectl exec -n k8ops-system deploy/k8ops -- curl -s https://api.openai.com/v1/models -H "Authorization: Bearer $AIOPS_API_KEY"
```

**一般的な原因：**

| 原因 | ログの特徴 | 解決策 |
|------|----------|----------|
| API Key が無効 | `401 Unauthorized` | `AIOPS_API_KEY` 環境変数を更新 |
| ネットワーク接続不可 | `context deadline exceeded` | LLM API egress を設定 |
| モデルが存在しない | `model not found` | `--provider-model` を更新 |
| レート制限 | `429 Too Many Requests` | 待機またはプロバイダーを切り替え |
| サーキットブレーカーがオープン | `circuit breaker open` | 60s のクールダウンを待機 |

### 3.2 AI 診断がトリガーされない

**調査：**
```bash
# イベントコレクターの状態を確認
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -i "collector\|event"

# 無効化されていないか確認
kubectl get deploy k8ops -n k8ops-system -o jsonpath='{.spec.template.spec.containers[0].command}'
# --disable-event-collector が含まれていないはず
```

---

## 4. Pod とクラスターの問題

### 4.1 Dashboard が "kubernetes client not available" を表示

**現象：** API が 503 を返し、UI に接続エラーが表示

**原因：** Pod 内の K8s ServiceAccount 権限が不足、または config の読み込み失敗

**解決策：**
```bash
# RBAC を再適用
kubectl apply -k config/rbac

# ServiceAccount を検証
kubectl auth can-i list pods --as=system:serviceaccount:k8ops-system:k8ops -n k8ops-system
```

### 4.2 操作（Scale/Delete/Restart）が 403 Forbidden を返す

**原因：** ユーザーの RBAC ロール権限が不足

**解決策：**
```bash
# ユーザーロールの確認
kubectl get rolebindings -n k8ops-system | grep <username>

# admin ロールに昇格
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops admin set-role <username> admin
```

---

## 5. ネットワークと Ingress の問題

### 5.1 Dashboard にアクセスできない (502/503)

**調査：**
```bash
# Service に Endpoints があるか確認
kubectl get endpoints -n k8ops-system

# Ingress 設定の確認
kubectl get ingress -n k8ops-system
kubectl describe ingress -n k8ops-system

# Pod ポートに直接アクセス
kubectl port-forward -n k8ops-system deploy/k8ops 9090:9090
# その後 http://localhost:9090 にアクセス
```

### 5.2 Traefik タイムアウト (499/504)

**現象：** Registry プッシュや大きなファイルのアップロードがタイムアウト

**解決策（Traefik 固有）：**
```bash
# Traefik のタイムアウト制限を無効化
kubectl patch deployment -n kube-system traefik \
  --type='json' \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.readtimeout=0s"}]'

# または IngressRoute で timeout を設定
```

### 5.3 SSE (Server-Sent Events) が機能しない

**現象：** Chat インターフェースにリアルタイム応答がない

**調査：**
- リバースプロキシがロング接続をサポートしているか確認
- Nginx の設定に必要：`proxy_buffering off; proxy_cache off;`
- Traefik は追加設定不要

---

## 6. データとストレージの問題

### 6.1 SQLite データベースの破損

**現象：** `database disk image is malformed`

**解決策：**
```bash
# Pod に入る
kubectl exec -it -n k8ops-system deploy/k8ops -- sh

# データベースの修復（distroless で shell がない場合、CLI ツールを使用）
# 方法 1: バックアップして再構築
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops db backup /data/k8ops-backup.db
kubectl exec -n k8ops-system deploy/k8ops -- rm /data/k8ops.db
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops db restore /data/k8ops-backup.db

# 方法 2: PVC を削除して再構築（ユーザーデータが失われます）
kubectl delete pvc -n k8ops-system k8ops-data
kubectl delete pod -n k8ops-system -l app.kubernetes.io/name=k8ops
```

### 6.2 PVC のディスク容量不足

**調査：**
```bash
kubectl exec -n k8ops-system deploy/k8ops -- df -h /data
# または Dashboard → Capacity ページで確認
```

**解決策：**
- PVC の拡張：
  ```bash
  kubectl patch pvc -n k8ops-system k8ops-data -p '{"spec":{"resources":{"requests":{"storage":"5Gi"}}}}'
  ```
- 古い監査ログのクリーンアップ：
  ```bash
  kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops audit cleanup --max-age 30d
  ```

---

## 7. パフォーマンスの問題

### 7.1 API レスポンスが遅い

**調査：**
```bash
# レスポンスタイムの確認（X-Response-Time ヘッダー）
curl -sk -o /dev/null -w '%{http_code} %{time_total}s\n' \
  -D - https://k8ops.iot2.win/api/cluster/overview 2>&1 | grep -i "x-response-time"

# Prometheus メトリクスの確認
curl -sk https://k8ops.iot2.win/metrics | grep k8ops_http_request_duration
```

**最適化案：**
- API キャッシュは有効（overview: 30s, resources: 60s, CRDs: 10min）
- `k8ops_http_requests_in_flight` が高すぎないか確認
- 遅いリクエスト（>500ms）は自動的に Pod ログに記録されます

### 7.2 メモリ使用量が高い

**調査：**
```bash
kubectl top pods -n k8ops-system
```

**最適化：**
- 会話メモリの自動管理：20k token 閾値を超えると自動的に要約
- アイドル状態の会話は 30min 後にクリーンアップ
- 持続的に高メモリの場合、Pod の再起動を検討（DaemonSet モードは自動的に再起動します）

---

## 8. 監視とアラートの問題

### 8.1 Prometheus が Metrics をスクレイプできない

**調査：**
```bash
# metrics エンドポイントが正常か確認
kubectl exec -it <prometheus-pod> -n monitoring -- curl -s http://k8ops.k8ops-system.svc:8080/metrics | head -5

# ServiceMonitor の確認
kubectl get servicemonitor -n k8ops-system
```

**注意：** `/metrics` エンドポイントは localhost からのアクセスのみ許可されます。Prometheus はクラスター内（同じ Pod または Service）からスクレイプする必要があります。

### 8.2 アラートルールが反映されない

**調査：**
```bash
# PrometheusRule の確認
kubectl get prometheusrule -n k8ops-system

# アラートルールを適用
kubectl apply -f config/alerting-rules.yaml
```

---

## 付録：よく使う診断コマンド

```bash
# ワンクリックステータスチェック
kubectl get pods -n k8ops-system
kubectl get events -n k8ops-system --sort-by='.lastTimestamp' | tail -20
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --tail=50

# ヘルスチェック
curl -sk https://k8ops.iot2.win/api/health
curl -sk https://k8ops.iot2.win/api/version

# セキュリティスキャン
curl -sk https://k8ops.iot2.win/api/security/audit | jq .summary
curl -sk https://k8ops.iot2.win/api/security/compliance | jq .score

# キャパシティプランニング
curl -sk https://k8ops.iot2.win/api/capacity/planning | jq .summary
```

## 付録：ログレベル

k8ops は構造化 JSON ログ (slog) を使用し、以下のレベルをサポートします：

| レベル | 用途 | 例 |
|------|------|------|
| `INFO` | 正常な操作 | サーバー起動、認証成功 |
| `WARN` | 遅いリクエスト、設定の問題 | リクエスト >500ms、PVC がほぼ満杯 |
| `ERROR` | 操作の失敗 | K8s API エラー、LLM 呼び出し失敗 |

Request ID でログを関連付け：
```bash
# Request ID を取得（HTTP レスポンスヘッダーの X-Request-ID から）
# その後ログ内で検索
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep "a1b2c3d4"
```

---

*最終更新: 2026-07-03*
*メンテナー: k8ops Team*
