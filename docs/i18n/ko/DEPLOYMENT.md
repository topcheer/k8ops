# k8ops 배포 가이드

## 원클릭 설치 및 제거

### 사전 조건

- Kubernetes 1.24+ (k3s / k8s / EKS / GKE / AKS 모두 가능)
- kubectl이 구성되어 있고 클러스터에 연결 가능
- 로컬 또는 원격 컨테이너 이미지 저장소 (기본값: `registry.iot2.win`)
- 선택 사항: LLM API Key (OpenAI / DeepSeek / ZAI 등 호환 인터페이스)

---

## 방법 1: Deployment 모드 (권장)

단일 복제본 Deployment로, 대부분의 시나리오에 적합합니다. Ingress, Service, ConfigMap, Secret, RBAC를 포함하여 한 번의 명령으로 전체 배포를 완료합니다.

### 설치

```bash
# 로컬 네트워크 (도메인, 이미지, CORS 등 모든 구성 포함)
kubectl apply -k config/deploy/overlays/local

# 또는 커스텀 overlay
cp -r config/deploy/overlays/local config/deploy/overlays/myorg
# myorg/kustomization.yaml 편집: 이미지 주소, 도메인, CORS 등 교체
kubectl apply -k config/deploy/overlays/myorg
```

### 검증

```bash
# Pod 상태 확인
kubectl get pods -n k8ops-system

# Ingress 확인
kubectl get ingress -n k8ops-system

# Dashboard 접속
# 브라우저에서 https://<your-domain> 열기 (예: https://k8ops.iot2.win)
# 기본 로그인: admin / admin (첫 로그인 시 비밀번호 변경 안내)
```

### 제거

```bash
kubectl delete -k config/deploy/overlays/local
```

---

## 방법 2: DaemonSet 모드

각 노드에서 하나의 Pod를 실행하며 노드 수준 진단을 지원합니다 (hostPID, hostPath). 깊이 있는 노드 모니터링이 필요한 시나리오에 적합합니다.

### 설치

```bash
kubectl apply -f config/daemonset-local.yaml
```

### 검증

```bash
# DaemonSet 확인 (각 노드당 하나의 Pod)
kubectl get ds -n k8ops-system
kubectl get pods -n k8ops-system -o wide

# Dashboard 접속 (Service ClusterIP 또는 Ingress 통해)
kubectl get svc k8ops-dashboard -n k8ops-system
```

### 제거

```bash
kubectl delete -f config/daemonset-local.yaml
```

---

## 방법 3: install.sh 스크립트

```bash
# 설치 (환경 자동 감지, 대화형으로 Deployment / DaemonSet 선택)
./install.sh install

# 제거
./install.sh uninstall

# 상태 확인
./install.sh status
```

---

## 이미지 빌드 및 푸시

```bash
# 로컬 빌드 (amd64, 클러스터 노드에 적합)
docker buildx build --platform linux/amd64 \
  --build-arg VERSION=v1.0.0 \
  -t registry.iot2.win/k8ops:v1.0.0 \
  -t registry.iot2.win/k8ops:latest \
  --load .

# registry에 푸시
docker push registry.iot2.win/k8ops:v1.0.0
docker push registry.iot2.win/k8ops:latest
```

### 다중 아키텍처 빌드

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  --build-arg VERSION=v1.0.0 \
  -t registry.iot2.win/k8ops:v1.0.0 \
  --push .
```

---

## LLM 프로바이더 구성

### 방법 1: Dashboard 인터페이스 구성 (권장)

1. Dashboard 로그인 → **Settings** 탭
2. 프로바이더 유형, API Key, Endpoint, Model 입력
3. **Save** 클릭, K8s ConfigMap/Secret에 자동 영속화

### 방법 2: 환경 변수

overlay의 ConfigMap에서 설정:

```yaml
configMapGenerator:
- name: k8ops-config
  literals:
  - PROVIDER_TYPE=openai          # openai / deepseek / zai / anthropic
  - PROVIDER_MODEL=gpt-4o
  - PROVIDER_ENDPOINT=https://api.openai.com/v1
```

API Key는 Secret을 통해:

```yaml
secretGenerator:
- name: k8ops-credentials
  literals:
  - api-key=sk-your-api-key-here
```

### 지원되는 프로바이더

| 프로바이더 | Endpoint | 예시 Model |
|----------|----------|------------|
| OpenAI | `https://api.openai.com/v1` | gpt-4o, gpt-4o-mini |
| DeepSeek | `https://api.deepseek.com/v1` | deepseek-chat |
| ZAI (지푸) | `https://open.bigmodel.cn/api/paas/v4` | glm-4-flash, glm-4-plus |
| Anthropic | `https://api.anthropic.com/v1` | claude-3-5-sonnet |
| 로컬 | `http://localhost:11434/v1` | llama3, qwen2 |

---

## 인증 구성

### 로컬 인증 (기본값)

즉시 사용 가능하며, 사용자는 SQLite에 저장됩니다. 첫 로그인: `admin / admin`.

### LDAP

```yaml
# ConfigMap 또는 Provider 구성에서 설정
LDAP_SERVER=ldap://your-ldap:389
LDAP_BIND_DN=cn=admin,dc=example,dc=com
LDAP_BIND_PASSWORD=secret
LDAP_USER_BASE=ou=users,dc=example,dc=com
LDAP_SKIP_TLS_VERIFY=false   # 프로덕션 환경에서는 반드시 false
```

### OIDC (GitHub / Google / Keycloak 등)

```yaml
# Provider 구성 (Dashboard Settings 페이지 또는 CRD)
OIDC_ISSUER=https://your-keycloak/realms/myrealm
OIDC_CLIENT_ID=k8ops
OIDC_CLIENT_SECRET=your-secret
OIDC_REDIRECT_URL=https://k8ops.iot2.win/auth/oidc/callback
```

---

## Ingress 및 TLS

### 자동 TLS (cert-manager + Let's Encrypt)

클러스터에 cert-manager가 설치되어 있는지 확인하고, Ingress에 annotation 추가:

```yaml
annotations:
  cert-manager.io/cluster-issuer: letsencrypt-prod
```

### 기존 TLS 인증서 사용

```bash
kubectl create secret tls k8ops-dashboard-tls \
  --cert=fullchain.pem \
  --key=privkey.pem \
  -n k8ops-system
```

---

## 자주 묻는 질문

### Pod가 계속 Pending 상태

```bash
# 스케줄링 실패 원인 확인
kubectl describe pod <pod-name> -n k8ops-system | tail -10

# 일반적인 원인:
# - hostNetwork 포트 충돌 → hostNetwork: true 제거 또는 포트 선언 충돌 회피
# - 리소스 부족 → resources.requests/limits 조정
# - 노드 taint → tolerations 확인
```

### Dashboard에서 502 반환

```bash
# 1. Pod가 Ready 상태인지 확인
kubectl get pods -n k8ops-system

# 2. Service endpoints 확인
kubectl get endpoints k8ops-dashboard -n k8ops-system

# 3. Ingress backend 확인
kubectl describe ingress -n k8ops-system

# 4. Pod가 완전히 준비될 때까지 대기 후 재시도
```

### 이미지 풀 실패

```bash
# 방법 1: imagePullPolicy: Always 설정 (구체적인 tag 사용 시 권장)
# 방법 2: 노드에 registry TLS 신뢰가 구성되어 있는지 확인
# 방법 3: 프라이빗 registry를 사용하는 경우 imagePullSecrets 생성
```

### LLM API 401

```bash
# API Key가 올바르게 구성되어 있는지 확인
kubectl get secret k8ops-credentials -n k8ops-system -o jsonpath='{.data.api-key}' | base64 -d

# 또는 Dashboard → Settings에서 Provider 재구성
```

---

## 업그레이드

```bash
# 새 이미지 빌드 및 푸시
docker buildx build --platform linux/amd64 \
  --build-arg VERSION=v2.0.0 \
  -t registry.iot2.win/k8ops:v2.0.0 \
  -t registry.iot2.win/k8ops:latest \
  --push .

# 롤링 업데이트 (Deployment 모드)
kubectl set image deployment/k8ops \
  k8ops=registry.iot2.win/k8ops:v2.0.0 \
  -n k8ops-system

# 또는 overlay의 newTag 수정 후 재적용
kubectl apply -k config/deploy/overlays/local

# DaemonSet 모드
kubectl set image daemonset/k8ops \
  k8ops=registry.iot2.win/k8ops:v2.0.0 \
  -n k8ops-system
```

---

## 데이터 백업 및 복구

### SQLite 자동 백업 (CronJob)

k8ops는 사용자, 감사 로그, 세션 데이터를 저장하기 위해 SQLite를 사용합니다. 시간별 자동 백업을 권장합니다:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: k8ops-backup
  namespace: k8ops-system
spec:
  schedule: "0 * * * *"  # 매시 정각
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
              # 최근 24개 백업 보관
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

### 수동 백업

```bash
# Pod에서 데이터베이스 복사
kubectl cp k8ops-system/<pod-name>:/data/k8ops.db ./k8ops-backup-$(date +%Y%m%d).db

# 또는 sqlite3 온라인 백업 사용 (쓰기 중단 없음)
kubectl exec -n k8ops-system <pod-name> -- sqlite3 /data/k8ops.db ".backup /data/k8ops-backup.db"
kubectl cp k8ops-system/<pod-name>:/data/k8ops-backup.db ./k8ops-backup.db
```

### 복구

```bash
# k8ops 중지
kubectl scale deployment k8ops -n k8ops-system --replicas=0

# 데이터베이스 복구
kubectl cp ./k8ops-backup.db k8ops-system/<pod-name>:/data/k8ops.db

# 재시작
kubectl scale deployment k8ops -n k8ops-system --replicas=1
```

---

## 고가용성 (HA) 배포

### 단일 노드 모드 (기본값, 개발/소규모 클러스터에 적합)

- 1 replica + SQLite + PVC
- Pod 재시작 시 서비스가 잠시 중단됨 (~10s)
- 50명 미만 사용자 팀에 적합

### 다중 복제본 HA (프로덕션 권장)

SQLite 대신 MySQL/PostgreSQL을 사용하여 다중 복제본을 지원합니다:

1. **데이터베이스를 MySQL로 전환**:

```yaml
# overlay ConfigMap에서 설정
configMapGenerator:
- name: k8ops-config
  literals:
  - DB_DRIVER=mysql
  - DB_DSN=k8ops:password@tcp(mysql:3306)/k8ops?charset=utf8mb4&parseTime=True
```

2. **다중 복제본 + leader election**:

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

3. **공유 스토리지**: MySQL은 별도 PVC를 사용하며, k8ops Pod는 상태 비저장

### 용량 계획

| 규모 | 사용자 수 | 리소스 권장사항 | 데이터베이스 |
|------|--------|----------|--------|
| 소규모 | < 20 | 1 pod, 500m CPU / 512Mi | SQLite |
| 중규모 | 20-100 | 2 pods, 1 CPU / 1Gi each | MySQL |
| 대규모 | 100+ | 3+ pods, 2 CPU / 2Gi each | MySQL + 읽기/쓰기 분리 |

---

## CI/CD 파이프라인 및 릴리스 관리

### 원클릭 배포 스크립트

k8ops는 사전 검사, 빌드, 릴리스, 상태 확인 및 자동 롤백을 포함한 자동화 배포 스크립트를 제공합니다:

```bash
# 새 버전 배포 (자동 사전 검사 + 빌드 + 릴리스 + 상태 확인)
./scripts/deploy.sh v14.36

# 배포 프로세스:
# 1. 사전 검사: go build + go vet + go test + gofmt
# 2. 빌드: Docker buildx + registry 푸시
# 3. 릴리스: kubectl set image + change-cause annotation
# 4. 검증: Pod Ready + HTTP 200 (120s 타임아웃)
# 5. 롤백: 상태 확인 실패 시 이전 버전으로 자동 롤백
```

### 빠른 롤백

```bash
# 이전 버전으로 롤백
./scripts/rollback.sh

# 특정 revision으로 롤백
./scripts/rollback.sh 58

# 특정 버전 번호로 롤백
./scripts/rollback.sh v14.30
```

### 릴리스 기록 추적

각 배포 시 change-cause annotation이 자동으로 기록됩니다:

```bash
# 릴리스 기록 보기
kubectl rollout history daemonset/k8ops -n k8ops-system

# 특정 revision 상세 보기
kubectl rollout history daemonset/k8ops -n k8ops-system --revision=55
```

### CI 파이프라인 (GitHub Actions)

| 워크플로우 | 트리거 조건 | 내용 |
|--------|----------|------|
| `ci.yml` — push/PR to main | 코드 커밋 | test + vet + lint + govulncheck + Docker build |
| `release.yml` — tag v* | 버전 태그 | 전체 테스트 + GoReleaser + Docker multi-arch + 자동 Release Notes |

### 이미지 관리

| 태그 | 설명 |
|------|------|
| `registry.iot2.win/k8ops:v14.XX` | 특정 버전 |
| `registry.iot2.win/k8ops:latest` | 최신 안정 버전 |
| `ghcr.io/<org>/k8ops:v14.XX` | GHCR 미러 (CI 게시) |

### 이미지 최적화

- 베이스 이미지: `gcr.io/distroless/static-debian12:nonroot` (shell 없음, 패키지 관리자 없음)
- 다단계 빌드: Go builder + distroless runtime
- BuildKit 캐시: `--mount=type=cache`로 CI 빌드 가속
- 바이너리 최적화: `-trimpath -ldflags="-s -w"`로 크기 감소

| 버전 | 이미지 크기 |
|------|----------|
| v14.30 (alpine) | 31.8 MB |
| v14.35 (distroless) | 28.6 MB |

### 고가용성 구성

#### PodDisruptionBudget (PDB)

노드 유지보수 중 최소 1개 Pod가 사용 가능하도록 보장:

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

Dashboard가 Ingress Controller의 트래픽만 수용하도록 제한:

- Ingress: kube-system 네임스페이스만 9090 (dashboard) 접근 가능
- Ingress: monitoring 네임스페이스만 8080 (metrics) 접근 가능
- Egress: DNS (53), HTTPS (443), K8s API (6443) 허용

#### PriorityClass

k8ops는 `system-cluster-critical` 우선순위를 사용하여 리소스 압박 상황에서도 축출되지 않도록 합니다.

#### 롤링 업데이트 전략

| 모드 | maxUnavailable | maxSurge | 설명 |
|------|---------------|----------|------|
| DaemonSet | 1 | - | 한 번에 1개 노드씩 업데이트 |
| Deployment | 0 | 1 | 새 Pod를 먼저 시작한 후 이전 Pod 삭제 |

#### 리소스 할당량

| 모드 | CPU Request | CPU Limit | Mem Request | Mem Limit |
|------|-------------|-----------|-------------|-----------|
| DaemonSet | 100m | 1 | 128Mi | 1Gi |
| Deployment | 500m | 2 | 512Mi | 2Gi |

#### 상태 확인 및 라이프사이클 관리

k8ops는 신뢰성을 보장하기 위해 3단계 프로브를 사용합니다:

| 프로브 | 경로 | 역할 | 매개변수 |
|------|------|------|------|
| **startupProbe** | `/healthz` | 시작 완료 대기 (느린 시작으로 인한 liveness 종료 방지) | failureThreshold: 30, period: 5s (최대 150s 대기) |
| **livenessProbe** | `/healthz` | 생존 확인 (실패 시 Pod 재시작) | period: 20s, failureThreshold: 3, timeout: 5s |
| **readinessProbe** | `/readyz` | 준비 확인 (실패 시 Service Endpoints에서 제거) | period: 10s, failureThreshold: 3, timeout: 5s |

**정상 종료 (Graceful Shutdown):**

```yaml
lifecycle:
  preStop:
    exec:
      command: ["/manager", "--pre-stop"]
# --pre-stop는 5s 대기, Ingress Controller가 로드 밸런서에서 해당 Pod를 제거할 때까지 대기
# 그 후 kubelet이 SIGTERM을 보내 dashboard 정상 종료 (SSE 연결 드레이닝) 트리거
# terminationGracePeriodSeconds: 30로 충분한 완료 시간 보장
```

종료 프로세스:
1. kubelet이 `preStop` 실행 → sleep 5s (연결 드레이닝)
2. kubelet이 SIGTERM 전송 → Go 시그널 핸들러가 정상 종료 시작
3. Dashboard HTTP 서버가 새 요청 수락 중지
4. SSE 연결 드레이닝 (10s 타임아웃)
5. Controller Manager 정상 종료
6. 프로세스 종료
