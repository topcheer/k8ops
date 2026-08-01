// audit-dashboard.js — Unified Audit Dashboard (v2: categorized, searchable, consolidated)
import { escapeHtml, fetchJSON } from './modules/utils.js';

// Consolidated endpoint registry: subcategories within each dimension
// "primary" marks the canonical endpoint; "alias" entries are visually grouped under it
const AUDIT_STRUCTURE = {
  'Security': {
    color: '#f85149',
    icon: '\u{1F6E1}',
    subcategories: {
      'RBAC & Access': [
        { path: '/api/security/rbac-audit', name: 'RBAC Audit', icon: '\u{1F511}' },
        { path: '/api/security/sa-token-audit', name: 'SA Token Audit', icon: '\u{1F510}' },
        { path: '/api/security/sa-token-lifecycle', name: 'SA Token Lifecycle', icon: '\u{1F511}' },
        { path: '/api/security/service-accounts', name: 'Service Accounts', icon: '\u{1F464}' },
        { path: '/api/security/rbac-effective', name: 'Effective RBAC', icon: '\u{1F50D}' },
        { path: '/api/security/rbac-graph', name: 'RBAC Graph', icon: '\u{1F5FA}' },
        { path: '/api/security/rbac-risk', name: 'RBAC Risk', icon: '\u26A0' },
        { path: '/api/security/rbac-blast', name: 'RBAC Blast Radius', icon: '\u{1F4A5}' },
        { path: '/api/security/privilege-escalation-path', name: 'Privilege Escalation', icon: '\u{1F6A8}' },
        { path: '/api/security/rbac-drift', name: 'RBAC Drift', icon: '\u{1F501}' },
      ],
      'Secrets Management': [
        { path: '/api/security/secret-scan', name: 'Secret Scanner', icon: '\u{1F576}', primary: true },
        { path: '/api/security/secret-rotation-v2', name: 'Secret Rotation', icon: '\u{1F504}', alias: ['secret-rotation-plan', 'secret-rotation', 'secrets/rotation'] },
        { path: '/api/security/secret-exposure', name: 'Secret Exposure', icon: '\u{1F441}' },
        { path: '/api/security/secret-posture', name: 'Secret Posture', icon: '\u{1F6E1}' },
        { path: '/api/security/secret-lifecycle', name: 'Secret Lifecycle', icon: '\u{1F501}' },
        { path: '/api/security/secret-age', name: 'Secret Age', icon: '\u23F0' },
        { path: '/api/security/secret-spray', name: 'Secret Spray', icon: '\u{1F510}' },
        { path: '/api/security/env-leak-scanner', name: 'Env Leak Scanner', icon: '\u{1F576}' },
      ],
      'Pod Security (PSP/PSS)': [
        { path: '/api/security/pss-scorecard', name: 'PSS Scorecard', icon: '\u{1F6E1}' },
        { path: '/api/security/pss-hardening', name: 'PSS Hardening', icon: '\u{1F6E1}' },
        { path: '/api/security/psa-audit', name: 'PSA Audit', icon: '\u{1F6AB}' },
        { path: '/api/security/seccomp-audit', name: 'Seccomp Audit', icon: '\u{1F6E1}' },
        { path: '/api/security/seccomp-profile-gap', name: 'Seccomp Gap', icon: '\u{1F6E1}' },
        { path: '/api/security/container-hardening', name: 'Container Hardening', icon: '\u{1F6E1}' },
        { path: '/api/security/privilege-map', name: 'Privilege Map', icon: '\u{1F512}' },
        { path: '/api/security/mac-audit', name: 'MAC Audit', icon: '\u{1F512}' },
        { path: '/api/security/hostpath-audit', name: 'HostPath Audit', icon: '\u{1F4C1}' },
        { path: '/api/security/container-capabilities', name: 'Cap Audit', icon: '\u{1F511}' },
        { path: '/api/security/readonly-rootfs-audit', name: 'Readonly RootFS', icon: '\u{1F512}' },
        { path: '/api/security/runtime-class-audit', name: 'Runtime Class', icon: '\u{1F9E9}' },
      ],
      'Network Security': [
        { path: '/api/security/network-policies', name: 'Network Policies', icon: '\u{1F6E1}' },
        { path: '/api/security/netpol-generator', name: 'NetPol Generator', icon: '\u{1F527}' },
        { path: '/api/security/net-policy-effectiveness', name: 'NetPol Effectiveness', icon: '\u{1F50D}' },
        { path: '/api/security/mtls-trust-domain', name: 'mTLS Trust', icon: '\u{1F511}' },
        { path: '/api/security/endpoint-exposure', name: 'Endpoint Exposure', icon: '\u{1F441}' },
        { path: '/api/security/network-segment-gap', name: 'Segment Gap', icon: '\u{1F310}' },
      ],
      'Compliance & Policy': [
        { path: '/api/security/compliance-map', name: 'Compliance Map', icon: '\u{1F4CB}' },
        { path: '/api/security/compliance-posture', name: 'Compliance Posture', icon: '\u{1F4DC}' },
        { path: '/api/security/compliance-gap', name: 'Compliance Gap', icon: '\u{1F4CB}' },
        { path: '/api/security/kyverno-compliance', name: 'Kyverno', icon: '\u{1F4DC}' },
        { path: '/api/security/opa-compliance', name: 'OPA/Gatekeeper', icon: '\u{1F6AB}' },
        { path: '/api/security/policy-governance', name: 'Policy Governance', icon: '\u{1F4DC}' },
        { path: '/api/security/linux-capabilities-audit', name: 'Linux Caps', icon: '\u{1F512}' },
        { path: '/api/security/egress-traffic-audit', name: 'Egress Audit', icon: '\u{1F517}' },
        { path: '/api/security/node-hardening-score', name: 'Node Hardening', icon: '\u{1F6E1}' },
        { path: '/api/security/run-as-non-root-audit', name: 'Non-Root Audit', icon: '\u{1F6AB}' },
        { path: '/api/security/host-pid-ipc-audit', name: 'Host PID/IPC', icon: '\u{1F510}' },
        { path: '/api/security/image-digest-pinning', name: 'Digest Pinning', icon: '\u{1F527}' },
        { path: '/api/security/container-uid-gid', name: 'UID/GID', icon: '\u{1F464}' },
        { path: '/api/security/default-sa-binding', name: 'Default SA', icon: '\u{1F511}' },
        { path: '/api/security/pod-security-posture-v2', name: 'Posture V2', icon: '\u{1F6E1}' },
        { path: '/api/security/sa-privilege-scope', name: 'SA Priv Scope', icon: '\u{1F511}' },
        { path: '/api/security/token-audit-trail', name: 'Token Audit', icon: '\u{1F4CB}' },
        { path: '/api/security/secret-volume-exposure', name: 'Secret Vol Exp', icon: '\u{1F512}' },
        { path: '/api/security/rbac-wildcard-verb', name: 'RBAC Wildcard', icon: '\u26A0' },
        { path: '/api/security/anonymous-auth-risk', name: 'Anon Auth', icon: '\u{1F6E1}' },
        { path: '/api/security/node-restriction-label', name: 'Node Restrict', icon: '\u{1F6AB}' },
        { path: '/api/security/priv-esc-audit', name: 'Priv Esc', icon: '\u2B06' },
        { path: '/api/security/seccomp-profile-audit', name: 'Seccomp', icon: '\u{1F6E1}' },
        { path: '/api/security/cap-drop-audit', name: 'Cap Drop', icon: '\u2696' },
        { path: '/api/security/host-path-audit', name: 'HostPath', icon: '\u{1F4C2}' },
        { path: '/api/security/readonly-rootfs', name: 'RO RootFS', icon: '\u{1F512}' },
        { path: '/api/security/sa-token-age', name: 'SA Token Age', icon: '\u23F1' },
        { path: '/api/security/fs-group-audit', name: 'FSGroup', icon: '\u{1F465}' },
        { path: '/api/security/proc-mount-type', name: 'Proc Mount', icon: '\u{1F4E6}' },
        { path: '/api/security/kernel-param-access', name: 'Kernel Param', icon: '\u2699' },
        { path: '/api/security/sa-image-pull-secret', name: 'SA Pull Sec', icon: '\u{1F511}' },
        { path: '/api/security/pod-dns-policy-restrict', name: 'DNS Policy', icon: '\u{1F310}' },
        { path: '/api/security/container-run-as-user', name: 'RunAsUser', icon: '\u{1F464}' },
        { path: '/api/security/validating-webhook-coverage', name: 'Val Webhook', icon: '\u2705' },
        { path: '/api/security/ingress-tls-cert-age', name: 'TLS Cert', icon: '\u{1F510}' },
        { path: '/api/security/sa-token-volume', name: 'SA Token Vol', icon: '\u{1F4DC}' },
        { path: '/api/security/container-cap-effective', name: 'Cap Effective', icon: '\u2696' },
        { path: '/api/security/secret-type-coverage', name: 'Secret Type', icon: '\u{1F511}' },
        { path: '/api/security/pod-serviceaccount-mapping', name: 'SA Mapping', icon: '\u{1F517}' },
        { path: '/api/security/policy-drift', name: 'Policy Drift', icon: '\u{1F501}' },
        { path: '/api/security/admission-bypass-audit', name: 'Admission Bypass', icon: '\u26D4' },
      ],
      'Supply Chain & Images': [
        { path: '/api/security/image-vuln', name: 'Image Vulnerabilities', icon: '\u26A0' },
        { path: '/api/security/supply-chain', name: 'Supply Chain', icon: '\u{1F4E6}' },
        { path: '/api/security/trust-chain', name: 'Trust Chain', icon: '\u{1F512}' },
        { path: '/api/security/image-baseline-drift', name: 'Image Baseline Drift', icon: '\u{1F4F7}' },
      ],
      'Runtime & Drift': [
        { path: '/api/security/runtime-scan', name: 'Runtime Scan', icon: '\u{1F50D}' },
        { path: '/api/security/runtime-drift-detect', name: 'Runtime Drift', icon: '\u{1F501}' },
        { path: '/api/security/runtime-threat', name: 'Runtime Threat', icon: '\u{1F6A8}' },
        { path: '/api/security/sec-drift', name: 'Security Drift', icon: '\u{1F501}' },
      ],
      'Certificates': [
        { path: '/api/security/cert-expiry', name: 'Cert Expiry', icon: '\u{1F510}' },
        { path: '/api/security/cert-inventory', name: 'Cert Inventory', icon: '\u{1F4DC}' },
        { path: '/api/security/cert-chain-validator', name: 'Cert Chain', icon: '\u{1F510}' },
      ],
      'Posture & Audit': [
        { path: '/api/security/posture-scorecard', name: 'Posture Scorecard', icon: '\u{1F4CB}' },
        { path: '/api/security/hardening-score', name: 'Hardening Score', icon: '\u{1F6E1}' },
        { path: '/api/security/attack-surface', name: 'Attack Surface', icon: '\u{1F575}' },
        { path: '/api/security/blast-radius', name: 'Blast Radius', icon: '\u{1F4A5}' },
        { path: '/api/security/fix-plan', name: 'Fix Plan', icon: '\u{1F527}' },
        { path: '/api/security/audit-policy', name: 'Audit Policy', icon: '\u{1F4DD}' },
        { path: '/api/security/audit-trail', name: 'Audit Trail', icon: '\u{1F4DD}' },
      ],
      'Supply Chain & TLS': [
        { path: '/api/security/image-registry-allowlist', name: 'Registry Allowlist', icon: '\u{1F4E6}' },
        { path: '/api/security/sa-mount-exposure', name: 'SA Mount Exposure', icon: '\u{1F511}' },
        { path: '/api/security/tls-version-audit', name: 'TLS Version Audit', icon: '\u{1F510}' },
        { path: '/api/security/pod-escape-risk', name: 'Pod Escape Risk', icon: '\u{1F6A8}' },
        { path: '/api/security/egress-policy-gap', name: 'Egress Policy Gap', icon: '\u2192' },
        { path: '/api/security/cis-benchmark-lite', name: 'CIS Benchmark', icon: '\u{1F4DC}' },
        { path: '/api/security/vol-encryption-audit', name: 'Volume Encryption', icon: '\u{1F512}' },
        { path: '/api/security/webhook-posture', name: 'Webhook Posture', icon: '\u26D4' },
        { path: '/api/security/key-rotation-compliance', name: 'Key Rotation', icon: '\u{1F511}' },
        { path: '/api/security/capability-audit', name: 'Capability Audit', icon: '\u{1F6E1}' },
        { path: '/api/security/host-namespace-audit', name: 'Host NS Audit', icon: '\u{1F3E0}' },
        { path: '/api/security/pss-compliance', name: 'PSS Compliance', icon: '\u{1F6E1}' },
        { path: '/api/security/dns-exfil-risk-v2', name: 'DNS Exfil Risk', icon: '\u{1F50C}' },
        { path: '/api/security/port-forward-audit-v2', name: 'Port Forward', icon: '\u2192' },
        { path: '/api/security/image-provenance-v3', name: 'Image Provenance', icon: '\u{1F4E6}' },
        { path: '/api/security/secret-exposure-graph', name: 'Secret Graph', icon: '\u{1F510}' },
        { path: '/api/security/admission-exception', name: 'Admission Exc', icon: '\u{1F6E1}' },
        { path: '/api/security/proc-mount-risk', name: 'Proc Mount', icon: '\u{1F527}' },
        { path: '/api/security/volume-mount-audit', name: 'Volume Mount', icon: '\u{1F4C1}' },
        { path: '/api/security/priv-esc-risk', name: 'Priv Esc Risk', icon: '\u{1F6A8}' },
        { path: '/api/security/image-base-scan', name: 'Base Image', icon: '\u{1F4F7}' },
        { path: '/api/docs/label-standardization', name: 'Label Std', icon: '\u{1F3F7}' },
        { path: '/api/docs/resource-age-distribution', name: 'Age Dist', icon: '\u{1F4C5}' },
        { path: '/api/docs/ns-isolation-matrix', name: 'NS Isolation', icon: '\u{1F310}' },
        { path: '/api/security/token-projection', name: 'Token Projection', icon: '\u{1F510}' },
        { path: '/api/security/sysctl-risk', name: 'Sysctl Risk', icon: '\u2699' },
        { path: '/api/security/hostport-exposure', name: 'HostPort Map', icon: '\u{1F6A7}' },
      ],
    },
  },
  'Operations': {
    color: '#d29922',
    icon: '\u26A1',
    subcategories: {
      'Control Plane': [
        { path: '/api/operations/etcd-health', name: 'Etcd Health', icon: '\u{1F50C}' },
        { path: '/api/operations/kubelet-health', name: 'Kubelet Health', icon: '\u{1F3E2}' },
        
        { path: '/api/operations/cni-health', name: 'CNI Health', icon: '\u{1F310}' },
        { path: '/api/operations/coredns-config-audit', name: 'CoreDNS Config', icon: '\u{1F310}' },
        { path: '/api/operations/webhook-timeout-audit', name: 'Webhook Timeout', icon: '\u23F1' },
        { path: '/api/operations/node-condition-trend', name: 'Node Condition', icon: '\u{1F4C9}' },
        { path: '/api/operations/container-log-size', name: 'Log Size', icon: '\u{1F4DD}' },
        { path: '/api/operations/kubelet-config-drift', name: 'Kubelet Drift', icon: '\u2699' },
        { path: '/api/operations/control-plane', name: 'Control Plane', icon: '\u{1F3E2}' },
        { path: '/api/operations/cert-transparency-monitor', name: 'Cert Transparency', icon: '\u{1F510}' },
        { path: '/api/operations/apf-audit', name: 'API Priority/Fairness', icon: '\u2696' },
      ],
      'Observability Stack': [
        { path: '/api/operations/metrics-pipeline', name: 'Metrics Pipeline', icon: '\u{1F4CA}' },
        { path: '/api/operations/prom-health', name: 'Prometheus', icon: '\u{1F525}' },
        { path: '/api/operations/grafana-health', name: 'Grafana', icon: '\u{1F4C4}' },
        { path: '/api/operations/alertmanager-health', name: 'Alertmanager', icon: '\u{1F514}' },
        { path: '/api/operations/audit-log-health', name: 'Audit Log Pipeline', icon: '\u{1F4DD}' },
        { path: '/api/operations/log-volume', name: 'Log Volume', icon: '\u{1F4DD}' },
        { path: '/api/operations/obs-coverage', name: 'Obs Coverage', icon: '\u{1F441}' },
        { path: '/api/operations/obs-cardinality', name: 'Obs Cardinality', icon: '\u{1F4CF}' },
      ],
      'Pod Health & Restarts': [
        { path: '/api/operations/pod-health-index', name: 'Pod Health Index', icon: '\u{1F493}' },
        { path: '/api/operations/crashloop', name: 'CrashLoopBackOff', icon: '\u{1F501}' },
        { path: '/api/operations/crash-budget-tracker', name: 'Crash Budget', icon: '\u{1F4B0}' },
        { path: '/api/operations/restart-analyzer', name: 'Restart Analyzer', icon: '\u{1F501}' },
        { path: '/api/operations/pod-restart-forensics-deep', name: 'Restart Forensics', icon: '\u{1F50D}' },
        { path: '/api/operations/pod-startup', name: 'Pod Startup', icon: '\u23F1' },
        { path: '/api/operations/oom-tracker', name: 'OOM Tracker', icon: '\u{1F4A9}' },
      ],
      'Events & Incidents': [
        { path: '/api/operations/event-noise-filter', name: 'Event Noise Filter', icon: '\u266A' },
        { path: '/api/operations/incident-correlation', name: 'Incident Correlation', icon: '\u{1F50D}' },
        { path: '/api/operations/deployment-health-trend', name: 'Deploy Health Trend', icon: '\u{1F4C8}' },
        { path: '/api/operations/event-correlation-matrix', name: 'Event Correlation', icon: '\u{1F9ED}' },
        { path: '/api/operations/incident-timeline', name: 'Incident Timeline', icon: '\u{1F4C5}' },
        { path: '/api/operations/triage', name: 'Triage', icon: '\u{1FA7A}' },
      ],
      'SLO & SLI': [
        { path: '/api/operations/pod-slo', name: 'Pod SLO', icon: '\u{1F3AF}' },
        { path: '/api/operations/slo-burn-rate', name: 'SLO Burn Rate', icon: '\u{1F525}' },
        { path: '/api/operations/golden-signal-budget', name: 'Golden Signals', icon: '\u{1F4A1}' },
        { path: '/api/operations/health-score', name: 'Cluster Health', icon: '\u{1F493}' },
        { path: '/api/operations/health-trend', name: 'Health Trend', icon: '\u{1F4C8}' },
      ],
      'Node & Scheduling': [
        { path: '/api/operations/node-pressure', name: 'Node Pressure', icon: '\u26A0' },
        { path: '/api/operations/node-trend', name: 'Node Trend', icon: '\u{1F4C8}' },
        { path: '/api/operations/drain-impact', name: 'Drain Impact', icon: '\u{1F6A6}' },
        { path: '/api/operations/pdb-audit', name: 'PDB Audit', icon: '\u{1F6E1}' },
        { path: '/api/operations/pdb-generator', name: 'PDB Generator', icon: '\u{1F527}' },
        { path: '/api/operations/scheduling-latency', name: 'Scheduling Latency', icon: '\u23F1' },
        { path: '/api/operations/cluster-version-skew', name: 'Version Skew', icon: '\u2195' },
        { path: '/api/operations/node-taint-impact', name: 'Taint Impact', icon: '\u26D4' },
      ],
      'API Server': [
        { path: '/api/operations/api-load', name: 'API Server Load', icon: '\u{1F4E6}' },
        { path: '/api/operations/api-latency', name: 'API Latency', icon: '\u23F1' },
        { path: '/api/operations/api-access-pattern', name: 'API Access Pattern', icon: '\u{1F511}' },
        { path: '/api/operations/api-server-slo', name: 'API Server SLO', icon: '\u{1F3AF}' },
      ],
      'Reliability': [
        { path: '/api/operations/chaos-readiness', name: 'Chaos Readiness', icon: '\u{1F4A5}' },
        { path: '/api/operations/throttle-risk', name: 'Throttle Risk', icon: '\u{1F4A7}' },
        { path: '/api/operations/pod-evictions', name: 'Pod Evictions', icon: '\u26A0' },
        { path: '/api/operations/mttr', name: 'MTTR', icon: '\u23F1' },
        { path: '/api/operations/probes', name: 'Health Probes', icon: '\u{1FA78}' },
      ],
      'Phase & Lifecycle': [
        { path: '/api/operations/pod-phase-timeline', name: 'Phase Timeline', icon: '\u23F1' },
        { path: '/api/operations/image-gc-pressure', name: 'Image GC Pressure', icon: '\u{1F4BE}' },
        { path: '/api/operations/controller-reconcile', name: 'Controller Reconcile', icon: '\u{1F501}' },
        { path: '/api/operations/node-maint-window', name: 'Maint Window', icon: '\u{1F6E0}' },
        { path: '/api/operations/resource-leak-detector', name: 'Resource Leaks', icon: '\u{1F4B8}' },
        { path: '/api/operations/log-agg-health', name: 'Log Agg Health', icon: '\u{1F4DD}' },
        { path: '/api/operations/backup-snapshot-audit', name: 'Backup Audit', icon: '\u{1F4BE}' },
        { path: '/api/operations/job-success-rate', name: 'Job Success', icon: '\u2705' },
        { path: '/api/operations/event-retention', name: 'Event Volume', icon: '\u{1F4CB}' },
        { path: '/api/operations/control-plane-health', name: 'Control Plane', icon: '\u{1F3D7}' },
        { path: '/api/operations/csi-driver-health', name: 'CSI Driver', icon: '\u{1F4BF}' },
        { path: '/api/operations/cert-renewal-timeline', name: 'Cert Timeline', icon: '\u{1F4C5}' },
        { path: '/api/operations/storage-io-latency', name: 'Storage Latency', icon: '\u{1F4BE}' },
        { path: '/api/operations/network-packet-loss', name: 'Network Loss', icon: '\u{1F310}' },
        { path: '/api/operations/cgroup-pressure', name: 'Cgroup Pressure', icon: '\u{1F4C9}' },
        { path: '/api/operations/job-lifecycle', name: 'Job Lifecycle', icon: '\u{1F4CB}' },
        { path: '/api/operations/leader-election', name: 'Leader Election', icon: '\u{1F451}' },
        { path: '/api/operations/pvc-lifecycle', name: 'PVC Lifecycle', icon: '\u{1F4BE}' },
        { path: '/api/operations/endpoint-latency', name: 'Endpoint Latency', icon: '\u23F1' },
        { path: '/api/operations/container-forensics', name: 'Container Forensics', icon: '\u{1F50D}' },
      ],
    },
  },
  'Scalability': {
    color: '#bc8cff',
    icon: '\u{1F4C8}',
    subcategories: {
      'Cost & Waste': [
        { path: '/api/scalability/cost-waste', name: 'Cost Waste', icon: '\u{1F4B0}' },
        { path: '/api/scalability/cost-allocation', name: 'Cost Allocation', icon: '\u{1F4B0}' },
        { path: '/api/scalability/cost-intelligence', name: 'Cost Intelligence', icon: '\u{1F9EE}' },
        { path: '/api/scalability/cost-anomaly', name: 'Cost Anomaly', icon: '\u26A0' },
        { path: '/api/scalability/idle-waste', name: 'Idle Waste', icon: '\u{1F4A9}' },
        { path: '/api/scalability/chargeback', name: 'Chargeback', icon: '\u{1F4B3}' },
        { path: '/api/scalability/unit-economics', name: 'Unit Economics', icon: '\u{1F4B0}' },
        { path: '/api/scalability/budget-alert', name: 'Budget Alert', icon: '\u26A0' },
      ],
      'Autoscaling': [
        { path: '/api/scalability/hpa-performance', name: 'HPA Performance', icon: '\u2195' },
        { path: '/api/scalability/hpa-behavior', name: 'HPA Behavior', icon: '\u2195' },
        { path: '/api/scalability/autoscale-readiness', name: 'Autoscale Readiness', icon: '\u2195' },
        { path: '/api/scalability/autoscaler-gap', name: 'Autoscaler Gap', icon: '\u2195' },
        { path: '/api/scalability/autoscaling-intel', name: 'Autoscaling Intel', icon: '\u{1F9EE}' },
        { path: '/api/scalability/vpa-audit', name: 'VPA Audit', icon: '\u2195' },
        { path: '/api/scalability/hpa-cooldown-audit', name: 'HPA Cooldown', icon: '\u2195' },
      ],
      'Resource Efficiency': [
        { path: '/api/scalability/alloc-efficiency', name: 'Alloc Efficiency', icon: '\u2696' },
        { path: '/api/scalability/overcommit', name: 'Overcommit', icon: '\u26A0' },
        { path: '/api/scalability/overcommit-risk', name: 'Overcommit Risk', icon: '\u26A0' },
        { path: '/api/scalability/right-size-engine', name: 'Right-Size Engine', icon: '\u{1F4CF}' },
        { path: '/api/scalability/request-accuracy', name: 'Request Accuracy', icon: '\u{1F3AF}' },
        { path: '/api/scalability/request-intelligence', name: 'Request Intel', icon: '\u{1F9ED}' },
        { path: '/api/scalability/resource-request-saturation', name: 'Request Saturation', icon: '\u{1F4CA}' },
      ],
      'Node Management': [
        { path: '/api/scalability/node-lifecycle', name: 'Node Lifecycle', icon: '\u{1F578}' },
        { path: '/api/scalability/node-pool-health', name: 'Node Pool Health', icon: '\u{1F4BB}' },
        { path: '/api/scalability/node-utilization-deep', name: 'Node Utilization', icon: '\u{1F4BB}' },
        { path: '/api/scalability/node-life-forecast', name: 'Node Life Forecast', icon: '\u{1F4C5}' },
        { path: '/api/scalability/node-pool-rightsize', name: 'Node Rightsize', icon: '\u2194' },
        { path: '/api/scalability/node-decomm', name: 'Node Decommission', icon: '\u{1F5D1}' },
      ],
      'Storage': [
        { path: '/api/scalability/pv-reclaim', name: 'PV Reclaim', icon: '\u{1F4BE}' },
        { path: '/api/scalability/storage-performance', name: 'Storage Performance', icon: '\u{1F4BE}' },
        { path: '/api/scalability/storage-tier', name: 'Storage Tier', icon: '\u{1F4BE}' },
        { path: '/api/scalability/volume-budget', name: 'Volume Budget', icon: '\u{1F4BE}' },
        { path: '/api/scalability/storage-forecast', name: 'Storage Forecast', icon: '\u{1F4C8}' },
        { path: '/api/scalability/storage-orphan', name: 'Storage Orphan', icon: '\u{1F9F9}' },
      ],
      'Scheduling & Density': [
        { path: '/api/scalability/scheduling-intel', name: 'Scheduling Intel', icon: '\u{1F9ED}' },
        { path: '/api/scalability/scheduler-fairness', name: 'Scheduler Fairness', icon: '\u2696' },
        { path: '/api/scalability/binpack-efficiency', name: 'Binpack', icon: '\u{1F4E6}' },
        { path: '/api/scalability/density-balance', name: 'Density Balance', icon: '\u2696' },
        { path: '/api/scalability/pod-density', name: 'Pod Density', icon: '\u{1F4CF}' },
        { path: '/api/scalability/fragmentation', name: 'Fragmentation', icon: '\u{1F9F9}' },
        { path: '/api/scalability/pod-affinity-spread', name: 'Affinity Spread', icon: '\u{1F4CD}' },
      ],
      'HA & DR': [
        { path: '/api/scalability/dr-readiness', name: 'DR Readiness', icon: '\u{1F6E1}' },
        { path: '/api/scalability/cluster-fault-tolerance', name: 'Fault Tolerance', icon: '\u{1F6E1}' },
        { path: '/api/scalability/pod-disruption-tolerance', name: 'Disruption Tolerance', icon: '\u{1F6E1}' },
        { path: '/api/scalability/eviction-risk', name: 'Eviction Risk', icon: '\u26A0' },
        { path: '/api/scalability/node-failure-blast', name: 'Node Failure Blast', icon: '\u{1F4A5}' },
        { path: '/api/scalability/ha-audit', name: 'HA Audit', icon: '\u{1F6E1}' },
      ],
      'Capacity & Forecast': [
        { path: '/api/scalability/capacity-headroom', name: 'Capacity Headroom', icon: '\u{1F4CF}' },
        { path: '/api/scalability/capacity-plan', name: 'Capacity Plan', icon: '\u{1F4CB}' },
        { path: '/api/scalability/capacity-forecast-deep', name: 'Capacity Forecast', icon: '\u{1F4C8}' },
        { path: '/api/scalability/cluster-pod-limit', name: 'Pod Limit', icon: '\u{1F4CF}' },
        { path: '/api/scalability/pdb-gap-analysis', name: 'PDB Gap', icon: '\u26A0' },
        { path: '/api/scalability/topology-spread-violation', name: 'Topo Spread', icon: '\u{1F4CD}' },
        { path: '/api/scalability/overcommit-deep', name: 'Overcommit Deep', icon: '\u{1F4CA}' },
        { path: '/api/scalability/resource-forecast', name: 'Resource Forecast', icon: '\u{1F52E}' },
        { path: '/api/scalability/bottleneck-predictor', name: 'Bottleneck Predictor', icon: '\u{1F9ED}' },
      ],
      'Quota & Multi-Tenant': [
        { path: '/api/scalability/quota-utilization', name: 'Quota Utilization', icon: '\u{1F4CA}' },
        { path: '/api/scalability/quota-saturation', name: 'Quota Saturation', icon: '\u26A0' },
        { path: '/api/scalability/quota-generator', name: 'Quota Generator', icon: '\u{1F527}' },
        { path: '/api/scalability/tenant-pressure', name: 'Tenant Pressure', icon: '\u{1F3E2}' },
        { path: '/api/scalability/namespace-isolation', name: 'NS Isolation', icon: '\u{1F6E1}' },
        { path: '/api/product/mesh-ready-check', name: 'Mesh Check', icon: '\u{1F575}' },
        { path: '/api/product/vol-access-audit', name: 'Vol Access', icon: '\u{1F4BE}' },
        { path: '/api/product/pdb-gap-analysis-v2', name: 'PDB Gap v2', icon: '\u{1F6E1}' },
        { path: '/api/scalability/sched-queue-depth', name: 'Sched Queue', icon: '\u23F1' },
        { path: '/api/scalability/pod-spread-violation', name: 'Pod Spread', icon: '\u{1F4CD}' },
        { path: '/api/scalability/ha-topo-score', name: 'HA Topology', icon: '\u{1F30D}' },
        { path: '/api/deployment/revision-timeline', name: 'Rev Timeline', icon: '\u{1F4DC}' },
        { path: '/api/deployment/qos-distribution', name: 'QoS Dist', icon: '\u2696' },
        { path: '/api/deployment/ds-health', name: 'DS Health', icon: '\u{1F4E6}' },
        { path: '/api/operations/hpa-scaling-events', name: 'HPA Scale Events', icon: '\u2195' },
        { path: '/api/operations/node-cond-history', name: 'Node Cond Hist', icon: '\u{1F4C9}' },
        { path: '/api/operations/config-change-tracker', name: 'Config Changes', icon: '\u{1F4DD}' },
        { path: '/api/security/rbac-overexpose', name: 'RBAC Overexpose', icon: '\u{1F511}' },
        { path: '/api/security/secret-enc-rest', name: 'Secret Enc', icon: '\u{1F512}' },
        { path: '/api/security/webhook-risk', name: 'Webhook Risk', icon: '\u26D4' },
        { path: '/api/docs/ownership-registry-v2', name: 'Ownership v2', icon: '\u{1F465}' },
        { path: '/api/docs/api-resource-inventory', name: 'API Inventory', icon: '\u{1F4E6}' },
        { path: '/api/docs/capacity-report', name: 'Capacity Report', icon: '\u{1F4CF}' },
        { path: '/api/scalability/ns-consumption', name: 'NS Consumption', icon: '\u{1F4CA}' },
        { path: '/api/scalability/namespace-budget-enforce', name: 'Budget Enforce', icon: '\u{1F4B0}' },
      ],
      'Cleanup & Sustainability': [
        { path: '/api/scalability/orphan-cleanup', name: 'Orphan Cleanup', icon: '\u{1F9F9}' },
        { path: '/api/scalability/image-cleanup', name: 'Image Cleanup', icon: '\u{1F9F9}' },
        { path: '/api/scalability/green-computing', name: 'Green Computing', icon: '\u{1F7E2}' },
        { path: '/api/scalability/carbon-footprint', name: 'Carbon Footprint', icon: '\u{1F7E2}' },
        { path: '/api/scalability/resource-waste-deep', name: 'Waste Deep', icon: '\u{1F4B8}' },
      ],
      'Pressure & Capacity Forecast': [
        { path: '/api/scalability/mem-pressure-forecast', name: 'Mem Pressure Forecast', icon: '\u{1F4CA}' },
        { path: '/api/scalability/scale-concurrency', name: 'Scale Concurrency', icon: '\u2195' },
        { path: '/api/scalability/disruption-window', name: 'Disruption Window', icon: '\u{1F6E1}' },
        { path: '/api/scalability/request-efficiency', name: 'Request Efficiency', icon: '\u2696' },
        { path: '/api/scalability/bin-packing-score', name: 'Bin-Packing Score', icon: '\u{1F4E6}' },
        { path: '/api/scalability/multi-zone-ha', name: 'Multi-Zone HA', icon: '\u{1F30D}' },
        { path: '/api/scalability/hpa-effectiveness-v2', name: 'HPA Effective', icon: '\u{1F4C9}' },
        { path: '/api/scalability/scheduling-latency-v2', name: 'Sched Latency', icon: '\u23F1' },
        { path: '/api/scalability/capacity-headroom-v2', name: 'Capacity Headroom', icon: '\u{1F4CF}' },
        { path: '/api/scalability/burst-capacity', name: 'Burst Capacity', icon: '\u26A1' },
        { path: '/api/scalability/elasticity-index', name: 'Elasticity Index', icon: '\u{1F4C8}' },
        { path: '/api/scalability/scale-bottleneck', name: 'Scale Bottleneck', icon: '\u{1F6A7}' },
        { path: '/api/scalability/api-throttle-risk', name: 'API Throttle', icon: '\u{1F525}' },
        { path: '/api/scalability/pod-density-opt', name: 'Pod Density', icon: '\u{1F4E6}' },
        { path: '/api/scalability/overcommit-forecast', name: 'Overcommit', icon: '\u{1F4C9}' },
        { path: '/api/scalability/rollback-window', name: 'Rollback Window', icon: '\u21A9' },
        { path: '/api/scalability/dns-scalability', name: 'DNS Scale', icon: '\u{1F310}' },
        { path: '/api/scalability/conn-pool-exhaustion', name: 'Conn Pool Risk', icon: '\u{1F517}' },
      ],
    },
  },
  'Product': {
    color: '#58a6ff',
    icon: '\u{1F4C2}',
    subcategories: {
      'Service & Traffic': [
        { path: '/api/product/service-connectivity', name: 'Service Connectivity', icon: '\u{1F517}' },
        { path: '/api/product/service-catalog', name: 'Service Catalog', icon: '\u{1F4C2}' },
        { path: '/api/product/service-dependency-map', name: 'Service Dependencies', icon: '\u{1F50D}' },
        { path: '/api/product/service-topology', name: 'Service Topology', icon: '\u{1F5FA}' },
        { path: '/api/product/traffic-flow', name: 'Traffic Flow', icon: '\u{1F500}' },
        { path: '/api/product/traffic-spike-guard', name: 'Traffic Spike Guard', icon: '\u{1F6A8}' },
        { path: '/api/product/east-west-traffic', name: 'East-West Traffic', icon: '\u2194' },
      ],
      'Mesh & Gateway': [
        { path: '/api/product/mesh-health', name: 'Service Mesh Health', icon: '\u{1F575}' },
        { path: '/api/product/svc-mesh-readiness', name: 'Mesh Readiness', icon: '\u{1F310}' },
        { path: '/api/product/mesh-injection', name: 'Mesh Injection', icon: '\u{1F500}' },
        { path: '/api/product/ingress-health', name: 'Ingress Health', icon: '\u{1F310}' },
        { path: '/api/product/api-gateway-health', name: 'API Gateway', icon: '\u{1F6A7}' },
        { path: '/api/product/ingress-conflict', name: 'Ingress Conflict', icon: '\u26A0' },
      ],
      'Endpoints': [
        { path: '/api/product/endpoint-dns-health', name: 'Endpoint & DNS', icon: '\u{1F310}' },
        { path: '/api/product/endpoint-health-deep', name: 'Endpoint Health Deep', icon: '\u2713' },
        { path: '/api/product/endpoint-mismatch', name: 'Endpoint Mismatch', icon: '\u26A0' },
        { path: '/api/product/endpoint-slice', name: 'Endpoint Slices', icon: '\u{1F4CA}' },
      ],
      'Workload Health': [
        { path: '/api/product/workload-criticality', name: 'Workload Criticality', icon: '\u26A0' },
        { path: '/api/product/workload-efficiency', name: 'Workload Efficiency', icon: '\u2696' },
        { path: '/api/product/workload-fingerprint', name: 'Workload Fingerprint', icon: '\u{1F194}' },
        { path: '/api/product/canary-health', name: 'Canary Health', icon: '\u{1F4AB}' },
        { path: '/api/product/reliability-scorecard', name: 'Reliability Scorecard', icon: '\u{1F4CB}' },
        { path: '/api/product/golden-signals', name: 'Golden Signals', icon: '\u{1F4A1}' },
      ],
      'Config & Labels': [
        { path: '/api/product/configmap-size', name: 'ConfigMap Size', icon: '\u{1F4C1}' },
        { path: '/api/product/config-audit', name: 'Config Audit', icon: '\u{1F4DC}' },
        { path: '/api/product/secret-mount-audit', name: 'Secret Mount', icon: '\u{1F511}' },
        { path: '/api/product/label-propagation', name: 'Label Propagation', icon: '\u{1F3F7}' },
        { path: '/api/product/cronjob-orphan-audit', name: 'CronJob Orphan', icon: '\u23F0' },
        { path: '/api/product/env-var-drift-detect', name: 'Env Var Drift', icon: '\u{1F500}' },
        { path: '/api/product/dns-record-audit', name: 'DNS Record Audit', icon: '\u{1F310}' },
        { path: '/api/product/workload-startup-profile', name: 'Startup Profile', icon: '\u{1F680}' },
        { path: '/api/product/config-warmstart', name: 'Config Warmstart', icon: '\u23F1' },
        { path: '/api/product/label-hygiene', name: 'Label Hygiene', icon: '\u{1F3F7}' },
        { path: '/api/product/ownership-map', name: 'Ownership Map', icon: '\u{1F464}' },
      ],
      'Scheduling & Placement': [
        { path: '/api/product/placement-score', name: 'Placement Score', icon: '\u{1F4CD}' },
        { path: '/api/product/topology-spread', name: 'Topology Spread', icon: '\u{1F5FA}' },
        { path: '/api/product/replica-distribution', name: 'Replica Distribution', icon: '\u{1F4CA}' },
        { path: '/api/product/affinity-conflict', name: 'Affinity Conflict', icon: '\u26A0' },
        { path: '/api/product/taint-toleration', name: 'Taint/Toleration', icon: '\u26D4' },
        { path: '/api/product/antiaffinity-ha', name: 'HA Readiness', icon: '\u{1F6E1}' },
      ],
      'Storage & PVC': [
        { path: '/api/product/pvc-health', name: 'PVC Health', icon: '\u{1F4BE}' },
        { path: '/api/product/pv-access', name: 'PV Access', icon: '\u{1F4BE}' },
        { path: '/api/product/config-mount-risk', name: 'Config Mount Risk', icon: '\u26A0' },
        { path: '/api/product/pvc-io-health', name: 'PVC I/O Health', icon: '\u{1F4BE}' },
      ],
      'API Governance': [
        { path: '/api/product/api-version-governance', name: 'API Version', icon: '\u{1F4C4}' },
        { path: '/api/product/api-deprecation', name: 'API Deprecation', icon: '\u26A0' },
        { path: '/api/product/slo-compliance', name: 'SLO Compliance', icon: '\u{1F3AF}' },
        { path: '/api/product/priority-class-audit', name: 'Priority Class', icon: '\u26A1' },
      ],
      'Network & Exposure': [
        { path: '/api/product/service-exposure-map', name: 'Service Exposure', icon: '\u{1F310}' },
        { path: '/api/product/workload-interdependency', name: 'Interdependency', icon: '\u{1F517}' },
        { path: '/api/product/dns-resolution-health', name: 'DNS Health', icon: '\u{1F310}' },
        { path: '/api/product/storage-class-audit', name: 'Storage Class', icon: '\u{1F4BE}' },
        { path: '/api/product/cost-attribution', name: 'Cost Attribution', icon: '\u{1F4B0}' },
        { path: '/api/product/quota-forecast', name: 'Quota Forecast', icon: '\u{1F4C9}' },
        { path: '/api/product/mesh-readiness-deep', name: 'Mesh Ready', icon: '\u{1F310}' },
        { path: '/api/product/env-secret-leak', name: 'Secret Leak', icon: '\u{1F510}' },
        { path: '/api/product/probe-coverage-gap', name: 'Probe Gap', icon: '\u{1F50C}' },
        { path: '/api/product/gpu-audit', name: 'GPU Audit', icon: '\u{1F3AE}' },
        { path: '/api/product/limit-range-audit', name: 'LimitRange', icon: '\u{1F4CF}' },
        { path: '/api/product/tenant-isolation', name: 'Tenant Iso', icon: '\u{1F3D7}' },
        { path: '/api/product/resource-share', name: 'Resource Share', icon: '\u2696' },
        { path: '/api/product/image-lifecycle', name: 'Image Lifecycle', icon: '\u{1F4E6}' },
        { path: '/api/product/volume-snapshot-readiness', name: 'Snapshot Ready', icon: '\u{1F4F7}' },
        { path: '/api/product/idle-resource', name: 'Idle Resource', icon: '\u{1F634}' },
        { path: '/api/product/secret-version-history', name: 'Secret Version', icon: '\u{1F510}' },
        { path: '/api/product/crd-health', name: 'CRD Health', icon: '\u{1F9E9}' },
        { path: '/api/product/autosize-recommender', name: 'Autosize Rec', icon: '\u{1F4CF}' },
        { path: '/api/product/res-wastage', name: 'Res Wastage', icon: '\u{1F4B8}' },
        { path: '/api/product/sa-usage-tracker', name: 'SA Usage', icon: '\u{1F464}' },
        { path: '/api/product/ep-slice-health', name: 'EP Slice', icon: '\u{1F4CA}' },
        { path: '/api/scalability/res-pressure-score', name: 'Res Pressure', icon: '\u{1F525}' },
        { path: '/api/scalability/startup-latency', name: 'Startup Latency', icon: '\u23F1' },
        { path: '/api/deployment/pause-detect', name: 'Pause Detect', icon: '\u23F8' },
        { path: '/api/deployment/tag-compliance', name: 'Tag Comply', icon: '\u{1F3F7}' },
        { path: '/api/deployment/rollout-strategy', name: 'Rollout Strat', icon: '\u{1F504}' },
        { path: '/api/operations/restart-storm', name: 'Restart Storm', icon: '\u26A1' },
        { path: '/api/operations/event-storm', name: 'Event Storm', icon: '\u{1F4A5}' },
        { path: '/api/operations/taint-impact', name: 'Taint Impact', icon: '\u{1F6AB}' },
        { path: '/api/security/netpol-coverage-v2', name: 'NetPol Coverage', icon: '\u{1F310}' },
        { path: '/api/security/seccomp-exposure', name: 'Seccomp', icon: '\u{1F6E1}' },
        { path: '/api/security/api-discovery-exposure', name: 'API Discovery', icon: '\u{1F441}' },
        { path: '/api/docs/dependency-graph', name: 'Dep Graph', icon: '\u{1F578}' },
        { path: '/api/docs/storage-class-inventory', name: 'SC Inventory', icon: '\u{1F4BE}' },
        { path: '/api/docs/dns-resolution-map', name: 'DNS Map', icon: '\u{1F310}' },
        { path: '/api/product/vol-snapshot-audit', name: 'Vol Snapshot', icon: '\u{1F4F7}' },
        { path: '/api/product/priority-class-inv', name: 'PriorityClass', icon: '\u26A0' },
        { path: '/api/product/pull-policy-audit', name: 'Pull Policy', icon: '\u2B07' },
        { path: '/api/scalability/controller-health', name: 'Ctrl Health', icon: '\u2699' },
        { path: '/api/scalability/gc-pressure', name: 'GC Pressure', icon: '\u{1F5D1}' },
        { path: '/api/scalability/pod-limit-proximity', name: 'Pod Limit', icon: '\u{1F4CF}' },
        { path: '/api/deployment/sts-ordinal-health', name: 'STS Ordinal', icon: '\u{1F522}' },
        { path: '/api/deployment/job-completion-tracker', name: 'Job Tracker', icon: '\u2705' },
        { path: '/api/deployment/cron-overlap', name: 'Cron Overlap', icon: '\u23F0' },
        { path: '/api/operations/log-volume-estimator', name: 'Log Volume', icon: '\u{1F4DD}' },
        { path: '/api/operations/eviction-history', name: 'Eviction Hist', icon: '\u274C' },
        { path: '/api/operations/kubelet-sync', name: 'Kubelet Sync', icon: '\u{1F494}' },
        { path: '/api/security/pod-forensics-snap', name: 'Pod Forensics', icon: '\u{1F50D}' },
        { path: '/api/security/egress-exposure', name: 'Egress Exposure', icon: '\u2197' },
        { path: '/api/security/sa-token-age-v2', name: 'SA Token Age', icon: '\u231B' },
        { path: '/api/docs/cluster-config-snap', name: 'Config Snap', icon: '\u{1F4CB}' },
        { path: '/api/docs/event-history-doc', name: 'Event History', icon: '\u{1F4C4}' },
        { path: '/api/docs/quota-doc', name: 'Quota Doc', icon: '\u{1F4D1}' },
        { path: '/api/product/helm-release-audit-v2', name: 'Helm Audit v2', icon: '\u2693' },
        { path: '/api/product/ingress-consolidation', name: 'Ingress Consolid', icon: '\u27A4' },
        { path: '/api/product/ns-lifecycle-tracker', name: 'NS Lifecycle', icon: '\u{1F310}' },
        { path: '/api/scalability/node-frag-analysis', name: 'Node Frag', icon: '\u{1F4A2}' },
        { path: '/api/scalability/ctrl-queue-pressure', name: 'Ctrl Queue', icon: '\u{1F4C4}' },
        { path: '/api/scalability/pod-density-optimizer', name: 'Pod Density Opt', icon: '\u{1F4CA}' },
        { path: '/api/deployment/canary-detector', name: 'Canary Detect', icon: '\u{1F426}' },
        { path: '/api/deployment/init-container-overhead', name: 'Init Overhead', icon: '\u23F3' },
        { path: '/api/deployment/lifecycle-hook-comp', name: 'Lifecycle Hooks', icon: '\u{1F504}' },
        { path: '/api/operations/oom-forecast', name: 'OOM Forecast', icon: '\u26A0' },
        { path: '/api/operations/api-request-pattern', name: 'API Pattern', icon: '\u{1F4CA}' },
        { path: '/api/operations/terminated-reason-catalog', name: 'Term Catalog', icon: '\u{1F4CB}' },
        { path: '/api/operations/quota-waste-detector', name: 'Quota Waste', icon: '\u{1F4B8}' },
        { path: '/api/operations/admission-health', name: 'Admission Health', icon: '\u{1F510}' },
        { path: '/api/operations/clock-sync', name: 'Clock Sync', icon: '\u23F1' },
        { path: '/api/operations/pod-restart-cost', name: 'Restart Cost', icon: '\u{1F4B0}' },
        { path: '/api/operations/node-disk-io-health', name: 'Disk I/O', icon: '\u{1F4BE}' },
        { path: '/api/operations/event-qps-analyzer', name: 'Event QPS', icon: '\u{1F4CA}' },
        { path: '/api/operations/pod-age-distribution', name: 'Pod Age', icon: '\u{1F4C5}' },
        { path: '/api/operations/node-condition-flap', name: 'Node Flap', icon: '\u{1F4C9}' },
        { path: '/api/operations/csi-attach-latency', name: 'CSI Latency', icon: '\u23F1' },
        { path: '/api/operations/exit-code-pattern', name: 'Exit Codes', icon: '\u26A0' },
        { path: '/api/operations/pod-qos-class', name: 'QoS Class', icon: '\u2696' },
        { path: '/api/operations/ns-resource-pressure', name: 'NS Pressure', icon: '\u{1F525}' },
        { path: '/api/operations/pod-probe-latency', name: 'Probe Config', icon: '\u{1F50D}' },
        { path: '/api/operations/image-pull-duration', name: 'Image Pull', icon: '\u23F3' },
        { path: '/api/operations/config-reload-health', name: 'Config Reload', icon: '\u{1F504}' },
        { path: '/api/operations/pod-grace-period', name: 'Grace Period', icon: '\u231B' },
        { path: '/api/operations/resource-limit-ratio', name: 'Limit Ratio', icon: '\u2696' },
        { path: '/api/operations/cronjob-execution-health', name: 'CronJob Health', icon: '\u23F0' },
        { path: '/api/operations/node-pressure-budget', name: 'Node Budget', icon: '\u{1F4CF}' },
        { path: '/api/operations/event-budget', name: 'Event Budget', icon: '\u{1F4CB}' },
        { path: '/api/operations/net-policy-budget', name: 'NetPol Budget', icon: '\u{1F6E1}' },
        { path: '/api/operations/pod-init-time', name: 'Pod Init', icon: '\u23F1' },
        { path: '/api/operations/kubelet-cert-expiry', name: 'Kubelet Cert', icon: '\u{1F511}' },
        { path: '/api/operations/ns-event-noise', name: 'Event Noise', icon: '\u{1F4E2}' },
        { path: '/api/operations/pod-taint-toleration-match', name: 'Taint Match', icon: '\u{1F6DE}' },
        { path: '/api/operations/node-condition-budget', name: 'Node Cond', icon: '\u2705' },
        { path: '/api/operations/cluster-log-volume', name: 'Log Volume', icon: '\u{1F4DD}' },
        { path: '/api/operations/pod-crash-loop-detect', name: 'Crash Loop', icon: '\u{1F534}' },
        { path: '/api/operations/deployment-replica-health', name: 'Replica Health', icon: '\u{1F4C9}' },
        { path: '/api/operations/event-warning-hotspot', name: 'Evt Hotspot', icon: '\u{1F525}' },
        { path: '/api/operations/pod-phase-distribution', name: 'Pod Phase', icon: '\u{1F4CA}' },
        { path: '/api/operations/container-restart-reason', name: 'Restart Reason', icon: '\u{1F501}' },
        { path: '/api/operations/node-kubelet-version', name: 'Kubelet Ver', icon: '\u{1F527}' },
        { path: '/api/operations/pod-cpu-throttling-estimator', name: 'CPU Throt Est', icon: '\u{1F525}' },
        { path: '/api/operations/namespace-resource-pressure', name: 'NS Pressure', icon: '\u{1F4CF}' },
        { path: '/api/operations/event-age-decay-tracker', name: 'Evt Age Decay', icon: '\u{1F4CB}' },
        { path: '/api/security/psa-violation', name: 'PSA Violation', icon: '\u{1F6E1}' },
        { path: '/api/security/automount-risk', name: 'AutoMount Risk', icon: '\u{1F511}' },
        { path: '/api/security/registry-trust', name: 'Registry Trust', icon: '\u{1F4DC}' },
        { path: '/api/security/secret-mount-exposure', name: 'Secret Exposure', icon: '\u{1F510}' },
        { path: '/api/security/bare-namespace-netpol', name: 'Bare NS NetPol', icon: '\u{1F6E1}' },
        { path: '/api/security/privileged-escalation-path', name: 'Priv Esc Path', icon: '\u26A0' },
        { path: '/api/docs/helm-release-inventory', name: 'Helm Inventory', icon: '\u26F5' },
        { path: '/api/docs/pdb-coverage', name: 'PDB Coverage', icon: '\u{1F6E1}' },
        { path: '/api/docs/sa-token-age', name: 'SA Token Age', icon: '\u{1F511}' },
        { path: '/api/product/cost-anomaly-detector', name: 'Cost Anomaly', icon: '\u{1F4B0}' },
        { path: '/api/product/right-size-recommender', name: 'Right-Size', icon: '\u{1F4CF}' },
        { path: '/api/product/image-dedup-report', name: 'Image Dedup', icon: '\u{1F4BE}' },
        { path: '/api/scalability/hpa-headroom', name: 'HPA Headroom', icon: '\u{1F4C8}' },
        { path: '/api/scalability/az-spread-validator', name: 'AZ Spread', icon: '\u{1F310}' },
        { path: '/api/scalability/leader-election-audit', name: 'Leader Election', icon: '\u{1F451}' },
        { path: '/api/deployment/revision-tracker', name: 'Rev Tracker', icon: '\u{1F501}' },
        { path: '/api/deployment/lifecycle-hook-audit', name: 'Lifecycle Hooks', icon: '\u23F1' },
        { path: '/api/deployment/topo-constraint-validator', name: 'Topo Constraint', icon: '\u{1F5FA}' },
        { path: '/api/operations/restart-budget', name: 'Restart Budget', icon: '\u{1F501}' },
        { path: '/api/operations/volume-iops-estimate', name: 'Vol IOPS Est', icon: '\u{1F4BF}' },
        { path: '/api/operations/node-allocatable-budget', name: 'Node Alloc Budget', icon: '\u{1F4B0}' },
        { path: '/api/security/image-tag-immutability', name: 'Tag Immutability', icon: '\u{1F4E6}' },
        { path: '/api/security/rbac-wildcard-audit', name: 'RBAC Wildcard', icon: '\u{1F510}' },
        { path: '/api/security/security-context-baseline', name: 'SecCtx Baseline', icon: '\u{1F6E1}' },
        { path: '/api/docs/label-taxonomy', name: 'Label Taxonomy', icon: '\u{1F3F7}' },
        { path: '/api/docs/annotation-inventory-doc', name: 'Annot Inventory', icon: '\u{1F4DD}' },
        { path: '/api/docs/quota-cross-ref', name: 'Quota XRef', icon: '\u{1F4D1}' },
        { path: '/api/product/workload-age-profile', name: 'Wkld Age', icon: '\u{1F4C5}' },
        { path: '/api/product/service-mesh-readiness', name: 'Mesh Ready', icon: '\u{1F310}' },
        { path: '/api/product/tls-expiry-forecast', name: 'TLS Forecast', icon: '\u{1F510}' },
        { path: '/api/scalability/node-capacity-headroom', name: 'Node Headroom', icon: '\u{1F4CF}' },
        { path: '/api/scalability/pod-density-analyzer', name: 'Pod Density', icon: '\u{1F4CA}' },
        { path: '/api/scalability/storage-capacity-forecast', name: 'Storage Forecast', icon: '\u{1F4BF}' },
        { path: '/api/product/ephemeral-tracker', name: 'Ephemeral Wkld', icon: '\u23F3' },
        { path: '/api/product/api-version-deprecation', name: 'API Deprecation', icon: '\u26A0' },
        { path: '/api/product/cross-ns-traffic-estimator', name: 'X-NS Traffic', icon: '\u{1F517}' },
        { path: '/api/deployment/rollout-window', name: 'Rollout Window', icon: '\u23F1' },
        { path: '/api/deployment/init-container-map', name: 'Init Container', icon: '\u{1F527}' },
        { path: '/api/deployment/probe-config-validator', name: 'Probe Config', icon: '\u{1F50D}' },
        { path: '/api/operations/dns-health-2039', name: 'DNS Health', icon: '\u{1F310}' },
        { path: '/api/operations/termination-grace-tracker', name: 'Term Grace', icon: '\u23F1' },
        { path: '/api/operations/kubelet-pleg-latency', name: 'PLEG Latency', icon: '\u{1F4C8}' },
        { path: '/api/security/sa-token-mount-risk', name: 'SA Token Mount', icon: '\u{1F511}' },
        { path: '/api/security/cr-binding-explosion', name: 'CR Binding', icon: '\u{1F465}' },
        { path: '/api/security/container-port-exposure', name: 'Port Exposure', icon: '\u{1F50C}' },
        { path: '/api/docs/namespace-catalog', name: 'NS Catalog', icon: '\u{1F4D2}' },
        { path: '/api/docs/limit-range-doc', name: 'LimitRange Doc', icon: '\u{1F4D1}' },
        { path: '/api/docs/event-freq-heatmap', name: 'Event Heatmap', icon: '\u{1F525}' },
        { path: '/api/scalability/control-plane-pressure', name: 'Ctrl Plane', icon: '\u{1F3D7}' },
        { path: '/api/scalability/etcd-size-forecast', name: 'Etcd Forecast', icon: '\u{1F4BE}' },
        { path: '/api/scalability/scheduling-latency-estimator', name: 'Sched Latency', icon: '\u23F1' },
        { path: '/api/product/strategy-audit-2043', name: 'Strategy Audit', icon: '\u{1F501}' },
        { path: '/api/product/cm-reload-detector', name: 'CM Reload', icon: '\u{1F504}' },
        { path: '/api/product/pdb-readiness', name: 'PDB Readiness', icon: '\u{1F6E1}' },
        { path: '/api/deployment/rs-staleness-v2044', name: 'RS Staleness', icon: '\u{1F4E6}' },
        { path: '/api/deployment/pull-policy-audit', name: 'Pull Policy', icon: '\u2B07' },
        { path: '/api/deployment/max-surge-analyzer', name: 'Max Surge', icon: '\u{1F4C8}' },
        { path: '/api/operations/pod-qps-estimate', name: 'Pod QPS Est', icon: '\u{1F4C8}' },
        { path: '/api/operations/log-volume-anomaly', name: 'Log Anomaly', icon: '\u{1F4DD}' },
        { path: '/api/operations/node-condition-budget-2045', name: 'Node Cond Budget', icon: '\u26A0' },
        { path: '/api/security/rootfs-writable-audit', name: 'RootFS Writable', icon: '\u{1F4BE}' },
        { path: '/api/security/hostpath-mount-audit', name: 'HostPath Mount', icon: '\u{1F4C2}' },
        { path: '/api/security/token-secret-rotation', name: 'Token Rotation', icon: '\u{1F501}' },
        { path: '/api/docs/volume-snapshot-catalog-v2047', name: 'Snapshot Catalog', icon: '\u{1F4F7}' },
        { path: '/api/docs/priority-class-doc', name: 'Priority Class', icon: '\u26A1' },
        { path: '/api/docs/endpoint-slice-topology', name: 'EP Slice Topo', icon: '\u{1F5FA}' },
        { path: '/api/scalability/autoscale-behavior', name: 'Auto Behavior', icon: '\u{1F4C8}' },
        { path: '/api/scalability/node-pool-diversification', name: 'Node Pool', icon: '\u{1F5FA}' },
        { path: '/api/scalability/csi-driver-capacity', name: 'CSI Driver', icon: '\u{1F4BE}' },
        { path: '/api/product/owner-ref-audit', name: 'Owner Ref', icon: '\u{1F465}' },
        { path: '/api/product/svc-type-compliance', name: 'Svc Type', icon: '\u{1F310}' },
        { path: '/api/product/res-gap-analyzer', name: 'Res Gap', icon: '\u{1F4CF}' },
        { path: '/api/deployment/sts-update-compliance', name: 'STS Update', icon: '\u{1F501}' },
        { path: '/api/deployment/dns-policy-audit-v2050', name: 'DNS Policy v2', icon: '\u{1F310}' },
        { path: '/api/deployment/cmd-args-standard', name: 'Cmd Args', icon: '\u{1F4BB}' },
        { path: '/api/operations/kube-proxy-health-v2051', name: 'Kube Proxy v2', icon: '\u{1F310}' },
        { path: '/api/operations/cni-plugin-audit', name: 'CNI Plugin', icon: '\u{1F3A7}' },
        { path: '/api/operations/storage-op-latency', name: 'Stor Op Latency', icon: '\u{1F4BF}' },
        { path: '/api/security/secret-data-volume-audit', name: 'Secret Vol Audit', icon: '\u{1F510}' },
        { path: '/api/security/default-deny-check', name: 'Default Deny', icon: '\u{1F6E1}' },
        { path: '/api/security/webhook-risk-audit', name: 'Webhook Risk', icon: '\u26A0' },
        { path: '/api/docs/age-timeline', name: 'Age Timeline', icon: '\u{1F4C5}' },
        { path: '/api/docs/node-label-standardization-v2053', name: 'Node Label Std v2', icon: '\u{1F3F7}' },
        { path: '/api/docs/cluster-component-inventory', name: 'Comp Inventory', icon: '\u{1F4E6}' },
        { path: '/api/scalability/hpa-metric-coverage', name: 'HPA Metric', icon: '\u{1F4C8}' },
        { path: '/api/scalability/anti-affinity-coverage-v2054', name: 'Anti-Affinity v2', icon: '\u{1F310}' },
        { path: '/api/scalability/cluster-capacity-headroom-v2054', name: 'Cluster Cap v2', icon: '\u{1F4CF}' },
        { path: '/api/product/registry-diversity', name: 'Reg Diversity', icon: '\u{1F4E6}' },
        { path: '/api/product/ingress-backend-health', name: 'Ing Backend', icon: '\u{1F310}' },
        { path: '/api/product/pvc-lifecycle', name: 'PVC Lifecycle', icon: '\u{1F4BF}' },
        { path: '/api/deployment/condition-drift', name: 'Cond Drift', icon: '\u{1F501}' },
        { path: '/api/deployment/pss-validator', name: 'PSS Validator', icon: '\u{1F6E1}' },
        { path: '/api/deployment/resource-equality', name: 'Res Equality', icon: '\u{1F4CF}' },
        { path: '/api/operations/oom-risk-predictor', name: 'OOM Risk', icon: '\u26A0' },
        { path: '/api/operations/api-server-qps', name: 'API QPS', icon: '\u{1F4C8}' },
        { path: '/api/operations/node-pressure-score', name: 'Node Pressure', icon: '\u{1F534}' },
        { path: '/api/security/secret-age-tracker', name: 'Secret Age', icon: '\u{1F510}' },
        { path: '/api/security/rbac-binding-audit', name: 'RBAC Binding', icon: '\u{1F465}' },
        { path: '/api/security/escalation-surface', name: 'Esc Surface', icon: '\u26A0' },
        { path: '/api/docs/svc-port-mapping-v2059', name: 'Svc Ports v2', icon: '\u{1F50C}' },
        { path: '/api/docs/taint-effect-catalog-v2059', name: 'Taint Effect v2', icon: '\u26D4' },
        { path: '/api/docs/cm-key-inventory-v2059', name: 'CM Keys v2', icon: '\u{1F4DD}' },
        { path: '/api/scalability/replica-availability-budget', name: 'Replica Avail', icon: '\u{1F465}' },
        { path: '/api/scalability/workload-distribution-score', name: 'Wkld Dist', icon: '\u{1F4CA}' },
        { path: '/api/scalability/failover-readiness-v2060', name: 'Failover Ready v2', icon: '\u{1F6E1}' },
        { path: '/api/product/image-cache-hit', name: 'Image Cache', icon: '\u{1F4BE}' },
        { path: '/api/product/endpoint-health-distribution', name: 'EP Health', icon: '\u2705' },
        { path: '/api/product/ns-cost-allocation', name: 'NS Cost', icon: '\u{1F4B0}' },
        { path: '/api/deployment/generation-tracker', name: 'Gen Tracker', icon: '\u{1F4C8}' },
        { path: '/api/deployment/stdin-toggle-audit', name: 'Stdin Toggle', icon: '\u2328' },
        { path: '/api/deployment/spread-constraint-audit-v2062', name: 'Spread Audit v2', icon: '\u{1F310}' },
        { path: '/api/operations/pod-ready-latency-v2063', name: 'Ready Latency v2', icon: '\u23F1' },
        { path: '/api/operations/restart-velocity', name: 'Restart Velocity', icon: '\u{1F501}' },
        { path: '/api/operations/event-noise-filter-v2063', name: 'Event Noise v2', icon: '\u{1F50A}' },
        { path: '/api/security/cap-drop-audit-v2064', name: 'Cap Drop v2', icon: '\u{1F6E1}' },
        { path: '/api/security/seccomp-coverage-v2064', name: 'Seccomp v2', icon: '\u{1F6E1}' },
        { path: '/api/security/ns-isolation-score', name: 'NS Isolation', icon: '\u{1F512}' },
        { path: '/api/docs/pv-reclaim-catalog-v2065', name: 'PV Reclaim v2', icon: '\u{1F4BF}' },
        { path: '/api/docs/sa-inventory-v2065', name: 'SA Inv v2', icon: '\u{1F511}' },
        { path: '/api/docs/node-condition-timeline-v2065', name: 'Node Cond TL v2', icon: '\u{1F550}' },
        { path: '/api/scalability/hpa-thrash-detect', name: 'HPA Thrash', icon: '\u{1F4C8}' },
        { path: '/api/scalability/eviction-readiness', name: 'Evict Ready', icon: '\u{1F6E1}' },
        { path: '/api/scalability/cluster-scaling-headroom', name: 'Scale Headroom', icon: '\u{1F4CF}' },
        { path: '/api/product/wkld-density-profile', name: 'Wkld Density', icon: '\u{1F4CA}' },
        { path: '/api/product/image-vintage-tracker', name: 'Img Vintage', icon: '\u{1F4D6}' },
        { path: '/api/product/svc-protocol-distribution', name: 'Svc Protocol', icon: '\u{1F310}' },
        { path: '/api/deployment/rolling-window-inspector-v2068', name: 'Rolling Window v2', icon: '\u{1F4C8}' },
        { path: '/api/deployment/preemption-tracker', name: 'Preempt Track', icon: '\u26A1' },
        { path: '/api/deployment/sts-partition-audit', name: 'STS Partition', icon: '\u{1F4CF}' },
        { path: '/api/operations/cpu-saturation', name: 'CPU Saturation', icon: '\u{1F525}' },
        { path: '/api/operations/mem-wset-estimate', name: 'Mem WSet', icon: '\u{1F4BE}' },
        { path: '/api/operations/startup-phase-tracker', name: 'Startup Phase', icon: '\u23F1' },
        { path: '/api/security/secret-type-audit-v2070', name: 'Secret Type v2', icon: '\u{1F510}' },
        { path: '/api/security/sa-privilege-analysis', name: 'SA Privilege', icon: '\u{1F451}' },
        { path: '/api/security/runas-user-validator', name: 'RunAsUser', icon: '\u{1F6E1}' },
        { path: '/api/docs/sc-provisioner-map-v2071', name: 'SC Provisioner v2', icon: '\u{1F4BF}' },
        { path: '/api/docs/ns-annotation-standard-v2071', name: 'NS Annot Std v2', icon: '\u{1F4DD}' },
        { path: '/api/docs/crd-conversion-strategy', name: 'CRD Conversion', icon: '\u{1F504}' },
        { path: '/api/scalability/pod-distribution-balance', name: 'Pod Balance', icon: '\u{1F4CA}' },
        { path: '/api/scalability/request-saturation-v2072', name: 'Req Saturation v2', icon: '\u{1F4CF}' },
        { path: '/api/scalability/volume-attachment-spread', name: 'Vol Attach', icon: '\u{1F4BF}' },
        { path: '/api/product/affinity-catalog-v2073', name: 'Affinity Cat v2', icon: '\u{1F517}' },
        { path: '/api/product/ingress-tls-coverage', name: 'Ing TLS Cover', icon: '\u{1F510}' },
        { path: '/api/product/configmap-rotation-tracker', name: 'CM Rotation', icon: '\u{1F504}' },
        { path: '/api/deployment/progress-deadline-audit', name: 'Progress Deadline', icon: '\u23F1' },
        { path: '/api/deployment/workdir-validator', name: 'WorkDir Valid', icon: '\u{1F4C2}' },
        { path: '/api/deployment/rs-orphan-detect', name: 'RS Orphan', icon: '\u{1F4E6}' },
        { path: '/api/operations/crash-pattern-detect', name: 'Crash Pattern', icon: '\u{1F534}' },
        { path: '/api/operations/pull-time-estimate-v2075', name: 'Pull Time v2', icon: '\u2B07' },
        { path: '/api/operations/allocatable-efficiency-v2075', name: 'Alloc Eff v2', icon: '\u{1F4CF}' },
        { path: '/api/security/privileged-inventory-v2076', name: 'Priv Inv v2', icon: '\u26A0' },
        { path: '/api/security/volume-permission-audit', name: 'Vol Perm', icon: '\u{1F4BF}' },
        { path: '/api/security/clusterrole-aggregation', name: 'CR Aggregation', icon: '\u{1F465}' },
        { path: '/api/docs/pod-ip-inventory-v2077', name: 'Pod IP Inv', icon: '\u{1F310}' },
        { path: '/api/docs/node-os-catalog-v2077', name: 'Node OS v2', icon: '\u{1F4BB}' },
        { path: '/api/docs/external-ip-tracker', name: 'Ext IP Track', icon: '\u{1F310}' },
        { path: '/api/scalability/scheduling-score-v2078', name: 'Sched Score v2', icon: '\u23F1' },
        { path: '/api/scalability/fragmentation-analysis-v2078', name: 'Fragmentation v2', icon: '\u{1F4CA}' },
        { path: '/api/scalability/multizone-ha-validator', name: 'Multi-Zone HA', icon: '\u{1F310}' },
        { path: '/api/product/topo-pin-analysis-v2079', name: 'Topo Pin', icon: '\u{1F4CD}' },
        { path: '/api/product/ingress-rule-complexity', name: 'Ing Rule Complex', icon: '\u{1F4C8}' },
        { path: '/api/product/storage-access-mode-coverage', name: 'Access Mode', icon: '\u{1F4BF}' },
        { path: '/api/deployment/vc-claim-audit-v2080', name: 'VC Claim Audit', icon: '\u{1F4BF}' },
        { path: '/api/deployment/ds-update-compliance-v2080', name: 'DS Update', icon: '\u{1F4E6}' },
        { path: '/api/deployment/history-depth-v2080', name: 'History Depth', icon: '\u{1F4DC}' },
        { path: '/api/operations/pod-phase-distribution-v2081', name: 'Phase Dist v2', icon: '\u{1F4CA}' },
        { path: '/api/operations/kubelet-version-drift-v2081', name: 'KV Drift', icon: '\u{1F4BB}' },
        { path: '/api/operations/port-conflict-detect-v2081', name: 'Port Conflict', icon: '\u26A0' },
        { path: '/api/security/egress-audit-v2082', name: 'Egress Audit', icon: '\u2197' },
        { path: '/api/security/audit-log-config-v2082', name: 'Audit Log', icon: '\u{1F4DD}' },
        { path: '/api/security/certificate-age-tracker-v2082', name: 'Cert Age v2', icon: '\u{1F4DC}' },
        { path: '/api/docs/node-arch-map-v2083', name: 'Node Arch v2', icon: '\u{1F578}' },
        { path: '/api/docs/event-age-distribution-v2083', name: 'Event Age v2', icon: '\u{1F550}' },
        { path: '/api/docs/clusterip-range-usage-v2083', name: 'ClusterIP Usage', icon: '\u{1F310}' },
        { path: '/api/scalability/capacity-trend-v2084', name: 'Cap Trend v2', icon: '\u{1F4C8}' },
        { path: '/api/scalability/pod-density-forecast-v2084', name: 'Pod Forecast v2', icon: '\u{1F4C8}' },
        { path: '/api/scalability/ha-coverage-v2084', name: 'HA Coverage', icon: '\u{1F6E1}' },
        { path: '/api/product/grace-shutdown-v2085', name: 'Grace Shutdown', icon: '\u23F1' },
        { path: '/api/product/mesh-readiness-v2085', name: 'Mesh Ready', icon: '\u{1F3D7}' },
        { path: '/api/product/snapshot-retention-v2085', name: 'Snap Retention', icon: '\u{1F4F7}' },
        { path: '/api/deployment/res-gap-wide-v2086', name: 'Res Gap Wide', icon: '\u{1F4CF}' },
        { path: '/api/deployment/qos-distribution-v2086', name: 'QoS Dist', icon: '\u{1F4CA}' },
        { path: '/api/deployment/revision-staleness-v2086', name: 'Rev Stale', icon: '\u{1F4DC}' },
        { path: '/api/operations/restart-trend-v2087', name: 'Restart Trend', icon: '\u{1F501}' },
        { path: '/api/operations/heartbeat-freshness-v2087', name: 'Heartbeat', icon: '\u2764' },
        { path: '/api/operations/event-severity-v2087', name: 'Event Severity', icon: '\u26A0' },
        { path: '/api/security/sa-token-age-v2088', name: 'SA Token Age', icon: '\u{1F511}' },
        { path: '/api/security/priv-escalation-v2088', name: 'Priv Esc', icon: '\u26A0' },
        { path: '/api/security/np-direction-coverage-v2088', name: 'NP Direction', icon: '\u2194' },
        { path: '/api/docs/node-capacity-summary-v2089', name: 'Node Cap Sum', icon: '\u{1F4BE}' },
        { path: '/api/docs/secret-key-count-v2089', name: 'Secret Keys', icon: '\u{1F510}' },
        { path: '/api/docs/quota-catalog-v2089', name: 'Quota Catalog', icon: '\u{1F4B0}' },
        { path: '/api/scalability/sched-latency-v2090', name: 'Sched Latency', icon: '\u23F1' },
        { path: '/api/scalability/overcommit-ratio-v2090', name: 'Overcommit', icon: '\u{1F4C8}' },
        { path: '/api/scalability/pod-density-histogram-v2090', name: 'Pod Density', icon: '\u{1F4CA}' },
        { path: '/api/product/label-compliance-v2091', name: 'Label Compliance', icon: '\u{1F3F7}' },
        { path: '/api/product/port-collision-v2091', name: 'Port Collision', icon: '\u26A0' },
        { path: '/api/product/restart-budget-v2091', name: 'Restart Budget', icon: '\u{1F501}' },
        { path: '/api/deployment/sts-ordinal-v2092', name: 'STS Ordinal', icon: '\u{1F522}' },
        { path: '/api/deployment/ds-selector-coverage-v2092', name: 'DS Selector', icon: '\u{1F50D}' },
        { path: '/api/deployment/env-var-consistency-v2092', name: 'Env Consistency', icon: '\u{1F4DD}' },
        { path: '/api/operations/image-size-estimate-v2093', name: 'Img Size Est', icon: '\u{1F4BE}' },
        { path: '/api/operations/kubelet-efficiency-v2093', name: 'Kubelet Eff', icon: '\u2699' },
        { path: '/api/operations/event-ttl-v2093', name: 'Event TTL', icon: '\u23F1' },
        { path: '/api/security/secret-exposure-v2094', name: 'Secret Exposure', icon: '\u{1F510}' },
        { path: '/api/security/wildcard-rbac-v2094', name: 'Wildcard RBAC', icon: '\u2728' },
        { path: '/api/security/fsgroup-validator-v2094', name: 'fsGroup Valid', icon: '\u{1F465}' },
        { path: '/api/docs/bind-mode-catalog-v2095', name: 'Bind Mode', icon: '\u{1F4BF}' },
        { path: '/api/docs/kernel-version-map-v2095', name: 'Kernel Map', icon: '\u{1F4BB}' },
        { path: '/api/docs/session-affinity-catalog-v2095', name: 'Session Affinity', icon: '\u{1F517}' },
        { path: '/api/scalability/limit-coverage-v2096', name: 'Limit Coverage', icon: '\u{1F4CF}' },
        { path: '/api/scalability/node-failure-impact-v2096', name: 'Node Fail Impact', icon: '\u{1F534}' },
        { path: '/api/scalability/pvc-storage-distribution-v2096', name: 'PVC Storage Dist', icon: '\u{1F4BF}' },
        { path: '/api/product/wait-reason-v2097', name: 'Wait Reason', icon: '\u23F3' },
        { path: '/api/product/epslice-count-v2097', name: 'EP Slice Cnt', icon: '\u{1F5FA}' },
        { path: '/api/product/lifecycle-hook-v2097', name: 'Lifecycle Hook', icon: '\u{1F501}' },
        { path: '/api/deployment/paused-status-v2098', name: 'Paused Status', icon: '\u23F8' },
        { path: '/api/deployment/topo-spread-v2098', name: 'Topo Spread v2', icon: '\u{1F310}' },
        { path: '/api/deployment/secctx-completeness-v2098', name: 'SecCtx Complete', icon: '\u{1F6E1}' },
        { path: '/api/operations/ctnr-state-v2099', name: 'Ctnr State', icon: '\u{1F4CA}' },
        { path: '/api/operations/runtime-map-v2099', name: 'Runtime Map', icon: '\u{1F4BB}' },
        { path: '/api/operations/qos-eviction-risk-v2099', name: 'QoS Eviction', icon: '\u26A0' },
        { path: '/api/security/np-empty-selector-v2100', name: 'NP Empty Sel', icon: '\u{1F310}' },
        { path: '/api/security/host-alias-v2100', name: 'Host Alias', icon: '\u{1F4BB}' },
        { path: '/api/security/secret-mount-path-v2100', name: 'Secret Mount', icon: '\u{1F510}' },
        { path: '/api/docs/boot-id-catalog-v2101', name: 'Boot ID', icon: '\u{1F4BB}' },
        { path: '/api/docs/svc-type-histogram-v2101', name: 'Svc Type Hist', icon: '\u{1F4CA}' },
        { path: '/api/docs/dns-config-catalog-v2101', name: 'DNS Config', icon: '\u{1F310}' },
        { path: '/api/scalability/antiaff-effectiveness-v2102', name: 'AntiAff Eff', icon: '\u{1F310}' },
        { path: '/api/scalability/cpu-request-waste-v2102', name: 'CPU Waste', icon: '\u{1F4C8}' },
        { path: '/api/scalability/ns-quota-headroom-v2102', name: 'NS Quota HR', icon: '\u{1F4CF}' },
        { path: '/api/product/startup-probe-v2103', name: 'Startup Probe', icon: '\u{1F50D}' },
        { path: '/api/product/ext-traffic-policy-v2103', name: 'Ext Traffic', icon: '\u{1F310}' },
        { path: '/api/product/ns-finalizer-v2103', name: 'NS Finalizer', icon: '\u{1F527}' },
        { path: '/api/deployment/strategy-validator-v2104', name: 'Strategy Valid', icon: '\u{1F501}' },
        { path: '/api/deployment/sts-svc-binding-v2104', name: 'STS Svc Binding', icon: '\u{1F517}' },
        { path: '/api/deployment/ds-toleration-v2104', name: 'DS Toleration', icon: '\u26D4' },
        { path: '/api/operations/alloc-pod-ratio-v2105', name: 'Alloc Pod Ratio', icon: '\u{1F4CA}' },
        { path: '/api/operations/ctnr-count-dist-v2105', name: 'Ctnr Count', icon: '\u{1F522}' },
        { path: '/api/operations/event-source-dist-v2105', name: 'Event Source', icon: '\u{1F4E1}' },
        { path: '/api/security/sa-token-mount-v2106', name: 'SA Token Mount', icon: '\u{1F511}' },
        { path: '/api/security/volume-projection-v2106', name: 'Vol Projection', icon: '\u{1F4BF}' },
        { path: '/api/security/crb-subject-type-v2106', name: 'CRB Subject', icon: '\u{1F465}' },
        { path: '/api/docs/pvc-sc-distribution-v2107', name: 'PVC SC Dist', icon: '\u{1F4BF}' },
        { path: '/api/docs/runtime-version-map-v2107', name: 'Runtime Ver Map', icon: '\u{1F4BB}' },
        { path: '/api/docs/restart-policy-catalog-v2107', name: 'Restart Policy', icon: '\u{1F501}' },
        { path: '/api/scalability/mem-efficiency-v2108', name: 'Mem Efficiency', icon: '\u{1F4BE}' },
        { path: '/api/scalability/pod-ip-allocation-v2108', name: 'Pod IP Alloc', icon: '\u{1F310}' },
        { path: '/api/scalability/replica-concentration-v2108', name: 'Replica Conc', icon: '\u{1F465}' },
        { path: '/api/product/subresource-inventory-v2109', name: 'Subresource Inv', icon: '\u{1F4E6}' },
        { path: '/api/product/ingress-annot-compliance-v2109', name: 'Ing Annot', icon: '\u{1F4DD}' },
        { path: '/api/product/ns-pod-capacity-ratio-v2109', name: 'NS Pod Cap', icon: '\u{1F4CF}' },
        { path: '/api/deployment/readiness-gate-v2110', name: 'Ready Gate', icon: '\u2705' },
        { path: '/api/deployment/pdb-min-available-v2110', name: 'PDB Min Avail', icon: '\u{1F6E1}' },
        { path: '/api/deployment/image-digest-pinning-v2110', name: 'Img Digest', icon: '\u{1F510}' },
        { path: '/api/operations/taint-effect-v2111', name: 'Taint Effect', icon: '\u26D4' },
        { path: '/api/operations/pod-condition-health-v2111', name: 'Pod Cond Health', icon: '\u2764' },
        { path: '/api/operations/vol-mount-count-v2111', name: 'Vol Mount Cnt', icon: '\u{1F4BF}' },
        { path: '/api/security/sa-pullsecret-v2112', name: 'SA PullSecret', icon: '\u{1F511}' },
        { path: '/api/security/runas-nonroot-v2112', name: 'runAsNonRoot', icon: '\u{1F6E1}' },
        { path: '/api/security/secret-immutable-v2112', name: 'Secret Immutable', icon: '\u{1F512}' },
        { path: '/api/docs/label-diversity-v2113', name: 'Label Diversity', icon: '\u{1F3F7}' },
        { path: '/api/docs/port-name-inventory-v2113', name: 'Port Name Inv', icon: '\u{1F50C}' },
        { path: '/api/docs/annot-key-distribution-v2113', name: 'Annot Key Dist', icon: '\u{1F4DD}' },
        { path: '/api/scalability/cpu-burst-headroom-v2114', name: 'CPU Burst HR', icon: '\u{1F525}' },
        { path: '/api/scalability/pod-spread-evenness-v2114', name: 'Pod Spread Even', icon: '\u{1F310}' },
        { path: '/api/scalability/ns-resource-footprint-v2114', name: 'NS Footprint', icon: '\u{1F4B0}' },
        { path: '/api/product/sa-token-volume-v2115', name: 'SA Token Vol', icon: '\u{1F511}' },
        { path: '/api/product/rev-history-limit-v2115', name: 'Rev History Lim', icon: '\u{1F4DC}' },
        { path: '/api/deployment/probe-timeout-v2116', name: 'Probe Timeout', icon: '\u23F1' },
        { path: '/api/deployment/init-resource-v2116', name: 'Init Resource', icon: '\u{1F4CF}' },
        { path: '/api/deployment/max-unavailable-v2116', name: 'Max Unavail', icon: '\u26A0' },
        { path: '/api/operations/machine-info-v2117', name: 'Machine Info', icon: '\u{1F4BB}' },
        { path: '/api/operations/exit-code-dist-v2117', name: 'Exit Code Dist', icon: '\u{1F534}' },
        { path: '/api/operations/lb-health-v2117', name: 'LB Health', icon: '\u2705' },
        { path: '/api/security/supp-group-v2118', name: 'Supp Group', icon: '\u{1F465}' },
        { path: '/api/security/default-sa-priv-v2118', name: 'Default SA Priv', icon: '\u26A0' },
        { path: '/api/security/np-ingress-complexity-v2118', name: 'NP Ingress Cmplx', icon: '\u{1F310}' },
        { path: '/api/docs/node-allocatable-v2119', name: 'Node Allocatable', icon: '\u{1F4BE}' },
        { path: '/api/docs/pvc-phase-dist-v2119', name: 'PVC Phase Dist', icon: '\u{1F4BF}' },
        { path: '/api/docs/toleration-catalog-v2119', name: 'Toleration Catalog', icon: '\u26D4' },
        { path: '/api/scalability/mem-limit-alloc-v2120', name: 'Mem Limit Alloc', icon: '\u{1F4BE}' },
        { path: '/api/scalability/ns-quartile-v2120', name: 'NS Quartile', icon: '\u{1F4CA}' },
        { path: '/api/scalability/blast-radius-v2120', name: 'Blast Radius', icon: '\u{1F534}' },
        { path: '/api/product/term-grace-v2121', name: 'Term Grace', icon: '\u23F1' },
        { path: '/api/product/int-traffic-policy-v2121', name: 'Int Traffic', icon: '\u{1F310}' },
        { path: '/api/product/ns-age-distribution-v2121', name: 'NS Age Dist', icon: '\u{1F4C5}' },
        { path: '/api/deployment/os-selector-v2122', name: 'OS Selector', icon: '\u{1F4BB}' },
        { path: '/api/deployment/lim-req-ratio-v2122', name: 'Lim Req Ratio', icon: '\u{1F4CF}' },
        { path: '/api/deployment/surge-efficiency-v2122', name: 'Surge Eff', icon: '\u{1F4C8}' },
        { path: '/api/operations/qos-guaranteed-v2123', name: 'QoS Guaranteed', icon: '\u2705' },
        { path: '/api/operations/kp-version-map-v2123', name: 'KP Version', icon: '\u{1F4BB}' },
        { path: '/api/operations/vol-device-path-v2123', name: 'Vol Device', icon: '\u{1F4BF}' },
        { path: '/api/security/sysctl-validator-v2124', name: 'Sysctl Valid', icon: '\u2699' },
        { path: '/api/security/psa-level-v2124', name: 'PSA Level', icon: '\u{1F6E1}' },
        { path: '/api/security/cr-verbs-resource-v2124', name: 'CR Verbs Res', icon: '\u{1F465}' },
        { path: '/api/docs/feature-label-v2125', name: 'Feature Label', icon: '\u{1F3F7}' },
        { path: '/api/docs/sa-mapping-v2125', name: 'SA Mapping', icon: '\u{1F511}' },
        { path: '/api/docs/cm-immutable-v2125', name: 'CM Immutable', icon: '\u{1F512}' },
        { path: '/api/scalability/cpu-throttle-risk-v2126', name: 'CPU Throttle', icon: '\u{1F525}' },
        { path: '/api/scalability/pvc-reclaim-waste-v2126', name: 'PVC Waste', icon: '\u{1F4BF}' },
        { path: '/api/scalability/ns-replica-spread-v2126', name: 'NS Replica', icon: '\u{1F465}' },
        { path: '/api/product/active-deadline-v2127', name: 'Active Deadline', icon: '\u23F1' },
        { path: '/api/product/ip-family-dist-v2127', name: 'IP Family', icon: '\u{1F310}' },
        { path: '/api/product/label-diversity-score-v2127', name: 'Label Div Score', icon: '\u{1F3F7}' },
        { path: '/api/deployment/res-delta-v2128', name: 'Res Delta', icon: '\u{1F4CF}' },
        { path: '/api/deployment/evict-multiplier-v2128', name: 'Evict Mult', icon: '\u{1F6E1}' },
        { path: '/api/deployment/ctnr-restart-budget-v2128', name: 'Ctnr Restart Bud', icon: '\u{1F501}' },
        { path: '/api/operations/alloc-eff-ratio-v2129', name: 'Alloc Eff Ratio', icon: '\u{1F4CA}' },
        { path: '/api/operations/restart-avg-v2129', name: 'Restart Avg', icon: '\u{1F4C8}' },
        { path: '/api/operations/port-range-dist-v2129', name: 'Port Range', icon: '\u{1F50C}' },
        { path: '/api/security/host-pid-ipc-v2130', name: 'Host PID/IPC', icon: '\u26A0' },
        { path: '/api/security/sa-automount-v2130', name: 'SA Automount', icon: '\u{1F511}' },
        { path: '/api/security/np-coverage-ratio-v2130', name: 'NP Coverage', icon: '\u{1F310}' },
        { path: '/api/docs/event-reason-top-v2131', name: 'Event Reason Top', icon: '\u{1F4CB}' },
        { path: '/api/docs/instance-type-v2131', name: 'Instance Type', icon: '\u{1F4BB}' },
        { path: '/api/docs/pv-reclaim-diversity-v2131', name: 'PV Reclaim Div', icon: '\u{1F4BF}' },
        { path: '/api/scalability/mem-eff-ns-v2132', name: 'Mem Eff NS', icon: '\u{1F4BE}' },
        { path: '/api/scalability/density-risk-v2132', name: 'Density Risk', icon: '\u26A0' },
        { path: '/api/scalability/hpa-target-match-v2132', name: 'HPA Target Match', icon: '\u{1F4C8}' },
        { path: '/api/product/sa-override-v2133', name: 'SA Override', icon: '\u{1F511}' },
        { path: '/api/product/ingress-path-type-v2133', name: 'Ing Path Type', icon: '\u{1F517}' },
        { path: '/api/product/pvc-finalizer-v2133', name: 'PVC Finalizer', icon: '\u{1F4BF}' },
        { path: '/api/deployment/scheduler-name-v2134', name: 'Scheduler Name', icon: '\u2699' },
        { path: '/api/deployment/stdin-tty-v2134', name: 'Stdin TTY', icon: '\u2328' },
        { path: '/api/deployment/replica-range-v2134', name: 'Replica Range', icon: '\u{1F4CF}' },
        { path: '/api/operations/node-trans-v2135', name: 'Node Trans', icon: '\u2705' },
        { path: '/api/operations/ctnr-age-v2135', name: 'Ctnr Age', icon: '\u{1F4C5}' },
        { path: '/api/operations/ep-ready-ratio-v2135', name: 'EP Ready Ratio', icon: '\u{1F4CA}' },
        { path: '/api/security/hostpath-write-v2136', name: 'HostPath Write', icon: '\u26A0' },
        { path: '/api/security/psa-priv-v2136', name: 'PSA Priv', icon: '\u26A0' },
        { path: '/api/security/sa-secret-ref-v2136', name: 'SA Secret Ref', icon: '\u{1F510}' },
        { path: '/api/docs/provider-id-v2137', name: 'Provider ID', icon: '\u{1F4BB}' },
        { path: '/api/docs/crd-scope-v2137', name: 'CRD Scope', icon: '\u{1F4C2}' },
        { path: '/api/docs/lb-class-v2137', name: 'LB Class', icon: '\u{1F310}' },
        { path: '/api/scalability/cpu-overcommit-node-v2138', name: 'CPU Overcommit Node', icon: '\u{1F525}' },
        { path: '/api/scalability/bin-pack-score-v2138', name: 'Bin Pack Score', icon: '\u{1F4E6}' },
        { path: '/api/scalability/ns-ha-multiplier-v2138', name: 'NS HA Mult', icon: '\u{1F6E1}' },
        { path: '/api/product/dns-policy-v2139', name: 'DNS Policy', icon: '\u{1F310}' },
        { path: '/api/product/oversubscription-v2139', name: 'Oversubscription', icon: '\u{1F4CF}' },
        { path: '/api/product/publish-notready-v2139', name: 'Pub NotReady', icon: '\u26A0' },
        { path: '/api/deployment/hostname-fqdn-v2140', name: 'Hostname FQDN', icon: '\u{1F4BB}' },
        { path: '/api/deployment/workdir-overlap-v2140', name: 'WorkDir Overlap', icon: '\u{1F4C2}' },
        { path: '/api/deployment/owner-ref-v2140', name: 'Owner Ref', icon: '\u{1F517}' },
        { path: '/api/operations/cond-chron-v2141', name: 'Cond Chron', icon: '\u{1F4CA}' },
        { path: '/api/operations/phase-rate-v2141', name: 'Phase Rate', icon: '\u{1F504}' },
        { path: '/api/operations/evt-obj-type-v2141', name: 'Evt Obj Type', icon: '\u{1F4E1}' },
        { path: '/api/security/selinux-v2142', name: 'SELinux', icon: '\u{1F6E1}' },
        { path: '/api/security/rbac-per-ns-v2142', name: 'RBAC Per NS', icon: '\u{1F465}' },
        { path: '/api/security/secret-type-enf-v2142', name: 'Secret Type Enf', icon: '\u{1F510}' },
        { path: '/api/docs/node-addr-v2143', name: 'Node Addr', icon: '\u{1F4CD}' },
        { path: '/api/docs/pvc-source-v2143', name: 'PVC Source', icon: '\u{1F4BF}' },
        { path: '/api/docs/pod-subdomain-v2143', name: 'Pod Subdomain', icon: '\u{1F310}' },
        { path: '/api/scalability/cpu-quartile-v2144', name: 'CPU Quartile', icon: '\u{1F4CA}' },
        { path: '/api/scalability/pvc-overhead-v2144', name: 'PVC Overhead', icon: '\u{1F4BF}' },
        { path: '/api/scalability/ns-forecast-v2144', name: 'NS Forecast', icon: '\u{1F4C8}' },
        { path: '/api/product/pod-overhead-v2145', name: 'Pod Overhead', icon: '\u{1F4CF}' },
        { path: '/api/product/stdin-once-v2145', name: 'Stdin Once', icon: '\u2328' },
        { path: '/api/product/alloc-lb-nodeports-v2145', name: 'Alloc LB NP', icon: '\u{1F310}' },
        { path: '/api/deployment/ephemeral-v2146', name: 'Ephemeral', icon: '\u{1F501}' },
        { path: '/api/deployment/min-ready-v2146', name: 'Min Ready', icon: '\u2705' },
        { path: '/api/deployment/pullsecret-cov-v2146', name: 'PullSecret Cov', icon: '\u{1F510}' },
        { path: '/api/operations/unschedulable-v2147', name: 'Unschedulable', icon: '\u26D4' },
        { path: '/api/operations/ctnr-ready-ratio-v2147', name: 'Ctnr Ready', icon: '\u2705' },
        { path: '/api/operations/evt-freshness-v2147', name: 'Evt Freshness', icon: '\u{1F550}' },
        { path: '/api/security/readonly-rootfs-v2148', name: 'RO RootFS', icon: '\u{1F512}' },
        { path: '/api/security/crb-agg-count-v2148', name: 'CRB Agg Count', icon: '\u{1F465}' },
        { path: '/api/security/sa-annot-secret-v2148', name: 'SA Annot Secret', icon: '\u{1F511}' },
        { path: '/api/docs/cap-alloc-gap-v2149', name: 'Cap Alloc Gap', icon: '\u{1F4BE}' },
        { path: '/api/docs/pvc-volname-v2149', name: 'PVC VolName', icon: '\u{1F4BF}' },
        { path: '/api/docs/pod-hostname-v2149', name: 'Pod Hostname', icon: '\u{1F4BB}' },
        { path: '/api/scalability/mem-forecast-v2150', name: 'Mem Forecast', icon: '\u{1F4BE}' },
        { path: '/api/scalability/topo-key-cov-v2150', name: 'Topo Key Cov', icon: '\u{1F310}' },
        { path: '/api/scalability/ns-cpu-quota-v2150', name: 'NS CPU Quota', icon: '\u{1F525}' },
        { path: '/api/product/share-proc-ns-v2151', name: 'Share ProcNS', icon: '\u{1F501}' },
        { path: '/api/product/term-msg-policy-v2151', name: 'Term Msg Policy', icon: '\u{1F4DD}' },
        { path: '/api/product/app-protocol-v2151', name: 'AppProtocol', icon: '\u{1F310}' },
        { path: '/api/deployment/node-sel-req-v2152', name: 'Node Sel Req', icon: '\u{1F4CD}' },
        { path: '/api/deployment/img-vol-mount-v2152', name: 'Img Vol Mount', icon: '\u{1F4BE}' },
        { path: '/api/deployment/dep-cond-v2152', name: 'Dep Cond', icon: '\u2705' },
        { path: '/api/operations/sys-info-v2153', name: 'Sys Info', icon: '\u{1F4BB}' },
        { path: '/api/operations/besteffort-v2153', name: 'BestEffort', icon: '\u26A0' },
        { path: '/api/operations/vol-name-cons-v2153', name: 'Vol Name Cons', icon: '\u{1F4BF}' },
        { path: '/api/security/seccomp-type-v2154', name: 'Seccomp Type', icon: '\u{1F6E1}' },
        { path: '/api/security/default-deny-v2154', name: 'Default Deny', icon: '\u{1F310}' },
        { path: '/api/security/cr-wild-verb-v2154', name: 'CR Wild Verb', icon: '\u2728' },
        { path: '/api/docs/taint-key-v2155', name: 'Taint Key', icon: '\u26D4' },
        { path: '/api/docs/pvc-default-sc-v2155', name: 'PVC Default SC', icon: '\u{1F4BF}' },
        { path: '/api/docs/rest-pol-owner-v2155', name: 'Rest Pol Owner', icon: '\u{1F501}' },
        { path: '/api/scalability/cpu-conc-v2156', name: 'CPU Conc', icon: '\u{1F525}' },
        { path: '/api/scalability/aff-cost-v2156', name: 'Aff Cost', icon: '\u{1F4C8}' },
        { path: '/api/scalability/ns-multi-ha-v2156', name: 'NS Multi HA', icon: '\u{1F6E1}' },
        { path: '/api/product/topo-spread-audit-v2158', name: 'Topo Spread Audit', icon: '\u{1F310}' },
        { path: '/api/product/extname-health-v2158', name: 'ExtName Health', icon: '\u{1F517}' },
        { path: '/api/product/cap-drop-audit-v2158', name: 'Cap Drop', icon: '\u{1F6E1}' },
        { path: '/api/deployment/sts-pod-mgmt-v2159', name: 'STS Pod Mgmt', icon: '\u{2699}' },
        { path: '/api/deployment/ds-upd-strat-v2159', name: 'DS Upd Strat', icon: '\u{1F501}' },
        { path: '/api/deployment/prog-deadline-v2159', name: 'Prog Deadline', icon: '\u{23F1}' },
        { path: '/api/operations/node-mem-eff-v2160', name: 'Node Mem Eff', icon: '\u{1F4BE}' },
        { path: '/api/operations/port-proto-v2160', name: 'Port Proto', icon: '\u{1F50C}' },
        { path: '/api/operations/eps-addr-count-v2160', name: 'EPS Addr Cnt', icon: '\u{1F5FA}' },
        { path: '/api/security/uid-range-v2161', name: 'UID Range', icon: '\u{1F464}' },
        { path: '/api/security/sec-count-type-v2161', name: 'Sec Count Type', icon: '\u{1F510}' },
        { path: '/api/security/np-egress-port-v2161', name: 'NP Egress Port', icon: '\u{1F310}' },
        { path: '/api/docs/node-uptime-v2162', name: 'Node Uptime', icon: '\u{23F1}' },
        { path: '/api/docs/pvc-access-mode-v2162', name: 'PVC Access Mode', icon: '\u{1F4BF}' },
        { path: '/api/docs/img-registry-v2162', name: 'Img Registry', icon: '\u{1F4BE}' },
        { path: '/api/scalability/pod-cap-forecast-v2163', name: 'Pod Cap Forecast', icon: '\u{1F4C8}' },
        { path: '/api/scalability/ns-dep-ha-v2163', name: 'NS Dep HA', icon: '\u{1F6E1}' },
        { path: '/api/scalability/pvc-quota-hr-v2163', name: 'PVC Quota HR', icon: '\u{1F4BF}' },
        { path: '/api/product/pull-policy-v2164', name: 'Pull Policy', icon: '\u{1F4BE}' },
        { path: '/api/product/term-msg-path-v2164', name: 'Term Msg Path', icon: '\u{1F4DD}' },
        { path: '/api/product/health-proxy-v2164', name: 'Health Proxy', icon: '\u{2705}' },
        { path: '/api/deployment/rs-orphan-v2165', name: 'RS Orphan', icon: '\u{1F501}' },
        { path: '/api/deployment/job-comp-v2165', name: 'Job Comp', icon: '\u{2705}' },
        { path: '/api/deployment/cron-conflict-v2165', name: 'Cron Conflict', icon: '\u{23F1}' },
        { path: '/api/operations/node-cond-summary-v2166', name: 'Node Cond Sum', icon: '\u{26A0}' },
        { path: '/api/operations/restart-timeline-v2166', name: 'Restart Timeline', icon: '\u{1F501}' },
        { path: '/api/operations/svc-ep-health-v2166', name: 'Svc EP Health', icon: '\u{2705}' },
        { path: '/api/security/priv-escalation-v2167', name: 'Priv Escalation', icon: '\u{26A0}' },
        { path: '/api/security/sa-token-expiry-v2167', name: 'SA Token Expiry', icon: '\u{1F511}' },
        { path: '/api/security/ns-rbac-scope-v2167', name: 'NS RBAC Scope', icon: '\u{1F465}' },
        { path: '/api/docs/node-arch-v2168', name: 'Node Arch', icon: '\u{1F4BB}' },
        { path: '/api/docs/cm-key-count-v2168', name: 'CM Key Count', icon: '\u{1F4DD}' },
        { path: '/api/docs/vol-type-v2168', name: 'Vol Type', icon: '\u{1F4BF}' },
        { path: '/api/scalability/frag-score-v2169', name: 'Frag Score', icon: '\u{1F4CA}' },
        { path: '/api/scalability/ns-wk-spread-v2169', name: 'NS Wk Spread', icon: '\u{1F310}' },
        { path: '/api/scalability/cluster-density-v2169', name: 'Cluster Density', icon: '\u{1F4CA}' },
        { path: '/api/product/svc-resolution-v2170', name: 'Svc Resolution', icon: '\u{1F517}' },
        { path: '/api/product/ctnr-args-v2170', name: 'Ctnr Args', icon: '\u{1F4DD}' },
        { path: '/api/product/port-target-v2170', name: 'Port Target', icon: '\u{1F50C}' },
        { path: '/api/deployment/avail-replica-v2171', name: 'Avail Replica', icon: '\u{2705}' },
        { path: '/api/deployment/sts-partition-v2171', name: 'STS Partition', icon: '\u{2699}' },
        { path: '/api/deployment/ds-rollout-v2171', name: 'DS Rollout', icon: '\u{1F501}' },
        { path: '/api/operations/img-freshness-v2172', name: 'Img Freshness', icon: '\u{1F4BE}' },
        { path: '/api/operations/alloc-cpu-ratio-v2172', name: 'Alloc CPU Ratio', icon: '\u{1F525}' },
        { path: '/api/operations/evt-type-dist-v2172', name: 'Evt Type Dist', icon: '\u{1F4CA}' },
        { path: '/api/security/cap-add-v2173', name: 'Cap Add', icon: '\u{26A0}' },
        { path: '/api/security/secret-age-v2173', name: 'Secret Age', icon: '\u{1F510}' },
        { path: '/api/security/rb-sa-validator-v2173', name: 'RB SA Valid', icon: '\u{1F465}' },
        { path: '/api/docs/kubelet-ver-dist-v2174', name: 'Kubelet Ver Dist', icon: '\u{1F4BB}' },
        { path: '/api/docs/ns-rest-pol-v2174', name: 'NS Rest Pol', icon: '\u{1F501}' },
        { path: '/api/docs/pvc-vol-mode-v2174', name: 'PVC Vol Mode', icon: '\u{1F4BF}' },
        { path: '/api/scalability/mem-waste-v2175', name: 'Mem Waste', icon: '\u{1F4BE}' },
        { path: '/api/scalability/cpu-pinning-v2175', name: 'CPU Pinning', icon: '\u{1F525}' },
        { path: '/api/scalability/ns-storage-forecast-v2175', name: 'NS Storage Forecast', icon: '\u{1F4BF}' },
        { path: '/api/product/working-set-v2176', name: 'Working Set', icon: '\u{1F4CF}' },
        { path: '/api/product/sel-target-v2176', name: 'Sel Target', icon: '\u{1F50C}' },
        { path: '/api/product/ns-active-deadline-v2176', name: 'NS Active Deadline', icon: '\u{23F1}' },
        { path: '/api/deployment/updated-replica-v2177', name: 'Updated Replica', icon: '\u{1F501}' },
        { path: '/api/deployment/sts-vol-claim-v2177', name: 'STS Vol Claim', icon: '\u{1F4BF}' },
        { path: '/api/deployment/rs-generation-v2177', name: 'RS Generation', icon: '\u{1F4DC}' },
        { path: '/api/operations/net-iface-v2178', name: 'Net Iface', icon: '\u{1F310}' },
        { path: '/api/operations/img-layer-v2178', name: 'Img Layer', icon: '\u{1F4BE}' },
        { path: '/api/operations/node-mem-pressure-v2178', name: 'Node Mem Pressure', icon: '\u{1F4BE}' },
        { path: '/api/security/runas-group-v2179', name: 'RunAs Group', icon: '\u{1F464}' },
        { path: '/api/security/secret-size-v2179', name: 'Secret Size', icon: '\u{1F510}' },
        { path: '/api/security/cr-resource-star-v2179', name: 'CR Resource Star', icon: '\u{2728}' },
        { path: '/api/docs/os-dist-v2180', name: 'OS Dist', icon: '\u{1F4BB}' },
        { path: '/api/docs/external-ip-v2180', name: 'External IP', icon: '\u{1F310}' },
        { path: '/api/docs/init-ctnr-count-v2180', name: 'Init Ctnr Count', icon: '\u{1F4E6}' },
        { path: '/api/scalability/cpu-frag-per-node-v2181', name: 'CPU Frag Per Node', icon: '\u{1F4CA}' },
        { path: '/api/scalability/ns-multi-replica-v2181', name: 'NS Multi Replica', icon: '\u{1F6E1}' },
        { path: '/api/scalability/pvc-storage-util-v2181', name: 'PVC Storage Util', icon: '\u{1F4BF}' },
        { path: '/api/product/dns-config-v2182', name: 'DNS Config', icon: '\u{1F310}' },
        { path: '/api/product/sess-affinity-v2182', name: 'Sess Affinity', icon: '\u{1F517}' },
        { path: '/api/product/env-src-v2182', name: 'Env Src', icon: '\u{1F4DD}' },
        { path: '/api/deployment/dep-collision-v2183', name: 'Dep Collision', icon: '\u{26A0}' },
        { path: '/api/deployment/sts-ready-gap-v2183', name: 'STS Ready Gap', icon: '\u{2705}' },
        { path: '/api/deployment/ds-node-cov-v2183', name: 'DS Node Cov', icon: '\u{1F5FA}' },
        { path: '/api/operations/wait-state-v2184', name: 'Wait State', icon: '\u{23F3}' },
        { path: '/api/operations/kernel-ver-v2184', name: 'Kernel Ver', icon: '\u{1F4BB}' },
        { path: '/api/operations/clusterip-util-v2184', name: 'ClusterIP Util', icon: '\u{1F310}' },
        { path: '/api/security/fsgroup-policy-v2185', name: 'FSGroup Policy', icon: '\u{1F6E1}' },
        { path: '/api/security/psa-audit-warn-v2185', name: 'PSA Audit Warn', icon: '\u{26A0}' },
        { path: '/api/security/np-peer-ns-selector-v2185', name: 'NP Peer NS Sel', icon: '\u{1F310}' },
        { path: '/api/docs/boot-id-v2186', name: 'Boot ID', icon: '\u{1F4BB}' },
        { path: '/api/docs/pvc-node-v2186', name: 'PVC Node', icon: '\u{1F4BF}' },
        { path: '/api/docs/host-alias-v2186', name: 'Host Alias', icon: '\u{1F4CD}' },
        { path: '/api/scalability/cpu-lim-hr-v2187', name: 'CPU Lim HR', icon: '\u{1F525}' },
        { path: '/api/scalability/ns-dep-dist-v2187', name: 'NS Dep Dist', icon: '\u{1F4CA}' },
        { path: '/api/scalability/res-eff-score-v2187', name: 'Res Eff Score', icon: '\u{1F4CF}' },
        { path: '/api/product/gms-creds-v2188', name: 'GMS Creds', icon: '\u{1F510}' },
        { path: '/api/product/liveness-type-v2188', name: 'Liveness Type', icon: '\u{23F1}' },
        { path: '/api/product/ext-traffic-health-v2188', name: 'Ext Traffic Health', icon: '\u{1F310}' },
        { path: '/api/deployment/paused-status-v2189', name: 'Paused Status', icon: '\u{23F8}' },
        { path: '/api/deployment/sts-ready-avail-v2189', name: 'STS Ready Avail', icon: '\u{2705}' },
        { path: '/api/deployment/rs-owner-kind-v2189', name: 'RS Owner Kind', icon: '\u{1F517}' },
        { path: '/api/operations/burstable-v2190', name: 'Burstable', icon: '\u{26A0}' },
        { path: '/api/operations/cr-ver-v2190', name: 'CR Version', icon: '\u{1F4BB}' },
        { path: '/api/operations/evt-msg-freq-v2190', name: 'Evt Msg Freq', icon: '\u{1F4CA}' },
        { path: '/api/security/selinux-level-v2191', name: 'SELinux Level', icon: '\u{1F6E1}' },
        { path: '/api/security/sec-annot-v2191', name: 'Secret Annot', icon: '\u{1F510}' },
        { path: '/api/security/crb-roleref-v2191', name: 'CRB RoleRef', icon: '\u{1F465}' },
        { path: '/api/docs/node-os-arch-v2192', name: 'Node OS Arch', icon: '\u{1F4BB}' },
        { path: '/api/docs/cm-binary-v2192', name: 'CM Binary', icon: '\u{1F4E6}' },
        { path: '/api/docs/img-id-ref-v2192', name: 'Img ID Ref', icon: '\u{1F4BE}' },
        { path: '/api/scalability/mem-lim-hr-v2193', name: 'Mem Lim HR', icon: '\u{1F4BE}' },
        { path: '/api/scalability/ns-density-v2193', name: 'NS Density', icon: '\u{1F4CA}' },
        { path: '/api/scalability/bin-pack-eff-v2193', name: 'Bin Pack Eff', icon: '\u{1F4E6}' },
        { path: '/api/product/ready-gate-type-v2194', name: 'Ready Gate Type', icon: '\u{1F6E1}' },
        { path: '/api/product/stdin-once-dist-v2194', name: 'Stdin Once Dist', icon: '\u{2328}' },
        { path: '/api/product/ext-traffic-risk-v2194', name: 'Ext Traffic Risk', icon: '\u{26A0}' },
        { path: '/api/deployment/strategy-dist-v2195', name: 'Strategy Dist', icon: '\u{1F501}' },
        { path: '/api/deployment/sts-svc-binding-v2195', name: 'STS Svc Binding', icon: '\u{1F517}' },
        { path: '/api/deployment/ds-tol-cov-v2195', name: 'DS Tol Cov', icon: '\u{26D4}' },
        { path: '/api/operations/oom-risk-v2196', name: 'OOM Risk', icon: '\u{26A0}' },
        { path: '/api/operations/disk-pressure-v2196', name: 'Disk Pressure', icon: '\u{1F4BF}' },
        { path: '/api/operations/evt-ns-dist-v2196', name: 'Evt NS Dist', icon: '\u{1F4CA}' },
        { path: '/api/security/seccomp-localhost-v2197', name: 'Seccomp Local', icon: '\u{1F6E1}' },
        { path: '/api/security/sa-token-risk-v2197', name: 'SA Token Risk', icon: '\u{1F511}' },
        { path: '/api/security/np-egress-default-v2197', name: 'NP Egress Default', icon: '\u{1F310}' },
        { path: '/api/docs/cr-id-v2198', name: 'CR ID', icon: '\u{1F4BB}' },
        { path: '/api/docs/port-name-v2198', name: 'Port Name', icon: '\u{1F50C}' },
        { path: '/api/docs/pvc-datasource-v2198', name: 'PVC DataSource', icon: '\u{1F4BF}' },
        { path: '/api/scalability/pod-skew-v2199', name: 'Pod Skew', icon: '\u{1F4CA}' },
        { path: '/api/scalability/ns-replica-eff-v2199', name: 'NS Replica Eff', icon: '\u{2705}' },
        { path: '/api/scalability/pvc-cap-hr-v2199', name: 'PVC Cap HR', icon: '\u{1F4BF}' },
        { path: '/api/product/ip-fam-policy-v2200', name: 'IP Fam Policy', icon: '\u{1F310}' },
        { path: '/api/product/startup-probe-type-v2200', name: 'Startup Probe', icon: '\u{1F50C}' },
        { path: '/api/product/sess-timeout-v2200', name: 'Sess Timeout', icon: '\u{23F1}' },
        { path: '/api/deployment/rev-hist-audit-v2201', name: 'Rev Hist Audit', icon: '\u{1F4DC}' },
        { path: '/api/deployment/sts-pod-mgmt-dist-v2201', name: 'STS PodMgmt Dist', icon: '\u{2699}' },
        { path: '/api/deployment/ds-rev-status-v2201', name: 'DS Rev Status', icon: '\u{1F501}' },
        { path: '/api/operations/terminal-dist-v2202', name: 'Terminal Dist', icon: '\u{26A0}' },
        { path: '/api/operations/pid-pressure-v2202', name: 'PID Pressure', icon: '\u{26A0}' },
        { path: '/api/operations/eps-ready-ratio-v2202', name: 'EPS Ready Ratio', icon: '\u{2705}' },
        { path: '/api/security/priv-esc-default-v2203', name: 'Priv Esc Default', icon: '\u{26A0}' },
        { path: '/api/security/sec-ext-data-v2203', name: 'Sec Ext Data', icon: '\u{1F510}' },
        { path: '/api/security/rb-subject-kind-v2203', name: 'RB Subject Kind', icon: '\u{1F465}' },
        { path: '/api/docs/node-feature-label-v2204', name: 'Node Feature Label', icon: '\u{1F3F7}' },
        { path: '/api/docs/cm-immutable-cat-v2204', name: 'CM Immutable', icon: '\u{1F512}' },
        { path: '/api/docs/priority-dist-v2204', name: 'Priority Dist', icon: '\u{26A1}' },
        { path: '/api/scalability/cpu-commit-ratio-v2205', name: 'CPU Commit Ratio', icon: '\u{1F525}' },
        { path: '/api/scalability/ns-ha-mult-score-v2205', name: 'NS HA Mult Score', icon: '\u{1F6E1}' },
        { path: '/api/scalability/cluster-storage-eff-v2205', name: 'Cluster Storage Eff', icon: '\u{1F4BF}' },
        { path: '/api/product/img-digest-v2206', name: 'Img Digest', icon: '\u{1F4BE}' },
        { path: '/api/product/probe-success-v2206', name: 'Probe Success', icon: '\u{2705}' },
        { path: '/api/product/lb-source-range-v2206', name: 'LB SourceRange', icon: '\u{1F310}' },
        { path: '/api/deployment/gen-lag-v2207', name: 'Gen Lag', icon: '\u{23F1}' },
        { path: '/api/deployment/sts-rev-tracker-v2207', name: 'STS Rev Tracker', icon: '\u{1F501}' },
        { path: '/api/deployment/ds-avail-gap-v2207', name: 'DS Avail Gap', icon: '\u{26A0}' },
        { path: '/api/operations/np-match-v2208', name: 'NP Match', icon: '\u{1F310}' },
        { path: '/api/operations/mem-cap-alloc-v2208', name: 'Mem Cap Alloc', icon: '\u{1F4BE}' },
        { path: '/api/operations/last-term-reason-v2208', name: 'Last Term Reason', icon: '\u{26A0}' },
        { path: '/api/security/supp-groups-v2209', name: 'Supp Groups', icon: '\u{1F6E1}' },
        { path: '/api/security/default-sa-risk-v2209', name: 'Default SA Risk', icon: '\u{1F511}' },
        { path: '/api/security/crb-subj-ns-v2209', name: 'CRB Subj NS', icon: '\u{1F465}' },
        { path: '/api/docs/machine-id-v2210', name: 'Machine ID', icon: '\u{1F4BB}' },
        { path: '/api/docs/pvc-phase-dist-v2210', name: 'PVC Phase Dist', icon: '\u{1F4BF}' },
        { path: '/api/docs/restart-count-dist-v2210', name: 'Restart Count Dist', icon: '\u{1F501}' },
        { path: '/api/scalability/ns-cpu-commit-v2211', name: 'NS CPU Commit', icon: '\u{1F525}' },
        { path: '/api/scalability/sched-gap-v2211', name: 'Sched Gap', icon: '\u{1F4CA}' },
        { path: '/api/scalability/img-pull-eff-v2211', name: 'Img Pull Eff', icon: '\u{1F4BE}' },
        { path: '/api/product/subpath-v2212', name: 'SubPath', icon: '\u{1F4BF}' },
        { path: '/api/product/mem-req-dist-v2212', name: 'Mem Req Dist', icon: '\u{1F4BE}' },
        { path: '/api/product/int-traffic-local-v2212', name: 'Int Traffic Local', icon: '\u{1F310}' },
        { path: '/api/deployment/max-unavail-v2213', name: 'Max Unavail', icon: '\u{26A0}' },
        { path: '/api/deployment/sts-upd-strat-dist-v2213', name: 'STS Upd Dist', icon: '\u{1F501}' },
        { path: '/api/deployment/rs-ready-gap-v2213', name: 'RS Ready Gap', icon: '\u{2705}' },
        { path: '/api/operations/node-sel-key-v2214', name: 'Node Sel Key', icon: '\u{1F4CD}' },
        { path: '/api/operations/net-unavail-v2214', name: 'Net Unavail', icon: '\u{26A0}' },
        { path: '/api/operations/ctnr-running-ratio-v2214', name: 'Ctnr Running', icon: '\u{2705}' },
        { path: '/api/security/host-users-v2215', name: 'Host Users', icon: '\u{1F464}' },
        { path: '/api/security/sec-key-ref-v2215', name: 'Sec Key Ref', icon: '\u{1F510}' },
        { path: '/api/security/np-ingress-ipblock-v2215', name: 'NP IPBlock', icon: '\u{1F310}' },
        { path: '/api/docs/sys-uuid-v2216', name: 'Sys UUID', icon: '\u{1F4BB}' },
        { path: '/api/docs/cm-mount-path-v2216', name: 'CM Mount Path', icon: '\u{1F4C2}' },
        { path: '/api/docs/pod-annot-key-v2216', name: 'Pod Annot Key', icon: '\u{1F3F7}' },
        { path: '/api/scalability/ns-mem-commit-v2217', name: 'NS Mem Commit', icon: '\u{1F4BE}' },
        { path: '/api/scalability/pod-cap-hr-v2217', name: 'Pod Cap HR', icon: '\u{1F4CA}' },
        { path: '/api/scalability/dep-spread-v2217', name: 'Dep Spread', icon: '\u{1F310}' },
        { path: '/api/product/prestop-hook-v2218', name: 'PreStop Hook', icon: '\u{1F501}' },
        { path: '/api/product/img-size-est-v2218', name: 'Img Size Est', icon: '\u{1F4BE}' },
        { path: '/api/product/lb-health-port-v2218', name: 'LB Health Port', icon: '\u{2705}' },
        { path: '/api/deployment/max-surge-v2219', name: 'Max Surge', icon: '\u{1F4C8}' },
        { path: '/api/deployment/sts-ordinal-v2219', name: 'STS Ordinal', icon: '\u{1F522}' },
        { path: '/api/deployment/ds-tmpl-gen-v2219', name: 'DS Tmpl Gen', icon: '\u{1F501}' },
        { path: '/api/operations/supp-groups-dist-v2220', name: 'Supp Groups Dist', icon: '\u{1F6E1}' },
        { path: '/api/operations/alloc-pods-per-node-v2220', name: 'Alloc Pods/Node', icon: '\u{1F4CA}' },
        { path: '/api/operations/evt-reason-freq-v2220', name: 'Evt Reason Freq', icon: '\u{1F4CA}' },
        { path: '/api/security/host-ipc-v2221', name: 'Host IPC', icon: '\u{26A0}' },
        { path: '/api/security/psa-enforce-v2221', name: 'PSA Enforce', icon: '\u{1F6E1}' },
        { path: '/api/security/rb-agg-count-v2221', name: 'RB Agg Count', icon: '\u{1F465}' },
        { path: '/api/docs/kube-proxy-ver-v2222', name: 'KubeProxy Ver', icon: '\u{1F4BB}' },
        { path: '/api/docs/secret-vol-count-v2222', name: 'Secret Vol Count', icon: '\u{1F510}' },
        { path: '/api/docs/priority-value-v2222', name: 'Priority Value', icon: '\u{26A1}' },
        { path: '/api/scalability/ns-pvc-storage-v2223', name: 'NS PVC Storage', icon: '\u{1F4BF}' },
        { path: '/api/scalability/cpu-alloc-eff-v2223', name: 'CPU Alloc Eff', icon: '\u{1F525}' },
        { path: '/api/scalability/cluster-replicas-ratio-v2223', name: 'Cluster Replicas Ratio', icon: '\u{2705}' },
        { path: '/api/product/ctnr-cmd-v2224', name: 'Ctnr Cmd', icon: '\u{2328}' },
        { path: '/api/product/svc-ip-fam-v2224', name: 'Svc IP Family', icon: '\u{1F310}' },
        { path: '/api/product/os-domain-v2224', name: 'OS Domain', icon: '\u{1F517}' },
        { path: '/api/deployment/rev-limit-comp-v2225', name: 'Rev Limit Comp', icon: '\u{1F4DC}' },
        { path: '/api/deployment/sts-tmpl-hash-v2225', name: 'STS Tmpl Hash', icon: '\u{1F501}' },
        { path: '/api/deployment/rs-active-ratio-v2225', name: 'RS Active Ratio', icon: '\u{1F4CA}' },
        { path: '/api/operations/term-signal-v2226', name: 'Term Signal', icon: '\u{26A0}' },
        { path: '/api/operations/img-gc-v2226', name: 'Img GC', icon: '\u{1F4BE}' },
        { path: '/api/operations/port-proto-cov-v2226', name: 'Port Proto Cov', icon: '\u{1F50C}' },
        { path: '/api/security/proc-mount-v2227', name: 'ProcMount', icon: '\u{1F6E1}' },
        { path: '/api/security/docker-cfg-v2227', name: 'Docker Cfg', icon: '\u{1F433}' },
        { path: '/api/security/np-port-named-v2227', name: 'NP Port Named', icon: '\u{1F310}' },
        { path: '/api/docs/kernel-boot-v2228', name: 'Kernel Boot', icon: '\u{23F1}' },
        { path: '/api/docs/cm-ns-dist-v2228', name: 'CM NS Dist', icon: '\u{1F4C2}' },
        { path: '/api/docs/owner-ref-apiver-v2228', name: 'Owner Ref APIVer', icon: '\u{1F517}' },
        { path: '/api/scalability/ns-mem-eff-v2229', name: 'NS Mem Eff', icon: '\u{1F4BE}' },
        { path: '/api/scalability/node-storage-hr-v2229', name: 'Node Storage HR', icon: '\u{1F4BF}' },
        { path: '/api/scalability/img-cache-hit-v2229', name: 'Img Cache Hit', icon: '\u{1F4BE}' },
        { path: '/api/product/res-claim-v2230', name: 'Res Claim', icon: '\u{1F4CF}' },
        { path: '/api/product/working-dir-v2230', name: 'Working Dir', icon: '\u{1F4C2}' },
        { path: '/api/product/publish-notready-v2230', name: 'Publish NotReady', icon: '\u{26A0}' },
        { path: '/api/deployment/selector-complexity-v2231', name: 'Selector Complexity', icon: '\u{1F50D}' },
        { path: '/api/deployment/sts-pv-retain-v2231', name: 'STS PV Retain', icon: '\u{1F4BF}' },
        { path: '/api/deployment/rs-tmpl-lag-v2231', name: 'RS Tmpl Lag', icon: '\u{23F1}' },
        { path: '/api/operations/active-deadline-v2232', name: 'Active Deadline', icon: '\u{23F1}' },
        { path: '/api/operations/kubelet-net-v2232', name: 'Kubelet Net', icon: '\u{2705}' },
        { path: '/api/operations/img-pull-backoff-v2232', name: 'Img Pull Backoff', icon: '\u{26A0}' },
        { path: '/api/security/seccomp-default-cov-v2233', name: 'Seccomp Default Cov', icon: '\u{1F6E1}' },
        { path: '/api/security/sa-automount-disabled-v2233', name: 'SA Automount Dis', icon: '\u{1F511}' },
        { path: '/api/security/cr-empty-res-v2233', name: 'CR Empty Res', icon: '\u{26A0}' },
        { path: '/api/docs/cr-hash-v2234', name: 'CR Hash', icon: '\u{1F4BB}' },
        { path: '/api/docs/cm-env-from-v2234', name: 'CM EnvFrom', icon: '\u{1F4DD}' },
        { path: '/api/docs/sched-hint-v2234', name: 'Sched Hint', icon: '\u{1F9ED}' },
        { path: '/api/scalability/ns-cpu-limit-overcommit-v2235', name: 'NS CPU Overcommit', icon: '\u{1F525}' },
        { path: '/api/scalability/node-mem-commit-v2235', name: 'Node Mem Commit', icon: '\u{1F4BE}' },
        { path: '/api/scalability/dep-multi-zone-v2235', name: 'Dep Multi Zone', icon: '\u{1F310}' },
        { path: '/api/product/os-feature-gate-v2236', name: 'OS Feature Gate', icon: '\u{1F527}' },
        { path: '/api/product/lim-cpu-dist-v2236', name: 'Lim CPU Dist', icon: '\u{1F525}' },
        { path: '/api/product/clusterip-block-v2236', name: 'ClusterIP Block', icon: '\u{1F310}' },
        { path: '/api/deployment/spec-status-ratio-v2237', name: 'Spec Status Ratio', icon: '\u{2705}' },
        { path: '/api/deployment/sts-replicas-updated-v2237', name: 'STS Rep Updated', icon: '\u{1F501}' },
        { path: '/api/deployment/ds-misscheduled-v2237', name: 'DS Misscheduled', icon: '\u{26A0}' },
        { path: '/api/operations/restart-reason-v2238', name: 'Restart Reason', icon: '\u{1F501}' },
        { path: '/api/operations/mem-cap-alloc-ratio-v2238', name: 'Mem Cap Alloc Ratio', icon: '\u{1F4BE}' },
        { path: '/api/operations/evt-component-v2238', name: 'Evt Component', icon: '\u{1F4CA}' },
        { path: '/api/security/gmsa-cred-spec-v2239', name: 'GMSA Cred Spec', icon: '\u{1F510}' },
        { path: '/api/security/sec-sa-token-v2239', name: 'Sec SA Token', icon: '\u{1F511}' },
        { path: '/api/security/np-cidr-exc-v2239', name: 'NP CIDR Exc', icon: '\u{1F310}' },
        { path: '/api/docs/kubelet-proxy-match-v2240', name: 'Kubelet Proxy Match', icon: '\u{1F4BB}' },
        { path: '/api/docs/pvc-finalizer-v2240', name: 'PVC Finalizer', icon: '\u{1F4BF}' },
        { path: '/api/docs/del-grace-v2240', name: 'Del Grace', icon: '\u{23F3}' },
        { path: '/api/scalability/ns-eph-storage-v2241', name: 'NS Eph Storage', icon: '\u{1F4BF}' },
        { path: '/api/scalability/cpu-req-limit-spread-v2241', name: 'CPU Req/Lim Spread', icon: '\u{1F4CA}' },
        { path: '/api/scalability/aff-antiaff-count-v2241', name: 'Aff/AntiAff Count', icon: '\u{1F517}' },
        { path: '/api/product/overhead-dist-v2242', name: 'Overhead Dist', icon: '\u{1F4CF}' },
        { path: '/api/product/probe-timeout-v2242', name: 'Probe Timeout', icon: '\u{23F1}' },
        { path: '/api/product/lb-class-v2242', name: 'LB Class', icon: '\u{1F4A1}' },
        { path: '/api/deployment/replicas-vs-ready-v2243', name: 'Replicas vs Ready', icon: '\u{2705}' },
        { path: '/api/deployment/sts-update-rev-v2243', name: 'STS Update Rev', icon: '\u{1F501}' },
        { path: '/api/deployment/ds-unavail-v2243', name: 'DS Unavail', icon: '\u{26A0}' },
        { path: '/api/operations/exit-code-dist-v2244', name: 'Exit Code Dist', icon: '\u{1F4CA}' },
        { path: '/api/operations/node-cpu-cap-alloc-v2244', name: 'Node CPU Cap/Alloc', icon: '\u{1F525}' },
        { path: '/api/operations/evt-action-v2244', name: 'Evt Action', icon: '\u{1F4CA}' },
        { path: '/api/security/app-armor-v2245', name: 'AppArmor', icon: '\u{1F6E1}' },
        { path: '/api/security/secret-immutable-v2245', name: 'Secret Immutable', icon: '\u{1F512}' },
        { path: '/api/security/rb-roleref-kind-v2245', name: 'RB RoleRef Kind', icon: '\u{1F465}' },
        { path: '/api/docs/os-distribution-v2246', name: 'OS Dist', icon: '\u{1F4BB}' },
        { path: '/api/docs/cm-data-size-v2246', name: 'CM Data Size', icon: '\u{1F4DD}' },
        { path: '/api/docs/sa-pull-secret-v2246', name: 'SA Pull Secret', icon: '\u{1F511}' },
        { path: '/api/scalability/ns-res-eff-composite-v2247', name: 'NS Res Eff', icon: '\u{1F4CA}' },
        { path: '/api/scalability/cpu-vs-mem-spread-v2247', name: 'CPU vs Mem Spread', icon: '\u{1F4CA}' },
        { path: '/api/scalability/svc-endpoint-dist-v2247', name: 'Svc Endpoint Dist', icon: '\u{1F517}' },
        { path: '/api/product/topo-spread-catalog-v2248', name: 'Topo Spread Cat', icon: '\u{1F310}' },
        { path: '/api/product/stdin-tty-v2248', name: 'Stdin TTY', icon: '\u{2328}' },
        { path: '/api/product/alloc-ports-v2248', name: 'Alloc Ports', icon: '\u{1F50C}' },
        { path: '/api/deployment/dep-cond-status-v2249', name: 'Dep Cond Status', icon: '\u{2705}' },
        { path: '/api/deployment/sts-min-ready-v2249', name: 'STS MinReady', icon: '\u{23F1}' },
        { path: '/api/deployment/ds-deprecated-v2249', name: 'DS Deprecated', icon: '\u{26A0}' },
        { path: '/api/operations/restart-bucket-v2250', name: 'Restart Bucket', icon: '\u{1F4CA}' },
        { path: '/api/operations/node-mem-pages-v2250', name: 'Node Mem Pages', icon: '\u{1F4BE}' },
        { path: '/api/operations/ep-ready-ratio-v2250', name: 'EP Ready Ratio', icon: '\u{2705}' },
        { path: '/api/security/readonly-fs-v2251', name: 'ReadOnly FS', icon: '\u{1F6E1}' },
        { path: '/api/security/sa-token-age-v2251', name: 'SA Token Age', icon: '\u{1F511}' },
        { path: '/api/security/cr-wildcard-verb-v2251', name: 'CR Wildcard Verb', icon: '\u{26A0}' },
        { path: '/api/docs/heartbeat-catalog-v2252', name: 'Heartbeat Catalog', icon: '\u{23F1}' },
        { path: '/api/docs/pvc-sc-size-v2252', name: 'PVC SC Size', icon: '\u{1F4BF}' },
        { path: '/api/docs/vol-device-mount-v2252', name: 'Vol Device Mount', icon: '\u{1F4BF}' },
        { path: '/api/scalability/ns-pod-density-v2253', name: 'NS Pod Density', icon: '\u{1F4CA}' },
        { path: '/api/scalability/cpu-lim-commit-v2253', name: 'CPU Lim Commit', icon: '\u{1F525}' },
        { path: '/api/scalability/pvc-bound-pending-v2253', name: 'PVC Bound/Pending', icon: '\u{1F4BF}' },
        // v22.54 Product
        { path: '/api/product/hostname-fqdn-v2254', name: 'Hostname FQDN', icon: '\u{1F5A5}' },
        { path: '/api/product/env-var-count-v2254', name: 'Env Var Count', icon: '\u{1F4DD}' },
        { path: '/api/product/svc-ipfamily-v2254', name: 'Svc IP Family', icon: '\u{1F310}' },
        // v22.55 Deployment
        { path: '/api/deployment/deploy-available-cond-v2255', name: 'Deploy Avail Cond', icon: '\u2705' },
        { path: '/api/deployment/sts-replicas-status-v2255', name: 'STS Rep Status', icon: '\u{1F504}' },
        { path: '/api/deployment/rs-full-status-v2255', name: 'RS Full Status', icon: '\u{1F4E6}' },
        // v22.56 Operations
        { path: '/api/ops/ready-container-ratio-v2256', name: 'Ready Ctnr Ratio', icon: '\u{1F4CA}' },
        { path: '/api/ops/node-kube-version-v2256', name: 'Node Kube Ver', icon: '\u{1F527}' },
        { path: '/api/ops/event-type-distribution-v2256', name: 'Event Type Dist', icon: '\u{1F4CB}' },
        // v22.57 Security
        { path: '/api/security/cap-add-audit-v2257', name: 'CapAdd Audit', icon: '\u{1F6E1}' },
        { path: '/api/security/secret-ns-distribution-v2257', name: 'Secret NS Dist', icon: '\u{1F510}' },
        { path: '/api/security/rbac-role-count-v2257', name: 'RBAC Role Count', icon: '\u{1F451}' },
        // v22.58 Documentation
        { path: '/api/docs/node-condition-summary-v2258', name: 'Node Cond Summary', icon: '\u{1F4D}' },
        { path: '/api/docs/pvc-volume-name-catalog-v2258', name: 'PVC Vol Name', icon: '\u{1F4BF}' },
        { path: '/api/docs/pod-image-count-v2258', name: 'Pod Image Count', icon: '\u{1F3E0}' },
        // v22.59 Scalability
        { path: '/api/scalability/ns-cpu-request-v2259', name: 'NS CPU Request', icon: '\u26A1' },
        { path: '/api/scalability/node-mem-fragmentation-v2259', name: 'Node Mem Frag', icon: '\u{1F4A8}' },
        { path: '/api/scalability/cluster-service-health-v2259', name: 'Svc Health', icon: '\u{1F493}' },
        // v22.60 Product
        { path: '/api/product/container-port-catalog-v2260', name: 'Port Catalog', icon: '\u{1F50C}' },
        { path: '/api/product/pod-qos-distribution-v2260', name: 'Pod QoS Dist', icon: '\u{1F4CA}' },
        { path: '/api/product/resource-limit-adherence-v2260', name: 'Res Limit Adh', icon: '\u{1F4CF}' },
        // v22.61 Deployment
        { path: '/api/deployment/deploy-strategy-census-v2261', name: 'Deploy Strategy', icon: '\u{1F4E6}' },
        { path: '/api/deployment/sts-update-strategy-v2261', name: 'STS Update Strat', icon: '\u{1F504}' },
        { path: '/api/deployment/ds-update-strategy-v2261', name: 'DS Update Strat', icon: '\u{1F680}' },
        // v22.62 Operations
        { path: '/api/ops/restart-policy-distribution-v2262', name: 'Restart Policy', icon: '\u{1F501}' },
        { path: '/api/ops/node-architecture-census-v2262', name: 'Node Arch', icon: '\u{1F5A5}' },
        { path: '/api/ops/privileged-escalation-v2262', name: 'Privileged Esc', icon: '\u{1F6E1}' },
        // v22.63 Security
        { path: '/api/security/service-account-audit-v2263', name: 'SvcAccount Audit', icon: '\u{1F451}' },
        { path: '/api/security/secret-type-distribution-v2263', name: 'Secret Type Dist', icon: '\u{1F510}' },
        { path: '/api/security/pod-security-violations-v2263', name: 'Pod Sec Violation', icon: '\u{1F6E1}' },
        // v22.64 Documentation
        { path: '/api/docs/tolerations-catalog-v2264', name: 'Tolerations Catalog', icon: '\u{1F6A7}' },
        { path: '/api/docs/node-os-image-census-v2264', name: 'Node OS Image', icon: '\u{1F4BB}' },
        { path: '/api/docs/pv-reclaim-policy-inventory-v2264', name: 'PV Reclaim Policy', icon: '\u{1F4BF}' },
        // v22.65 Scalability
        { path: '/api/scalability/ns-memory-request-v2265', name: 'NS Mem Request', icon: '\u{1F4BE}' },
        { path: '/api/scalability/pod-density-per-node-v2265', name: 'Pod Density/Node', icon: '\u{1F3D7}' },
        { path: '/api/scalability/cluster-endpoint-count-v2265', name: 'Endpoint Count', icon: '\u{1F310}' },
        // v22.66 Product
        { path: '/api/product/pod-priority-distribution-v2266', name: 'Pod Priority', icon: '\u26A1' },
        { path: '/api/product/probe-coverage-v2266', name: 'Probe Coverage', icon: '\u{1F50D}' },
        { path: '/api/product/image-pull-policy-census-v2266', name: 'Pull Policy', icon: '\u{1F4E5}' },
        // v22.67 Deployment
        { path: '/api/deployment/hpa-target-utilization-v2267', name: 'HPA Target Util', icon: '\u{1F4C8}' },
        { path: '/api/deployment/pdb-min-available-v2267', name: 'PDB Min Avail', icon: '\u{1F6E1}' },
        { path: '/api/deployment/job-completion-status-v2267', name: 'Job Completion', icon: '\u2705' },
        // v22.68 Operations
        { path: '/api/ops/pod-phase-distribution-v2268', name: 'Pod Phase Dist', icon: '\u{1F4CA}' },
        { path: '/api/ops/node-container-runtime-v2268', name: 'Node Runtime', icon: '\u{1F527}' },
        { path: '/api/ops/container-state-census-v2268', name: 'Ctnr State', icon: '\u{1F4E6}' },
        // v22.69 Security
        { path: '/api/security/run-as-non-root-v2269', name: 'NonRoot Audit', icon: '\u{1F6E1}' },
        { path: '/api/security/sa-token-auto-mount-v2269', name: 'SA Token Mount', icon: '\u{1F511}' },
        { path: '/api/security/hostpath-mount-audit-v2269', name: 'HostPath Audit', icon: '\u{1F4BD}' },
        // v22.70 Documentation
        { path: '/api/docs/affinity-rules-catalog-v2270', name: 'Affinity Rules', icon: '\u{1F9ED}' },
        { path: '/api/docs/ns-label-inventory-v2270', name: 'NS Label Inv', icon: '\u{1F3F7}' },
        { path: '/api/docs/service-type-distribution-v2270', name: 'Svc Type Dist', icon: '\u{1F310}' },
        // v22.71 Scalability
        { path: '/api/scalability/node-allocatable-vs-capacity-v2271', name: 'Node Alloc vs Cap', icon: '\u{1F4CF}' },
        { path: '/api/scalability/storageclass-usage-v2271', name: 'SC Usage', icon: '\u{1F4BF}' },
        { path: '/api/scalability/pvc-size-quartile-v2271', name: 'PVC Size Quartile', icon: '\u{1F4D0}' },
        // v22.72 Product
        { path: '/api/product/volume-mount-count-v2272', name: 'Vol Mount Count', icon: '\u{1F4BF}' },
        { path: '/api/product/dns-policy-census-v2272', name: 'DNS Policy', icon: '\u{1F310}' },
        { path: '/api/product/init-container-audit-v2272', name: 'Init Ctnr Audit', icon: '\u{1F680}' },
        // v22.73 Deployment
        { path: '/api/deployment/cronjob-schedule-catalog-v2273', name: 'CronJob Catalog', icon: '\u{23F0}' },
        { path: '/api/deployment/revision-history-v2273', name: 'Revision History', icon: '\u{1F4DC}' },
        { path: '/api/deployment/sts-ordinal-status-v2273', name: 'STS Ordinal', icon: '\u{1F522}' },
        // v22.74 Operations
        { path: '/api/ops/node-kernel-version-v2274', name: 'Node Kernel', icon: '\u{1F527}' },
        { path: '/api/ops/termination-grace-period-v2274', name: 'Grace Period', icon: '\u{23F1}' },
        { path: '/api/ops/event-reason-top-v2274', name: 'Event Reason Top', icon: '\u{1F4CB}' },
        // v22.75 Security
        { path: '/api/security/readonly-rootfs-audit-v2275', name: 'ReadOnly RootFS', icon: '\u{1F512}' },
        { path: '/api/security/privilege-escalation-audit-v2275', name: 'PrivEsc Audit', icon: '\u{1F6E1}' },
        { path: '/api/security/runas-user-distribution-v2275', name: 'RunAsUser UID', icon: '\u{1F464}' },
        // v22.76 Documentation
        { path: '/api/docs/resourcequota-catalog-v2276', name: 'ResourceQuota', icon: '\u{1F4CA}' },
        { path: '/api/docs/topology-spread-constraints-v2276', name: 'Topo Spread', icon: '\u{1F9ED}' },
        { path: '/api/docs/node-taints-inventory-v2276', name: 'Node Taints', icon: '\u{1F6A7}' },
        // v22.77 Scalability
        { path: '/api/scalability/cluster-cpu-utilization-ratio-v2277', name: 'CPU Util Ratio', icon: '\u26A1' },
        { path: '/api/scalability/cluster-memory-utilization-ratio-v2277', name: 'Mem Util Ratio', icon: '\u{1F4BE}' },
        { path: '/api/scalability/node-pod-capacity-usage-v2277', name: 'Pod Cap Usage', icon: '\u{1F3E0}' },
        // v22.78 Product
        { path: '/api/product/nil-secctx-rate-v2278', name: 'Nil SecCtx Rate', icon: '\u{1F6E1}' },
        { path: '/api/product/netpol-direction-v2278', name: 'NetPol Direction', icon: '\u{1F310}' },
        { path: '/api/product/externalname-svc-catalog-v2278', name: 'ExtName Svc', icon: '\u{1F517}' },
        // v22.79 Deployment
        { path: '/api/deployment/hpa-scaling-audit-v2279', name: 'HPA Scaling', icon: '\u{1F4C8}' },
        { path: '/api/deployment/max-surge-max-unavailable-v2279', name: 'MaxSurge', icon: '\u{1F4C9}' },
        { path: '/api/deployment/sts-pvc-template-v2279', name: 'STS PVC Tmpl', icon: '\u{1F4BF}' },
        // v22.80 Operations
        { path: '/api/ops/oom-risk-detection-v2280', name: 'OOM Risk', icon: '\u{1F6A8}' },
        { path: '/api/ops/node-pid-pressure-v2280', name: 'Node PID Pressure', icon: '\u{1F534}' },
        { path: '/api/ops/last-termination-reason-v2280', name: 'Last Term Reason', icon: '\u{1F480}' },
        // v22.81 Security
        { path: '/api/security/seccomp-profile-audit-v2281', name: 'Seccomp Audit', icon: '\u{1F6E1}' },
        { path: '/api/security/netpol-default-deny-v2281', name: 'Default Deny', icon: '\u{1F512}' },
        { path: '/api/security/clusterrole-wildcard-v2281', name: 'CR Wildcard', icon: '\u{1F451}' },
        // v22.82 Documentation
        { path: '/api/docs/configmap-age-distribution-v2282', name: 'CM Age Dist', icon: '\u{1F4C5}' },
        { path: '/api/docs/namespace-phase-inventory-v2282', name: 'NS Phase', icon: '\u{1F4CD}' },
        { path: '/api/docs/pvc-access-mode-catalog-v2282', name: 'PVC Access Mode', icon: '\u{1F511}' },
        // v22.83 Scalability
        { path: '/api/scalability/top-namespace-by-pod-v2283', name: 'Top NS by Pod', icon: '\u{1F4CA}' },
        { path: '/api/scalability/node-cpu-oversubscription-v2283', name: 'CPU Oversub', icon: '\u26A1' },
        { path: '/api/scalability/storage-by-namespace-v2283', name: 'Storage by NS', icon: '\u{1F4E6}' },
        // v22.84 Product
        { path: '/api/product/service-port-mapping-v2284', name: 'Svc Port Map', icon: '\u{1F50C}' },
        { path: '/api/product/pod-subdomain-dns-v2284', name: 'Subdomain DNS', icon: '\u{1F310}' },
        { path: '/api/product/container-workdir-audit-v2284', name: 'WorkDir Audit', icon: '\u{1F4C1}' },
        // v22.85 Deployment
        { path: '/api/deployment/ds-nodeselector-census-v2285', name: 'DS NodeSelector', icon: '\u{1F3ED}' },
        { path: '/api/deployment/deployment-paused-status-v2285', name: 'Deploy Paused', icon: '\u{23F8}' },
        { path: '/api/deployment/sts-service-name-link-v2285', name: 'STS Svc Link', icon: '\u{1F517}' },
        // v22.86 Operations
        { path: '/api/ops/crashloop-detection-v2286', name: 'CrashLoop', icon: '\u{1F6A8}' },
        { path: '/api/ops/node-disk-pressure-v2286', name: 'Disk Pressure', icon: '\u{1F4BF}' },
        { path: '/api/ops/restart-distribution-v2286', name: 'Restart Dist', icon: '\u{1F501}' },
        // v22.87 Security
        { path: '/api/security/secret-data-size-audit-v2287', name: 'Secret Size', icon: '\u{1F510}' },
        { path: '/api/security/pod-fsgroup-audit-v2287', name: 'FSGroup Audit', icon: '\u{1F465}' },
        { path: '/api/security/role-binding-count-v2287', name: 'Role Binding Count', icon: '\u{1F451}' },
        // v22.88 Documentation
        { path: '/api/docs/endpoint-subset-catalog-v2288', name: 'EP Subset', icon: '\u{1F310}' },
        { path: '/api/docs/node-ip-range-catalog-v2288', name: 'Node IP Range', icon: '\u{1F5A5}' },
        { path: '/api/docs/service-session-affinity-v2288', name: 'Session Affinity', icon: '\u{1F91D}' },
        // v22.89 Scalability
        { path: '/api/scalability/top-image-by-replica-v2289', name: 'Top Image', icon: '\u{1F4E6}' },
        { path: '/api/scalability/node-memory-oversubscription-v2289', name: 'Mem Oversub', icon: '\u{1F4BE}' },
        { path: '/api/scalability/pod-age-distribution-v2289', name: 'Pod Age Dist', icon: '\u{1F4C5}' },
        // v22.90 Product
        { path: '/api/product/pod-preemption-history-v2290', name: 'Preemption', icon: '\u{26A1}' },
        { path: '/api/product/stdin-tty-audit-v2290', name: 'Stdin/TTY', icon: '\u{2328}' },
        { path: '/api/product/loadbalancer-health-v2290', name: 'LB Health', icon: '\u{1F310}' },
        // v22.91 Deployment
        { path: '/api/deployment/deployment-progress-audit-v2291', name: 'Deploy Progress', icon: '\u{1F4C8}' },
        { path: '/api/deployment/rs-owner-reference-v2291', name: 'RS Owner Ref', icon: '\u{1F517}' },
        { path: '/api/deployment/job-active-deadline-v2291', name: 'Job Deadline', icon: '\u{23F1}' },
        // v22.92 Operations
        { path: '/api/ops/node-memory-pressure-v2292', name: 'Node Mem Pressure', icon: '\u{1F4BE}' },
        { path: '/api/ops/restart-top-containers-v2292', name: 'Restart Top', icon: '\u{1F501}' },
        { path: '/api/ops/image-pull-duration-risk-v2292', name: 'Pull Duration Risk', icon: '\u{1F4E5}' },
        // v22.93 Security
        { path: '/api/security/cap-drop-audit-v2293', name: 'CapDrop Audit', icon: '\u{1F6E1}' },
        { path: '/api/security/image-registry-trust-v2293', name: 'Registry Trust', icon: '\u{1F510}' },
        { path: '/api/security/secret-env-var-exposure-v2293', name: 'Secret Env Var', icon: '\u{1F451}' },
        // v22.94 Documentation
        { path: '/api/docs/volume-type-census-v2294', name: 'Volume Types', icon: '\u{1F4BF}' },
        { path: '/api/docs/nodeselector-key-inventory-v2294', name: 'NodeSelector Keys', icon: '\u{1F50D}' },
        { path: '/api/docs/namespace-finalizer-catalog-v2294', name: 'NS Finalizer', icon: '\u{1F527}' },
        // v22.95 Scalability
        { path: '/api/scalability/resource-waste-detection-v2295', name: 'Res Waste', icon: '\u{1F9F9}' },
        { path: '/api/scalability/pod-spread-balance-v2295', name: 'Pod Spread Balance', icon: '\u{1F3D7}' },
        { path: '/api/scalability/workload-concentration-v2295', name: 'Workload Conc', icon: '\u{1F4CA}' },
        // v22.96 Product
        { path: '/api/product/pod-completion-index-v2296', name: 'Pod Completion', icon: '\u2705' },
        { path: '/api/product/container-args-catalog-v2296', name: 'Args Catalog', icon: '\u{2328}' },
        { path: '/api/product/sa-pull-secret-audit-v2296', name: 'SA PullSecret', icon: '\u{1F511}' },
        // v22.97 Deployment
        { path: '/api/deployment/ds-desired-vs-ready-v2297', name: 'DS Desired/Ready', icon: '\u{1F680}' },
        { path: '/api/deployment/rollout-condition-v2297', name: 'Rollout Cond', icon: '\u{1F4C8}' },
        { path: '/api/deployment/cronjob-last-schedule-v2297', name: 'CJ Last Sched', icon: '\u{23F0}' },
        // v22.98 Operations
        { path: '/api/ops/node-network-unavailable-v2298', name: 'Node Net Unavail', icon: '\u{1F6A6}' },
        { path: '/api/ops/pod-ready-transition-v2298', name: 'Pod Ready Trans', icon: '\u{1F504}' },
        { path: '/api/ops/event-involved-object-v2298', name: 'Event InvObj', icon: '\u{1F4CB}' },
        // v22.99 Security
        { path: '/api/security/seccomp-type-audit-v2299', name: 'Seccomp Type', icon: '\u{1F6E1}' },
        { path: '/api/security/binding-subject-census-v2299', name: 'Binding Subject', icon: '\u{1F451}' },
        { path: '/api/security/netpol-port-catalog-v2299', name: 'NetPol Ports', icon: '\u{1F50C}' },
        // v23.00 Documentation
        { path: '/api/docs/service-clusterip-catalog-v2300', name: 'ClusterIP Catalog', icon: '\u{1F310}' },
        { path: '/api/docs/pod-node-distribution-v2300', name: 'Pod Node Dist', icon: '\u{1F5FA}' },
        { path: '/api/docs/configmap-key-count-v2300', name: 'CM Key Count', icon: '\u{1F4DD}' },
        // v23.01 Scalability
        { path: '/api/scalability/cluster-efficiency-score-v2301', name: 'Cluster Eff', icon: '\u{1F4CA}' },
        { path: '/api/scalability/namespace-resource-density-v2301', name: 'NS Density', icon: '\u{1F3D7}' },
        { path: '/api/scalability/node-cpu-commit-ratio-v2301', name: 'Node CPU Commit', icon: '\u26A1' },
        // v23.02 Product
        { path: '/api/product/pod-overhead-audit-v2302', name: 'Pod Overhead', icon: '\u{1F4CF}' },
        { path: '/api/product/lifecycle-hook-coverage-v2302', name: 'Lifecycle Hooks', icon: '\u{1F501}' },
        { path: '/api/product/external-traffic-policy-v2302', name: 'Ext Traffic Pol', icon: '\u{1F310}' },
        // v23.03 Deployment
        { path: '/api/deployment/sts-status-replicas-v2303', name: 'STS Status', icon: '\u{1F4C8}' },
        { path: '/api/deployment/ds-scheduled-vs-misscheduled-v2303', name: 'DS Scheduled', icon: '\u{1F680}' },
        { path: '/api/deployment/job-parallelism-config-v2303', name: 'Job Parallelism', icon: '\u{1F5C3}' },
        // v23.04 Operations
        { path: '/api/ops/scheduling-gate-audit-v2304', name: 'Sched Gate', icon: '\u{1F6A7}' },
        { path: '/api/ops/node-kubeproxy-version-v2304', name: 'KubeProxy Ver', icon: '\u{1F527}' },
        { path: '/api/ops/container-started-state-v2304', name: 'Started State', icon: '\u25B6' },
        // v23.05 Security
        { path: '/api/security/sa-age-audit-v2305', name: 'SA Age', icon: '\u{1F4C5}' },
        { path: '/api/security/fsgroup-change-policy-v2305', name: 'FSGroup ChgPol', icon: '\u{1F465}' },
        { path: '/api/security/clusterrole-aggregation-v2305', name: 'CR Aggregation', icon: '\u{1F451}' },
        // v23.06 Documentation
        { path: '/api/docs/pv-phase-inventory-v2306', name: 'PV Phase', icon: '\u{1F4BF}' },
        { path: '/api/docs/pod-resource-claim-catalog-v2306', name: 'Res Claims', icon: '\u{1F4DD}' },
        { path: '/api/docs/node-podcidr-catalog-v2306', name: 'Node PodCIDR', icon: '\u{1F5FA}' },
        // v23.07 Scalability
        { path: '/api/scalability/ns-cpu-limit-request-ratio-v2307', name: 'NS CPU Ratio', icon: '\u26A1' },
        { path: '/api/scalability/node-storage-commit-v2307', name: 'Node Storage', icon: '\u{1F4E6}' },
        { path: '/api/scalability/cluster-pod-churn-rate-v2307', name: 'Pod Churn', icon: '\u{1F504}' },
        // v23.08 Product
        { path: '/api/product/pod-gmsa-audit-v2308', name: 'Pod GMSA', icon: '\u{1F510}' },
        { path: '/api/product/startup-probe-type-v2308', name: 'Startup Probe', icon: '\u{1F680}' },
        { path: '/api/product/internal-traffic-policy-v2308', name: 'Int Traffic Pol', icon: '\u{1F310}' },
        // v23.09 Deployment
        { path: '/api/deployment/observed-generation-v2309', name: 'Observed Gen', icon: '\u{1F501}' },
        { path: '/api/deployment/rs-template-hash-v2309', name: 'RS Template Hash', icon: '\u{1F3F7}' },
        { path: '/api/deployment/cronjob-concurrency-v2309', name: 'CJ Concurrency', icon: '\u{23F0}' },
        // v23.10 Operations
        { path: '/api/ops/ephemeral-container-count-v2310', name: 'Ephemeral Ctnr', icon: '\u{1F50D}' },
        { path: '/api/ops/node-unschedulable-v2310', name: 'Node Unsched', icon: '\u{1F6A6}' },
        { path: '/api/ops/event-source-component-v2310', name: 'Event Source', icon: '\u{1F4CB}' },
        // v23.11 Security
        { path: '/api/security/selinux-audit-v2311', name: 'SELinux Audit', icon: '\u{1F6E1}' },
        { path: '/api/security/configmap-binarydata-v2311', name: 'CM BinaryData', icon: '\u{1F4DD}' },
        { path: '/api/security/sa-secret-ref-v2311', name: 'SA Secret Ref', icon: '\u{1F511}' },
        // v23.12 Documentation
        { path: '/api/docs/service-port-name-catalog-v2312', name: 'Svc Port Name', icon: '\u{1F50C}' },
        { path: '/api/docs/pod-hostalias-inventory-v2312', name: 'Pod HostAlias', icon: '\u{1F310}' },
        { path: '/api/docs/node-bootid-census-v2312', name: 'Node BootID', icon: '\u{1F4BB}' },
        // v23.13 Scalability
        { path: '/api/scalability/ns-limit-request-balance-v2313', name: 'NS Limit/Req', icon: '\u2696' },
        { path: '/api/scalability/node-ephemeral-storage-v2313', name: 'Node Ephemeral', icon: '\u{1F4BE}' },
        { path: '/api/scalability/cluster-replica-total-v2313', name: 'Replica Total', icon: '\u{1F4CA}' },
        // v23.14 Product
        { path: '/api/product/pod-os-audit-v2314', name: 'Pod OS', icon: '\u{1F4BB}' },
        { path: '/api/product/resource-resize-policy-v2314', name: 'Resize Policy', icon: '\u{1F4CF}' },
        { path: '/api/product/publish-notready-v2314', name: 'PubNotReady', icon: '\u{1F4E1}' },
        // v23.15 Deployment
        { path: '/api/deployment/deployment-available-ratio-v2315', name: 'Deploy Avail Ratio', icon: '\u{1F4C8}' },
        { path: '/api/deployment/sts-generation-sync-v2315', name: 'STS Gen Sync', icon: '\u{1F501}' },
        { path: '/api/deployment/ds-number-available-v2315', name: 'DS Num Avail', icon: '\u{1F680}' },
        // v23.16 Operations
        { path: '/api/ops/image-pull-backoff-v2316', name: 'ImgPullBackOff', icon: '\u{1F6A8}' },
        { path: '/api/ops/node-ready-transition-v2316', name: 'Node Ready', icon: '\u2705' },
        { path: '/api/ops/event-warning-rate-v2316', name: 'Event Warn Rate', icon: '\u{26A0}' },
        // v23.17 Security
        { path: '/api/security/proc-mount-audit-v2317', name: 'ProcMount Audit', icon: '\u{1F6E1}' },
        { path: '/api/security/pv-security-context-v2317', name: 'PV SecCtx', icon: '\u{1F512}' },
        { path: '/api/security/namespace-deletion-guard-v2317', name: 'NS Del Guard', icon: '\u{1F6AB}' },
        // v23.18 Documentation
        { path: '/api/docs/secret-age-distribution-v2318', name: 'Secret Age', icon: '\u{1F4C5}' },
        { path: '/api/docs/pvc-finalizer-catalog-v2318', name: 'PVC Finalizer', icon: '\u{1F527}' },
        { path: '/api/docs/node-machineid-census-v2318', name: 'Node MachineID', icon: '\u{1F5A5}' },
        // v23.19 Scalability
        { path: '/api/scalability/top-namespace-cpu-request-v2319', name: 'Top NS CPU', icon: '\u26A1' },
        { path: '/api/scalability/node-pod-allocation-balance-v2319', name: 'Node Pod Balance', icon: '\u{1F3D7}' },
        { path: '/api/scalability/service-endpoint-density-v2319', name: 'Svc EP Density', icon: '\u{1F310}' },
        // v23.20 Product
        { path: '/api/product/readiness-gate-audit-v2320', name: 'Readiness Gate', icon: '\u{1F6A6}' },
        { path: '/api/product/topo-spread-constraint-audit-v2320', name: 'Topo Spread Audit', icon: '\u{1F9ED}' },
        { path: '/api/product/ipfamily-policy-v2320', name: 'IP Family Pol', icon: '\u{1F310}' },
        // v23.21 Deployment
        { path: '/api/deployment/deploy-collision-check-v2321', name: 'Deploy Collision', icon: '\u{26A0}' },
        { path: '/api/deployment/sts-collision-check-v2321', name: 'STS Collision', icon: '\u{26A0}' },
        { path: '/api/deployment/rs-replica-status-v2321', name: 'RS Replica Status', icon: '\u{1F504}' },
        // v23.22 Operations
        { path: '/api/ops/pending-duration-risk-v2322', name: 'Pending Risk', icon: '\u{23F1}' },
        { path: '/api/ops/cpu-throttling-risk-v2322', name: 'CPU Throttle', icon: '\u{1F525}' },
        { path: '/api/ops/exit-code-distribution-v2322', name: 'Exit Code', icon: '\u{1F480}' },
        // v23.23 Security
        { path: '/api/security/apparmor-audit-v2323', name: 'AppArmor', icon: '\u{1F6E1}' },
        { path: '/api/security/configmap-immutable-v2323', name: 'CM Immutable', icon: '\u{1F512}' },
        { path: '/api/security/secret-rotation-risk-v2323', name: 'Secret Rotation', icon: '\u{1F501}' },
        // v23.24 Documentation
        { path: '/api/docs/sa-token-age-v2324', name: 'SA Token Age', icon: '\u{1F4C5}' },
        { path: '/api/docs/node-feature-label-v2324', name: 'Node Feature', icon: '\u{1F527}' },
        { path: '/api/docs/pod-annotation-count-v2324', name: 'Pod Annot Count', icon: '\u{1F4DD}' },
        // v23.25 Scalability
        { path: '/api/scalability/top-namespace-memory-request-v2325', name: 'Top NS Mem', icon: '\u{1F4BE}' },
        { path: '/api/scalability/node-container-density-v2325', name: 'Node Ctnr Density', icon: '\u{1F3E0}' },
        { path: '/api/scalability/cluster-configmap-total-v2325', name: 'CM Total', icon: '\u{1F4DA}' },

        // v23.26 Product
        { path: '/api/product/runtime-class-audit-v2326', name: 'RuntimeClass', icon: '\u{1F3E0}' },
        { path: '/api/product/stdin-once-audit-v2326', name: 'StdinOnce', icon: '\u{2328}' },
        { path: '/api/product/service-alloc-cidr-v2326', name: 'Alloc CIDR', icon: '\u{1F310}' },
        // v23.27 Deployment
        { path: '/api/deployment/sts-current-revision-v2327', name: 'STS Revision', icon: '\u{1F4C8}' },
        { path: '/api/deployment/ds-updated-number-v2327', name: 'DS Updated', icon: '\u{1F680}' },
        { path: '/api/deployment/job-failing-rate-v2327', name: 'Job Fail Rate', icon: '\u{1F6A8}' },
        // v23.28 Operations
        { path: '/api/ops/burstable-qos-audit-v2328', name: 'Burstable QoS', icon: '\u{1F4CA}' },
        { path: '/api/ops/node-memory-frag-v2328', name: 'Node Mem Frag', icon: '\u{1F4A8}' },
        { path: '/api/ops/image-age-audit-v2328', name: 'Image Age', icon: '\u{1F4C5}' },
        // v23.29 Security
        { path: '/api/security/sysctl-audit-v2329', name: 'Sysctl Audit', icon: '\u{1F6E1}' },
        { path: '/api/security/projected-volume-audit-v2329', name: 'Projected Vol', icon: '\u{1F4E6}' },
        { path: '/api/security/rolebinding-user-audit-v2329', name: 'RB User Audit', icon: '\u{1F451}' },
        // v23.30 Documentation
        { path: '/api/docs/endpoint-ready-status-v2330', name: 'EP Ready', icon: '\u2705' },
        { path: '/api/docs/node-allocatable-cpu-v2330', name: 'Node Alloc CPU', icon: '\u26A1' },
        { path: '/api/docs/pod-dns-config-catalog-v2330', name: 'Pod DNS Config', icon: '\u{1F310}' },
        // v23.31 Scalability
        { path: '/api/scalability/image-registry-distribution-v2331', name: 'Img Registry', icon: '\u{1F5C3}' },
        { path: '/api/scalability/node-pod-headroom-v2331', name: 'Node Headroom', icon: '\u{1F3D7}' },
        { path: '/api/scalability/namespace-service-density-v2331', name: 'NS Svc Density', icon: '\u{1F4CA}' },
        // v23.32 Product
        { path: '/api/product/supplemental-groups-audit-v2332', name: 'Suppl Groups', icon: '\u{1F465}' },
        { path: '/api/product/termination-message-path-v2332', name: 'TermMsg Path', icon: '\u{1F4DD}' },
        { path: '/api/product/lb-source-range-audit-v2332', name: 'LB Src Range', icon: '\u{1F6E1}' },
        // v23.33 Deployment
        { path: '/api/deployment/sts-pvc-template-size-v2333', name: 'STS PVC Size', icon: '\u{1F4BF}' },
        { path: '/api/deployment/max-unavailable-custom-v2333', name: 'MaxUnavail', icon: '\u{1F4C9}' },
        { path: '/api/deployment/cronjob-history-limits-v2333', name: 'CJ Hist Lim', icon: '\u{23F0}' },
        // v23.34 Operations
        { path: '/api/ops/failed-scheduling-census-v2334', name: 'Failed Sched', icon: '\u{26A0}' },
        { path: '/api/ops/node-network-condition-v2334', name: 'Node Net Cond', icon: '\u{1F310}' },
        { path: '/api/ops/resource-summary-v2334', name: 'Res Summary', icon: '\u{1F4CA}' },
        // v23.35 Security
        { path: '/api/security/fsgroup-always-policy-v2335', name: 'FSGroup Always', icon: '\u{1F465}' },
        { path: '/api/security/sa-automount-default-v2335', name: 'SA Automount', icon: '\u{1F511}' },
        { path: '/api/security/secret-immutable-mark-v2335', name: 'Secret Immutable', icon: '\u{1F512}' },
        // v23.36 Documentation
        { path: '/api/docs/hostnetwork-namespace-audit-v2336', name: 'HostNet NS', icon: '\u{1F310}' },
        { path: '/api/docs/node-systemuuid-census-v2336', name: 'Node SysUUID', icon: '\u{1F5A5}' },
        { path: '/api/docs/configmap-immutable-mark-v2336', name: 'CM Immutable', icon: '\u{1F4DD}' },
        // v23.37 Scalability
        { path: '/api/scalability/top-namespace-secret-count-v2337', name: 'Top NS Secret', icon: '\u{1F510}' },
        { path: '/api/scalability/node-cpu-headroom-v2337', name: 'Node CPU Headrm', icon: '\u26A1' },
        { path: '/api/scalability/endpoint-health-ratio-v2337', name: 'EP Health Ratio', icon: '\u{1F493}' },
        // v23.38 Product
        { path: '/api/product/pod-os-name-windows-v2338', name: 'Pod OS Name', icon: '\u{1F4BB}' },
        { path: '/api/product/stderr-redirect-audit-v2338', name: 'Stderr Redirect', icon: '\u{1F4DD}' },
        { path: '/api/product/session-affinity-config-v2338', name: 'Session Aff Cfg', icon: '\u{1F91D}' },
        // v23.39 Deployment
        { path: '/api/deployment/sts-collision-count-v2339', name: 'STS Collision', icon: '\u{26A0}' },
        { path: '/api/deployment/ds-updated-desired-v2339', name: 'DS Upd/Desired', icon: '\u{1F680}' },
        { path: '/api/deployment/job-ttl-seconds-v2339', name: 'Job TTL', icon: '\u{23F1}' },
        // v23.40 Operations
        { path: '/api/ops/image-id-catalog-v2340', name: 'Image ID', icon: '\u{1F4E6}' },
        { path: '/api/ops/node-condition-disk-v2340', name: 'Node Disk Cond', icon: '\u{1F4BF}' },
        { path: '/api/ops/volume-device-audit-v2340', name: 'Vol Device', icon: '\u{1F4BD}' },
        // v23.41 Security
        { path: '/api/security/uid-range-audit-v2341', name: 'UID Range', icon: '\u{1F464}' },
        { path: '/api/security/docker-config-secret-v2341', name: 'Docker Config', icon: '\u{1F510}' },
        { path: '/api/security/role-verb-wildcard-v2341', name: 'Role Verb Wild', icon: '\u{1F451}' },
        // v23.42 Documentation
        { path: '/api/docs/node-region-label-v2342', name: 'Node Region', icon: '\u{1F5FA}' },
        { path: '/api/docs/pod-owner-kind-v2342', name: 'Pod Owner Kind', icon: '\u{1F517}' },
        { path: '/api/docs/secret-creation-order-v2342', name: 'Secret Create Order', icon: '\u{1F4C5}' },
        // v23.43 Scalability
        { path: '/api/scalability/node-zone-distribution-v2343', name: 'Node Zone Dist', icon: '\u{1F310}' },
        { path: '/api/scalability/scheduling-latency-risk-v2343', name: 'Sched Latency', icon: '\u{23F1}' },
        { path: '/api/scalability/deployment-density-v2343', name: 'Deploy Density', icon: '\u{1F4CA}' },
        // v23.44 Product
        { path: '/api/product/pod-hostusers-audit-v2344', name: 'HostUsers Audit', icon: '\u{1F464}' },
        { path: '/api/product/container-hostport-audit-v2344', name: 'HostPort Audit', icon: '\u{1F50C}' },
        { path: '/api/product/service-externalip-catalog-v2344', name: 'ExternalIP', icon: '\u{1F310}' },
        // v23.45 Deployment
        { path: '/api/deployment/sts-replicas-vs-ready-v2345', name: 'STS Rep/Ready', icon: '\u{1F504}' },
        { path: '/api/deployment/ds-number-unavailable-v2345', name: 'DS Unavail', icon: '\u{1F680}' },
        { path: '/api/deployment/job-completion-duration-v2345', name: 'Job Duration', icon: '\u{23F1}' },
        // v23.46 Operations
        { path: '/api/ops/unhealthy-container-v2346', name: 'Unhealthy Ctnr', icon: '\u{1F6A8}' },
        { path: '/api/ops/node-condition-pid-v2346', name: 'Node PID Cond', icon: '\u{1F534}' },
        { path: '/api/ops/event-message-catalog-v2346', name: 'Event Msg', icon: '\u{1F4CB}' },
        // v23.47 Security
        { path: '/api/security/auto-sa-token-audit-v2347', name: 'Auto SA Token', icon: '\u{1F511}' },
        { path: '/api/security/secret-type-tls-v2347', name: 'Secret TLS', icon: '\u{1F510}' },
        { path: '/api/security/clusterrole-verbs-census-v2347', name: 'CR Verbs', icon: '\u{1F451}' },
        // v23.48 Documentation
        { path: '/api/docs/node-zone-label-v2348', name: 'Node Zone Label', icon: '\u{1F310}' },
        { path: '/api/docs/pod-resource-request-summary-v2348', name: 'Pod ResReq', icon: '\u{1F4CA}' },
        { path: '/api/docs/secret-namespace-count-v2348', name: 'Secret NS Count', icon: '\u{1F4C4}' },
        // v23.49 Scalability
        { path: '/api/scalability/top-node-container-count-v2349', name: 'Top Node Ctnr', icon: '\u{1F3E0}' },
        { path: '/api/scalability/cluster-hpa-coverage-v2349', name: 'HPA Coverage', icon: '\u{1F4C8}' },
        { path: '/api/scalability/namespace-replica-distribution-v2349', name: 'NS Replica Dist', icon: '\u{1F4CA}' },
        // v23.50 Product
        { path: '/api/product/fqdn-coverage-v2350', name: 'FQDN Coverage', icon: '\u{1F310}' },
        { path: '/api/product/empty-resource-audit-v2350', name: 'Empty Res Audit', icon: '\u{26A0}' },
        { path: '/api/product/nodeport-healthcheck-v2350', name: 'NodePort HC', icon: '\u{1F50C}' },
        // v23.51 Deployment
        { path: '/api/deployment/deployment-updated-replicas-v2351', name: 'Deploy Updated', icon: '\u{1F4C8}' },
        { path: '/api/deployment/sts-current-replicas-v2351', name: 'STS Current Rep', icon: '\u{1F504}' },
        { path: '/api/deployment/rs-full-status-v2351', name: 'RS Full Status', icon: '\u{1F4CA}' },
        // v23.52 Operations
        { path: '/api/ops/waiting-reason-catalog-v2352', name: 'Waiting Reason', icon: '\u{23F3}' },
        { path: '/api/ops/node-memory-allocatable-v2352', name: 'Node Mem Alloc', icon: '\u{1F4BE}' },
        { path: '/api/ops/limit-cpu-summary-v2352', name: 'Limit CPU Summary', icon: '\u26A1' },
        // v23.53 Security
        { path: '/api/security/seccomp-localhost-v2353', name: 'Seccomp Local', icon: '\u{1F6E1}' },
        { path: '/api/security/secret-basic-auth-v2353', name: 'Secret BasicAuth', icon: '\u{1F510}' },
        { path: '/api/security/role-resource-wildcard-v2353', name: 'Role Res Wild', icon: '\u{1F451}' },
        // v23.54 Documentation
        { path: '/api/docs/node-instance-type-v2354', name: 'Node InstanceType', icon: '\u{1F527}' },
        { path: '/api/docs/env-from-configmap-v2354', name: 'Env From CM', icon: '\u{1F4DD}' },
        { path: '/api/docs/pvc-storageclassname-v2354', name: 'PVC SC Name', icon: '\u{1F4BF}' },
        // v23.55 Scalability
        { path: '/api/scalability/top-namespace-configmap-v2355', name: 'Top NS CM', icon: '\u{1F4DA}' },
        { path: '/api/scalability/node-cpu-allocatable-core-v2355', name: 'Node CPU Core', icon: '\u26A1' },
        { path: '/api/scalability/sts-density-v2355', name: 'STS Density', icon: '\u{1F3E8}' },
        // v23.56 Product
        { path: '/api/product/pod-hostname-audit-v2356', name: 'Pod Hostname', icon: '\u{1F4BB}' },
        { path: '/api/product/container-stdin-audit-v2356', name: 'Ctnr Stdin', icon: '\u{2328}' },
        { path: '/api/product/service-ipfamily-v2356', name: 'Svc IP Family', icon: '\u{1F310}' },
        // v23.57 Deployment
        { path: '/api/deployment/ds-nodename-target-v2357', name: 'DS NodeName', icon: '\u{1F3ED}' },
        { path: '/api/deployment/sts-podmgmt-policy-v2357', name: 'STS PodMgmt', icon: '\u{1F522}' },
        { path: '/api/deployment/cronjob-timezone-v2357', name: 'CJ TimeZone', icon: '\u{1F30D}' },
        // v23.58 Operations
        { path: '/api/ops/terminated-signal-v2358', name: 'Term Signal', icon: '\u{1F480}' },
        { path: '/api/ops/kubelet-version-v2358', name: 'Kubelet Ver', icon: '\u{1F527}' },
        { path: '/api/ops/event-type-distribution-v2358', name: 'Event Type', icon: '\u{1F4CB}' },
        // v23.59 Security
        { path: '/api/security/runas-group-audit-v2359', name: 'RunAsGroup', icon: '\u{1F465}' },
        { path: '/api/security/secret-ssh-key-v2359', name: 'SSH Key Secret', icon: '\u{1F511}' },
        { path: '/api/security/role-apigroups-census-v2359', name: 'Role APIGroups', icon: '\u{1F451}' },
        // v23.60 Documentation
        { path: '/api/docs/node-hostname-audit-v2360', name: 'Node Hostname', icon: '\u{1F5A5}' },
        { path: '/api/docs/image-pull-policy-v2360', name: 'Pull Policy', icon: '\u{1F4E5}' },
        { path: '/api/docs/pv-reclaim-policy-v2360', name: 'PV Reclaim', icon: '\u{1F4BF}' },
        // v23.61 Scalability
        { path: '/api/scalability/namespace-pvc-total-v2361', name: 'NS PVC Total', icon: '\u{1F4BF}' },
        { path: '/api/scalability/node-container-runtime-v2361', name: 'Node Runtime', icon: '\u{1F527}' },
        { path: '/api/scalability/cluster-ingress-total-v2361', name: 'Ingress Total', icon: '\u{1F310}' },
        // v23.62 Product
        { path: '/api/product/share-process-namespace-v2362', name: 'Share ProcNS', icon: '\u{1F517}' },
        { path: '/api/product/missing-resource-limits-v2362', name: 'Missing Limits', icon: '\u{26A0}' },
        { path: '/api/product/healthcheck-port-audit-v2362', name: 'HC Port', icon: '\u{1F50C}' },
        // v23.63 Deployment
        { path: '/api/deployment/deployment-strategy-type-v2363', name: 'Deploy Strategy', icon: '\u{1F4C8}' },
        { path: '/api/deployment/sts-update-strategy-v2363', name: 'STS Update Strat', icon: '\u{1F504}' },
        { path: '/api/deployment/ds-revision-count-v2363', name: 'DS Revision', icon: '\u{1F4DC}' },
        // v23.64 Operations
        { path: '/api/ops/pod-start-time-audit-v2364', name: 'Pod StartTime', icon: '\u{23F1}' },
        { path: '/api/ops/node-architecture-census-v2364', name: 'Node Arch', icon: '\u{1F527}' },
        { path: '/api/ops/event-recent-count-v2364', name: 'Recent Events', icon: '\u{1F4CB}' },
        // v23.65 Security
        { path: '/api/security/fsgroup-override-v2365', name: 'FSGroup Override', icon: '\u{1F465}' },
        { path: '/api/security/sa-token-secret-v2365', name: 'SA Token Secret', icon: '\u{1F511}' },
        { path: '/api/security/role-nonresource-url-v2365', name: 'Role NonRes URL', icon: '\u{1F451}' },
        // v23.66 Documentation
        { path: '/api/docs/restart-policy-distribution-v2366', name: 'Restart Policy', icon: '\u{1F501}' },
        { path: '/api/docs/node-os-image-v2366', name: 'Node OS Image', icon: '\u{1F4BB}' },
        { path: '/api/docs/service-port-target-v2366', name: 'Svc Port Target', icon: '\u{1F50C}' },
        // v23.67 Scalability
        { path: '/api/scalability/top-namespace-deployment-v2367', name: 'Top NS Deploy', icon: '\u{1F4CA}' },
        { path: '/api/scalability/node-capacity-storage-v2367', name: 'Node Cap Storage', icon: '\u{1F4BF}' },
        { path: '/api/scalability/networkpolicy-density-v2367', name: 'NetPol Density', icon: '\u{1F6E1}' },
        // v23.68 Product
        { path: '/api/product/host-ipc-audit-v2368', name: 'HostIPC Audit', icon: '\u{1F517}' },
        { path: '/api/product/container-probe-timeout-v2368', name: 'Probe Timeout', icon: '\u{23F1}' },
        { path: '/api/product/clusterip-type-audit-v2368', name: 'ClusterIP Type', icon: '\u{1F310}' },
        // v23.69 Deployment
        { path: '/api/deployment/deployment-minready-v2369', name: 'Deploy MinReady', icon: '\u{1F4C8}' },
        { path: '/api/deployment/sts-minready-v2369', name: 'STS MinReady', icon: '\u{1F4C8}' },
        { path: '/api/deployment/ds-minready-v2369', name: 'DS MinReady', icon: '\u{1F4C8}' },
        // v23.70 Operations
        { path: '/api/ops/pod-condition-type-v2370', name: 'Pod CondType', icon: '\u{1F4CB}' },
        { path: '/api/ops/node-os-name-v2370', name: 'Node OS Name', icon: '\u{1F4BB}' },
        { path: '/api/ops/termination-exitcode-v2370', name: 'Term ExitCode', icon: '\u{1F480}' },
        // v23.71 Security
        { path: '/api/security/nonroot-uid-audit-v2371', name: 'NonRoot UID', icon: '\u{1F464}' },
        { path: '/api/security/helm-secret-audit-v2371', name: 'Helm Secret', icon: '\u{1F510}' },
        { path: '/api/security/crb-roleref-census-v2371', name: 'CRB RoleRef', icon: '\u{1F451}' },
        // v23.72 Documentation
        { path: '/api/docs/node-provider-label-v2372', name: 'Node Provider', icon: '\u{1F527}' },
        { path: '/api/docs/pod-env-var-count-v2372', name: 'Pod Env Count', icon: '\u{1F4DD}' },
        { path: '/api/docs/secret-annotation-census-v2372', name: 'Secret Annot', icon: '\u{1F4DD}' },
        // v23.73 Scalability
        { path: '/api/scalability/top-node-by-pod-v2373', name: 'Top Node Pods', icon: '\u{1F3E0}' },
        { path: '/api/scalability/namespace-hpa-coverage-v2373', name: 'NS HPA Cov', icon: '\u{1F4C8}' },
        { path: '/api/scalability/endpoint-service-ratio-v2373', name: 'EP/Svc Ratio', icon: '\u{1F4CA}' },
        // v23.74 Product
        { path: '/api/product/pod-priority-audit-v2374', name: 'Pod Priority', icon: '\u{1F3ED}' },
        { path: '/api/product/readiness-probe-exist-v2374', name: 'Readiness Exist', icon: '\u2705' },
        { path: '/api/product/service-targetport-custom-v2374', name: 'TargetPort', icon: '\u{1F50C}' },
        // v23.75 Deployment
        { path: '/api/deployment/max-surge-config-v2375', name: 'MaxSurge Cfg', icon: '\u{1F4C9}' },
        { path: '/api/deployment/sts-servicename-empty-v2375', name: 'STS SvcName Empty', icon: '\u{26A0}' },
        { path: '/api/deployment/cronjob-suspend-status-v2375', name: 'CJ Suspend', icon: '\u{23F8}' },
        // v23.76 Operations
        { path: '/api/ops/qos-guaranteed-ratio-v2376', name: 'QoS Guaranteed', icon: '\u{1F4CA}' },
        { path: '/api/ops/node-kernel-version-v2376', name: 'Node Kernel', icon: '\u{1F527}' },
        { path: '/api/ops/event-reason-catalog-v2376', name: 'Event Reason', icon: '\u{1F4CB}' },
        // v23.77 Security
        { path: '/api/security/runas-nonroot-audit-v2377', name: 'RunAsNonRoot', icon: '\u{1F464}' },
        { path: '/api/security/secret-type-census-v2377', name: 'Secret Type', icon: '\u{1F510}' },
        { path: '/api/security/rolebinding-kind-census-v2377', name: 'RB Kind', icon: '\u{1F451}' },
        // v23.78 Documentation
        { path: '/api/docs/node-cr-version-v2378', name: 'Node CR Ver', icon: '\u{1F527}' },
        { path: '/api/docs/pod-uid-audit-v2378', name: 'Pod UID', icon: '\u{1F194}' },
        { path: '/api/docs/configmap-age-distribution-v2378', name: 'CM Age Dist', icon: '\u{1F4C5}' },
        // v23.79 Scalability
        { path: '/api/scalability/top-namespace-replica-v2379', name: 'Top NS Replica', icon: '\u{1F4CA}' },
        { path: '/api/scalability/node-memory-capacity-v2379', name: 'Node Mem Cap', icon: '\u{1F4BE}' },
        { path: '/api/scalability/daemonset-spread-v2379', name: 'DS Spread', icon: '\u{1F680}' },
        // v23.80 Product
        { path: '/api/product/dns-search-domains-v2380', name: 'DNS Search', icon: '\u{1F310}' },
        { path: '/api/product/liveness-probe-audit-v2380', name: 'Liveness Probe', icon: '\u{1F494}' },
        { path: '/api/product/service-type-distribution-v2380', name: 'Svc Type Dist', icon: '\u{1F4CA}' },
        // v23.81 Deployment
        { path: '/api/deployment/deploy-revision-history-v2381', name: 'Deploy RevHist', icon: '\u{1F4DC}' },
        { path: '/api/deployment/sts-template-hash-count-v2381', name: 'STS Hash Count', icon: '\u{1F3F7}' },
        { path: '/api/deployment/job-active-pods-v2381', name: 'Job Active Pods', icon: '\u{1F680}' },
        // v23.82 Operations
        { path: '/api/ops/pod-phase-census-v2382', name: 'Pod Phase', icon: '\u{1F4CA}' },
        { path: '/api/ops/node-capacity-pods-v2382', name: 'Node Cap Pods', icon: '\u{1F3E0}' },
        { path: '/api/ops/container-state-running-v2382', name: 'Ctnr Running', icon: '\u25B6' },
        // v23.83 Security
        { path: '/api/security/readonly-rootfs-audit-v2383', name: 'ReadOnly RootFS', icon: '\u{1F512}' },
        { path: '/api/security/secret-empty-data-v2383', name: 'Secret Empty', icon: '\u{1F4ED}' },
        { path: '/api/security/role-binding-all-v2383', name: 'Role Bind All', icon: '\u{1F451}' },
        // v23.84 Documentation
        { path: '/api/docs/node-kubeproxy-version-v2384', name: 'Node KP Ver', icon: '\u{1F527}' },
        { path: '/api/docs/image-size-catalog-v2384', name: 'Image Size', icon: '\u{1F4E6}' },
        { path: '/api/docs/pvc-access-mode-v2384', name: 'PVC Access Mode', icon: '\u{1F4BF}' },
        // v23.85 Scalability
        { path: '/api/scalability/top-image-deployment-v2385', name: 'Top Img Deploy', icon: '\u{1F4E6}' },
        { path: '/api/scalability/node-cpu-limit-commit-v2385', name: 'Node CPU Limit', icon: '\u26A1' },
        { path: '/api/scalability/cluster-service-total-v2385', name: 'Svc Total', icon: '\u{1F4CA}' },
        // v23.86 Product
        { path: '/api/product/pod-tolerations-audit-v2386', name: 'Tolerations', icon: '\u{1F6E1}' },
        { path: '/api/product/container-port-catalog-v2386', name: 'Ctnr Ports', icon: '\u{1F50C}' },
        { path: '/api/product/service-annotation-census-v2386', name: 'Svc Annot', icon: '\u{1F4DD}' },
        // v23.87 Deployment
        { path: '/api/deployment/deploy-label-count-v2387', name: 'Deploy Labels', icon: '\u{1F3F7}' },
        { path: '/api/deployment/sts-volumeclaim-count-v2387', name: 'STS VolClaim', icon: '\u{1F4BF}' },
        { path: '/api/deployment/cronjob-failed-jobs-v2387', name: 'CJ Failed', icon: '\u{1F6A8}' },
        // v23.88 Operations
        { path: '/api/ops/init-container-count-v2388', name: 'Init Ctnr', icon: '\u{1F504}' },
        { path: '/api/ops/node-allocatable-pods-v2388', name: 'Node Alloc Pods', icon: '\u{1F3E0}' },
        { path: '/api/ops/event-by-namespace-v2388', name: 'Event by NS', icon: '\u{1F4CB}' },
        // v23.89 Security
        { path: '/api/security/privilege-escalation-audit-v2389', name: 'PrivEsc Audit', icon: '\u{26A0}' },
        { path: '/api/security/secret-key-count-v2389', name: 'Secret KeyCnt', icon: '\u{1F511}' },
        { path: '/api/security/clusterrole-rule-count-v2389', name: 'CR Rule Cnt', icon: '\u{1F451}' },
        // v23.90 Documentation
        { path: '/api/docs/node-kernel-commit-v2390', name: 'Node Kernel', icon: '\u{1F527}' },
        { path: '/api/docs/pod-finalizer-count-v2390', name: 'Pod Finalizer', icon: '\u{1F527}' },
        { path: '/api/docs/pvc-size-summary-v2390', name: 'PVC Size', icon: '\u{1F4BF}' },
        // v23.91 Scalability
        { path: '/api/scalability/top-namespace-event-v2391', name: 'Top NS Event', icon: '\u{1F4CB}' },
        { path: '/api/scalability/node-allocatable-mem-v2391', name: 'Node Alloc Mem', icon: '\u{1F4BE}' },
        { path: '/api/scalability/pod-by-controller-v2391', name: 'Pod by Ctrl', icon: '\u{1F517}' },
        // v23.92 Product
        { path: '/api/product/affinity-rule-count-v2392', name: 'Affinity Rules', icon: '\u{1F517}' },
        { path: '/api/product/env-valuefrom-audit-v2392', name: 'Env ValueFrom', icon: '\u{1F4DD}' },
        { path: '/api/product/loadbalancer-class-v2392', name: 'LB Class', icon: '\u{1F310}' },
        // v23.93 Deployment
        { path: '/api/deployment/deployment-paused-status-v2393', name: 'Deploy Paused', icon: '\u{23F8}' },
        { path: '/api/deployment/sts-ordinal-replicas-v2393', name: 'STS Ordinal', icon: '\u{1F522}' },
        { path: '/api/deployment/job-backoff-limit-v2393', name: 'Job Backoff', icon: '\u{1F501}' },
        // v23.94 Operations
        { path: '/api/ops/crashloop-backoff-v2394', name: 'CrashLoop', icon: '\u{1F6A8}' },
        { path: '/api/ops/node-condition-memory-v2394', name: 'Node Mem Cond', icon: '\u{1F4BE}' },
        { path: '/api/ops/container-restart-count-v2394', name: 'Restart Count', icon: '\u{1F504}' },
        // v23.95 Security
        { path: '/api/security/capabilities-audit-v2395', name: 'Caps Audit', icon: '\u{1F6E1}' },
        { path: '/api/security/secret-data-size-v2395', name: 'Secret Data Size', icon: '\u{1F510}' },
        { path: '/api/security/rolebinding-namespace-v2395', name: 'RB by NS', icon: '\u{1F451}' },
        // v23.96 Documentation
        { path: '/api/docs/node-taint-count-v2396', name: 'Node Taints', icon: '\u{1F6A7}' },
        { path: '/api/docs/pod-nodeselector-key-v2396', name: 'NodeSelector Key', icon: '\u{1F50D}' },
        { path: '/api/docs/endpoint-address-by-node-v2396', name: 'EP Addr by Node', icon: '\u{1F5FA}' },
        // v23.97 Scalability
        { path: '/api/scalability/top-namespace-cpu-limit-v2397', name: 'Top NS CPULim', icon: '\u26A1' },
        { path: '/api/scalability/node-allocatable-cpu-summary-v2397', name: 'Node Alloc CPU', icon: '\u26A1' },
        { path: '/api/scalability/pvc-density-v2397', name: 'PVC Density', icon: '\u{1F4BF}' },
        // v23.98 Product
        { path: '/api/product/node-affinity-required-v2398', name: 'NodeAff Req', icon: '\u{1F517}' },
        { path: '/api/product/volume-mount-count-v2398', name: 'VolMount Count', icon: '\u{1F4BD}' },
        { path: '/api/product/alloc-lb-nodeports-v2398', name: 'Alloc LB NP', icon: '\u{1F50C}' },
        // v23.99 Deployment
        { path: '/api/deployment/progress-deadline-v2399', name: 'Progress Dl', icon: '\u{23F1}' },
        { path: '/api/deployment/sts-pvc-retain-policy-v2399', name: 'STS PVC Retain', icon: '\u{1F4BF}' },
        { path: '/api/deployment/ds-template-generation-v2399', name: 'DS TemplateGen', icon: '\u{1F4DC}' },
        // v24.00 Operations
        { path: '/api/ops/oom-killed-audit-v2400', name: 'OOMKilled', icon: '\u{1F480}' },
        { path: '/api/ops/node-cond-kubelet-v2400', name: 'Node Kubelet', icon: '\u{1F527}' },
        { path: '/api/ops/volume-device-count-v2400', name: 'VolDevice Cnt', icon: '\u{1F4BD}' },
        // v24.01 Security
        { path: '/api/security/privileged-container-v2401', name: 'Privileged Ctnr', icon: '\u{26A0}' },
        { path: '/api/security/secret-type-opaque-v2401', name: 'Secret Opaque', icon: '\u{1F510}' },
        { path: '/api/security/rolebinding-subjects-count-v2401', name: 'RB Subjects', icon: '\u{1F451}' },
        // v24.02 Documentation
        { path: '/api/docs/node-label-count-v2402', name: 'Node Label Cnt', icon: '\u{1F3F7}' },
        { path: '/api/docs/pod-volume-count-v2402', name: 'Pod Volume Cnt', icon: '\u{1F4E6}' },
        { path: '/api/docs/configmap-data-key-size-v2402', name: 'CM DataKey', icon: '\u{1F4DD}' },
        // v24.03 Scalability
        { path: '/api/scalability/top-node-memory-request-v2403', name: 'Top Node MemReq', icon: '\u{1F4BE}' },
        { path: '/api/scalability/namespace-sa-count-v2403', name: 'NS SA Count', icon: '\u{1F511}' },
        { path: '/api/scalability/cluster-image-unique-v2403', name: 'Cluster Img Unique', icon: '\u{1F4E6}' },
        // v24.04 Product
        { path: '/api/product/pod-overhead-resource-v2404', name: 'Pod Overhead', icon: '\u{1F4CA}' },
        { path: '/api/product/container-runasuser-v2404', name: 'Ctnr RunAsUser', icon: '\u{1F464}' },
        { path: '/api/product/external-traffic-policy-v2404', name: 'Ext Traffic Pol', icon: '\u{1F697}' },
        // v24.05 Deployment
        { path: '/api/deployment/deployment-conditions-v2405', name: 'Deploy Conds', icon: '\u{2705}' },
        { path: '/api/deployment/sts-status-available-v2405', name: 'STS Available', icon: '\u{1F504}' },
        { path: '/api/deployment/ds-conditions-ready-v2405', name: 'DS Ready', icon: '\u{1F680}' },
        // v24.06 Operations
        { path: '/api/ops/high-restarts-audit-v2406', name: 'High Restarts', icon: '\u{1F6A8}' },
        { path: '/api/ops/node-bootid-census-v2406', name: 'Node BootID', icon: '\u{1F4BB}' },
        { path: '/api/ops/event-involved-object-kind-v2406', name: 'Event ObjKind', icon: '\u{1F4CB}' },
        // v24.07 Security
        { path: '/api/security/seccomp-runtimedefault-v2407', name: 'Seccomp RD', icon: '\u{1F6E1}' },
        { path: '/api/security/secret-helm-annotation-v2407', name: 'Secret Helm', icon: '\u{1F510}' },
        { path: '/api/security/crb-subject-sa-v2407', name: 'CRB SA Subject', icon: '\u{1F511}' },
        // v24.08 Documentation
        { path: '/api/docs/allocatable-ephemeral-v2408', name: 'Alloc Ephemeral', icon: '\u{1F4BD}' },
        { path: '/api/docs/pod-subdomain-audit-v2408', name: 'Pod Subdomain', icon: '\u{1F310}' },
        { path: '/api/docs/configmap-binarydata-v2408', name: 'CM BinaryData', icon: '\u{1F4DD}' },
        // v24.09 Scalability
        { path: '/api/scalability/top-namespace-container-v2409', name: 'Top NS Ctnr', icon: '\u{1F3E0}' },
        { path: '/api/scalability/node-allocatable-stor-ephemeral-v2409', name: 'Node Ephemeral', icon: '\u{1F4BD}' },
        { path: '/api/scalability/cluster-role-total-v2409', name: 'Role Total', icon: '\u{1F451}' },
        // v24.10 Product
        { path: '/api/product/pod-preemption-policy-v2410', name: 'Preemption Pol', icon: '\u{26A1}' },
        { path: '/api/product/container-workingdir-v2410', name: 'WorkingDir', icon: '\u{1F4C1}' },
        { path: '/api/product/internal-traffic-policy-v2410', name: 'Int Traffic Pol', icon: '\u{1F697}' },
        // v24.11 Deployment
        { path: '/api/deployment/sts-servicename-audit-v2411', name: 'STS SvcName', icon: '\u{1F517}' },
        { path: '/api/deployment/job-parallelism-config-v2411', name: 'Job Parallel', icon: '\u{1F504}' },
        { path: '/api/deployment/cronjob-concurrency-allow-v2411', name: 'CJ Concurrency', icon: '\u{1F91D}' },
        // v24.12 Operations
        { path: '/api/ops/grace-period-audit-v2412', name: 'Grace Period', icon: '\u{23F1}' },
        { path: '/api/ops/node-memory-capacity-gb-v2412', name: 'Node Mem Cap GB', icon: '\u{1F4BE}' },
        { path: '/api/ops/event-source-component-v2412', name: 'Event Source', icon: '\u{1F4CB}' },
        // v24.13 Security
        { path: '/api/security/drop-all-capabilities-v2413', name: 'Drop ALL Cap', icon: '\u{1F6E1}' },
        { path: '/api/security/secret-stale-365d-v2413', name: 'Secret Stale', icon: '\u{1F4C5}' },
        { path: '/api/security/role-resourcenames-count-v2413', name: 'Role ResNames', icon: '\u{1F451}' },
        // v24.14 Documentation
        { path: '/api/docs/node-kernel-bootid-v2414', name: 'Node BootID', icon: '\u{1F4BB}' },
        { path: '/api/docs/pod-imagepullsecret-count-v2414', name: 'Pod IPS Count', icon: '\u{1F510}' },
        { path: '/api/docs/configmap-namespace-count-v2414', name: 'CM NS Count', icon: '\u{1F4DA}' },
        // v24.15 Scalability
        { path: '/api/scalability/top-namespace-pvc-v2415', name: 'Top NS PVC', icon: '\u{1F4BF}' },
        { path: '/api/scalability/node-storage-allocatable-v2415', name: 'Node Stor Alloc', icon: '\u{1F4BD}' },
        { path: '/api/scalability/secret-by-type-v2415', name: 'Secret by Type', icon: '\u{1F510}' },
        // v24.16 Product
        { path: '/api/product/scheduler-name-audit-v2416', name: 'Scheduler Name', icon: '\u{1F4CB}' },
        { path: '/api/product/request-memory-summary-v2416', name: 'Req Mem Summary', icon: '\u{1F4BE}' },
        { path: '/api/product/service-clusterips-count-v2416', name: 'ClusterIPs Count', icon: '\u{1F310}' },
        // v24.17 Deployment
        { path: '/api/deployment/deploy-selector-matchlabels-v2417', name: 'Deploy MatchLbls', icon: '\u{1F3F7}' },
        { path: '/api/deployment/rs-owner-ref-controller-v2417', name: 'RS OwnerRef', icon: '\u{1F517}' },
        { path: '/api/deployment/cronjob-lastschedule-v2417', name: 'CJ LastSched', icon: '\u{1F4C5}' },
        // v24.18 Operations
        { path: '/api/ops/podip-distribution-v2418', name: 'PodIP Dist', icon: '\u{1F310}' },
        { path: '/api/ops/node-machine-info-v2418', name: 'Node Machine', icon: '\u{1F5A5}' },
        { path: '/api/ops/event-firsttimestamp-age-v2418', name: 'Event FirstTS', icon: '\u{1F4CB}' },
        // v24.19 Security
        { path: '/api/security/seccomp-unconfined-v2419', name: 'Seccomp Unc', icon: '\u{26A0}' },
        { path: '/api/security/secret-namespace-count-v2419', name: 'Secret NS Count', icon: '\u{1F510}' },
        { path: '/api/security/rolebinding-subject-user-v2419', name: 'RB User Sub', icon: '\u{1F464}' },
        // v24.20 Documentation
        { path: '/api/docs/node-os-version-v2420', name: 'Node OS Ver', icon: '\u{1F4BB}' },
        { path: '/api/docs/pod-nodename-distribution-v2420', name: 'Pod NodeName', icon: '\u{1F5FA}' },
        { path: '/api/docs/configmap-immutable-key-v2420', name: 'CM Immutable Key', icon: '\u{1F512}' },
        // v24.21 Scalability
        { path: '/api/scalability/top-namespace-storage-v2421', name: 'Top NS Storage', icon: '\u{1F4BF}' },
        { path: '/api/scalability/node-cpu-capacity-v2421', name: 'Node CPU Cap', icon: '\u26A1' },
        { path: '/api/scalability/networkpolicy-by-ns-v2421', name: 'NetPol by NS', icon: '\u{1F6E1}' },
        // v24.22 Product
        { path: '/api/product/sa-missing-audit-v2422', name: 'SA Missing', icon: '\u{26A0}' },
        { path: '/api/product/startup-probe-audit-v2422', name: 'Startup Probe', icon: '\u{1F680}' },
        { path: '/api/product/service-externalname-v2422', name: 'Svc ExternalName', icon: '\u{1F310}' },
        // v24.23 Deployment
        { path: '/api/deployment/sts-volclaim-default-v2423', name: 'STS VolClaim', icon: '\u{1F4BF}' },
        { path: '/api/deployment/job-completions-config-v2423', name: 'Job Completions', icon: '\u2705' },
        { path: '/api/deployment/cronjob-starting-deadline-v2423', name: 'CJ StartDeadline', icon: '\u{23F1}' },
        // v24.24 Operations
        { path: '/api/ops/pod-completed-status-v2424', name: 'Pod Completed', icon: '\u2705' },
        { path: '/api/ops/node-outofdisk-v2424', name: 'Node OutDisk', icon: '\u{1F4BF}' },
        { path: '/api/ops/image-latest-count-v2424', name: 'Image Latest', icon: '\u{1F4E6}' },
        // v24.25 Security
        { path: '/api/security/selinux-level-audit-v2425', name: 'SELinux Level', icon: '\u{1F6E1}' },
        { path: '/api/security/secret-keyname-census-v2425', name: 'Secret KeyName', icon: '\u{1F511}' },
        { path: '/api/security/cr-resourcenames-audit-v2425', name: 'CR ResNames', icon: '\u{1F451}' },
        // v24.26 Documentation
        { path: '/api/docs/node-role-label-v2426', name: 'Node Role', icon: '\u{1F451}' },
        { path: '/api/docs/pod-hostaliases-audit-v2426', name: 'Pod HostAliases', icon: '\u{1F310}' },
        { path: '/api/docs/pvc-phase-distribution-v2426', name: 'PVC Phase', icon: '\u{1F4CA}' },
        // v24.27 Scalability
        { path: '/api/scalability/top-namespace-restart-v2427', name: 'Top NS Restart', icon: '\u{1F504}' },
        { path: '/api/scalability/node-ephemeral-gb-v2427', name: 'Node Ephemeral', icon: '\u{1F4BD}' },
        { path: '/api/scalability/configmap-keys-total-v2427', name: 'CM Keys Total', icon: '\u{1F4DD}' },
        // v24.28 Product
        { path: '/api/product/topology-spread-v2428', name: 'Topology Spread', icon: '\u{1F310}' },
        { path: '/api/product/stdinonce-audit-v2428', name: 'StdinOnce', icon: '\u{2328}' },
        { path: '/api/product/publish-notready-v2428', name: 'Publish NotReady', icon: '\u{1F4E1}' },
        // v24.29 Deployment
        { path: '/api/deployment/deploy-status-replicas-v2429', name: 'Deploy StatusRep', icon: '\u{1F4CA}' },
        { path: '/api/deployment/rs-available-replicas-v2429', name: 'RS Available', icon: '\u{1F504}' },
        { path: '/api/deployment/cronjob-active-count-v2429', name: 'CJ Active Count', icon: '\u{1F680}' },
        // v24.30 Operations
        { path: '/api/ops/pod-pending-count-v2430', name: 'Pod Pending', icon: '\u{23F3}' },
        { path: '/api/ops/node-condition-ready-v2430', name: 'Node Ready', icon: '\u2705' },
        { path: '/api/ops/limit-memory-summary-v2430', name: 'Limit Mem Summary', icon: '\u{1F4BE}' },
        // v24.31 Security
        { path: '/api/security/procmount-unmasked-v2431', name: 'ProcMount Unmask', icon: '\u{26A0}' },
        { path: '/api/security/secret-data-count-v2431', name: 'Secret Data Cnt', icon: '\u{1F510}' },
        { path: '/api/security/cr-verb-create-v2431', name: 'CR Verb Create', icon: '\u{1F451}' },
        // v24.32 Documentation
        { path: '/api/docs/taint-effect-audit-v2432', name: 'Taint Effect', icon: '\u{1F6A7}' },
        { path: '/api/docs/container-args-count-v2432', name: 'Ctnr Args Count', icon: '\u{1F4DD}' },
        { path: '/api/docs/pv-status-phase-v2432', name: 'PV Status Phase', icon: '\u{1F4BF}' },
        // v24.33 Scalability
        { path: '/api/scalability/top-namespace-sa-v2433', name: 'Top NS SA', icon: '\u{1F511}' },
        { path: '/api/scalability/node-storage-capacity-v2433', name: 'Node Stor Cap', icon: '\u{1F4BD}' },
        { path: '/api/scalability/secret-bytes-total-v2433', name: 'Secret Bytes', icon: '\u{1F510}' },
        { path: '/api/product/ephemeral-ctnr-v2434', name: 'Ephemeral Ctnr', icon: '\u{1F4E6}' },
        { path: '/api/product/envfrom-count-v2434', name: 'EnvFrom Count', icon: '\u{1F4CB}' },
        { path: '/api/product/session-timeout-v2434', name: 'Session Timeout', icon: '\u23F1' },
        { path: '/api/deploy/rs-label-v2435', name: 'RS Label', icon: '\u{1F3F7}' },
        { path: '/api/deploy/sts-label-v2435', name: 'STS Label', icon: '\u{1F3F7}' },
        { path: '/api/deploy/ds-label-v2435', name: 'DS Label', icon: '\u{1F3F7}' },
        { path: '/api/ops/qos-ratio-v2436', name: 'QoS Ratio', icon: '\u{1F4CA}' },
        { path: '/api/ops/node-cond-net-v2436', name: 'Node Net Cond', icon: '\u{1F310}' },
        { path: '/api/ops/term-msg-v2436', name: 'Term Msg', icon: '\u{1F4DD}' },
        { path: '/api/security/capadd-specific-v2437', name: 'CapAdd Specific', icon: '\u{1F6E1}' },
        { path: '/api/security/secret-rotation-v2437', name: 'Secret Rotation', icon: '\u{1F504}' },
        { path: '/api/security/rb-roleref-kind-v2437', name: 'RB RoleRef Kind', icon: '\u{1F511}' },
        { path: '/api/docs/node-zone-v2438', name: 'Node Zone', icon: '\u{1F30D}' },
        { path: '/api/docs/pod-finalizer-list-v2438', name: 'Pod Finalizer', icon: '\u{1F9F9}' },
        { path: '/api/docs/cm-annot-count-v2438', name: 'CM Annot Count', icon: '\u{1F4D1}' },
        { path: '/api/scalability/top-node-cpureq-v2439', name: 'Top Node CPUReq', icon: '\u{1F525}' },
        { path: '/api/scalability/node-podcap-usage-v2439', name: 'Node PodCap', icon: '\u{1F4BE}' },
        { path: '/api/scalability/sa-total-v2439', name: 'SA Total', icon: '\u{1F465}' },
        { path: '/api/product/top-pod-mem-req-v2440', name: 'Top Pod MemReq', icon: '\u{1F4A7}' },
        { path: '/api/product/stdin-count-v2440', name: 'Stdin Count', icon: '\u2328' },
        { path: '/api/product/pvc-access-modes-v2440', name: 'PVC Access Modes', icon: '\u{1F4BF}' },
        { path: '/api/deploy/sts-rev-history-v2441', name: 'STS Rev History', icon: '\u{1F4DC}' },
        { path: '/api/deploy/dep-max-surge-v2441', name: 'Dep MaxSurge', icon: '\u{1F4C8}' },
        { path: '/api/deploy/ds-max-unavail-v2441', name: 'DS MaxUnavail', icon: '\u26A0' },
        { path: '/api/ops/pod-restart-total-v2442', name: 'Pod Restart Total', icon: '\u{1F501}' },
        { path: '/api/ops/node-mem-pressure-v2442', name: 'Node Mem Pressure', icon: '\u{1F4BE}' },
        { path: '/api/ops/event-timestamp-spread-v2442', name: 'Event TS Spread', icon: '\u{1F4C5}' },
        { path: '/api/security/runas-nonroot-v2443', name: 'RunAsNonRoot', icon: '\u{1F6E1}' },
        { path: '/api/security/cr-aggregated-rules-v2443', name: 'CR Aggregated Rules', icon: '\u{1F510}' },
        { path: '/api/security/sa-automount-disabled-v2443', name: 'SA AutoMount Off', icon: '\u{1F513}' },
        { path: '/api/docs/node-instance-type-v2444', name: 'Node Instance Type', icon: '\u{1F5A5}' },
        { path: '/api/docs/pod-priority-dist-v2444', name: 'Pod Priority', icon: '\u26A1' },
        { path: '/api/docs/endpoint-slice-count-v2444', name: 'EndpointSlice Count', icon: '\u{1F4CD}' },
        { path: '/api/scalability/top-ns-by-pod-v2445', name: 'Top NS by Pod', icon: '\u{1F4C8}' },
        { path: '/api/scalability/node-alloc-mem-v2445', name: 'Node Alloc Mem', icon: '\u{1F4BE}' },
        { path: '/api/scalability/secret-type-dist-v2445', name: 'Secret Type Dist', icon: '\u{1F511}' },
        { path: '/api/product/host-network-v2446', name: 'HostNetwork', icon: '\u{1F310}' },
        { path: '/api/product/working-dir-v2446', name: 'WorkingDir', icon: '\u{1F4C2}' },
        { path: '/api/product/ext-traffic-policy-v2446', name: 'Ext Traffic Pol', icon: '\u{1F697}' },
        { path: '/api/deploy/dep-partition-v2447', name: 'Dep Partition', icon: '\u{1F4CF}' },
        { path: '/api/deploy/sts-podmgmt-v2447', name: 'STS PodMgmt', icon: '\u{1F5C3}' },
        { path: '/api/deploy/ds-update-strategy-v2447', name: 'DS UpdateStrategy', icon: '\u{1F504}' },
        { path: '/api/ops/node-pid-pressure-v2448', name: 'Node PID Pressure', icon: '\u26A0' },
        { path: '/api/ops/term-grace-period-v2448', name: 'Term Grace Period', icon: '\u23F1' },
        { path: '/api/ops/lifecycle-hooks-v2448', name: 'Lifecycle Hooks', icon: '\u{1F517}' },
        { path: '/api/security/host-pid-v2449', name: 'HostPID', icon: '\u{1F6E1}' },
        { path: '/api/security/docker-config-json-v2449', name: 'DockerConfigJson', icon: '\u{1F433}' },
        { path: '/api/security/crb-subject-kind-v2449', name: 'CRB Subject Kind', icon: '\u{1F511}' },
        { path: '/api/docs/node-os-image-v2450', name: 'Node OS Image', icon: '\u{1F4BB}' },
        { path: '/api/docs/restart-policy-dist-v2450', name: 'RestartPolicy', icon: '\u{1F501}' },
        { path: '/api/docs/service-type-dist-v2450', name: 'ServiceType Dist', icon: '\u{1F4F1}' },
        { path: '/api/scalability/top-ns-cpureq-v2451', name: 'Top NS CPUReq', icon: '\u{1F525}' },
        { path: '/api/scalability/node-cpu-alloc-total-v2451', name: 'Node CPU Alloc', icon: '\u26A1' },
        { path: '/api/scalability/cm-total-v2451', name: 'CM Total', icon: '\u{1F4D1}' },
        { path: '/api/product/host-ipc-v2452', name: 'HostIPC', icon: '\u{1F510}' },
        { path: '/api/product/pull-policy-dist-v2452', name: 'PullPolicy Dist', icon: '\u2B07' },
        { path: '/api/product/lbip-count-v2452', name: 'LBIP Count', icon: '\u{1F310}' },
        { path: '/api/deploy/rs-replicas-dist-v2453', name: 'RS Replicas', icon: '\u{1F4CF}' },
        { path: '/api/deploy/sts-servicename-v2453', name: 'STS SvcName', icon: '\u{1F5C3}' },
        { path: '/api/deploy/ds-nodeselector-v2453', name: 'DS NodeSelector', icon: '\u{1F4CD}' },
        { path: '/api/ops/node-disk-pressure-v2454', name: 'Node DiskPress', icon: '\u{1F4BF}' },
        { path: '/api/ops/host-aliases-count-v2454', name: 'HostAliases', icon: '\u{1F4CD}' },
        { path: '/api/ops/stdin-once-usage-v2454', name: 'StdinOnce', icon: '\u2328' },
        { path: '/api/security/privileged-ctnr-v2455', name: 'Privileged Ctnr', icon: '\u26A0' },
        { path: '/api/security/secret-tls-v2455', name: 'Secret TLS', icon: '\u{1F510}' },
        { path: '/api/security/rb-subject-ns-v2455', name: 'RB Subject NS', icon: '\u{1F511}' },
        { path: '/api/docs/node-kernel-v2456', name: 'Node Kernel', icon: '\u{1F4BB}' },
        { path: '/api/docs/dns-policy-dist-v2456', name: 'DNSPolicy', icon: '\u{1F310}' },
        { path: '/api/docs/service-port-summary-v2456', name: 'Svc Port Summary', icon: '\u{1F4F1}' },
        { path: '/api/scalability/top-ns-mem-v2457', name: 'Top NS Mem', icon: '\u{1F4A7}' },
        { path: '/api/scalability/node-pod-pressure-v2457', name: 'Node Pod Pressure', icon: '\u26A0' },
        { path: '/api/scalability/event-total-v2457', name: 'Event Total', icon: '\u{1F4C2}' },
        { path: '/api/product/dns-config-v2458', name: 'DNS Config', icon: '\u{1F310}' },
        { path: '/api/product/image-size-est-v2458', name: 'Image Size Est', icon: '\u{1F4E6}' },
        { path: '/api/product/clusterip-dist-v2458', name: 'ClusterIP Dist', icon: '\u{1F310}' },
        { path: '/api/deploy/rs-ownerref-v2459', name: 'RS OwnerRef', icon: '\u{1F517}' },
        { path: '/api/deploy/sts-volclaim-v2459', name: 'STS VolClaim', icon: '\u{1F4BF}' },
        { path: '/api/deploy/ds-template-gen-v2459', name: 'DS TemplateGen', icon: '\u{1F4DC}' },
        { path: '/api/ops/node-kubelet-ver-v2460', name: 'Node Kubelet Ver', icon: '\u{1F4BB}' },
        { path: '/api/ops/pod-ready-ratio-v2460', name: 'Pod Ready Ratio', icon: '\u2705' },
        { path: '/api/ops/ctnr-port-exposure-v2460', name: 'Ctnr Port Expose', icon: '\u{1F4F1}' },
        { path: '/api/security/fsgroup-v2461', name: 'FSGroup', icon: '\u{1F510}' },
        { path: '/api/security/secret-immutable-v2461', name: 'Secret Immutable', icon: '\u{1F512}' },
        { path: '/api/security/rb-clusterwide-v2461', name: 'RB ClusterWide', icon: '\u{1F310}' },
        { path: '/api/docs/node-runtime-v2462', name: 'Node Runtime', icon: '\u{1F4BB}' },
        { path: '/api/docs/scheduler-name-v2462', name: 'SchedulerName', icon: '\u{1F4C5}' },
        { path: '/api/docs/ingress-tls-summary-v2462', name: 'Ingress TLS', icon: '\u{1F510}' },
        { path: '/api/scalability/top-node-by-pod-v2463', name: 'Top Node by Pod', icon: '\u{1F4C8}' },
        { path: '/api/scalability/node-cpu-cap-total-v2463', name: 'Node CPU Cap', icon: '\u26A1' },
        { path: '/api/scalability/pv-total-v2463', name: 'PV Total', icon: '\u{1F4BF}' },
        { path: '/api/product/nodeselector-count-v2464', name: 'NodeSelector Count', icon: '\u{1F4CD}' },
        { path: '/api/product/resourcelimit-dist-v2464', name: 'ResourceLimit Dist', icon: '\u{1F4CA}' },
        { path: '/api/product/externalname-count-v2464', name: 'ExternalName', icon: '\u{1F517}' },
        { path: '/api/deploy/dep-paused-v2465', name: 'Dep Paused', icon: '\u23F8' },
        { path: '/api/deploy/sts-ordinal-v2465', name: 'STS Ordinal', icon: '\u{1F522}' },
        { path: '/api/deploy/ds-deletion-v2465', name: 'DS Deletion', icon: '\u{1F5D1}' },
        { path: '/api/ops/node-ready-duration-v2466', name: 'Node Ready Dur', icon: '\u2705' },
        { path: '/api/ops/crash-loop-v2466', name: 'CrashLoop', icon: '\u{1F501}' },
        { path: '/api/ops/image-age-v2466', name: 'Image Age', icon: '\u{1F4E6}' },
        { path: '/api/security/seccomp-profile-v2467', name: 'SeccompProfile', icon: '\u{1F6E1}' },
        { path: '/api/security/secret-key-count-v2467', name: 'Secret Key Count', icon: '\u{1F511}' },
        { path: '/api/security/cr-verbs-total-v2467', name: 'CR Verbs Total', icon: '\u{1F4DD}' },
        { path: '/api/docs/node-arch-v2468', name: 'Node Arch', icon: '\u{1F5A5}' },
        { path: '/api/docs/toleration-summary-v2468', name: 'Toleration Summary', icon: '\u{1F44B}' },
        { path: '/api/docs/ns-label-count-v2468', name: 'NS Label Count', icon: '\u{1F3F7}' },
        { path: '/api/scalability/top-ns-by-svc-v2469', name: 'Top NS by Svc', icon: '\u{1F4F1}' },
        { path: '/api/scalability/node-storcap-total-v2469', name: 'Node StorCap Total', icon: '\u{1F4BF}' },
        { path: '/api/scalability/pvc-bound-v2469', name: 'PVC Bound', icon: '\u2705' },
        { path: '/api/product/affinity-rule-v2470', name: 'Affinity Rule', icon: '\u{1F517}' },
        { path: '/api/product/image-latest-tag-v2470', name: 'Image Latest Tag', icon: '\u{1F4E6}' },
        { path: '/api/product/session-affinity-v2470', name: 'Session Affinity', icon: '\u{1F501}' },
        { path: '/api/deploy/dep-progress-deadline-v2471', name: 'Dep ProgressDeadline', icon: '\u23F1' },
        { path: '/api/deploy/sts-parallel-v2471', name: 'STS Parallel', icon: '\u26D4' },
        { path: '/api/deploy/ds-tolerations-v2471', name: 'DS Tolerations', icon: '\u{1F44B}' },
        { path: '/api/ops/node-net-unavail-v2472', name: 'Node Net Unavail', icon: '\u{1F310}' },
        { path: '/api/ops/qos-burstable-v2472', name: 'QOS Burstable', icon: '\u{1F4CA}' },
        { path: '/api/ops/env-var-count-v2472', name: 'Env Var Count', icon: '\u{1F4CB}' },
        { path: '/api/security/supplemental-groups-v2473', name: 'SupplementalGroups', icon: '\u{1F510}' },
        { path: '/api/security/secret-basic-auth-v2473', name: 'Secret BasicAuth', icon: '\u{1F512}' },
        { path: '/api/security/crb-roleref-name-v2473', name: 'CRB RoleRef Name', icon: '\u{1F511}' },
        { path: '/api/docs/node-bootid-v2474', name: 'Node BootID', icon: '\u{1F4BB}' },
        { path: '/api/docs/pod-subdomain-v2474', name: 'Pod Subdomain', icon: '\u{1F310}' },
        { path: '/api/docs/ingress-hostname-v2474', name: 'Ingress Hostname', icon: '\u{1F517}' },
        { path: '/api/scalability/top-ns-by-pvc-v2475', name: 'Top NS by PVC', icon: '\u{1F4BF}' },
        { path: '/api/scalability/node-alloc-pods-total-v2475', name: 'Node Alloc Pods', icon: '\u{1F465}' },
        { path: '/api/scalability/storageclass-dist-v2475', name: 'StorageClass Dist', icon: '\u{1F4BF}' },
        { path: '/api/product/topology-spread-v2476', name: 'TopologySpread', icon: '\u{1F310}' },
        { path: '/api/product/image-registry-v2476', name: 'Image Registry', icon: '\u{1F4E6}' },
        { path: '/api/product/session-affinity-cfg-v2476', name: 'SessionAffinity Cfg', icon: '\u{1F501}' },
        { path: '/api/deploy/rs-generation-v2477', name: 'RS Generation', icon: '\u{1F522}' },
        { path: '/api/deploy/sts-pvc-retention-v2477', name: 'STS PVC Retention', icon: '\u{1F4BF}' },
        { path: '/api/deploy/ds-affinity-v2477', name: 'DS Affinity', icon: '\u{1F517}' },
        { path: '/api/ops/node-runtime-check-v2478', name: 'Node Runtime Check', icon: '\u{1F4BB}' },
        { path: '/api/ops/pod-phase-dist-v2478', name: 'Pod Phase Dist', icon: '\u{1F4CA}' },
        { path: '/api/ops/probe-summary-v2478', name: 'Probe Summary', icon: '\u{1F50C}' },
        { path: '/api/security/ro-rootfs-v2479', name: 'RO RootFS', icon: '\u{1F6E1}' },
        { path: '/api/security/secret-sa-token-v2479', name: 'Secret SA Token', icon: '\u{1F511}' },
        { path: '/api/security/cr-rules-total-v2479', name: 'CR Rules Total', icon: '\u{1F4DD}' },
        { path: '/api/docs/node-machineid-v2480', name: 'Node MachineID', icon: '\u{1F4BB}' },
        { path: '/api/docs/pod-hostname-v2480', name: 'Pod Hostname', icon: '\u{1F310}' },
        { path: '/api/docs/ns-phase-v2480', name: 'NS Phase', icon: '\u{1F4CB}' },
        { path: '/api/scalability/top-ns-by-cm-v2481', name: 'Top NS by CM', icon: '\u{1F4D1}' },
        { path: '/api/scalability/node-cpu-vs-cap-v2481', name: 'Node CPU vs Cap', icon: '\u26A1' },
        { path: '/api/scalability/netpolicy-total-v2481', name: 'NetPolicy Total', icon: '\u{1F6E1}' },
        { path: '/api/product/init-container-v2482', name: 'InitContainer', icon: '\u{1F4E6}' },
        { path: '/api/product/term-msg-path-v2482', name: 'TermMsgPath', icon: '\u{1F4DD}' },
        { path: '/api/product/ipfamily-policy-v2482', name: 'IPFamilyPolicy', icon: '\u{1F310}' },
        { path: '/api/deploy/rs-image-summary-v2483', name: 'RS Image Summary', icon: '\u{1F4E6}' },
        { path: '/api/deploy/sts-min-ready-v2483', name: 'STS MinReady', icon: '\u23F1' },
        { path: '/api/deploy/ds-template-hash-v2483', name: 'DS TemplateHash', icon: '\u{1F522}' },
        { path: '/api/ops/node-taint-v2484', name: 'Node Taint', icon: '\u26A0' },
        { path: '/api/ops/pod-condition-dist-v2484', name: 'Pod Condition Dist', icon: '\u2705' },
        { path: '/api/ops/image-pull-count-v2484', name: 'ImagePull Count', icon: '\u{1F50C}' },
        { path: '/api/security/host-users-v2485', name: 'NonRoot User', icon: '\u{1F6E1}' },
        { path: '/api/security/secret-ssh-auth-v2485', name: 'Secret SSHAuth', icon: '\u{1F511}' },
        { path: '/api/security/rb-roleref-kind-v2485', name: 'RB RoleRef Kind', icon: '\u{1F511}' },
        { path: '/api/docs/node-kubeproxy-v2486', name: 'Node KubeProxy', icon: '\u{1F4BB}' },
        { path: '/api/docs/pod-nodename-dist-v2486', name: 'Pod NodeName Dist', icon: '\u{1F4CD}' },
        { path: '/api/docs/svc-clusterip-v2486', name: 'Svc ClusterIP', icon: '\u{1F310}' },
        { path: '/api/scalability/top-node-cpu-limit-v2487', name: 'Top Node CPULimit', icon: '\u{1F525}' },
        { path: '/api/scalability/node-memcap-total-v2487', name: 'Node MemCap Total', icon: '\u{1F4BE}' },
        { path: '/api/scalability/epslice-endpoint-total-v2487', name: 'EPSlice Endpoint Total', icon: '\u{1F4CD}' },
        { path: '/api/product/pod-overhead-v2488', name: 'Pod Overhead', icon: '\u{1F4E6}' },
        { path: '/api/product/image-without-tag-v2488', name: 'Image Without Tag', icon: '\u26A0' },
        { path: '/api/product/publish-not-ready-v2488', name: 'PublishNotReady', icon: '\u{1F4E8}' },
        { path: '/api/deploy/rs-status-replicas-v2489', name: 'RS Status Replicas', icon: '\u{1F4CF}' },
        { path: '/api/deploy/sts-update-strategy-v2489', name: 'STS UpdateStrategy', icon: '\u{1F504}' },
        { path: '/api/deploy/ds-desired-count-v2489', name: 'DS DesiredCount', icon: '\u{1F4C8}' },
        { path: '/api/ops/node-unschedulable-v2490', name: 'Node Unschedulable', icon: '\u26D4' },
        { path: '/api/ops/image-pull-backoff-v2490', name: 'ImagePullBackOff', icon: '\u{1F501}' },
        { path: '/api/ops/volume-mount-v2490', name: 'VolumeMount', icon: '\u{1F4BF}' },
        { path: '/api/security/runas-group-v2491', name: 'RunAsGroup', icon: '\u{1F510}' },
        { path: '/api/security/secret-auth-token-v2491', name: 'Secret AuthToken', icon: '\u{1F511}' },
        { path: '/api/security/rb-api-groups-v2491', name: 'RB API Groups', icon: '\u{1F4DD}' },
        { path: '/api/docs/node-os-arch-v2492', name: 'Node OS Arch', icon: '\u{1F5A5}' },
        { path: '/api/docs/pod-priority-value-v2492', name: 'Pod Priority Value', icon: '\u26A1' },
        { path: '/api/docs/ns-finalizer-v2492', name: 'NS Finalizer', icon: '\u{1F9F9}' },
        { path: '/api/scalability/top-ns-by-event-v2493', name: 'Top NS by Event', icon: '\u{1F4C2}' },
        { path: '/api/scalability/node-alloc-stor-total-v2493', name: 'Node Alloc Stor', icon: '\u{1F4BF}' },
        { path: '/api/scalability/priority-class-count-v2493', name: 'PriorityClass Count', icon: '\u26A1' },
        { path: '/api/product/ephemeral-storage-v2494', name: 'EphemeralStorage', icon: '\u{1F4BF}' },
        { path: '/api/product/image-id-summary-v2494', name: 'ImageID Summary', icon: '\u{1F4E6}' },
        { path: '/api/product/internal-traffic-v2494', name: 'InternalTraffic', icon: '\u{1F697}' },
        { path: '/api/deploy/rs-fully-labeled-v2495', name: 'RS FullyLabeled', icon: '\u{1F4CF}' },
        { path: '/api/deploy/sts-available-rep-v2495', name: 'STS AvailableRep', icon: '\u2705' },
        { path: '/api/deploy/ds-number-ready-v2495', name: 'DS NumberReady', icon: '\u2705' },
        { path: '/api/ops/node-kubelet-drift-v2496', name: 'Node KubeletDrift', icon: '\u26A0' },
        { path: '/api/ops/ctnr-status-summary-v2496', name: 'Ctnr Status', icon: '\u{1F4CA}' },
        { path: '/api/ops/ns-resource-quota-v2496', name: 'NS ResourceQuota', icon: '\u{1F4CB}' },
        { path: '/api/security/cap-drop-v2497', name: 'CapDrop', icon: '\u{1F6E1}' },
        { path: '/api/security/secret-helm-v2497', name: 'Secret Helm', icon: '\u{1F512}' },
        { path: '/api/security/crb-uids-v2497', name: 'CRB UIDs', icon: '\u{1F511}' },
        { path: '/api/docs/node-cap-pods-v2498', name: 'Node Cap Pods', icon: '\u{1F465}' },
        { path: '/api/docs/share-proc-ns-v2498', name: 'ShareProcNS', icon: '\u{1F517}' },
        { path: '/api/docs/ns-creation-v2498', name: 'NS Creation', icon: '\u{1F4C5}' },
        { path: '/api/scalability/top-ns-by-secret-v2499', name: 'Top NS by Secret', icon: '\u{1F511}' },
        { path: '/api/scalability/node-cpu-limit-total-v2499', name: 'Node CPULimit Total', icon: '\u{1F525}' },
        { path: '/api/scalability/hpa-total-v2499', name: 'HPA Total', icon: '\u{1F4C8}' },
        { path: '/api/product/runtime-class-v2500', name: 'RuntimeClass', icon: '\u{1F4BB}' },
        { path: '/api/product/resource-req-summary-v2500', name: 'ResourceReq Summary', icon: '\u{1F4CA}' },
        { path: '/api/product/alloc-lb-nodeports-v2500', name: 'Alloc LB NodePorts', icon: '\u{1F697}' },
        { path: '/api/deploy/rs-observed-gen-v2501', name: 'RS ObservedGen', icon: '\u{1F522}' },
        { path: '/api/deploy/sts-collision-v2501', name: 'STS Collision', icon: '\u26A0' },
        { path: '/api/deploy/ds-updated-number-v2501', name: 'DS UpdatedNum', icon: '\u{1F504}' },
        { path: '/api/ops/node-address-v2502', name: 'Node Address', icon: '\u{1F4CD}' },
        { path: '/api/ops/qos-guaranteed-v2502', name: 'QOS Guaranteed', icon: '\u2705' },
        { path: '/api/ops/last-state-v2502', name: 'Last State', icon: '\u{1F501}' },
        { path: '/api/security/selinux-v2503', name: 'SELinux', icon: '\u{1F6E1}' },
        { path: '/api/security/secret-owner-ref-v2503', name: 'Secret OwnerRef', icon: '\u{1F511}' },
        { path: '/api/security/cr-agg-verbs-v2503', name: 'CR AggVerbs', icon: '\u{1F4DD}' },
        { path: '/api/docs/node-system-uuid-v2504', name: 'Node SystemUUID', icon: '\u{1F4BB}' },
        { path: '/api/docs/set-hostname-fqdn-v2504', name: 'SetHostnameFQDN', icon: '\u{1F310}' },
        { path: '/api/docs/ns-annotation-v2504', name: 'NS Annotation', icon: '\u{1F3F7}' },
        { path: '/api/scalability/top-ns-by-deploy-v2505', name: 'Top NS by Deploy', icon: '\u{1F4C8}' },
        { path: '/api/scalability/node-mem-alloc-total-v2505', name: 'Node MemAlloc', icon: '\u{1F4BE}' },
        { path: '/api/scalability/pv-phase-dist-v2505', name: 'PV Phase Dist', icon: '\u{1F4BF}' },
        { path: '/api/product/preemption-policy-v2506', name: 'PreemptionPolicy', icon: '\u26A1' },
        { path: '/api/product/image-registry-domain-v2506', name: 'Registry Domain', icon: '\u{1F4E6}' },
        { path: '/api/product/external-ips-v2506', name: 'ExternalIPs', icon: '\u{1F310}' },
        { path: '/api/deploy/rs-template-gen-v2507', name: 'RS TemplateGen', icon: '\u{1F522}' },
        { path: '/api/deploy/sts-replicas-vs-ready-v2507', name: 'STS Rep vs Ready', icon: '\u{1F4CF}' },
        { path: '/api/deploy/ds-num-unavail-v2507', name: 'DS NumUnavail', icon: '\u26A0' },
        { path: '/api/ops/node-cap-cpu-v2508', name: 'Node Cap CPU', icon: '\u26A1' },
        { path: '/api/ops/pod-hostnet-count-v2508', name: 'Pod HostNet', icon: '\u{1F310}' },
        { path: '/api/ops/volume-device-v2508', name: 'VolumeDevice', icon: '\u{1F4BF}' },
        { path: '/api/security/cap-add-summary-v2509', name: 'CapAdd Summary', icon: '\u{1F6E1}' },
        { path: '/api/security/secret-type-full-v2509', name: 'Secret Type Full', icon: '\u{1F512}' },
        { path: '/api/security/cr-resource-v2509', name: 'CR Resource', icon: '\u{1F4DD}' },
        { path: '/api/docs/node-ver-compare-v2510', name: 'Node Ver Compare', icon: '\u{1F4BB}' },
        { path: '/api/docs/active-deadline-v2510', name: 'ActiveDeadline', icon: '\u23F1' },
        { path: '/api/docs/ns-finalizer-list-v2510', name: 'NS Finalizer List', icon: '\u{1F9F9}' },
        { path: '/api/scalability/top-ns-by-rs-v2511', name: 'Top NS by RS', icon: '\u{1F4C8}' },
        { path: '/api/scalability/node-mem-limit-total-v2511', name: 'Node MemLimit', icon: '\u{1F4BE}' },
        { path: '/api/scalability/lease-count-v2511', name: 'Lease Count', icon: '\u{1F511}' },
        { path: '/api/product/pod-os-v2512', name: 'Pod OS', icon: '\u{1F4BB}' },
        { path: '/api/product/image-versioned-tag-v2512', name: 'Versioned Tag', icon: '\u{1F4E6}' },
        { path: '/api/product/lb-source-ranges-v2512', name: 'LB SourceRanges', icon: '\u{1F6E1}' },
        { path: '/api/deploy/rs-ready-ratio-v2513', name: 'RS ReadyRatio', icon: '\u{1F4CF}' },
        { path: '/api/deploy/sts-gen-observed-v2513', name: 'STS GenObserved', icon: '\u{1F522}' },
        { path: '/api/deploy/ds-unavail-detail-v2513', name: 'DS Unavail Detail', icon: '\u26A0' },
        { path: '/api/ops/node-alloc-cpu-v2514', name: 'Node Alloc CPU', icon: '\u26A1' },
        { path: '/api/ops/pod-completed-count-v2514', name: 'Pod Completed', icon: '\u2705' },
        { path: '/api/ops/ctnr-restart-total-v2514', name: 'Ctnr Restart Total', icon: '\u{1F501}' },
        { path: '/api/security/fsgroup-change-v2515', name: 'FSGroupChange', icon: '\u{1F510}' },
        { path: '/api/security/secret-annotation-v2515', name: 'Secret Annotation', icon: '\u{1F3F7}' },
        { path: '/api/security/rb-roleref-name-v2515', name: 'RB RoleRef Name', icon: '\u{1F511}' },
        { path: '/api/docs/node-os-v2516', name: 'Node OS', icon: '\u{1F4BB}' },
        { path: '/api/docs/ctnr-vs-initctnr-v2516', name: 'Ctnr vs InitCtnr', icon: '\u{1F4E6}' },
        { path: '/api/docs/ns-uid-v2516', name: 'NS UID', icon: '\u{1F194}' },
        { path: '/api/scalability/top-ns-by-sts-v2517', name: 'Top NS by STS', icon: '\u{1F4C8}' },
        { path: '/api/scalability/node-pod-vs-cap-v2517', name: 'Node Pod vs Cap', icon: '\u{1F465}' },
        { path: '/api/scalability/controller-rev-v2517', name: 'ControllerRev', icon: '\u{1F504}' },
        { path: '/api/product/hostname-vs-node-v2518', name: 'Hostname vs Node', icon: '\u{1F4CD}' },
        { path: '/api/product/image-layer-v2518', name: 'Image Layer', icon: '\u{1F4E6}' },
        { path: '/api/product/hc-nodeport-v2518', name: 'HC NodePort', icon: '\u{1F689}' },
        { path: '/api/deploy/rs-conditions-v2519', name: 'RS Conditions', icon: '\u{1F4CB}' },
        { path: '/api/deploy/sts-current-rep-v2519', name: 'STS CurrentRep', icon: '\u{1F4CF}' },
        { path: '/api/deploy/ds-conditions-v2519', name: 'DS Conditions', icon: '\u{1F4CB}' },
        { path: '/api/ops/node-conditions-v2520', name: 'Node Conditions', icon: '\u2705' },
        { path: '/api/ops/pod-failed-count-v2520', name: 'Pod Failed', icon: '\u274C' },
        { path: '/api/ops/exit-code-v2520', name: 'Exit Code', icon: '\u{1F4DD}' },
        { path: '/api/security/seccomp-onroot-v2521', name: 'Seccomp OnRoot', icon: '\u{1F6E1}' },
        { path: '/api/security/secret-owner-kind-v2521', name: 'Secret OwnerKind', icon: '\u{1F511}' },
        { path: '/api/security/cr-non-resource-v2521', name: 'CR NonResource', icon: '\u{1F517}' },
        { path: '/api/docs/runtime-vs-kubelet-v2522', name: 'Runtime vs Kubelet', icon: '\u{1F4BB}' },
        { path: '/api/docs/pod-sa-v2522', name: 'Pod SA', icon: '\u{1F465}' },
        { path: '/api/docs/ns-deletion-v2522', name: 'NS Deletion', icon: '\u{1F5D1}' },
        { path: '/api/scalability/top-node-mem-req-v2523', name: 'Top Node MemReq', icon: '\u{1F4A7}' },
        { path: '/api/scalability/node-pods-alloc-ratio-v2523', name: 'Node Pods AllocRatio', icon: '\u{1F4CA}' },
        { path: '/api/scalability/job-total-v2523', name: 'Job Total', icon: '\u{1F4CB}' },
        { path: '/api/product/enable-service-links-v2524', name: 'EnableServiceLinks', icon: '\u{1F517}' },
        { path: '/api/product/cpu-limit-summary-v2524', name: 'CPULimit Summary', icon: '\u26A1' },
        { path: '/api/product/clusterips-count-v2524', name: 'ClusterIPs Count', icon: '\u{1F310}' },
        { path: '/api/deploy/rs-available-rep-v2525', name: 'RS AvailableRep', icon: '\u{1F4CF}' },
        { path: '/api/deploy/sts-updated-rep-v2525', name: 'STS UpdatedRep', icon: '\u{1F504}' },
        { path: '/api/deploy/ds-misscheduled-detail-v2525', name: 'DS Misscheduled', icon: '\u26A0' },
        { path: '/api/ops/node-heartbeat-v2526', name: 'Node Heartbeat', icon: '\u{1F493}' },
        { path: '/api/ops/pod-pending-count-v2526', name: 'Pod Pending', icon: '\u23F3' },
        { path: '/api/ops/term-reason-v2526', name: 'Term Reason', icon: '\u{1F4DD}' },
        { path: '/api/security/proc-mount-v2527', name: 'ProcMount', icon: '\u{1F6E1}' },
        { path: '/api/security/secret-age-v2527', name: 'Secret Age', icon: '\u{1F4C5}' },
        { path: '/api/security/rb-verbs-total-v2527', name: 'RB Verbs Total', icon: '\u{1F4DD}' },
        { path: '/api/docs/nodeinfo-compare-v2528', name: 'NodeInfo Compare', icon: '\u{1F4BB}' },
        { path: '/api/docs/host-aliases-detail-v2528', name: 'HostAliases Detail', icon: '\u{1F4CD}' },
        { path: '/api/docs/ns-label-key-v2528', name: 'NS LabelKey', icon: '\u{1F3F7}' },
        { path: '/api/scalability/top-ns-by-ingress-v2529', name: 'Top NS by Ingress', icon: '\u{1F517}' },
        { path: '/api/scalability/node-cpu-alloc-vs-limit-v2529', name: 'Node CPU vs Limit', icon: '\u26A1' },
        { path: '/api/scalability/cronjob-total-v2529', name: 'CronJob Total', icon: '\u{1F552}' },
        { path: '/api/product/term-grace-dist-v2530', name: 'TermGrace Dist', icon: '\u23F1' },
        { path: '/api/product/ephemeral-limit-v2530', name: 'EphemeralLimit', icon: '\u{1F4BF}' },
        { path: '/api/product/ipfamily-policy-detail-v2530', name: 'IPFamilyPolicy Detail', icon: '\u{1F310}' },
        { path: '/api/deploy/rs-status-detail-v2531', name: 'RS Status Detail', icon: '\u{1F4CF}' },
        { path: '/api/deploy/sts-replicas-detail-v2531', name: 'STS Replicas Detail', icon: '\u{1F4CF}' },
        { path: '/api/deploy/ds-observed-gen-v2531', name: 'DS ObservedGen', icon: '\u{1F522}' },
        { path: '/api/ops/node-alloc-mem-v2532', name: 'Node AllocMem', icon: '\u{1F4BE}' },
        { path: '/api/ops/pod-volumes-v2532', name: 'Pod Volumes', icon: '\u{1F4BF}' },
        { path: '/api/ops/mem-req-summary-v2532', name: 'MemReq Summary', icon: '\u{1F4CA}' },
        { path: '/api/security/windows-gmsa-v2533', name: 'Windows GMSA', icon: '\u{1F510}' },
        { path: '/api/security/secret-creation-rate-v2533', name: 'Secret Creation Rate', icon: '\u{1F4C5}' },
        { path: '/api/security/crb-verbs-summary-v2533', name: 'CRB Verbs Summary', icon: '\u{1F4DD}' },
        { path: '/api/docs/node-kernel-detail-v2534', name: 'Node Kernel Detail', icon: '\u{1F4BB}' },
        { path: '/api/docs/pod-priority-detail-v2534', name: 'Pod Priority Detail', icon: '\u26A1' },
        { path: '/api/docs/ns-resource-version-v2534', name: 'NS ResourceVer', icon: '\u{1F522}' },
        { path: '/api/scalability/top-ns-by-ds-v2535', name: 'Top NS by DS', icon: '\u{1F4C8}' },
        { path: '/api/scalability/node-mem-cap-vs-alloc-v2535', name: 'Node MemCap vs Alloc', icon: '\u{1F4BE}' },
        { path: '/api/scalability/event-type-dist-v2535', name: 'Event Type Dist', icon: '\u{1F4C2}' },
        { path: '/api/product/cpu-req-summary-v2536', name: 'CPUReq Summary', icon: '\u26A1' },
        { path: '/api/product/volume-mount-detail-v2536', name: 'VolumeMount Detail', icon: '\u{1F4BF}' },
        { path: '/api/product/service-ports-summary-v2536', name: 'Svc Ports Summary', icon: '\u{1F4F1}' },
        { path: '/api/deploy/rs-label-selector-v2537', name: 'RS LabelSelector', icon: '\u{1F4CD}' },
        { path: '/api/deploy/sts-current-rev-v2537', name: 'STS CurrentRev', icon: '\u{1F522}' },
        { path: '/api/deploy/ds-updated-vs-desired-v2537', name: 'DS Updated vs Desired', icon: '\u{1F504}' },
        { path: '/api/ops/node-alloc-pods-detail-v2538', name: 'Node AllocPods', icon: '\u{1F465}' },
        { path: '/api/ops/pod-cpu-limit-total-v2538', name: 'Pod CPULimit Total', icon: '\u26A1' },
        { path: '/api/ops/running-state-detail-v2538', name: 'Running State Detail', icon: '\u2705' },
        { path: '/api/security/cap-add-vs-drop-v2539', name: 'CapAdd vs CapDrop', icon: '\u{1F6E1}' },
        { path: '/api/security/secret-type-vs-keys-v2539', name: 'Secret Type vs Keys', icon: '\u{1F512}' },
        { path: '/api/security/rb-user-vs-group-v2539', name: 'RB User vs Group', icon: '\u{1F465}' },
        { path: '/api/docs/node-alloc-vs-cap-cpu-v2540', name: 'Node Alloc vs Cap CPU', icon: '\u26A1' },
        { path: '/api/docs/dns-policy-detail-v2540', name: 'DNSPolicy Detail', icon: '\u{1F310}' },
        { path: '/api/docs/ns-finalizer-summary-v2540', name: 'NS Finalizer Summary', icon: '\u{1F9F9}' },
        { path: '/api/scalability/top-ns-by-svc-2541', name: 'Top NS by Svc', icon: '\u{1F4F1}' },
        { path: '/api/scalability/node-mem-usage-ratio-v2541', name: 'Node MemUsage Ratio', icon: '\u{1F4CA}' },
        { path: '/api/scalability/pdb-count-v2541', name: 'PDB Count', icon: '\u{1F6E1}' },
        { path: '/api/product/scheduler-name-dist-v2542', name: 'SchedulerName Dist', icon: '\u{1F4C5}' },
        { path: '/api/product/mem-limit-summary-v2542', name: 'MemLimit Summary', icon: '\u{1F4BE}' },
        { path: '/api/product/service-selector-v2542', name: 'Svc Selector', icon: '\u{1F50D}' },
        { path: '/api/deploy/rs-replicas-vs-ready-v2543', name: 'RS Rep vs Ready', icon: '\u{1F4CF}' },
        { path: '/api/deploy/sts-update-rev-v2543', name: 'STS UpdateRev', icon: '\u{1F522}' },
        { path: '/api/deploy/ds-obs-vs-gen-v2543', name: 'DS Obs vs Gen', icon: '\u26A0' },
        { path: '/api/ops/node-addr-detail-v2544', name: 'Node Addr Detail', icon: '\u{1F4CD}' },
        { path: '/api/ops/pod-host-aliases-count-v2544', name: 'Pod HostAliases', icon: '\u{1F4CD}' },
        { path: '/api/ops/read-only-mount-v2544', name: 'ReadOnly Mount', icon: '\u{1F4BF}' },
        { path: '/api/security/apparmor-v2545', name: 'AppArmor', icon: '\u{1F6E1}' },
        { path: '/api/security/secret-max-age-v2545', name: 'Secret MaxAge', icon: '\u{1F4C5}' },
        { path: '/api/security/crb-resource-names-v2545', name: 'CRB ResourceNames', icon: '\u{1F4DD}' },
        { path: '/api/docs/node-feature-labels-v2546', name: 'Node FeatureLabels', icon: '\u{1F4CB}' },
        { path: '/api/docs/resource-claim-v2546', name: 'ResourceClaim', icon: '\u{1F4B0}' },
        { path: '/api/docs/ns-uid-dist-v2546', name: 'NS UID Dist', icon: '\u{1F194}' },
        { path: '/api/scalability/top-ns-by-cm2-v2547', name: 'Top NS by CM v2', icon: '\u{1F4D1}' },
        { path: '/api/scalability/node-pod-usage-ratio-v2547', name: 'Node PodUsage Ratio', icon: '\u{1F4CA}' },
        { path: '/api/scalability/rs-total-v2547', name: 'RS Total', icon: '\u{1F4CF}' },
        { path: '/api/product/pod-os-name-v2548', name: 'Pod OSName', icon: '\u{1F4BB}' },
        { path: '/api/product/req-vs-limit-v2548', name: 'Req vs Limit', icon: '\u{1F4CA}' },
        { path: '/api/product/service-type-summary-v2548', name: 'Svc Type Summary', icon: '\u{1F697}' },
        { path: '/api/deploy/rs-spec-replicas-v2549', name: 'RS SpecReplicas', icon: '\u{1F4CF}' },
        { path: '/api/deploy/sts-rolling-update-v2549', name: 'STS RollingUpd', icon: '\u{1F504}' },
        { path: '/api/deploy/ds-number-avail-v2549', name: 'DS NumAvail', icon: '\u2705' },
        { path: '/api/ops/node-cap-mem-v2550', name: 'Node CapMem', icon: '\u{1F4BE}' },
        { path: '/api/ops/pod-priority-summary-v2550', name: 'Pod Priority Summary', icon: '\u26A1' },
        { path: '/api/ops/resource-summary-v2550', name: 'Resource Summary', icon: '\u{1F4CA}' },
        { path: '/api/security/runas-user-detail-v2551', name: 'RunAsUser Detail', icon: '\u{1F510}' },
        { path: '/api/security/secret-ns-dist-v2551', name: 'Secret NSDist', icon: '\u{1F512}' },
        { path: '/api/security/rb-rules-summary-v2551', name: 'RB Rules Summary', icon: '\u{1F4DD}' },
        { path: '/api/docs/node-pods-vs-cap-v2552', name: 'Node Pods vs Cap', icon: '\u{1F465}' },
        { path: '/api/docs/restart-policy-v2552', name: 'RestartPolicy', icon: '\u{1F501}' },
        { path: '/api/docs/ns-annot-key-v2552', name: 'NS AnnotKey', icon: '\u{1F3F7}' },
        { path: '/api/scalability/top-ns-by-evt-v2553', name: 'Top NS by Evt', icon: '\u{1F4C2}' },
        { path: '/api/scalability/node-stor-alloc-v2553', name: 'Node StorAlloc', icon: '\u{1F4BF}' },
        { path: '/api/scalability/sts-total-v2553', name: 'STS Total', icon: '\u{1F4CF}' },
        { path: '/api/product/sa-dist-v2554', name: 'SA Dist', icon: '\u{1F465}' },
        { path: '/api/product/cpu-req-container-v2554', name: 'CPUReq Container', icon: '\u26A1' },
        { path: '/api/product/service-port-range-v2554', name: 'Svc Port Range', icon: '\u{1F4F1}' },
        { path: '/api/deploy/rs-owner-detail-v2555', name: 'RS Owner Detail', icon: '\u{1F517}' },
        { path: '/api/deploy/sts-spec-rep-total-v2555', name: 'STS SpecRep Total', icon: '\u{1F4CF}' },
        { path: '/api/deploy/ds-gen-summary-v2555', name: 'DS Gen Summary', icon: '\u{1F522}' },
        { path: '/api/ops/node-mem-vs-cap-v2556', name: 'Node Mem vs Cap', icon: '\u{1F4BE}' },
        { path: '/api/ops/pod-vol-count-v2556', name: 'Pod Vol Count', icon: '\u{1F4BF}' },
        { path: '/api/ops/liveness-probe-v2556', name: 'Liveness Probe', icon: '\u{1F50C}' },
        { path: '/api/security/privileged-container-v2557', name: 'Privileged Container', icon: '\u26A0' },
        { path: '/api/security/secret-type-detail-v2557', name: 'Secret Type Detail', icon: '\u{1F512}' },
        { path: '/api/security/cr-api-groups-v2557', name: 'CR APIGroups', icon: '\u{1F4DD}' },
        { path: '/api/docs/node-kernel-dist-v2558', name: 'Node Kernel Dist', icon: '\u{1F4BB}' },
        { path: '/api/docs/host-pid-ipc-v2558', name: 'HostPID HostIPC', icon: '\u26A0' },
        { path: '/api/docs/ns-creation-time-v2558', name: 'NS Creation Time', icon: '\u{1F4C5}' },
        { path: '/api/scalability/top-ns-by-sts-rep-v2559', name: 'Top NS by STSRep', icon: '\u{1F4C8}' },
        { path: '/api/scalability/node-cpu-cap-detail-v2559', name: 'Node CPUCap Detail', icon: '\u26A1' },
        { path: '/api/scalability/deploy-total-v2559', name: 'Deploy Total', icon: '\u{1F4C8}' },
        { path: '/api/product/term-grace-summary-v2560', name: 'TermGrace Summary', icon: '\u23F1' },
        { path: '/api/product/mem-limit-container-v2560', name: 'MemLimit Container', icon: '\u{1F4BE}' },
        { path: '/api/product/clusterip-none-v2560', name: 'Headless Services', icon: '\u{1F697}' },
        { path: '/api/deploy/rs-paused-v2561', name: 'RS Paused', icon: '\u23F8' },
        { path: '/api/deploy/sts-partition-v2561', name: 'STS Partition', icon: '\u26AB' },
        { path: '/api/deploy/ds-deletion-v2561', name: 'DS Deletion', icon: '\u{1F5D1}' },
        { path: '/api/ops/node-alloc-vs-running-v2562', name: 'Node Alloc vs Running', icon: '\u{1F4CA}' },
        { path: '/api/ops/pod-volume-size-v2562', name: 'Pod Volume Size', icon: '\u{1F4BF}' },
        { path: '/api/ops/startup-probe-v2562', name: 'Startup Probe', icon: '\u{1F50C}' },
        { path: '/api/security/host-pid-detail-v2563', name: 'HostPID Detail', icon: '\u26A0' },
        { path: '/api/security/secret-immutable-v2563', name: 'Secret Immutable', icon: '\u{1F512}' },
        { path: '/api/security/rb-subjects-count-v2563', name: 'RB Subjects Count', icon: '\u{1F465}' },
        { path: '/api/docs/node-cap-vs-alloc-stor-v2564', name: 'Node Cap vs Alloc Stor', icon: '\u{1F4BF}' },
        { path: '/api/docs/node-selector-detail-v2564', name: 'NodeSelector Detail', icon: '\u{1F50D}' },
        { path: '/api/docs/ns-label-vs-annot-v2564', name: 'NS Label vs Annot', icon: '\u{1F3F7}' },
        { path: '/api/scalability/top-ns-by-pvc2-v2565', name: 'Top NS by PVC v2', icon: '\u{1F4BF}' },
        { path: '/api/scalability/node-mem-alloc-detail-v2565', name: 'Node MemAlloc Detail', icon: '\u{1F4BE}' },
        { path: '/api/scalability/job-active-v2565', name: 'Job Active', icon: '\u{1F4CB}' },
        { path: '/api/product/priority-class-v2115', name: 'Priority Class', icon: '\u26A1' },

        { path: '/api/docs/annotation-report', name: 'Annot Report', icon: '\u{1F4DD}' },
        { path: '/api/docs/topology-map-v2', name: 'Topology Map v2', icon: '\u{1F5FA}' },
        { path: '/api/docs/storage-attachment-inv', name: 'Storage Attach', icon: '\u{1F4BF}' },
        { path: '/api/docs/port-catalog', name: 'Port Catalog', icon: '\u{1F50C}' },
        { path: '/api/docs/rbac-cheatsheet', name: 'RBAC Cheatsheet', icon: '\u{1F511}' },
        { path: '/api/docs/cluster-blueprint', name: 'Cluster Blueprint', icon: '\u{1F3D7}' },
        { path: '/api/docs/ingress-catalog', name: 'Ingress Catalog', icon: '\u{1F310}' },
        { path: '/api/docs/network-policy-catalog', name: 'NetPol Catalog', icon: '\u{1F6E1}' },
        { path: '/api/docs/label-inventory', name: 'Label Inventory', icon: '\u{1F3F7}' },
        { path: '/api/docs/configmap-catalog', name: 'CM Catalog', icon: '\u{1F4DD}' },
        { path: '/api/docs/hpa-catalog', name: 'HPA Catalog', icon: '\u2195' },
        { path: '/api/docs/pdb-catalog', name: 'PDB Catalog', icon: '\u{1F6E1}' },
        { path: '/api/docs/secret-inventory', name: 'Secret Inv', icon: '\u{1F512}' },
        { path: '/api/docs/service-account-inventory', name: 'SA Inv', icon: '\u{1F464}' },
        { path: '/api/docs/event-type-catalog', name: 'Event Types', icon: '\u{1F4CB}' },
        { path: '/api/docs/node-taint-catalog', name: 'Taint Catalog', icon: '\u26D4' },
        { path: '/api/docs/volume-snapshot-catalog', name: 'Snap Catalog', icon: '\u{1F4F7}' },
        { path: '/api/docs/storage-class-catalog', name: 'SC Catalog', icon: '\u{1F4BE}' },
        { path: '/api/docs/priority-class-catalog', name: 'PC Catalog', icon: '\u26A1' },
        { path: '/api/docs/role-binding-catalog', name: 'RB Catalog', icon: '\u{1F511}' },
        { path: '/api/docs/endpoint-slice-catalog', name: 'EP Slice', icon: '\u{1F310}' },
        { path: '/api/docs/topology-spread-catalog', name: 'Topo Spread', icon: '\u{1F5FA}' },
        { path: '/api/docs/limitrange-catalog', name: 'LimitRange', icon: '\u2696' },
        { path: '/api/docs/lease-holder-catalog', name: 'Lease Catalog', icon: '\u{1F527}' },
        { path: '/api/docs/runtime-class-inventory', name: 'RC Inv', icon: '\u{1F9E9}' },
        { path: '/api/docs/ingress-backend-catalog', name: 'Ing Backend', icon: '\u{1F310}' },
        { path: '/api/docs/csi-driver-inventory', name: 'CSI Driver', icon: '\u{1F4BE}' },
        { path: '/api/docs/pod-disruption-coverage', name: 'PDB Coverage', icon: '\u{1F6E1}' },
        { path: '/api/docs/csi-snapshot-class-inventory', name: 'SnapClass', icon: '\u{1F4F7}' },
        { path: '/api/docs/mutating-webhook-catalog', name: 'Mut Webhook', icon: '\u{1F50C}' },
        { path: '/api/docs/validating-webhook-config-inventory', name: 'ValWH Config', icon: '\u2705' },
        { path: '/api/docs/ingress-class-inventory', name: 'Ing Class', icon: '\u{1F6A7}' },
        { path: '/api/docs/apiservice-registration-catalog', name: 'APISvc Reg', icon: '\u{1F527}' },
        { path: '/api/docs/storage-class-binding-mode', name: 'SC BindMode', icon: '\u{1F4BE}' },
        { path: '/api/docs/crd-version-catalog', name: 'CRD Ver', icon: '\u{1F9E9}' },
        { path: '/api/docs/prioritylevel-config-catalog', name: 'PrioLevel', icon: '\u26A1' },
        { path: '/api/product/scale-limit-analysis', name: 'Scale Limits', icon: '\u2195' },
        { path: '/api/product/cm-key-exposure', name: 'CM Key Exposure', icon: '\u{1F511}' },
        { path: '/api/product/pvc-access-pattern', name: 'PVC Access', icon: '\u{1F4BE}' },
        { path: '/api/product/workload-insights', name: 'WL Insights', icon: '\u{1F4CA}' },
        { path: '/api/product/storage-summary', name: 'Storage Summary', icon: '\u{1F4E6}' },
        { path: '/api/product/network-topology-insights', name: 'Net Topology', icon: '\u{1F310}' },
        { path: '/api/product/pod-efficiency-score', name: 'Pod Efficiency', icon: '\u{1F4CA}' },
        { path: '/api/product/service-health-overview', name: 'Svc Health', icon: '\u2764' },
        { path: '/api/product/cluster-utilization-summary', name: 'Cluster Util', icon: '\u{1F4CF}' },
        { path: '/api/product/pod-uptime-tracker', name: 'Pod Uptime', icon: '\u23F1' },
        { path: '/api/product/namespace-cost-summary', name: 'NS Cost', icon: '\u{1F4B0}' },
        { path: '/api/product/replica-health-summary', name: 'Replica Health', icon: '\u2705' },
        { path: '/api/product/pod-density-score', name: 'Pod Density', icon: '\u{1F4CF}' },
        { path: '/api/product/image-cache-efficiency', name: 'Image Cache', icon: '\u{1F4F7}' },
        { path: '/api/product/node-bin-packing', name: 'Bin Packing', icon: '\u{1F4E6}' },
        { path: '/api/product/statefulset-health', name: 'STS Health', icon: '\u{1F4E5}' },
        { path: '/api/product/daemonset-coverage', name: 'DS Coverage', icon: '\u{1F5C2}' },
        { path: '/api/product/job-completion-rate', name: 'Job Rate', icon: '\u2705' },
        { path: '/api/product/cpu-throttle-est', name: 'CPU Throttle', icon: '\u23F1' },
        { path: '/api/product/image-layer-dedup', name: 'Img Dedup', icon: '\u{1F4F7}' },
        { path: '/api/product/pod-scheduling-latency', name: 'Sched Latency', icon: '\u23F3' },
        { path: '/api/product/vol-lifecycle-age', name: 'Vol Lifecycle', icon: '\u{1F4BE}' },
        { path: '/api/product/service-endpoint-health', name: 'Svc EP Health', icon: '\u{1F310}' },
        { path: '/api/product/image-tag-freshness', name: 'Img Tag Fresh', icon: '\u{1F523}' },
        { path: '/api/product/pod-restart-trend', name: 'Restart Trend', icon: '\u{1F501}' },
        { path: '/api/product/deployment-rollout-status', name: 'Rollout Status', icon: '\u{1F504}' },
        { path: '/api/product/pvc-binding-health', name: 'PVC Binding', icon: '\u{1F4BE}' },
        { path: '/api/product/hpa-target-utilization', name: 'HPA Target', icon: '\u{1F4C8}' },
        { path: '/api/product/replica-age-distribution', name: 'Replica Age', icon: '\u{1F4C5}' },
        { path: '/api/product/node-pod-affinity-score', name: 'Node Affinity', icon: '\u2696' },
        { path: '/api/product/pod-network-policy-match', name: 'Pod NetPol', icon: '\u{1F6E1}' },
        { path: '/api/product/deployment-maxunavailable', name: 'MaxUnavail', icon: '\u{1F4C8}' },
        { path: '/api/product/container-image-pull-policy', name: 'Pull Policy', icon: '\u2B07' },
        { path: '/api/product/pvc-resize-tracking', name: 'PVC Resize', icon: '\u{1F4CF}' },
        { path: '/api/product/service-type-distribution', name: 'Svc Type', icon: '\u{1F310}' },
        { path: '/api/product/pod-qos-distribution', name: 'QoS Dist', icon: '\u{1F3AF}' },
        { path: '/api/scalability/restart-rate', name: 'Restart Rate', icon: '\u{1F501}' },
        { path: '/api/scalability/node-affinity-compliance', name: 'Node Affinity', icon: '\u{1F4CD}' },
        { path: '/api/scalability/quota-pressure-index', name: 'Quota Pressure', icon: '\u{1F4CA}' },
        { path: '/api/scalability/autoscaler-readiness-v2', name: 'Autoscaler Ready', icon: '\u2195' },
        { path: '/api/scalability/failover-readiness', name: 'Failover Ready', icon: '\u{1F6E1}' },
        { path: '/api/scalability/api-object-count', name: 'Object Count', icon: '\u{1F4CF}' },
        { path: '/api/scalability/watch-cache-pressure', name: 'Watch Cache', icon: '\u{1F441}' },
        { path: '/api/scalability/scheduler-cache-health', name: 'Sched Cache', icon: '\u23F1' },
        { path: '/api/scalability/conntrack-capacity', name: 'Conntrack', icon: '\u{1F517}' },
        { path: '/api/scalability/ip-pool-health', name: 'IP Pool', icon: '\u{1F310}' },
        { path: '/api/scalability/resource-version-staleness', name: 'Res Version', icon: '\u{1F501}' },
        { path: '/api/scalability/node-allocatable-gap', name: 'Alloc Gap', icon: '\u{1F4CF}' },
        { path: '/api/scalability/pod-overhead-ratio', name: 'Pod Overhead', icon: '\u{1F4CA}' },
        { path: '/api/scalability/api-server-qps-est', name: 'API QPS Est', icon: '\u{1F525}' },
        { path: '/api/scalability/pod-topology-skew', name: 'Topo Skew', icon: '\u{1F5FA}' },
        { path: '/api/scalability/iptables-size', name: 'IPTables Size', icon: '\u2699' },
        { path: '/api/scalability/etcd-compaction', name: 'ETCD Compact', icon: '\u{1F50C}' },
        { path: '/api/scalability/kubelet-pod-limit', name: 'Pod Limit', icon: '\u{1F4CF}' },
        { path: '/api/scalability/dns-query-pressure', name: 'DNS Pressure', icon: '\u{1F310}' },
        { path: '/api/scalability/cni-ipam-capacity', name: 'CNI IPAM', icon: '\u{1F517}' },
        { path: '/api/scalability/control-plane-load', name: 'CP Load', icon: '\u{1F680}' },
        { path: '/api/scalability/volume-attach-density', name: 'Vol Attach', icon: '\u{1F4BE}' },
        { path: '/api/scalability/ns-quota-utilization', name: 'Quota Util', icon: '\u{1F4CA}' },
        { path: '/api/scalability/control-plane-ha', name: 'CP HA', icon: '\u{1F6E1}' },
        { path: '/api/scalability/anti-affinity-coverage', name: 'Anti-Affinity', icon: '\u{1F5FA}' },
        { path: '/api/scalability/request-headroom', name: 'Req Headroom', icon: '\u{1F4CF}' },
        { path: '/api/scalability/node-cordone-readiness', name: 'Cordon Ready', icon: '\u{1F6DE}' },
        { path: '/api/scalability/pv-reclaim-gap', name: 'PV Reclaim', icon: '\u{1F9F9}' },
        { path: '/api/scalability/cluster-object-budget', name: 'Obj Budget', icon: '\u{1F4CA}' },
        { path: '/api/scalability/etcd-db-size-estimator', name: 'ETCD Size', icon: '\u{1F4BD}' },
        { path: '/api/scalability/scheduler-cache-pressure', name: 'Sched Cache', icon: '\u{1F9E0}' },
        { path: '/api/scalability/apiserver-request-latency', name: 'API Latency', icon: '\u23F1' },
        { path: '/api/scalability/cluster-pod-density-trend', name: 'Pod Density', icon: '\u{1F4CF}' },
        { path: '/api/scalability/service-mesh-endpoint-budget', name: 'Mesh EP', icon: '\u{1F310}' },
        { path: '/api/scalability/node-allocatable-headroom', name: 'Alloc Head', icon: '\u{1F4CA}' },
        { path: '/api/scalability/cluster-label-cardinality', name: 'Label Card', icon: '\u{1F3F7}' },
        { path: '/api/scalability/configmap-size-budget', name: 'CM Size', icon: '\u{1F4D0}' },
        { path: '/api/scalability/epslice-address-budget', name: 'EPS Addr', icon: '\u{1F310}' },
        { path: '/api/deployment/sts-health', name: 'STS Health', icon: '\u{1F4E6}' },
        { path: '/api/deployment/image-pull-secret-gap', name: 'Pull Secret Gap', icon: '\u{1F511}' },
        { path: '/api/deployment/topology-distribution', name: 'Topology Dist', icon: '\u{1F5FA}' },
      ],
    },
  },
  'Deployment': {
    color: '#3fb950',
    icon: '\u{1F680}',
    subcategories: {
      'GitOps & Helm': [
        { path: '/api/deployment/helm-drift-monitor', name: 'Helm Drift', icon: '\u{1F4E6}' },
        { path: '/api/deployment/gitops-audit', name: 'GitOps Audit', icon: '\u{1F4C1}' },
        { path: '/api/deployment/gitops-sync-deep', name: 'GitOps Sync', icon: '\u{1F504}' },
      ],
      'Rollout & Progressive': [
        { path: '/api/deployment/progressive-delivery', name: 'Progressive Delivery', icon: '\u{1F4C8}' },
        { path: '/api/deployment/rollout-health', name: 'Rollout Health', icon: '\u2728' },
        { path: '/api/deployment/update-strategy', name: 'Update Strategy', icon: '\u{1F504}' },
        { path: '/api/deployment/surge-capacity', name: 'Surge Capacity', icon: '\u26A1' },
      ],
      'Rollback Safety': [
        { path: '/api/deployment/rollback-risk', name: 'Rollback Risk', icon: '\u21A9' },
        { path: '/api/deployment/rollback-safety', name: 'Rollback Safety', icon: '\u21A9' },
        { path: '/api/deployment/rollback-simulator', name: 'Rollback Simulator', icon: '\u21A9' },
      ],
      'Image Management': [
        { path: '/api/deployment/image-hygiene', name: 'Image Hygiene', icon: '\u{1F4F7}' },
        { path: '/api/deployment/image-freshness', name: 'Image Freshness', icon: '\u{1F34E}' },
        { path: '/api/deployment/image-pull-latency', name: 'Image Pull Latency', icon: '\u{1F4F7}' },
        { path: '/api/deployment/image-pull-audit', name: 'Image Pull Audit', icon: '\u{1F4F7}' },
      ],
      'Readiness & Gates': [
        { path: '/api/deployment/readiness-gate', name: 'Readiness Gate', icon: '\u2705' },
        { path: '/api/deployment/deploy-window', name: 'Deploy Window', icon: '\u{1F4C5}' },
        { path: '/api/deployment/change-freeze', name: 'Change Freeze', icon: '\u2744' },
      ],
      'Sidecar & Quota': [
        { path: '/api/deployment/sidecar-injection-audit', name: 'Sidecar Injection', icon: '\u{1F916}' },
        { path: '/api/deployment/resource-quota-drift', name: 'Quota Drift', icon: '\u2696' },
        { path: '/api/deployment/resource-limit-coverage', name: 'Limit Coverage', icon: '\u2696' },
        { path: '/api/deployment/ephemeral-storage-quota', name: 'Ephemeral Storage', icon: '\u{1F4BE}' },
      ],
      'DORA Metrics': [
        { path: '/api/deployment/dora-metrics', name: 'DORA Metrics', icon: '\u{1F4C8}' },
        { path: '/api/deployment/deploy-frequency', name: 'Deploy Frequency', icon: '\u{1F4C8}' },
        { path: '/api/deployment/deploy-heatmap', name: 'Deploy Heatmap', icon: '\u{1F525}' },
      ],
      'Probe Health': [
        { path: '/api/deployment/probe-compliance', name: 'Probe Compliance', icon: '\u{1FA78}' },
        { path: '/api/deployment/probe-generator', name: 'Probe Generator', icon: '\u{1F527}' },
        { path: '/api/deployment/probe-timeout-audit', name: 'Probe Timeout', icon: '\u23F1' },
        { path: '/api/deployment/init-container-health', name: 'Init Container', icon: '\u{1F9E9}' },
        { path: '/api/deployment/rollout-blocker-detect', name: 'Rollout Blocker', icon: '\u26D4' },
        { path: '/api/deployment/termination-grace-audit', name: 'Termination Grace', icon: '\u23F3' },
        { path: '/api/deployment/max-surge-audit', name: 'Max Surge', icon: '\u2191' },
        { path: '/api/deployment/graceful-shutdown', name: 'Graceful Shutdown', icon: '\u{1F6D1}' },
      ],
      'Config & Drift': [
        { path: '/api/deployment/config-sync', name: 'Config Sync', icon: '\u{1F504}' },
        { path: '/api/deployment/config-snapshot', name: 'Config Snapshot', icon: '\u{1F4F8}' },
        { path: '/api/deployment/revision-drift', name: 'Revision Drift', icon: '\u{1F501}' },
        { path: '/api/deployment/revision-history-hygiene', name: 'Revision History', icon: '\u{1F4DC}' },
        { path: '/api/deployment/env-config-drift', name: 'Env Config Drift', icon: '\u{1F501}' },
        { path: '/api/deployment/immutable-config-audit', name: 'Immutable Config', icon: '\u{1F512}' },
      ],
      'Reproducibility & Compliance': [
        { path: '/api/deployment/deploy-reproducibility', name: 'Reproducibility', icon: '\u{1F50D}' },
        { path: '/api/deployment/update-compliance-deep', name: 'Update Compliance', icon: '\u2705' },
        { path: '/api/deployment/restart-policy-deep', name: 'Restart Policy', icon: '\u{1F501}' },
        { path: '/api/deployment/graceful-shutdown-audit', name: 'Graceful Shutdown', icon: '\u{1F6D1}' },
        { path: '/api/deployment/rollout-speed', name: 'Rollout Speed', icon: '\u23F1' },
        { path: '/api/deployment/deploy-conflict', name: 'Deploy Conflict', icon: '\u26A0' },
        { path: '/api/deployment/image-consistency', name: 'Image Consist', icon: '\u{1F4F7}' },
        { path: '/api/deployment/config-reload-readiness', name: 'Config Reload', icon: '\u{1F504}' },
        { path: '/api/deployment/deploy-freeze-status', name: 'Deploy Freeze', icon: '\u2744' },
        { path: '/api/deployment/manifest-drift', name: 'Manifest Drift', icon: '\u{1F503}' },
        { path: '/api/deployment/preflight-check', name: 'Pre-Flight', icon: '\u2708' },
        { path: '/api/deployment/annotation-compliance', name: 'Annot Compliance', icon: '\u{1F4DD}' },
        { path: '/api/deployment/multi-arch-audit', name: 'Multi-Arch', icon: '\u{1F578}' },
        { path: '/api/deployment/container-deps', name: 'Container Deps', icon: '\u{1F9E9}' },
        { path: '/api/deployment/helm-health', name: 'Helm Health', icon: '\u2693' },
        { path: '/api/deployment/antiaffinity-gap', name: 'Anti-Aff Gap', icon: '\u{1F6E1}' },
        { path: '/api/deployment/command-audit', name: 'Cmd Audit', icon: '\u{1F4BB}' },
        { path: '/api/deployment/annotation-signal', name: 'Annot Signal', icon: '\u{1F4DD}' },
        { path: '/api/deployment/dns-policy-audit', name: 'DNS Policy', icon: '\u{1F310}' },
        { path: '/api/deployment/pod-priority-preempt', name: 'Pod Priority', icon: '\u26A1' },
        { path: '/api/deployment/secret-env-ref', name: 'Secret Env Ref', icon: '\u{1F511}' },
        { path: '/api/deployment/resource-request-gap', name: 'Req Gap', icon: '\u2696' },
        { path: '/api/deployment/container-port-map', name: 'Port Map', icon: '\u{1F50C}' },
        { path: '/api/deployment/termination-message-audit', name: 'Term Msg', icon: '\u{1F4DD}' },
        { path: '/api/deployment/init-container-dep', name: 'Init Dep', icon: '\u{1F9E9}' },
        { path: '/api/deployment/strategy-compliance', name: 'Strategy', icon: '\u{1F504}' },
        { path: '/api/deployment/pull-secret-coverage', name: 'Pull Secret Cov', icon: '\u{1F511}' },
        { path: '/api/deployment/node-selector-audit', name: 'Node Selector', icon: '\u{1F3AF}' },
        { path: '/api/deployment/pod-os-selector', name: 'Pod OS', icon: '\u{1F4BB}' },
        { path: '/api/deployment/container-working-dir', name: 'Work Dir', icon: '\u{1F4C2}' },
        { path: '/api/deployment/container-stdin-tty', name: 'Stdin/TTY', icon: '\u2328' },
        { path: '/api/deployment/pod-dns-config', name: 'DNS Config', icon: '\u{1F310}' },
        { path: '/api/deployment/host-alias-audit', name: 'Host Alias', icon: '\u{1F4CD}' },
        { path: '/api/deployment/restart-policy-audit', name: 'Restart Pol', icon: '\u{1F501}' },
        { path: '/api/deployment/revision-history-audit', name: 'Rev History', icon: '\u{1F4DC}' },
        { path: '/api/deployment/container-env-health', name: 'Env Health', icon: '\u{1F9EA}' },
        { path: '/api/deployment/share-process-namespace', name: 'Share PID', icon: '\u{1F517}' },
        { path: '/api/deployment/pod-priority-audit', name: 'Pod Priority', icon: '\u26A1' },
        { path: '/api/deployment/container-subpath', name: 'SubPath', icon: '\u{1F4C1}' },
        { path: '/api/deployment/pod-ephemeral-storage', name: 'Eph Storage', icon: '\u{1F4BE}' },
        { path: '/api/deployment/deployment-condition-history', name: 'Cond History', icon: '\u{1F4DC}' },
        { path: '/api/deployment/container-resources-gap', name: 'Res Gap', icon: '\u2696' },
        { path: '/api/deployment/pod-sethostname-domain', name: 'Hostname', icon: '\u{1F3E2}' },
        { path: '/api/deployment/container-tc-egress-mark', name: 'TC Egress', icon: '\u{1F6A7}' },
        { path: '/api/deployment/pod-nodeselector-validation', name: 'NS Valid', icon: '\u2705' },
        { path: '/api/deployment/startup-probe-audit', name: 'Startup Probe', icon: '\u{1F6A8}' },
        { path: '/api/deployment/container-command-hash', name: 'Cmd Hash', icon: '\u{1F9ED}' },
        { path: '/api/deployment/deployment-strategy-type', name: 'Strategy Type', icon: '\u{1F504}' },
        { path: '/api/deployment/pod-toleration-scope', name: 'Tol Scope', icon: '\u{1F6DE}' },
        { path: '/api/deployment/container-port-hostport-map', name: 'HostPort Map', icon: '\u{1F4CD}' },
        { path: '/api/deployment/deploy-progress-deadline', name: 'Prog Deadline', icon: '\u23F3' },
      ],
    },
  },
  'Documentation': {
    color: '#8b949e',
    icon: '\u{1F4DA}',
    subcategories: {
      'Overview': [
        { path: '/api/docs/platform-scorecard', name: 'Platform Scorecard', icon: '\u{1F4CB}' },
        { path: '/api/docs/exec-dashboard', name: 'Executive Dashboard', icon: '\u{1F4BC}' },
        { path: '/api/docs/platform-maturity', name: 'Platform Maturity', icon: '\u{1F3AF}' },
        { path: '/api/docs/resource-inventory', name: 'Resource Inventory', icon: '\u{1F4C2}' },
        { path: '/api/docs/platform-risk-heatmap', name: 'Risk Heatmap', icon: '\u{1F525}' },
      ],
      'Maturity & Playbooks': [
        { path: '/api/docs/workload-maturity-matrix', name: 'Maturity Matrix', icon: '\u{1F3AF}' },
        { path: '/api/docs/incident-playbook', name: 'Incident Playbook', icon: '\u{1F691}' },
        { path: '/api/docs/tech-debt-radar', name: 'Tech Debt Radar', icon: '\u{1F4A1}' },
        { path: '/api/docs/sre-scorecard', name: 'SRE Scorecard', icon: '\u{1F3AF}' },
        { path: '/api/docs/compliance-crosswalk', name: 'Compliance Crosswalk', icon: '\u{1F4DC}' },
        { path: '/api/docs/cost-optimization-roadmap', name: 'Cost Roadmap', icon: '\u{1F4B0}' },
        { path: '/api/docs/security-posture-trend', name: 'Security Posture', icon: '\u{1F6E1}' },
        { path: '/api/docs/capacity-planning-report', name: 'Capacity Report', icon: '\u{1F4CF}' },
      ],
      'API Docs': [
        { path: '/api/docs/api-coverage-map', name: 'API Coverage Map', icon: '\u{1F5FA}' },
        { path: '/api/docs/api-explorer', name: 'API Explorer', icon: '\u{1F50D}' },
        { path: '/api/docs/api-quality', name: 'API Quality', icon: '\u{1F50D}' },
        { path: '/api/docs/api-coverage-gap', name: 'Coverage Gap', icon: '\u{1F50D}' },
        { path: '/api/docs/api-governance-score', name: 'API Governance', icon: '\u{1F4DC}' },
      ],
      'Operations Docs': [
        { path: '/api/docs/action-priority-matrix', name: 'Action Priority Matrix', icon: '\u{1F4CB}' },
        { path: '/api/docs/oncall-readiness', name: 'Oncall Readiness', icon: '\u{1F6E1}' },
        { path: '/api/docs/runbook-coverage', name: 'Runbook Coverage', icon: '\u{1F4D6}' },
        { path: '/api/docs/upgrade-planner', name: 'Upgrade Planner', icon: '\u2B06' },
        { path: '/api/docs/training-readiness', name: 'Training Readiness', icon: '\u{1F4DA}' },
        { path: '/api/docs/backup-compliance-deep', name: 'Backup Compliance', icon: '\u{1F4BE}' },
        { path: '/api/docs/label-taxonomy-standard', name: 'Label Taxonomy', icon: '\u{1F3F7}' },
        { path: '/api/docs/change-impact-brief', name: 'Change Impact', icon: '\u{1F4C8}' },
        { path: '/api/docs/ownership-registry', name: 'Ownership Registry', icon: '\u{1F465}' },
        { path: '/api/docs/release-note-gen', name: 'Release Notes', icon: '\u{1F4DD}' },
        { path: '/api/docs/incident-postmortem', name: 'Postmortem', icon: '\u{1F691}' },
        { path: '/api/docs/cluster-runbook-gen', name: 'Cluster Runbook', icon: '\u{1F4D6}' },
        { path: '/api/docs/api-drift-detector', name: 'API Drift', icon: '\u{1F500}' },
        { path: '/api/docs/resource-topology-doc', name: 'Topology Map', icon: '\u{1F5FA}' },
        { path: '/api/docs/compliance-report', name: 'Compliance Report', icon: '\u{1F4DC}' },
        { path: '/api/docs/slo-handbook', name: 'SLO Handbook', icon: '\u{1F4CA}' },
        { path: '/api/docs/cluster-faq', name: 'Cluster FAQ', icon: '\u2753' },
        { path: '/api/docs/dr-plan-gen', name: 'DR Plan', icon: '\u{1F6E0}' },
        { path: '/api/docs/adr-generator', name: 'ADR Gen', icon: '\u{1F4D1}' },
        { path: '/api/docs/migration-checklist', name: 'Migration Checklist', icon: '\u2702' },
        { path: '/api/docs/policy-catalog', name: 'Policy Catalog', icon: '\u{1F4DC}' },
        { path: '/api/docs/service-dependency-graph', name: 'Dependency Graph', icon: '\u{1F5FA}' },
        { path: '/api/docs/performance-baseline', name: 'Perf Baseline', icon: '\u{1F4CA}' },
        { path: '/api/docs/naming-audit', name: 'Naming Audit', icon: '\u{1F4DD}' },
        { path: '/api/docs/env-var-catalog', name: 'Env Catalog', icon: '\u{1F4F1}' },
        { path: '/api/docs/annotation-inventory', name: 'Annot Inventory', icon: '\u{1F3F7}' },
      ],
    },
  },
};

// Flatten for search and backward compat
const AUDIT_ENDPOINTS = {};
for (const [dim, info] of Object.entries(AUDIT_STRUCTURE)) {
  const all = [];
  for (const eps of Object.values(info.subcategories)) {
    all.push(...eps);
  }
  AUDIT_ENDPOINTS[dim] = all;
}

const DIMENSION_COLORS = {};
for (const [dim, info] of Object.entries(AUDIT_STRUCTURE)) {
  DIMENSION_COLORS[dim] = info.color;
}

window.loadAuditDashboard = function() {
  const container = document.getElementById('audit-dashboard-content');
  if (!container) return;

  // Count totals
  let totalCards = 0;
  for (const info of Object.values(AUDIT_STRUCTURE)) {
    for (const eps of Object.values(info.subcategories)) {
      totalCards += eps.length;
    }
  }

  container.innerHTML = `
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:16px;flex-wrap:wrap;gap:8px;">
      <div>
        <h2 style="margin:0 0 4px 0;font-size:18px;">Audit Dashboard</h2>
        <p style="margin:0;color:#8b949e;font-size:13px;">${totalCards} audits across ${Object.keys(AUDIT_STRUCTURE).length} dimensions</p>
      </div>
      <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap;">
        <input type="text" id="audit-search" placeholder="Search audits..." 
          style="background:#0d1117;border:1px solid #30363d;border-radius:6px;padding:6px 12px;color:#c9d1d9;font-size:13px;width:200px;"
          oninput="window.filterAuditCards(this.value)" />
        <button id="audit-critical-btn" onclick="window.toggleCriticalOnly()"
          style="background:#da3633;border:none;border-radius:6px;padding:6px 12px;color:white;font-size:12px;cursor:pointer;white-space:nowrap;">
          Show Issues Only
        </button>
        <select id="audit-filter-score" onchange="window.filterAuditScore(this.value)"
          style="background:#0d1117;border:1px solid #30363d;border-radius:6px;padding:6px 8px;color:#c9d1d9;font-size:13px;">
          <option value="">All Scores</option>
          <option value="critical">Critical (&lt;40)</option>
          <option value="warning">Warning (40-79)</option>
          <option value="healthy">Healthy (&ge;80)</option>
        </select>
      </div>
    </div>
    <div id="audit-summary-grid" style="display:grid;grid-template-columns:repeat(auto-fill,minmax(160px,1fr));gap:10px;margin-bottom:20px;"></div>
    <div id="audit-critical-section" style="display:none;margin-bottom:20px;"></div>
    <div id="audit-dimensions"></div>
  `;

  // Render dimension sections with collapsible subcategories
  let dimHtml = '';
  for (const [dim, info] of Object.entries(AUDIT_STRUCTURE)) {
    const color = info.color;
    let dimTotal = 0;
    for (const eps of Object.values(info.subcategories)) dimTotal += eps.length;

    dimHtml += `
      <div class="audit-dim-section" data-dim="${dim}" style="margin-bottom:20px;border:1px solid #30363d;border-radius:8px;overflow:hidden;">
        <div class="audit-dim-header" style="display:flex;align-items:center;gap:8px;padding:12px 16px;background:#161b22;cursor:pointer;" 
             onclick="window.toggleAuditDim('${dim}')">
          <span style="font-size:16px;">${info.icon}</span>
          <h3 style="margin:0;font-size:14px;color:${color};flex:1;">${dim}</h3>
          <span style="color:#8b949e;font-size:12px;">${dimTotal} audits</span>
          <span class="dim-toggle" id="toggle-${dim}" style="color:#8b949e;font-size:12px;">[+]</span>
        </div>
        <div id="audit-dim-body-${dim}" style="padding:12px 16px;display:none;">
    `;

    for (const [subcat, endpoints] of Object.entries(info.subcategories)) {
      dimHtml += `
        <div class="audit-subcat" style="margin-bottom:16px;">
          <div style="font-size:12px;font-weight:600;color:var(--text-muted);text-transform:uppercase;letter-spacing:0.5px;margin-bottom:8px;padding-bottom:4px;border-bottom:1px solid var(--border-default);">
            ${subcat} <span style="color:#484f58;font-weight:400;">(${endpoints.length})</span>
          </div>
          <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(240px,1fr));gap:8px;">
      `;
      for (const ep of endpoints) {
        const cardId = btoa(ep.path).replace(/=/g, '');
        dimHtml += `
          <div class="audit-card" data-name="${ep.name.toLowerCase()}" data-dim="${dim}"
               id="audit-card-${cardId}"
               onclick="window.loadAuditDetail('${ep.path}','${ep.name.replace(/'/g, '')}')">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:4px;">
              <span style="font-size:12px;font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;color:var(--text-primary);">${ep.name}</span>
              <span class="audit-score" id="score-${cardId}">--</span>
            </div>
            <div class="audit-status" id="status-${cardId}">Loading...</div>
          </div>
        `;
      }
      dimHtml += `</div></div>`;
    }

    dimHtml += `</div></div>`;
  }
  document.getElementById('audit-dimensions').innerHTML = dimHtml;

  // Fetch ALL audit results in ONE request via batch summary endpoint
  // This replaces 1000+ individual HTTP requests with a single call
  // The backend caches results for 60s, so 100 users = 1 K8s API call per minute
  fetchJSON('/api/audit/summary')
    .then(data => {
      if (!data.results) return;
      for (const [path, entry] of Object.entries(data.results)) {
        const cardId = btoa(path).replace(/=/g, '');
        const scoreEl = document.getElementById('score-' + cardId);
        const statusEl = document.getElementById('status-' + cardId);
        const cardEl = document.getElementById('audit-card-' + cardId);
        if (!scoreEl || !cardEl) continue;

        if (entry.score !== undefined && entry.score > 0) {
          scoreEl.textContent = entry.score;
          cardEl.dataset.score = entry.score;
          cardEl.className = entry.score >= 80 ? 'audit-card audit-card-good'
            : entry.score >= 60 ? 'audit-card audit-card-warn'
            : entry.score >= 40 ? 'audit-card audit-card-bad'
            : 'audit-card audit-card-crit';
        }
        if (statusEl) {
          if (entry.status === 'pending') {
            statusEl.textContent = 'Loading...';
          } else if (entry.summary) {
            const parts = [];
            for (const [k, v] of Object.entries(entry.summary)) {
              if (typeof v === 'number' && parts.length < 2) {
                const label = k.replace(/([A-Z])/g, ' $1').replace(/^./, c => c.toLowerCase()).replace(/total/g, '').trim();
                parts.push(`${v} ${label}`);
              }
            }
            statusEl.textContent = parts.join(', ') || (entry.grade ? 'Grade: ' + entry.grade : 'OK');
          } else {
            statusEl.textContent = entry.grade ? 'Grade: ' + entry.grade : 'OK';
          }
        }
      }
    })
    .catch(err => {
      // Fallback: if batch endpoint fails, load endpoints in small batches
      const allEP = [];
      for (const info of Object.values(AUDIT_STRUCTURE)) {
        for (const eps of Object.values(info.subcategories)) {
          for (const ep of eps) { allEP.push(ep); }
        }
      }
      const BATCH = 8;
      let idx = 0;
      function fallback() {
        const batch = allEP.slice(idx, idx + BATCH);
        if (!batch.length) return;
        Promise.allSettled(batch.map(ep => {
          const cid = btoa(ep.path).replace(/=/g, '');
          return fetchJSON(ep.path).then(d => {
            const sEl = document.getElementById('score-' + cid);
            const stEl = document.getElementById('status-' + cid);
            const cEl = document.getElementById('audit-card-' + cid);
            if (sEl && d.healthScore !== undefined) {
              sEl.textContent = d.healthScore;
              cEl.className = d.healthScore >= 80 ? 'audit-card audit-card-good' : d.healthScore >= 60 ? 'audit-card audit-card-warn' : d.healthScore >= 40 ? 'audit-card audit-card-bad' : 'audit-card audit-card-crit';
            }
            if (stEl && d.summary) {
              const p = [];
              for (const [k, v] of Object.entries(d.summary)) { if (typeof v === 'number' && p.length < 2) p.push(`${v} ${k.replace(/([A-Z])/g, ' $1').replace(/^./, c => c.toLowerCase()).replace(/total/g, '').trim()}`); }
              stEl.textContent = p.join(', ') || 'OK';
            }
          }).catch(() => { const st = document.getElementById('status-' + cid); if (st) st.textContent = 'Failed'; });
        })).then(() => { idx += BATCH; if (idx < allEP.length) setTimeout(fallback, 300); });
      }
      fallback();
    });

  // Render dimension summary cards
  renderSummaryCards();
};

window.toggleAuditDim = function(dim) {
  const body = document.getElementById('audit-dim-body-' + dim);
  const toggle = document.getElementById('toggle-' + dim);
  if (body.style.display === 'none') {
    body.style.display = ''; toggle.textContent = '[-]';
  } else {
    body.style.display = 'none'; toggle.textContent = '[+]';
  }
};

window.filterAuditCards = function(query) {
  query = query.toLowerCase().trim();
  document.querySelectorAll('.audit-card').forEach(card => {
    if (!query || card.dataset.name.includes(query)) {
      card.style.display = '';
    } else {
      card.style.display = 'none';
    }
  });
  // Hide empty subcategories
  document.querySelectorAll('.audit-subcat').forEach(sub => {
    const visible = sub.querySelectorAll('.audit-card:not([style*="display: none"])').length;
    sub.style.display = visible > 0 ? '' : 'none';
  });
  // Collapse empty dimensions
  document.querySelectorAll('.audit-dim-section').forEach(sec => {
    const visible = sec.querySelectorAll('.audit-card:not([style*="display: none"])').length;
    sec.style.display = visible > 0 ? '' : 'none';
  });
};

window.filterAuditScore = function(filter) {
  if (!filter) {
    document.querySelectorAll('.audit-card').forEach(c => c.style.display = '');
    document.querySelectorAll('.audit-subcat').forEach(s => s.style.display = '');
    document.querySelectorAll('.audit-dim-section').forEach(s => s.style.display = '');
    return;
  }
  document.querySelectorAll('.audit-card').forEach(card => {
    const score = parseInt(card.dataset.score || '0');
    let show = false;
    if (filter === 'critical' && score < 40) show = true;
    if (filter === 'warning' && score >= 40 && score < 80) show = true;
    if (filter === 'healthy' && score >= 80) show = true;
    card.style.display = show ? '' : 'none';
  });
  document.querySelectorAll('.audit-subcat').forEach(sub => {
    const visible = sub.querySelectorAll('.audit-card:not([style*="display: none"])').length;
    sub.style.display = visible > 0 ? '' : 'none';
  });
  document.querySelectorAll('.audit-dim-section').forEach(sec => {
    const visible = sec.querySelectorAll('.audit-card:not([style*="display: none"])').length;
    sec.style.display = visible > 0 ? '' : 'none';
  });
};

window.toggleCriticalOnly = function() {
  const btn = document.getElementById('audit-critical-btn');
  const section = document.getElementById('audit-critical-section');
  const dims = document.getElementById('audit-dimensions');
  if (section.style.display === 'none') {
    // Show critical only mode
    section.style.display = '';
    dims.style.display = 'none';
    btn.textContent = 'Show All';
    btn.style.background = '#238636';
    // Collect all non-healthy cards
    let html = '<h3 style="font-size:14px;margin:0 0 12px 0;color:#f85149;">Items Needing Attention (Score &lt; 80)</h3>';
    let issues = 0;
    document.querySelectorAll('.audit-card').forEach(card => {
      const score = parseInt(card.dataset.score || '100');
      if (score < 80) {
        issues++;
        const cardClone = card.cloneNode(true);
        html += cardClone.outerHTML;
      }
    });
    if (issues === 0) {
      html = '<div style="text-align:center;padding:40px;color:#3fb950;font-size:16px;">All checks are healthy! No issues found.</div>';
    } else {
      html = `<div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(240px,1fr));gap:8px;">${html}</div>`;
    }
    section.innerHTML = html;
  } else {
    section.style.display = 'none';
    dims.style.display = '';
    btn.textContent = 'Show Issues Only';
    btn.style.background = '#da3633';
  }
};

function renderSummaryCards() {
  const grid = document.getElementById('audit-summary-grid');
  if (!grid) return;
  let html = '';
  for (const [dim, info] of Object.entries(AUDIT_STRUCTURE)) {
    let total = 0;
    for (const eps of Object.values(info.subcategories)) total += eps.length;
    html += `
      <div style="border:1px solid #30363d;border-left:3px solid ${info.color};border-radius:6px;padding:12px;background:#0d1117;cursor:pointer;"
           onclick="document.getElementById('audit-dim-body-${dim}').scrollIntoView({behavior:'smooth',block:'start'})">
        <div style="font-size:11px;color:#8b949e;">${info.icon} ${dim}</div>
        <div style="display:flex;align-items:baseline;gap:4px;margin-top:4px;">
          <span id="dim-avg-${dim}" style="font-size:22px;font-weight:700;color:${info.color};">--</span>
          <span style="font-size:11px;color:#484f58;">/ 100 (${total})</span>
        </div>
      </div>
    `;
  }
  grid.innerHTML = html;
}

window.loadAuditDetail = function(path, name) {
  const container = document.getElementById('audit-dashboard-content');
  if (!container) return;

  container.innerHTML = `<div style="text-align:center;padding:40px;color:#8b949e;">Loading ${escapeHtml(name)}...</div>`;

  fetchJSON(path)
    .then(data => {
      let html = `
        <div style="margin-bottom:16px;">
          <button onclick="window.loadAuditDashboard()" class="btn-secondary" style="margin-bottom:12px;">&#8592; Back to Dashboard</button>
          <h2 style="margin:0 0 4px 0;font-size:18px;">${escapeHtml(name)}</h2>
          <div style="display:flex;gap:12px;align-items:center;margin-top:8px;">
      `;

      const score = data.healthScore !== undefined ? data.healthScore : data.riskScore !== undefined ? data.riskScore : data.score;
      if (score !== undefined) {
        const color = score >= 80 ? '#3fb950' : score >= 60 ? '#d29922' : score >= 40 ? '#f0883e' : '#f85149';
        html += `<span style="font-size:28px;font-weight:700;color:${color};">${score}</span><span style="color:#8b949e;font-size:13px;">/ 100</span>`;
      }
      html += `</div></div>`;

      // Summary
      if (data.summary) {
        html += '<div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(160px,1fr));gap:8px;margin-bottom:20px;">';
        for (const [key, val] of Object.entries(data.summary)) {
          if (typeof val === 'boolean') {
            html += `<div style="border:1px solid #30363d;border-radius:6px;padding:8px;background:#0d1117;"><div style="font-size:10px;color:#8b949e;">${escapeHtml(key)}</div><div style="font-size:13px;font-weight:600;color:${val ? '#3fb950' : '#f85149'};">${val ? 'Yes' : 'No'}</div></div>`;
          } else if (typeof val === 'number') {
            html += `<div style="border:1px solid #30363d;border-radius:6px;padding:8px;background:#0d1117;"><div style="font-size:10px;color:#8b949e;">${escapeHtml(key)}</div><div style="font-size:16px;font-weight:700;">${val}</div></div>`;
          } else if (typeof val === 'string' && val.length < 50) {
            html += `<div style="border:1px solid #30363d;border-radius:6px;padding:8px;background:#0d1117;"><div style="font-size:10px;color:#8b949e;">${escapeHtml(key)}</div><div style="font-size:13px;font-weight:600;">${escapeHtml(val)}</div></div>`;
          }
        }
        html += '</div>';
      }

      // Issues
      if (data.issues && data.issues.length > 0) {
        html += '<h3 style="font-size:14px;margin:16px 0 8px 0;">Issues (' + data.issues.length + ')</h3><div style="max-height:300px;overflow-y:auto;">';
        for (const issue of data.issues.slice(0, 50)) {
          const sev = issue.severity || 'info';
          const color = sev === 'critical' ? '#f85149' : sev === 'warning' ? '#d29922' : '#58a6ff';
          html += `<div style="border-left:3px solid ${color};padding:8px 12px;margin-bottom:4px;background:#0d1117;border-radius:0 4px 4px 0;"><span style="font-size:10px;color:${color};font-weight:600;text-transform:uppercase;">${escapeHtml(sev)}</span> <span style="font-size:12px;">${escapeHtml(issue.message || '')}</span></div>`;
        }
        html += '</div>';
      }

      // Recommendations
      if (data.recommendations && data.recommendations.length > 0) {
        html += '<h3 style="font-size:14px;margin:16px 0 8px 0;">Recommendations</h3>';
        for (const rec of data.recommendations) {
          html += `<div style="padding:8px 12px;margin-bottom:4px;background:#0d1117;border-radius:4px;font-size:12px;border-left:2px solid #58a6ff;">${escapeHtml(rec)}</div>`;
        }
      }

      // Raw JSON (collapsible)
      html += `<details style="margin-top:16px;"><summary style="cursor:pointer;color:#8b949e;font-size:12px;">Raw JSON</summary><pre style="background:#0d1117;border:1px solid #30363d;border-radius:6px;padding:12px;font-size:11px;overflow-x:auto;max-height:400px;color:#c9d1d9;">${escapeHtml(JSON.stringify(data, null, 2))}</pre></details>`;

      container.innerHTML = html;
    })
    .catch(err => {
      container.innerHTML = `<div style="text-align:center;padding:40px;color:#f85149;">Failed to load: ${escapeHtml(err.message)}</div>`;
    });
};
