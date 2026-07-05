# k8ops 설치 마법사 가이드

대화형 설치 마법사(`wizard.sh`)는 배포 전에 모든 주요 k8ops 컴포넌트(데이터베이스 백엔드, SSO 통합, AI 프로바이더)를 구성하도록 안내합니다.

## 빠른 시작

### 대화형 모드

```bash
git clone https://github.com/topcheer/k8ops.git
cd k8ops
./wizard.sh
```

### 비대화형 모드

```bash
# config/wizard-values.yaml 파일을 설정으로 편집한 후:
./wizard.sh --values config/wizard-values.yaml
```

### 드라이런 (매니페스트만 생성)

```bash
./wizard.sh --dry-run
# 생성된 파일 검토: .wizard-*.yaml
# kubectl apply -f ...로 수동 배포
```

## 마법사 단계

### 1단계: 배포 모드

| 모드 | 설명 | 적합한 경우 |
|------|-------------|----------|
| **DaemonSet** | 모든 노드에서 실행 | K3s/베어메탈 클러스터, 노드 수준 모니터링 |
| **Deployment** | PVC 기반 단일 복제본 | 관리형 K8s (EKS/GKE/AKS), 비용에 민감한 환경 |

### 2단계: 데이터베이스 백엔드

k8ops는 사용자 계정, 역할 및 인증 프로바이더를 위해 데이터베이스를 사용합니다.

| 백엔드 | 용도 | HA | 설정 |
|---------|----------|----|-------|
| **SQLite** | 소규모 클러스터, 단일 노드 | 아니오 | 제로 설정 (내장형) |
| **MySQL** | 다중 복제본, 공유 인증 | 예 | 내부 StatefulSet 또는 외부 연결 |
| **PostgreSQL** | 다중 복제본, 공유 인증 | 예 | 내부 StatefulSet 또는 외부 연결 |

#### 내부 vs 외부 데이터베이스

- **내부**: 마법사가 `k8ops-system` 네임스페이스에 PVC가 있는 MySQL/PostgreSQL StatefulSet을 배포합니다. 완전히 관리되며 외부 종속성이 없습니다.
- **외부**: 기존 데이터베이스에 연결합니다. DSN 연결 문자열을 제공합니다.

#### DSN 형식

**MySQL:**
```
k8ops:password@tcp(mysql-host:3306)/k8ops?charset=utf8mb4&parseTime=True
```

**PostgreSQL:**
```
host=postgres-host user=k8ops password=secret dbname=k8ops sslmode=disable
```

### 3단계: SSO / 아이덴티티 프로바이더

k8ops는 내장 프리셋을 통해 여러 SSO 프로바이더를 지원합니다:

| 프로바이더 | 유형 | 프리셋 |
|----------|------|--------|
| **GitHub** | OIDC | 사전 구성된 issuer |
| **Google** | OIDC | 사전 구성된 issuer |
| **Microsoft** (Entra ID) | OIDC | 사전 구성된 issuer |
| **GitLab** | OIDC | 사전 구성된 issuer |
| **Keycloak** | OIDC | 커스텀 issuer (사용자 realm) |
| **Okta** | OIDC | 커스텀 issuer |
| **Auth0** | OIDC | 커스텀 issuer |
| **LDAP / AD** | LDAP | 서버 + bind DN |
| **커스텀 OIDC** | OIDC | 수동 issuer URL |

#### OIDC 리다이렉트 URL

아이덴티티 프로바이더에 애플리케이션을 등록할 때 이 리다이렉트 URL을 사용하세요:

```
https://<your-dashboard-host>/api/auth/oidc/<provider-name>/callback
```

GitHub 예시:
```
https://k8ops.example.com/api/auth/oidc/github/callback
```

#### LDAP 구성

제공 항목:
- **서버 URL**: `ldap://host:389` 또는 `ldaps://host:636`
- **Search Base**: 예: `ou=users,dc=example,dc=com`
- **Bind DN**: 서비스 계정 DN, 예: `cn=admin,dc=example,dc=com`
- **Bind Password**: 서비스 계정 비밀번호

SSO는 설치 중에 건너뛰고 대시보드의 **Settings > Auth Providers**에서 나중에 구성할 수 있습니다.

### 4단계: AI 프로바이더

| 프로바이더 | 모델 | 비고 |
|----------|--------|-------|
| **OpenAI** | gpt-4o, gpt-4o-mini | 기본값 |
| **Anthropic** | claude-sonnet-4-20250514 | Claude 계열 |
| **Gemini** | gemini-1.5-flash | Google AI |
| **커스텀** | 모든 모델 | OpenAI 호환 엔드포인트 |

AI 프로바이더는 대시보드의 **Settings**를 통해 설치 후로 미룰 수 있습니다.

### 5단계: 확인 및 배포

마법사는 모든 선택 사항의 요약을 표시합니다. 확인 후 다음을 수행합니다:

1. Kubernetes 매니페스트 생성 (시크릿, 선택적 DB StatefulSet)
2. 클러스터에 적용
3. k8ops 배포 (DaemonSet 또는 Deployment)
4. Pod가 준비될 때까지 대기
5. 접속 URL 및 로그인 자격 증명 표시

## 설치 후

### 기본 로그인

- 사용자명: `admin`
- 비밀번호: `admin`
- **첫 로그인 후 즉시 변경하세요**

### 설치 후 SSO 구성

설치 중에 SSO를 건너뛴 경우:

1. **Settings > Auth Providers**로 이동
2. **Add Provider** 클릭
3. 프리셋 선택 (GitHub, Google 등)
4. Client ID 및 Client Secret 입력
5. 저장 및 활성화

### 환경 변수 참조

마법사는 다음 환경 변수를 설정합니다 (수동으로 설정할 수도 있음):

| 변수 | 설명 | 기본값 |
|----------|-------------|---------|
| `AUTH_DB_DRIVER` | 데이터베이스 드라이버 | `sqlite` |
| `AUTH_DB_DSN` | 데이터베이스 연결 문자열 | (비어 있음) |
| `AUTH_DB_PATH` | SQLite 파일 경로 | `/data/k8ops.db` |
| `AUTH_JWT_SECRET` | JWT 서명 비밀키 | (자동 생성) |
| `AUTH_DEFAULT_ROLE` | SSO 사용자 기본 역할 | `viewer` |
| `AIOPS_API_KEY` | AI 프로바이더 API 키 | (비어 있음) |

## CLI 플래그

```bash
./manager \
  --auth-db-driver=postgres \
  --auth-db-dsn="host=localhost user=k8ops password=secret dbname=k8ops sslmode=disable" \
  --auth-jwt-secret=my-secret \
  --provider-type=openai \
  --provider-model=gpt-4o \
  --provider-api-key=sk-... \
  --dashboard-address=:9090
```

## 문제 해결

### SQLite "out of memory" 오류

이는 SQLite 데이터베이스 경로에 쓰기 권한이 없을 때 발생합니다 (예: 읽기 전용 컨테이너 파일 시스템). `/data`가 `emptyDir` 또는 PVC 볼륨으로 지원되는지 확인하세요.

### MySQL/PostgreSQL 연결 실패

1. DSN 형식이 데이터베이스 유형과 일치하는지 확인
2. k8ops Pod에서 데이터베이스로의 네트워크 연결 확인
3. 데이터베이스 사용자에게 CREATE/ALTER 권한이 있는지 확인 (자동 마이그레이션용)

### SSO 리다이렉트 작동 안 함

1. 리다이렉트 URL이 정확히 일치하는지 확인 (후행 슬래시 포함)
2. HTTPS가 올바르게 구성되어 있는지 확인 (OIDC는 HTTPS 필요)
3. 아이덴티티 프로바이더에 올바른 리다이렉트 URL이 등록되어 있는지 확인
