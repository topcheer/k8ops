# k8ops Runbook

> This document is intended for operations personnel. It covers routine maintenance, troubleshooting procedures, emergency contacts, and standard operating procedures.

---

## Table of Contents

1. [Service Overview](#1-service-overview)
2. [Routine Operations](#2-routine-operations)
3. [Troubleshooting](#3-troubleshooting)
4. [Emergency Operations](#4-emergency-operations)
5. [Backup and Recovery](#5-backup-and-recovery)
6. [Performance Tuning](#6-performance-tuning)
7. [Emergency Contacts](#7-emergency-contacts)
8. [SLO/SLA Definitions](#8-slosla-definitions)

---

## 1. Service Overview

### Architecture Overview

```
┌─────────────────────────────────────────────────┐
│                   User Browser                    │
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
│  │  Go Binary (embedded frontend static assets) │ │
│  │  :8080 Dashboard                             │ │
│  │  /metrics  Prometheus                        │ │
│  │  /api/chat  SSE → LLM Provider               │ │
│  │  nsenter → host kubectl                      │ │
│  └─────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────┘
```

### Key Components

| Component | Location | Purpose |
|-----------|----------|---------|
| k8ops DaemonSet | k8ops-system | Main service, one Pod per node |
| Traefik | kube-system | Ingress controller, TLS termination |
| Registry | registry.iot2.win | Private image registry |
| LLM Provider | External API | AI Chat / Diagnostics / Optimization engine |

### Health Check Endpoints

| Endpoint | Expected Response | Description |
|----------|-------------------|-------------|
| `https://k8ops.iot2.win/` | 200/303 | Frontend page |
| `https://k8ops.iot2.win/readyz` | 200 | K8s readiness probe |
| `https://k8ops.iot2.win/api/version` | 200 JSON | Version information |
| `https://k8ops.iot2.win/metrics` | 200 (local only) | Prometheus metrics |

---

## 2. Routine Operations

### 2.1 Checking Service Status

```bash
# Pod status
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops

# Service logs (last 100 lines)
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --tail=100

# Version information
curl -sk https://k8ops.iot2.win/api/version | jq .

# Cluster overview
curl -sk https://k8ops.iot2.win/api/cluster/overview | jq .
```

### 2.2 Updating the Deployment

```bash
# Build new version
cd /Volumes/new/ggai/k8ops
VERSION=v14XX
docker buildx build --platform linux/amd64 \
  --build-arg VERSION=$VERSION \
  -t registry.iot2.win/k8ops:$VERSION \
  -t registry.iot2.win/k8ops:latest \
  --push .

# Rolling update
kubectl set image daemonset/k8ops \
  k8ops=registry.iot2.win/k8ops:$VERSION -n k8ops-system

# Verify
sleep 15
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops
curl -sk -o /dev/null -w '%{http_code}' https://k8ops.iot2.win/
```

### 2.3 Log Management

k8ops uses structured logging via `log/slog`. The log level is controlled by the `LOG_LEVEL` environment variable:

| Level | Purpose |
|-------|---------|
| `DEBUG` | Development debugging, outputs all logs |
| `INFO` (default) | Production operation, logs key operations |
| `WARN` | Warnings and errors only |

```bash
# Change log level
kubectl set env daemonset/k8ops LOG_LEVEL=DEBUG -n k8ops-system
```

### 2.4 Provider Configuration

AI features require an LLM Provider configuration:

1. Navigate to Settings → Provider configuration page
2. Select a Provider (OpenAI / Zhipu / DeepSeek, etc.)
3. Enter the API Key
4. Test the connection

If not configured, the Dashboard will display a Provider not configured warning banner.

---

## 3. Troubleshooting

### 3.1 Pod Won't Start (CrashLoopBackOff)

**Symptom**: k8ops Pod repeatedly restarts

**Troubleshooting Steps**:
```bash
# 1. View Pod events
kubectl describe pod -n k8ops-system -l app.kubernetes.io/name=k8ops

# 2. View container logs
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops --previous

# 3. Check RBAC permissions
kubectl auth can-i --list --as=system:serviceaccount:k8ops-system:k8ops

# 4. Check ConfigMap/Secret mounts
kubectl exec -n k8ops-system -it deploy/k8ops -- ls -la /etc/k8ops/
```

**Common Causes**:
- Insufficient RBAC permissions → Check `config/rbac/`
- Invalid kubeconfig → Check mounted kubeconfig
- Port conflict → Check if port 8080 is already in use
- Insufficient memory → Check node resources with `kubectl describe nodes`

### 3.2 Dashboard Unreachable (502/503)

**Symptom**: https://k8ops.iot2.win returns 502 or 503

**Troubleshooting Steps**:
```bash
# 1. Check Ingress
kubectl get ingress -A | grep k8ops

# 2. Check Traefik
kubectl get pods -n kube-system -l app.kubernetes.io/name=traefik
kubectl logs -n kube-system -l app.kubernetes.io/name=traefik --tail=50

# 3. Check k8ops Service
kubectl get svc -n k8ops-system
kubectl get endpoints -n k8ops-system

# 4. Test Pod directly
kubectl exec -n k8ops-system -it deploy/k8ops -- curl -s localhost:8080/api/version
```

**Common Causes**:
- Traefik not routing correctly → Check Ingress rules
- k8ops not ready → Check readyz probe
- TLS certificate expired → Check cert-manager

### 3.3 AI Chat Not Responding

**Symptom**: Chat sends a message but gets no response or times out

**Troubleshooting Steps**:
```bash
# 1. Check Provider status
curl -sk https://k8ops.iot2.win/api/provider/status | jq .

# 2. View engine logs
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep -i 'llm\|provider\|chat'

# 3. Test Provider connection
curl -sk https://k8ops.iot2.win/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"message":"hello","conversationId":"test"}' --max-time 30
```

**Common Causes**:
- API Key not configured or expired
- Provider API rate limited (429)
- Network unreachable (DNS/firewall)
- Token limit exceeded → Agent auto-compresses context, but may fail in extreme cases

### 3.4 Registry Push Failure (499)

**Symptom**: `docker push` returns 499 Client Closed Request

**Solution**:
```bash
# Check Traefik timeout configuration
kubectl get deploy -n kube-system traefik -o jsonpath='{.spec.template.spec.containers[0].args}'

# If timeout parameters are missing, add them:
kubectl patch deploy -n kube-system traefik --type='json' -p='[
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.readtimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.writetimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.respondingtimeouts.idletimeout=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.keepalivemaxtime=0s"},
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--entrypoints.websecure.transport.keepalivemaxrequests=0"}
]'
```

### 3.5 Write Operations Failing (Scale/Delete/Restart)

**Symptom**: Scale/Delete/Restart buttons fail after clicking

**Troubleshooting Steps**:
```bash
# Check RBAC permissions
kubectl auth can-i patch deployments --as=system:serviceaccount:k8ops-system:k8ops -n default
kubectl auth can-i delete pods --as=system:serviceaccount:k8ops-system:k8ops -n default

# View audit logs
curl -sk https://k8ops.iot2.win/api/audit?severity=critical | jq .

# Check security policies
kubectl get psp,podsecurity --all-namespaces 2>/dev/null
```

---

## 4. Emergency Operations

### 4.1 Quick Rollback

```bash
# View version history
kubectl rollout history daemonset/k8ops -n k8ops-system

# Roll back to previous version
kubectl rollout undo daemonset/k8ops -n k8ops-system

# Roll back to a specific version
kubectl rollout undo daemonset/k8ops -n k8ops-system --to-revision=3
```

### 4.2 Emergency Scale-Down (Retain 0 Replicas)

```bash
# Note: DaemonSet does not support scale 0; pods must be deleted directly
kubectl delete pod -n k8ops-system -l app.kubernetes.io/name=k8ops --grace-period=0 --force

# To completely stop, temporarily modify the nodeSelector
kubectl patch daemonset k8ops -n k8ops-system -p='{"spec":{"template":{"spec":{"nodeSelector":{"non-existent":"true"}}}}}'
```

### 4.3 Data Cleanup

```bash
# Clean up diagnostic history CRDs
kubectl delete diagnostics --all --all-namespaces

# Clean up audit logs (retain last 7 days)
kubectl get auditlogs -A -o json | jq '.items[] | select(.metadata.creationTimestamp < "'$(date -d '7 days ago' -Iseconds)'")' | kubectl delete -f -

# Clean up optimization reports
kubectl delete optimizations --all --all-namespaces
```

---

## 5. Backup and Recovery

### 5.1 Configuration Backup

```bash
# Back up k8ops configuration
kubectl get cm,secret,daemonset -n k8ops-system -o yaml > k8ops-backup-$(date +%Y%m%d).yaml

# Back up CRD data
kubectl get diagnostics,remediations,optimizations -A -o yaml > k8ops-crd-backup-$(date +%Y%m%d).yaml

# Back up RBAC
kubectl get clusterrole,clusterrolebinding -o yaml | grep -A5 k8ops > k8ops-rbac-backup-$(date +%Y%m%d).yaml
```

### 5.2 Recovery Procedure

```bash
# Restore configuration
kubectl apply -f k8ops-backup-YYYYMMDD.yaml

# Restore CRD data
kubectl apply -f k8ops-crd-backup-YYYYMMDD.yaml

# Verify
kubectl get pods -n k8ops-system -l app.kubernetes.io/name=k8ops
curl -sk https://k8ops.iot2.win/api/version | jq .
```

### 5.3 Regular Backup Recommendations

Use Velero or a cron job for daily backups:
```bash
# Velero backup (recommended)
velero backup create k8ops-daily-$(date +%Y%m%d) \
  --include-namespaces k8ops-system \
  --include-cluster-resources=true
```

---

## 6. Performance Tuning

### 6.1 Key Metrics

| Metric | Prometheus Metric | Alert Threshold |
|--------|-------------------|-----------------|
| API latency | `k8ops_tool_call_duration_seconds` | P99 > 10s |
| LLM call latency | `k8ops_llm_call_duration_seconds` | P99 > 60s |
| Active diagnostics | `k8ops_active_diagnostics` | > 10 |
| Safety blocks | `k8ops_safety_blocks_total` | rate > 10/min |
| Token consumption | `k8ops_llm_tokens_total` | Abnormal daily growth |
| Cluster health score | `k8ops_cluster_health_score` | < 60 |

### 6.2 Resource Recommendations

| Node Scale | k8ops Resource Request | Resource Limit |
|------------|------------------------|----------------|
| ≤ 5 nodes | 100m CPU / 128Mi | 500m CPU / 512Mi |
| 5-20 nodes | 200m CPU / 256Mi | 1 CPU / 1Gi |
| 20-50 nodes | 500m CPU / 512Mi | 2 CPU / 2Gi |

### 6.3 Log Level Optimization

For production, it is recommended to keep the `INFO` level. Only switch to `DEBUG` temporarily when troubleshooting issues:
```bash
# Temporarily enable DEBUG
kubectl set env daemonset/k8ops LOG_LEVEL=DEBUG -n k8ops-system
# Restore after troubleshooting
kubectl set env daemonset/k8ops LOG_LEVEL=INFO -n k8ops-system
```

---

## 7. Emergency Contacts

### 7.1 Escalation Process

```
Issue detected → On-call Ops (L1)
    ├── Unresolved within 5 minutes → Ops Lead (L2)
    │     ├── Unresolved within 15 minutes → Architect (L3)
    │     │     ├── Production impact → CTO notification
```

### 7.2 Contact List

> Fill in according to your actual situation

| Role | Name | Phone | Responsibility Scope |
|------|------|-------|---------------------|
| L1 On-call Ops | ____ | ____ | First response, basic incident handling |
| L2 Ops Lead | ____ | ____ | Complex incidents affecting multiple services |
| L3 Architect | ____ | ____ | Architecture-level issues, data recovery |
| Cluster Administrator | ____ | ____ | Kubernetes cluster infrastructure failures |
| Network/Security | ____ | ____ | Network policies, certificates, security incidents |

### 7.3 Vendor Contacts

| Vendor | Purpose | Contact |
|--------|---------|---------|
| LLM Provider | AI Chat/Diagnostics | ____ |
| Registry | Image registry | ____ |
| DNS/CDN | Domain resolution | ____ |

---

## Appendix: Prometheus Metrics List

k8ops exposes the following custom metrics (at the `/metrics` endpoint):

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `k8ops_diagnostics_total` | Counter | phase, trigger | Total diagnostic reports |
| `k8ops_remediation_actions_total` | Counter | type, result, risk | Total remediation actions |
| `k8ops_llm_call_duration_seconds` | Histogram | provider, model, status | LLM call latency |
| `k8ops_llm_tokens_total` | Counter | provider, model, type | Token consumption |
| `k8ops_agent_steps` | Histogram | - | Agent execution steps |
| `k8ops_tool_call_duration_seconds` | Histogram | tool, success | Tool call latency |
| `k8ops_safety_blocks_total` | Counter | reason | Safety block count |
| `k8ops_active_diagnostics` | Gauge | - | Currently active diagnostics |
| `k8ops_active_remediations` | Gauge | - | Currently executing remediations |
| `k8ops_audit_events_total` | Counter | type, severity | Total audit events |
| `k8ops_cluster_health_score` | Gauge | - | Cluster health score (0-100) |
| `k8ops_conversation_count` | Gauge | - | Active conversations |
| `k8ops_tool_executions_total` | Counter | tool, success | Total tool executions |
| `k8ops_http_requests_total` | Counter | method, path, status | Total HTTP requests |
| `k8ops_http_request_duration_seconds` | Histogram | method, path | HTTP request latency |
| `k8ops_http_requests_in_flight` | Gauge | - | Currently in-flight requests |
| `k8ops_api_errors_total` | Counter | method, path, status | API errors (4xx+5xx) |

---

## 8. SLO/SLA Definitions

### 8.1 Service Level Objectives (SLO)

| Metric | Target | Measurement Window | Error Budget |
|--------|--------|---------------------|-------------|
| Dashboard availability | 99.9% | 30-day rolling | 43.2 minutes/month |
| API success rate (non-429) | 99.5% | 30-day rolling | 3.6 hours/month |
| API P99 latency | < 2s | Real-time | - |
| AI Chat response time | < 30s (first token) | Real-time | - |
| Security audit scan completion | < 60s | Real-time | - |

### 8.2 Error Budget Management

Monthly availability target of 99.9% = **43.2 minutes error budget**:

- **Within budget (<30min)**: Normal release cadence, no additional approval required
- **Budget warning (30-43min)**: Freeze non-urgent changes, prioritize reliability fixes
- **Budget exhausted (>43min)**: Full release freeze, conduct post-mortem

### 8.3 SLO Monitoring Queries (Prometheus PromQL)

**API error rate (5 minutes):**
```promql
sum(rate(k8ops_api_errors_total{status=~"5.."}[5m])) by (path)
/ sum(rate(k8ops_http_requests_total[5m])) by (path)
```

**API P99 latency:**
```promql
histogram_quantile(0.99,
  sum(rate(k8ops_http_request_duration_seconds_bucket[5m])) by (le, path)
)
```

**Error budget burn rate:**
```promql
1 - (
  sum(rate(k8ops_http_requests_total{status!~"5.."}[30d]))
  / sum(rate(k8ops_http_requests_total[30d]))
)
```

### 8.4 Degradation Strategy

When an SLO is about to be breached, degrade in priority order:

1. **Disable AI Chat** — Highest resource-consuming feature; degrading it does not affect core K8s management
2. **Increase cache TTL** — Increase overview/nodes/pods cache from 30s to 120s
3. **Limit concurrent diagnostics** — Lower the `k8ops_active_diagnostics` ceiling
4. **Disable event collector** — `--disable-event-collector` flag

### 8.5 Request Tracing

All HTTP responses include an `X-Request-ID` header, used for:
- Log correlation — All log lines for the same request share a request_id
- Audit tracing — The request_id in audit logs can be correlated to a specific HTTP request
- Troubleshooting — When users report an issue, providing the request_id enables quick log lookup

Log query example:
```bash
kubectl logs -n k8ops-system -l app.kubernetes.io/name=k8ops | grep "a1b2c3d4e5f6"
```

### 8.6 Log Level Configuration

k8ops uses structured JSON logging (slog) and supports configuring the level via the `LOG_LEVEL` environment variable or the `--log-level` command-line flag:

| Level | Purpose | Description |
|-------|---------|-------------|
| `debug` | Troubleshooting | Includes source file:line, very verbose logging (not recommended for production) |
| `info` | Default | Normal operation logs (recommended for production) |
| `warn` | Warnings only | Slow requests, configuration issues, approaching thresholds |
| `error` | Errors only | Logs only operation failures |

Configuration methods:
```bash
# Via environment variable (recommended)
kubectl set env daemonset/k8ops -n k8ops-system LOG_LEVEL=debug

# Via ConfigMap
kubectl patch configmap k8ops-config -n k8ops-system \
  --type='json' -p='[{"op":"add","path":"/data/log-level","value":"debug"}]'

# Via command-line argument (Deployment mode only)
# args:
# - --log-level=debug
```

Restart the Pod after changing the level:
```bash
kubectl rollout restart daemonset/k8ops -n k8ops-system
```

### 8.7 Log Rotation

The audit log file (`/data/k8ops-audit.jsonl`) is rotated automatically:
- **Automatic rotation**: File is split when it exceeds 100MB
- **Manual rotation**: `POST /api/system/log/rotate` (admin permission)
- **Clean up old files**: `POST /api/system/log/cleanup` (deletes rotated files older than 30 days)

Container stdout logs are managed by Kubelet, with a default limit of 10MB x 3 files = 30MB per container.
In k3s, this can be adjusted via `--container-log-max-size` and `--container-log-max-files`.

---

*Last updated: 2026-07-02*
*Maintainer: k8ops Team*
