# k8ops — Kubernetes AI Operations Operator

<div align="center">

**문제를 진단하고 자동으로 복구하며 AI를 활용해 클러스터를 최적화하는 Kubernetes AIOps 오퍼레이터입니다.**

[![GitHub release](https://img.shields.io/github/v/release/topcheer/k8ops?style=flat-square)](https://github.com/topcheer/k8ops/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/topcheer/k8ops/ci.yml?branch=main&style=flat-square&label=CI)](https://github.com/topcheer/k8ops/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/topcheer/k8ops?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![Docker](https://img.shields.io/badge/Docker-ghcr.io-2496ED?style=flat-square&logo=docker)](https://github.com/topcheer/k8ops/pkgs/container/k8ops)
[![Built with ggcode](https://img.shields.io/badge/Built%20with-ggcode-6C43BC?style=flat-square)](https://github.com/topcheer/ggcode)

</div>

---

**언어：** [English](../../README.md) | [中文](../zh-CN/README.md) | [日本語](../ja/README.md) | [한국어](README.md) | [Español](../es/README.md) | [Français](../fr/README.md) | [Deutsch](../de/README.md)

---

## 주요 기능

### AI 기반 운영
- **지능형 진단** — 문제 설명을 제출하면 도구 기반 추론(kubectl describe, logs, events, metrics)을 활용한 AI 기반 근본 원인 분석을 제공합니다
- **자동 복구** — AI가 안전한 복구 작업을 제안하고 (승인 시) 실행합니다: 파드 재시작, 디플로이먼트 스케일링, 리소스 정리
- **최적화 제안** — 리소스 사용량, HPA/PDB 격차, 비용 절감 기회에 대한 지속적인 분석
- **스트리밍 채팅** — 추론 과정, 도구 호출 투명성, diff 기반 결과 렌더링을 갖춘 실시간 SSE 스트리밍

### 엔터프라이즈 보안
- **다중 프로바이더 인증** — 로컬(bcrypt), LDAP(구성 가능한 TLS 검증), OIDC(GitHub, Google, GitLab, Keycloak, Okta, Auth0, Microsoft)
- **RBAC** — admin/operator/viewer 역할과 네임스페이스 범위 권한을 갖춘 역할 기반 접근 제어
- **OIDC CSRF 보호** — 프로바이더별 상태 쿠키와 `ConstantTimeCompare` 검증
- **CORS 허용 목록** — 오리진 기반 허용 목록(자격 증명과 함께 와일드카드 사용 안 함), `Vary: Origin` 헤더
- **감사 로깅** — 모든 AI 작업, 도구 실행, LLM 호출이 구조화된 감사 이벤트로 기록됩니다
- **JWT 영속성** — 서명된 JWT 시크릿이 K8s Secret에 저장되며 선택적 폴백 지원
- **속도 제한** — 무차별 대입 공격 방지를 위한 로그인 엔드포인트의 토큰 버킷 속도 제한기
- **보안 헤더** — X-Content-Type-Options, X-Frame-Options, HSTS, CSP

### 운영 및 안정성
- **우아한 종료** — SSE 드레인, SQLite WAL 플러시, 컨트롤러 중지를 포함한 SIGTERM/SIGINT 처리
- **대화 TTL** — 유휴 채팅 세션 자동 정리(30분 타임아웃, 최대 1000개 대화)
- **서킷 브레이커** — 구성 가능한 재시도, 백오프, 서킷 브레이킹을 갖춘 복원력 있는 LLM 호출
- **Prometheus 메트릭** — 클러스터 상태 게이지, 대화 카운터, 도구 실행 메트릭

### 배포
- **Kustomize** — 프로덕션 준비 기본값을 갖춘 베이스 + 오버레이 배포
- **내장 웹 UI** — 단일 바이너리, 외부 프론트엔드 의존성 불필요
- **SQLite + K8s CRD** — 경량 영속성, 외부 데이터베이스 불필요
- **PVC 영속성** — 파드 재시작 후에도 데이터 보존

---

## 아키텍처

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

상세한 컴포넌트 문서는 [docs/ARCHITECTURE.md](../../docs/ARCHITECTURE.md)를 참조하세요.

---

## 빠른 시작

### 사전 요구 사항
- Kubernetes 1.24+ (k3s / k8s / EKS / GKE / AKS)
- kubectl 구성 완료
- LLM API 키 (OpenAI, DeepSeek, ZAI 또는 OpenAI 호환 프로바이더)

### 1. Kubernetes에 배포

**옵션 A: Deployment 모드 (권장)**

```bash
# 한 번의 명령으로 — 네임스페이스, RBAC, 시크릿, 인그레스, TLS 포함
kubectl apply -k config/deploy/overlays/local

# 또는 직접 오버레이 생성
cp -r config/deploy/overlays/local config/deploy/overlays/myorg
# myorg/kustomization.yaml 편집: 도메인, 레지스트리, CORS 설정
kubectl apply -k config/deploy/overlays/myorg
```

**옵션 B: DaemonSet 모드 (노드별 진단)**

```bash
kubectl apply -f config/daemonset-local.yaml
```

**옵션 C: install.sh (대화형)**

```bash
./install.sh install    # 배포
./install.sh status     # 상태 확인
./install.sh uninstall  # 제거
```

상세한 배포 가이드는 [docs/DEPLOYMENT.md](../../docs/DEPLOYMENT.md)를 참조하세요.

### 2. LLM 프로바이더 구성

```bash
# 대시보드에서: Settings 탭 → 프로바이더 유형, API 키, 모델 입력
# 또는 오버레이 ConfigMap의 환경 변수를 통해:

configMapGenerator:
- name: k8ops-config
  literals:
  - PROVIDER_TYPE=openai
  - PROVIDER_MODEL=gpt-4o
  - PROVIDER_ENDPOINT=https://api.openai.com/v1

# Secret을 통한 API 키:
secretGenerator:
- name: k8ops-credentials
  literals:
  - api-key=sk-your-key-here
```

### 3. 대시보드 접속

```bash
# 인그레스 경유 (구성된 경우)
# https://<your-domain> 열기  (예: https://k8ops.iot2.win)

# 또는 포트 포워딩
kubectl port-forward svc/k8ops-dashboard 9090:9090 -n k8ops-system
# http://localhost:9090 열기
# 기본 로그인: admin / admin (비밀번호 변경 프롬프트 표시)
```

### 4. 진단 실행

```bash
# kubectl 경유 (CRD)
kubectl apply -f examples/diagnostic.yaml

# CLI 경유
go run ./cmd/k8ops diagnose --problem "pods in production keep CrashLoopBackOff"

# 웹 대시보드 채팅 인터페이스 경유
```

---

## 설정

모든 설정은 ConfigMap/Secret을 통해 이루어집니다 (Kustomize 오버레이로 관리). 작동 예제는 [config/deploy/overlays/local/kustomization.yaml](../../config/deploy/overlays/local/kustomization.yaml)을 참조하세요.

### 핵심
| 변수 | 기본값 | 설명 |
|----------|---------|-------------|
| `PROVIDER_TYPE` | `openai` | LLM 프로바이더 유형 |
| `PROVIDER_MODEL` | `gpt-4o` | 모델 이름 |
| `PROVIDER_ENDPOINT` | `https://api.openai.com/v1` | LLM 프로바이더 기본 URL |
| `AIOPS_API_KEY` | (필수) | LLM API 키 (Secret에서 가져옴) |

### 보안
| 변수 | 기본값 | 설명 |
|----------|---------|-------------|
| `AUTH_JWT_SECRET` | (자동 생성) | JWT 서명 시크릿 (K8s Secret에 영속화) |
| `CORS_ALLOWED_ORIGINS` | (빈 값) | 쉼표로 구분된 허용 오리진 |
| `LDAP_SERVER` | (빈 값) | LDAP 서버 URL |
| `LDAP_SKIP_TLS_VERIFY` | `false` | LDAP TLS 인증서 검증 건너뛰기 |
| `OIDC_ISSUER` | (빈 값) | OIDC 발급자 URL |

### 알림
| 변수 | 기본값 | 설명 |
|----------|---------|-------------|
| `SLACK_WEBHOOK_URL` | (빈 값) | 알림용 Slack 수신 웹훅 |

### AI / 채팅
| 변수 | 기본값 | 설명 |
|----------|---------|-------------|
| `MAX_STEPS` | `15` | 요청당 최대 에이전트 추론 단계 |
| `CONVERSATION_TTL` | `30m` | 유휴 대화 타임아웃 |
| `MAX_CONVERSATIONS` | `1000` | 최대 동시 대화 수 |

---

## API

대시보드는 `http://<host>:9090/api/`에서 REST API를 제공합니다. 주요 엔드포인트:

| 메서드 | 경로 | 설명 | 인증 |
|--------|------|-------------|------|
| GET | `/api/health` | 헬스 체크 | 공개 |
| GET | `/api/version` | 빌드 버전 | 공개 |
| GET | `/api/cluster/overview` | 클러스터 요약 | Viewer+ |
| GET | `/api/cluster/nodes` | 노드 목록 + 상태 | Viewer+ |
| GET | `/api/cluster/pods` | 파드 목록 및 상태 | Viewer+ |
| POST | `/api/chat/stream` | AI 채팅 (SSE 스트리밍) | Viewer+ |
| GET | `/api/resources/{type}` | K8s 리소스 조회 | Viewer+ |
| POST | `/api/auth/login` | 로컬/LDAP 로그인 | 공개 |
| GET | `/api/auth/status` | 인증 구성 + 프로바이더 | 공개 |
| GET | `/api/auth/providers` | 인증 프로바이더 목록 | Admin |
| GET/POST | `/api/rbac/users` | 사용자 관리 | Admin |
| GET/POST | `/api/rbac/roles` | 역할 관리 | Admin |

전체 API 참조는 [docs/API.md](../../docs/API.md)를 확인하세요.

---

## 개발

### 사전 요구 사항
- Go 1.22+
- kubectl (통합 테스트용)
- Kubernetes 클러스터 접근 권한 (컨트롤러 테스트용)

### 빌드 및 테스트

```bash
# 매니저 바이너리 빌드
make build

# 모든 테스트 실행
make test

# 레이스 디텍터와 함께 테스트 실행
go test -race -count=1 ./internal/...

# CRD 생성
make manifests

# Docker 이미지 빌드
make docker-build IMG=ghcr.io/topcheer/k8ops:latest
```

### 프로젝트 구조

```
k8ops/
├── api/v1alpha1/           # CRD 타입 정의
├── cmd/
│   ├── manager/            # 오퍼레이터 진입점
│   └── k8ops/              # CLI 도구
├── config/
│   ├── crd/                # CRD 매니페스트
│   ├── deploy/             # Kustomize 배포 (베이스 + 오버레이)
│   │   ├── base/           # 네임스페이스, SA, RBAC, Deployment, Service, Ingress
│   │   └── overlays/
│   │       ├── local/      # 로컬 네트워크 오버레이 (레지스트리, 도메인, CORS)
│   │       └── prod/       # 프로덕션 오버레이 템플릿
│   └── daemonset/          # DaemonSet 매니페스트 (노드별 배포)
├── internal/
│   ├── agent/              # AI 에이전트 (추론 + 도구 호출)
│   ├── audit/              # 구조화된 감사 로깅
│   ├── auth/               # 인증 (Local/LDAP/OIDC) + RBAC
│   ├── chat/               # 대화 관리를 갖춘 채팅 엔진
│   ├── collector/          # 클러스터 이벤트 수집기
│   ├── controller/         # CRD 컨트롤러 (진단/최적화/복구)
│   ├── dashboard/          # 웹 UI + REST API
│   │   └── web/            # 내장 프론트엔드 (HTML/JS/CSS)
│   ├── memory/             # 대화 메모리 저장소
│   ├── metrics/            # Prometheus 메트릭
│   ├── provider/           # LLM 프로바이더 인터페이스
│   ├── providermanager/    # 다중 프로바이더 관리
│   ├── resilience/         # 서킷 브레이커 + 속도 제한기
│   ├── safety/             # 안전 검사기 (드라이런 검증)
│   └── tools/              # K8s 및 호스트 도구 (kubectl, exec 등)
├── docs/                   # 아키텍처, API, 보안, 배포 문서
├── install.sh              # 원클릭 설치/제거 스크립트
├── .env.example            # 환경 변수 참조
└── examples/               # 예제 CRD 매니페스트
```

개발 가이드라인은 [CONTRIBUTING.md](../../CONTRIBUTING.md)를 참조하세요.

---

## 로컬 개발

Kubernetes 배포 없이 워크스테이션에서 직접 k8ops를 실행하세요:

```bash
# 빌드
go build -o k8ops-manager ./cmd/manager/

# 실행
AIOPS_API_KEY=your-key ./k8ops-manager \
  --leader-elect=false \
  --dashboard-address=:9090 \
  --auth-db-path=/tmp/k8ops.db
```

바이너리가 자동으로 kubeconfig(`~/.kube/config`)를 감지하므로, 모든 K8s 데이터는 연결된 클러스터에서 가져옵니다. 자세한 내용은 [docs/LOCAL_RUN.md](../../docs/LOCAL_RUN.md)를 참조하세요.

---

## 문서

| 문서 | 설명 |
|----------|-------------|
| [docs/USER_GUIDE.md](../../docs/USER_GUIDE.md) | 종합 사용자 매뉴얼 (15가지 기능 전체) |
| [docs/ARCHITECTURE.md](../../docs/ARCHITECTURE.md) | 시스템 아키텍처 및 컴포넌트 설계 |
| [docs/DEPLOYMENT.md](../../docs/DEPLOYMENT.md) | 배포 가이드 (Deployment / DaemonSet / Helm) |
| [docs/LOCAL_RUN.md](../../docs/LOCAL_RUN.md) | 로컬에서 k8ops 바이너리 실행 (K8s 배포 불필요) |
| [docs/API.md](../../docs/API.md) | REST API 참조 |
| [docs/SECURITY.md](../../docs/SECURITY.md) | 보안 정책 및 RBAC 모델 |
| [CHANGELOG.md](../../CHANGELOG.md) | 릴리스 기록 (v0.1.0 → v14.1) |

---

## 보안

전체 보안 정책은 [SECURITY.md](../../SECURITY.md)를 참조하세요. 다음 내용을 포함합니다:
- 인증 방법 및 구성
- RBAC 모델 및 네임스페이스 범위 지정
- 보고된 취약점 처리

---

## 변경 이력

[CHANGELOG.md](../../CHANGELOG.md)를 참조하세요.

---

## 라이선스

GNU Affero General Public License v3.0 (AGPL-3.0). [LICENSE](../../LICENSE)를 참조하세요.
