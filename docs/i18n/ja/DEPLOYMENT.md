# k8ops デプロイメントガイド

## ワンクリックインストールとアンインストール

### 前提条件

- Kubernetes 1.24+（k3s / k8s / EKS / GKE / AKS いずれも可）
- kubectl が設定済みでクラスターに接続可能
- ローカルまたはリモートのコンテナイメージレジストリ（デフォルトでは `registry.iot2.win` を使用）
- オプション: LLM API Key（OpenAI / DeepSeek / ZAI などの互換インターフェース）

---

## 方法 1: Deployment モード（推奨）

単一レプリカの Deployment で、ほとんどのシナリオに適しています。Ingress、Service、ConfigMap、Secret、RBAC を含め、1 コマンドですべてのデプロイを完了します。

### インストール

```bash
# ローカルネットワーク（ドメイン、イメージ、CORS など全設定済み）
kubectl apply -k config/deploy/overlays/local

# またはカスタム overlay
cp -r config/deploy/overlays/local config/deploy/overlays/myorg
# myorg/kustomization.yaml を編集：イメージアドレス、ドメイン、CORS などを置換
kubectl apply -k config/deploy/overlays/myorg
```

### 検証

```bash
# Pod ステータスの確認
kubectl get pods -n k8ops-system

# Ingress の確認
kubectl get ingress -n k8ops-system

# Dashboard にアクセス
# ブラウザで https://<あなたのドメイン> を開く  (例: https://k8ops.iot2.win)
# デフォルトログイン: admin / admin（初回ログイン時にパスワード変更を促されます）
```

### アンインストール

```bash
kubectl delete -k config/deploy/overlays/local
```

---

## 方法 2: DaemonSet モード

各ノードで 1 つの Pod を実行し、ノードレベルの診断（hostPID、hostPath）をサポートします。深いノード監視が必要なシナリオに適しています。

### インストール

```bash
kubectl apply -f config/daemonset-local.yaml
```

### 検証

```bash
# DaemonSet の確認（各ノードに 1 つの Pod）
kubectl get ds -n k8ops-system
kubectl get pods -n k8ops-system -o wide

# Dashboard へのアクセス（Service ClusterIP または Ingress 経由）
kubectl get svc k8ops-dashboard -n k8ops-system
```

### アンインストール

```bash
kubectl delete -f config/daemonset-local.yaml
```

---

## 方法 3: install.sh スクリプト

```bash
# インストール（環境を自動検出し、Deployment / DaemonSet をインタラクティブに選択）
./install.sh install

# アンインストール
./install.sh uninstall

# ステータスの確認
./install.sh status
```

---

## イメージのビルドとプッシュ

```bash
# ローカルビルド（amd64、クラスターノード向け）
docker buildx build --platform linux/amd64 \
  --build-arg VERSION=v1.0.0 \
  -t registry.iot2.win/k8ops:v1.0.0 \
  -t registry.iot2.win/k8ops:latest \
  --load .

# レジストリにプッシュ
docker push registry.iot2.win/k8ops:v1.0.0
docker push registry.iot2.win/k8ops:latest
```

### マルチアーキテクチャビルド

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  --build-arg VERSION=v1.0.0 \
  -t registry.iot2.win/k8ops:v1.0.0 \
  --push .
```

---

## LLM プロバイダー設定

### 方法 1: Dashboard インターフェースでの設定（推奨）

1. Dashboard にログイン → **Settings** タブ
2. プロバイダータイプ、API Key、Endpoint、Model を入力
3. **Save** をクリック、K8s ConfigMap/Secret に自動的に永続化

### 方法 2: 環境変数

overlay の ConfigMap で設定：

```yaml
configMapGenerator:
- name: k8ops-config
  literals:
  - PROVIDER_TYPE=openai          # openai / deepseek / zai / anthropic
  - PROVIDER_MODEL=gpt-4o
  - PROVIDER_ENDPOINT=https://api.openai.com/v1
```

API Key は Secret 経由：

```yaml
secretGenerator:
- name: k8ops-credentials
  literals:
  - api-key=sk-your-api-key-here
```

### サポートされるプロバイダー

| プロバイダー | Endpoint | モデル例 |
|----------|----------|------------|
| OpenAI | `https://api.openai.com/v1` | gpt-4o, gpt-4o-mini |
| DeepSeek | `https://api.deepseek.com/v1` | deepseek-chat |
| ZAI (智谱) | `https://open.bigmodel.cn/api/paas/v4` | glm-4-flash, glm-4-plus |
| Anthropic | `https://api.anthropic.com/v1` | claude-3-5-sonnet |
| ローカル | `http://localhost:11434/v1` | llama3, qwen2 |

---

## 認証設定

### ローカル認証（デフォルト）

すぐに使用可能、ユーザーは SQLite に保存。初回ログイン：`admin / admin`。

### LDAP

```yaml
# ConfigMap またはプロバイダー設定で設定
LDAP_SERVER=ldap://your-ldap:389
LDAP_BIND_DN=cn=admin,dc=example,dc=com
LDAP_BIND_PASSWORD=secret
LDAP_USER_BASE=ou=users,dc=example,dc=com
LDAP_SKIP_TLS_VERIFY=false   # 本番環境では必ず false に
```

### OIDC（GitHub / Google / Keycloak など）

```yaml
# プロバイダー設定（Dashboard Settings ページまたは CRD）
OIDC_ISSUER=https://your-keycloak/realms/myrealm
OIDC_CLIENT_ID=k8ops
OIDC_CLIENT_SECRET=your-secret
OIDC_REDIRECT_URL=https://k8ops.iot2.win/auth/oidc/callback
```

---

## Ingress と TLS

### 自動 TLS（cert-manager + Let's Encrypt）

クラスターに cert-manager がインストールされていることを確認し、Ingress に annotation を追加：

```yaml
annotations:
  cert-manager.io/cluster-issuer: letsencrypt-prod
```

### 既存の TLS 証明書の使用

```bash
kubectl create secret tls k8ops-dashboard-tls \
  --cert=fullchain.pem \
  --key=privkey.pem \
  -n k8ops-system
```

---

## FAQ

### Pod が Pending のまま

```bash
# スケジュール失敗の原因を確認
kubectl describe pod <pod-name> -n k8ops-system | tail -10

# 一般的な原因：
# - hostNetwork ポート競合 → hostNetwork: true を削除またはポート宣言の競合を回避
# - リソース不足 → resources.requests/limits を調整
# - ノード taint → tolerations を確認
```

### Dashboard が 502 を返す

```bash
# 1. Pod が Ready か確認
kubectl get pods -n k8ops-system

# 2. Service エンドポイントを確認
kubectl get endpoints k8ops-dashboard -n k8ops-system

# 3. Ingress バックエンドを確認
kubectl describe ingress -n k8ops-system

# 4. Pod が完全に準備完了してから再試行
```

### イメージのプル失敗

```bash
# 方法 1: imagePullPolicy: Always を設定（具体的なタグ使用時に推奨）
# 方法 2: ノードがレジストリの TLS 信頼を設定済みか確認
# 方法 3: プライベートレジストリの場合、imagePullSecrets を作成
```

### LLM API 401

```bash
# API Key が正しく設定されているか確認
kubectl get secret k8ops-credentials -n k8ops-system -o jsonpath='{.data.api-key}' | base64 -d

# または Dashboard → Settings でプロバイダーを再設定
```

---

## アップグレード

```bash
# 新しいイメージをビルドしてプッシュ
docker buildx build --platform linux/amd64 \
  --build-arg VERSION=v2.0.0 \
  -t registry.iot2.win/k8ops:v2.0.0 \
  -t registry.iot2.win/k8ops:latest \
  --push .

# ローリングアップデート（Deployment モード）
kubectl set image deployment/k8ops \
  k8ops=registry.iot2.win/k8ops:v2.0.0 \
  -n k8ops-system

# または overlay の newTag を変更して再 apply
kubectl apply -k config/deploy/overlays/local

# DaemonSet モード
kubectl set image daemonset/k8ops \
  k8ops=registry.iot2.win/k8ops:v2.0.0 \
  -n k8ops-system
```

---

## データのバックアップとリストア

### SQLite 自動バックアップ（CronJob）

k8ops は SQLite でユーザー、監査ログ、セッションデータを保存します。毎時の自動バックアップを推奨：

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: k8ops-backup
  namespace: k8ops-system
spec:
  schedule: "0 * * * *"  # 毎時 0 分
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: backup
            image: busybox
            command:
            - sh
            - -c
            - |
              TIMESTAMP=$(date +%Y%m%d-%H%M%S)
              cp /data/k8ops.db /backup/k8ops-${TIMESTAMP}.db
              # 直近 24 個のバックアップを保持
              ls -t /backup/k8ops-*.db | tail -n +25 | xargs rm -f
            volumeMounts:
            - name: data
              mountPath: /data
              readOnly: true
            - name: backup
              mountPath: /backup
          volumes:
          - name: data
            persistentVolumeClaim:
              claimName: k8ops-data
          - name: backup
            hostPath:
              path: /var/lib/k8ops-backup
              type: DirectoryOrCreate
          restartPolicy: OnFailure
```

### 手動バックアップ

```bash
# Pod からデータベースをコピー
kubectl cp k8ops-system/<pod-name>:/data/k8ops.db ./k8ops-backup-$(date +%Y%m%d).db

# または sqlite3 オンラインバックアップ（書き込みを止めない）
kubectl exec -n k8ops-system <pod-name> -- sqlite3 /data/k8ops.db ".backup /data/k8ops-backup.db"
kubectl cp k8ops-system/<pod-name>:/data/k8ops-backup.db ./k8ops-backup.db
```

### リストア

```bash
# k8ops を停止
kubectl scale deployment k8ops -n k8ops-system --replicas=0

# データベースをリストア
kubectl cp ./k8ops-backup.db k8ops-system/<pod-name>:/data/k8ops.db

# 再起動
kubectl scale deployment k8ops -n k8ops-system --replicas=1
```

---

## 高可用性 (HA) デプロイメント

### 単一ノードモード（デフォルト、開発/小規模クラスター向け）

- 1 replica + SQLite + PVC
- Pod 再起動時にサービスが短時間中断（~10s）
- 50 ユーザー未満のチームに適しています

### マルチレプリカ HA（本番推奨）

SQLite の代わりに MySQL/PostgreSQL を使用し、マルチレプリカをサポート：

1. **データベースを MySQL に切り替え**：

```yaml
# overlay ConfigMap で設定
configMapGenerator:
- name: k8ops-config
  literals:
  - DB_DRIVER=mysql
  - DB_DSN=k8ops:password@tcp(mysql:3306)/k8ops?charset=utf8mb4&parseTime=True
```

2. **マルチレプリカ + leader election**：

```yaml
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: k8ops
        env:
        - name: LEADER_ELECT
          value: "true"
```

3. **共有ストレージ**: MySQL は独立した PVC を使用、k8ops Pod はステートレス

### キャパシティプランニング

| 規模 | ユーザー数 | リソース推奨 | データベース |
|------|--------|----------|--------|
| 小規模 | < 20 | 1 pod, 500m CPU / 512Mi | SQLite |
| 中規模 | 20-100 | 2 pods, 1 CPU / 1Gi each | MySQL |
| 大規模 | 100+ | 3+ pods, 2 CPU / 2Gi each | MySQL + 読み書き分離 |

---

## CI/CD パイプラインとリリース管理

### ワンクリックデプロイスクリプト

k8ops はプレチェック、ビルド、リリース、ヘルスチェック、自動ロールバックを含む自動デプロイスクリプトを提供します：

```bash
# 新バージョンのデプロイ（自動プレチェック + ビルド + リリース + ヘルスチェック）
./scripts/deploy.sh v14.36

# デプロイフロー：
# 1. プレチェック：go build + go vet + go test + gofmt
# 2. ビルド：Docker buildx + registry にプッシュ
# 3. リリース：kubectl set image + change-cause annotation
# 4. 検証：Pod Ready + HTTP 200（120s タイムアウト）
# 5. ロールバック：ヘルスチェック失敗時に前バージョンへ自動ロールバック
```

### クイックロールバック

```bash
# 前バージョンへロールバック
./scripts/rollback.sh

# 特定 revision へロールバック
./scripts/rollback.sh 58

# 特定バージョン番号へロールバック
./scripts/rollback.sh v14.30
```

### リリース履歴の追跡

デプロイごとに change-cause annotation が自動的に記録されます：

```bash
# リリース履歴の確認
kubectl rollout history daemonset/k8ops -n k8ops-system

# 特定 revision の詳細を確認
kubectl rollout history daemonset/k8ops -n k8ops-system --revision=55
```

### CI フロー (GitHub Actions)

| ワークフロー | トリガー条件 | 内容 |
|--------|----------|------|
| `ci.yml` — push/PR to main | コードコミット | test + vet + lint + govulncheck + Docker build |
| `release.yml` — tag v* | バージョンタグ | 全量テスト + GoReleaser + Docker multi-arch + 自動リリースノート |

### イメージ管理

| タグ | 説明 |
|------|------|
| `registry.iot2.win/k8ops:v14.XX` | 特定バージョン |
| `registry.iot2.win/k8ops:latest` | 最新安定版 |
| `ghcr.io/<org>/k8ops:v14.XX` | GHCR イメージ（CI リリース） |

### イメージ最適化

- ベースイメージ: `gcr.io/distroless/static-debian12:nonroot`（shell なし、パッケージマネージャーなし）
- マルチステージビルド: Go builder + distroless runtime
- BuildKit キャッシュ: `--mount=type=cache` で CI ビルドを高速化
- バイナリ最適化: `-trimpath -ldflags="-s -w"` でサイズを削減

| バージョン | イメージサイズ |
|------|----------|
| v14.30 (alpine) | 31.8 MB |
| v14.35 (distroless) | 28.6 MB |

### 高可用性設定

#### PodDisruptionBudget (PDB)

ノードメンテナンス中に最低 1 つの Pod が利用可能であることを保証：

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: k8ops-pdb
  namespace: k8ops-system
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: k8ops
```

#### NetworkPolicy

Dashboard が Ingress Controller からのトラフィックのみを受け入れるよう制限：

- Ingress: kube-system ネームスペースのみ 9090 (dashboard) にアクセス可能
- Ingress: monitoring ネームスペースのみ 8080 (metrics) にアクセス可能
- Egress: DNS (53)、HTTPS (443)、K8s API (6443) を許可

#### PriorityClass

k8ops は `system-cluster-critical` 優先度を使用し、リソース圧迫下でも退避されないことを保証します。

#### ローリングアップデート戦略

| モード | maxUnavailable | maxSurge | 説明 |
|------|---------------|----------|------|
| DaemonSet | 1 | - | 毎回 1 ノードずつ更新 |
| Deployment | 0 | 1 | 新 Pod を起動してから旧 Pod を削除 |

#### リソース割り当て

| モード | CPU Request | CPU Limit | Mem Request | Mem Limit |
|------|-------------|-----------|-------------|-----------|
| DaemonSet | 100m | 1 | 128Mi | 1Gi |
| Deployment | 500m | 2 | 512Mi | 2Gi |

#### ヘルスチェックとライフサイクル管理

k8ops は信頼性を保証するために 3 層のプローブを使用します：

| プローブ | パス | 役割 | パラメータ |
|------|------|------|------|
| **startupProbe** | `/healthz` | 起動完了を待機（スロースタートが liveness でキルされるのを防止） | failureThreshold: 30, period: 5s（最大 150s 待機） |
| **livenessProbe** | `/healthz` | 存続チェック（失敗すると Pod を再起動） | period: 20s, failureThreshold: 3, timeout: 5s |
| **readinessProbe** | `/readyz` | 準備チェック（失敗すると Service Endpoints から削除） | period: 10s, failureThreshold: 3, timeout: 5s |

**グレースフルシャットダウン:**

```yaml
lifecycle:
  preStop:
    exec:
      command: ["/manager", "--pre-stop"]
# --pre-stop は 5s sleep し、Ingress Controller がこの Pod をロードバランサーから削除するのを待ちます
# その後 kubelet が SIGTERM を送信し、dashboard のグレースフルシャットダウン（SSE 接続のドレイン）をトリガーします
# terminationGracePeriodSeconds: 30 は完了するのに十分な時間を保証します
```

シャットダウンフロー：
1. kubelet が `preStop` を実行 → sleep 5s（接続ドレイン）
2. kubelet が SIGTERM を送信 → Go シグナルハンドラーがグレースフルシャットダウンを開始
3. Dashboard HTTP サーバーが新規リクエストの受け付けを停止
4. SSE 接続のドレイン（10s タイムアウト）
5. Controller Manager のグレースフルシャットダウン
6. プロセスの終了
