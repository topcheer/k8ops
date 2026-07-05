# k8ops — Kubernetes AI Operations Operator

<div align="center">

**問題の診断、自動修復、AI によるクラスター最適化を実現する Kubernetes AIOps オペレータ。**

[![GitHub release](https://img.shields.io/github/v/release/topcheer/k8ops?style=flat-square)](https://github.com/topcheer/k8ops/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/topcheer/k8ops/ci.yml?branch=main&style=flat-square&label=CI)](https://github.com/topcheer/k8ops/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/topcheer/k8ops?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![Docker](https://img.shields.io/badge/Docker-ghcr.io-2496ED?style=flat-square&logo=docker)](https://github.com/topcheer/k8ops/pkgs/container/k8ops)
[![Built with ggcode](https://img.shields.io/badge/Built%20with-ggcode-6C43BC?style=flat-square)](https://github.com/topcheer/ggcode)

</div>

---

**言語：** [English](../../README.md) | [中文](../zh-CN/README.md) | [日本語](README.md) | [한국어](../ko/README.md) | [Español](../es/README.md) | [Français](../fr/README.md) | [Deutsch](../de/README.md)

---

## 機能

### AI 駆動オペレーション
- **インテリジェント診断** — 問題の説明を送信すると、ツール拡張推論（kubectl describe、logs、events、metrics）による AI 主導の根本原因分析を取得
- **自動修復** — AI が安全な修復アクション（Pod の再起動、Deployment のスケール、リソースのクリーンアップ）を提案し、承認後に実行
- **最適化の提案** — リソース使用量、HPA/PDB のギャップ、コスト削減機会の継続的分析
- **ストリーミングチャット** — 思考ブロック、ツール呼び出しの透明性、差分ベースの結果レンダリングを備えたリアルタイム SSE ストリーミング

### エンタープライズセキュリティ
- **マルチプロバイダー認証** — ローカル（bcrypt）、LDAP（TLS 検証設定可能）、OIDC（GitHub、Google、GitLab、Keycloak、Okta、Auth0、Microsoft）
- **RBAC** — admin/operator/viewer ロールと namespace スコープの権限によるロールベースアクセス制御
- **OIDC CSRF 保護** — `ConstantTimeCompare` 検証によるプロバイダーごとの state Cookie
- **CORS ホワイトリスト** — オリジンベースのホワイトリスト（認証付きワイルドカードなし）、`Vary: Origin` ヘッダー
- **監査ログ** — すべての AI アクション、ツール実行、LLM 呼び出しが構造化された監査イベントとして記録
- **JWT 永続化** — 署名済み JWT シークレットは K8s Secret に保存（オプションのフォールバック付き）
- **レート制限** — ブルートフォース攻撃を防ぐためのログインエンドポイントでのトークンバケットレートリミッタ
- **セキュリティヘッダー** — X-Content-Type-Options、X-Frame-Options、HSTS、CSP

### 運用と信頼性
- **グレースフルシャットダウン** — SIGTERM/SIGINT ハンドリング、SSE ドレイン、SQLite WAL フラッシュ、コントローラー停止
- **会話 TTL** — アイドル状態のチャットセッションの自動クリーンアップ（30 分タイムアウト、最大 1000 会話）
- **サーキットブレーカー** — 設定可能なリトライ、バックオフ、サーキットブレーキングによる耐障害性のある LLM 呼び出し
- **Prometheus メトリクス** — クラスター健全性ゲージ、会話カウンター、ツール実行メトリクス

### デプロイメント
- **Kustomize** — 本番対応のデフォルト設定を備えたベース + オーバーレイ Deployment
- **組み込み Web UI** — 単一バイナリ、外部フロントエンド依存なし
- **SQLite + K8s CRD** — 軽量な永続化、外部データベース不要
- **PVC 永続化** — データは Pod 再起動後も保持

---

## アーキテクチャ

```
┌─────────────────────────────────────────────────────────┐
│                    Dashboard / Web UI                     │
│  (Embedded SPA + REST API + SSE streaming)               │
├─────────────────────────────────────────────────────────┤
│            Auth (Local/LDAP/OIDC) + RBAC                 │
├─────────────────────────────────────────────────────────┤
│                      AI Agent                            │
│  (LLM reasoning + tool calling + streaming)              │
├──────────┬──────────┬──────────┬────────────────────────┤
│  Chat    │  Safety  │  Audit   │  Resilience            │
│  Engine  │  Checker │  Logger  │  (Circuit Breaker)     │
├──────────┴──────────┴──────────┴────────────────────────┤
│                    Tool Registry                         │
│  (kubectl get/describe/logs, exec, events, metrics)      │
├─────────────────────────────────────────────────────────┤
│              Controller Runtime + CRDs                   │
│  (DiagnosticReport, RemediationPlan, OptimizationSuggestion) │
├─────────────────────────────────────────────────────────┤
│                   Kubernetes API                         │
│  (Impersonation: user-scoped RBAC)                       │
└─────────────────────────────────────────────────────────┘
```

コンポーネントの詳細については[docs/ARCHITECTURE.md](../../docs/ARCHITECTURE.md)を参照してください。

---

## クイックスタート

### 前提条件
- Kubernetes 1.24+（k3s / k8s / EKS / GKE / AKS）
- kubectl が設定済みであること
- LLM API キー（OpenAI、DeepSeek、ZAI、または任意の OpenAI 互換プロバイダー）

### 1. Kubernetes へのデプロイ

**オプション A: Deployment モード（推奨）**

```bash
# コマンド 1 つ — namespace、RBAC、Secret、Ingress、TLS を含む
kubectl apply -k config/deploy/overlays/local

# または独自のオーバーレイを作成
cp -r config/deploy/overlays/local config/deploy/overlays/myorg
# myorg/kustomization.yaml を編集: ドメイン、レジストリ、CORS を設定
kubectl apply -k config/deploy/overlays/myorg
```

**オプション B: DaemonSet モード（ノードごとの診断）**

```bash
kubectl apply -f config/daemonset-local.yaml
```

**オプション C: install.sh（対話式）**

```bash
./install.sh install    # デプロイ
./install.sh status     # ステータス確認
./install.sh uninstall  # 削除
```

デプロイの詳細については [docs/DEPLOYMENT.md](../../docs/DEPLOYMENT.md) を参照してください。

### 2. LLM プロバイダーの設定

```bash
# ダッシュボードから: Settings タブ → プロバイダータイプ、API キー、モデルを入力
# またはオーバーレイ ConfigMap の環境変数で設定:

configMapGenerator:
- name: k8ops-config
  literals:
  - PROVIDER_TYPE=openai
  - PROVIDER_MODEL=gpt-4o
  - PROVIDER_ENDPOINT=https://api.openai.com/v1

# API キーは Secret 経由:
secretGenerator:
- name: k8ops-credentials
  literals:
  - api-key=sk-your-key-here
```

### 3. ダッシュボードへのアクセス

```bash
# Ingress 経由（設定済みの場合）
# https://<your-domain> を開く（例: https://k8ops.iot2.win）

# またはポートフォワード
kubectl port-forward svc/k8ops-dashboard 9090:9090 -n k8ops-system
# http://localhost:9090 を開く
# デフォルトログイン: admin / admin（パスワード変更を促されます）
```

### 4. 診断の実行

```bash
# kubectl 経由（CRD）
kubectl apply -f examples/diagnostic.yaml

# CLI 経由
go run ./cmd/k8ops diagnose --problem "pods in production keep CrashLoopBackOff"

# Web ダッシュボードのチャットインターフェース経由
```

---

## 設定

すべての設定は ConfigMap/Secret 経由（Kustomize オーバーレイで管理）で行います。動作例は [config/deploy/overlays/local/kustomization.yaml](../../config/deploy/overlays/local/kustomization.yaml) を参照してください。

### コア
| 変数 | デフォルト | 説明 |
|----------|---------|-------------|
| `PROVIDER_TYPE` | `openai` | LLM プロバイダータイプ |
| `PROVIDER_MODEL` | `gpt-4o` | モデル名 |
| `PROVIDER_ENDPOINT` | `https://api.openai.com/v1` | LLM プロバイダーのベース URL |
| `AIOPS_API_KEY` | (必須) | LLM API キー（Secret から取得） |

### セキュリティ
| 変数 | デフォルト | 説明 |
|----------|---------|-------------|
| `AUTH_JWT_SECRET` | (自動生成) | JWT 署名シークレット（K8s Secret に永続化） |
| `CORS_ALLOWED_ORIGINS` | (空) | カンマ区切りの許可オリジン |
| `LDAP_SERVER` | (空) | LDAP サーバー URL |
| `LDAP_SKIP_TLS_VERIFY` | `false` | LDAP TLS 証明書検証をスキップ |
| `OIDC_ISSUER` | (空) | OIDC issuer URL |

### 通知
| 変数 | デフォルト | 説明 |
|----------|---------|-------------|
| `SLACK_WEBHOOK_URL` | (空) | 通知用 Slack Incoming Webhook |

### AI / チャット
| 変数 | デフォルト | 説明 |
|----------|---------|-------------|
| `MAX_STEPS` | `15` | リクエストごとの最大エージェント推論ステップ数 |
| `CONVERSATION_TTL` | `30m` | アイドル会話のタイムアウト |
| `MAX_CONVERSATIONS` | `1000` | 最大同時会話数 |

---

## API

ダッシュボードは `http://<host>:9090/api/` で REST API を公開しています。主なエンドポイント:

| メソッド | パス | 説明 | 認証 |
|--------|------|-------------|------|
| GET | `/api/health` | ヘルスチェック | 公開 |
| GET | `/api/version` | ビルドバージョン | 公開 |
| GET | `/api/cluster/overview` | クラスターサマリー | Viewer 以上 |
| GET | `/api/cluster/nodes` | ノード一覧 + ヘルス | Viewer 以上 |
| GET | `/api/cluster/pods` | ステータス付き Pod 一覧 | Viewer 以上 |
| POST | `/api/chat/stream` | AI チャット（SSE ストリーミング） | Viewer 以上 |
| GET | `/api/resources/{type}` | K8s リソースクエリ | Viewer 以上 |
| POST | `/api/auth/login` | ローカル/LDAP ログイン | 公開 |
| GET | `/api/auth/status` | 認証設定 + プロバイダー | 公開 |
| GET | `/api/auth/providers` | 認証プロバイダー一覧 | Admin |
| GET/POST | `/api/rbac/users` | ユーザー管理 | Admin |
| GET/POST | `/api/rbac/roles` | ロール管理 | Admin |

完全な API リファレンスについては [docs/API.md](../../docs/API.md) を参照してください。

---

## 開発

### 前提条件
- Go 1.22+
- kubectl（インテグレーションテスト用）
- Kubernetes クラスターへのアクセス（コントローラーテスト用）

### ビルドとテスト

```bash
# マネージャーバイナリをビルド
make build

# すべてのテストを実行
make test

# レース検出器付きでテストを実行
go test -race -count=1 ./internal/...

# CRD を生成
make manifests

# Docker イメージをビルド
make docker-build IMG=ghcr.io/topcheer/k8ops:latest
```

### プロジェクト構成

```
k8ops/
├── api/v1alpha1/           # CRD type definitions
├── cmd/
│   ├── manager/            # Operator entry point
│   └── k8ops/              # CLI tool
├── config/
│   ├── crd/                # CRD manifests
│   ├── deploy/             # Kustomize deployment (base + overlays)
│   │   ├── base/           # Namespace, SA, RBAC, Deployment, Service, Ingress
│   │   └── overlays/
│   │       ├── local/      # Local network overlay (registry, domain, CORS)
│   │       └── prod/       # Production overlay template
│   └── daemonset/          # DaemonSet manifests (per-node deployment)
├── internal/
│   ├── agent/              # AI agent (reasoning + tool calling)
│   ├── audit/              # Structured audit logging
│   ├── auth/               # Authentication (Local/LDAP/OIDC) + RBAC
│   ├── chat/               # Chat engine with conversation management
│   ├── collector/          # Cluster event collector
│   ├── controller/         # CRD controllers (diagnostic/optimization/remediation)
│   ├── dashboard/          # Web UI + REST API
│   │   └── web/            # Embedded frontend (HTML/JS/CSS)
│   ├── memory/             # Conversation memory store
│   ├── metrics/            # Prometheus metrics
│   ├── provider/           # LLM provider interface
│   ├── providermanager/    # Multi-provider management
│   ├── resilience/         # Circuit breaker + rate limiter
│   ├── safety/             # Safety checker (dry-run validation)
│   └── tools/              # K8s and host tools (kubectl, exec, etc.)
├── docs/                   # Architecture, API, Security, Deployment docs
├── install.sh              # One-click install/uninstall script
├── .env.example            # Environment variable reference
└── examples/               # Example CRD manifests
```

開発ガイドラインについては [CONTRIBUTING.md](../../CONTRIBUTING.md) を参照してください。

---

## ローカル開発

Kubernetes デプロイなしでワークステーション上で直接 k8ops を実行できます:

```bash
# ビルド
go build -o k8ops-manager ./cmd/manager/

# 実行
AIOPS_API_KEY=your-key ./k8ops-manager \
  --leader-elect=false \
  --dashboard-address=:9090 \
  --auth-db-path=/tmp/k8ops.db
```

バイナリは kubeconfig（`~/.kube/config`）を自動的に検出するため、すべての K8s データは接続先クラスターから取得されます。詳細については [docs/LOCAL_RUN.md](../../docs/LOCAL_RUN.md) を参照してください。

---

## ドキュメント

| ドキュメント | 説明 |
|----------|-------------|
| [docs/USER_GUIDE.md](../../docs/USER_GUIDE.md) | 包括的なユーザーマニュアル（全 15 機能） |
| [docs/ARCHITECTURE.md](../../docs/ARCHITECTURE.md) | システムアーキテクチャとコンポーネント設計 |
| [docs/DEPLOYMENT.md](../../docs/DEPLOYMENT.md) | デプロイガイド（Deployment / DaemonSet / Helm） |
| [docs/LOCAL_RUN.md](../../docs/LOCAL_RUN.md) | k8ops バイナリのローカル実行（K8s デプロイ不要） |
| [docs/API.md](../../docs/API.md) | REST API リファレンス |
| [docs/SECURITY.md](../../docs/SECURITY.md) | セキュリティポリシーと RBAC モデル |
| [CHANGELOG.md](../../CHANGELOG.md) | リリース履歴（v0.1.0 → v14.1） |

---

## セキュリティ

以下を含む完全なセキュリティポリシーについては [SECURITY.md](../../SECURITY.md) を参照してください:
- 認証方式と設定
- RBAC モデルと namespace スコープ
- 脆弱性報告の対応

---

## 変更履歴

[CHANGELOG.md](../../CHANGELOG.md) を参照してください。

---

## ライセンス

GNU Affero General Public License v3.0 (AGPL-3.0)。[LICENSE](../../LICENSE) を参照してください。
