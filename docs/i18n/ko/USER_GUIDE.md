# k8ops 사용자 매뉴얼

> 설치부터 숙련까지, 모든 기능을 다루는 상세 사용 가이드입니다.

---

## 목차

1. [빠른 시작](#1-빠른-시작)
2. [클러스터 개요](#2-클러스터-개요)
3. [AI Chat — 지능형 어시스턴트](#3-ai-chat--지능형-어시스턴트)
4. [진단 및 복구](#4-진단-및-복구)
5. [최적화 제안](#5-최적화-제안)
6. [비용 분석 (FinOps)](#6-비용-분석-finops)
7. [클러스터 토폴로지 시각화](#7-클러스터-토폴로지-시각화)
8. [노드 및 Pod 관리](#8-노드-및-pod-관리)
9. [이벤트 스트림 및 알림](#9-이벤트-스트림-및-알림)
10. [리소스 브라우저 및 YAML 에디터](#10-리소스-브라우저-및-yaml-에디터)
11. [RBAC 접근 제어](#11-rbac-접근-제어)
12. [감사 로그](#12-감사-로그)
13. [설정 및 구성](#13-설정-및-구성)
14. [키보드 단축키](#14-키보드-단축키)
15. [테마 전환](#15-테마-전환)
16. [용량 계획](#16-용량-계획)
17. [HPA 시각화](#17-hpa-시각화)
18. [컨테이너 이미지 인벤토리](#18-컨테이너-이미지-인벤토리)
19. [네임스페이스 리소스 순위](#19-네임스페이스-리소스-순위)
20. [보안 컴플라이언스](#20-보안-컴플라이언스)
21. [시스템 관리](#21-시스템-관리)
22. [운영 진단 API](#22-운영-진단-apiv1461)

---

## 1. 빠른 시작

### 최초 로그인

1. 브라우저에서 k8ops 주소에 접속 (예: `https://k8ops.iot2.win` 또는 `http://localhost:9090`)
2. 기본 계정: `admin` / `admin`
3. 최초 로그인 시 비밀번호 변경을 요구합니다

### 페이지 레이아웃

```
┌─────────┬───────────────────────────────┐
│         │  [Namespace ▼]  [🔔]  [☀/☽]  │  ← 상단 바
│ Sidebar ├───────────────────────────────┤
│         │                                │
│ Overview│       Content Area             │  ← 콘텐츠 영역
│ Diagnose│                                │
│ Nodes   │                                │
│ Pods    │                                │
│ ...     │                                │
└─────────┴───────────────────────────────┘
```

### Ctrl+K 커맨드 팔레트

언제든지 `Ctrl+K`(Mac: `Cmd+K`)로 전역 커맨드 팔레트를 엽니다:

- `nodes` 입력 → 노드 페이지로 이동
- `chat` 입력 → AI Chat 열기
- `cost` 입력 → 비용 분석 보기
- 방향키로 선택, Enter로 확인, Esc로 닫기

---

## 2. 클러스터 개요

Overview 페이지는 클러스터 전체 상태를 표시합니다.

### 통계 카드

| 카드 | 의미 |
|------|------|
| Nodes | 클러스터 노드 총 수 / Ready 수 |
| Pods | 실행 중인 Pod 수 / 총 수 |
| CPU | 클러스터 전체 CPU 사용률 |
| Memory | 클러스터 전체 메모리 사용률 |
| Warnings | 현재 Warning 이벤트 수 |

### Sparkline 트렌드 그래프

각 카드 하단에는 SVG 미니 꺾은선 그래프가 있으며, 최근 30분간의 트렌드 변화를 표시합니다.

### Namespace 전환

상단 바 왼쪽의 드롭다운 셀렉터로 namespace 스코프를 전환할 수 있습니다. 전환 후 Pods, Events, Nodes 등의 페이지에 영향을 줍니다. 선택은 localStorage에 영속화됩니다.

---

## 3. AI Chat — 지능형 어시스턴트

사이드바 하단의 Chat 버튼을 클릭하거나 `Ctrl+K`로 `chat`을 입력하여 엽니다.

### 기본 사용법

입력 상자에 질문을 입력하면 AI가 다음을 수행합니다:

1. 자연어 의도 이해
2. 적절한 K8s 도구를 자동으로 호출
3. 분석 결과를 스트리밍으로 반환

### 쿼리 예시

```
# 리소스 확인
default 네임스페이스의 pod 확인
CPU 사용률이 높은 노드는?

# 장애 진단
nginx-deployment의 pod가 CrashLoopBackOff인 이유는?
클러스터에 이상이 있나요?

# 최적화 제안
리소스 사용 현황을 분석해 주세요
복제본 수를 줄일 수 있는 pod는?
```

### 도구 호출 투명성

AI가 도구 호출을 실행할 때 접이식 Thinking 패널이 표시됩니다:

- 클릭하여 펼치면 각 도구 호출의 매개변수와 반환 결과를 확인할 수 있습니다
- JSON 포맷으로 표시, 검색 기능 지원

### 진단 제안 카드

AI가 kubectl 명령 실행을 제안할 때 코드 블록 아래에 다음이 표시됩니다:

- **▶ Run in Chat** — 명령을 입력 상자에 로드하여 전송 및 실행하기 쉽게 합니다
- **📋 Copy** — 명령을 클립보드에 복사

### 세션 관리

- **New** — 새 세션 생성
- **좌측 세션 목록** — 클릭하여 과거 세션으로 전환
- 세션은 자동으로 요약 및 압축됩니다 (20k token 초과 시 자동으로 트리거)

### Markdown 렌더링

Chat은 다음을 지원합니다:
- 코드 블록 (구문 강조 및 복사 버튼 포함)
- 테이블
- 목록, 굵게, 기울임
- 링크 (http/https/mailto 프로토콜만)

---

## 4. 진단 및 복구

### 진단 트리거

**방법 1: 웹 인터페이스**

1. Diagnostics 페이지로 이동
2. "New Diagnostic" 클릭
3. 문제 설명 입력 (예: "production 네임스페이스의 API 응답이 느림")
4. 제출 후 AI가 자동으로 분석

**방법 2: AI Chat**

Chat에서 직접 문제를 설명하면 AI가 자동으로 진단 흐름을 실행합니다.

**방법 3: CRD**

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

**방법 4: CLI**

```bash
k8ops diagnose --problem "pods in production keep CrashLoopBackOff"
```

### 진단 결과

각 진단 보고서에는 다음이 포함됩니다:

- **Root Cause** — AI가 분석한 근본 원인
- **Evidence** — 분석을 뒷받침하는 로그, 이벤트, 메트릭 데이터
- **Recommendations** — 권장되는 복구 액션
- **Severity** — 심각도 (Info / Warning / Critical)

### 자동 복구 (Remediation)

AI가 생성한 복구 계획은 수동 승인이 필요합니다:

1. Remediations 페이지로 이동
2. 승인 대기 중인 복구 계획 확인
3. **Approve**를 클릭하여 실행하거나 **Reject**로 거부
4. 모든 작업은 감사 로그에 기록됩니다

---

## 5. 최적화 제안

Optimizations 페이지는 클러스터 리소스에 대한 AI의 최적화 제안을 표시합니다.

### 제안 유형

| 유형 | 설명 |
|------|------|
| Resource Rightsizing | CPU/Memory requests 및 limits 조정 제안 |
| HPA Gap | 수평 오토스케일링 구성이 누락된 Deployment |
| PDB Gap | PodDisruptionBudget이 누락된 워크로드 |
| Cost Saving | 절감 가능한 비용 (유휴 리소스, 과잉 복제본 등) |

### 작업

- 제안을 클릭하여 상세 보기
- 바로 Apply하거나 무시 가능

---

## 6. 비용 분석 (FinOps)

Cost 페이지는 클러스터 비용 가시성을 제공합니다.

### 기능

- **네임스페이스 비용 집계** — namespace별 리소스 소비 및 예상 비용 표시
- **리소스 사용률** — CPU/Memory 실제 사용량 vs 할당량
- **Rightsizing 제안** — 과도하게 할당된 리소스 조정 제안
- **유휴 리소스** — 장기 미사용 PV, LoadBalancer, 탄력적 IP 등

---

## 7. 클러스터 토폴로지 시각화

Topology 페이지는 노드와 Pod의 관계를 SVG 그래픽으로 표시합니다.

### 시각 요소

| 요소 | 의미 |
|------|------|
| 녹색 테두리 | Ready 노드 |
| 빨간색 테두리 | NotReady 노드 |
| 노드 테두리 내 진행률 바 | CPU (상) / MEM (하) 사용률 |
| Pod 녹색 점 | Running |
| Pod 노란색 점 | Pending |
| Pod 빨간색 점 | Failed |
| Pod 깜빡이는 테두리 | CrashLoop (restarts > 3) |

### 인터랙션

- **Pod 클릭** — 해당 Pod의 로그 뷰어 열기
- **하단 통계** — Ready/NotReady 노드 수, Pod 상태 요약

---

## 8. 노드 및 Pod 관리

### Nodes 페이지

- 노드 목록 테이블: 이름, 역할, 상태, CPU, 메모리, Pod 수
- 각 열에서 검색 필터 지원
- 노드 이름 클릭 시 상세 정보와 해당 노드의 모든 Pod 표시

### Pods 페이지

- Pod 목록 테이블: 이름, 네임스페이스, 상태, 재시작 횟수, 노드, 경과 시간
- 네임스페이스 필터 및 실시간 검색 지원

### Pod 로그 뷰어

Pod 행을 클릭하면 로그 뷰어가 열립니다:

- **실시간 스트리밍** — SSE 푸시로 로그 실시간 업데이트
- **로그 레벨 하이라이트** — ERROR (빨강), WARN (노랑), DEBUG (회색)
- **검색 필터** — 키워드 입력으로 로그 행 필터
- **자동 스크롤** — 새 로그 도착 시 자동으로 최하단으로 스크롤 (일시정지 가능)
- **다운로드** — 현재 로그를 파일로 내보내기

---

## 9. 이벤트 스트림 및 알림

### Events 페이지

K8s 클러스터 이벤트를 표시합니다. 다음을 지원:

- 실시간 검색 필터
- Warning 이벤트 빨간색 하이라이트
- 네임스페이스별 필터링

### 실시간 이벤트 스트림

Events 페이지 우측에 Live Events 패널이 있습니다:

- **Go Live** 클릭하여 SSE 실시간 푸시 활성화
- 새 이벤트는 파란색 NEW 배지 애니메이션과 함께 표시
- 삭제된 이벤트는 빨간색 DEL 배지
- Warning 이벤트는 자동으로 빨간색 하이라이트

### 알림 센터

상단 바 우측의 종 아이콘:

- 알림이 있으면 빨간색 숫자 배지 + 펄스 애니메이션 표시
- 클릭하여 드롭다운 패널 확장
- 최근 Warning 이벤트와 NotReady 노드 표시
- 60초마다 자동 새로고침

---

## 10. 리소스 브라우저 및 YAML 에디터

### Resources 페이지

클러스터의 모든 K8s 리소스를 탐색:

- API Group / Resource Type별로 그룹화
- 리소스 이름 클릭하여 YAML 정의 보기
- 네임스페이스 다중 선택 필터 지원

### YAML 뷰어

아무 리소스나 클릭하면 YAML 오버레이가 열립니다:

- 포맷된 전체 YAML 표시
- **Copy** 버튼으로 원클릭 복사

### YAML 에디터

YAML 뷰어에서 **Edit** 버튼을 클릭하면 편집 모드로 전환됩니다:

1. YAML 콘텐츠가 편집 가능한 textarea로 전환됩니다
2. 수정 후 **Apply**를 클릭하여 제출
3. 백엔드는 server-side apply (kubectl apply 시맨틱)를 사용
4. 성공 시 녹색 알림, 실패 시 빨간색 에러 메시지 표시

---

## 11. RBAC 접근 제어

RBAC 페이지 (admin 권한 필요)에서 사용자와 역할을 관리합니다.

### 사용자 관리

- **사용자 생성** — 사용자 이름, 비밀번호, 역할, 네임스페이스 스코프
- **사용자 편집** — 역할 수정, 활성화/비활성화
- **사용자 삭제**

### 역할

| 역할 | 권한 |
|------|------|
| admin | 전체 클러스터 읽기/쓰기, 사용자 관리 가능 |
| operator | 대부분의 리소스 읽기/쓰기, RBAC/Secrets 관리 불가 |
| viewer | 읽기 전용 접근 |

### 네임스페이스 스코프

각 사용자는 특정 네임스페이스에 바인딩할 수 있으며, 해당 범위 내의 리소스에만 접근할 수 있습니다 (K8s impersonation으로 구현).

---

## 12. 감사 로그

Audit 페이지는 모든 AI 작업의 감사 기록을 표시합니다.

### 기능

- **Severity 필터** — 드롭다운에서 Info / Warning / Error / Critical 선택
- **실시간 검색** — 키워드 입력으로 필터
- **통계 카드** — Total / Successful / Failed / Critical / Warnings
- **테이블** — 시간, 심각도, 액션, 대상 리소스, 작업자, 성공/실패, 소요 시간

### 감사 범위

다음 모든 작업이 기록됩니다:

- AI 도구 호출 (kubectl get/describe/logs 등)
- AI가 시작한 복구 작업
- LLM API 호출
- 사용자 로그인/로그아웃
- 리소스 수정

---

## 13. 설정 및 구성

Settings 페이지에서 AI Provider와 인증을 구성합니다.

### AI Provider 구성

| 필드 | 설명 |
|------|------|
| Provider Type | openai / deepseek / zai / anthropic |
| Model | gpt-4o / deepseek-chat / glm-4-plus 등 |
| Endpoint | LLM API 주소 (비워두면 기본값 사용) |
| API Key | LLM API 키 |

### 인증 구성

- **Local** — 내장 사용자 시스템 (기본값)
- **LDAP** — 엔터프라이즈 LDAP/AD 통합
- **OIDC** — GitHub / Google / Keycloak 등

---

## 14. 키보드 단축키

| 단축키 | 기능 |
|--------|------|
| `Ctrl+K` / `Cmd+K` | 커맨드 팔레트 열기 |
| `Esc` | 커맨드 팔레트 / 팝업 닫기 |
| `↓` / `↑` | 커맨드 팔레트에서 선택 |
| `Enter` | 커맨드 팔레트에서 확인 |

---

## 15. 테마 전환

사이드바 우측 상단의 달/태양 버튼을 클릭하여 다크/라이트 테마를 전환합니다. 선택은 localStorage에 영속화되며, 페이지 새로고침 후에도 유지됩니다.

---

## 부록

### 관련 문서

- [아키텍처 설계](ARCHITECTURE.md)
- [배포 가이드](DEPLOYMENT.md)
- [로컬 실행](LOCAL_RUN.md)
- [API 참조](API.md)
- [보안 정책](SECURITY.md)

### 자주 묻는 질문

**Q: Chat이 응답하지 않나요?**
A: Settings → Provider 구성이 올바른지, API Key가 유효한지 확인하세요.

**Q: 일부 네임스페이스가 보이지 않나요?**
A: 현재 사용자의 RBAC 역할이 네임스페이스 접근 범위를 제한하고 있을 수 있습니다. 관리자에게 연락하여 조정하세요.

**Q: Pod 로그 뷰어가 비어 있나요?**
A: Pod가 방금 시작하여 로그가 없거나, 로그 권한이 없을 수 있습니다. RBAC 구성을 확인하세요.

**Q: AI가 제안하는 명령이 안전한가요?**
A: 모든 AI 제안 작업은 먼저 Safety Checker의 dry-run 검증을 거치며, 복구 작업은 수동 승인이 필요합니다.

---

## 16. 용량 계획

### 스토리지 용량 모니터링

**경로:** Dashboard → Capacity 탭

클러스터의 모든 PVC (PersistentVolumeClaim)의 스토리지 상태를 표시:

| 지표 | 설명 |
|------|------|
| Total PVCs | 클러스터 내 PVC 총 수 |
| Bound | PV에 바인딩된 PVC 수 |
| Pending | 바인딩 대기 중인 PVC |
| Total Capacity | 모든 PVC의 총 용량 |
| Requested | 모든 PVC가 요청한 총량 |

### 노드 용량 분석

Capacity 페이지는 각 노드의 리소스 사용률도 표시합니다:

- **CPU 사용률**: 요청된 CPU / 할당 가능 CPU (색상 코딩: <60% 녹색, 60-80% 노란색, >80% 빨간색)
- **메모리 사용률**: 요청된 메모리 / 할당 가능 메모리
- **Pod 밀도**: 실행 중인 Pod 수 / 최대 Pod 수 제한
- **스케일아웃 제안**: 노드 리소스가 80%를 초과하면 자동으로 스케일아웃 제안 생성

### 클러스터 수준 요약

| 지표 | 설명 |
|------|------|
| Cluster CPU Utilization | 전체 클러스터 CPU 요청/할당 가능 비율 |
| Cluster Mem Utilization | 전체 클러스터 메모리 요청/할당 가능 비율 |
| Total CPU Allocatable | 전체 클러스터 할당 가능 CPU 총량 |
| Total CPU Requested | 전체 클러스터 요청된 CPU 총량 |

---

## 17. HPA 시각화

**경로:** Dashboard → HPA 탭

모든 HorizontalPodAutoscaler의 오토스케일링 상태를 표시:

### 기능

- **복제본 스케일 바**: 현재 복제본 수, 기대 복제본 수, 최소/최대 범위 시각화
- **메트릭 사용률 바**: CPU/메모리 현재 사용률 vs 목표값 (녹색/노란색/빨간색)
- **스케일링 상태 표시**: 현재 복제본 수 ≠ 기대 복제본 수일 때 "SCALING" 배지 표시
- **요약 카드**: HPA 총 수, 스케일링 중인 수, 현재/기대 복제본 총 수

### 지원 메트릭 유형

| 유형 | 설명 |
|------|------|
| Resource | CPU/메모리 사용률 백분율 |
| Pods | 커스텀 Pod 메트릭 (예: QPS) |
| External | 외부 메트릭 (예: SQS 대기열 길이) |
| ContainerResource | 컨테이너 수준 리소스 메트릭 |

---

## 18. 컨테이너 이미지 인벤토리

**경로:** Dashboard → Images 탭

클러스터에서 사용 중인 모든 컨테이너 이미지를 표시:

| 지표 | 설명 |
|------|------|
| Unique Images | 중복 제거한 이미지 총 수 |
| Using :latest | `:latest` 태그를 사용하는 이미지 수 (프로덕션에는 권장하지 않음) |
| No Limits | 리소스 limits가 설정되지 않은 이미지 수 |
| No Requests | 리소스 requests가 설정되지 않은 이미지 수 |
| Registries | 사용 중인 이미지 레지스트리 수 |

### 보안 모범 사례

- `:latest` 태그 사용을 피하세요 — 고정 버전 번호를 사용하여 재현 가능한 배포를 보장
- 모든 컨테이너에 CPU/메모리 limits를 설정하세요 — 리소스 고갈 방지
- 모든 컨테이너에 CPU/메모리 requests를 설정하세요 — 스케줄러의 정확한 할당 보장

---

## 19. 네임스페이스 리소스 순위

**경로:** Dashboard → Namespaces 탭

CPU 소비량 순으로 모든 네임스페이스의 리소스 사용 현황을 나열:

### 기능

- **리소스 집계**: 각 namespace의 CPU/메모리 requests + limits, Pod 수, PVC 스토리지량
- **클러스터 점유율**: CPU/메모리 요청의 클러스터 할당 가능 총량에 대한 비율 (시각적 진행률 바 포함)
- **검색 필터**: 특정 namespace를 빠르게 찾기
- **상세 드릴다운**: 아무 namespace 클릭하여 ResourceQuota 사용 현황, LimitRange 구성, 최근 Warning 이벤트 보기

---

## 20. 보안 컴플라이언스

### CIS Benchmark 컴플라이언스 스캔

**경로:** Dashboard → Compliance 탭

CIS Kubernetes Benchmark 검사를 실행합니다. 다음 카테고리를 다룹니다:

| 카테고리 | 검사 항목 |
|------|--------|
| RBAC | cluster-admin 바인딩 범위, 와일드카드 ClusterRole, 기본 SA 사용 |
| Pod Security | 권한 있는 컨테이너, hostNetwork/hostPID/hostIPC, hostPath 볼륨, root 사용자, 리소스 limits |
| Network | NetworkPolicy 커버리지율 |
| Secrets | Secret 관리 건전성 |

### 컴플라이언스 보고서 다운로드

"Download Report" 버튼을 클릭하여 전체 컴플라이언스 보고서 (.txt 형식)를 다운로드할 수 있습니다. 내용:

- 컴플라이언스 점수 (백분율)
- 각 검사 항목의 상태 (PASS/WARN/FAIL)
- 수정 제안 (WARN/FAIL 항목에 대해)

### 감사 이벤트 검색

**경로:** API → `GET /api/audit/events`

다차원 필터링으로 감사 로그 검색:

| 매개변수 | 설명 |
|------|------|
| `actor` | 사용자 이름으로 필터 |
| `action` | 작업 유형으로 필터 (예: delete, scale, exec) |
| `q` | 전체 텍스트 검색 |
| `severity` | 심각도로 필터 |
| `from`/`to` | 시간 범위 (RFC3339 형식) |

### CSV 내보내기

`GET /api/audit/export` — 감사 로그를 CSV 형식으로 내보냅니다. SIEM 시스템에 가져와 컴플라이언스 분석이 가능합니다.

---

## 21. 시스템 관리

### 시스템 정보

`GET /api/system/info`가 런타임 정보를 제공합니다:

- 버전 번호, Go 버전, 실행 플랫폼
- 메모리 사용량 (Alloc/Sys/GC cycles/Heap objects)
- Goroutine 수
- 서비스 가동 시간
- 감사 로그 크기 및 이벤트 수

### 로그 관리

| API | 기능 |
|-----|------|
| `POST /api/system/log/rotate` | 감사 로그 수동 로테이션 (admin) |
| `POST /api/system/log/cleanup` | 30일 이상 경과한 로테이션 파일 정리 (admin) |

### 로그 레벨 구성

환경 변수 `LOG_LEVEL`로 구성 (debug/info/warn/error):

```bash
kubectl set env daemonset/k8ops -n k8ops-system LOG_LEVEL=debug
kubectl rollout restart daemonset/k8ops -n k8ops-system
```

### 백업 관리

| API | 기능 |
|-----|------|
| `GET /api/system/backup` | 모든 백업 파일 나열 |
| `POST /api/system/backup` | 데이터베이스 백업 생성 |
| `DELETE /api/system/backup?name=X` | 지정된 백업 삭제 |
| `POST /api/system/backup/restore?name=X` | 백업에서 데이터베이스 복원 |

### API 성능 모니터링

`GET /api/system/performance`가 각 API 엔드포인트의 레이턴시 통계를 제공합니다:

- **p50/p95/p99** 백분위수 레이턴시
- 평균 및 최대 레이턴시
- 에러율 및 요청 총 수

---

## 22. 운영 진단 API (v14.61+)

### Network Policy 감사

`GET /api/security/network-policies`가 클러스터의 NetworkPolicy 커버리지를 감사합니다:

- NetworkPolicy가 없는 네임스페이스 감지 (기본적으로 완전 개방)
- 느슨한 정책 식별 (0.0.0.0/0 인바운드/아웃바운드)
- 심각도별 분류: critical / warning / info
- 각 발견 사항에는 상세한 설명과 수정 제안이 포함됨

### Pod 재시작 진단

`GET /api/diagnostics/restarts`가 Pod 재시작 패턴과 근본 원인을 진단합니다:

- 재시작 패턴 분류: crash-loop / occasional / post-deploy
- 종료 원인 추출: OOMKilled / Error / 종료 코드
- 대기 상태 식별: CrashLoopBackOff / ImagePullBackOff
- 컨테이너별 개별 진단 정보

### 배포 Rollout 상태

`GET /api/deployments/rollout`이 모든 워크로드의 rollout 건강 상태를 추적합니다:

- Deployment / StatefulSet / DaemonSet 커버
- 7가지 상태: complete / in-progress / stalled / degraded / paused / failed / scaled-to-zero
- ProgressDeadlineExceeded 및 ReplicaFailure 감지
- 상태별 필터링 지원: `?status=failed`

### 리소스 낭비 감지

`GET /api/resources/waste`가 비용 절감을 위한 낭비 및 고립된 리소스를 스캔합니다:

- 6가지 낭비 유형: 죽은 서비스, 미사용 PVC, 고립된 ConfigMap/Secret, 빈 네임스페이스, 미바인딩 PV
- 비용 위험 평가: low / moderate / high
- 각 항목에 심각도, 경과 시간, 정리 제안 포함
- 시스템 리소스를 지능적으로 필터링 (kube-system, SA token, Helm release)

### 확장 병목 감지

`GET /api/scaling/bottlenecks`가 수평 확장을 제한하는 요소를 식별합니다:

- 7가지 병목 유형: 노드 스케줄링, 노드 압력, 쿼터 제한, HPA 정체, PDB 차단, 스토리지 고갈
- 클러스터 용량 요약: 노드 수, CPU/메모리, Pod 용량, 확장 여유
- 각 항목에 영향 수준과 수정 제안 포함

### RBAC 권한 위험 분석

`GET /api/security/rbac-risk`가 클러스터의 모든 RBAC 바인딩의 보안 위험을 분석합니다:

- 0-100 점수 시스템, 고위험 바인딩 자동 식별
- 5단계 위험 등급: critical / high / elevated / moderate / low
- 감지 항목: cluster-admin 바인딩, 권한 승격 (escalate/bind/impersonate), 와일드카드 권한 (verbs/resources: *), 클러스터 범위 쓰기 작업, 민감한 리소스 접근 (secrets/pods/exec)
- 각 항목에 상세한 점수 내역과 수정 제안 포함 (최소 권한 원칙)
- 네임스페이스별 필터링 지원: `?namespace=default`

### CronJob 실행 건강 모니터링

`GET /api/operations/cronjobs/health`가 모든 CronJob의 실행 건전성을 모니터링합니다:

- 5단계 건강 상태: healthy / warning / failing / suspended / no-runs
- 연속 실패 감지 (3회 이상 = failing), 성공률 50% 미만, 일시정지된 스케줄, 미실행 감지
- OwnerReferences를 통해 CronJob과 하위 Job 연결
- 다음 예상 실행 시간 계산
- 네임스페이스별 필터링 지원: `?namespace=production`

### Service & Endpoint 네트워크 건강 모니터링

`GET /api/networking/health`가 모든 Service와 Ingress의 네트워크 연결성을 스캔합니다:

- 5단계 Service 건강 상태: healthy / degraded / no-endpoints / misconfigured / external
- 셀렉터 불일치 (label mismatch), 모든 엔드포인트 사용 불가, 부분적 저하, LoadBalancer IP 대기 감지
- Ingress 백엔드 검증: 백엔드 Service 존재 여부, 사용 가능한 엔드포인트 유무
- Pod 셀렉터 매칭을 교차 참조하여 근본 원인 분석 제공
- 네임스페이스별 필터링 지원: `?namespace=default`

### PV/PVC 스토리지 건강 모니터링

`GET /api/storage/health`가 모든 PVC/PV의 스토리지 건전성을 스캔합니다:

- 6단계 PVC 건강 상태: bound / pending / lost / failed / orphaned / near-capacity
- Pending 진단: 스토리지 클래스 없음, WaitForFirstConsumer 바인딩 모드, provisioner 로그 확인
- 고립된 PVC 감지: 바인딩되었으나 1일 이상 Pod에 사용되지 않음 (용량 낭비)
- PV 문제: Released (수동 정리 필요), Failed (회수 실패), 오래된 Available (>7일)
- 스토리지 클래스 분포: 기본 클래스 표시, provisioner, reclaim policy, volume expansion 지원
- 네임스페이스별 필터링 지원: `?namespace=default`

### ServiceAccount 보안 감사

`GET /api/security/service-accounts`가 클러스터의 모든 ServiceAccount의 보안 위험을 종합적으로 감사합니다:

- 0-100 위험 점수 시스템, 고위험 SA 자동 식별
- 5단계 심각도: critical / high / elevated / moderate / low
- 감지 항목: 미사용 SA (>7일), cluster-admin 바인딩 (critical), 기본 SA의 Pod 사용, 불필요한 token 자동 마운트, 오래된 SA (>30일 권한 있으나 미사용), 레거시 장기 유효 token secret
- 각 항목에 상세한 보안 위험 설명과 수정 제안 포함
- 네임스페이스별 필터링 지원: `?namespace=default`

### SLO/SLA 에러 버짓 추적

`GET /api/operations/slo`가 다중 윈도우 다중 버닝율 알고리즘 기반의 SLO/SLA 달성 현황을 추적합니다:

- 5개 시간 윈도우: 5분, 1시간, 6시간, 24시간, 7일
- 가용성 백분율 및 에러 버짓 잔여량/소비율
- 다중 윈도우 버닝율 감지 (fast: 5m+1h, slow: 6h+24h)
- P50/P95/P99 레이턴시 백분위수 및 SLO 목표
- 3단계 상태: meeting (달성) / at-risk (위험) / violated (위반)
- 네임스페이스별 필터링 지원: `?namespace=production`

### ResourceQuota 및 LimitRange 모니터링

`GET /api/resources/quota`가 모든 네임스페이스의 쿼터 사용률과 LimitRange 제약을 스캔합니다:

- 4단계 쿼터 상태: ok (<70%) / warning (70-85%) / critical (85-100%) / exceeded (>100%)
- 네임스페이스별 CPU/메모리/Pod/ConfigMap/Secret/스토리지 쿼터 사용률
- 쿼터 보호가 없는 네임스페이스 식별
- LimitRange 기본/최소/최대 제약 분석
- Top 소비자 순위
- 네임스페이스별 필터링 지원: `?namespace=default`

### 배포 구성 감사

`GET /api/deployments/audit`가 모든 워크로드의 구성 베스트 프랙티스 위반을 감사합니다:

- 8개 검사 카테고리: revision-history / image-policy / resources / probes / security-context / update-strategy / lifecycle / config-drift
- 각 항목에 심각도 (critical/warning/info), 구체적인 문제 설명과 실행 가능한 수정 제안 포함
- 건강 점수 0 (완벽)부터 100 (최악)
- 집계된 Top Findings로 클러스터 전체의 가장 일반적인 문제 표시
- 네임스페이스 및 심각도별 필터링 지원: `?namespace=default&severity=critical`

### 스케줄링 건강 및 리소스 단편화 분석

`GET /api/scheduling/health`가 클러스터 스케줄링 건강과 리소스 사용률을 분석합니다:

- 노드별 스케줄 가능성 (cordon/taint/pressure conditions)과 리소스 가용량
- Pending Pod 진단: FailedScheduling 이벤트 원인 파싱 (CPU/메모리 부족, taint 불일치, nodeSelector 충돌, 볼륨 바인딩 실패 등)
- 최대 스케줄 가능 Pod 계산 (현재 배포 가능한 최대 Pod 크기)
- 유효 용량 vs 이론적 용량 (스케줄 불가 노드로 인한 용량 손실)
- 리소스 단편화 분석 (분산된 여유 용량)
- 초대형 Pod 감지 (단일 노드 용량을 초과하는 요청)
- 24h 퇴거(Eviction) 이력 (원인 포함)
- 건강 점수 0-100 (가중 페널티)
- 실행 가능한 수정 제안
- 네임스페이스별 필터링 지원: `?namespace=default`

### Pod 보안 태세 스캔

`GET /api/security/pods?namespace=xxx&severity=critical`이 모든 실행 중 Pod의 실시간 보안 태세를 감사합니다:

- 15개 검사 카테고리가 권한 있는 컨테이너, 호스트 접근 (network/PID/IPC), HostPath 마운트, 위험한 capabilities, root 실행, 권한 승격 등을 커버
- Pod별 위험 점수 0-100 (critical=25점/warning=8점/info=2점)
- 검사 유형 및 네임스페이스별 집계 통계
- 네임스페이스 및 심각도별 필터링 지원

### 이벤트 스톰 및 캐스케이드 장애 감지

`GET /api/operations/event-storm?namespace=xxx`가 클러스터 Warning 이벤트를 분석합니다:

- 4단계 스톰 심각도: critical (>50) / high (>20) / medium (>10) / low (>5)
- 플래핑 리소스 감지 (동일 리소스 동일 원인 3회 이상 반복, 플랩 빈도 포함)
- 네임스페이스 및 이벤트 원인별 집계
- 폭발 반경 평가 (영향받는 리소스 수)
- 실행 가능한 조사 제안
- 네임스페이스별 필터링 지원: `?namespace=kube-system`

### 리소스 의존성 그래프 및 영향 범위 분석

`GET /api/dependencies?kind=Deployment&name=xxx&namespace=xxx`가 워크로드의 완전한 의존성 그래프를 추적합니다:

- 정방향 의존성: ConfigMap, Secret, PVC, ServiceAccount
- 역방향 의존성: Service (label selector), Ingress, NetworkPolicy, HPA, 구성을 공유하는 다른 Pod
- 영향 범위 평가: blastRadius 점수 및 위험 등급
- 변경 전 영향 평가에 사용하여 캐스케이드 장애 회피

### 토폴로지 분산 컴플라이언스 검사

`GET /api/topology/spread?namespace=xxx&domain=topology.kubernetes.io/zone`가 Pod의 토폴로지 분산 컴플라이언스를 검사합니다:

- 4단계 워크로드 상태: balanced / skewed / no-constraint / single-replica
- 워크로드별 토폴로지 도메인 분포 및 편차 분석
- 토폴로지 제약이 누락된 다중 복제본 워크로드 감지
- 토폴로지 라벨이 누락된 노드 식별
- 단일 도메인 클러스터 힌트
- 네임스페이스 및 토폴로지 도메인 키별 필터링 지원

### Secret 로테이션 및 수명 주기 감사

`GET /api/security/secrets/rotation?namespace=xxx`가 모든 Secret의 수명 주기를 감사합니다:

- 경과 시간 추적: stale (>90d) / very stale (>180d)
- 미사용 Secret 감지 (어떤 Pod에서도 참조되지 않음)
- TLS 인증서 만료 감지 (인증서 파싱, 만료 및 <30d 만료 감지)
- Docker registry Secret, 레거시 SA token 추적
- 민감한 이름 감지 (password/key/token/credential)
- Secret별 위험 등급, 클러스터 로테이션 점수 0-100
- 네임스페이스별 필터링 지원

### 헬스 프로브 유효성 감사

`GET /api/operations/probes?namespace=xxx`가 프로브 구성을 감사합니다:

- 8개 검사 카테고리: 프로브 누락, 너무 공격적, 타임아웃이 너무 짧음, 부적절한 임계값 등
- 워크로드별 위험 점수, 클러스터 유효성 점수 (0-100)
- 집계된 Top 문제 통계
- 실행 가능한 제안

### 워크로드 노후화 추적

`GET /api/product/staleness?namespace=xxx`가 배포 노후화를 추적합니다:

- 5단계 노후화 분류: fresh/recent/stale/very-stale/ancient
- 이미지 태그 분석: :latest, digest, no-tag
- 경과 시간 분포 버킷, 네임스페이스 통계
- 클러스터 신선도 점수 (0-100)

### 리소스 오버커밋 및 압력 분석

`GET /api/scalability/overcommit?namespace=xxx`가 리소스 오버커밋을 분석합니다:

- 노드별 CPU/메모리 request 및 limit 오버커밋 비율
- 압력 점수 0-100 및 위험 등급
- limits/requests가 없는 Pod 감지
- 클러스터 안전 점수 0-100
- 네임스페이스별 리소스 소비 명세

### 이미지 보안 및 공급망 분석

`GET /api/security/images?namespace=xxx`가 모든 컨테이너 이미지의 공급망 보안을 스캔합니다:

- Digest 잠금 감지 (@sha256: 불변 참조)
- :latest 태그 감지 (가변, 재현 불가)
- 태그 없는 이미지 감지 (기본 :latest)
- 오래된 버전 태그 감지 (v1, 1.0 — 알려진 CVE를 포함할 수 있음)
- 공개 vs 개인 이미지 레지스트리 분석
- 이미지별 위험 등급, 레지스트리별 통계
- 클러스터 이미지 보안 점수 0-100

### 용량 계획

`GET /api/capacity/planning`이 노드 용량 계획을 제공합니다:

- 노드별 CPU/메모리 요청 vs 할당 가능량
- 잔여 용량 및 스케일아웃 제안
- 리소스 단편화 감지

### 용량 예측

`GET /api/capacity/forecast`가 용량 트렌드를 예측합니다:

- 과거 데이터 기반 리소스 성장 트렌드
- 예상 고갈 시간
- 스케일아웃 제안

### 리소스 효율 분석

`GET /api/efficiency`가 리소스 사용 효율을 분석합니다:

- 과대한 리소스 할당 감지
- 리소스 낭비 식별
- 최적화 제안

### PDB 상태

`GET /api/pdbs`가 Pod Disruption Budget의 상태를 제공합니다:

- PDB 구성 검사
- 허용 중단 수 vs 현재 가용 수
- PDB 차단 감지

### 버전 호환성

`GET /api/compatibility`가 K8s 버전 호환성을 제공합니다:

- API 폐기 검사
- 리소스 버전 호환성
- 업그레이드 영향 평가

### 인증서 만료

`GET /api/certificates/expiry`가 TLS 인증서 만료를 스캔합니다:

- 클러스터 인증서 만료 시간
- 만료 임박 인증서 경고
- 갱신 제안

### Addon 건강

`GET /api/addons/health`가 클러스터 애드온 건강을 검사합니다:

- 핵심 애드온 실행 상태
- 비정상 애드온 감지
- 수정 제안

### 클러스터 건강 점수

`GET /api/operations/health-score`가 모든 클러스터 건강 신호를 하나의 종합 점수로 집약합니다:

- 5개 가중 차원: Node(25%) + Pod(25%) + Workload(20%) + Events(15%) + API Server(15%)
- 총점 0-100, 알파벳 등급 A-F
- 상태: healthy / warning / critical
- 각 차원의 점수, 가중치, 상세 정보
- 클러스터 요약: 노드/Pod/워크로드 수
- 심각도순 Top 문제

### HPA/VPA 리소스 적정 구성 제안

`GET /api/scalability/autoscale-recommendations?namespace=xxx`가 오토스케일링과 리소스 적정화(right-sizing)를 분석합니다:

- HPA가 누락된 다중 복제본 워크로드 감지
- CPU 요청 과대 (>1 core/container)
- 메모리 요청 과대 (>2GB/container)
- HPA 효율 분석 (상한/하한/유휴 도달)
- 워크로드별 현재 vs 권장 리소스 값
- 잠재적 CPU 코어 및 메모리 절감량
- 클러스터 오토스케일링 점수 0-100

### Ingress 및 트래픽 라우팅 건강 모니터링

`GET /api/product/ingress-health?namespace=xxx`가 모든 Ingress의 트래픽 라우팅 건강을 검사합니다:

- 백엔드 Service 존재 여부 및 엔드포인트 준비 상태 검사
- TLS 구성 감지
- IngressClass 유효성 검증
- host+path 충돌 감지
- 라우팅 규칙 없음 감지
- Ingress별 상태 및 클러스터 건강 점수 0-100

### 노드 상태 및 리소스 압력

`GET /api/operations/node-pressure`가 모든 노드의 상태와 리소스 압력을 분석합니다:

- DiskPressure / MemoryPressure / PIDPressure / NetworkUnavailable 감지
- CPU/메모리/Pod 사용률 vs 할당 가능량
- 노드별 위험 등급 (critical/high/medium/low)
- 클러스터 압력 점수 0-100

### PVC 바인딩 및 스토리지 성능

`GET /api/scalability/pvc-analysis?namespace=xxx`가 스토리지 바인딩 건전성을 분석합니다:

- Stuck PVC 근본 원인 감지 (>5분 pending)
- 바인딩 시간 측정 및 느린 바인딩 감지 (>30s)
- Lost PVC 감지
- StorageClass별 통계 및 프로비저너 분석
- 클러스터 스토리지 건강 점수 0-100

### Namespace 거버넌스 및 수명 주기

`GET /api/product/namespaces/lifecycle`가 네임스페이스 거버넌스를 감사합니다:

- ResourceQuota / LimitRange / NetworkPolicy 커버리지율
- 전용 ServiceAccount 감지 (최소 권한)
- 필수 라벨 검사 (app, team, env, owner)
- 네임스페이스 수명 주기 (active / stale / terminating)
- 클러스터 거버넌스 점수 0-100

### RBAC 유효 권한 및 권한 승격 분석

`GET /api/security/rbac-effective`가 모든 주체의 RBAC 유효 권한을 분석합니다:

- ClusterRoleBindings + RoleBindings를 집약하여 실제 권한 계산
- cluster-admin 등가 감지
- 권한 승격 경로 감지 (RBAC를 수정할 수 있는 주체)
- 와일드카드 (*) 권한 감지
- Secret 읽기 및 Pod exec 접근 분석
- 클러스터 RBAC 보안 점수 0-100

### 컨테이너 OOM Kill 추적

`GET /api/operations/oom-tracker?namespace=xxx`가 컨테이너 OOM 이벤트를 추적합니다:

- OOMKilled 컨테이너 감지 및 근본 원인 분석
- 높은 재시작 횟수 감지 (>=5)
- 누락/과소 메모리 제한 감지
- 제한이 요청보다 대폭 큰 (10배 이상) 노드 압력 위험
- Top OOM 순위 및 네임스페이스별 통계
- 클러스터 OOM 위험 점수 0-100

### 스토리지 용량 고갈 예측

`GET /api/scalability/storage-forecast`가 스토리지 용량을 예측합니다:

- PV별 사용률, 성장률, 고갈 일수 예측
- Longhorn actual-size 어노테이션 지원
- 클러스터 스토리지 고갈 일수 추정
- StorageClass별 통계 및 프로비저너 분석
- 고위험 네임스페이스 순위
- 스토리지 건강 점수 0-100

### DNS 해석 건강 검사

`GET /api/product/dns-health`가 DNS 해석 건전성을 분석합니다:

- CoreDNS Pod 건강 검사 (실행/준비/재시작/버전)
- Corefile 구성 분석 (forwarders, plugins)
- Headless Service 엔드포인트 커버리지 및 NXDOMAIN 위험
- NodeLocal DNS 캐시 감지
- Pod dnsConfig ndots 커버리지 감지
- External-DNS 관리 서비스 디스커버리
- 클러스터 DNS 건강 점수 0-100
