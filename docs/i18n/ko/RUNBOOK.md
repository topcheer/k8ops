# k8ops 운영 매뉴얼 (Runbook)

> 이 문서는 운영 담당자를 대상으로 하며, 일상 운영 작업, 장애 처리 절차, 비상 연락처 및 표준 작업 절차를 다룹니다.

---

## 목차

1. [서비스 개요](#1-서비스-개요)
2. [일상 운영](#2-일상-운영)
3. [장애 처리](#3-장애-처리)
4. [긴급 작업](#4-긴급-작업)
5. [백업 및 복원](#5-백업-및-복원)
6. [성능 튜닝](#6-성능-튜닝)
7. [비상 연락처](#7-비상-연락처)
8. [SLO/SLA 정의](#8-slosla-정의)

---

## 1. 서비스 개요

### 아키텍처 개요

```
┌─────────────────────────────────────────────────┐
│                   사용자 브라우저                  │
│              https://k8ops.iot2.win               │
└───────────────────┬─────────────────────────────┘
                    │ HTTPS (Traefik Ingress)
┌───────────────────▼─────────────────────────────┐
│              Traefik (kube-system)               │
│         websecure: 8443 → 8000                   │
└───────────────────┬─────────────────────────────┘
                    │
┌───────────────────▼─────────────────────────────┐
│            k8ops DaemonSet (k8ops-system)         │
│  ┌─────────────────────────────────────────────┐ │
│  │  Go Binary (프론트엔드 정적 리소스 임베디드)      │ │
│  │  :8080 Dashboard                             │ │
│  │  /metrics  Prometheus                        │ │
│  │  /api/chat  SSE → LLM Provider               │ │
│  │  nsenter → host kubectl                      │ │
│  └─────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────┘
```

### 핵심 컴포넌트

| 컴포넌트 | 위치 | 역할 |
|------|------|------|
| k8ops DaemonSet | k8ops-system | 메인 서비스, 노드당 1개 Pod |
| Traefik | kube-system | Ingress 컨트롤러, TLS 종단 |
| Registry | registry.iot2.win | 프라이빗 이미지 레지스트리 |
| LLM Provider | 외부 API | AI Chat / 진단 / 최적화 엔진 |

### 헬스 체크 엔드포인트

| 엔드포인트 | 예상 응답 | 설명 |
|------|---------|------|
| `https://k8ops.iot2.win/` | 200/303 | 프론트엔드 페이지 |
| `https://k8ops.iot2.win/readyz` | 200 | K8s Readiness Probe |
| `https://k8ops.iot2.win/api/version` | 200 JSON | 버전 정보 |
| `https://k8ops.iot2.win/metrics` | 200 (로컬만) | Prometheus 메트릭 |

---

## 2. 일상 운영

### 2.1 서비스 상태 확인

```bash
# Pod 상태
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops

# 서비스 로그 (최근 100행)
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --tail=100

# 버전 정보
curl -sk https://k8ops.iot2.win/api/version | jq .

# 클러스터 개요
curl -sk https://k8ops.iot2.win/api/cluster/overview | jq .
```

### 2.2 배포 업데이트

```bash
# 새 버전 빌드
cd /Volumes/new/ggai/k8ops
VERSION=v14XX
docker buildx build --platform linux/amd64 \
  --build-arg VERSION=$VERSION \
  -t registry.iot2.win/k8ops:$VERSION \
  -t registry.iot2.win/k8ops:latest \
  --push .

# 롤링 업데이트
kubectl set image daemonset/k8ops \
  k8ops=registry.iot2.win/k8ops:$VERSION -n k8ops-system

# 검증
sleep 15
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops
curl -sk -o /dev/null -w '%{http_code}' https://k8ops.iot2.win/
```

### 2.3 로그 관리

k8ops는 `log/slog` 구조화 로그를 사용하며, 로그 레벨은 환경 변수 `LOG_LEVEL`로 제어합니다:

| 레벨 | 용도 |
|------|------|
| `DEBUG` | 개발 디버깅, 모든 로그 출력 |
| `INFO` (기본값) | 프로덕션 운영, 주요 작업 기록 |
| `WARN` | 경고 및 에러만 |

```bash
# 로그 레벨 변경
kubectl set env daemonset/k8ops LOG_LEVEL=DEBUG -n k8ops-system
```

### 2.4 Provider 구성

AI 기능에는 LLM Provider 구성이 필요합니다:

1. Settings → Provider 구성 페이지 접속
2. Provider 선택 (OpenAI / Zhipu / DeepSeek 등)
3. API Key 입력
4. 연결 테스트

미구성 시 Dashboard에 Provider 미구성 경고 배너가 표시됩니다.

---

## 3. 장애 처리

### 3.1 Pod가 시작되지 않음 (CrashLoopBackOff)

**증상**: k8ops Pod가 반복적으로 재시작

**조사 절차**:
```bash
# 1. Pod 이벤트 확인
kubectl describe pod -n k8ops-system -l app.kubernetes.io/name=k8ops

# 2. 컨테이너 로그 확인
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --previous

# 3. RBAC 권한 확인
kubectl auth can-i --list --as=system:serviceaccount:k8ops-system:k8ops

# 4. ConfigMap/Secret 마운트 확인
kubectl exec -n k8ops-system -it deploy/k8ops -- ls -la /etc/k8ops/
```

**일반적인 원인**:
- RBAC 권한 부족 → `config/rbac/` 확인
- kubeconfig 무효 → 마운트된 kubeconfig 확인
- 포트 충돌 → 8080 포트 점유 여부 확인
- 메모리 부족 → 노드 리소스 확인 `kubectl describe nodes`

### 3.2 Dashboard에 접근할 수 없음 (502/503)

**증상**: https://k8ops.iot2.win이 502 또는 503 반환

**조사 절차**:
```bash
# 1. Ingress 확인
kubectl get ingress -A | grep k8ops

# 2. Traefik 확인
kubectl get pods -n kube-system -l app.kubernetes.io/name=traefik
kubectl logs -n kube-system -l app.kubernetes.io/name=traefik --tail=50

# 3. k8ops Service 확인
kubectl get svc -n k8ops-system
kubectl get endpoints -n k8ops-system

# 4. Pod로 직접 테스트
kubectl exec -n k8ops-system -it deploy/k8ops -- curl -s localhost:8080/api/version
```

**일반적인 원인**:
- Traefik 라우팅 미흡 → Ingress 규칙 확인
- k8ops 미준비 → readyz Probe 확인
- TLS 인증서 만료 → cert-manager 확인

### 3.3 AI Chat이 응답하지 않음

**증상**: Chat에서 메시지 전송 후 응답이 없거나 타임아웃

**조사 절차**:
```bash
# 1. Provider 상태 확인
curl -sk https://k8ops.iot2.win/api/provider/status | jq .

# 2. 엔진 로그 확인
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -i 'llm\|provider\|chat'

# 3. Provider 연결 테스트
curl -sk https://k8ops.iot2.win/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"message":"hello","conversationId":"test"}' --max-time 30
```

**일반적인 원인**:
- API Key 미구성 또는 만료
- Provider API 속도 제한 (429)
- 네트워크 도달 불가 (DNS/방화벽)
- Token 초과 → Agent가 자동으로 컨텍스트를 압축하지만, 극단적인 경우 실패할 수 있음

### 3.4 Registry 푸시 실패 (499)

**증상**: `docker push`가 499 Client Closed Request 반환

**해결책**:
```bash
# Traefik 타임아웃 설정 확인
kubectl get deploy -n kube-system traefik -o jsonpath='{.spec.template.spec.containers[0].args}'

# 타임아웃 매개변수가 누락된 경우 추가:
kubectl patch deploy -n kube-system traefik --type='json' -p='[
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.readtimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.writetimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.idletimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.keepalivemaxtime=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.keepalivemaxrequests=0"}
]'
```

### 3.5 쓰기 작업 실패 (Scale/Delete/Restart)

**증상**: Scale/Delete/Restart 버튼 클릭 후 작업 실패

**조사 절차**:
```bash
# RBAC 권한 확인
kubectl auth can-i patch deployments --as=system:serviceaccount:k8ops-system:k8ops -n default
kubectl auth can-i delete pods --as=system:serviceaccount:k8ops-system:k8ops -n default

# 감사 로그 확인
curl -sk https://k8ops.iot2.win/api/audit?severity=critical | jq .

# 보안 정책 확인
kubectl get psp,podsecurity --all-namespaces 2>/dev/null
```

---

## 4. 긴급 작업

### 4.1 빠른 롤백

```bash
# 이력 버전 확인
kubectl rollout history daemonset/k8ops -n k8ops-system

# 이전 버전으로 롤백
kubectl rollout undo daemonset/k8ops -n k8ops-system

# 지정된 버전으로 롤백
kubectl rollout undo daemonset/k8ops -n k8ops-system --to-revision=3
```

### 4.2 긴급 축소 (0 복제본 유지)

```bash
# 주의: DaemonSet은 scale 0을 지원하지 않으므로 직접 삭제 필요
kubectl delete pod -n k8ops-system -l app.kubernetes.io/name=k8ops --grace-period=0 --force

# 완전히 중지해야 하는 경우, 일시적으로 nodeSelector 변경
kubectl patch daemonset k8ops -n k8ops-system -p='{"spec":{"template":{"spec":{"nodeSelector":{"non-existent":"true"}}}}}'
```

### 4.3 데이터 정리

```bash
# 진단 이력 CRD 정리
kubectl delete diagnostics --all --all-namespaces

# 감사 로그 정리 (최근 7일 보존)
kubectl get auditlogs -A -o json | jq '.items[] | select(.metadata.creationTimestamp < "'$(date -d '7 days ago' -Iseconds)'")' | kubectl delete -f -

# 최적화 보고서 정리
kubectl delete optimizations --all --all-namespaces
```

---

## 5. 백업 및 복원

### 5.1 구성 백업

```bash
# k8ops 구성 백업
kubectl get cm,secret,daemonset -n k8ops-system -o yaml > k8ops-backup-$(date +%Y%m%d).yaml

# CRD 데이터 백업
kubectl get diagnostics,remediations,optimizations -A -o yaml > k8ops-crd-backup-$(date +%Y%m%d).yaml

# RBAC 백업
kubectl get clusterrole,clusterrolebinding -o yaml | grep -A5 k8ops > k8ops-rbac-backup-$(date +%Y%m%d).yaml
```

### 5.2 복원 절차

```bash
# 구성 복원
kubectl apply -f k8ops-backup-YYYYMMDD.yaml

# CRD 데이터 복원
kubectl apply -f k8ops-crd-backup-YYYYMMDD.yaml

# 검증
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops
curl -sk https://k8ops.iot2.win/api/version | jq .
```

### 5.3 정기 백업 권장

Velero 또는 cron job로 매일 백업:
```bash
# Velero 백업 (권장)
velero backup create k8ops-daily-$(date +%Y%m%d) \
  --include-namespaces k8ops-system \
  --include-cluster-resources=true
```

---

## 6. 성능 튜닝

### 6.1 핵심 지표

| 지표 | Prometheus Metric | 알림 임계값 |
|------|-------------------|---------|
| API 레이턴시 | `k8ops_tool_call_duration_seconds` | P99 > 10s |
| LLM 호출 레이턴시 | `k8ops_llm_call_duration_seconds` | P99 > 60s |
| 활성 진단 수 | `k8ops_active_diagnostics` | > 10 |
| 보안 차단 | `k8ops_safety_blocks_total` | rate > 10/min |
| Token 소비 | `k8ops_llm_tokens_total` | 일일 소비 비정상 증가 |
| 클러스터 건강 점수 | `k8ops_cluster_health_score` | < 60 |

### 6.2 리소스 권장값

| 노드 규모 | k8ops 리소스 Request | 리소스 Limit |
|---------|-------------------|-----------|
| 5 노드 이하 | 100m CPU / 128Mi | 500m CPU / 512Mi |
| 5-20 노드 | 200m CPU / 256Mi | 1 CPU / 1Gi |
| 20-50 노드 | 500m CPU / 512Mi | 2 CPU / 2Gi |

### 6.3 로그 레벨 최적화

프로덕션 환경에서는 `INFO` 레벨을 유지할 것을 권장합니다. 문제 조사 시에만 일시적으로 `DEBUG`로 전환하세요:
```bash
# 임시 DEBUG 활성화
kubectl set env daemonset/k8ops LOG_LEVEL=DEBUG -n k8ops-system
# 조사 후 복원
kubectl set env daemonset/k8ops LOG_LEVEL=INFO -n k8ops-system
```

---

## 7. 비상 연락처

### 7.1 에스컬레이션 흐름

```
장애 발견 → 당번 운영자 (L1)
    ├── 5분 이내 미해결 → 운영 책임자 (L2)
    │     ├── 15분 이내 미해결 → 아키텍트 (L3)
    │     │     ├── 프로덕션 영향 → CTO 보고
```

### 7.2 연락처 목록

> 실제 상황에 맞게 작성하세요

| 역할 | 이름 | 전화 | 담당 범위 |
|------|------|------|---------|
| L1 당번 운영자 | ____ | ____ | 최초 대응, 기본 장애 처리 |
| L2 운영 책임자 | ____ | ____ | 복잡한 장애, 다중 서비스 영향 |
| L3 아키텍트 | ____ | ____ | 아키텍처 수준 문제, 데이터 복원 |
| 클러스터 관리자 | ____ | ____ | K8s 클러스터 자체 장애 |
| 네트워크/보안 | ____ | ____ | 네트워크 정책, 인증서, 보안 사고 |

### 7.3 공급업체 연락처

| 공급업체 | 용도 | 연락처 |
|--------|------|---------|
| LLM Provider | AI Chat/진단 | ____ |
| Registry | 이미지 레지스트리 | ____ |
| DNS/CDN | 도메인 이름 해석 | ____ |

---

## 부록: Prometheus 메트릭 목록

k8ops는 다음 커스텀 메트릭을 노출합니다 (`/metrics` 엔드포인트):

| Metric | 유형 | 라벨 | 설명 |
|--------|------|------|------|
| `k8ops_diagnostics_total` | Counter | phase, trigger | 진단 보고서 총 수 |
| `k8ops_remediation_actions_total` | Counter | type, result, risk | 복구 작업 총 수 |
| `k8ops_llm_call_duration_seconds` | Histogram | provider, model, status | LLM 호출 레이턴시 |
| `k8ops_llm_tokens_total` | Counter | provider, model, type | Token 소비량 |
| `k8ops_agent_steps` | Histogram | - | Agent 실행 단계 수 |
| `k8ops_tool_call_duration_seconds` | Histogram | tool, success | 도구 호출 레이턴시 |
| `k8ops_safety_blocks_total` | Counter | reason | 보안 차단 횟수 |
| `k8ops_active_diagnostics` | Gauge | - | 현재 활성 진단 수 |
| `k8ops_active_remediations` | Gauge | - | 현재 실행 중인 복구 |
| `k8ops_audit_events_total` | Counter | type, severity | 감사 이벤트 총 수 |
| `k8ops_cluster_health_score` | Gauge | - | 클러스터 건강 점수 (0-100) |
| `k8ops_conversation_count` | Gauge | - | 활성 대화 수 |
| `k8ops_tool_executions_total` | Counter | tool, success | 도구 실행 총 수 |
| `k8ops_http_requests_total` | Counter | method, path, status | HTTP 요청 총 수 |
| `k8ops_http_request_duration_seconds` | Histogram | method, path | HTTP 요청 레이턴시 |
| `k8ops_http_requests_in_flight` | Gauge | - | 현재 처리 중인 요청 수 |
| `k8ops_api_errors_total` | Counter | method, path, status | API 에러 수 (4xx+5xx) |

---

## 8. SLO/SLA 정의

### 8.1 서비스 수준 목표 (SLO)

| 지표 | 목표 | 측정 기간 | 에러 버짓 |
|------|------|----------|----------|
| Dashboard 가용성 | 99.9% | 30일 롤링 | 43.2 분/월 |
| API 성공률 (429 제외) | 99.5% | 30일 롤링 | 3.6 시간/월 |
| API P99 레이턴시 | < 2s | 실시간 | - |
| AI Chat 응답 시간 | < 30s (첫 token) | 실시간 | - |
| 보안 감사 스캔 완료 | < 60s | 실시간 | - |

### 8.2 에러 버짓 관리

월간 가용성 목표 99.9% = **43.2분 에러 버짓**:

- **버짓 내 (<30분)**: 정상 릴리스 페이스, 추가 승인 불필요
- **버짓 경고 (30-43분)**: 긴급하지 않은 변경 동결, 신뢰성 문제 수정 우선
- **버짓 소진 (>43분)**: 릴리스 전면 동결, 사후 분석 (post-mortem) 실시

### 8.3 SLO 모니터링 쿼리 (Prometheus PromQL)

**API 에러율 (5분간):**
```promql
sum(rate(k8ops_api_errors_total{status=~"5.."}[5m])) by (path)
/ sum(rate(k8ops_http_requests_total[5m])) by (path)
```

**API P99 레이턴시:**
```promql
histogram_quantile(0.99,
  sum(rate(k8ops_http_request_duration_seconds_bucket[5m])) by (le, path)
)
```

**에러 버짓 소비율:**
```promql
1 - (
  sum(rate(k8ops_http_requests_total{status!~"5.."}[30d]))
  / sum(rate(k8ops_http_requests_total[30d]))
)
```

### 8.4 성능 저하 전략

SLO가 위반될 가능성이 있을 때, 우선순위에 따라 성능을 저하시킵니다:

1. **AI Chat 비활성화** — 최대 리소스 소비 기능, 저하 후 핵심 K8s 관리에 영향 없음
2. **캐시 TTL 증가** — overview/nodes/pods 캐시를 30s에서 120s로 상향
3. **동시 진단 제한** — `k8ops_active_diagnostics` 상한 인하
4. **이벤트 수집기 중지** — `--disable-event-collector` 플래그

### 8.5 요청 추적

모든 HTTP 응답에는 `X-Request-ID` 헤더가 포함되며, 다음에 사용됩니다:
- 로그 연관 — 동일 요청의 모든 로그 행이 request_id를 공유
- 감사 추적 — 감사 로그 내의 request_id로 구체적인 HTTP 요청에 연관
- 장애 조사 — 사용자가 문제를 보고할 때 request_id를 제공하면 로그를 빠르게 특정 가능

로그 검색 예시:
```bash
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep "a1b2c3d4e5f6"
```

### 8.6 로그 레벨 구성

k8ops는 구조화된 JSON 로그 (slog)를 사용하며, 환경 변수 `LOG_LEVEL` 또는 명령행 `--log-level`로 레벨을 구성할 수 있습니다:

| 레벨 | 용도 | 설명 |
|------|------|------|
| `debug` | 문제 조사 | source file:line 포함, 매우 상세한 로그 (프로덕션에는 권장하지 않음) |
| `info` | 기본값 | 정상 작업 로그 (프로덕션 사용 권장) |
| `warn` | 경고만 | 느린 요청, 구성 문제, 임계값 접근 |
| `error` | 에러만 | 작업 실패만 기록 |

구성 방법:
```bash
# 환경 변수로 구성 (권장)
kubectl set env daemonset/k8ops -n k8ops-system LOG_LEVEL=debug

# ConfigMap으로 구성
kubectl patch configmap k8ops-config -n k8ops-system \
  --type='json' -p='[{"op":"add","path":"/data/log-level","value":"debug"}]'

# 명령행 인수로 구성 (Deployment 모드에만 적용)
# args:
# - --log-level=debug
```

레벨 전환 후 Pod 재시작:
```bash
kubectl rollout restart daemonset/k8ops -n k8ops-system
```

### 8.7 로그 로테이션

감사 로그 파일 (`/data/k8ops-audit.jsonl`)은 자동으로 로테이션됩니다:
- **자동 로테이션**: 파일이 100MB를 초과하면 자동으로 분할
- **수동 로테이션**: `POST /api/system/log/rotate` (admin 권한)
- **오래된 파일 정리**: `POST /api/system/log/cleanup` (30일 이상 경과한 로테이션 파일 삭제)

컨테이너 stdout 로그는 Kubelet이 관리하며, 기본적으로 각 컨테이너당 10MB x 3 파일 = 30MB가 상한입니다.
k3s에서는 `--container-log-max-size` 및 `--container-log-max-files`로 조정할 수 있습니다.

---

*최종 업데이트: 2026-07-02*
*유지보수자: k8ops Team*
