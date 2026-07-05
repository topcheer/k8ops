# k8ops 문제 해결 가이드

> 이 문서는 k8ops의 일반적인 문제에 대한 진단 방법과 해결책을 요약합니다. 심각도별로 분류되어 있어 빠르게 문제를 파악할 수 있습니다.

---

## 목차

1. [설치 및 시작 문제](#1-설치-및-시작-문제)
2. [인증 및 로그인 문제](#2-인증-및-로그인-문제)
3. [AI 기능 문제](#3-ai-기능-문제)
4. [Pod 및 클러스터 문제](#4-pod-및-클러스터-문제)
5. [네트워크 및 Ingress 문제](#5-네트워크-및-ingress-문제)
6. [데이터 및 스토리지 문제](#6-데이터-및-스토리지-문제)
7. [성능 문제](#7-성능-문제)
8. [모니터링 및 알림 문제](#8-모니터링-및-알림-문제)

---

## 1. 설치 및 시작 문제

### 1.1 Pod가 계속 Pending 상태

**현상:** `kubectl get pods -n k8ops-system`이 Pending 표시

**해결 단계:**
```bash
# Pending 원인 확인
kubectl describe pod -n k8ops-system -l app.kubernetes.io/name=k8ops

# 일반적인 원인:
# - PVC가 바인딩되지 않음 (StorageClass 확인)
# - 리소스 부족 (노드 용량 확인)
# - Node Selector 불일치
```

**해결책:**
- **PVC 바인딩 안 됨:** 클러스터에 기본 StorageClass가 있는지 확인
  ```bash
  kubectl get storageclass
  # 기본 SC가 없으면 표시:
  kubectl patch storageclass local-path -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
  ```
- **리소스 부족:** DaemonSet 모드 사용 (PVC 종속성 없음)
  ```bash
  kubectl apply -k config/daemonset
  ```

### 1.2 Pod CrashLoopBackOff

**현상:** Pod가 반복적으로 재시작

**해결 단계:**
```bash
# 컨테이너 로그 확인
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --tail=50

# 이벤트 확인
kubectl get events -n k8ops-system --sort-by='.lastTimestamp' | tail -20
```

**일반적인 원인 및 해결책:**

| 원인 | 로그 특징 | 해결책 |
|------|----------|----------|
| SQLite 권한 문제 | `unable to open database file` | `mkdir -p /data && chown 65532:65532 /data` |
| JWT Secret 누락 | `JWT secret not configured` | `AUTH_JWT_SECRET` 환경 변수 설정 |
| K8s API 연결 실패 | `failed to get Kubernetes config` | ServiceAccount 및 RBAC 확인 |
| 포트 충돌 | `bind: address already in use` | `--dashboard-address` 수정 |

### 1.3 이미지 풀 실패 (ImagePullBackOff)

**현상:** `Failed to pull image`

**해결책:**
```bash
# 이미지에 접근 가능한지 확인
docker pull registry.iot2.win/k8ops:latest

# 프라이빗 저장소를 사용하는 경우 imagePullSecrets 구성
kubectl create secret docker-registry regcred \
  --docker-server=registry.iot2.win \
  --docker-username=<user> \
  --docker-password=<pass> \
  -n k8ops-system

# 또는 DaemonSet 모드 + hostPath 사용 (외부 이미지 풀 불필요)
```

---

## 2. 인증 및 로그인 문제

### 2.1 로그인 시 401 Unauthorized 반환

**해결:**
```bash
# auth 구성 확인
kubectl exec -n k8ops-system deploy/k8ops -- /manager --help | grep auth

# auth 관련 로그 확인
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -i auth
```

**해결책:**
- `AUTH_JWT_SECRET`이 설정되어 있고 일치하는지 확인
- 관리자 비밀번호 재설정:
  ```bash
  kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops admin reset-password
  ```
- 기본 자격 증명: `admin` / `changeme` (첫 로그인 후 변경하세요)

### 2.2 OIDC 로그인 실패

**해결:**
- Pod 내부에서 OIDC Provider URL에 도달 가능한지 확인
- redirect URL이 Ingress 도메인과 일치하는지 확인
- callback 오류 확인: `kubectl logs ... | grep oidc`

---

## 3. AI 기능 문제

### 3.1 Chat 응답 없음 또는 시간 초과

**현상:** 메시지 전송 후 응답 없음 또는 시간 초과 반환

**해결 단계:**
```bash
# Provider 구성 확인
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops config show

# AI 관련 로그 확인
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -E "provider|llm|agent"

# LLM 연결성 테스트
kubectl exec -n k8ops-system deploy/k8ops -- curl -s https://api.openai.com/v1/models -H "Authorization: Bearer $AIOPS_API_KEY"
```

**일반적인 원인:**

| 원인 | 로그 특징 | 해결책 |
|------|----------|----------|
| API Key 무효 | `401 Unauthorized` | `AIOPS_API_KEY` 환경 변수 업데이트 |
| 네트워크 불통 | `context deadline exceeded` | LLM API egress 구성 |
| 모델이 존재하지 않음 | `model not found` | `--provider-model` 업데이트 |
| 속도 제한 | `429 Too Many Requests` | 대기 또는 Provider 전환 |
| 서킷 브레이커 오픈 | `circuit breaker open` | 60s 쿨다운 대기 |

### 3.2 AI 진단이 트리거되지 않음

**해결:**
```bash
# 이벤트 수집기 상태 확인
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -i "collector\|event"

# 비활성화되지 않았는지 확인
kubectl get deploy k8ops -n k8ops-system -o jsonpath='{.spec.template.spec.containers[0].command}'
# --disable-event-collector가 포함되어 있지 않아야 함
```

---

## 4. Pod 및 클러스터 문제

### 4.1 Dashboard에 "kubernetes client not available" 표시

**현상:** API가 503 반환, UI에 연결 오류 표시

**원인:** Pod 내 K8s ServiceAccount 권한 부족 또는 config 로드 실패

**해결책:**
```bash
# RBAC 재적용
kubectl apply -k config/rbac

# ServiceAccount 검증
kubectl auth can-i list pods --as=system:serviceaccount:k8ops-system:k8ops -n k8ops-system
```

### 4.2 작업(Scale/Delete/Restart)이 403 Forbidden 반환

**원인:** 사용자 RBAC 역할 권한 부족

**해결책:**
```bash
# 사용자 역할 확인
kubectl get rolebindings -n k8ops-system | grep <username>

# admin 역할로 승격
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops admin set-role <username> admin
```

---

## 5. 네트워크 및 Ingress 문제

### 5.1 Dashboard 접근 불가 (502/503)

**해결:**
```bash
# Service에 Endpoints가 있는지 확인
kubectl get endpoints -n k8ops-system

# Ingress 구성 확인
kubectl get ingress -n k8ops-system
kubectl describe ingress -n k8ops-system

# Pod 포트에 직접 접근
kubectl port-forward -n k8ops-system deploy/k8ops 9090:9090
# 그런 다음 http://localhost:9090 접속
```

### 5.2 Traefik 시간 초과 (499/504)

**현상:** Registry push 또는 대용량 파일 업로드 시간 초과

**해결책 (Traefik 특정):**
```bash
# Traefik 시간 초과 제한 비활성화
kubectl patch deployment -n kube-system traefik \
  --type='json' \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.readtimeout=0s"}]'

# 또는 IngressRoute에 timeout 설정
```

### 5.3 SSE (Server-Sent Events) 작동 안 함

**현상:** Chat 인터페이스에 실시간 응답 없음

**해결:**
- 리버스 프록시가 긴 연결을 지원하는지 확인
- Nginx 구성 필요: `proxy_buffering off; proxy_cache off;`
- Traefik은 추가 구성 불필요

---

## 6. 데이터 및 스토리지 문제

### 6.1 SQLite 데이터베이스 손상

**현상:** `database disk image is malformed`

**해결책:**
```bash
# Pod 진입
kubectl exec -it -n k8ops-system deploy/k8ops -- sh

# 데이터베이스 복구 (distroless에 shell이 없는 경우 CLI 도구 사용)
# 방법 1: 백업 후 재구축
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops db backup /data/k8ops-backup.db
kubectl exec -n k8ops-system deploy/k8ops -- rm /data/k8ops.db
kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops db restore /data/k8ops-backup.db

# 방법 2: PVC 삭제 후 재구축 (사용자 데이터 손실)
kubectl delete pvc -n k8ops-system k8ops-data
kubectl delete pod -n k8ops-system -l app.kubernetes.io/name=k8ops
```

### 6.2 PVC 디스크 공간 부족

**해결:**
```bash
kubectl exec -n k8ops-system deploy/k8ops -- df -h /data
# 또는 Dashboard → Capacity 페이지에서 확인
```

**해결책:**
- PVC 확장:
  ```bash
  kubectl patch pvc -n k8ops-system k8ops-data -p '{"spec":{"resources":{"requests":{"storage":"5Gi"}}}}'
  ```
- 오래된 감사 로그 정리:
  ```bash
  kubectl exec -n k8ops-system deploy/k8ops -- /usr/local/bin/k8ops audit cleanup --max-age 30d
  ```

---

## 7. 성능 문제

### 7.1 API 응답 느림

**해결:**
```bash
# 응답 시간 확인 (X-Response-Time 헤더)
curl -sk -o /dev/null -w '%{http_code} %{time_total}s\n' \
  -D - https://k8ops.iot2.win/api/cluster/overview 2>&1 | grep -i "x-response-time"

# Prometheus 메트릭 확인
curl -sk https://k8ops.iot2.win/metrics | grep k8ops_http_request_duration
```

**최적화 방안:**
- API 캐싱이 활성화됨 (overview: 30s, resources: 60s, CRDs: 10min)
- `k8ops_http_requests_in_flight`가 너무 높은지 확인
- 느린 요청 로그(>500ms)가 자동으로 Pod 로그에 기록됨

### 7.2 메모리 사용량 높음

**해결:**
```bash
kubectl top pods -n k8ops-system
```

**최적화:**
- 대화 메모리 자동 관리: 20k token 임계값 도달 시 자동 요약
- 유휴 대화 30min 후 정리
- 지속적으로 메모리가 높다면 Pod 재시작 고려 (DaemonSet 모드는 자동 재시작)

---

## 8. 모니터링 및 알림 문제

### 8.1 Prometheus에서 Metrics를 가져오지 못함

**해결:**
```bash
# metrics 엔드포인트 정상 확인
kubectl exec -it <prometheus-pod> -n monitoring -- curl -s http://k8ops.k8ops-system.svc:8080/metrics | head -5

# ServiceMonitor 확인
kubectl get servicemonitor -n k8ops-system
```

**참고:** `/metrics` 엔드포인트는 localhost 접근만 허용합니다. Prometheus는 클러스터 내부(동일 Pod 또는 Service)에서 스크랩해야 합니다.

### 8.2 알림 규칙이 작동하지 않음

**해결:**
```bash
# PrometheusRule 확인
kubectl get prometheusrule -n k8ops-system

# 알림 규칙 적용
kubectl apply -f config/alerting-rules.yaml
```

---

## 부록: 자주 사용하는 진단 명령어

```bash
# 원클릭 상태 확인
kubectl get pods -n k8ops-system
kubectl get events -n k8ops-system --sort-by='.lastTimestamp' | tail -20
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --tail=50

# 상태 확인
curl -sk https://k8ops.iot2.win/api/health
curl -sk https://k8ops.iot2.win/api/version

# 보안 스캔
curl -sk https://k8ops.iot2.win/api/security/audit | jq .summary
curl -sk https://k8ops.iot2.win/api/security/compliance | jq .score

# 용량 계획
curl -sk https://k8ops.iot2.win/api/capacity/planning | jq .summary
```

## 부록: 로그 레벨

k8ops는 구조화된 JSON 로깅(slog)을 사용하며, 다음 레벨을 지원합니다:

| 레벨 | 용도 | 예시 |
|------|------|------|
| `INFO` | 정상 작업 | 서버 시작, 인증 성공 |
| `WARN` | 느린 요청, 구성 문제 | 요청 >500ms, PVC 거의 가득 참 |
| `ERROR` | 작업 실패 | K8s API 오류, LLM 호출 실패 |

Request ID로 로그 연관시키기:
```bash
# Request ID 획득 (HTTP 응답 헤더 X-Request-ID에서)
# 그런 다음 로그에서 검색
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep "a1b2c3d4"
```

---

*최종 업데이트: 2026-07-03*
*유지보수자: k8ops Team*
