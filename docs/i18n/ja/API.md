# k8ops API リファレンス

すべてのエンドポイントはダッシュボードポート（デフォルト `:9090`）で提供されます。

**認証:** JWT Cookie（`k8ops_token`）または `Authorization: Bearer <token>` ヘッダー。
**Content-Type:** すべての POST/PUT リクエストで `application/json`。

## OpenAPI 3.0 仕様

k8ops は OpenAPI 3.0 仕様を自動生成し、SDK の自動生成、API ゲートウェイの統合、Swagger UI での参照に利用できます。

| エンドポイント | 説明 |
|------|------|
| `GET /api/openapi.json` | 完全な OpenAPI 3.0 JSON 仕様を返す |
| `GET /api/docs` | タググループ別の API ドキュメントメタデータを返す（spec + tagGroups を含む） |

**仕様の取得：**
```bash
curl -sk https://k8ops.iot2.win/api/openapi.json -o k8ops-openapi.json
```

**Swagger Editor へのインポート：**
1. https://editor.swagger.io を開く
2. ファイル → ファイルのインポート → `k8ops-openapi.json` を選択

**Dashboard で参照：** サイドバー → API Docs ページがインタラクティブな API ブラウザーを提供し、検索、フィルタリング、オンライン試行をサポートします。

---

## ヘルス & システム

| Method | Path | Auth | 説明 |
|--------|------|------|-------------|
| GET | `/api/health` | なし | Liveness プローブ — `{"status":"ok"}` を返す |
| GET | `/api/version` | なし | ビルドバージョン、git commit、Go バージョン |

## クラスター

| Method | Path | Auth | 説明 |
|--------|------|------|-------------|
| GET | `/api/cluster/overview` | 要 | クラスターサマリー: ノード数、Pod 数、CPU/メモリ使用量、警告（30s キャッシュ） |
| GET | `/api/nodes` | 要 | すべてのノードのリソース使用量とコンディション（30s キャッシュ） |
| GET | `/api/nodes/{node}/pods` | 要 | 特定ノードで実行中の Pod |
| GET | `/api/pods` | 要 | 全ネームスペースの Pod リスト（30s キャッシュ） |
| GET | `/api/pods/{namespace}/{name}/containers` | 要 | Pod のコンテナリスト |
| GET | `/api/pods/{namespace}/{name}/logs?container=&follow=&tailLines=` | 要 | Pod ログ（`follow=true` で SSE ストリーミングをサポート） |
| GET | `/api/events?namespace=&warning=` | 要 | Kubernetes イベント、ネームスペース/警告でフィルタ可能 |
| GET | `/api/resources?kind=&namespace=` | 要 | 汎用リソースリスター（Deployments、Services など）（60s キャッシュ） |
| GET | `/api/crds?with_counts=true` | 要 | Custom Resource Definitions（カウント付きで 10min キャッシュ） |
| GET | `/api/crd-resources?group=&version=&resource=&namespace=` | 要 | CRD インスタンス（60s キャッシュ） |
| GET | `/api/yaml?namespace=&name=&group=&version=&resource=&kind=` | 要 | 任意の Kubernetes リソースの YAML ビュー |

## 診断と修復

| Method | Path | Auth | 説明 |
|--------|------|------|-------------|
| GET | `/api/diagnostics` | 要 | DiagnosticReport CR のリスト、オプション `?namespace=` フィルタ |
| GET | `/api/diagnostics/{namespace}/{name}` | 要 | AI 分析を含む診断詳細 |
| GET | `/api/remediations` | 要 | Remediation CR のリスト、オプション `?namespace=` フィルタ |
| GET | `/api/optimizations` | 要 | Optimization CR のリスト、オプション `?namespace=` フィルタ |

## AI チャット

| Method | Path | Auth | 説明 |
|--------|------|------|-------------|
| POST | `/api/chat` | 要 | AI アシスタントにメッセージを送信（SSE ストリーミングレスポンス） |
| GET | `/api/chat/conversations?id=` | 要 | 会話リスト、または ID で 1 件取得 |

### POST /api/chat

**リクエスト:**
```json
{
  "message": "Why is my pod crashing?",
  "conversation_id": "optional-existing-id",
  "stream": true
}
```

**レスポンス:** ツール呼び出しと結果を含む AI 分析の SSE ストリーム。

### GET /api/chat/conversations

会話履歴を返します。`?id=<uuid>` で単一の会話を取得可能。

## プロバイダー管理

| Method | Path | Auth | 説明 |
|--------|------|------|-------------|
| GET | `/api/provider/status` | 要 | 現在の AI プロバイダー設定（API キーはマスク済み） |
| POST | `/api/provider/update` | 要 | 実行時にプロバイダータイプ/モデル/エンドポイントを更新 |
| POST | `/api/provider/reload` | 要 | K8opsConfig CRD からプロバイダー設定をリロード |
| GET | `/api/tools` | 要 | 登録済み診断ツールのリスト |

## 認証

| Method | Path | Auth | 説明 |
|--------|------|------|-------------|
| POST | `/api/auth/login` | 公開 | ローカルログイン（レート制限あり） |
| POST | `/api/auth/logout` | 要 | 認証 Cookie をクリア |
| GET | `/api/auth/me` | 要 | 現在のユーザー情報 |
| POST | `/api/auth/change-password` | 要 | 自分のパスワードを変更 |
| GET | `/api/auth/status` | 公開 | 認証設定状態（auth_enabled、user_count、ldap/oidc フラグ） |
| GET | `/api/auth/provider-presets` | 公開 | 利用可能な OIDC/LDAP プロバイダーテンプレート |

### POST /api/auth/login

**リクエスト:**
```json
{
  "username": "admin",
  "password": "admin"
}
```

**レスポンス (200):**
```json
{
  "user": {"id": 1, "username": "admin", "role": "admin", "display_name": "Administrator"},
  "must_change": true,
  "redirect_url": "/"
}
```

`k8ops_token` Cookie を設定（HttpOnly、SameSite=Lax、24h）。

**エラー (401):**
```json
{"error": "invalid username or password"}
```

## OIDC

| Method | Path | Auth | 説明 |
|--------|------|------|-------------|
| GET | `/api/auth/oidc/{provider}/login` | 公開 | OIDC プロバイダーへリダイレクト（CSRF state Cookie を設定） |
| GET | `/api/auth/oidc/{provider}/callback` | 公開 | OIDC コールバック（state を検証、ユーザーセッションを作成） |

## 認証プロバイダー管理 (管理者)

| Method | Path | Auth | 説明 |
|--------|------|------|-------------|
| GET | `/api/auth/providers` | 管理者 | 設定済み認証プロバイダーのリスト |
| POST | `/api/auth/providers` | 管理者 | 認証プロバイダーを作成（LDAP/OIDC） |
| GET | `/api/auth/providers/{id}` | 管理者 | ID でプロバイダーを取得 |
| PUT | `/api/auth/providers/{id}` | 管理者 | プロバイダー設定を更新 |
| DELETE | `/api/auth/providers/{id}` | 管理者 | プロバイダーを削除 |

## ユーザー管理 (管理者)

| Method | Path | Auth | 説明 |
|--------|------|------|-------------|
| GET | `/api/admin/users` | 管理者 | 全ユーザーのリスト |
| POST | `/api/admin/users` | 管理者 | ユーザーを作成（デフォルトロール: viewer、MustChangePwd=true） |
| GET | `/api/admin/users/{id}` | 管理者 | ID でユーザーを取得 |
| PUT | `/api/admin/users/{id}` | 管理者 | ユーザーを更新（ロール、ネームスペースなど） |
| DELETE | `/api/admin/users/{id}` | 管理者 | ユーザーを削除 |
| POST | `/api/admin/users/{id}/reset-password` | 管理者 | パスワードをリセット（MustChangePwd=true を設定） |
| GET | `/api/admin/auth-config` | 管理者 | 認証設定を取得 |
| PUT | `/api/admin/auth-config` | 管理者 | 認証設定を更新 |

## API キー

| Method | Path | Auth | 説明 |
|--------|------|------|-------------|
| GET | `/api/auth/api-keys` | 要 | 自分の API キーのリスト |
| POST | `/api/auth/api-keys` | 要 | API キーを作成 |
| DELETE | `/api/auth/api-keys/{id}` | 要 | API キーを取り消す |

## RBAC 管理 (管理者)

| Method | Path | Auth | 説明 |
|--------|------|------|-------------|
| GET | `/api/rbac/clusterroles` | 管理者 | ClusterRole のリスト |
| GET | `/api/rbac/clusterroles/{name}` | 管理者 | 名前で ClusterRole を取得 |
| DELETE | `/api/rbac/clusterroles/{name}` | 管理者 | ClusterRole を削除 |
| GET | `/api/rbac/roles?namespace=` | 管理者 | ネームスペースロールのリスト |
| GET | `/api/rbac/roles/{namespace}/{name}` | 管理者 | ネームスペースロールを取得 |
| DELETE | `/api/rbac/roles/{namespace}/{name}` | 管理者 | ネームスペースロールを削除 |
| GET | `/api/rbac/rolebindings?namespace=` | 管理者 | RoleBinding のリスト |
| GET | `/api/rbac/rolebindings/{namespace}/{name}` | 管理者 | RoleBinding を取得 |
| DELETE | `/api/rbac/rolebindings/{namespace}/{name}` | 管理者 | RoleBinding を削除 |
| GET | `/api/rbac/api-resources` | 管理者 | Kubernetes API リソースタイプのリスト |
| GET | `/api/rbac/namespaces` | 管理者 | 全ネームスペースのリスト |
| GET | `/api/rbac/role-mapping?role=&kind=&name=&namespace=` | 管理者 | ロールとサブジェクトのマッピングを表示 |
| GET | `/api/rbac/role-defs` | 管理者 | k8ops カスタムロール定義のリスト |
| GET | `/api/rbac/subjects?kind=&namespace=` | 管理者 | サブジェクト（ユーザー/グループ/ServiceAccount）のリスト |

## 監査

| Method | Path | Auth | 説明 |
|--------|------|------|-------------|
| GET | `/api/audit?namespace=&limit=` | 要 | 監査ログエントリ（ページネーション） |
| GET | `/api/audit/stats` | 要 | 監査統計サマリー |

## 設定

| Method | Path | Auth | 説明 |
|--------|------|------|-------------|
| GET | `/api/config` | 要 | k8ops コントローラー設定（プロバイダータイプ/モデル、機能） |

## セキュリティ監査

| Method | Path | Auth | 説明 |
|--------|------|------|-------------|
| GET | `/api/security/audit` | 要 | クラスターセキュリティスキャン — Pod Security Standards、RBAC、ネットワークポリシーカバレッジ、シークレットセキュリティをチェック |
| GET | `/api/security/health` | 要 | プラットフォームセキュリティヘルスチェック — 認証/TLS/K8s API 接続性 |

### GET /api/security/audit

クラスター全体をスキャンし、重大度順（critical > high > medium > low > info）にソートされたセキュリティ発見リストを返します。

**チェック項目：**
- **Pod Security:** 特権コンテナ、root 実行、権限昇格、危険な capabilities、hostPath/hostNetwork
- **RBAC:** cluster-admin バインディング、デフォルト SA の使用
- **Network:** NetworkPolicy のないネームスペース
- **Secrets:** Docker registry シークレットのローテーション推奨
- **Resources:** resource limits がないコンテナ

**レスポンス例：**
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

## 書き込み操作

| Method | Path | Auth | 説明 |
|--------|------|------|-------------|
| POST | `/api/scale` | 要 | deployment/statefulset のスケール |
| POST | `/api/pod/delete` | 要 | 単一 Pod の削除 |
| POST | `/api/rollout/restart` | 要 | deployment/daemonset/statefulset のローリング再起動 |
| POST | `/api/node/cordon` | 要 | ノードの cordon/uncordon |
| POST | `/api/yaml/apply` | 要 | YAML の適用 (kubectl apply) |

すべての書き込み操作は監査ログに記録されます。

---

## エラーレスポンス

すべてのエラーは JSON を返します：

```json
{"error": "descriptive error message"}
```

| コード | 意味 |
|------|---------|
| 400 | Bad request（パラメータの欠落/無効） |
| 401 | Unauthorized（トークンの欠落/期限切れ/無効） |
| 403 | Forbidden（ロールが不足） |
| 404 | リソースが見つからない |
| 500 | Internal server error |
| 503 | Service unavailable（AI プロバイダー未設定） |

## ロール

| ロール | 権限 |
|------|-------------|
| `admin` | ユーザー/RBAC/プロバイダー管理を含むフルアクセス |
| `operator` | Dashboard + 診断 + チャット（ユーザー管理なし） |
| `viewer` | 読み取り専用 Dashboard + チャット |
| `ns-admin` | 割り当てられたネームスペース内のみ管理者 |
| `ns-viewer` | 割り当てられたネームスペース内のみ閲覧者 |

## 新規エンドポイント (v14.48-v14.53)

以下のエンドポイントは v14.48 から v14.53 の間に追加され、OpenAPI 3.0 仕様に含まれています。

### コンテナイメージインベントリ

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/images` | クラスター内のすべてのコンテナイメージインベントリ、リソース制限監査と `:latest` タグ検出を含む |

**レスポンスサマリーフィールド：** `totalImages`、`withoutLimits`、`withoutRequests`、`usingLatestTag`、`uniqueRegistries`

### 警告イベントサマリー

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/events/summary` | すべての Warning イベントを Reason で集計、重大度分類と影響を受けたネームスペース統計を含む |

### クラスター効率分析

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/efficiency` | クラスターのリソース効率分析：無制限 Pod、過剰プロビジョニングコンテナ、未活用ノード、効率スコア 0-100 |

### セキュリティ: Secret 露出スキャン

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/security/secrets` | ハードコードされた認証情報の検出、Secret ローテーション追跡（90日）、未使用 Secret、機密キー名 |

### 監査検索とエクスポート

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/audit/events` | 監査イベント検索：`actor`、`action`、`q`（全文検索）、`severity`、日付範囲フィルタをサポート |
| GET | `/api/audit/export` | 監査イベントを CSV 形式でエクスポート（SIEM システムにインポート可能） |

### バックアップ管理

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/system/backup` | すべてのバックアップファイルのリスト（サイズ、経過日数、タイプ） |
| POST | `/api/system/backup` | データベースバックアップを作成（タイムスタンプ付きファイル名） |
| DELETE | `/api/system/backup?name=X` | 指定バックアップを削除（パストラバーサル防止） |
| POST | `/api/system/backup/restore?name=X` | バックアップからデータベースをリストア |

### Alertmanager Webhook

| Method | Path | 説明 |
|--------|------|-------------|
| POST | `/api/webhooks/alertmanager` | Prometheus Alertmanager v4 アラートを受信、調査提案を自動生成 |
| POST | `/api/webhooks/alertmanager/test` | テストアラートを送信してレシーバーを検証 |

**Alertmanager 設定例：**
```yaml
receivers:
  - name: k8ops
    webhook_configs:
      - url: http://k8ops.k8ops-system.svc:9090/api/webhooks/alertmanager
        send_resolved: true
```

### 変更履歴

| バージョン | エンドポイント | 次元 |
|------|------|------|
| v14.49 | `GET /api/events/summary` | Product |
| v14.50 | スタートアッププローブ + preStop | Deployment |
| v14.51 | `POST /api/webhooks/alertmanager` | Operations |
| v14.52 | `GET /api/efficiency` | Scalability |
| v14.53 | `GET /api/security/secrets` | Security |
| v14.54 | OpenAPI 3.0 spec + API.md | Documentation |
| v14.55 | `GET /api/pdbs` `GET /api/compatibility` | Product |
| v14.56 | `GET /api/certificates/expiry` | Operations |
| v14.57 | グレースフルシャットダウン draining gate | Deployment |
| v14.58 | `GET /api/addons/health` | Product |
| v14.59 | `GET /api/capacity/forecast` | Scalability |
| v14.60 | OpenAPI spec 補完 + API.md 更新 | Documentation |
| v14.61 | `GET /api/security/network-policies` | Security |
| v14.62 | `GET /api/diagnostics/restarts` | Operations |
| v14.63 | `GET /api/deployments/rollout` | Deployment |
| v14.64 | `GET /api/resources/waste` | Product |
| v14.65 | `GET /api/scaling/bottlenecks` | Scalability |

### Pod Disruption Budget ステータス (v14.55+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/pdbs` | すべての PDB のリスト、disruption ステータス、一致するワークロード、健全性評価（healthy/at-risk/blocked）を含む、drain 前の安全確認用 |

### K8s ディストリビューション互換性検出 (v14.55+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/compatibility` | クラスターのディストリビューション（vanilla/k3s/RKE2/EKS/GKE/AKS/OpenShift/Talos）、バージョン互換性、ARM/Windows/GPU ノード特性を自動検出 |

### TLS 証明書期限切れスキャン (v14.56+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/certificates/expiry` | すべての TLS/Opaque Secret 内の X.509 証明書をスキャン、期限切れ時間で分類（expired/critical/warning/ok）、Ingress リソースを関連付け |

### サーバー Drain ステータス (v14.57+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/system/drain-status` | サーバーのグレースフルシャットダウン状態を報告：draining、shutdownInitiated、activeConnections、uptime |

### アドオン健全性検出 (v14.58+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/addons/health` | 39 種類の一般的な K8s アドオンを非侵襲的に検出（12 カテゴリ：CNI/DNS/Ingress/CertManager/LB/Mesh/Backup/Monitoring/Policy/Storage/GitOps/VM）、健全性状態を報告 |

### 容量枯渇予測 (v14.59+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/capacity/forecast` | CPU/メモリ/Pod/ストレージ容量がいつ枯渇するかを予測、成長率に基づく days-to-exhaustion と拡張推奨を提供 |

### Network Policy 監査スキャン (v14.61+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/security/network-policies` | NetworkPolicy カバレッジを監査：NetworkPolicy のないネームスペース、ルーズなポリシー（0.0.0.0/0 の入/出力）、部分カバレッジを検出、重大度で分類（critical/warning/info） |

**クエリパラメータ：** `namespace`（オプション、ネームスペースフィルタ）

**返却例：**
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

### Pod 再起動診断 (v14.62+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/diagnostics/restarts` | Pod の再起動パターンと根本原因を診断：再起動動作の分類（crash-loop/occasional/post-deploy）、終了理由の抽出（OOMKilled/Error/終了コード）、待機状態（CrashLoopBackOff/ImagePullBackOff） |

**クエリパラメータ：** `namespace`（オプション）

**診断パターン：**
- **crash-loop**: 短時間に大量の再起動
- **occasional**: 長期間に少数の再起動
- **post-deploy**: デプロイ直後の再起動

### デプロイメント Rollout ステータス追跡 (v14.63+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/deployments/rollout` | すべての Deployment/StatefulSet/DaemonSet の rollout 健全性状態をスキャン：7 つの状態（complete/in-progress/stalled/degraded/paused/failed/scaled-to-zero）、ProgressDeadlineExceeded、ReplicaFailure、generation mismatch を検出 |

**クエリパラメータ：**
- `namespace`（オプション）— ネームスペースフィルタ
- `status`（オプション）— rollout 状態でフィルタ：`failed`、`degraded`、`stalled`、`in-progress`、`paused`、`scaled-to-zero`、`complete`

**状態の説明：**
| 状態 | 意味 |
|------|------|
| `complete` | すべてのレプリカが更新され準備完了 |
| `in-progress` | ローリングアップデートが進行中 |
| `stalled` | コントローラーが最新 spec を観測していない（generation mismatch） |
| `degraded` | 一部のレプリカが利用不可 |
| `paused` | Deployment が明示的に一時停止されている |
| `failed` | ProgressDeadlineExceeded、デプロイタイムアウトで失敗 |
| `scaled-to-zero` | レプリカ数が 0 |

### リソース無駄検出 (v14.64+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/resources/waste` | クラスター内の無駄と孤立リソースをスキャンしてコストを削減：6 種類の無駄（dead-service/unused-pvc/orphaned-configmap/orphaned-secret/empty-namespace/unattached-pv）、4 レベルの重大度（critical/high/medium/low）、コストリスク評価 |

**クエリパラメータ：** `namespace`（オプション）

**無駄のタイプ：**
| カテゴリ | 検出内容 | デフォルト重大度 |
|------|---------|-----------|
| `dead-service` | バックエンドエンドポイントのない Service（LoadBalancer は critical） | medium/critical |
| `unused-pvc` | どの Pod にもマウントされていない PVC | high |
| `orphaned-configmap` | どの Pod にも参照されていない ConfigMap | low/medium |
| `orphaned-secret` | どの Pod にも参照されていない Secret（セキュリティリスク） | high |
| `empty-namespace` | 実行中 Pod のないネームスペース | medium |
| `unattached-pv` | Available 状態の PV（どの PVC にもバインドされていない） | critical |

**スマートフィルタリング：** kube-system ネームスペース、ServiceAccount token Secret、Helm release Secret、自動生成された ConfigMap を自動的にスキップ

### スケーリングボトルネック検出 (v14.65+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/scaling/bottlenecks` | 水平スケーリングを制限する要因をスキャン：7 種類のボトルネック（node-schedulable/node-pressure/resource-quota/hpa-stuck/pdb-blocking/storage-exhaust/image-pull-limit）、4 レベルの影響度（critical/high/moderate/low）、クラスターキャパシティサマリー |

**クエリパラメータ：** `namespace`（オプション）

**ボトルネックのタイプ：**
| カテゴリ | 検出内容 |
|------|---------|
| `node-schedulable` | cordon されたノード、クラスター Pod 容量超過（>75% 警告 / >90% 重大） |
| `node-pressure` | メモリ、ディスク、PID 圧力状態 |
| `resource-quota` | ネームスペースクォータが 75%/90% を超過 |
| `hpa-stuck` | HPA が最大レプリカ数に達した、またはメトリクスが不足 |
| `pdb-blocking` | PDB が 0 回の自発的中断を許可 |
| `storage-exhaust` | ネームスペース PVC リクエストが 500Gi を超過 |

**クラスターキャパシティサマリー：** ノード数、CPU/メモリ容量と割り当て可能量、Pod 容量と割り当て済み量、スケーリングの余裕を提供

### RBAC 権限リスク分析 (v14.67+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/security/rbac-risk` | すべての RoleBinding/ClusterRoleBinding の権限リスクを分析、0-100 スコアリングシステム、5 レベルのリスク（critical/high/elevated/moderate/low）、cluster-admin バインディング、権限昇格、ワイルドカード権限、機密リソースアクセスを検出 |

**クエリパラメータ：** `namespace`（オプション）

**リスクスコアリングルール：**
| 検出項目 | 基礎スコア | 追加スコア |
|--------|--------|--------|
| ClusterRoleBinding + cluster-admin | 100 | - |
| 権限昇格（escalate/bind/impersonate） | - | +25 |
| ワイルドカード動詞（verbs: *） | - | +25 |
| ワイルドカードリソース（resources: *） | - | +20 |
| クラスタースコープ書き込み操作 | - | +30 |
| 機密リソースアクセス（secrets/pods/exec） | - | +15 |

### CronJob 実行健全性監視 (v14.68+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/operations/cronjobs/health` | すべての CronJob の実行健全性を監視：成功率、連続失敗、一時停止/停滞スケジュール、未実行、5 レベルの健全性状態（healthy/warning/failing/suspended/no-runs） |

**クエリパラメータ：** `namespace`（オプション）

**健全性状態：**
| 状態 | トリガー条件 |
|------|---------|
| `failing` | 3 回以上の連続失敗 |
| `warning` | 1-2 回の連続失敗、または成功率 < 50% |
| `suspended` | CronJob が suspend されている |
| `no-runs` | 一度も実行されていない |
| `healthy` | 最近すべて成功 |

### Service & Endpoint 健全性監視 (v14.69+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/networking/health` | すべての Service と Ingress のネットワーク健全性をスキャン：エンドポイントのない Service、セレクター不一致、エンドポイント劣化、LoadBalancer 待機、Ingress バックエンド Service 欠落/エンドポイントなし、5 レベルの健全性状態 |

**クエリパラメータ：** `namespace`（オプション）

**Service 健全性状態：**
| 状態 | 意味 |
|------|------|
| `misconfigured` | セレクター不一致 — label に一致する Pod がない |
| `no-endpoints` | すべてのエンドポイントが利用不可 |
| `degraded` | 一部のエンドポイントが利用不可 |
| `external` | ExternalName/LoadBalancer（情報提供） |
| `healthy` | すべてのエンドポイントが正常 |

**Ingress 健全性チェック：** バックエンド Service の存在確認、利用可能なエンドポイントの有無を検出、デフォルトバックエンドとルールパスを検証

### PV/PVC ストレージ健全性監視 (v14.70+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/storage/health` | すべての PVC/PV のストレージ健全性をスキャン：Pending PVC 診断、孤立 PVC（バインド済みだが Pod 使用なし > 1日）、Lost/Failed PVC、Released/Failed PV の手動クリーンアップ必要性、古い Available PV の容量無駄、6 レベルの健全性状態 + ストレージクラス分布分析 |

**クエリパラメータ：** `namespace`（オプション）

**PVC 健全性状態：**
| 状態 | 意味 |
|------|------|
| `failed` | PVC のプロビジョニング失敗 |
| `lost` | 基盤の PV が削除された |
| `pending` | プロビジョニング待ち（ストレージクラスなし、WaitForFirstConsumer） |
| `near-capacity` | 容量上限に接近 |
| `orphaned` | バインド済みだが 1 日以上 Pod 使用がない |
| `bound` | 正常にバインドされている |

**PV 問題検出：** Released PV（手動クリーンアップが必要）、Failed PV（回収失敗）、古い Available PV（>7 日間の容量無駄）

**ストレージクラス分析：** デフォルトクラスのマーク、provisioner、reclaim policy、binding mode、volume expansion サポート、PVC 数量分布

### ServiceAccount セキュリティ監査 (v14.72+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/security/service-accounts` | すべての ServiceAccount のセキュリティ状況を包括的に監査：未使用 SA、デフォルト SA の Pod 使用、不要な token 自動マウント、cluster-admin バインディング、クラスタースコープ権限、古い SA、レガシーの長寿命 token secret |

**クエリパラメータ：** `namespace`（オプション）

**リスクスコア：** 0-100（高いほど危険）、5 レベルの重大度：critical / high / elevated / moderate / low

**検出項目：**
| 検出項目 | 重大度 | 説明 |
|--------|--------|------|
| 未使用 SA（>7 日 Pod 参照なし） | moderate | 攻撃面の拡大 |
| デフォルト SA の Pod 使用 | elevated | 最小権限の原則違反 |
| cluster-admin バインディング | critical | クラスター级のスーパー権限 |
| 不要な token 自動マウント | moderate | token が不要な SA はマウントすべきでない |
| 古い SA（>30 日未使用だが権限あり） | high | ゾンビ権限 |
| レガシー長寿命 token secret（K8s <1.24） | high | 非推奨の長寿命 token |

### SLO/SLA エラー予算追跡 (v14.73+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/operations/slo` | マルチウィンドウ・マルチバーンレートアルゴリズムに基づく SLO/SLA 可用性とエラー予算の追跡 |

**クエリパラメータ：** `namespace`（オプション）

**ウィンドウ設定：** 5m / 1h / 6h / 24h / 7d

**返却内容：**
| フィールド | 説明 |
|------|------|
| `availability` | 各ウィンドウの可用性パーセンテージ |
| `errorBudget` | エラー予算の残量と消費率 |
| `burnRate` | マルチウィンドウバーンレート（fast: 5m/1h, slow: 6h/24h） |
| `latencySLO` | P50/P95/P99 レイテンシパーセンタイルと目標 |
| `status` | meeting（達成）/ at-risk（リスク）/ violated（違反）|

### ResourceQuota と LimitRange 監視 (v14.74+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/resources/quota` | すべてのネームスペースの ResourceQuota 使用率と LimitRange デフォルト制約をスキャン |

**クエリパラメータ：** `namespace`（オプション）

**クォータステータスレベル：**
| 状態 | 使用率 | 説明 |
|------|--------|------|
| `ok` | <70% | 正常 |
| `warning` | 70-85% | 上限に接近 |
| `critical` | 85-100% | 危険 |
| `exceeded` | >100% | 超過済み |
| `no-limit` | — | クォータ設定なし |

**検出項目：** ネームスペースごとの CPU/メモリ/Pod/ConfigMap/Secret/ストレージクォータ使用率、クォータ保護のないネームスペース、LimitRange デフォルト/最小/最大制約分析、上位消費者ランキング

### デプロイメント設定監査 (v14.75+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/deployments/audit` | すべての Deployment/StatefulSet/DaemonSet の設定ベストプラクティス違反を監査、8 チェックカテゴリ、各項目に重大度と修正提案を含む |

**クエリパラメータ：** `namespace`（オプション）、`severity`（オプション：critical / warning / info）

**チェックカテゴリ：**
| カテゴリ | チェック項目 |
|------|--------|
| `revision-history` | リビジョン履歴が少なすぎる（< 2、ロールバック不可）または多すぎる（> 20、リソース浪費） |
| `image-policy` | `:latest` タグだが pullPolicy が Always でない、固定タグだが pullPolicy が Always |
| `resources` | リソース limits/requests が不足 |
| `probes` | liveness/readiness/startup プローブが不足 |
| `security-context` | 特権コンテナ、root 実行、書き込み可能なルートファイルシステム、権限昇格の許可 |
| `update-strategy` | Recreate 戦略（ダウンタイム）、OnDelete（Pod の手動削除が必要）、パーティションローリングアップデート |
| `lifecycle` | terminationGracePeriod が短すぎる（< 10s）または長すぎる（> 300s）、preStop フックが不足 |
| `config-drift` | seccomp profile が不足 |

**健全性スコア：** 0（完璧）から 100（最悪）、critical=20点/warning=8点/info=2点

### スケジューリング健全性とリソース断片化分析 (v14.76+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/scheduling/health` | クラスターのスケジューリング健全性、ノードのスケジュール可能性、リソース断片化、Pending Pod 診断を分析 |

**クエリパラメータ：** `namespace`（オプション）

**返却内容：**
| フィールド | 説明 |
|------|------|
| `summary` | ノード統計（スケジュール可能/不可/cordon/圧力あり）、Pending Pod 数、FailedScheduling 数、24h の退去数、健全性スコア 0-100 |
| `nodes` | 各ノードのスケジュール可能性、圧力タイプ、taints、CPU/メモリ/Pod の空き量とパーセンテージ |
| `pendingPods` | Pending Pod リスト、CPU/メモリリクエスト、nodeSelector、解析済みのスケジュール失敗原因 |
| `largestFittablePod` | 現在スケジュール可能な最大 Pod（CPU/メモリ/Pod 数）、最適ノード |
| `effectiveCapacity` | 理論容量 vs 有効容量（スケジュール不可ノードによる容量損失パーセンテージ） |
| `fragmentation` | リソース断片化指標（平均 CPU/メモリ断片化率、最悪の断片化ノード、巨大 Pod 検出） |
| `evictions` | 24h 以内の退去記録（Pod、ノード、理由） |
| `recommendations` | 実行可能な修正提案 |

**スケジュール失敗原因の解析：** insufficient-cpu / insufficient-memory / untolerated-taint / node-selector-mismatch / node-affinity-mismatch / pod-affinity-conflict / pod-limit-reached / volume-binding-failure / no-nodes-available

### Pod セキュリティポスチャスキャン (v14.79+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/security/pods` | すべての実行中 Pod のセキュリティポスチャを監査：特権コンテナ、hostNetwork/hostPID/hostIPC、HostPath マウント、危険な Linux capabilities、root 実行、権限昇格の許可、書き込み可能なルートファイルシステム、セキュリティコンテキストの欠落、:latest/タグなしイメージ、digest ロック未使用、Secret 環境変数の注入、リソース制限なし、ホストポートのバインド |

**クエリパラメータ：** `namespace`（オプション）、`severity`（オプション：critical / warning / info）

**リスクスコア：** 0（安全）から 100（極めて危険）、critical=25点/warning=8点/info=2点

**チェックカテゴリ：**
| カテゴリ | 重大度 | 説明 |
|------|--------|------|
| `privileged` | critical | 特権コンテナ — 完全なホストアクセス |
| `host-network` | critical | ノードのネットワークネームスペースを共有 |
| `host-pid` | critical | ノードのすべてのプロセスが可視 |
| `host-ipc` | critical | IPC ネームスペースを共有 |
| `host-path` | critical | ノードから HostPath ボリュームをマウント |
| `dangerous-capabilities` | critical | SYS_ADMIN/NET_ADMIN/NET_RAW/SYS_PTRACE/SYS_MODULE/DAC_OVERRIDE/SETUID/SETGID |
| `runs-as-root` | warning | UID 0 で実行 |
| `privilege-escalation` | warning | 権限昇格が許可されている |
| `missing-security-context` | warning | セキュリティコンテキストの欠落 |
| `image-latest` | warning | :latest タグの使用 |
| `image-no-tag` | warning | タグなし（デフォルト :latest） |
| `host-port` | warning | ホストポートのバインド |
| `image-no-digest` | info | digest によるロック未使用 |
| `writable-rootfs` | info | 書き込み可能なルートファイルシステム |
| `secret-env-vars` | info | Secret が環境変数として注入されている |
| `no-resource-limits` | info | リソース制限なし |

### イベントストームとカスケード障害検出 (v14.80+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/operations/event-storm` | クラスターの Warning イベントを分析し、イベントストーム、カスケード障害、リソースフラッピングを検出。15min/1h/24h タイムウィンドウのアラートイベントを集計し、ストームの重大度を分類し、フラッピングリソース（同じリソース・同じ原因で 3 回以上繰り返し）を特定し、ネームスペースと原因で集計し、実行可能な推奨を提供 |

**クエリパラメータ：** `namespace`（オプション）

**ストームの重大度：**
| 重大度 | 条件 | 説明 |
|--------|------|------|
| `critical` | >50 events/15min | 緊急調査が必要 |
| `high` | >20 events/15min | 注意が必要 |
| `medium` | >10 events/15min | トレンドを監視 |
| `low` | >5 events/15min | 情報提供 |

**返却内容：** ストーム検出結果、ネームスペースアラートランキング、上位イベント原因、フラッピングリソースリスト（フラッピング頻度を含む）、直近 15 分のイベントタイムライン、影響を受けたリソース数（ブラストレイディアス）、実行可能な推奨

### リソース依存グラフと影響範囲分析 (v14.81+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/dependencies` | 任意のワークロードの完全な依存グラフを追跡（Deployment/StatefulSet/DaemonSet/Pod）、変更の影響範囲を評価 |

**クエリパラメータ：**

| パラメータ | 必須 | 説明 |
|------|------|------|
| `kind` | はい | リソースタイプ: Deployment / StatefulSet / DaemonSet / Pod |
| `name` | はい | リソース名 |
| `namespace` | いいえ | ネームスペース（デフォルト: default） |

**正方向の依存（このワークロードが何に依存しているか）：** ConfigMap、Secret、PVC、ServiceAccount

**逆方向の依存（何がこのワークロードに依存しているか）：**
- Service（label selector で Pod にマッチ）
- Ingress（マッチする Service にルーティング）
- NetworkPolicy（この Pod に適用）
- HPA（このワークロードをターゲット）
- ConfigMap/Secret を共有する他の Pod

**影響範囲評価：** blastRadius = 正方向の依存数 + 逆方向の依存数、リスクレベル low(<6) / medium(6-10) / high(11-20) / critical(>20)

### トポロジー分散コンプライアンスチェック (v14.82+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/topology/spread` | Pod のトポロジードメイン（zone/region/node）での分散を分析し、topologySpreadConstraints のコンプライアンスを検証 |

**クエリパラメータ：** `namespace`（オプション）、`domain`（オプション、トポロジードメイン key、デフォルト `kubernetes.io/hostname`、`topology.kubernetes.io/zone` に設定可能）

**ワークロード状態：**
| 状態 | 意味 |
|------|------|
| `balanced` | 均等に分散（actualSkew ≤ maxSkew） |
| `skewed` | 不均等な分散（actualSkew > maxSkew） |
| `no-constraint` | 複数レプリカだがトポロジー制約なし |
| `single-replica` | 単一レプリカ（トポロジー分散は適用外） |

**返却内容：** トポロジードメイン統計、ワークロードごとのドメイン分布（Pod 数/期待値）、実際の偏差 vs 最大偏差、ノードごとのドメインラベルと Pod 数、推奨（制約の追加、ノードのラベル付け、単一ドメインクラスターのヒント）

### Secret ローテーションとライフサイクル監査 (v14.85+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/security/secrets/rotation` | すべての Secret のローテーションコンプライアンスとライフサイクル管理を監査：経過期間の追跡（stale >90d / very stale >180d）、未使用 Secret の検出（どの Pod にも参照されない）、TLS 証明書の期限切れ検出（証明書を解析）、Docker registry Secret の追跡、レガシー ServiceAccount Token の検出、機密名の検出 |

**クエリパラメータ：** `namespace`（オプション）

**リスクスコア：** Secret ごとのリスクレベル（critical / high / medium / low）、クラスターローテーションスコア 0-100

**チェックカテゴリ：**
| チェック項目 | 重大度 | 説明 |
|---------|--------|------|
| TLS 証明書の期限切れ | critical | 即時更新が必要 |
| Docker Secret の経過 >180d | critical | 期限切れのレジストリ認証情報を含む可能性 |
| TLS 証明書の期限切れまで <30d | high | 早急に更新を予定 |
| Stale + 未使用 + 機密名 | high | セキュリティリスク |
| Stale Docker Secret | medium | ローテーションを推奨 |
| Stale だが使用中 | low | ローテーションを計画 |

### ヘルスプローブ有効性監査 (v14.86+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/operations/probes` | すべてのワークロードの liveness/readiness/startup プローブ設定を監査、不適切な設定によるカスケード再起動、未準備 Pod へのトラフィック、起動失敗などの問題を検出 |

**クエリパラメータ：** `namespace`（オプション）

**チェックカテゴリ：**
| チェック項目 | 重大度 | 説明 |
|---------|--------|------|
| liveness の欠落 | warning | ハングしたコンテナが再起動されない |
| readiness の欠落 | warning | トラフィックが未準備の Pod に到達する可能性 |
| プローブが過激すぎる (period <5s) | warning | API server への過大な負荷 |
| タイムアウトが短すぎる (<2s) | warning | レイテンシスパイクで誤判定の可能性 |
| 失敗しきい値が低すぎる (<3) | warning | 一時的エラーに過敏 |
| readiness 間隔が長すぎる (>60s) | info | 準備検出が遅い |
| liveness 失敗しきい値が高すぎる (>10) | info | 再起動の復旧が遅い |
| 同じ liveness+readiness | info | 差別化して設定すべき |

**返却内容：** ワークロードごとのリスクスコア、クラスターレベル有効性スコア (0-100)、上位問題の集計、実行可能な推奨

### ワークロードの陳腐化追跡 (v14.87+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/product/staleness` | すべてのワークロードのデプロイ陳腐化を追跡、長期間未更新のワークロード、:latest タグのイメージ、digest ロック未使用のイメージを検出 |

**クエリパラメータ：** `namespace`（オプション）

**陳腐化分類：**
| 状態 | 条件 | 説明 |
|------|------|------|
| `fresh` | <7d | 最近更新 |
| `recent` | <30d | 比較的新しい |
| `stale` | <90d | 要注意 |
| `very-stale` | <180d | 更新を推奨 |
| `ancient` | >180d | セキュリティリスク |

**返却内容：** ワークロードごとのリスクレベル、イメージタグ分析（:latest / digest / no-tag）、経過期間の分布バケット、ネームスペース統計、クラスターフレッシュネススコア (0-100)、実行可能な推奨

### リソースオーバーコミットと圧力分析 (v14.88+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/scalability/overcommit` | すべてのノードの CPU とメモリのオーバーコミット比率を分析、危険な over-commit、limits のない Pod、リソース圧力スコアを検出 |

**クエリパラメータ：** `namespace`（オプション）

**ノードごとの分析：**
| 指標 | 説明 |
|------|------|
| CPU request commit | sum(requests) / allocatable |
| CPU limit commit | sum(limits) / allocatable |
| Mem request/limit commit | 同上 |
| 圧力スコア | 0-100（加重計算） |
| リスクレベル | safe / moderate / high / critical (>3x) |

**クラスターメトリクス：** 総 CPU/メモリオーバーコミット比率、リスクノード数、limits のない Pod 数、安全スコア (0-100)、ネームスペース別リソース消費明細、実行可能な推奨

### イメージセキュリティとサプライチェーン分析 (v14.92+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/security/images` | すべての実行中コンテナイメージのサプライチェーンセキュリティリスクをスキャン：digest ロック、:latest タグ、タグなしイメージ、古いバージョンタグ、パブリック vs プライベートレジストリ、不明なレジストリ |

**クエリパラメータ：** `namespace`（オプション）

**チェックカテゴリ：**
| チェック項目 | リスクスコア | 説明 |
|---------|--------|------|
| タグなし | +25 | デフォルト :latest、バージョンが不確定 |
| :latest 使用 | +15 | 可変タグ、再現不可 |
| digest ロック未使用 | +10 | イメージ内容がサイレントに置換可能 |
| 不明なレジストリ | +10 | レジストリプレフィックスなし、デフォルト Docker Hub |
| 古いバージョンタグ | +15 | 既知の脆弱性を含む可能性 |
| パブリックレジストリ + ロックなし | +5 | 出典保証なし |

**返却内容：** イメージごとのリスクレベル (critical/high/medium/low)、レジストリごとの統計、上位リスクイメージ、クラスターイメージセキュリティスコア (0-100)、実行可能な推奨

### キャパシティプランニング (v14.50+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/capacity/planning` | ノードキャパシティプランニング分析：ノードごとの CPU/メモリリクエスト vs 割り当て可能量、残り容量、拡張推奨時期、リソース断片化検出 |

### クラスター健全性スコア集計 (v14.93+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/operations/health-score` | すべてのクラスター健全性シグナルを 1 つの総合スコア (0-100、グレード A-F) に集計、5 つの加重次元を組み合わせ |

**5 つの加重次元：**
| 次元 | 重み | チェック内容 |
|------|------|----------|
| Node Health | 25% | ノードの準備状態 |
| Pod Health | 25% | CrashLoop、Pending、Failed、高再起動回数 |
| Workload Health | 20% | Deployment/StatefulSet/DaemonSet の準備完了レプリカ |
| Event Activity | 15% | 直近 1 時間の Warning イベント数 |
| API Server | 15% | API server のリアルタイムレイテンシ計測 |

**返却内容：** 総合スコア 0-100、アルファベットグレード A-F、状態 (healthy/warning/critical)、次元ごとのスコア詳細、クラスターサマリー（ノード/Pod/ワークロード数）、重大度順の上位問題

### HPA/VPA リソース適正設定推奨 (v14.94+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/scalability/autoscale-recommendations` | すべてのワークロードの HPA カバレッジとリソース適正設定を分析、過剰プロビジョニング、HPA のないマルチレプリカワークロード、HPA 効率を検出 |

**クエリパラメータ：** `namespace`（オプション）

**検出カテゴリ：**
| チェック項目 | 説明 |
|---------|------|
| HPA のないマルチレプリカワークロード | オートスケールの追加を推奨 |
| CPU リクエストが高すぎる (>1 core/container) | 高信頼度、半減を推奨 |
| メモリリクエストが高すぎる (>2GB/container) | ライトサイジングを推奨 |
| HPA が maxReplicas に到達 | 容量増加が必要 |
| HPA がアイドル (<20% 使用率) | maxReplicas の削減を推奨 |

**返却内容：** ワークロードごとの現行 vs 推奨 CPU/メモリ値、変化パーセンテージ、信頼度、潜在的 CPU コアとメモリ節約量、HPA 効率分析、クラスターオートスケールスコア (0-100)

### Ingress とトラフィックルーティング健全性監視 (v14.96+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/product/ingress-health` | すべての Ingress リソースのトラフィックルーティング健全性と設定問題を分析 |

**クエリパラメータ：** `namespace`（オプション）

**チェックカテゴリ：**
| チェック項目 | 重大度 | 説明 |
|---------|--------|------|
| バックエンド Service が存在しない | critical | 参照先の Service が存在しない |
| バックエンドに準備完了エンドポイントがない | warning | Service に ready endpoints がない |
| TLS 設定なし | warning | host があるが暗号化されていない |
| IngressClass が存在しない | critical | 指定された class がデプロイされていない |
| host+path の競合 | warning | 複数の Ingress が同じルートを争奪 |
| ルーティングルールなし | warning | Ingress が機能していない |

**返却内容：** Ingress ごとの状態 (healthy/warning/critical)、ネームスペースごとの統計、クラスター健全性スコア (0-100)、実行可能な推奨

### ノードコンディションとリソース圧力分析 (v14.99+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/operations/node-pressure` | すべてのノードのコンディション状態とリソース飽和度を分析 |

**検出カテゴリ：**
| コンディション | リスクスコア | 説明 |
|------|--------|------|
| NetworkUnavailable | +30 | CNI/ネットワークが準備未完了 |
| DiskPressure | +25 | ディスクが満杯または接近 |
| MemoryPressure | +25 | ノードのメモリが枯渇 |
| PIDPressure | +20 | プロセス数が多すぎる |
| NotReady | →critical | kubelet/ランタイムの問題 |
| CPU >90% | +20 | CPU リクエストが飽和 |
| Memory >95% | +20 | メモリリクエストが飽和 |
| Cordoned | — | スケジュール不可 |

**返却内容：** ノードごとのリスクレベル (critical/high/medium/low)、CPU/メモリ/Pod 使用率、コンディション詳細（理由、メッセージ、持続時間）、クラスター圧力スコア (0-100)、実行可能な推奨

### PVC バインディングとストレージパフォーマンス分析 (v15.00+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/scalability/pvc-analysis` | すべての PVC のバインディング健全性とストレージパフォーマンスを分析 |

**クエリパラメータ：** `namespace`（オプション）

**検出カテゴリ：**
| チェック項目 | 重大度 | 説明 |
|---------|--------|------|
| Stuck PVC (>5min) | critical | スタックした PVC + 根本原因分析 |
| Lost PVC | critical | 基盤の PV が削除された可能性 |
| 遅いバインディング (>30s) | warning | ストレージプロビジョニングの遅延 |
| Pending PVC | warning | バインディング待ち |
| デフォルト StorageClass が不足 | info | デフォルト SC が未設定 |

**返却内容：** PVC ごとの状態 (healthy/warning/critical)、バインディング時間、StorageClass ごとの統計、Stuck PVC の根本原因、クラスターのストレージ健全性スコア (0-100)

### Namespace ガバナンスとライフサイクル監査 (v15.02+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/product/namespaces/lifecycle` | すべてのネームスペースのガバナンスコンプライアンスとライフサイクルを監査 |

**ガバナンスチェック：**
| チェック項目 | リスクスコア | 説明 |
|---------|--------|------|
| ResourceQuota なし | +15 | リソースの無制限消費 |
| NetworkPolicy なし | +15 | トラフィックが制限されない |
| LimitRange なし | +5 | デフォルトのリソース制限なし |
| ネームスペースの期限切れ | +10 | 実行中 Pod なし、クリーンアップ候補 |
| 必須ラベルの欠落 | +5 | app/team/env/owner の欠落 |
| デフォルト SA のみ | 0 | 最小権限 SA の欠落 |

**返却内容：** ネームスペースごとのリスクレベル (critical/high/medium/low)、コンプライアンスフラグ、ライフサイクル状態 (active/stale/terminating)、クラスターガバナンススコア (0-100)、実行可能な推奨

### RBAC 実効権限と権限昇格分析 (v15.04+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/security/rbac-effective` | すべてのサブジェクトの RBAC 実効権限と権限昇格リスクを分析 |

ClusterRoleBindings + RoleBindings を集計し、各サブジェクト (User/Group/ServiceAccount) の実際の権限を計算します。

**検出カテゴリ：**

| チェック項目 | リスクスコア | 説明 |
|---------|--------|------|
| cluster-admin と同等 | →critical | ワイルドカード verbs + resources |
| RBAC の作成/変更が可能 | +25 | 自己権限昇格パス |
| ワイルドカード (*) 権限 | +20 | 過度な権限付与 |
| Secrets の読み取りが可能 | +10 | 機密データの漏洩 |
| Pod の exec が可能 | +10 | コンテナ脱出の入口 |

**返却内容：** サブジェクトごとのリスクレベル、権限昇格パスの詳細、クラスター RBAC セキュリティスコア (0-100)、実行可能な推奨

### コンテナ OOM Kill トラッカー (v15.05+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/operations/oom-tracker` | コンテナの OOMKill イベントとメモリ設定の分析を追跡 |

**クエリパラメータ：** `namespace`（オプション）

**検出カテゴリ：**

| チェック項目 | リスクスコア | 説明 |
|---------|--------|------|
| OOMKilled コンテナ | +15/個 | メモリ不足でキルされた |
| 高再起動回数 (>=10) | +20 | CrashLoop の指標 |
| 高再起動回数 (>=5) | +10 | 頻繁な再起動 |
| メモリ制限なし | +5 | OOM の挙動が予測不能 |
| 低メモリ制限 (<256MB) | — | 不要な OOM を引き起こす可能性 |
| 制限>>リクエスト (10x+) | — | ノードメモリ圧力リスク |

**返却内容：** Pod ごとの OOM リスクレベル、上位 OOM ランキング、ネームスペースごとの統計、クラスター OOM リスクスコア (0-100)

### ストレージ容量枯渇予測器 (v15.06+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/scalability/storage-forecast` | ストレージ容量がいつ枯渇するかを予測 |

PV 使用量のトレンドと成長率の見積もりに基づき、ストレージ容量の枯渇時期を予測します。

**分析次元：**

| 指標 | 説明 |
|------|------|
| 容量 vs 使用済み | Longhorn actual-size アノテーションをサポートし、実際の使用量を取得 |
| 日次成長率 | 使用率と PV 経過日数に基づくヒューリスティック推定 |
| 枯渇までの日数 | 残り容量 / 日次成長率 |
| 予測枯渇日 | 日付または ">10年" または "成長なし" |
| リスクレベル | critical(>95%) / high(>85%または<14d) / medium(<30d) / low |

**返却内容：** PV ごとの予測、クラスターの容量枯渇までの日数見積もり、StorageClass ごとの統計、高リスクネームスペースランキング、ストレージ健全性スコア (0-100)

### DNS 解決健全性チェッカー (v15.08+)

| Method | Path | 説明 |
|--------|------|-------------|
| GET | `/api/product/dns-health` | クラスターの DNS 解決健全性状態を分析 |

**CoreDNS 分析：**

| チェック項目 | 説明 |
|---------|------|
| Pod の健全性 | running/ready/restarts/version per pod |
| Corefile | forwarders、plugins、missing Corefile の検出 |
| レプリカ数 | 高可用性のために >= 2 を推奨 |

**その他の検出：**
- Headless Service エンドポイントカバレッジ (NXDOMAIN リスク)
- NodeLocal DNS キャッシュの検出
- Pod dnsConfig ndots カバレッジの検出 (>5 = 過剰な DNS クエリ)
- External-DNS によるマネージドサービスディスカバリ

**返却内容：** CoreDNS Pod ステータス、Headless Service カバレッジ、DNS 設定分析、クラスター DNS 健全性スコア (0-100)、実行可能な推奨
