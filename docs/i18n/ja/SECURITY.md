# k8ops セキュリティ

## 認証

k8ops はデプロイメントごとに設定可能な 3 つの認証方式をサポートしています：

### ローカル認証

- SQLite に保存されるユーザー名/パスワード
- パスワードは bcrypt でハッシュ化
- `AUTH_DEFAULT_ROLE` 環境変数による管理者ブートストラップ

### LDAP / Active Directory

- 設定可能なサーバー URL、バインド DN、検索ベース
- 自己署名証明書向けの `SkipTLSVerify` オプション（デフォルト: `false`）
- マルチプロバイダーサポート: 複数の LDAP サーバーを同時に設定可能

### OIDC（OpenID Connect）

- 任意の OIDC 互換 IdP（Google、GitHub、Keycloak など）をサポート
- **CSRF 保護**: state パラメータを `crypto/subtle.ConstantTimeCompare` で検証
- **プロバイダーごとの Cookie**: `oidc_state_{provider}` によりマルチプロバイダーの衝突を防止
- **Secure フラグ**: TLS または `X-Forwarded-Proto` ヘッダーで自動検出
- **HttpOnly + SameSite**: state Cookie は JavaScript からアクセス不可

## RBAC モデル

### ロール

| ロール | スコープ | 権限 |
|------|-------|------------|
| `admin` | クラスター | フルアクセス: ユーザー、プロバイダー、全ネームスペースの管理 |
| `operator` | クラスター | 全ての読み取り + チャット + 診断の実行 |
| `viewer` | クラスター | 読み取り専用: ダッシュボード、レポートの閲覧 |
| `ns-admin` | ネームスペース | 割り当てられたネームスペース内の管理者権限 |
| `ns-viewer` | ネームスペース | 割り当てられたネームスペース内の読み取り専用 |

### ネームスペーススコープ

ネームスペーススコープのロールを持つユーザーは、以下の方法で割り当てられたネームスペースに制限されます：

1. **K8s RBAC 同期**: ネームスペースごとに `RoleBinding` リソースを作成
2. **API 偽装**: ダッシュボード API は K8s API との通信時にユーザー ID を使用
3. **ネームスペースフィルタリング**: API レスポンスは許可されたネームスペースにフィルタリング

### 組み込みロールの保護

組み込みロール（`admin`、`operator`、`viewer`）は `Builtin: true` としてマークされ、API から削除することはできません。

## セキュリティ機能

### CORS 許可リスト

- `CORS_ALLOWED_ORIGINS` 環境変数で設定（カンマ区切り）
- 認証情報が関与する場合、ワイルドカード（`*`）は使用不可
- 未設定の場合は同一オリジンのみ

### OIDC CSRF 保護

- state パラメータ: 認証試行ごとにランダムな nonce を生成
- `subtle.ConstantTimeCompare` で検証（タイミングセーフ）
- Secure + SameSite フラグ付きの HttpOnly Cookie に保存

### JWT 永続化

- JWT 署名シークレットは K8s Secret `k8ops-auth`（キー: `jwt-secret`）に永続化
- Secret が存在しない場合は一時的なランダムシークレットにフォールバック（警告ログ付き）
- Pod 再起動時のセッション無効化を防止

### 監査ログ

すべての機密操作が記録されます：

- ユーザーログイン/ログアウト
- プロバイダー設定の変更
- 診断の実行
- 修復アクション
- ユーザーロールの変更

### レート制限

- `resilience.RateLimiter` が利用可能（HTTP レイヤーには未接続 — 今後の課題）

### グレースフルシャットダウン

- `SIGTERM`/`SIGINT` → SSE 接続のドレイン → SQLite WAL のフラッシュ → manager の停止
- Pod の退避時のデータ破損を防止

## セキュリティ設定

### 環境変数

| 変数 | デフォルト | 説明 |
|----------|---------|-------------|
| `AUTH_DB_DRIVER` | `sqlite` | データベースドライバ |
| `AUTH_DB_DSN` | — | データベース接続文字列 |
| `AUTH_DB_PATH` | `/data/k8ops.db` | SQLite データベースパス |
| `AUTH_JWT_SECRET` | (ランダム) | JWT 署名シークレット（K8s Secret で永続化） |
| `AUTH_DEFAULT_ROLE` | `viewer` | 新規ユーザーのロール |
| `CORS_ALLOWED_ORIGINS` | — | カンマ区切りの許可オリジン |
| `AIOPS_API_KEY` | — | LLM プロバイダー API キー |

### K8s Secret 管理

```yaml
# JWT 永続化用の K8s Secret
apiVersion: v1
kind: Secret
metadata:
  name: k8ops-auth
  namespace: k8ops-system
type: Opaque
stringData:
  jwt-secret: "<openssl rand -base64 32>"
```

デプロイメントは以下で読み取ります：
```yaml
env:
- name: AUTH_JWT_SECRET
  valueFrom:
    secretKeyRef:
      name: k8ops-auth
      key: jwt-secret
      optional: true  # 存在しない場合はランダムにフォールバック
```

### LDAP TLS 設定

LDAP プロバイダーは `skip_tls_verify`（デフォルト: `false`）をサポートします：

```json
{
  "ldap": {
    "server": "ldaps://ldap.corp.com",
    "skip_tls_verify": false
  }
}
```

自己署名証明書を使用する開発環境でのみ `skip_tls_verify: true` を設定してください。

## 既知の制限事項

1. **ログインのレート制限なし** — `resilience.RateLimiter` は存在しますが、HTTP ハンドラーに接続されていません
2. **HTTPS 終端なし** — k8ops は HTTP を提供し、TLS は Ingress コントローラーで処理する必要があります
3. **SQLite 単一ノード** — HA データベースなし、単一レプリカデプロイメントに適しています
4. **セッション失効なし** — JWT トークンは有効期限（24h）まで有効、サーバー側の失効リストなし

## セキュリティ報告

セキュリティの脆弱性を報告する場合：

1. **公開の GitHub Issue を開かないでください**
2. 詳細と再現手順を security@ggai.dev にメールで送信してください
3. 48 時間以内に確認し、修正スケジュールを提供します
4. 責任ある開示に感謝します

## 今後のセキュリティ改善

- [ ] ログイン API へのレート制限の接続
- [ ] セッション失効（拒否リスト）の追加
- [ ] RBAC のための外部 OAuth プロバイダーのサポート
- [ ] サービス間通信の mTLS サポート
- [ ] 保存時のシークレット暗号化の実装（PVC を超えて）
