# k8ops ユーザーマニュアル

> インストールから習熟まで、すべての機能を網羅した詳細な利用ガイド。

---

## 目次

1. [クイックスタート](#1-クイックスタート)
2. [クラスターオーバービュー](#2-クラスターオーバービュー)
3. [AI Chat — インテリジェントアシスタント](#3-ai-chat--インテリジェントアシスタント)
4. [診断と修復](#4-診断と修復)
5. [最適化提案](#5-最適化提案)
6. [コスト分析 (FinOps)](#6-コスト分析-finops)
7. [クラスタートポロジーの可視化](#7-クラスタートポロジーの可視化)
8. [ノードと Pod の管理](#8-ノードと-pod-の管理)
9. [イベントストリームと通知](#9-イベントストリームと通知)
10. [リソースブラウザーと YAML エディター](#10-リソースブラウザーと-yaml-エディター)
11. [RBAC アクセス制御](#11-rbac-アクセス制御)
12. [監査ログ](#12-監査ログ)
13. [設定と構成](#13-設定と構成)
14. [キーボードショートカット](#14-キーボードショートカット)
15. [テーマ切り替え](#15-テーマ切り替え)
16. [キャパシティプランニング](#16-キャパシティプランニング)
17. [HPA の可視化](#17-hpa-の可視化)
18. [コンテナイメージインベントリ](#18-コンテナイメージインベントリ)
19. [名前空間リソースランキング](#19-名前空間リソースランキング)
20. [セキュリティコンプライアンス](#20-セキュリティコンプライアンス)
21. [システム管理](#21-システム管理)
22. [運用診断 API](#22-運用診断-apiv1461)

---

## 1. クイックスタート

### 初回ログイン

1. ブラウザーで k8ops のアドレスにアクセス（例: `https://k8ops.iot2.win` または `http://localhost:9090`）
2. デフォルトアカウント: `admin` / `admin`
3. 初回ログイン時にパスワード変更を求められます

### ページレイアウト

```
┌─────────┬───────────────────────────────┐
│         │  [Namespace ▼]  [🔔]  [☀/☽]  │  ← トップバー
│ Sidebar ├───────────────────────────────┤
│         │                                │
│ Overview│       Content Area             │  ← コンテンツエリア
│ Diagnose│                                │
│ Nodes   │                                │
│ Pods    │                                │
│ ...     │                                │
└─────────┴───────────────────────────────┘
```

### Ctrl+K コマンドパレット

いつでも `Ctrl+K`（Mac: `Cmd+K`）でグローバルコマンドパレットを開きます：

- `nodes` と入力 → ノードページに移動
- `chat` と入力 → AI Chat を開く
- `cost` と入力 → コスト分析を表示
- 方向キーで選択、Enter で確定、Esc で閉じる

---

## 2. クラスターオーバービュー

Overview ページはクラスター全体のステータスを表示します。

### 統計カード

| カード | 説明 |
|------|------|
| Nodes | クラスターノード総数 / Ready 数 |
| Pods | 実行中 Pod 数 / 総数 |
| CPU | クラスター全体の CPU 使用率 |
| Memory | クラスター全体のメモリ使用率 |
| Warnings | 現在の Warning イベント数 |

### Sparkline トレンドグラフ

各カードの下部には SVG ミニ折れ線グラフがあり、直近 30 分間のトレンド変化を表示します。

### Namespace 切り替え

トップバー左側のドロップダウンセレクターで namespace スコープを切り替えられます。切り替え後、Pods、Events、Nodes などのページに影響します。選択内容は localStorage に永続化されます。

---

## 3. AI Chat — インテリジェントアシスタント

サイドバー下部の Chat ボタンをクリック、または `Ctrl+K` で `chat` と入力して開きます。

### 基本的な使い方

入力ボックスに質問を入力すると、AI は以下のことを行います：

1. 自然言語の意図を理解
2. 適切な K8s ツールを自動的に呼び出し
3. 分析結果をストリーミングで返す

### クエリ例

```
# リソースの確認
default 名前空間の Pod を確認して
CPU 使用率が高いノードはどれ？

# トラブルシューティング
なぜ nginx-deployment の Pod が CrashLoopBackOff になっていますか？
クラスターに異常はありますか？

# 最適化提案
リソース使用状況を分析して
レプリカ数を削減できる Pod はありますか？
```

### ツール呼び出しの透明性

AI がツール呼び出しを実行する際、折りたたみ式の Thinking パネルが表示されます：

- クリックで展開すると、各ツール呼び出しのパラメーターと戻り値が確認できます
- JSON フォーマットで表示、検索機能あり

### 診断提案カード

AI が kubectl コマンドの実行を提案する場合、コードブロックの下に以下が表示されます：

- **▶ Run in Chat** — コマンドを入力ボックスに読み込み、送信・実行しやすくします
- **📋 Copy** — コマンドをクリップボードにコピー

### セッション管理

- **New** — 新しいセッションを作成
- **左側のセッションリスト** — クリックで過去のセッションに切り替え
- セッションは自動的に要約・圧縮されます（20k token を超えると自動的にトリガー）

### Markdown レンダリング

Chat は以下をサポートします：
- コードブロック（シンタックスハイライトとコピーボタン付き）
- テーブル
- リスト、太字、斜体
- リンク（http/https/mailto プロトコルのみ）

---

## 4. 診断と修復

### 診断のトリガー

**方法 1: Web インターフェース**

1. Diagnostics ページに移動
2. "New Diagnostic" をクリック
3. 問題の説明を入力（例: "production 名前空間の API レスポンスが遅い"）
4. 送信後、AI が自動的に分析

**方法 2: AI Chat**

Chat で直接問題を説明すると、AI が自動的に診断フローを実行します。

**方法 3: CRD**

```bash
kubectl apply -f - <<EOF
apiVersion: aiops.ggai.dev/v1alpha1
kind: DiagnosticReport
metadata:
  name: check-nginx
  namespace: k8ops-system
spec:
  problem: "nginx pods keep restarting"
EOF
```

**方法 4: CLI**

```bash
k8ops diagnose --problem "pods in production keep CrashLoopBackOff"
```

### 診断結果

各診断レポートには以下が含まれます：

- **Root Cause** — AI が分析した根本原因
- **Evidence** — 分析を裏付けるログ、イベント、メトリクスデータ
- **Recommendations** — 推奨される修復アクション
- **Severity** — 重要度（Info / Warning / Critical）

### 自動修復 (Remediation)

AI が生成した修復プランは人手による承認が必要です：

1. Remediations ページに移動
2. 承認待ちの修復プランを確認
3. **Approve** をクリックして実行、または **Reject** で却下
4. すべての操作は監査ログに記録されます

---

## 5. 最適化提案

Optimizations ページは、クラスターリソースに対する AI の最適化提案を表示します。

### 提案タイプ

| タイプ | 説明 |
|------|------|
| Resource Rightsizing | CPU/Memory の requests と limits の調整提案 |
| HPA Gap | 水平オートスケーリング設定が欠如している Deployment |
| PDB Gap | PodDisruptionBudget が欠如しているワークロード |
| Cost Saving | 削減可能なコスト（アイドルリソース、過剰なレプリカなど） |

### 操作

- 提案をクリックして詳細を表示
- そのまま Apply するか無視可能

---

## 6. コスト分析 (FinOps)

Cost ページはクラスターのコスト可視性を提供します。

### 機能

- **名前空間別コスト集計** — namespace ごとのリソース消費と推定コストを表示
- **リソース使用率** — CPU/Memory の実際の使用量 vs 割り当て量
- **Rightsizing 提案** — 過剰割り当てリソースの調整提案
- **アイドルリソース** — 長期間未使用の PV、LoadBalancer、Elastic IP など

---

## 7. クラスタートポロジーの可視化

Topology ページは、ノードと Pod の関係を SVG グラフィックで表示します。

### 視覚要素

| 要素 | 説明 |
|------|------|
| 緑色の枠 | Ready ノード |
| 赤色の枠 | NotReady ノード |
| ノード枠内のプログレスバー | CPU（上）/ MEM（下）使用率 |
| Pod の緑色のドット | Running |
| Pod の黄色のドット | Pending |
| Pod の赤色のドット | Failed |
| Pod の点滅する枠線 | CrashLoop（restarts > 3） |

### インタラクション

- **Pod をクリック** — 該当 Pod のログビューアーを開く
- **下部の統計** — Ready/NotReady ノード数、Pod ステータスの集計

---

## 8. ノードと Pod の管理

### Nodes ページ

- ノードリストテーブル：名前、ロール、ステータス、CPU、メモリ、Pod 数
- 各列で検索フィルターをサポート
- ノード名をクリックすると詳細情報とそのノード上のすべての Pod を表示

### Pods ページ

- Pod リストテーブル：名前、名前空間、ステータス、再起動回数、ノード、経過時間
- 名前空間フィルターとリアルタイム検索をサポート

### Pod ログビューアー

Pod の行をクリックするとログビューアーが開きます：

- **リアルタイムストリーミング** — SSE プッシュでログがリアルタイム更新
- **ログレベルハイライト** — ERROR（赤）、WARN（黄）、DEBUG（グレー）
- **検索フィルター** — キーワード入力でログ行をフィルター
- **自動スクロール** — 新しいログが到着した際、自動的に最下部へスクロール（一時停止可能）
- **ダウンロード** — 現在のログをファイルとしてエクスポート

---

## 9. イベントストリームと通知

### Events ページ

K8s クラスターイベントを表示します。以下をサポート：

- リアルタイム検索フィルター
- Warning イベントの赤色ハイライト
- 名前空間によるフィルタリング

### リアルタイムイベントストリーム

Events ページの右側に Live Events パネルがあります：

- **Go Live** をクリックして SSE リアルタイムプッシュを有効化
- 新しいイベントには青色の NEW バッジアニメーション
- 削除されたイベントには赤色の DEL バッジ
- Warning イベントは自動的に赤色ハイライト

### 通知センター

トップバー右側のベルアイコン：

- アラートがある場合、赤色の数字バッジ + パルスアニメーションを表示
- クリックでドロップダウンパネルを展開
- 直近の Warning イベントと NotReady ノードを表示
- 60 秒ごとに自動更新

---

## 10. リソースブラウザーと YAML エディター

### Resources ページ

クラスター内のすべての K8s リソースをブラウズ：

- API Group / Resource Type ごとにグループ化
- リソース名をクリックして YAML 定義を表示
- 名前空間の複数選択フィルターをサポート

### YAML ビューアー

任意のリソースをクリックすると YAML オーバーレイが開きます：

- フォーマットされた完全な YAML を表示
- **Copy** ボタンでワンクリックコピー

### YAML エディター

YAML ビューアーで **Edit** ボタンをクリックすると編集モードに切り替わります：

1. YAML コンテンツが編集可能な textarea に切り替わります
2. 変更後、**Apply** をクリックして送信
3. バックエンドは server-side apply（kubectl apply セマンティクス）を使用
4. 成功時は緑色の通知、失敗時は赤色のエラーメッセージを表示

---

## 11. RBAC アクセス制御

RBAC ページ（admin 権限が必要）でユーザーとロールを管理します。

### ユーザー管理

- **ユーザー作成** — ユーザー名、パスワード、ロール、名前空間スコープ
- **ユーザー編集** — ロールの変更、有効化/無効化
- **ユーザー削除**

### ロール

| ロール | 権限 |
|------|------|
| admin | クラスター全体の読み書き、ユーザー管理可能 |
| operator | ほとんどのリソースの読み書き、RBAC/Secrets の管理は不可 |
| viewer | 読み取り専用アクセス |

### 名前空間スコープ

各ユーザーは特定の名前空間にバインドでき、そのスコープ内のリソースのみアクセス可能です（K8s impersonation で実現）。

---

## 12. 監査ログ

Audit ページはすべての AI 操作の監査記録を表示します。

### 機能

- **Severity フィルター** — ドロップダウンで Info / Warning / Error / Critical を選択
- **リアルタイム検索** — キーワード入力でフィルター
- **統計カード** — Total / Successful / Failed / Critical / Warnings
- **テーブル** — 時間、重要度、アクション、対象リソース、操作者、成功/失敗、所要時間

### 監査範囲

以下のすべての操作が記録されます：

- AI ツール呼び出し（kubectl get/describe/logs など）
- AI による修復操作
- LLM API 呼び出し
- ユーザーログイン/ログアウト
- リソース変更

---

## 13. 設定と構成

Settings ページで AI Provider と認証を構成します。

### AI Provider 構成

| フィールド | 説明 |
|------|------|
| Provider Type | openai / deepseek / zai / anthropic |
| Model | gpt-4o / deepseek-chat / glm-4-plus など |
| Endpoint | LLM API アドレス（空欄でデフォルトを使用） |
| API Key | LLM API キー |

### 認証構成

- **Local** — 内蔵ユーザーシステム（デフォルト）
- **LDAP** — エンタープライズ LDAP/AD 統合
- **OIDC** — GitHub / Google / Keycloak など

---

## 14. キーボードショートカット

| ショートカット | 機能 |
|--------|------|
| `Ctrl+K` / `Cmd+K` | コマンドパレットを開く |
| `Esc` | コマンドパレット / ポップアップを閉じる |
| `↓` / `↑` | コマンドパレットで選択 |
| `Enter` | コマンドパレットで確定 |

---

## 15. テーマ切り替え

サイドバー右上の月/太陽ボタンをクリックして、ダーク/ライトテーマを切り替えます。選択は localStorage に永続化され、ページ更新後も維持されます。

---

## 付録

### 関連ドキュメント

- [アーキテクチャ設計](ARCHITECTURE.md)
- [デプロイガイド](DEPLOYMENT.md)
- [ローカル実行](LOCAL_RUN.md)
- [API リファレンス](API.md)
- [セキュリティポリシー](SECURITY.md)

### よくある質問

**Q: Chat が応答しない？**
A: Settings → Provider の構成が正しいか、API Key が有効かを確認してください。

**Q: 特定の名前空間が見えない？**
A: 現在のユーザーの RBAC ロールが名前空間のアクセス範囲を制限している可能性があります。管理者に連絡して調整してください。

**Q: Pod ログビューアーが空白？**
A: Pod が起動したばかりでログがないか、ログの権限がない可能性があります。RBAC 構成を確認してください。

**Q: AI が提案するコマンドは安全ですか？**
A: すべての AI 提案操作は、まず Safety Checker の dry-run 検証を経て、修復操作は人手による承認が必要です。

---

## 16. キャパシティプランニング

### ストレージ容量監視

**パス:** Dashboard → Capacity タブ

クラスター内のすべての PVC（PersistentVolumeClaim）のストレージステータスを表示：

| 指標 | 説明 |
|------|------|
| Total PVCs | クラスター内の PVC 総数 |
| Bound | PV にバインド済みの PVC 数 |
| Pending | バインド待ちの PVC |
| Total Capacity | すべての PVC の総容量 |
| Requested | すべての PVC が要求した総量 |

### ノード容量分析

Capacity ページでは各ノードのリソース使用率も表示します：

- **CPU 使用率**: 要求済み CPU / 割り当て可能 CPU（色分け: <60% 緑、60-80% 黄、>80% 赤）
- **メモリ使用率**: 要求済みメモリ / 割り当て可能メモリ
- **Pod 密度**: 実行中 Pod 数 / 最大 Pod 数制限
- **スケールアウト提案**: ノードリソースが 80% を超えると自動的にスケールアウト提案を生成

### クラスター級の集計

| 指標 | 説明 |
|------|------|
| Cluster CPU Utilization | クラスター全体の CPU 要求/割り当て可能比率 |
| Cluster Mem Utilization | クラスター全体のメモリ要求/割り当て可能比率 |
| Total CPU Allocatable | クラスター全体の割り当て可能 CPU 総量 |
| Total CPU Requested | クラスター全体の要求済み CPU 総量 |

---

## 17. HPA の可視化

**パス:** Dashboard → HPA タブ

すべての HorizontalPodAutoscaler のオートスケーリングステータスを表示：

### 機能

- **レプリカスケールバー**: 現在のレプリカ数、期待レプリカ数、最小/最大範囲を可視化
- **メトリクス使用率バー**: CPU/メモリの現在の使用率 vs 目標値（緑/黄/赤）
- **スケーリングステータス表示**: 現在のレプリカ数 ≠ 期待レプリカ数の場合に "SCALING" バッジを表示
- **サマリーカード**: HPA 総数、スケーリング中の数、現在/期待レプリカ総数

### 対応メトリクスタイプ

| タイプ | 説明 |
|------|------|
| Resource | CPU/メモリ使用率のパーセンテージ |
| Pods | カスタム Pod メトリクス（例: QPS） |
| External | 外部メトリクス（例: SQS キュー長） |
| ContainerResource | コンテナ級のリソースメトリクス |

---

## 18. コンテナイメージインベントリ

**パス:** Dashboard → Images タブ

クラスター内で使用中のすべてのコンテナイメージを表示：

| 指標 | 説明 |
|------|------|
| Unique Images | 重複排除後のイメージ総数 |
| Using :latest | `:latest` タグを使用しているイメージ数（本番環境では非推奨） |
| No Limits | リソース limits が設定されていないイメージ数 |
| No Requests | リソース requests が設定されていないイメージ数 |
| Registries | 使用中のイメージレジストリ数 |

### セキュリティのベストプラクティス

- `:latest` タグの使用を避ける — 固定バージョン番号で再現可能なデプロイを確保
- すべてのコンテナに CPU/メモリ limits を設定 — リソース枯渇を防止
- すべてのコンテナに CPU/メモリ requests を設定 — スケジューラーの正確な割り当てを確保

---

## 19. 名前空間リソースランキング

**パス:** Dashboard → Namespaces タブ

CPU 消費量でソートしてすべての名前空間のリソース使用状況を一覧表示：

### 機能

- **リソース集計**: 各 namespace の CPU/メモリ requests + limits、Pod 数、PVC ストレージ量
- **クラスター占有率**: CPU/メモリ要求のクラスター割り当て可能総量に対する割合（視覚的なプログレスバー付き）
- **検索フィルター**: 特定の namespace を素早く特定
- **詳細ドリルダウン**: 任意の namespace をクリックして ResourceQuota の使用状況、LimitRange 構成、直近の Warning イベントを表示

---

## 20. セキュリティコンプライアンス

### CIS Benchmark コンプライアンススキャン

**パス:** Dashboard → Compliance タブ

CIS Kubernetes Benchmark チェックを実行。以下のカテゴリをカバー：

| カテゴリ | チェック項目 |
|------|--------|
| RBAC | cluster-admin バインド範囲、ワイルドカード ClusterRole、デフォルト SA の使用 |
| Pod Security | 特権コンテナ、hostNetwork/hostPID/hostIPC、hostPath ボリューム、root ユーザー、リソース limits |
| Network | NetworkPolicy カバレッジ率 |
| Secrets | Secret 管理の健全性 |

### コンプライアンスレポートのダウンロード

"Download Report" ボタンをクリックして完全なコンプライアンスレポート（.txt 形式）をダウンロードできます。内容：

- コンプライアンススコア（パーセンテージ）
- 各チェック項目のステータス（PASS/WARN/FAIL）
- 修正提案（WARN/FAIL 項目に対して）

### 監査イベント検索

**パス:** API → `GET /api/audit/events`

複数の次元で監査ログをフィルタリング：

| パラメーター | 説明 |
|------|------|
| `actor` | ユーザー名でフィルター |
| `action` | 操作タイプでフィルター（例: delete, scale, exec） |
| `q` | 全文検索 |
| `severity` | 重要度でフィルター |
| `from`/`to` | 時間範囲（RFC3339 形式） |

### CSV エクスポート

`GET /api/audit/export` — 監査ログを CSV 形式でエクスポート。SIEM システムにインポートしてコンプライアンス分析が可能です。

---

## 21. システム管理

### システム情報

`GET /api/system/info` がランタイム情報を提供：

- バージョン番号、Go バージョン、実行プラットフォーム
- メモリ使用量（Alloc/Sys/GC cycles/Heap objects）
- Goroutine 数
- サービス稼働時間
- 監査ログのサイズとイベント数

### ログ管理

| API | 機能 |
|-----|------|
| `POST /api/system/log/rotate` | 監査ログの手動ローテーション（admin） |
| `POST /api/system/log/cleanup` | 30 日以上経過したローテーションファイルのクリーンアップ（admin） |

### ログレベル構成

環境変数 `LOG_LEVEL` で構成（debug/info/warn/error）：

```bash
kubectl set env daemonset/k8ops -n k8ops-system LOG_LEVEL=debug
kubectl rollout restart daemonset/k8ops -n k8ops-system
```

### バックアップ管理

| API | 機能 |
|-----|------|
| `GET /api/system/backup` | すべてのバックアップファイルを一覧表示 |
| `POST /api/system/backup` | データベースバックアップを作成 |
| `DELETE /api/system/backup?name=X` | 指定したバックアップを削除 |
| `POST /api/system/backup/restore?name=X` | バックアップからデータベースを復元 |

### API パフォーマンス監視

`GET /api/system/performance` が各 API エンドポイントのレイテンシー統計を提供：

- **p50/p95/p99** パーセンタイルレイテンシー
- 平均および最大レイテンシー
- エラー率とリクエスト総数

---

## 22. 運用診断 API（v14.61+）

### Network Policy 監査

`GET /api/security/network-policies` がクラスターの NetworkPolicy カバレッジを監査：

- NetworkPolicy のない名前空間を検出（デフォルトは完全オープン）
- 緩いポリシーを特定（0.0.0.0/0 のインバウンド/アウトバウンド）
- 重要度別の分類: critical / warning / info
- 各発見事項には詳細な説明と修正提案を含む

### Pod 再起動診断

`GET /api/diagnostics/restarts` が Pod の再起動パターンと根本原因を診断：

- 再起動パターンの分類: crash-loop / occasional / post-deploy
- 終了原因の抽出: OOMKilled / Error / 終了コード
- 待機ステータスの特定: CrashLoopBackOff / ImagePullBackOff
- コンテナごとの個別診断情報

### デプロイ Rollout ステータス

`GET /api/deployments/rollout` がすべてのワークロードの rollout ヘルスステータスを追跡：

- Deployment / StatefulSet / DaemonSet をカバー
- 7 種類のステータス: complete / in-progress / stalled / degraded / paused / failed / scaled-to-zero
- ProgressDeadlineExceeded と ReplicaFailure を検出
- ステータスによるフィルタリングをサポート: `?status=failed`

### リソース浪費検出

`GET /api/resources/waste` がコスト削減のための浪費・孤立リソースをスキャン：

- 6 種類の浪費: 停止サービス、未使用 PVC、孤立した ConfigMap/Secret、空の名前空間、未バインド PV
- コストリスク評価: low / moderate / high
- 各項目に重要度、経過時間、クリーンアップ提案を含む
- システムリソースをインテリジェントにフィルター（kube-system、SA token、Helm release）

### スケーリングボトルネック検出

`GET /api/scaling/bottlenecks` が水平スケーリングを制限する要因を特定：

- 7 種類のボトルネック: ノードスケジューリング、ノードプレッシャー、クォータ制限、HPA の停滞、PDB のブロック、ストレージ枯渇
- クラスター容量サマリー: ノード数、CPU/メモリ、Pod 容量、スケーリング余地
- 各項目に影響レベルと修正提案を含む

### RBAC 権限リスク分析

`GET /api/security/rbac-risk` がクラスター内のすべての RBAC バインドのセキュリティリスクを分析：

- 0-100 のスコアリングシステム、高リスクバインドを自動的に特定
- 5 段階のリスクレベル: critical / high / elevated / moderate / low
- 検出項目: cluster-admin バインド、権限昇格（escalate/bind/impersonate）、ワイルドカード権限（verbs/resources: *）、クラスター範囲の書き込み操作、機密リソースアクセス（secrets/pods/exec）
- 各項目に詳細なスコア内訳と修正提案を含む（最小権限の原則）
- 名前空間によるフィルタリングをサポート: `?namespace=default`

### CronJob 実行ヘルス監視

`GET /api/operations/cronjobs/health` がすべての CronJob の実行健全性を監視：

- 5 段階のヘルスステータス: healthy / warning / failing / suspended / no-runs
- 連続失敗の検出（3 回以上 = failing）、成功率 50% 未満、一時停止中のスケジュール、未実行の検出
- OwnerReferences を通じて CronJob と子 Job を関連付け
- 次回予想実行時間の計算
- 名前空間によるフィルタリングをサポート: `?namespace=production`

### Service & Endpoint ネットワークヘルス監視

`GET /api/networking/health` がすべての Service と Ingress のネットワーク接続性をスキャン：

- 5 段階の Service ヘルスステータス: healthy / degraded / no-endpoints / misconfigured / external
- セレクターの不一致（label mismatch）、すべてのエンドポイント利用不可、部分的なデグレード、LoadBalancer の IP 待ちを検出
- Ingress バックエンドの検証: バックエンド Service の存在確認、利用可能なエンドポイントの有無
- Pod セレクターのマッチングをクロスリファレンスし、根本原因分析を提供
- 名前空間によるフィルタリングをサポート: `?namespace=default`

### PV/PVC ストレージヘルス監視

`GET /api/storage/health` がすべての PVC/PV のストレージ健全性をスキャン：

- 6 段階の PVC ヘルスステータス: bound / pending / lost / failed / orphaned / near-capacity
- Pending の診断: ストレージクラスなし、WaitForFirstConsumer バインドモード、provisioner ログの確認
- 孤立した PVC の検出: バインド済みだが 1 日以上 Pod に使用されていない（容量の浪費）
- PV の問題: Released（手動クリーンアップが必要）、Failed（回収失敗）、古い Available（>7 日）
- ストレージクラス分布: デフォルトクラスのマーク、provisioner、reclaim policy、volume expansion サポート
- 名前空間によるフィルタリングをサポート: `?namespace=default`

### ServiceAccount セキュリティ監査

`GET /api/security/service-accounts` がクラスター内のすべての ServiceAccount のセキュリティリスクを包括的に監査：

- 0-100 のリスクスコアリングシステム、高リスク SA を自動的に特定
- 5 段階の重要度: critical / high / elevated / moderate / low
- 検出項目: 未使用 SA（>7 日）、cluster-admin バインド（critical）、デフォルト SA の Pod 使用、不要な token 自動マウント、古い SA（>30 日で権限ありだが未使用）、レガシーの長期間有効な token secret
- 各項目に詳細なセキュリティリスクの説明と修正提案を含む
- 名前空間によるフィルタリングをサポート: `?namespace=default`

### SLO/SLA エラーバジェット追跡

`GET /api/operations/slo` がマルチウィンドウ・マルチバーンレートアルゴリズムに基づく SLO/SLA 達成状況を追跡：

- 5 つの時間ウィンドウ: 5 分、1 時間、6 時間、24 時間、7 日
- 可用性パーセンテージとエラーバジェットの残量/消費率
- マルチウィンドウバーンレート検出（fast: 5m+1h、slow: 6h+24h）
- P50/P95/P99 レイテンシーパーセンタイルと SLO 目標
- 3 段階のステータス: meeting（達成）/ at-risk（リスク）/ violated（違反）
- 名前空間によるフィルタリングをサポート: `?namespace=production`

### ResourceQuota と LimitRange 監視

`GET /api/resources/quota` がすべての名前空間のクォータ使用率と LimitRange 制約をスキャン：

- 4 段階のクォータステータス: ok（<70%）/ warning（70-85%）/ critical（85-100%）/ exceeded（>100%）
- 名前空間ごとの CPU/メモリ/Pod/ConfigMap/Secret/ストレージクォータ使用率
- クォータ保護のない名前空間を特定
- LimitRange のデフォルト/最小/最大制約の分析
- Top コンシューマーランキング
- 名前空間によるフィルタリングをサポート: `?namespace=default`

### デプロイ構成監査

`GET /api/deployments/audit` がすべてのワークロードの構成ベストプラクティス違反を監査：

- 8 つのチェックカテゴリ: revision-history / image-policy / resources / probes / security-context / update-strategy / lifecycle / config-drift
- 各項目に重要度（critical/warning/info）、具体的な問題の説明と実行可能な修正提案を含む
- ヘルススコア 0（完璧）から 100（最悪）
- 集計された Top Findings でクラスター全体の最も一般的な問題を表示
- 名前空間と重要度によるフィルタリングをサポート: `?namespace=default&severity=critical`

### スケジューリングヘルスとリソース断片化分析

`GET /api/scheduling/health` がクラスターのスケジューリングヘルスとリソース使用率を分析：

- ノードごとのスケジュール可能性（cordon/taint/pressure conditions）とリソース利用可能量
- Pending Pod の診断: FailedScheduling イベントの原因を解析（CPU/メモリ不足、taint の不一致、nodeSelector の競合、ボリュームバインド失敗など）
- 最大スケジュール可能 Pod の計算（現在配置可能な最大 Pod サイズ）
- 有効容量 vs 理論容量（スケジュール不可ノードによる容量損失）
- リソース断片化分析（散在する空き容量）
- 超大型 Pod の検出（単一ノードの容量を超えるリクエスト）
- 24h の退去（Eviction）履歴（原因付き）
- ヘルススコア 0-100（重み付きペナルティ）
- 実行可能な修正提案
- 名前空間によるフィルタリングをサポート: `?namespace=default`

### Pod セキュリティポスチャスキャン

`GET /api/security/pods?namespace=xxx&severity=critical` がすべての実行中 Pod のリアルタイムセキュリティポスチャを監査：

- 15 のチェックカテゴリが特権コンテナ、ホストアクセス（network/PID/IPC）、HostPath マウント、危険な capabilities、root 実行、権限昇格などをカバー
- Pod ごとのリスクスコア 0-100（critical=25点/warning=8点/info=2点）
- チェックタイプと名前空間で集計統計
- 名前空間と重要度によるフィルタリングをサポート

### イベントストームとカスケード障害検出

`GET /api/operations/event-storm?namespace=xxx` がクラスターの Warning イベントを分析：

- 4 段階のストーム重要度: critical（>50）/ high（>20）/ medium（>10）/ low（>5）
- フラッピングリソースの検出（同一リソース・同一原因の 3 回以上の繰り返し、フラップ頻度付き）
- 名前空間とイベント理由で集計
- 爆発半径の評価（影響を受けるリソース数）
- 実行可能な調査の提案
- 名前空間によるフィルタリングをサポート: `?namespace=kube-system`

### リソース依存グラフと影響範囲分析

`GET /api/dependencies?kind=Deployment&name=xxx&namespace=xxx` がワークロードの完全な依存グラフを追跡：

- 正方向の依存: ConfigMap、Secret、PVC、ServiceAccount
- 逆方向の依存: Service（label selector）、Ingress、NetworkPolicy、HPA、設定を共有する他の Pod
- 影響範囲の評価: blastRadius スコアとリスクレベル
- 変更前の影響評価に使用し、カスケード障害を回避

### トポロジー分布コンプライアンスチェック

`GET /api/topology/spread?namespace=xxx&domain=topology.kubernetes.io/zone` が Pod のトポロジー分布コンプライアンスをチェック：

- 4 段階のワークロードステータス: balanced / skewed / no-constraint / single-replica
- ワークロードごとのトポロジードメイン分布と偏差の分析
- トポロジー制約が欠如しているマルチレプリカワークロードの検出
- トポロジーラベルが欠如しているノードの特定
- シングルドメインクラスターのヒント
- 名前空間とトポロジードメインキーによるフィルタリングをサポート

### Secret ローテーションとライフサイクル監査

`GET /api/security/secrets/rotation?namespace=xxx` がすべての Secret のライフサイクルを監査：

- 経過時間の追跡: stale（>90d）/ very stale（>180d）
- 未使用 Secret の検出（いかなる Pod からも参照されていない）
- TLS 証明書の期限切れ検出（証明書を解析し、期限切れと <30d の期限切れを検出）
- Docker registry Secret、レガシー SA token の追跡
- 機密名の検出（password/key/token/credential）
- Secret ごとのリスクレベル、クラスターローテーションスコア 0-100
- 名前空間によるフィルタリングをサポート

### ヘルスプローブ有効性監査

`GET /api/operations/probes?namespace=xxx` がプローブ構成を監査：

- 8 つのチェックカテゴリ: プローブの欠落、過度にアグレッシブ、タイムアウトが短すぎる、不適切な閾値など
- ワークロードごとのリスクスコア、クラスターの有効性スコア（0-100）
- 集計された Top 問題の統計
- 実行可能な提案

### ワークロードの陳腐化追跡

`GET /api/product/staleness?namespace=xxx` がデプロイの陳腐化を追跡：

- 5 段階の陳腐化分類: fresh/recent/stale/very-stale/ancient
- イメージタグの分析: :latest、digest、no-tag
- 経過時間の分布バケット、名前空間統計
- クラスターの鮮度スコア（0-100）

### リソースオーバーコミットとプレッシャー分析

`GET /api/scalability/overcommit?namespace=xxx` がリソースのオーバーコミットを分析：

- ノードごとの CPU/メモリ request と limit のオーバーコミット比率
- プレッシャースコア 0-100 とリスクレベル
- limits/requests のない Pod の検出
- クラスターの安全スコア 0-100
- 名前空間別のリソース消費明細

### イメージセキュリティとサプライチェーン分析

`GET /api/security/images?namespace=xxx` がすべてのコンテナイメージのサプライチェーンセキュリティをスキャン：

- Digest ロックの検出（@sha256: 不変参照）
- :latest タグの検出（可変、再現不可）
- タグなしイメージの検出（デフォルト :latest）
- 古いバージョンタグの検出（v1, 1.0 — 既知の CVE を含む可能性）
- パブリック vs プライベートイメージレジストリの分析
- イメージごとのリスクレベル、レジストリごとの統計
- クラスターのイメージセキュリティスコア 0-100

### キャパシティプランニング

`GET /api/capacity/planning` がノードの容量計画：

- ノードごとの CPU/メモリ要求 vs 割り当て可能量
- 残り容量とスケールアウト提案
- リソース断片化の検出

### 容量予測

`GET /api/capacity/forecast` が容量トレンド予測：

- 履歴データに基づくリソース成長トレンド
- 予想枯渇時間
- スケールアウト提案

### リソース効率分析

`GET /api/efficiency` がリソース使用効率：

- 過大なリソース割り当ての検出
- リソース浪費の特定
- 最適化提案

### PDB ステータス

`GET /api/pdbs` が Pod Disruption Budget のステータス：

- PDB 構成チェック
- 許容中断数 vs 現在の利用可能数
- PDB ブロックの検出

### バージョン互換性

`GET /api/compatibility` が K8s バージョンの互換性：

- API 廃止のチェック
- リソースバージョンの互換性
- アップグレード影響の評価

### 証明書の期限切れ

`GET /api/certificates/expiry` が TLS 証明書の期限切れスキャン：

- クラスター証明書の期限切れ時間
- 期限切れ間近の証明書の警告
- 更新提案

### Addon ヘルス

`GET /api/addons/health` がクラスターアドオンのヘルスチェック：

- コアアドオンの稼働ステータス
- 異常なアドオンの検出
- 修正提案

### クラスター健康スコア

`GET /api/operations/health-score` がすべてのクラスター健康シグナルを一つの総合スコアに集約：

- 5 つの重み付け次元: Node(25%) + Pod(25%) + Workload(20%) + Events(15%) + API Server(15%)
- 総合スコア 0-100、アルファベット評価 A-F
- ステータス: healthy / warning / critical
- 各次元のスコア、重み、詳細
- クラスターサマリー: ノード/Pod/ワークロード数
- 重要度順の Top 問題

### HPA/VPA リソース適正構成提案

`GET /api/scalability/autoscale-recommendations?namespace=xxx` がオートスケーリングとリソースの右サイジングを分析：

- HPA が欠如しているマルチレプリカワークロードの検出
- CPU 要求の過大化（>1 core/container）
- メモリ要求の過大化（>2GB/container）
- HPA 効率分析（上限/下限/アイドルへの到達）
- ワークロードごとの現在 vs 推奨リソース値
- 潜在的な CPU コアとメモリの節約量
- クラスターのオートスケーリングスコア 0-100

### Ingress とトラフィックルーティングヘルス監視

`GET /api/product/ingress-health?namespace=xxx` がすべての Ingress のトラフィックルーティングヘルスをチェック：

- バックエンド Service の存在とエンドポイントの準備状態チェック
- TLS 構成の検出
- IngressClass の有効性検証
- host+path の競合検出
- ルーティングルールなしの検出
- Ingress ごとのステータスとクラスターのヘルススコア 0-100

### ノード条件とリソースプレッシャー

`GET /api/operations/node-pressure` がすべてのノードの条件とリソースプレッシャーを分析：

- DiskPressure / MemoryPressure / PIDPressure / NetworkUnavailable の検出
- CPU/メモリ/Pod の使用率 vs 割り当て可能量
- ノードごとのリスクレベル（critical/high/medium/low）
- クラスターのプレッシャースコア 0-100

### PVC バインドとストレージパフォーマンス

`GET /api/scalability/pvc-analysis?namespace=xxx` がストレージバインドの健全性を分析：

- Stuck PVC の根本原因検出（>5分 pending）
- バインド時間の測定と遅いバインドの検出（>30s）
- Lost PVC の検出
- StorageClass ごとの統計とプロビジョナー分析
- クラスターのストレージヘルススコア 0-100

### Namespace ガバナンスとライフサイクル

`GET /api/product/namespaces/lifecycle` が名前空間のガバナンスを監査：

- ResourceQuota / LimitRange / NetworkPolicy のカバレッジ率
- 専用 ServiceAccount の検出（最小権限）
- 必須ラベルのチェック（app, team, env, owner）
- 名前空間のライフサイクル（active / stale / terminating）
- クラスターのガバナンススコア 0-100

### RBAC 実効権限と権限昇格分析

`GET /api/security/rbac-effective` がすべての主体の RBAC 実効権限を分析：

- ClusterRoleBindings + RoleBindings を集約して実際の権限を計算
- cluster-admin 等価の検出
- 権限昇格パスの検出（RBAC を変更できる主体）
- ワイルドカード（*）権限の検出
- Secret 読み取りと Pod exec アクセスの分析
- クラスターの RBAC セキュリティスコア 0-100

### コンテナ OOM Kill 追跡

`GET /api/operations/oom-tracker?namespace=xxx` がコンテナ OOM イベントを追跡：

- OOMKilled コンテナの検出と根本原因分析
- 高再起動回数の検出（>=5）
- 欠落/過小なメモリ制限の検出
- 制限が要求より大幅に大きい（10倍以上）ノードプレッシャーリスク
- Top OOM ランキングと名前空間ごとの統計
- クラスターの OOM リスクスコア 0-100

### ストレージ容量枯渇予測

`GET /api/scalability/storage-forecast` がストレージ容量を予測：

- PV ごとの使用率、成長率、枯渇日数の予測
- Longhorn actual-size アノテーション対応
- クラスターのストレージ枯渇日数の推定
- StorageClass ごとの統計とプロビジョナー分析
- 高リスク名前空間のランキング
- ストレージヘルススコア 0-100

### DNS 解決ヘルスチェック

`GET /api/product/dns-health` が DNS 解決の健全性を分析：

- CoreDNS Pod のヘルスチェック（実行/準備/再起動/バージョン）
- Corefile 構成の分析（forwarders, plugins）
- Headless Service のエンドポイントカバレッジと NXDOMAIN リスク
- NodeLocal DNS キャッシュの検出
- Pod dnsConfig ndots カバレッジの検出
- External-DNS マネージドサービスディスカバリー
- クラスターの DNS ヘルススコア 0-100
