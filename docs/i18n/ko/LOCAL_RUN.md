# k8ops 로컬 실행 가이드

> Kubernetes 클러스터 배포 없이 노트북/워크스테이션에서 k8ops 바이너리를 직접 실행합니다.

---

## 적용 시나리오

- **로컬 개발 디버깅** — 매번 이미지를 빌드할 필요 없이 빠르게 코드 반복
- **오프라인 관리 도구** — 스마트 kubectl 대체제로 사용
- **데모 및 평가** — 클러스터 내부 배포 없이 모든 기능 체험
- **CI/CD 통합** — 파이프라인에서 진단 도구로 실행

---

## 사전 조건

- Go 1.26+ (또는 사전 컴파일된 바이너리 직접 다운로드)
- kubectl이 구성되어 있고 클러스터에 연결 가능
- LLM API Key (OpenAI / DeepSeek / ZAI 등)

---

## 방법 1: 소스에서 컴파일

```bash
cd k8ops

# manager 컴파일 (dashboard 서버)
go build -o k8ops-manager ./cmd/manager/

# CLI 도구 컴파일
go build -o k8ops ./cmd/k8ops/
```

## 방법 2: 사전 컴파일된 바이너리 다운로드

[GitHub Releases](https://github.com/topcheer/k8ops/releases)에서 해당 플랫폼의 바이너리를 다운로드합니다.

---

## Dashboard 시작

```bash
AIOPS_API_KEY=your-api-key \
  ./k8ops-manager \
  --leader-elect=false \
  --dashboard-address=:9090 \
  --auth-db-path=/tmp/k8ops.db
```

시작 후 `http://localhost:9090`에 접속, 기본 로그인 `admin / admin`.

### 매개변수 설명

| 매개변수 | 기본값 | 설명 |
|------|--------|------|
| `--dashboard-address` | `:9090` | Dashboard 수신 주소 |
| `--leader-elect` | `false` | Leader Election (단일 인스턴스 실행 시 비활성화 필요) |
| `--metrics-bind-address` | `:8080` | Prometheus metrics 포트 |
| `--health-probe-bind-address` | `:8081` | 상태 확인 포트 |
| `--auth-db-path` | `/data/k8ops.db` | SQLite 데이터베이스 경로 |
| `--auth-jwt-secret` | (무작위 생성) | JWT 서명 키 |
| `--provider-type` | `openai` | LLM provider |
| `--provider-model` | `gpt-4o` | 모델 이름 |
| `--provider-api-key` | (필수) | LLM API Key |
| `--provider-endpoint` | (기본값) | 커스텀 API 엔드포인트 |

### 환경 변수

모든 매개변수는 환경 변수로도 설정할 수 있습니다:

```bash
export AIOPS_API_KEY=sk-your-key
export PROVIDER_TYPE=deepseek
export PROVIDER_MODEL=deepseek-chat
export AUTH_DB_PATH=$HOME/.k8ops/k8ops.db
export AUTH_JWT_SECRET=your-secret

./k8ops-manager --leader-elect=false
```

---

## kubeconfig 검색 메커니즘

k8ops는 controller-runtime의 `ctrl.GetConfigOrDie()`를 사용하여 kubeconfig를 자동으로 검색합니다. 검색 순서:

1. `KUBECONFIG` 환경 변수
2. `~/.kube/config` (기본 경로)
3. In-cluster config (`/var/run/secrets/kubernetes.io/serviceaccount/`)

로컬 실행 시 자동으로 `~/.kube/config`를 사용하며, 추가 구성이 필요 없습니다.

### 특정 클러스터 지정

```bash
KUBECONFIG=~/.kube/prod-config ./k8ops-manager --leader-elect=false
```

### 다중 클러스터 전환

```bash
# kubectx를 사용하여 전환
kubectx prod-cluster
./k8ops-manager --leader-elect=false
```

---

## 데이터 흐름 차이

### 클러스터 내 실행 vs 로컬 실행

| 구분 | 클러스터 내 (DaemonSet/Deployment) | 로컬 실행 |
|------|------|------|
| K8s API 인증 | ServiceAccount token | kubeconfig |
| Host 도구 | `nsenter`로 호스트 머신 접근 | 로컬 머신에서 직접 실행 |
| Auth 데이터 | PVC 영속화 | 로컬 SQLite 파일 |
| Leader Election | 다중 복제본에 필요 | 단일 인스턴스에서 비활성화 |
| RBAC 임퍼스네이션 | 사용자 → ServiceAccount | 사용자 → kubeconfig 사용자 |
| 네트워크 권한 | Pod 네트워크 | 로컬 머신 네트워크 |
| 로그 출력 | stdout → kubectl logs | 터미널에 직접 출력 |

### Host 도구 동작

컨테이너에서 Host 도구는 `nsenter -m -u -i -n -p --`를 통해 호스트 네임스페이스에 접근합니다. 로컬 실행 시 `/bin/sh -c`로 직접 실행되며, 로컬 운영 체제에 접근합니다.

즉:
- `host_disk_check`는 로컬 디스크를 확인
- `host_process_list`는 로컬 프로세스를 나열
- `host_exec`는 로컬에서 명령을 실행

---

## CLI 도구 사용

```bash
# 진단
./k8ops diagnose --problem "pods in production keep CrashLoopBackOff"

# 최적화 제안 보기
./k8ops optimize --namespace production

# 수정 트리거
./k8ops remediate --plan <plan-name> --approve
```

---

## 백그라운드 상주 실행

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

## 개발 모드

### 핫 리로드

```bash
# air 설치
go install github.com/air-verse/air@latest

# k8ops 프로젝트 루트 디렉토리에서
air --build.cmd "go build ./cmd/manager/" --build.bin "./manager"
```

### 디버그

```bash
# DEBUG 로그 활성화
DEBUG=true ./k8ops-manager --leader-elect=false

# JSON 구조화 로그 보기
tail -f /tmp/k8ops.log
```

---

## 문제 해결

### "unable to get kubeconfig"

`~/.kube/config`가 존재하고 유효한지 확인:
```bash
kubectl cluster-info  # kubeconfig 테스트
```

### "address already in use :9090"

```bash
# 9090을 점유한 프로세스 확인
lsof -i :9090
# 또는 다른 포트 사용
./k8ops-manager --dashboard-address=:9091
```

### Auth DB 잠김

DB 파일을 삭제하고 재초기화:
```bash
rm /tmp/k8ops.db
./k8ops-manager --auth-db-path=/tmp/k8ops.db
```

### 프로바이더 시간 초과

더 긴 타임아웃을 설정하거나 네트워크를 확인:
```bash
export PROVIDER_ENDPOINT=https://api.openai.com/v1
# 네트워크 도달 가능성 확인
curl -I https://api.openai.com/v1/models
```
