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
        { path: '/api/security/seccomp-profile-audit', name: 'Seccomp Profile', icon: '\u{1F6E1}' },
        { path: '/api/security/sa-token-age-v2', name: 'SA Token Age', icon: '\u{1F511}' },
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
        { path: '/api/security/image-provenance-v3', name: 'Image Provenance', icon: '\u{1F50D}' },
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
        { path: '/api/product/mesh-ready-check', name: 'Mesh Check', icon: '\u{1F575}' },
        { path: '/api/product/vol-access-audit', name: 'Vol Access', icon: '\u{1F4BE}' },
        { path: '/api/product/pdb-gap-analysis-v2', name: 'PDB Gap v2', icon: '\u{1F6E1}' },
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
        { path: '/api/operations/restart-storm', name: 'Restart Storm', icon: '\u26A1' },
        { path: '/api/operations/pod-startup', name: 'Pod Startup', icon: '\u23F1' },
        { path: '/api/operations/oom-tracker', name: 'OOM Tracker', icon: '\u{1F4A9}' },
      ],
      'Events & Incidents': [
        { path: '/api/operations/event-storm', name: 'Event Storm', icon: '\u26A1' },
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
        { path: '/api/operations/ingress-health', name: 'Ingress Health', icon: '\u{1F310}' },
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
        { path: '/api/scalability/anti-affinity-coverage', name: 'Anti-Affinity', icon: '\u{1F6E1}' },
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
        { path: '/api/product/priority-class-v2115', name: 'Priority Class', icon: '\u26A1' },
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
        { path: '/api/scalability/request-headroom', name: 'Request Headroom', icon: '\u{1F4CF}' },
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
        { path: '/api/deployment/helm-health', name: 'Helm Health', icon: '\u{1F4E6}' },
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
        { path: '/api/deployment/preflight-check', name: 'Preflight Check', icon: '\u2705' },
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
        <p style="margin:0;color:#8b949e;font-size:13px;">${totalCards} audits across ${Object.keys(AUDIT_STRUCTURE).length} dimensions, organized by subcategory</p>
      </div>
      <div style="display:flex;gap:8px;align-items:center;">
        <input type="text" id="audit-search" placeholder="Search audits..." 
          style="background:#0d1117;border:1px solid #30363d;border-radius:6px;padding:6px 12px;color:#c9d1d9;font-size:13px;width:220px;"
          oninput="window.filterAuditCards(this.value)" />
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
          <span class="dim-toggle" id="toggle-${dim}" style="color:#8b949e;font-size:12px;">[-]</span>
        </div>
        <div id="audit-dim-body-${dim}" style="padding:12px 16px;">
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

  // Fetch all endpoints with concurrency limiting (max 8 concurrent to prevent HTTP/2 stream exhaustion)
  const allEndpoints = [];
  for (const info of Object.values(AUDIT_STRUCTURE)) {
    for (const endpoints of Object.values(info.subcategories)) {
      for (const ep of endpoints) { allEndpoints.push(ep); }
    }
  }

  const BATCH_SIZE = 8;
  let batchIdx = 0;

  function processBatch() {
    const batch = allEndpoints.slice(batchIdx, batchIdx + BATCH_SIZE);
    if (batch.length === 0) return;

    Promise.allSettled(batch.map(ep => {
      const cardId = btoa(ep.path).replace(/=/g, '');
      return fetchJSON(ep.path)
        .then(data => {
          const score = data.healthScore !== undefined ? data.healthScore
            : data.riskScore !== undefined ? data.riskScore
            : data.score !== undefined ? data.score
            : data.grade ? undefined : null;
          const scoreEl = document.getElementById('score-' + cardId);
          const statusEl = document.getElementById('status-' + cardId);
          const cardEl = document.getElementById('audit-card-' + cardId);
          if (scoreEl && score !== undefined && score !== null) {
            scoreEl.textContent = score;
            cardEl.dataset.score = score;
            cardEl.className = score >= 80 ? 'audit-card audit-card-good' : score >= 60 ? 'audit-card audit-card-warn' : score >= 40 ? 'audit-card audit-card-bad' : 'audit-card audit-card-crit';
          }
          if (statusEl) {
            const summary = data.summary || {};
            const parts = [];
            for (const [k, v] of Object.entries(summary)) {
              if (typeof v === 'number' && parts.length < 3) parts.push(`${v} ${k.replace(/([A-Z])/g, ' $1').replace(/^./, c => c.toLowerCase()).replace(/total/g, '').trim()}`.trim());
            }
            if (parts.length === 0 && data.recommendations && data.recommendations.length > 0) {
              let rec = data.recommendations[0] || '';
              statusEl.textContent = rec.length > 60 ? rec.substring(0, 60) + '...' : rec;
            } else {
              statusEl.textContent = parts.join(', ') || 'OK';
            }
          }
        })
        .catch(() => {
          const statusEl = document.getElementById('status-' + cardId);
          if (statusEl) statusEl.textContent = 'Failed';
        });
    })).then(() => {
      batchIdx += BATCH_SIZE;
      if (batchIdx < allEndpoints.length) { setTimeout(processBatch, 200); }
    });
  }
  processBatch();

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
