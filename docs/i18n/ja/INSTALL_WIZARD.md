# k8ops インストールウィザードガイド

インタラクティブなインストールウィザード（`wizard.sh`）は、デプロイ前に k8ops の主要コンポーネントをすべて設定する手順をガイドします：データベースバックエンド、SSO 統合、AI プロバイダー。

## クイックスタート

### インタラクティブモード

```bash
git clone https://github.com/topcheer/k8ops.git
cd k8ops
./wizard.sh
```

### 非インタラクティブモード

```bash
# config/wizard-values.yaml に設定を編集してから：
./wizard.sh --values config/wizard-values.yaml
```

### ドライラン（マニフェストのみ生成）

```bash
./wizard.sh --dry-run
# 生成されたファイルを確認: .wizard-*.yaml
# kubectl apply -f ... で手動デプロイ
```

## ウィザードのステップ

### ステップ 1: デプロイメントモード

| モード | 説明 | 最適な用途 |
|------|-------------|----------|
| **DaemonSet** | 全ノードで実行 | K3s/ベアメタルクラスター、ノードレベルの監視 |
| **Deployment** | PVC 付き単一レプリカ | マネージド K8s（EKS/GKE/AKS）、コスト重視の構成 |

### ステップ 2: データベースバックエンド

k8ops はユーザーアカウント、ロール、認証プロバイダーにデータベースを使用します。

| バックエンド | ユースケース | HA | セットアップ |
|---------|----------|----|-------|
| **SQLite** | 小規模クラスター、単一ノード | なし | ゼロ設定（組み込み） |
| **MySQL** | マルチレプリカ、認証の共有 | あり | 内部 StatefulSet または外部接続 |
| **PostgreSQL** | マルチレプリカ、認証の共有 | あり | 内部 StatefulSet または外部接続 |

#### 内部データベース vs 外部データベース

- **内部**: ウィザードは `k8ops-system` ネームスペースに PVC 付きの MySQL/PostgreSQL StatefulSet をデプロイします。完全に管理されており、外部の依存関係はありません。
- **外部**: 既存のデータベースに接続します。DSN 接続文字列を指定します。

#### DSN 形式

**MySQL:**
```
k8ops:password@tcp(mysql-host:3306)/k8ops?charset=utf8mb4&parseTime=True
```

**PostgreSQL:**
```
host=postgres-host user=k8ops password=secret dbname=k8ops sslmode=disable
```

### ステップ 3: SSO / アイデンティティプロバイダー

k8ops は組み込みプリセットで複数の SSO プロバイダーをサポートしています：

| プロバイダー | タイプ | プリセット |
|----------|------|--------|
| **GitHub** | OIDC | 事前設定済みの issuer |
| **Google** | OIDC | 事前設定済みの issuer |
| **Microsoft** (Entra ID) | OIDC | 事前設定済みの issuer |
| **GitLab** | OIDC | 事前設定済みの issuer |
| **Keycloak** | OIDC | カスタム issuer（あなたのレルム） |
| **Okta** | OIDC | カスタム issuer |
| **Auth0** | OIDC | カスタム issuer |
| **LDAP / AD** | LDAP | サーバー + バインド DN |
| **カスタム OIDC** | OIDC | 手動の issuer URL |

#### OIDC リダイレクト URL

アイデンティティプロバイダーにアプリケーションを登録する際、以下のリダイレクト URL を使用してください：

```
https://<your-dashboard-host>/api/auth/oidc/<provider-name>/callback
```

GitHub の例：
```
https://k8ops.example.com/api/auth/oidc/github/callback
```

#### LDAP 設定

以下を指定します：
- **サーバー URL**: `ldap://host:389` または `ldaps://host:636`
- **検索ベース**: 例 `ou=users,dc=example,dc=com`
- **バインド DN**: サービスアカウント DN、例 `cn=admin,dc=example,dc=com`
- **バインドパスワード**: サービスアカウントのパスワード

SSO はインストール時にスキップでき、後からダッシュボードの **Settings > Auth Providers** で設定できます。

### ステップ 4: AI プロバイダー

| プロバイダー | モデル | 備考 |
|----------|--------|-------|
| **OpenAI** | gpt-4o, gpt-4o-mini | デフォルト |
| **Anthropic** | claude-sonnet-4-20250514 | Claude ファミリー |
| **Gemini** | gemini-1.5-flash | Google AI |
| **カスタム** | 任意 | OpenAI 互換エンドポイント |

AI プロバイダーはインストール後、ダッシュボードの **Settings** で設定できます。

### ステップ 5: 確認とデプロイ

ウィザードはすべての選択内容のサマリーを表示します。確認後、以下を実行します：

1. Kubernetes マニフェストの生成（シークレット、オプションの DB StatefulSet）
2. クラスターへの適用
3. k8ops のデプロイ（DaemonSet または Deployment）
4. Pod の準備完了を待機
5. アクセス URL とログイン認証情報の表示

## インストール後

### デフォルトログイン

- ユーザー名: `admin`
- パスワード: `admin`
- **初回ログイン後すぐに変更してください**

### インストール後の SSO 設定

インストール時に SSO をスキップした場合：

1. **Settings > Auth Providers** に移動
2. **Add Provider** をクリック
3. プリセットを選択（GitHub、Google など）
4. Client ID と Client Secret を入力
5. 保存して有効化

### 環境変数リファレンス

ウィザードは以下の環境変数を設定します（手動設定も可能）：

| 変数 | 説明 | デフォルト |
|----------|-------------|---------|
| `AUTH_DB_DRIVER` | データベースドライバ | `sqlite` |
| `AUTH_DB_DSN` | データベース接続文字列 | (空) |
| `AUTH_DB_PATH` | SQLite ファイルパス | `/data/k8ops.db` |
| `AUTH_JWT_SECRET` | JWT 署名シークレット | (自動生成) |
| `AUTH_DEFAULT_ROLE` | SSO ユーザーのデフォルトロール | `viewer` |
| `AIOPS_API_KEY` | AI プロバイダー API キー | (空) |

## CLI フラグ

```bash
./manager \
  --auth-db-driver=postgres \
  --auth-db-dsn="host=localhost user=k8ops password=secret dbname=k8ops sslmode=disable" \
  --auth-jwt-secret=my-secret \
  --provider-type=openai \
  --provider-model=gpt-4o \
  --provider-api-key=sk-... \
  --dashboard-address=:9090
```

## トラブルシューティング

### SQLite "out of memory" エラー

これは SQLite データベースパスが書き込み可能でない場合（例: 読み取り専用のコンテナファイルシステム）に発生します。
`/data` が `emptyDir` または PVC ボリュームでバックアップされていることを確認してください。

### MySQL/PostgreSQL 接続失敗

1. DSN 形式がデータベースタイプと一致していることを確認
2. k8ops Pod からデータベースへのネットワーク接続を確認
3. データベースユーザーに CREATE/ALTER 権限があることを確認（自動マイグレーション用）

### SSO リダイレクトが機能しない

1. リダイレクト URL が正確に一致していることを確認（末尾のスラッシュを含む）
2. HTTPS が正しく設定されていることを確認（OIDC には HTTPS が必要）
3. アイデンティティプロバイダーに正しいリダイレクト URL が登録されていることを確認
