# k8ops 보안

## 인증

k8ops는 배포별로 구성 가능한 세 가지 인증 방식을 지원합니다:

### 로컬 인증

- SQLite에 사용자명/비밀번호 저장
- 비밀번호는 bcrypt로 해시 처리
- `AUTH_DEFAULT_ROLE` 환경 변수를 통한 관리자 부트스트랩

### LDAP/Active Directory

- 구성 가능한 서버 URL, bind DN, search base
- 자체 서명 인증서를 위한 `SkipTLSVerify` 옵션 (기본값: `false`)
- 멀티 프로바이더 지원: 여러 LDAP 서버를 동시에 구성 가능

### OIDC (OpenID Connect)

- 모든 OIDC 호환 IdP 지원 (Google, GitHub, Keycloak 등)
- **CSRF 보호**: state 매개변수를 `crypto/subtle.ConstantTimeCompare`로 검증
- **프로바이더별 쿠키**: `oidc_state_{provider}`로 멀티 프로바이더 충돌 방지
- **Secure 플래그**: TLS 또는 `X-Forwarded-Proto` 헤더를 통해 자동 감지
- **HttpOnly + SameSite**: state 쿠키는 JavaScript로 접근 불가

## RBAC 모델

### 역할

| 역할 | 범위 | 권한 |
|------|-------|------------|
| `admin` | 클러스터 | 전체 접근: 사용자, 프로바이더, 모든 네임스페이스 관리 |
| `operator` | 클러스터 | 모든 읽기 + 채팅 + 진단 실행 |
| `viewer` | 클러스터 | 읽기 전용: 대시보드, 보고서 조회 |
| `ns-admin` | 네임스페이스 | 할당된 네임스페이스 내 관리자 권한 |
| `ns-viewer` | 네임스페이스 | 할당된 네임스페이스 내 읽기 전용 |

### 네임스페이스 스코핑

네임스페이스 스코프 역할을 가진 사용자는 다음을 통해 할당된 네임스페이스로 제한됩니다:

1. **K8s RBAC 동기화**: 네임스페이스별 `RoleBinding` 리소스 생성
2. **API 임퍼스네이션 (Impersonation)**: Dashboard API 호출 시 K8s API와 통신할 때 사용자 식별자 사용
3. **네임스페이스 필터링**: API 응답이 허용된 네임스페이스로 필터링됨

### 기본 제공 역할 보호

기본 역할 (`admin`, `operator`, `viewer`)은 `Builtin: true`로 표시되며 API를 통해 삭제할 수 없습니다.

## 보안 기능

### CORS 허용 목록

- `CORS_ALLOWED_ORIGINS` 환경 변수로 구성 (쉼표 구분)
- 자격 증명이 포함된 경우 와일드카드(`*`) 사용 불가
- 구성되지 않은 경우 동일 출처만 허용

### OIDC CSRF 보호

- State 매개변수: 인증 시도마다 임의의 nonce 생성
- `subtle.ConstantTimeCompare`로 검증 (타이밍 안전)
- Secure + SameSite 플래그가 있는 HttpOnly 쿠키에 저장

### JWT 영속화

- JWT 서명 비밀키는 K8s Secret `k8ops-auth` (key: `jwt-secret`)에 영속화됨
- Secret이 없는 경우 경고 로그와 함께 임시 무작위 비밀키로 폴백
- Pod 재시작 시 세션 무효화 방지

### 감사 로깅

모든 민감한 작업이 로깅됩니다:

- 사용자 로그인/로그아웃
- 프로바이더 구성 변경
- 진단 실행
- 수정 조치
- 사용자 역할 변경

### 속도 제한

- `resilience.RateLimiter` 사용 가능 (아직 HTTP 계층에 연결되지 않음 — 향후 작업)

### 정상 종료

- `SIGTERM`/`SIGINT` → SSE 연결 드레이닝 → SQLite WAL 플러시 → 매니저 중지
- Pod 제거 시 데이터 손상 방지

## 보안 구성

### 환경 변수

| 변수 | 기본값 | 설명 |
|----------|---------|-------------|
| `AUTH_DB_DRIVER` | `sqlite` | 데이터베이스 드라이버 |
| `AUTH_DB_DSN` | — | 데이터베이스 연결 문자열 |
| `AUTH_DB_PATH` | `/data/k8ops.db` | SQLite 데이터베이스 경로 |
| `AUTH_JWT_SECRET` | (무작위) | JWT 서명 비밀키 (K8s Secret을 통해 영속화) |
| `AUTH_DEFAULT_ROLE` | `viewer` | 신규 사용자 역할 |
| `CORS_ALLOWED_ORIGINS` | — | 쉼표로 구분된 허용 출처 |
| `AIOPS_API_KEY` | — | LLM 프로바이더 API 키 |

### K8s Secret 관리

```yaml
# K8s Secret for JWT persistence
apiVersion: v1
kind: Secret
metadata:
  name: k8ops-auth
  namespace: k8ops-system
type: Opaque
stringData:
  jwt-secret: "<openssl rand -base64 32>"
```

배포에서 다음을 통해 읽습니다:
```yaml
env:
- name: AUTH_JWT_SECRET
  valueFrom:
    secretKeyRef:
      name: k8ops-auth
      key: jwt-secret
      optional: true  # falls back to random if absent
```

### LDAP TLS 구성

LDAP 프로바이더는 `skip_tls_verify` (기본값: `false`)를 지원합니다:

```json
{
  "ldap": {
    "server": "ldaps://ldap.corp.com",
    "skip_tls_verify": false
  }
}
```

자체 서명 인증서를 사용하는 개발 환경에서만 `skip_tls_verify: true`로 설정하세요.

## 알려진 제한 사항

1. **로그인 속도 제한 없음** — `resilience.RateLimiter`가 존재하지만 HTTP 핸들러에 연결되지 않음
2. **HTTPS 종료 없음** — k8ops는 HTTP를 서빙하며, TLS는 ingress controller가 처리해야 함
3. **SQLite 단일 노드** — HA 데이터베이스가 없으며 단일 복제본 배포에 적합
4. **세션 취소 없음** — JWT 토큰은 만료(24h)까지 유효하며 서버 측 취소 목록이 없음

## 보안 신고

보안 취약점을 신고하려면:

1. **공개 GitHub 이슈를 열지 마세요**
2. security@ggai.dev로 상세 내용과 재현 단계를 이메일로 보내세요
3. 48시간 이내에 확인하고 수정 일정을 제공합니다
4. 책임 있는 공개를 감사하게 생각합니다

## 향후 보안 개선 사항

- [ ] 로그인 API에 속도 제한 연결
- [ ] 세션 취소 (denylist) 추가
- [ ] RBAC를 위한 외부 OAuth 프로바이더 지원
- [ ] 서비스 간 통신을 위한 mTLS 추가
- [ ] 저장 시 암호화 구현 (PVC 이상)
