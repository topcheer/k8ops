# k8ops ローカル実行ガイド

> Kubernetes クラスターへのデプロイなしに、ラップトップ/ワークステーションで k8ops バイナリを直接実行します。

---

## 適用シナリオ

- **ローカル開発デバッグ** — コードの迅速な反復、毎回のイメージビルドが不要
- **オフライン管理ツール** — スマートな kubectl の代替として
- **デモとトライアル** — クラスター内デプロイなしで全機能を体験
- **CI/CD 統合** — パイプラインで診断ツールとして実行

---

## 前提条件

- Go 1.26+（またはプレビルド済みバイナリの直接ダウンロード）
- kubectl が設定済みでクラスターに接続可能
- LLM API Key（OpenAI / DeepSeek / ZAI など）

---

## 方法 1: ソースからビルド

```bash
cd k8ops

# manager（dashboard サーバー）をビルド
go build -o k8ops-manager ./cmd/manager/

# CLI ツールをビルド
go build -o k8ops ./cmd/k8ops/
```

## 方法 2: プレビルド済みバイナリのダウンロード

[GitHub Releases](https://github.com/topcheer/k8ops/releases) から対応プラットフォームのバイナリをダウンロードします。

---

## Dashboard の起動

```bash
AIOPS_API_KEY=your-api-key \
  ./k8ops-manager \
  --leader-elect=false \
  --dashboard-address=:9090 \
  --auth-db-path=/tmp/k8ops.db
```

起動後 `http://localhost:9090` にアクセスし、デフォルトログイン `admin / admin` を使用します。

### パラメータ説明

| パラメータ | デフォルト値 | 説明 |
|------|--------|------|
| `--dashboard-address` | `:9090` | Dashboard リッスンアドレス |
| `--leader-elect` | `false` | Leader Election（単一インスタンス実行時は無効化） |
| `--metrics-bind-address` | `:8080` | Prometheus metrics ポート |
| `--health-probe-bind-address` | `:8081` | ヘルスチェックポート |
| `--auth-db-path` | `/data/k8ops.db` | SQLite データベースパス |
| `--auth-jwt-secret` | (ランダム生成) | JWT 署名シークレット |
| `--provider-type` | `openai` | LLM プロバイダー |
| `--provider-model` | `gpt-4o` | モデル名 |
| `--provider-api-key` | (必須) | LLM API Key |
| `--provider-endpoint` | (デフォルト) | カスタム API エンドポイント |

### 環境変数

すべてのパラメータは環境変数でも設定可能です：

```bash
export AIOPS_API_KEY=sk-your-key
export PROVIDER_TYPE=deepseek
export PROVIDER_MODEL=deepseek-chat
export AUTH_DB_PATH=$HOME/.k8ops/k8ops.db
export AUTH_JWT_SECRET=your-secret

./k8ops-manager --leader-elect=false
```

---

## kubeconfig 発見メカニズム

k8ops は controller-runtime の `ctrl.GetConfigOrDie()` を使用して kubeconfig を自動発見します。検索順序：

1. `KUBECONFIG` 環境変数
2. `~/.kube/config`（デフォルトパス）
3. In-cluster config（`/var/run/secrets/kubernetes.io/serviceaccount/`）

ローカル実行時は自動的に `~/.kube/config` を使用し、追加設定は不要です。

### クラスターの指定

```bash
KUBECONFIG=~/.kube/prod-config ./k8ops-manager --leader-elect=false
```

### マルチクラスター切り替え

```bash
# kubectx で切り替え
kubectx prod-cluster
./k8ops-manager --leader-elect=false
```

---

## データフローの違い

### クラスター内実行 vs ローカル実行

| 次元 | クラスター内 (DaemonSet/Deployment) | ローカル実行 |
|------|------|------|
| K8s API 認証 | ServiceAccount token | kubeconfig |
| Host ツール | `nsenter` でホストにアクセス | ローカルマシンで直接実行 |
| 認証データ | PVC で永続化 | ローカル SQLite ファイル |
| Leader Election | マルチレプリカで必要 | 単一インスタンスで無効 |
| RBAC 偽装 | ユーザー → ServiceAccount | ユーザー → kubeconfig ユーザー |
| ネットワーク権限 | Pod ネットワーク | ローカルネットワーク |
| ログ出力 | stdout → kubectl logs | 端末に直接出力 |

### Host ツールの動作

コンテナ内では、Host ツールは `nsenter -m -u -i -n -p --` でホストのネームスペースにアクセスします。ローカル実行時は `/bin/sh -c` で直接実行し、ローカル OS にアクセスします。

これは以下を意味します：
- `host_disk_check` はローカルディスクをチェック
- `host_process_list` はローカルプロセスをリスト
- `host_exec` はローカルでコマンドを実行

---

## CLI ツールの使用

```bash
# 診断
./k8ops diagnose --problem "pods in production keep CrashLoopBackOff"

# 最適化提案の確認
./k8ops optimize --namespace production

# 修復のトリガー
./k8ops remediate --plan <plan-name> --approve
```

---

## バックグラウンド常駐実行

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

## 開発モード

### ホットリロード

```bash
# air をインストール
go install github.com/air-verse/air@latest

# k8ops プロジェクトルートで
air --build.cmd "go build ./cmd/manager/" --build.bin "./manager"
```

### デバッグ

```bash
# DEBUG ログを有効化
DEBUG=true ./k8ops-manager --leader-elect=false

# JSON 構造化ログの確認
tail -f /tmp/k8ops.log
```

---

## トラブルシューティング

### "unable to get kubeconfig"

`~/.kube/config` が存在し、有効であることを確認してください：
```bash
kubectl cluster-info  # kubeconfig のテスト
```

### "address already in use :9090"

```bash
# 9090 を占有しているプロセスを確認
lsof -i :9090
# または別のポートを使用
./k8ops-manager --dashboard-address=:9091
```

### Auth DB のロック

DB ファイルを削除して再初期化：
```bash
rm /tmp/k8ops.db
./k8ops-manager --auth-db-path=/tmp/k8ops.db
```

### プロバイダータイムアウト

より長いタイムアウトを設定するか、ネットワークを確認：
```bash
export PROVIDER_ENDPOINT=https://api.openai.com/v1
# ネットワーク到達性の確認
curl -I https://api.openai.com/v1/models
```
