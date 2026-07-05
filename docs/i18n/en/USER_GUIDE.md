# k8ops User Guide

> From installation to mastery: a detailed guide covering all features.

---

## Table of Contents

1. [Quick Start](#1-quick-start)
2. [Cluster Overview](#2-cluster-overview)
3. [AI Chat — Intelligent Assistant](#3-ai-chat--intelligent-assistant)
4. [Diagnosis and Remediation](#4-diagnosis-and-remediation)
5. [Optimization Recommendations](#5-optimization-recommendations)
6. [Cost Analysis (FinOps)](#6-cost-analysis-finops)
7. [Cluster Topology Visualization](#7-cluster-topology-visualization)
8. [Node and Pod Management](#8-node-and-pod-management)
9. [Event Stream and Notifications](#9-event-stream-and-notifications)
10. [Resource Browser and YAML Editor](#10-resource-browser-and-yaml-editor)
11. [RBAC Access Control](#11-rbac-access-control)
12. [Audit Logs](#12-audit-logs)
13. [Settings and Configuration](#13-settings-and-configuration)
14. [Keyboard Shortcuts](#14-keyboard-shortcuts)
15. [Theme Switching](#15-theme-switching)
16. [Capacity Planning](#16-capacity-planning)
17. [HPA Visualization](#17-hpa-visualization)
18. [Container Image Inventory](#18-container-image-inventory)
19. [Namespace Resource Ranking](#19-namespace-resource-ranking)
20. [Security and Compliance](#20-security-and-compliance)
21. [System Administration](#21-system-administration)
22. [Operations Diagnostic API](#22-operations-diagnostic-apiv1461)

---

## 1. Quick Start

### First Login

1. Open your browser and navigate to the k8ops address (e.g., `https://k8ops.iot2.win` or `http://localhost:9090`)
2. Default credentials: `admin` / `admin`
3. You will be prompted to change your password on first login

### Page Layout

```
┌─────────┬───────────────────────────────┐
│         │  [Namespace ▼]  [🔔]  [☀/☽]  │  ← Top bar
│ Sidebar ├───────────────────────────────┤
│         │                                │
│ Overview│       Content Area             │  ← Content area
│ Diagnose│                                │
│ Nodes   │                                │
│ Pods    │                                │
│ ...     │                                │
└─────────┴───────────────────────────────┘
```

### Ctrl+K Command Palette

Press `Ctrl+K` (Mac: `Cmd+K`) at any time to open the global command palette:

- Type `nodes` → navigate to the Nodes page
- Type `chat` → open AI Chat
- Type `cost` → view cost analysis
- Use arrow keys to navigate, Enter to confirm, Esc to close

---

## 2. Cluster Overview

The Overview page displays the overall status of the cluster.

### Statistics Cards

| Card | Description |
|------|-------------|
| Nodes | Total cluster nodes / Ready count |
| Pods | Running pods / Total pods |
| CPU | Cluster-wide CPU utilization |
| Memory | Cluster-wide memory utilization |
| Warnings | Current number of Warning events |

### Sparkline Charts

Each card includes a mini SVG line chart showing the trend over the last 30 minutes.

### Namespace Switching

The dropdown selector on the left side of the top bar allows you to switch the namespace scope. Switching affects the Pods, Events, Nodes, and other pages. The selection is persisted to localStorage.

---

## 3. AI Chat — Intelligent Assistant

Click the Chat button at the bottom of the sidebar, or press `Ctrl+K` and type `chat` to open it.

### Basic Usage

Type your question in the input box, and the AI will:

1. Understand your natural-language intent
2. Automatically invoke the appropriate Kubernetes tools
3. Stream the analysis results back to you

### Example Queries

```
# View resources
Show pods in the default namespace
Which nodes have high CPU usage?

# Troubleshooting
Why are the nginx-deployment pods in CrashLoopBackOff?
What's wrong with the cluster?

# Optimization suggestions
Help me analyze resource usage
Which pods can have their replica count reduced?
```

### Tool Call Transparency

When the AI executes tool calls, a collapsible Thinking panel is displayed:

- Click to expand and view the parameters and results of each tool call
- Results are shown in formatted JSON with search support

### Diagnostic Suggestion Cards

When the AI suggests running a kubectl command, action buttons appear below the code block:

- **▶ Run in Chat** — Loads the command into the input box for convenient execution
- **📋 Copy** — Copies the command to your clipboard

### Session Management

- **New** — Create a new session
- **Session list on the left** — Click to switch between historical sessions
- Sessions are automatically summarized and compressed (auto-triggered when exceeding 20k tokens)

### Markdown Rendering

Chat supports:
- Code blocks (with syntax highlighting and copy button)
- Tables
- Lists, bold, italic
- Links (http/https/mailto protocols only)

---

## 4. Diagnosis and Remediation

### Triggering Diagnostics

**Method 1: Web Interface**

1. Navigate to the Diagnostics page
2. Click "New Diagnostic"
3. Fill in the problem description (e.g., "API responses are slow in the production namespace")
4. Submit and the AI will analyze automatically

**Method 2: AI Chat**

Describe the problem directly in Chat, and the AI will automatically execute the diagnostic workflow.

**Method 3: CRD**

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

**Method 4: CLI**

```bash
k8ops diagnose --problem "pods in production keep CrashLoopBackOff"
```

### Diagnostic Results

Each diagnostic report includes:

- **Root Cause** — The root cause identified by AI analysis
- **Evidence** — Logs, events, and metric data supporting the analysis
- **Recommendations** — Suggested remediation actions
- **Severity** — Severity level (Info / Warning / Critical)

### Automated Remediation

Remediation plans generated by the AI require manual approval:

1. Navigate to the Remediations page
2. Review the pending remediation plans
3. Click **Approve** to execute, or **Reject** to decline
4. All actions are recorded in the audit log

---

## 5. Optimization Recommendations

The Optimizations page displays AI-generated resource optimization suggestions for the cluster.

### Recommendation Types

| Type | Description |
|------|-------------|
| Resource Rightsizing | Suggested CPU/Memory requests and limits adjustments |
| HPA Gap | Deployments missing horizontal autoscaling configuration |
| PDB Gap | Workloads missing PodDisruptionBudget |
| Cost Saving | Potential cost savings (idle resources, excess replicas, etc.) |

### Actions

- Click a recommendation to view details
- Apply directly or dismiss

---

## 6. Cost Analysis (FinOps)

The Cost page provides cost visibility for the cluster.

### Features

- **Namespace Cost Summary** — Resource consumption and estimated costs per namespace
- **Resource Utilization** — Actual CPU/Memory usage vs. allocation
- **Rightsizing Recommendations** — Suggestions for adjusting over-allocated resources
- **Idle Resources** — Long-unused PVs, LoadBalancers, elastic IPs, etc.

---

## 7. Cluster Topology Visualization

The Topology page displays node and Pod relationships as an SVG diagram.

### Visual Elements

| Element | Description |
|---------|-------------|
| Green box | Ready node |
| Red box | NotReady node |
| Progress bar inside node box | CPU (top) / MEM (bottom) utilization |
| Green pod dot | Running |
| Yellow pod dot | Pending |
| Red pod dot | Failed |
| Flashing pod border | CrashLoop (restarts > 3) |

### Interactions

- **Click a Pod** — Opens the log viewer for that Pod
- **Bottom statistics** — Ready/NotReady node counts, Pod status summary

---

## 8. Node and Pod Management

### Nodes Page

- Node list table: name, role, status, CPU, memory, Pod count
- Each column supports search filtering
- Click a node name to view detailed information and all Pods on that node

### Pods Page

- Pod list table: name, namespace, status, restart count, node, age
- Supports namespace filtering and real-time search

### Pod Log Viewer

Click a Pod row to open the log viewer:

- **Real-time streaming** — SSE push, logs update in real time
- **Log level highlighting** — ERROR (red), WARN (yellow), DEBUG (gray)
- **Search filtering** — Type keywords to filter log lines
- **Auto-scroll** — Automatically scrolls to the bottom when new logs arrive (can be paused)
- **Download** — Export current logs as a file

---

## 9. Event Stream and Notifications

### Events Page

Displays Kubernetes cluster events with support for:

- Real-time search filtering
- Red highlighting for Warning events
- Filtering by namespace

### Real-time Event Stream

The Events page includes a Live Events panel on the right side:

- Click **Go Live** to enable real-time SSE push
- New events display a blue NEW badge animation
- Deleted events display a red DEL badge
- Warning events are automatically highlighted in red

### Notification Center

The bell icon on the right side of the top bar:

- Displays a red numeric badge with a pulse animation when there are alerts
- Click to expand the dropdown panel
- Shows recent Warning events and NotReady nodes
- Auto-refreshes every 60 seconds

---

## 10. Resource Browser and YAML Editor

### Resources Page

Browse all Kubernetes resources in the cluster:

- Grouped by API Group / Resource Type
- Click a resource name to view its YAML definition
- Supports multi-select namespace filtering

### YAML Viewer

Click any resource to open a YAML overlay:

- Displays the complete YAML in formatted view
- **Copy** button for one-click copying

### YAML Editor

Click the **Edit** button in the YAML viewer to enter edit mode:

1. The YAML content becomes an editable textarea
2. Make changes and click **Apply** to submit
3. The backend uses server-side apply (kubectl apply semantics)
4. Success shows a green notification; failure shows a red error message

---

## 11. RBAC Access Control

The RBAC page (requires admin permissions) manages users and roles.

### User Management

- **Create user** — Username, password, role, namespace scope
- **Edit user** — Modify role, enable/disable
- **Delete user**

### Roles

| Role | Permissions |
|------|-------------|
| admin | Full cluster read/write, can manage users |
| operator | Most resources read/write, cannot manage RBAC/Secrets |
| viewer | Read-only access |

### Namespace Scope

Each user can be bound to specific namespaces and can only access resources within that scope (implemented via K8s impersonation).

---

## 12. Audit Logs

The Audit page displays audit records for all AI operations.

### Features

- **Severity filter** — Dropdown to select Info / Warning / Error / Critical
- **Real-time search** — Type keywords to filter
- **Statistics cards** — Total / Successful / Failed / Critical / Warnings
- **Table** — Time, severity, action, target resource, operator, success/failure, duration

### Audit Scope

All of the following operations are logged:

- AI tool calls (kubectl get/describe/logs, etc.)
- AI-initiated remediation actions
- LLM API calls
- User login/logout
- Resource modifications

---

## 13. Settings and Configuration

The Settings page configures the AI Provider and authentication.

### AI Provider Configuration

| Field | Description |
|-------|-------------|
| Provider Type | openai / deepseek / zai / anthropic |
| Model | gpt-4o / deepseek-chat / glm-4-plus, etc. |
| Endpoint | LLM API address (leave empty to use default) |
| API Key | LLM API key |

### Authentication Configuration

- **Local** — Built-in user system (default)
- **LDAP** — Enterprise LDAP/AD integration
- **OIDC** — GitHub / Google / Keycloak, etc.

---

## 14. Keyboard Shortcuts

| Shortcut | Function |
|----------|----------|
| `Ctrl+K` / `Cmd+K` | Open command palette |
| `Esc` | Close command palette / popup |
| `↓` / `↑` | Navigate in command palette |
| `Enter` | Confirm in command palette |

---

## 15. Theme Switching

Click the moon/sun button in the upper right corner of the sidebar to toggle between dark and light themes. The selection is persisted to localStorage and maintained after page refresh.

---

## Appendix

### Related Documentation

- [Architecture Design](ARCHITECTURE.md)
- [Deployment Guide](DEPLOYMENT.md)
- [Local Run](LOCAL_RUN.md)
- [API Reference](API.md)
- [Security Policy](SECURITY.md)

### Frequently Asked Questions

**Q: Chat is not responding?**
A: Check whether the Settings → Provider configuration is correct and the API Key is valid.

**Q: Can't see certain namespaces?**
A: The current user's RBAC role may restrict namespace access scope. Contact your administrator to adjust.

**Q: Pod log viewer is blank?**
A: The Pod may have just started and has no logs yet, or the user may lack log permissions. Check the RBAC configuration.

**Q: Are the AI-suggested commands safe?**
A: All AI-suggested operations first go through the Safety Checker's dry-run validation, and remediation actions require manual approval before execution.

---

## 16. Capacity Planning

### Storage Capacity Monitoring

**Path:** Dashboard → Capacity tab

Displays the storage status of all PVCs (PersistentVolumeClaims) in the cluster:

| Metric | Description |
|--------|-------------|
| Total PVCs | Total number of PVCs in the cluster |
| Bound | Number of PVCs bound to a PV |
| Pending | PVCs waiting to be bound |
| Total Capacity | Total capacity across all PVCs |
| Requested | Total requested capacity across all PVCs |

### Node Capacity Analysis

The Capacity page also displays resource utilization for each node:

- **CPU utilization**: Requested CPU / Allocatable CPU (color-coded: <60% green, 60-80% yellow, >80% red)
- **Memory utilization**: Requested memory / Allocatable memory
- **Pod density**: Running Pods / Maximum Pod limit
- **Scale-up recommendations**: Automatically generates scale-up recommendations when node resources exceed 80%

### Cluster-Level Summary

| Metric | Description |
|--------|-------------|
| Cluster CPU Utilization | Cluster-wide CPU requested/allocatable ratio |
| Cluster Mem Utilization | Cluster-wide memory requested/allocatable ratio |
| Total CPU Allocatable | Total allocatable CPU across the cluster |
| Total CPU Requested | Total requested CPU across the cluster |

---

## 17. HPA Visualization

**Path:** Dashboard → HPA tab

Displays the autoscaling status of all HorizontalPodAutoscalers:

### Features

- **Replica scaling bar**: Visualizes current replica count, desired replica count, and min/max range
- **Metric utilization bar**: Current CPU/memory utilization vs. target value (green/yellow/red)
- **Scaling status indicator**: Shows a "SCALING" badge when current replicas ≠ desired replicas
- **Summary cards**: Total HPA count, actively scaling count, total current/desired replicas

### Supported Metric Types

| Type | Description |
|------|-------------|
| Resource | CPU/memory utilization percentage |
| Pods | Custom Pod metrics (e.g., QPS) |
| External | External metrics (e.g., SQS queue length) |
| ContainerResource | Container-level resource metrics |

---

## 18. Container Image Inventory

**Path:** Dashboard → Images tab

Displays all container images currently in use across the cluster:

| Metric | Description |
|--------|-------------|
| Unique Images | Total number of unique images after deduplication |
| Using :latest | Number of images using the `:latest` tag (not recommended for production) |
| No Limits | Number of images without resource limits set |
| No Requests | Number of images without resource requests set |
| Registries | Number of image registries in use |

### Security Best Practices

- Avoid using `:latest` tags — Use fixed version numbers for reproducible deployments
- All containers should have CPU/memory limits set — Prevents resource exhaustion
- All containers should have CPU/memory requests set — Ensures proper scheduler allocation

---

## 19. Namespace Resource Ranking

**Path:** Dashboard → Namespaces tab

Lists resource usage for all namespaces sorted by CPU consumption:

### Features

- **Resource summary**: CPU/memory requests + limits, Pod count, PVC storage for each namespace
- **Cluster share**: Percentage of CPU/memory requests relative to total cluster allocatable (with visual progress bars)
- **Search filtering**: Quickly locate a specific namespace
- **Detail drill-down**: Click any namespace to view ResourceQuota usage, LimitRange configuration, and recent warning events

---

## 20. Security and Compliance

### CIS Benchmark Compliance Scan

**Path:** Dashboard → Compliance tab

Runs CIS Kubernetes Benchmark checks covering the following categories:

| Category | Check Items |
|----------|-------------|
| RBAC | cluster-admin binding scope, wildcard ClusterRoles, default SA usage |
| Pod Security | privileged containers, hostNetwork/hostPID/hostIPC, hostPath volumes, root user, resource limits |
| Network | NetworkPolicy coverage |
| Secrets | Secret management health |

### Compliance Report Download

Click the "Download Report" button to download a full compliance report (.txt format) containing:

- Compliance score (percentage)
- Status of each check (PASS/WARN/FAIL)
- Remediation recommendations (for WARN/FAIL items)

### Audit Event Search

**Path:** API → `GET /api/audit/events`

Supports multi-dimensional filtering of audit logs:

| Parameter | Description |
|-----------|-------------|
| `actor` | Filter by username |
| `action` | Filter by action type (e.g., delete, scale, exec) |
| `q` | Full-text search |
| `severity` | Filter by severity level |
| `from`/`to` | Time range (RFC3339 format) |

### CSV Export

`GET /api/audit/export` — Exports audit logs in CSV format, which can be imported into SIEM systems for compliance analysis.

---

## 21. System Administration

### System Information

`GET /api/system/info` provides runtime information:

- Version number, Go version, runtime platform
- Memory usage (Alloc/Sys/GC cycles/Heap objects)
- Goroutine count
- Service uptime
- Audit log size and event count

### Log Management

| API | Function |
|-----|----------|
| `POST /api/system/log/rotate` | Manually trigger audit log rotation (admin) |
| `POST /api/system/log/cleanup` | Clean up rotated files older than 30 days (admin) |

### Log Level Configuration

Configure via the `LOG_LEVEL` environment variable (debug/info/warn/error):

```bash
kubectl set env daemonset/k8ops -n k8ops-system LOG_LEVEL=debug
kubectl rollout restart daemonset/k8ops -n k8ops-system
```

### Backup Management

| API | Function |
|-----|----------|
| `GET /api/system/backup` | List all backup files |
| `POST /api/system/backup` | Create a database backup |
| `DELETE /api/system/backup?name=X` | Delete a specific backup |
| `POST /api/system/backup/restore?name=X` | Restore the database from a backup |

### API Performance Monitoring

`GET /api/system/performance` provides latency statistics for each API endpoint:

- **p50/p95/p99** percentile latency
- Average and maximum latency
- Error rate and total request count

---

## 22. Operations Diagnostic API (v14.61+)

### Network Policy Audit

`GET /api/security/network-policies` audits the cluster's NetworkPolicy coverage:

- Detects namespaces without NetworkPolicies (default fully open)
- Identifies permissive policies (0.0.0.0/0 ingress/egress)
- Categorizes by severity: critical / warning / info
- Each finding includes a detailed description and remediation recommendation

### Pod Restart Diagnostics

`GET /api/diagnostics/restarts` diagnoses Pod restart patterns and root causes:

- Classifies restart patterns: crash-loop / occasional / post-deploy
- Extracts termination reasons: OOMKilled / Error / exit code
- Identifies waiting states: CrashLoopBackOff / ImagePullBackOff
- Independent diagnostic information per container

### Deployment Rollout Status

`GET /api/deployments/rollout` tracks the rollout health of all workloads:

- Covers Deployment / StatefulSet / DaemonSet
- 7 states: complete / in-progress / stalled / degraded / paused / failed / scaled-to-zero
- Detects ProgressDeadlineExceeded and ReplicaFailure
- Supports filtering by status: `?status=failed`

### Resource Waste Detection

`GET /api/resources/waste` scans for wasted and orphaned resources to reduce costs:

- 6 waste categories: dead services, unused PVCs, orphaned ConfigMaps/Secrets, empty namespaces, unbound PVs
- Cost risk assessment: low / moderate / high
- Each item includes severity, age, and cleanup recommendations
- Intelligently filters system resources (kube-system, SA tokens, Helm releases)

### Scaling Bottleneck Detection

`GET /api/scaling/bottlenecks` identifies factors limiting horizontal scaling:

- 7 bottleneck types: node scheduling, node pressure, quota limits, stuck HPA, PDB blocking, storage exhaustion
- Cluster capacity summary: node count, CPU/memory, Pod capacity, scaling headroom
- Each item includes impact level and remediation recommendations

### RBAC Permission Risk Analysis

`GET /api/security/rbac-risk` analyzes the security risk of all RBAC bindings in the cluster:

- 0-100 scoring system, automatically identifies high-risk bindings
- 5 risk levels: critical / high / elevated / moderate / low
- Detection items: cluster-admin bindings, privilege escalation (escalate/bind/impersonate), wildcard permissions (verbs/resources: *), cluster-wide write operations, sensitive resource access (secrets/pods/exec)
- Each item includes detailed scoring breakdown and remediation recommendations (least privilege principle)
- Supports filtering by namespace: `?namespace=default`

### CronJob Execution Health Monitoring

`GET /api/operations/cronjobs/health` monitors the execution health of all CronJobs:

- 5 health states: healthy / warning / failing / suspended / no-runs
- Detects consecutive failures (3+ = failing), success rate below 50%, suspended schedules, never executed
- Associates CronJobs with their child Jobs via OwnerReferences
- Calculates next expected run time
- Supports filtering by namespace: `?namespace=production`

### Service & Endpoint Network Health Monitoring

`GET /api/networking/health` scans the network connectivity of all Services and Ingresses:

- 5-level Service health states: healthy / degraded / no-endpoints / misconfigured / external
- Detects selector mismatch (label mismatch), all endpoints unavailable, partial degradation, LoadBalancer waiting for IP
- Ingress backend validation: whether backend Service exists and has available endpoints
- Cross-references Pod selector matching for root cause analysis
- Supports filtering by namespace: `?namespace=default`

### PV/PVC Storage Health Monitoring

`GET /api/storage/health` scans the storage health of all PVCs/PVs:

- 6-level PVC health states: bound / pending / lost / failed / orphaned / near-capacity
- Pending diagnostics: no storage class, WaitForFirstConsumer binding mode, provisioner log inspection
- Orphaned PVC detection: bound but unused by any Pod for over 1 day (capacity waste)
- PV issues: Released (needs manual cleanup), Failed (reclaim failed), stale Available (>7 days)
- Storage class distribution: default class flag, provisioner, reclaim policy, volume expansion support
- Supports filtering by namespace: `?namespace=default`

### ServiceAccount Security Audit

`GET /api/security/service-accounts` comprehensively audits the security risks of all ServiceAccounts in the cluster:

- 0-100 risk scoring system, automatically identifies high-risk SAs
- 5 severity levels: critical / high / elevated / moderate / low
- Detection items: unused SAs (>7 days), cluster-admin bindings (critical), default SA used by Pods, unnecessary automatic token mounting, stale SAs (>30 days with permissions but unused), legacy long-lived token secrets
- Each item includes detailed security risk explanation and remediation recommendations
- Supports filtering by namespace: `?namespace=default`

### SLO/SLA Error Budget Tracking

`GET /api/operations/slo` tracks SLO/SLA compliance using a multi-window, multi-burn-rate algorithm:

- 5 time windows: 5 minutes, 1 hour, 6 hours, 24 hours, 7 days
- Availability percentage and error budget remaining amount / burn rate
- Multi-window burn rate detection (fast: 5m+1h, slow: 6h+24h)
- P50/P95/P99 latency percentiles and SLO targets
- 3 status levels: meeting (compliant) / at-risk / violated
- Supports filtering by namespace: `?namespace=production`

### ResourceQuota and LimitRange Monitoring

`GET /api/resources/quota` scans quota utilization and LimitRange constraints across all namespaces:

- 4 quota states: ok (<70%) / warning (70-85%) / critical (85-100%) / exceeded (>100%)
- Per-namespace CPU/memory/Pod/ConfigMap/Secret/storage quota utilization
- Identifies namespaces without quota protection
- LimitRange default/min/max constraint analysis
- Top consumer rankings
- Supports filtering by namespace: `?namespace=default`

### Deployment Configuration Audit

`GET /api/deployments/audit` audits best-practice configuration violations across all workloads:

- 8 check categories: revision-history / image-policy / resources / probes / security-context / update-strategy / lifecycle / config-drift
- Each item includes severity (critical/warning/info), specific problem description, and actionable remediation recommendations
- Health score from 0 (perfect) to 100 (worst)
- Aggregated Top Findings show the most common issues cluster-wide
- Supports filtering by namespace and severity: `?namespace=default&severity=critical`

### Scheduling Health and Resource Fragmentation Analysis

`GET /api/scheduling/health` analyzes cluster scheduling health and resource utilization:

- Per-node schedulability (cordoned/tainted/pressure conditions) and resource availability
- Pending Pod diagnostics: parses FailedScheduling event reasons (insufficient CPU/memory, taint mismatch, nodeSelector conflict, volume binding failure, etc.)
- Maximum schedulable Pod calculation (how large a Pod can currently be deployed)
- Effective capacity vs. theoretical capacity (capacity loss from unschedulable nodes)
- Resource fragmentation analysis (scattered free capacity)
- Oversized Pod detection (requests exceeding any single node's capacity)
- 24h eviction history (with reasons)
- Health score 0-100 (weighted penalties)
- Actionable remediation recommendations
- Supports filtering by namespace: `?namespace=default`

### Pod Security Posture Scan

`GET /api/security/pods?namespace=xxx&severity=critical` audits the real-time security posture of all running Pods:

- 15 check categories covering privileged containers, host access (network/PID/IPC), HostPath mounts, dangerous capabilities, running as root, privilege escalation, etc.
- Per-Pod risk score 0-100 (critical=25 points/warning=8 points/info=2 points)
- Aggregated statistics by check type and namespace
- Supports filtering by namespace and severity

### Event Storm and Cascading Failure Detection

`GET /api/operations/event-storm?namespace=xxx` analyzes cluster Warning events:

- 4 storm severity levels: critical (>50) / high (>20) / medium (>10) / low (>5)
- Flapping resource detection (same resource, same reason repeated 3+ times, with flapping frequency)
- Aggregation by namespace and event reason
- Blast radius assessment (number of affected resources)
- Actionable troubleshooting recommendations
- Supports filtering by namespace: `?namespace=kube-system`

### Resource Dependency Graph and Blast Radius Analysis

`GET /api/dependencies?kind=Deployment&name=xxx&namespace=xxx` traces the complete dependency graph of a workload:

- Forward dependencies: ConfigMap, Secret, PVC, ServiceAccount
- Reverse dependencies: Service (label selector), Ingress, NetworkPolicy, HPA, other Pods sharing the configuration
- Blast radius assessment: blastRadius score and risk level
- Used for pre-change impact assessment to prevent cascading failures

### Topology Spread Compliance Check

`GET /api/topology/spread?namespace=xxx&domain=topology.kubernetes.io/zone` checks Pod topology spread compliance:

- 4-level workload status: balanced / skewed / no-constraint / single-replica
- Topology domain distribution and skew analysis per workload
- Detects multi-replica workloads missing topology constraints
- Identifies nodes missing topology labels
- Single-domain cluster hints
- Supports filtering by namespace and topology domain key

### Secret Rotation and Lifecycle Audit

`GET /api/security/secrets/rotation?namespace=xxx` audits the lifecycle of all Secrets:

- Age tracking: stale (>90d) / very stale (>180d)
- Unused Secret detection (not referenced by any Pod)
- TLS certificate expiration detection (parses certificates, detects expired and <30d to expiry)
- Docker registry Secret, legacy SA token tracking
- Sensitive name detection (password/key/token/credential)
- Per-Secret risk level, cluster rotation score 0-100
- Supports filtering by namespace

### Health Probe Effectiveness Audit

`GET /api/operations/probes?namespace=xxx` audits probe configuration:

- 8 check categories: missing probes, too aggressive, short timeouts, improper thresholds, etc.
- Per-workload risk score, cluster effectiveness score (0-100)
- Aggregated top issue statistics
- Actionable recommendations

### Workload Staleness Tracking

`GET /api/product/staleness?namespace=xxx` tracks deployment staleness:

- 5-level staleness classification: fresh/recent/stale/very-stale/ancient
- Image tag analysis: :latest, digest, no-tag
- Age distribution buckets, namespace statistics
- Cluster freshness score (0-100)

### Resource Overcommit and Pressure Analysis

`GET /api/scalability/overcommit?namespace=xxx` analyzes resource overcommit:

- Per-node CPU/memory request and limit overcommit ratios
- Pressure score 0-100 and risk level
- Pod detection without limits/requests
- Cluster safety score 0-100
- Namespace resource consumption breakdown

### Image Security and Supply Chain Analysis

`GET /api/security/images?namespace=xxx` scans the supply chain security of all container images:

- Digest pinning detection (@sha256: immutable reference)
- :latest tag detection (mutable, non-reproducible)
- Untagged image detection (defaults to :latest)
- Old version tag detection (v1, 1.0 — may contain known CVEs)
- Public vs. private image registry analysis
- Per-image risk level, per-registry statistics
- Cluster image security score 0-100

### Capacity Planning

`GET /api/capacity/planning` node capacity planning:

- Per-node CPU/memory requests vs. allocatable
- Remaining capacity and scale-up recommendations
- Resource fragmentation detection

### Capacity Forecasting

`GET /api/capacity/forecast` capacity trend forecasting:

- Resource growth trends based on historical data
- Estimated exhaustion time
- Scale-up recommendations

### Resource Efficiency Analysis

`GET /api/efficiency` resource usage efficiency:

- Oversized resource allocation detection
- Resource waste identification
- Optimization recommendations

### PDB Status

`GET /api/pdbs` Pod Disruption Budget status:

- PDB configuration checks
- Allowed disruptions vs. current available count
- PDB blocking detection

### Version Compatibility

`GET /api/compatibility` Kubernetes version compatibility:

- API deprecation checks
- Resource version compatibility
- Upgrade impact assessment

### Certificate Expiry

`GET /api/certificates/expiry` TLS certificate expiry scanning:

- Cluster certificate expiration times
- Expiring certificate warnings
- Renewal recommendations

### Addon Health

`GET /api/addons/health` cluster addon health check:

- Core addon running status
- Abnormal addon detection
- Remediation recommendations

### Cluster Health Score

`GET /api/operations/health-score` aggregates all cluster health signals into a comprehensive score:

- 5 weighted dimensions: Node(25%) + Pod(25%) + Workload(20%) + Events(15%) + API Server(15%)
- Total score 0-100, letter grade A-F
- Status: healthy / warning / critical
- Per-dimension score, weight, and details
- Cluster summary: node/Pod/workload counts
- Top issues sorted by severity

### HPA/VPA Resource Rightsizing Recommendations

`GET /api/scalability/autoscale-recommendations?namespace=xxx` analyzes autoscaling and resource rightsizing:

- Detects multi-replica workloads missing HPA
- Excessive CPU requests (>1 core/container)
- Excessive memory requests (>2GB/container)
- HPA efficiency analysis (at ceiling/floor/idle)
- Per-workload current vs. recommended resource values
- Potential CPU core and memory savings
- Cluster autoscaling score 0-100

### Ingress and Traffic Routing Health Monitoring

`GET /api/product/ingress-health?namespace=xxx` checks the traffic routing health of all Ingresses:

- Backend Service existence and endpoint readiness checks
- TLS configuration detection
- IngressClass validity validation
- host+path conflict detection
- No routing rule detection
- Per-Ingress status and cluster health score 0-100

### Node Conditions and Resource Pressure

`GET /api/operations/node-pressure` analyzes conditions and resource pressure for all nodes:

- DiskPressure / MemoryPressure / PIDPressure / NetworkUnavailable detection
- CPU/memory/Pod utilization vs. allocatable
- Per-node risk level (critical/high/medium/low)
- Cluster pressure score 0-100

### PVC Binding and Storage Performance

`GET /api/scalability/pvc-analysis?namespace=xxx` analyzes storage binding health:

- Stuck PVC root cause detection (>5min pending)
- Binding time measurement and slow binding detection (>30s)
- Lost PVC detection
- Per-StorageClass statistics and provisioner analysis
- Cluster storage health score 0-100

### Namespace Governance and Lifecycle

`GET /api/product/namespaces/lifecycle` audits namespace governance:

- ResourceQuota / LimitRange / NetworkPolicy coverage
- Dedicated ServiceAccount detection (least privilege)
- Required label checks (app, team, env, owner)
- Namespace lifecycle (active / stale / terminating)
- Cluster governance score 0-100

### RBAC Effective Permissions and Privilege Escalation Analysis

`GET /api/security/rbac-effective` analyzes the effective RBAC permissions of all subjects:

- Aggregates ClusterRoleBindings + RoleBindings to compute actual permissions
- cluster-admin equivalence detection
- Privilege escalation path detection (subjects who can modify RBAC)
- Wildcard (*) permission detection
- Secret read and Pod exec access analysis
- Cluster RBAC security score 0-100

### Container OOM Kill Tracking

`GET /api/operations/oom-tracker?namespace=xxx` tracks container OOM events:

- OOMKilled container detection and root cause analysis
- High restart count detection (>=5)
- Missing or too-low memory limit detection
- Node pressure risk from limits far exceeding requests (10x+)
- Top OOM rankings and per-namespace statistics
- Cluster OOM risk score 0-100

### Storage Capacity Exhaustion Forecasting

`GET /api/scalability/storage-forecast` forecasts storage capacity:

- Per-PV utilization, growth rate, days-to-exhaustion prediction
- Longhorn actual-size annotation support
- Cluster storage-full day estimation
- Per-StorageClass statistics and provisioner analysis
- High-risk namespace rankings
- Storage health score 0-100

### DNS Resolution Health Check

`GET /api/product/dns-health` analyzes DNS resolution health:

- CoreDNS Pod health check (running/ready/restarts/version)
- Corefile configuration analysis (forwarders, plugins)
- Headless Service endpoint coverage and NXDOMAIN risk
- NodeLocal DNS cache detection
- Pod dnsConfig ndots coverage detection
- External-DNS managed service discovery
- Cluster DNS health score 0-100
