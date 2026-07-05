# k8ops API 레퍼런스

모든 엔드포인트는 dashboard 포트(기본값 `:9090`)에서 서비스됩니다.

**인증:** JWT 쿠키 (`k8ops_token`) 또는 `Authorization: Bearer <token>` 헤더.
**Content-Type:** 모든 POST/PUT 요청에 `application/json`.

## OpenAPI 3.0 사양

k8ops는 OpenAPI 3.0 사양을 자동 생성하여 SDK 자동 생성, API 게이트웨이 통합, Swagger UI 탐색에 사용할 수 있습니다.

| 엔드포인트 | 설명 |
|------|------|
| `GET /api/openapi.json` | 완전한 OpenAPI 3.0 JSON 사양 반환 |
| `GET /api/docs` | 태그별로 그룹화된 API 문서 메타데이터 반환 (spec + tagGroups 포함) |

**사양 가져오기:**
```bash
curl -sk https://k8ops.iot2.win/api/openapi.json -o k8ops-openapi.json
```

**Swagger Editor로 가져오기:**
1. https://editor.swagger.io 열기
2. File → Import File → `k8ops-openapi.json` 선택

**Dashboard에서 탐색:** 사이드바 → API Docs 페이지에서 검색, 필터링, 온라인 테스트를 지원하는 대화형 API 브라우저 제공.

---

## Health & System

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/health` | None | Liveness probe — `{"status":"ok"}` 반환 |
| GET | `/api/version` | None | 빌드 버전, git commit, Go 버전 |

## Cluster

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/cluster/overview` | Required | 클러스터 요약: 노드 수, Pod 수, CPU/메모리 사용량, 경고 (30s 캐시) |
| GET | `/api/nodes` | Required | 리소스 사용량 및 조건이 포함된 모든 노드 목록 (30s 캐시) |
| GET | `/api/nodes/{node}/pods` | Required | 특정 노드에서 실행 중인 Pod |
| GET | `/api/pods` | Required | 모든 네임스페이스의 Pod 목록 (30s 캐시) |
| GET | `/api/pods/{namespace}/{name}/containers` | Required | Pod의 컨테이너 목록 |
| GET | `/api/pods/{namespace}/{name}/logs?container=&follow=&tailLines=` | Required | Pod 로그 (`follow=true`로 SSE 스트리밍 지원) |
| GET | `/api/events?namespace=&warning=` | Required | Kubernetes 이벤트, 네임스페이스/경고별 선택적 필터링 |
| GET | `/api/resources?kind=&namespace=` | Required | 일반 리소스 리스터 (Deployments, Services 등) (60s 캐시) |
| GET | `/api/crds?with_counts=true` | Required | Custom Resource Definitions (10min 캐시, 카운트 포함) |
| GET | `/api/crd-resources?group=&version=&resource=&namespace=` | Required | CRD 인스턴스 (60s 캐시) |
| GET | `/api/yaml?namespace=&name=&group=&version=&resource=&kind=` | Required | 모든 Kubernetes 리소스의 YAML 보기 |

## Diagnostics & Remediation

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/diagnostics` | Required | DiagnosticReport CR 목록, 선택적 `?namespace=` 필터 |
| GET | `/api/diagnostics/{namespace}/{name}` | Required | AI 분석이 포함된 진단 상세 |
| GET | `/api/remediations` | Required | Remediation CR 목록, 선택적 `?namespace=` 필터 |
| GET | `/api/optimizations` | Required | Optimization CR 목록, 선택적 `?namespace=` 필터 |

## AI Chat

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/chat` | Required | AI 어시스턴트에 메시지 전송 (SSE 스트리밍 응답) |
| GET | `/api/chat/conversations?id=` | Required | 대화 목록 또는 ID로 단일 대화 조회 |

### POST /api/chat

**요청:**
```json
{
  "message": "Why is my pod crashing?",
  "conversation_id": "optional-existing-id",
  "stream": true
}
```

**응답:** 도구 호출 및 결과가 포함된 AI 분석의 SSE 스트림.

### GET /api/chat/conversations

대화 기록을 반환합니다. 단일 대화를 원하면 `?id=<uuid>`를 전달하세요.

## Provider Management

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/provider/status` | Required | 현재 AI 프로바이더 구성 (마스킹된 API 키) |
| POST | `/api/provider/update` | Required | 런타임에 프로바이더 유형/모델/엔드포인트 업데이트 |
| POST | `/api/provider/reload` | Required | K8opsConfig CRD에서 프로바이더 구성 재로드 |
| GET | `/api/tools` | Required | 등록된 진단 도구 목록 |

## Auth

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/auth/login` | Public | 로컬 로그인 (속도 제한 적용) |
| POST | `/api/auth/logout` | Required | 인증 쿠키 삭제 |
| GET | `/api/auth/me` | Required | 현재 사용자 정보 |
| POST | `/api/auth/change-password` | Required | 본인 비밀번호 변경 |
| GET | `/api/auth/status` | Public | 인증 구성 상태 (auth_enabled, user_count, ldap/oidc 플래그) |
| GET | `/api/auth/provider-presets` | Public | 사용 가능한 OIDC/LDAP 프로바이더 템플릿 |

### POST /api/auth/login

**요청:**
```json
{
  "username": "admin",
  "password": "admin"
}
```

**응답 (200):**
```json
{
  "user": {"id": 1, "username": "admin", "role": "admin", "display_name": "Administrator"},
  "must_change": true,
  "redirect_url": "/"
}
```

`k8ops_token` 쿠키 설정 (HttpOnly, SameSite=Lax, 24h).

**오류 (401):**
```json
{"error": "invalid username or password"}
```

## OIDC

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/auth/oidc/{provider}/login` | Public | OIDC 프로바이더로 리다이렉트 (CSRF state 쿠키 설정) |
| GET | `/api/auth/oidc/{provider}/callback` | Public | OIDC 콜백 (state 검증, 사용자 세션 생성) |

## Auth Provider Management (Admin)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/auth/providers` | Admin | 구성된 인증 프로바이더 목록 |
| POST | `/api/auth/providers` | Admin | 인증 프로바이더 생성 (LDAP/OIDC) |
| GET | `/api/auth/providers/{id}` | Admin | ID로 프로바이더 조회 |
| PUT | `/api/auth/providers/{id}` | Admin | 프로바이더 구성 업데이트 |
| DELETE | `/api/auth/providers/{id}` | Admin | 프로바이더 삭제 |

## User Management (Admin)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/admin/users` | Admin | 모든 사용자 목록 |
| POST | `/api/admin/users` | Admin | 사용자 생성 (기본 역할: viewer, MustChangePwd=true) |
| GET | `/api/admin/users/{id}` | Admin | ID로 사용자 조회 |
| PUT | `/api/admin/users/{id}` | Admin | 사용자 업데이트 (역할, 네임스페이스 등) |
| DELETE | `/api/admin/users/{id}` | Admin | 사용자 삭제 |
| POST | `/api/admin/users/{id}/reset-password` | Admin | 비밀번호 재설정 (MustChangePwd=true 설정) |
| GET | `/api/admin/auth-config` | Admin | 인증 구성 조회 |
| PUT | `/api/admin/auth-config` | Admin | 인증 구성 업데이트 |

## API Keys

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/auth/api-keys` | Required | 본인 API 키 목록 |
| POST | `/api/auth/api-keys` | Required | API 키 생성 |
| DELETE | `/api/auth/api-keys/{id}` | Required | API 키 폐기 |

## RBAC Management (Admin)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/rbac/clusterroles` | Admin | 클러스터 역할 목록 |
| GET | `/api/rbac/clusterroles/{name}` | Admin | 이름으로 클러스터 역할 조회 |
| DELETE | `/api/rbac/clusterroles/{name}` | Admin | 클러스터 역할 삭제 |
| GET | `/api/rbac/roles?namespace=` | Admin | 네임스페이스 역할 목록 |
| GET | `/api/rbac/roles/{namespace}/{name}` | Admin | 네임스페이스 역할 조회 |
| DELETE | `/api/rbac/roles/{namespace}/{name}` | Admin | 네임스페이스 역할 삭제 |
| GET | `/api/rbac/rolebindings?namespace=` | Admin | 역할 바인딩 목록 |
| GET | `/api/rbac/rolebindings/{namespace}/{name}` | Admin | 역할 바인딩 조회 |
| DELETE | `/api/rbac/rolebindings/{namespace}/{name}` | Admin | 역할 바인딩 삭제 |
| GET | `/api/rbac/api-resources` | Admin | Kubernetes API 리소스 유형 목록 |
| GET | `/api/rbac/namespaces` | Admin | 모든 네임스페이스 목록 |
| GET | `/api/rbac/role-mapping?role=&kind=&name=&namespace=` | Admin | 역할-주체 매핑 보기 |
| GET | `/api/rbac/role-defs` | Admin | k8ops 커스텀 역할 정의 목록 |
| GET | `/api/rbac/subjects?kind=&namespace=` | Admin | 주체 목록 (users/groups/service accounts) |

## Audit

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/audit?namespace=&limit=` | Required | 감사 로그 항목 (페이지네이션) |
| GET | `/api/audit/stats` | Required | 감사 통계 요약 |

## Config

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/config` | Required | k8ops 컨트롤러 구성 (프로바이더 유형/모델, 기능) |

## Security Audit

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/security/audit` | Required | 클러스터 보안 스캔 — Pod Security Standards, RBAC, NetworkPolicy 커버리지, Secret 보안 점검 |
| GET | `/api/security/health` | Required | 플랫폼 보안 상태 확인 — 인증/TLS/K8s API 연결성 |

### GET /api/security/audit

전체 클러스터를 스캔하여 심각도별로 정렬된 보안 발견 목록을 반환합니다 (critical > high > medium > low > info).

**점검 항목:**
- **Pod Security:** 권한 있는 컨테이너, root 실행, 권한 상승, 위험한 capabilities, hostPath/hostNetwork
- **RBAC:** cluster-admin 바인딩, 기본 SA 사용
- **Network:** NetworkPolicy가 없는 네임스페이스
- **Secrets:** Docker registry 키 로테이션 권장
- **Resources:** resource limits가 없는 컨테이너

**응답 예시:**
```json
{
  "summary": {"critical": 0, "high": 2, "medium": 5, "low": 8, "info": 3, "total": 18},
  "findings": [
    {
      "severity": "high",
      "category": "Pod Security",
      "resource": "default/pod/nginx/container/app",
      "namespace": "default",
      "detail": "Container \"app\" allows privilege escalation",
      "fix": "Set allowPrivilegeEscalation: false in securityContext"
    }
  ],
  "scannedAt": "2025-01-15T10:30:00Z"
}
```

## Write Operations

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/scale` | Required | deployment/statefulset 스케일 조정 |
| POST | `/api/pod/delete` | Required | 단일 Pod 삭제 |
| POST | `/api/rollout/restart` | Required | deployment/daemonset/statefulset 롤링 재시작 |
| POST | `/api/node/cordon` | Required | 노드 차단/복구 |
| POST | `/api/yaml/apply` | Required | YAML 적용 (kubectl apply) |

모든 쓰기 작업은 감사 로그에 기록됩니다.

---

## Error Responses

모든 오류는 JSON을 반환합니다:

```json
{"error": "descriptive error message"}
```

| Code | Meaning |
|------|---------|
| 400 | Bad request (missing/invalid parameters) |
| 401 | Unauthorized (missing/expired/invalid token) |
| 403 | Forbidden (insufficient role) |
| 404 | Resource not found |
| 500 | Internal server error |
| 503 | Service unavailable (AI provider not configured) |

## Roles

| Role | Permissions |
|------|-------------|
| `admin` | 사용자/RBAC/프로바이더 관리를 포함한 전체 접근 |
| `operator` | Dashboard + 진단 + 채팅 (사용자 관리 불가) |
| `viewer` | 읽기 전용 dashboard + 채팅 |
| `ns-admin` | 할당된 네임스페이스 내에서만 관리자 |
| `ns-viewer` | 할당된 네임스페이스 내에서만 viewer |

## 신규 엔드포인트 (v14.48-v14.53)

다음 엔드포인트는 v14.48부터 v14.53 사이에 추가되었으며, OpenAPI 3.0 사양에 포함되어 있습니다.

### 컨테이너 이미지 인벤토리

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/images` | 클러스터의 모든 컨테이너 이미지 인벤토리, 리소스 제한 감사 및 `:latest` 태그 감지 포함 |

**응답 요약 필드:** `totalImages`, `withoutLimits`, `withoutRequests`, `usingLatestTag`, `uniqueRegistries`

### Warning 이벤트 요약

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/events/summary` | Reason별로 모든 Warning 이벤트 집계, 심각도 분류 및 영향받는 네임스페이스 통계 포함 |

### 클러스터 효율성 분석

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/efficiency` | 클러스터 리소스 효율성 분석: 무제한 Pod, 과도한 프로비저닝 컨테이너, 미활용 노드, 효율성 점수 0-100 |

### 보안: Secret 노출 스캔

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/secrets` | 하드코딩된 자격 증명 감지, Secret 로테이션 추적 (90일), 미사용 Secret, 민감한 키 이름 |

### 감사 검색 및 내보내기

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/audit/events` | 감사 이벤트 검색: `actor`, `action`, `q`(전문 검색), `severity`, 날짜 범위 필터 지원 |
| GET | `/api/audit/export` | 감사 이벤트를 CSV 형식으로 내보내기 (SIEM 시스템 가져오기 가능) |

### 백업 관리

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/system/backup` | 모든 백업 파일 목록 (크기, 기간, 유형) |
| POST | `/api/system/backup` | 데이터베이스 백업 생성 (타임스탬프 이름) |
| DELETE | `/api/system/backup?name=X` | 지정된 백업 삭제 (경로 순회 방지) |
| POST | `/api/system/backup/restore?name=X` | 백업에서 데이터베이스 복원 |

### Alertmanager Webhook

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/webhooks/alertmanager` | Prometheus Alertmanager v4 알림 수신, 자동 조사 제안 생성 |
| POST | `/api/webhooks/alertmanager/test` | 테스트 알림 전송으로 수신기 검증 |

**Alertmanager 구성 예시:**
```yaml
receivers:
  - name: k8ops
    webhook_configs:
      - url: http://k8ops.k8ops-system.svc:9090/api/webhooks/alertmanager
        send_resolved: true
```

### 변경 이력

| 버전 | 엔드포인트 | 차원 |
|------|------|------|
| v14.49 | `GET /api/events/summary` | Product |
| v14.50 | 시작 프로브 + preStop | Deployment |
| v14.51 | `POST /api/webhooks/alertmanager` | Operations |
| v14.52 | `GET /api/efficiency` | Scalability |
| v14.53 | `GET /api/security/secrets` | Security |
| v14.54 | OpenAPI 3.0 spec + API.md | Documentation |
| v14.55 | `GET /api/pdbs` `GET /api/compatibility` | Product |
| v14.56 | `GET /api/certificates/expiry` | Operations |
| v14.57 | 정상 종료 draining gate | Deployment |
| v14.58 | `GET /api/addons/health` | Product |
| v14.59 | `GET /api/capacity/forecast` | Scalability |
| v14.60 | OpenAPI spec 보완 + API.md 업데이트 | Documentation |
| v14.61 | `GET /api/security/network-policies` | Security |
| v14.62 | `GET /api/diagnostics/restarts` | Operations |
| v14.63 | `GET /api/deployments/rollout` | Deployment |
| v14.64 | `GET /api/resources/waste` | Product |
| v14.65 | `GET /api/scaling/bottlenecks` | Scalability |

### Pod Disruption Budget 상태 (v14.55+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/pdbs` | 모든 PDB 목록, disruption 상태, 매칭 워크로드, 건강 평가(healthy/at-risk/blocked) 포함, drain 전 안전 점검용 |

### K8s 배포판 호환성 감지 (v14.55+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/compatibility` | 클러스터 배포판 자동 감지 (vanilla/k3s/RKE2/EKS/GKE/AKS/OpenShift/Talos), 버전 호환성, ARM/Windows/GPU 노드 특성 |

### TLS 인증서 만료 스캔 (v14.56+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/certificates/expiry` | 모든 TLS/Opaque Secret의 X.509 인증서 스캔, 만료 시간별 분류(expired/critical/warning/ok), Ingress 리소스 연관 |

### 서버 Drain 상태 (v14.57+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/system/drain-status` | 서버 정상 종료 상태 보고: draining, shutdownInitiated, activeConnections, uptime |

### 애드온 상태 감지 (v14.58+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/addons/health` | 39가지 일반적인 K8s 애드온 비침습적 감지 (12 카테고리: CNI/DNS/Ingress/CertManager/LB/Mesh/Backup/Monitoring/Policy/Storage/GitOps/VM)의 건강 상태 |

### 용량 고갈 예측 (v14.59+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/capacity/forecast` | 성장률 추정을 기반으로 CPU/메모리/Pod/스토리지 용량이 언제 고갈되는지 예측, days-to-exhaustion 및 확장 권장사항 제공 |

### Network Policy 감사 스캔 (v14.61+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/network-policies` | NetworkPolicy 커버리지 감사: NetworkPolicy가 없는 네임스페이스, 관대한 정책(0.0.0.0/0 인/아웃바운드), 부분 커버리지 감지, 심각도별 분류(critical/warning/info) |

**쿼리 매개변수:** `namespace` (선택, 네임스페이스 필터링)

**반환 예시:**
```json
{
  "summary": {
    "totalNamespaces": 27,
    "withoutNetPol": 25,
    "findings": 18,
    "critical": 10,
    "warning": 8
  },
  "namespaces": [...]
}
```

### Pod 재시작 진단 (v14.62+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/diagnostics/restarts` | Pod 재시작 패턴 및 근본 원인 진단: 재시작 동작 분류(crash-loop/occasional/post-deploy), 종료 원인(OOMKilled/Error/종료 코드) 추출, 대기 상태(CrashLoopBackOff/ImagePullBackOff) |

**쿼리 매개변수:** `namespace` (선택)

**진단 모드:**
- **crash-loop**: 단기간 내 대량 재시작
- **occasional**: 장기간에 걸친 소량 재시작
- **post-deploy**: 배포 후 즉시 재시작

### 배포 Rollout 상태 추적 (v14.63+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/deployments/rollout` | 모든 Deployment/StatefulSet/DaemonSet의 rollout 건강 상태 스캔: 7가지 상태(complete/in-progress/stalled/degraded/paused/failed/scaled-to-zero), ProgressDeadlineExceeded, ReplicaFailure, generation 불일치 감지 |

**쿼리 매개변수:**
- `namespace` (선택) — 네임스페이스 필터링
- `status` (선택) — rollout 상태 필터링: `failed`, `degraded`, `stalled`, `in-progress`, `paused`, `scaled-to-zero`, `complete`

**상태 설명:**
| 상태 | 의미 |
|------|------|
| `complete` | 모든 복제본이 업데이트되고 준비됨 |
| `in-progress` | 롤링 업데이트 진행 중 |
| `stalled` | 컨트롤러가 최신 spec을 관찰하지 못함 (generation 불일치) |
| `degraded` | 일부 복제본 사용 불가 |
| `paused` | Deployment가 명시적으로 일시정지됨 |
| `failed` | ProgressDeadlineExceeded, 배포 시간 초과 실패 |
| `scaled-to-zero` | 복제본 수가 0 |

### 리소스 낭비 감지 (v14.64+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/resources/waste` | 비용 절감을 위해 클러스터의 낭비 및 고아 리소스 스캔: 6가지 낭비 유형(dead-service/unused-pvc/orphaned-configmap/orphaned-secret/empty-namespace/unattached-pv), 4단계 심각도(critical/high/medium/low), 비용 위험 평가 |

**쿼리 매개변수:** `namespace` (선택)

**낭비 유형:**
| 카테고리 | 감지 내용 | 기본 심각도 |
|------|---------|-----------|
| `dead-service` | 백엔드 endpoint가 없는 Service (LoadBalancer는 critical) | medium/critical |
| `unused-pvc` | 어떤 Pod도 마운트하지 않은 PVC | high |
| `orphaned-configmap` | 어떤 Pod도 참조하지 않는 ConfigMap | low/medium |
| `orphaned-secret` | 어떤 Pod도 참조하지 않는 Secret (보안 위험) | high |
| `empty-namespace` | 실행 중인 Pod가 없는 네임스페이스 | medium |
| `unattached-pv` | Available 상태의 PV (어떤 PVC에도 바인딩되지 않음) | critical |

**스마트 필터링:** kube-system 네임스페이스, ServiceAccount token Secret, Helm release Secret, 자동 생성된 ConfigMap 자동 제외

### 확장 병목 감지 (v14.65+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/scaling/bottlenecks` | 수평 확장 제한 요소 스캔: 7가지 병목 유형(node-schedulable/node-pressure/resource-quota/hpa-stuck/pdb-blocking/storage-exhaust/image-pull-limit), 4단계 영향도(critical/high/moderate/low), 클러스터 용량 요약 |

**쿼리 매개변수:** `namespace` (선택)

**병목 유형:**
| 카테고리 | 감지 내용 |
|------|---------|
| `node-schedulable` | 차단된 노드, 클러스터 Pod 용량 초과 (>75% 경고 / >90% 심각) |
| `node-pressure` | 메모리, 디스크, PID 압력 상태 |
| `resource-quota` | 네임스페이스 할당량 75%/90% 초과 |
| `hpa-stuck` | HPA가 최대 복제본에 도달하거나 메트릭 누락 |
| `pdb-blocking` | PDB가 자발적 중단을 0회 허용 |
| `storage-exhaust` | 네임스페이스 PVC 요청이 500Gi 초과 |

**클러스터 용량 요약:** 노드 수, CPU/메모리 용량 및 할당 가능량, Pod 용량 및 할당량, 확장 여유 제공

### RBAC 권한 위험 분석 (v14.67+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/rbac-risk` | 모든 RoleBinding/ClusterRoleBinding의 권한 위험 분석, 0-100 점수 시스템, 5단계 위험 등급(critical/high/elevated/moderate/low), cluster-admin 바인딩, 권한 상승, 와일드카드 권한, 민감한 리소스 접근 감지 |

**쿼리 매개변수:** `namespace` (선택)

**위험 점수 규칙:**
| 감지 항목 | 기본 점수 | 추가 점수 |
|--------|--------|--------|
| ClusterRoleBinding + cluster-admin | 100 | - |
| 권한 상승 (escalate/bind/impersonate) | - | +25 |
| 와일드카드 동사 (verbs: *) | - | +25 |
| 와일드카드 리소스 (resources: *) | - | +20 |
| 클러스터 범위 쓰기 작업 | - | +30 |
| 민감한 리소스 접근 (secrets/pods/exec) | - | +15 |

### CronJob 실행 상태 모니터링 (v14.68+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/cronjobs/health` | 모든 CronJob의 실행 건강 모니터링: 성공률, 연속 실패, 일시정지/지연 스케줄링, 실행 이력 없음, 5단계 건강 상태(healthy/warning/failing/suspended/no-runs) |

**쿼리 매개변수:** `namespace` (선택)

**건강 상태:**
| 상태 | 트리거 조건 |
|------|---------|
| `failing` | 연속 3회 이상 실패 |
| `warning` | 1-2회 연속 실패, 또는 성공률 < 50% |
| `suspended` | CronJob이 suspend됨 |
| `no-runs` | 실행 이력이 없음 |
| `healthy` | 최근 모두 성공 |

### Service & Endpoint 상태 모니터링 (v14.69+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/networking/health` | 모든 Service 및 Ingress의 네트워크 건강 스캔: endpoint 없는 서비스, selector 불일치, endpoint 성능 저하, LoadBalancer 대기, Ingress 백엔드 서비스 누락/endpoint 없음, 5단계 건강 상태 |

**쿼리 매개변수:** `namespace` (선택)

**Service 건강 상태:**
| 상태 | 의미 |
|------|------|
| `misconfigured` | selector 불일치 — label과 일치하는 Pod 없음 |
| `no-endpoints` | 모든 endpoint 사용 불가 |
| `degraded` | 일부 endpoint 사용 불가 |
| `external` | ExternalName/LoadBalancer (정보성) |
| `healthy` | 모든 endpoint 정상 |

**Ingress 건강 점검:** 백엔드 Service 존재 여부, 사용 가능한 endpoint 여부 감지, 기본 백엔드 및 규칙 경로 검증

### PV/PVC 스토리지 상태 모니터링 (v14.70+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/storage/health` | 모든 PVC/PV의 스토리지 건강 스캔: Pending PVC 진단, 고아 PVC(바인딩되었으나 1일 이상 Pod 미사용), Lost/Failed PVC, 수동 정리 필요한 Released/Failed PV, 오래된 Available PV 용량 낭비, 6단계 건강 상태 + 스토리지 클래스 분포 분석 |

**쿼리 매개변수:** `namespace` (선택)

**PVC 건강 상태:**
| 상태 | 의미 |
|------|------|
| `failed` | PVC 프로비저닝 실패 |
| `lost` | 기반 PV가 삭제됨 |
| `pending` | 프로비저닝 대기 중 (스토리지 클래스 없음, WaitForFirstConsumer) |
| `near-capacity` | 용량 한계에 근접 |
| `orphaned` | 바인딩되었으나 1일 이상 Pod 미사용 |
| `bound` | 정상 바인딩 |

**PV 문제 감지:** Released PV (수동 정리 필요), Failed PV (회수 실패), 오래된 Available PV (>7일 용량 낭비)

**스토리지 클래스 분석:** 기본 클래스 표시, provisioner, reclaim policy, binding mode, volume expansion 지원, PVC 수량 분포

### ServiceAccount 보안 감사 (v14.72+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/service-accounts` | 모든 ServiceAccount의 보안 상태 종합 감사: 미사용 SA, 기본 SA가 Pod에 사용됨, 불필요한 token 자동 마운트, cluster-admin 바인딩, 클러스터 범위 권한, 오래된 SA, 레거시 장기 토큰 secret |

**쿼리 매개변수:** `namespace` (선택)

**위험 점수:** 0-100 (높을수록 위험), 5단계 심각도: critical / high / elevated / moderate / low

**감지 항목:**
| 감지 항목 | 심각도 | 설명 |
|--------|--------|------|
| 미사용 SA (>7일 Pod 미참조) | moderate | 공격 표면 확대 |
| 기본 SA가 Pod에 사용됨 | elevated | 최소 권한 원칙 위반 |
| cluster-admin 바인딩 | critical | 클러스터 수준 슈퍼 권한 |
| 불필요한 token 자동 마운트 | moderate | 토큰이 필요 없는 SA는 마운트하지 않아야 함 |
| 오래된 SA (>30일 미사용이지만 권한 유지) | high | 좀비 권한 |
| 레거시 장기 토큰 secret (K8s <1.24) | high | 권장되지 않는 장기 토큰 |

### SLO/SLA 오류 예산 추적 (v14.73+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/slo` | 다중 윈도우 다중 연소율 알고리즘 기반 SLO/SLA 가용성 및 오류 예산 추적 |

**쿼리 매개변수:** `namespace` (선택)

**윈도우 구성:** 5m / 1h / 6h / 24h / 7d

**반환 내용:**
| 필드 | 설명 |
|------|------|
| `availability` | 각 윈도우의 가용성 백분율 |
| `errorBudget` | 오류 예산 잔여량 및 소비율 |
| `burnRate` | 다중 윈도우 연소율 (fast: 5m/1h, slow: 6h/24h) |
| `latencySLO` | P50/P95/P99 지연 백분위수 및 목표 |
| `status` | meeting(달성) / at-risk(위험) / violated(위반) |

### ResourceQuota 및 LimitRange 모니터링 (v14.74+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/resources/quota` | 모든 네임스페이스의 ResourceQuota 사용률 및 LimitRange 기본 제약 조건 스캔 |

**쿼리 매개변수:** `namespace` (선택)

**할당량 상태 레벨:**
| 상태 | 사용률 | 설명 |
|------|--------|------|
| `ok` | <70% | 정상 |
| `warning` | 70-85% | 상한에 근접 |
| `critical` | 85-100% | 위험 |
| `exceeded` | >100% | 초과 |
| `no-limit` | — | 할당량 설정 없음 |

**감지 항목:** 네임스페이스별 CPU/메모리/Pod/ConfigMap/Secret/스토리지 할당량 사용률, 할당량 보호 없는 네임스페이스, LimitRange 기본/최소/최대 제약 조건 분석, Top 소비자 순위

### 배포 구성 감사 (v14.75+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/deployments/audit` | 모든 Deployment/StatefulSet/DaemonSet의 구성 모범 사례 위반 감사, 8개 점검 카테고리, 각 항목에 심각도 및 수정 권장사항 포함 |

**쿼리 매개변수:** `namespace` (선택), `severity` (선택: critical / warning / info)

**점검 카테고리:**
| 카테고리 | 점검 항목 |
|------|--------|
| `revision-history` | 수정 이력이 너무 적음 (< 2, 롤백 불가) 또는 너무 많음 (> 20, 리소스 낭비) |
| `image-policy` | `:latest` 태그이지만 pullPolicy가 Always가 아님; 고정 태그이지만 pullPolicy가 Always |
| `resources` | 리소스 limits/requests 누락 |
| `probes` | liveness/readiness/startup 프로브 누락 |
| `security-context` | 권한 있는 컨테이너, root 실행, 쓰기 가능한 루트 파일 시스템, 권한 상승 허용 |
| `update-strategy` | Recreate 전략(중단), OnDelete(수동 Pod 삭제 필요), 파티션 롤링 업데이트 |
| `lifecycle` | terminationGracePeriod가 너무 짧음 (< 10s) 또는 너무 김 (> 300s), preStop 훅 누락 |
| `config-drift` | seccomp profile 누락 |

**건강 점수:** 0(완벽)에서 100(최악), critical=20점/warning=8점/info=2점

### 스케줄링 건강 및 리소스 단편화 분석 (v14.76+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/scheduling/health` | 클러스터 스케줄링 건강, 노드 스케줄 가능성, 리소스 단편화 및 Pending Pod 진단 분석 |

**쿼리 매개변수:** `namespace` (선택)

**반환 내용:**
| 필드 | 설명 |
|------|------|
| `summary` | 노드 통계 (스케줄 가능/불가/차단됨/압력 있음), Pending Pod 수, FailedScheduling 수, 24h 축출 수, 건강 점수 0-100 |
| `nodes` | 노드별 스케줄 가능 상태, 압력 유형, taints, CPU/메모리/Pod 가용량 및 백분율 |
| `pendingPods` | Pending Pod 목록, CPU/메모리 요청, nodeSelector, 파싱된 스케줄링 실패 원인 포함 |
| `largestFittablePod` | 현재 스케줄 가능한 최대 Pod (CPU/메모리/Pod 수량), 최적 노드 |
| `effectiveCapacity` | 이론적 용량 vs 유효 용량 (스케줄 불가 노드로 인한 용량 손실 백분율) |
| `fragmentation` | 리소스 단편화 지표 (평균 CPU/메모리 단편화율, 최악 단편화 노드, 초대형 Pod 감지) |
| `evictions` | 24h 내 축출 기록 (Pod, 노드, 원인) |
| `recommendations` | 실행 가능한 수정 권장사항 |

**스케줄링 실패 원인 파싱:** insufficient-cpu / insufficient-memory / untolerated-taint / node-selector-mismatch / node-affinity-mismatch / pod-affinity-conflict / pod-limit-reached / volume-binding-failure / no-nodes-available

### Pod 보안 태세 스캔 (v14.79+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/pods` | 모든 실행 중인 Pod의 보안 태세 감사: 권한 있는 컨테이너, hostNetwork/hostPID/hostIPC, HostPath 마운트, 위험한 Linux capabilities, root 실행, 권한 상승 허용, 쓰기 가능한 루트 파일 시스템, 보안 컨텍스트 누락, :latest/태그 없는 이미지, digest 미사용, Secret 환경 변수 주입, 리소스 제한 없음, host port 바인딩 |

**쿼리 매개변수:** `namespace` (선택), `severity` (선택: critical / warning / info)

**위험 점수:** 0(안전)에서 100(극히 위험), critical=25점/warning=8점/info=2점

**점검 카테고리:**
| 카테고리 | 심각도 | 설명 |
|------|--------|------|
| `privileged` | critical | 권한 있는 컨테이너 — 전체 호스트 접근 |
| `host-network` | critical | 노드 네트워크 네임스페이스 공유 |
| `host-pid` | critical | 노드의 모든 프로세스 가시 |
| `host-ipc` | critical | IPC 네임스페이스 공유 |
| `host-path` | critical | 노드에서 HostPath 볼륨 마운트 |
| `dangerous-capabilities` | critical | SYS_ADMIN/NET_ADMIN/NET_RAW/SYS_PTRACE/SYS_MODULE/DAC_OVERRIDE/SETUID/SETGID |
| `runs-as-root` | warning | UID 0으로 실행 |
| `privilege-escalation` | warning | 권한 상승 허용 |
| `missing-security-context` | warning | 보안 컨텍스트 누락 |
| `image-latest` | warning | :latest 태그 사용 |
| `image-no-tag` | warning | 태그 없음 (기본 :latest) |
| `host-port` | warning | 호스트 포트 바인딩 |
| `image-no-digest` | info | digest 미사용 |
| `writable-rootfs` | info | 쓰기 가능한 루트 파일 시스템 |
| `secret-env-vars` | info | Secret이 환경 변수로 주입됨 |
| `no-resource-limits` | info | 리소스 제한 없음 |

### 이벤트 스톰 및 연쇄 장애 감지 (v14.80+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/event-storm` | 클러스터 Warning 이벤트 분석, 이벤트 스톰, 연쇄 장애 및 리소스 스래싱 감지. 15min/1h/24h 시간 윈도우의 알림 이벤트 집계, 스톰 심각도 분류, 스래싱 리소스 식별(동일 리소스 동일 원인 3회 이상 반복), 네임스페이스 및 원인별 집계, 실행 가능한 권장사항 제공 |

**쿼리 매개변수:** `namespace` (선택)

**스톰 심각도:**
| 심각도 | 조건 | 설명 |
|--------|------|------|
| `critical` | >50 events/15min | 긴급 조사 필요 |
| `high` | >20 events/15min | 주의 필요 |
| `medium` | >10 events/15min | 추세 모니터링 |
| `low` | >5 events/15min | 정보성 |

**반환 내용:** 스톰 감지 결과, 네임스페이스 알림 순위, Top 이벤트 원인, 스래싱 리소스 목록(스래싱 빈도 포함), 최근 15분 이벤트 타임라인, 영향받은 리소스 수(폭발 반경), 실행 가능한 권장사항

### 리소스 의존성 그래프 및 영향 범위 분석 (v14.81+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/dependencies` | 모든 워크로드(Deployment/StatefulSet/DaemonSet/Pod)의 완전한 의존성 그래프 추적, 변경 영향 범위 평가 |

**쿼리 매개변수:**

| 매개변수 | 필수 | 설명 |
|------|------|------|
| `kind` | 예 | 리소스 유형: Deployment / StatefulSet / DaemonSet / Pod |
| `name` | 예 | 리소스 이름 |
| `namespace` | 아니오 | 네임스페이스 (기본값: default) |

**정방향 의존성(해당 워크로드가 의존하는 것):** ConfigMap, Secret, PVC, ServiceAccount

**역방향 의존성(해당 워크로드에 의존하는 것):**
- Service (label selector로 Pod 매칭)
- Ingress (매칭되는 Service로 라우팅)
- NetworkPolicy (해당 Pod에 적용)
- HPA (해당 워크로드를 대상으로 함)
- ConfigMap/Secret을 공유하는 다른 Pod

**영향 범위 평가:** blastRadius = 정방향 의존성 수 + 역방향 의존성 수, 위험 등급 low(<6) / medium(6-10) / high(11-20) / critical(>20)

### 토폴로지 분산 규정 준수 점검 (v14.82+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/topology/spread` | 토폴로지 도메인(zone/region/node) 내 Pod 분산 분석, topologySpreadConstraints 규정 준수 검증 |

**쿼리 매개변수:** `namespace` (선택), `domain` (선택, 토폴로지 도메인 key, 기본값 `kubernetes.io/hostname`, `topology.kubernetes.io/zone`으로 설정 가능)

**워크로드 상태:**
| 상태 | 의미 |
|------|------|
| `balanced` | 균등 분산 (actualSkew <= maxSkew) |
| `skewed` | 불균등 분산 (actualSkew > maxSkew) |
| `no-constraint` | 다중 복제본이지만 토폴로지 제약 없음 |
| `single-replica` | 단일 복제본 (토폴로지 분산 적용 불가) |

**반환 내용:** 토폴로지 도메인 통계, 워크로드별 도메인 분산(Pod 수/기대값), 실제 편차 vs 최대 편차, 노드별 도메인 라벨 및 Pod 수, 권장사항(제약 추가, 노드 라벨링, 단일 도메인 클러스터 힌트)

### Secret 로테이션 및 라이프사이클 감사 (v14.85+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/secrets/rotation` | 모든 Secret의 로테이션 규정 준수 및 라이프사이클 관리 감사: 기간 추적(stale >90d / very stale >180d), 미사용 Secret 감지(어떤 Pod도 참조하지 않음), TLS 인증서 만료 감지(인증서 파싱), Docker registry Secret 추적, 레거시 ServiceAccount Token 감지, 민감한 이름 감지 |

**쿼리 매개변수:** `namespace` (선택)

**위험 점수:** Secret별 위험 등급(critical / high / medium / low), 클러스터 로테이션 점수 0-100

**점검 카테고리:**
| 점검 항목 | 심각도 | 설명 |
|---------|--------|------|
| TLS 인증서 만료 | critical | 즉시 갱신 |
| Docker Secret >180d 만료 | critical | 만료된 registry 자격 증명 포함 가능 |
| TLS 인증서 <30d 만료 | high | 가능한 빨리 갱신 예약 |
| Stale + 미사용 + 민감한 이름 | high | 보안 위험 |
| Stale Docker Secret | medium | 로테이션 권장 |
| Stale이지만 사용 중 | low | 로테이션 계획 |

### 상태 프로브 유효성 감사 (v14.86+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/probes` | 모든 워크로드의 liveness/readiness/startup 프로브 구성 감사, 부적절한 구성으로 인한 연쇄 재시작, 미준비 Pod로의 트래픽, 시작 실패 등의 문제 감지 |

**쿼리 매개변수:** `namespace` (선택)

**점검 카테고리:**
| 점검 항목 | 심각도 | 설명 |
|---------|--------|------|
| liveness 누락 | warning | 멈춘 컨테이너가 재시작되지 않음 |
| readiness 누락 | warning | 트래픽이 미준비 Pod에 도달 가능 |
| 프로브가 너무 공격적 (period <5s) | warning | API server에 과도한 부하 |
| 타임아웃이 너무 짧음 (<2s) | warning | 지연 스파이크 시 오탐 가능 |
| 실패 임계값이 너무 낮음 (<3) | warning | 일시적 오류에 과민 |
| readiness 간격이 너무 김 (>60s) | info | 준비 감지 지연 |
| liveness 실패 임계값이 너무 높음 (>10) | info | 재시작 복구 지연 |
| 동일한 liveness+readiness | info | 차별화 구성 권장 |

**반환 내용:** 워크로드별 위험 점수, 클러스터 유효성 점수 (0-100), 집계 Top 문제, 실행 가능한 권장사항

### 워크로드 신선도 추적 (v14.87+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/product/staleness` | 모든 워크로드의 배포 신선도 추적, 장기 미업데이트 워크로드, :latest 태그 이미지, digest 미사용 이미지 감지 |

**쿼리 매개변수:** `namespace` (선택)

**신선도 분류:**
| 상태 | 조건 | 설명 |
|------|------|------|
| `fresh` | <7d | 최근 업데이트 |
| `recent` | <30d | 비교적 새로움 |
| `stale` | <90d | 주의 필요 |
| `very-stale` | <180d | 업데이트 권장 |
| `ancient` | >180d | 보안 위험 |

**반환 내용:** 워크로드별 위험 등급, 이미지 태그 분석(:latest / digest / no-tag), 기간 분포 버킷, 네임스페이스 통계, 클러스터 신선도 점수 (0-100), 실행 가능한 권장사항

### 리소스 오버커밋 및 압력 분석 (v14.88+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/scalability/overcommit` | 모든 노드의 CPU 및 메모리 오버커밋 비율 분석, 위험한 over-commit, limits 없는 Pod, 리소스 압력 점수 감지 |

**쿼리 매개변수:** `namespace` (선택)

**노드별 분석:**
| 지표 | 설명 |
|------|------|
| CPU request commit | sum(requests) / allocatable |
| CPU limit commit | sum(limits) / allocatable |
| Mem request/limit commit | 상동 |
| 압력 점수 | 0-100 (가중치 계산) |
| 위험 등급 | safe / moderate / high / critical (>3x) |

**클러스터 지표:** 총 CPU/메모리 오버커밋 비율, 위험 노드 수, limits 없는 Pod 수, 안전 점수 (0-100), 네임스페이스 리소스 소비 상세, 실행 가능한 권장사항

### 이미지 보안 및 공급망 분석 (v14.92+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/images` | 모든 실행 중인 컨테이너 이미지의 공급망 보안 위험 스캔: digest 잠금, :latest 태그, 태그 없는 이미지, 구버전 태그, 공개 vs 프라이빗 이미지 저장소, 알 수 없는 이미지 저장소 |

**쿼리 매개변수:** `namespace` (선택)

**점검 카테고리:**
| 점검 항목 | 위험 점수 | 설명 |
|---------|--------|------|
| 태그 없음 | +25 | 기본 :latest 사용, 버전 불확실 |
| :latest 사용 | +15 | 가변 태그, 재현 불가 |
| digest 미사용 | +10 | 이미지 내용이 조용히 교체될 수 있음 |
| 알 수 없는 저장소 | +10 | 저장소 접두사 없음, 기본 Docker Hub |
| 구버전 태그 | +15 | 알려진 취약점 포함 가능 |
| 공개 저장소 + 미잠금 | +5 | 출처 보증 없음 |

**반환 내용:** 이미지별 위험 등급(critical/high/medium/low), 저장소별 통계, Top 위험 이미지, 클러스터 이미지 보안 점수 (0-100), 실행 가능한 권장사항

### 용량 계획 (v14.50+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/capacity/planning` | 노드 용량 계획 분석: 노드별 CPU/메모리 요청 vs 할당 가능량, 잔여 용량, 확장 권장 시점, 리소스 단편화 감지 |

### 클러스터 건강 점수 집계 (v14.93+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/health-score` | 모든 클러스터 건강 신호를 하나의 종합 점수(0-100, 등급 A-F)로 집계, 5개 가중 차원 결합 |

**5개 가중 차원:**
| 차원 | 가중치 | 점검 내용 |
|------|------|----------|
| Node Health | 25% | 노드 준비 상태 |
| Pod Health | 25% | CrashLoop, Pending, Failed, 높은 재시작 횟수 |
| Workload Health | 20% | Deployment/StatefulSet/DaemonSet 준비 복제본 |
| Event Activity | 15% | 최근 1시간 Warning 이벤트 수 |
| API Server | 15% | API server 실시간 지연 측정 |

**반환 내용:** 총점 0-100, 알파벳 등급 A-F, 상태(healthy/warning/critical), 차원별 점수 상세, 클러스터 요약(노드/Pod/워크로드 수), 심각도순 정렬 Top 문제

### HPA/VPA 리소스 적정 구성 권장 (v14.94+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/scalability/autoscale-recommendations` | 모든 워크로드의 HPA 커버리지 및 리소스 적정 구성 분석, 과도한 프로비저닝, HPA 없는 다중 복제본 워크로드, HPA 효율성 감지 |

**쿼리 매개변수:** `namespace` (선택)

**감지 카테고리:**
| 점검 항목 | 설명 |
|---------|------|
| HPA 없는 다중 복제본 워크로드 | 자동 스케일링 추가 권장 |
| CPU 요청 과다 (>1 core/container) | 높은 신뢰도, 반감 권장 |
| 메모리 요청 과다 (>2GB/container) | right-sizing 권장 |
| HPA가 maxReplicas 도달 | 용량 증설 필요 |
| HPA 유휴 (<20% 사용률) | maxReplicas 감소 권장 |

**반환 내용:** 워크로드별 현재 vs 권장 CPU/메모리 값, 변화율, 신뢰도, 잠재 CPU 코어 및 메모리 절감, HPA 효율성 분석, 클러스터 자동 스케일링 점수 (0-100)

### Ingress 및 트래픽 라우팅 상태 모니터링 (v14.96+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/product/ingress-health` | 모든 Ingress 리소스의 트래픽 라우팅 건강 및 구성 문제 분석 |

**쿼리 매개변수:** `namespace` (선택)

**점검 카테고리:**
| 점검 항목 | 심각도 | 설명 |
|---------|--------|------|
| 백엔드 서비스가 존재하지 않음 | critical | 참조된 Service가 없음 |
| 백엔드에 준비된 endpoint 없음 | warning | Service에 ready endpoints 없음 |
| TLS 구성 없음 | warning | host가 있으나 암호화되지 않음 |
| IngressClass가 존재하지 않음 | critical | 지정된 class가 배포되지 않음 |
| host+path 충돌 | warning | 여러 Ingress가 동일 라우트 경합 |
| 라우팅 규칙 없음 | warning | Ingress가 작동하지 않음 |

**반환 내용:** Ingress별 상태(healthy/warning/critical), 네임스페이스별 통계, 클러스터 건강 점수 (0-100), 실행 가능한 권장사항

### 노드 조건 및 리소스 압력 분석 (v14.99+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/node-pressure` | 모든 노드의 조건 상태 및 리소스 포화도 분석 |

**감지 카테고리:**
| 조건 | 위험 점수 | 설명 |
|------|--------|------|
| NetworkUnavailable | +30 | CNI/네트워크 미준비 |
| DiskPressure | +25 | 디스크 가득 참 또는 근접 |
| MemoryPressure | +25 | 노드 메모리 고갈 |
| PIDPressure | +20 | 프로세스 수 과다 |
| NotReady | ->critical | kubelet/런타임 문제 |
| CPU >90% | +20 | CPU 요청 포화 |
| Memory >95% | +20 | 메모리 요청 포화 |
| Cordoned | — | 스케줄 불가 |

**반환 내용:** 노드별 위험 등급(critical/high/medium/low), CPU/메모리/Pod 사용률, 조건 상세(원인, 메시지, 지속 시간), 클러스터 압력 점수 (0-100), 실행 가능한 권장사항

### PVC 바인딩 및 스토리지 성능 분석 (v15.00+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/scalability/pvc-analysis` | 모든 PVC의 바인딩 건강 및 스토리지 성능 분석 |

**쿼리 매개변수:** `namespace` (선택)

**감지 카테고리:**
| 점검 항목 | 심각도 | 설명 |
|---------|--------|------|
| Stuck PVC (>5min) | critical | 멈춘 PVC + 근본 원인 분석 |
| Lost PVC | critical | 기반 PV가 삭제되었을 수 있음 |
| 느린 바인딩 (>30s) | warning | 스토리지 프로비저닝 지연 |
| Pending PVC | warning | 바인딩 대기 중 |
| 기본 StorageClass 누락 | info | 기본 SC 미설정 |

**반환 내용:** PVC별 상태(healthy/warning/critical), 바인딩 시간, StorageClass별 통계, Stuck PVC 근본 원인, 클러스터 스토리지 건강 점수 (0-100)

### Namespace 거버넌스 및 라이프사이클 감사 (v15.02+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/product/namespaces/lifecycle` | 모든 네임스페이스의 거버넌스 규정 준수 및 라이프사이클 감사 |

**거버넌스 점검:**
| 점검 항목 | 위험 점수 | 설명 |
|---------|--------|------|
| ResourceQuota 없음 | +15 | 무제한 리소스 소비 |
| NetworkPolicy 없음 | +15 | 트래픽 제한 없음 |
| LimitRange 없음 | +5 | 기본 리소스 제한 없음 |
| 네임스페이스 만료 | +10 | 실행 중인 Pod 없음, 정리 대상 |
| 필수 라벨 누락 | +5 | app/team/env/owner 누락 |
| default SA만 존재 | 0 | 최소 권한 SA 누락 |

**반환 내용:** 네임스페이스별 위험 등급(critical/high/medium/low), 규정 준수 플래그, 라이프사이클 상태(active/stale/terminating), 클러스터 거버넌스 점수 (0-100), 실행 가능한 권장사항

### RBAC 유효 권한 및 권한 상승 분석 (v15.04+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/security/rbac-effective` | 모든 주체의 RBAC 유효 권한 및 권한 상승 위험 분석 |

ClusterRoleBindings + RoleBindings를 집계하여 각 주체(User/Group/ServiceAccount)의 실제 권한을 계산합니다.

**감지 카테고리:**

| 점검 항목 | 위험 점수 | 설명 |
|---------|--------|------|
| cluster-admin 동등 | ->critical | 와일드카드 verbs + resources |
| RBAC 생성/수정 가능 | +25 | 자기 권한 상승 경로 |
| 와일드카드 (*) 권한 | +20 | 과도한 권한 부여 |
| Secrets 읽기 가능 | +10 | 민감한 데이터 유출 |
| Pod exec 가능 | +10 | 컨테이너 탈출 진입점 |

**반환 내용:** 주체별 위험 등급, 권한 상승 경로 상세, 클러스터 RBAC 보안 점수 (0-100), 실행 가능한 권장사항

### 컨테이너 OOM Kill 트래커 (v15.05+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/operations/oom-tracker` | 컨테이너 OOMKill 이벤트 및 메모리 구성 분석 추적 |

**쿼리 매개변수:** `namespace` (선택)

**감지 카테고리:**

| 점검 항목 | 위험 점수 | 설명 |
|---------|--------|------|
| OOMKilled 컨테이너 | +15/개 | 메모리 부족으로 종료 |
| 높은 재시작 횟수 (>=10) | +20 | CrashLoop 지표 |
| 높은 재시작 횟수 (>=5) | +10 | 빈번한 재시작 |
| 메모리 제한 없음 | +5 | OOM 동작 예측 불가 |
| 낮은 메모리 제한 (<256MB) | — | 불필요한 OOM 유발 가능 |
| 제한>>요청 (10x+) | — | 노드 메모리 압력 위험 |

**반환 내용:** Pod별 OOM 위험 등급, Top OOM 순위, 네임스페이스별 통계, 클러스터 OOM 위험 점수 (0-100)

### 스토리지 용량 고갈 예측기 (v15.06+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/scalability/storage-forecast` | 스토리지 용량 고갈 시점 예측 |

PV 사용 추세 및 성장률 추정을 기반으로 스토리지 공간 고갈 시점을 예측합니다.

**분석 차원:**

| 지표 | 설명 |
|------|------|
| 용량 vs 사용량 | Longhorn actual-size annotation으로 실제 사용량 획득 지원 |
| 일일 성장률 | 사용률 및 PV 기간 기반 휴리스틱 추정 |
| 고갈까지 일수 | 잔여 공간 / 일일 성장률 |
| 예측 고갈일 | 날짜 또는 ">10년" 또는 "성장 없음" |
| 위험 등급 | critical(>95%) / high(>85%또는<14d) / medium(<30d) / low |

**반환 내용:** PV별 예측, 클러스터 가득 찰 때까지 일수 추정, StorageClass별 통계, 고위험 네임스페이스 순위, 스토리지 건강 점수 (0-100)

### DNS 해석 상태 점검기 (v15.08+)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/product/dns-health` | 클러스터 DNS 해석 건강 상태 분석 |

**CoreDNS 분석:**

| 점검 항목 | 설명 |
|---------|------|
| Pod 건강 | running/ready/restarts/version per pod |
| Corefile | forwarders, plugins, missing Corefile 감지 |
| 복제본 수 | HA를 위해 >= 2 권장 |

**기타 감지:**
- Headless Service endpoint 커버리지 (NXDOMAIN 위험)
- NodeLocal DNS 캐시 감지
- Pod dnsConfig ndots 커버리지 감지 (>5 = 과도한 DNS 쿼리)
- External-DNS 호스팅 서비스 디스커버리

**반환 내용:** CoreDNS Pod 상태, Headless Service 커버리지, DNS 구성 분석, 클러스터 DNS 건강 점수 (0-100), 실행 가능한 권장사항
