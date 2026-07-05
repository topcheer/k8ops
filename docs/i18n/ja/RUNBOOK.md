# k8ops 運用マニュアル (Runbook)

> 本ドキュメントは運用担当者向けであり、日常の運用操作、トラブルシューティングフロー、緊急連絡先、および標準操作手順を網羅しています。

---

## 目次

1. [サービス概要](#1-サービス概要)
2. [日常運用](#2-日常運用)
3. [トラブルシューティング](#3-トラブルシューティング)
4. [緊急操作](#4-緊急操作)
5. [バックアップと復元](#5-バックアップと復元)
6. [パフォーマンスチューニング](#6-パフォーマンスチューニング)
7. [緊急連絡先](#7-緊急連絡先)
8. [SLO/SLA 定義](#8-slosla-定義)

---

## 1. サービス概要

### アーキテクチャ概要

```
┌─────────────────────────────────────────────────┐
│                   ユーザーブラウザー                │
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
│  │  Go Binary (フロントエンド静的リソースを埋め込み)  │ │
│  │  :8080 Dashboard                             │ │
│  │  /metrics  Prometheus                        │ │
│  │  /api/chat  SSE → LLM Provider               │ │
│  │  nsenter → host kubectl                      │ │
│  └─────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────┘
```

### 主要コンポーネント

| コンポーネント | 場所 | 役割 |
|------|------|------|
| k8ops DaemonSet | k8ops-system | メインサービス、ノードごとに 1 Pod |
| Traefik | kube-system | Ingress コントローラー、TLS 終端 |
| Registry | registry.iot2.win | プライベートイメージレジストリ |
| LLM Provider | 外部 API | AI Chat / 診断 / 最適化エンジン |

### ヘルスチェックエンドポイント

| エンドポイント | 期待されるレスポンス | 説明 |
|------|---------|------|
| `https://k8ops.iot2.win/` | 200/303 | フロントエンドページ |
| `https://k8ops.iot2.win/readyz` | 200 | K8s Readiness Probe |
| `https://k8ops.iot2.win/api/version` | 200 JSON | バージョン情報 |
| `https://k8ops.iot2.win/metrics` | 200 (ローカルのみ) | Prometheus メトリクス |

---

## 2. 日常運用

### 2.1 サービスステータスの確認

```bash
# Pod ステータス
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops

# サービスログ（直近 100 行）
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --tail=100

# バージョン情報
curl -sk https://k8ops.iot2.win/api/version | jq .

# クラスター概要
curl -sk https://k8ops.iot2.win/api/cluster/overview | jq .
```

### 2.2 デプロイの更新

```bash
# 新バージョンのビルド
cd /Volumes/new/ggai/k8ops
VERSION=v14XX
docker buildx build --platform linux/amd64 \
  --build-arg VERSION=$VERSION \
  -t registry.iot2.win/k8ops:$VERSION \
  -t registry.iot2.win/k8ops:latest \
  --push .

# ローリングアップデート
kubectl set image daemonset/k8ops \
  k8ops=registry.iot2.win/k8ops:$VERSION -n k8ops-system

# 検証
sleep 15
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops
curl -sk -o /dev/null -w '%{http_code}' https://k8ops.iot2.win/
```

### 2.3 ログ管理

k8ops は `log/slog` 構造化ログを使用し、ログレベルは環境変数 `LOG_LEVEL` で制御します：

| レベル | 用途 |
|------|------|
| `DEBUG` | 開発デバッグ、全ログを出力 |
| `INFO` (デフォルト) | 本番稼働、主要な操作を記録 |
| `WARN` | 警告とエラーのみ |

```bash
# ログレベルの変更
kubectl set env daemonset/k8ops LOG_LEVEL=DEBUG -n k8ops-system
```

### 2.4 Provider 構成

AI 機能には LLM Provider の構成が必要です：

1. Settings → Provider 構成ページにアクセス
2. Provider を選択（OpenAI / Zhipu / DeepSeek など）
3. API Key を入力
4. 接続をテスト

未構成の場合、Dashboard に Provider 未構成の警告バナーが表示されます。

---

## 3. トラブルシューティング

### 3.1 Pod が起動しない (CrashLoopBackOff)

**症状**: k8ops Pod が繰り返し再起動する

**調査手順**:
```bash
# 1. Pod イベントの確認
kubectl describe pod -n k8ops-system -l app.kubernetes.io/name=k8ops

# 2. コンテナログの確認
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --previous

# 3. RBAC 権限の確認
kubectl auth can-i --list --as=system:serviceaccount:k8ops-system:k8ops

# 4. ConfigMap/Secret のマウント確認
kubectl exec -n k8ops-system -it deploy/k8ops -- ls -la /etc/k8ops/
```

**一般的な原因**:
- RBAC 権限不足 → `config/rbac/` を確認
- kubeconfig が無効 → マウントされた kubeconfig を確認
- ポートの競合 → 8080 ポートの占有状況を確認
- メモリ不足 → ノードリソースを確認 `kubectl describe nodes`

### 3.2 Dashboard にアクセスできない (502/503)

**症状**: https://k8ops.iot2.win が 502 または 503 を返す

**調査手順**:
```bash
# 1. Ingress の確認
kubectl get ingress -A | grep k8ops

# 2. Traefik の確認
kubectl get pods -n kube-system -l app.kubernetes.io/name=traefik
kubectl logs -n kube-system -l app.kubernetes.io/name=traefik --tail=50

# 3. k8ops Service の確認
kubectl get svc -n k8ops-system
kubectl get endpoints -n k8ops-system

# 4. Pod への直接テスト
kubectl exec -n k8ops-system -it deploy/k8ops -- curl -s localhost:8080/api/version
```

**一般的な原因**:
- Traefik のルーティング不備 → Ingress ルールを確認
- k8ops が未準備 → readyz Probe を確認
- TLS 証明書の期限切れ → cert-manager を確認

### 3.3 AI Chat が応答しない

**症状**: Chat でメッセージ送信後に応答がない、またはタイムアウトする

**調査手順**:
```bash
# 1. Provider ステータスの確認
curl -sk https://k8ops.iot2.win/api/provider/status | jq .

# 2. エンジンログの確認
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -i 'llm\|provider\|chat'

# 3. Provider 接続のテスト
curl -sk https://k8ops.iot2.win/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"message":"hello","conversationId":"test"}' --max-time 30
```

**一般的な原因**:
- API Key が未構成または期限切れ
- Provider API のレート制限 (429)
- ネットワーク到達不可 (DNS/ファイアウォール)
- トークン超過 → Agent が自動的にコンテキストを圧縮しますが、極端な場合は失敗する可能性があります

### 3.4 Registry へのプッシュ失敗 (499)

**症状**: `docker push` が 499 Client Closed Request を返す

**解決策**:
```bash
# Traefik のタイムアウト設定の確認
kubectl get deploy -n kube-system traefik -o jsonpath='{.spec.template.spec.containers[0].args}'

# タイムアウトパラメーターが不足している場合は追加：
kubectl patch deploy -n kube-system traefik --type='json' -p='[
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.readtimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.writetimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.idletimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.keepalivemaxtime=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.keepalivemaxrequests=0"}
]'
```

### 3.5 書き込み操作の失敗 (Scale/Delete/Restart)

**症状**: Scale/Delete/Restart ボタンをクリックした後に操作が失敗する

**調査手順**:
```bash
# RBAC 権限の確認
kubectl auth can-i patch deployments --as=system:serviceaccount:k8ops-system:k8ops -n default
kubectl auth can-i delete pods --as=system:serviceaccount:k8ops-system:k8ops -n default

# 監査ログの確認
curl -sk https://k8ops.iot2.win/api/audit?severity=critical | jq .

# セキュリティポリシーの確認
kubectl get psp,podsecurity --all-namespaces 2>/dev/null
```

---

## 4. 緊急操作

### 4.1 クイックロールバック

```bash
# 履歴バージョンの確認
kubectl rollout history daemonset/k8ops -n k8ops-system

# 1 つ前のバージョンにロールバック
kubectl rollout undo daemonset/k8ops -n k8ops-system

# 指定バージョンにロールバック
kubectl rollout undo daemonset/k8ops -n k8ops-system --to-revision=3
```

### 4.2 緊急スケールダウン（0 レプリカを保持）

```bash
# 注意: DaemonSet は scale 0 をサポートしないため、直接削除が必要
kubectl delete pod -n k8ops-system -l app.kubernetes.io/name=k8ops --grace-period=0 --force

# 完全に停止する場合は、一時的に nodeSelector を変更
kubectl patch daemonset k8ops -n k8ops-system -p='{"spec":{"template":{"spec":{"nodeSelector":{"non-existent":"true"}}}}}'
```

### 4.3 データクリーンアップ

```bash
# 診断履歴 CRD のクリーンアップ
kubectl delete diagnostics --all --all-namespaces

# 監査ログのクリーンアップ（直近 7 日間を保持）
kubectl get auditlogs -A -o json | jq '.items[] | select(.metadata.creationTimestamp < "'$(date -d '7 days ago' -Iseconds)'")' | kubectl delete -f -

# 最適化レポートのクリーンアップ
kubectl delete optimizations --all --all-namespaces
```

---

## 5. バックアップと復元

### 5.1 構成のバックアップ

```bash
# k8ops 構成のバックアップ
kubectl get cm,secret,daemonset -n k8ops-system -o yaml > k8ops-backup-$(date +%Y%m%d).yaml

# CRD データのバックアップ
kubectl get diagnostics,remediations,optimizations -A -o yaml > k8ops-crd-backup-$(date +%Y%m%d).yaml

# RBAC のバックアップ
kubectl get clusterrole,clusterrolebinding -o yaml | grep -A5 k8ops > k8ops-rbac-backup-$(date +%Y%m%d).yaml
```

### 5.2 復元手順

```bash
# 構成の復元
kubectl apply -f k8ops-backup-YYYYMMDD.yaml

# CRD データの復元
kubectl apply -f k8ops-crd-backup-YYYYMMDD.yaml

# 検証
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops
curl -sk https://k8ops.iot2.win/api/version | jq .
```

### 5.3 定期バックアップの推奨

Velero または cron job で毎日バックアップ：
```bash
# Velero バックアップ（推奨）
velero backup create k8ops-daily-$(date +%Y%m%d) \
  --include-namespaces k8ops-system \
  --include-cluster-resources=true
```

---

## 6. パフォーマンスチューニング

### 6.1 主要指標

| 指標 | Prometheus Metric | アラート閾値 |
|------|-------------------|---------|
| API レイテンシー | `k8ops_tool_call_duration_seconds` | P99 > 10s |
| LLM 呼び出しレイテンシー | `k8ops_llm_call_duration_seconds` | P99 > 60s |
| アクティブ診断数 | `k8ops_active_diagnostics` | > 10 |
| セキュリティブロック | `k8ops_safety_blocks_total` | rate > 10/min |
| Token 消費 | `k8ops_llm_tokens_total` | 日次消費の異常増加 |
| クラスター健康スコア | `k8ops_cluster_health_score` | < 60 |

### 6.2 リソース推奨値

| ノード規模 | k8ops リソース Request | リソース Limit |
|---------|-------------------|-----------|
| 5 ノード以下 | 100m CPU / 128Mi | 500m CPU / 512Mi |
| 5-20 ノード | 200m CPU / 256Mi | 1 CPU / 1Gi |
| 20-50 ノード | 500m CPU / 512Mi | 2 CPU / 2Gi |

### 6.3 ログレベルの最適化

本番環境では `INFO` レベルを維持することを推奨します。問題の調査時のみ一時的に `DEBUG` に切り替えてください：
```bash
# 一時的な DEBUG の有効化
kubectl set env daemonset/k8ops LOG_LEVEL=DEBUG -n k8ops-system
# 調査後に復元
kubectl set env daemonset/k8ops LOG_LEVEL=INFO -n k8ops-system
```

---

## 7. 緊急連絡先

### 7.1 エスカレーションフロー

```
障害発見 → 当番運用担当 (L1)
    ├── 5分以内に未解決 → 運用責任者 (L2)
    │     ├── 15分以内に未解決 → アーキテクト (L3)
    │     │     ├── 本番影響あり → CTO へ報告
```

### 7.2 連絡先一覧

> 実際の状況に応じて記入してください

| 役割 | 氏名 | 電話 | 担当範囲 |
|------|------|------|---------|
| L1 当番運用担当 | ____ | ____ | 初動対応、基本的な障害対応 |
| L2 運用責任者 | ____ | ____ | 複雑な障害、複数サービスへの影響 |
| L3 アーキテクト | ____ | ____ | アーキテクチャ級の問題、データ復旧 |
| クラスター管理者 | ____ | ____ | K8s クラスター自体の障害 |
| ネットワーク/セキュリティ | ____ | ____ | ネットワークポリシー、証明書、セキュリティインシデント |

### 7.3 ベンダー連絡先

| ベンダー | 用途 | 連絡先 |
|--------|------|---------|
| LLM Provider | AI Chat/診断 | ____ |
| Registry | イメージレジストリ | ____ |
| DNS/CDN | ドメイン名解決 | ____ |

---

## 付録: Prometheus メトリクス一覧

k8ops は以下のカスタムメトリクスを公開します（`/metrics` エンドポイント）：

| Metric | タイプ | ラベル | 説明 |
|--------|------|------|------|
| `k8ops_diagnostics_total` | Counter | phase, trigger | 診断レポート総数 |
| `k8ops_remediation_actions_total` | Counter | type, result, risk | 修復操作総数 |
| `k8ops_llm_call_duration_seconds` | Histogram | provider, model, status | LLM 呼び出しレイテンシー |
| `k8ops_llm_tokens_total` | Counter | provider, model, type | Token 消費量 |
| `k8ops_agent_steps` | Histogram | - | Agent 実行ステップ数 |
| `k8ops_tool_call_duration_seconds` | Histogram | tool, success | ツール呼び出しレイテンシー |
| `k8ops_safety_blocks_total` | Counter | reason | セキュリティブロック回数 |
| `k8ops_active_diagnostics` | Gauge | - | 現在のアクティブ診断数 |
| `k8ops_active_remediations` | Gauge | - | 現在実行中の修復 |
| `k8ops_audit_events_total` | Counter | type, severity | 監査イベント総数 |
| `k8ops_cluster_health_score` | Gauge | - | クラスター健康スコア (0-100) |
| `k8ops_conversation_count` | Gauge | - | アクティブな対話数 |
| `k8ops_tool_executions_total` | Counter | tool, success | ツール実行総数 |
| `k8ops_http_requests_total` | Counter | method, path, status | HTTP リクエスト総数 |
| `k8ops_http_request_duration_seconds` | Histogram | method, path | HTTP リクエストレイテンシー |
| `k8ops_http_requests_in_flight` | Gauge | - | 現在処理中のリクエスト数 |
| `k8ops_api_errors_total` | Counter | method, path, status | API エラー数 (4xx+5xx) |

---

## 8. SLO/SLA 定義

### 8.1 サービスレベル目標 (SLO)

| 指標 | 目標 | 測定期間 | エラーバジェット |
|------|------|----------|----------|
| Dashboard 可用性 | 99.9% | 30日ローリング | 43.2 分/月 |
| API 成功率 (429 以外) | 99.5% | 30日ローリング | 3.6 時間/月 |
| API P99 レイテンシー | < 2s | リアルタイム | - |
| AI Chat 応答時間 | < 30s (初回 token) | リアルタイム | - |
| セキュリティ監査スキャン完了 | < 60s | リアルタイム | - |

### 8.2 エラーバジェット管理

月間可用性目標 99.9% = **43.2 分のエラーバジェット**:

- **バジェット内 (<30分)**: 通常のリリースペース、追加承認は不要
- **バジェット警告 (30-43分)**: 緊急でない変更を凍結、信頼性問題の修正を優先
- **バジェット消費 (>43分)**: リリースを全面凍結、事後分析 (post-mortem) を実施

### 8.3 SLO 監視クエリ (Prometheus PromQL)

**API エラー率 (5分間):**
```promql
sum(rate(k8ops_api_errors_total{status=~"5.."}[5m])) by (path)
/ sum(rate(k8ops_http_requests_total[5m])) by (path)
```

**API P99 レイテンシー:**
```promql
histogram_quantile(0.99,
  sum(rate(k8ops_http_request_duration_seconds_bucket[5m])) by (le, path)
)
```

**エラーバジェット消費率:**
```promql
1 - (
  sum(rate(k8ops_http_requests_total{status!~"5.."}[30d]))
  / sum(rate(k8ops_http_requests_total[30d]))
)
```

### 8.4 デグレーション戦略

SLO が破られそうな場合、優先度順にデグレードします：

1. **AI Chat の無効化** — 最大のリソース消費機能、デグレード後もコアの K8s 管理に影響なし
2. **キャッシュ TTL の増加** — overview/nodes/pods キャッシュを 30s から 120s に引き上げ
3. **同時診断の制限** — `k8ops_active_diagnostics` の上限を引き下げ
4. **イベントコレクターの停止** — `--disable-event-collector` フラグ

### 8.5 リクエスト追跡

すべての HTTP レスポンスには `X-Request-ID` ヘッダーが含まれ、以下に使用されます：
- ログ相関 — 同一リクエストのすべてのログ行が request_id を共有
- 監査追跡 — 監査ログ内の request_id で具体的な HTTP リクエストに関連付け
- トラブルシューティング — ユーザーが問題を報告する際に request_id を提供すればログを迅速に特定可能

ログ検索例:
```bash
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep "a1b2c3d4e5f6"
```

### 8.6 ログレベル構成

k8ops は構造化 JSON ログ (slog) を使用し、環境変数 `LOG_LEVEL` またはコマンドライン `--log-level` でレベルを構成できます：

| レベル | 用途 | 説明 |
|------|------|------|
| `debug` | 問題の調査 | source file:line を含む、非常に詳細なログ（本番環境では非推奨） |
| `info` | デフォルト | 通常の操作ログ（本番環境での使用を推奨） |
| `warn` | 警告のみ | 遅いリクエスト、構成の問題、閾値への接近 |
| `error` | エラーのみ | 操作失敗のみを記録 |

構成方法：
```bash
# 環境変数で構成（推奨）
kubectl set env daemonset/k8ops -n k8ops-system LOG_LEVEL=debug

# ConfigMap で構成
kubectl patch configmap k8ops-config -n k8ops-system \
  --type='json' -p='[{"op":"add","path":"/data/log-level","value":"debug"}]'

# コマンドライン引数で構成（Deployment モードのみ適用）
# args:
# - --log-level=debug
```

レベル切り替え後に Pod を再起動：
```bash
kubectl rollout restart daemonset/k8ops -n k8ops-system
```

### 8.7 ログローテーション

監査ログファイル (`/data/k8ops-audit.jsonl`) は自動的にローテーションされます：
- **自動ローテーション**: ファイルが 100MB を超えると自動的に分割
- **手動ローテーション**: `POST /api/system/log/rotate`（admin 権限）
- **古いファイルのクリーンアップ**: `POST /api/system/log/cleanup`（30 日以上のローテーションファイルを削除）

コンテナの stdout ログは Kubelet が管理し、デフォルトで各コンテナ 10MB x 3 ファイル = 30MB が上限です。
k3s では `--container-log-max-size` と `--container-log-max-files` で調整可能です。

---

*最終更新: 2026-07-02*
*メンテナー: k8ops Team*
