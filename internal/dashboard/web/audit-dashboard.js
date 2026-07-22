// audit-dashboard.js — Unified Audit Dashboard (v2: categorized, searchable, consolidated)
import { escapeHtml, fetchJSON } from './modules/utils.js';

// Consolidated endpoint registry: subcategories within each dimension
// "primary" marks the canonical endpoint; "alias" entries are visually grouped under it
const AUDIT_STRUCTURE = {
  'Security': {
    color: '#f85149',
    icon: '\u1F6E1',
    subcategories: {
      'RBAC & Access': [
        { path: '/api/security/rbac-audit', name: 'RBAC Audit', icon: '\u1F511' },
        { path: '/api/security/sa-token-audit', name: 'SA Token Audit', icon: '\u1F510' },
        { path: '/api/security/sa-token-lifecycle', name: 'SA Token Lifecycle', icon: '\u1F511' },
        { path: '/api/security/service-accounts', name: 'Service Accounts', icon: '\u1F464' },
        { path: '/api/security/rbac-effective', name: 'Effective RBAC', icon: '\u1F50D' },
        { path: '/api/security/rbac-graph', name: 'RBAC Graph', icon: '\u1F5FA' },
        { path: '/api/security/rbac-risk', name: 'RBAC Risk', icon: '\u26A0' },
        { path: '/api/security/rbac-blast', name: 'RBAC Blast Radius', icon: '\u1F4A5' },
        { path: '/api/security/privilege-escalation-path', name: 'Privilege Escalation', icon: '\u1F6A8' },
        { path: '/api/security/rbac-drift', name: 'RBAC Drift', icon: '\u1F501' },
      ],
      'Secrets Management': [
        { path: '/api/security/secret-scan', name: 'Secret Scanner', icon: '\u1F576', primary: true },
        { path: '/api/security/secret-rotation-v2', name: 'Secret Rotation', icon: '\u1F504', alias: ['secret-rotation-plan', 'secret-rotation', 'secrets/rotation'] },
        { path: '/api/security/secret-exposure', name: 'Secret Exposure', icon: '\u1F441' },
        { path: '/api/security/secret-posture', name: 'Secret Posture', icon: '\u1F6E1' },
        { path: '/api/security/secret-lifecycle', name: 'Secret Lifecycle', icon: '\u1F501' },
        { path: '/api/security/secret-age', name: 'Secret Age', icon: '\u23F0' },
        { path: '/api/security/secret-spray', name: 'Secret Spray', icon: '\u1F510' },
        { path: '/api/security/env-leak-scanner', name: 'Env Leak Scanner', icon: '\u1F576' },
      ],
      'Pod Security (PSP/PSS)': [
        { path: '/api/security/pss-scorecard', name: 'PSS Scorecard', icon: '\u1F6E1' },
        { path: '/api/security/pss-hardening', name: 'PSS Hardening', icon: '\u1F6E1' },
        { path: '/api/security/psa-audit', name: 'PSA Audit', icon: '\u1F6AB' },
        { path: '/api/security/seccomp-audit', name: 'Seccomp Audit', icon: '\u1F6E1' },
        { path: '/api/security/seccomp-profile-gap', name: 'Seccomp Gap', icon: '\u1F6E1' },
        { path: '/api/security/container-hardening', name: 'Container Hardening', icon: '\u1F6E1' },
        { path: '/api/security/privilege-map', name: 'Privilege Map', icon: '\u1F512' },
        { path: '/api/security/mac-audit', name: 'MAC Audit', icon: '\u1F512' },
        { path: '/api/security/hostpath-audit', name: 'HostPath Audit', icon: '\u1F4C1' },
        { path: '/api/security/container-capabilities', name: 'Cap Audit', icon: '\u1F511' },
        { path: '/api/security/readonly-rootfs-audit', name: 'Readonly RootFS', icon: '\u1F512' },
        { path: '/api/security/seccomp-profile-audit', name: 'Seccomp Profile', icon: '\u1F6E1' },
        { path: '/api/security/sa-token-age', name: 'SA Token Age', icon: '\u1F511' },
        { path: '/api/security/runtime-class-audit', name: 'Runtime Class', icon: '\u1F9E9' },
      ],
      'Network Security': [
        { path: '/api/security/network-policies', name: 'Network Policies', icon: '\u1F6E1' },
        { path: '/api/security/netpol-generator', name: 'NetPol Generator', icon: '\u1F527' },
        { path: '/api/security/net-policy-effectiveness', name: 'NetPol Effectiveness', icon: '\u1F50D' },
        { path: '/api/security/mtls-trust-domain', name: 'mTLS Trust', icon: '\u1F511' },
        { path: '/api/security/endpoint-exposure', name: 'Endpoint Exposure', icon: '\u1F441' },
        { path: '/api/security/network-segment-gap', name: 'Segment Gap', icon: '\u1F310' },
      ],
      'Compliance & Policy': [
        { path: '/api/security/compliance-map', name: 'Compliance Map', icon: '\u1F4CB' },
        { path: '/api/security/compliance-posture', name: 'Compliance Posture', icon: '\u1F4DC' },
        { path: '/api/security/compliance-gap', name: 'Compliance Gap', icon: '\u1F4CB' },
        { path: '/api/security/kyverno-compliance', name: 'Kyverno', icon: '\u1F4DC' },
        { path: '/api/security/opa-compliance', name: 'OPA/Gatekeeper', icon: '\u1F6AB' },
        { path: '/api/security/policy-governance', name: 'Policy Governance', icon: '\u1F4DC' },
        { path: '/api/security/policy-drift', name: 'Policy Drift', icon: '\u1F501' },
        { path: '/api/security/admission-bypass-audit', name: 'Admission Bypass', icon: '\u26D4' },
      ],
      'Supply Chain & Images': [
        { path: '/api/security/image-vuln', name: 'Image Vulnerabilities', icon: '\u26A0' },
        { path: '/api/security/supply-chain', name: 'Supply Chain', icon: '\u1F4E6' },
        { path: '/api/security/trust-chain', name: 'Trust Chain', icon: '\u1F512' },
        { path: '/api/security/image-provenance-v3', name: 'Image Provenance', icon: '\u1F50D' },
        { path: '/api/security/image-baseline-drift', name: 'Image Baseline Drift', icon: '\u1F4F7' },
      ],
      'Runtime & Drift': [
        { path: '/api/security/runtime-scan', name: 'Runtime Scan', icon: '\u1F50D' },
        { path: '/api/security/runtime-drift-detect', name: 'Runtime Drift', icon: '\u1F501' },
        { path: '/api/security/runtime-threat', name: 'Runtime Threat', icon: '\u1F6A8' },
        { path: '/api/security/sec-drift', name: 'Security Drift', icon: '\u1F501' },
      ],
      'Certificates': [
        { path: '/api/security/cert-expiry', name: 'Cert Expiry', icon: '\u1F510' },
        { path: '/api/security/cert-inventory', name: 'Cert Inventory', icon: '\u1F4DC' },
        { path: '/api/security/cert-chain-validator', name: 'Cert Chain', icon: '\u1F510' },
      ],
      'Posture & Audit': [
        { path: '/api/security/posture-scorecard', name: 'Posture Scorecard', icon: '\u1F4CB' },
        { path: '/api/security/hardening-score', name: 'Hardening Score', icon: '\u1F6E1' },
        { path: '/api/security/attack-surface', name: 'Attack Surface', icon: '\u1F575' },
        { path: '/api/security/blast-radius', name: 'Blast Radius', icon: '\u1F4A5' },
        { path: '/api/security/fix-plan', name: 'Fix Plan', icon: '\u1F527' },
        { path: '/api/security/audit-policy', name: 'Audit Policy', icon: '\u1F4DD' },
        { path: '/api/security/audit-trail', name: 'Audit Trail', icon: '\u1F4DD' },
      ],
      'Supply Chain & TLS': [
        { path: '/api/security/image-registry-allowlist', name: 'Registry Allowlist', icon: '\u1F4E6' },
        { path: '/api/security/sa-mount-exposure', name: 'SA Mount Exposure', icon: '\u1F511' },
        { path: '/api/security/tls-version-audit', name: 'TLS Version Audit', icon: '\u1F510' },
        { path: '/api/security/pod-escape-risk', name: 'Pod Escape Risk', icon: '\u1F6A8' },
        { path: '/api/security/egress-policy-gap', name: 'Egress Policy Gap', icon: '\u2192' },
        { path: '/api/security/cis-benchmark-lite', name: 'CIS Benchmark', icon: '\u1F4DC' },
        { path: '/api/security/vol-encryption-audit', name: 'Volume Encryption', icon: '\u1F512' },
        { path: '/api/security/webhook-posture', name: 'Webhook Posture', icon: '\u26D4' },
        { path: '/api/security/key-rotation-compliance', name: 'Key Rotation', icon: '\u1F511' },
        { path: '/api/security/capability-audit', name: 'Capability Audit', icon: '\u1F6E1' },
        { path: '/api/security/host-namespace-audit', name: 'Host NS Audit', icon: '\u1F3E0' },
        { path: '/api/security/pss-compliance', name: 'PSS Compliance', icon: '\u1F6E1' },
        { path: '/api/security/dns-exfil-risk-v2', name: 'DNS Exfil Risk', icon: '\u1F50C' },
        { path: '/api/security/port-forward-audit-v2', name: 'Port Forward', icon: '\u2192' },
        { path: '/api/security/image-provenance-v3', name: 'Image Provenance', icon: '\u1F4E6' },
      ],
    },
  },
  'Operations': {
    color: '#d29922',
    icon: '\u26A1',
    subcategories: {
      'Control Plane': [
        { path: '/api/operations/etcd-health', name: 'Etcd Health', icon: '\u1F50C' },
        { path: '/api/operations/kubelet-health', name: 'Kubelet Health', icon: '\u1F3E2' },
        
        { path: '/api/operations/cni-health', name: 'CNI Health', icon: '\u1F310' },
        { path: '/api/operations/coredns-config-audit', name: 'CoreDNS Config', icon: '\u1F310' },
        { path: '/api/operations/webhook-timeout-audit', name: 'Webhook Timeout', icon: '\u23F1' },
        { path: '/api/operations/node-condition-trend', name: 'Node Condition', icon: '\u1F4C9' },
        { path: '/api/operations/container-log-size', name: 'Log Size', icon: '\u1F4DD' },
        { path: '/api/operations/kubelet-config-drift', name: 'Kubelet Drift', icon: '\u2699' },
        { path: '/api/operations/control-plane', name: 'Control Plane', icon: '\u1F3E2' },
        { path: '/api/operations/cert-transparency-monitor', name: 'Cert Transparency', icon: '\u1F510' },
        { path: '/api/operations/apf-audit', name: 'API Priority/Fairness', icon: '\u2696' },
      ],
      'Observability Stack': [
        { path: '/api/operations/metrics-pipeline', name: 'Metrics Pipeline', icon: '\u1F4CA' },
        { path: '/api/operations/prom-health', name: 'Prometheus', icon: '\u1F525' },
        { path: '/api/operations/grafana-health', name: 'Grafana', icon: '\u1F4C4' },
        { path: '/api/operations/alertmanager-health', name: 'Alertmanager', icon: '\u1F514' },
        { path: '/api/operations/audit-log-health', name: 'Audit Log Pipeline', icon: '\u1F4DD' },
        { path: '/api/operations/log-volume', name: 'Log Volume', icon: '\u1F4DD' },
        { path: '/api/operations/obs-coverage', name: 'Obs Coverage', icon: '\u1F441' },
        { path: '/api/operations/obs-cardinality', name: 'Obs Cardinality', icon: '\u1F4CF' },
      ],
      'Pod Health & Restarts': [
        { path: '/api/operations/pod-health-index', name: 'Pod Health Index', icon: '\u1F493' },
        { path: '/api/operations/crashloop', name: 'CrashLoopBackOff', icon: '\u1F501' },
        { path: '/api/operations/crash-budget-tracker', name: 'Crash Budget', icon: '\u1F4B0' },
        { path: '/api/operations/restart-analyzer', name: 'Restart Analyzer', icon: '\u1F501' },
        { path: '/api/operations/pod-restart-forensics-deep', name: 'Restart Forensics', icon: '\u1F50D' },
        { path: '/api/operations/restart-storm', name: 'Restart Storm', icon: '\u26A1' },
        { path: '/api/operations/pod-startup', name: 'Pod Startup', icon: '\u23F1' },
        { path: '/api/operations/oom-tracker', name: 'OOM Tracker', icon: '\u1F4A9' },
      ],
      'Events & Incidents': [
        { path: '/api/operations/event-storm', name: 'Event Storm', icon: '\u26A1' },
        { path: '/api/operations/event-noise-filter', name: 'Event Noise Filter', icon: '\u266A' },
        { path: '/api/operations/incident-correlation', name: 'Incident Correlation', icon: '\u1F50D' },
        { path: '/api/operations/deployment-health-trend', name: 'Deploy Health Trend', icon: '\u1F4C8' },
        { path: '/api/operations/event-correlation-matrix', name: 'Event Correlation', icon: '\u1F9ED' },
        { path: '/api/operations/incident-timeline', name: 'Incident Timeline', icon: '\u1F4C5' },
        { path: '/api/operations/triage', name: 'Triage', icon: '\u1FA7A' },
      ],
      'SLO & SLI': [
        { path: '/api/operations/pod-slo', name: 'Pod SLO', icon: '\u1F3AF' },
        { path: '/api/operations/slo-burn-rate', name: 'SLO Burn Rate', icon: '\u1F525' },
        { path: '/api/operations/golden-signal-budget', name: 'Golden Signals', icon: '\u1F4A1' },
        { path: '/api/operations/health-score', name: 'Cluster Health', icon: '\u1F493' },
        { path: '/api/operations/health-trend', name: 'Health Trend', icon: '\u1F4C8' },
      ],
      'Node & Scheduling': [
        { path: '/api/operations/node-pressure', name: 'Node Pressure', icon: '\u26A0' },
        { path: '/api/operations/node-trend', name: 'Node Trend', icon: '\u1F4C8' },
        { path: '/api/operations/drain-impact', name: 'Drain Impact', icon: '\u1F6A6' },
        { path: '/api/operations/pdb-audit', name: 'PDB Audit', icon: '\u1F6E1' },
        { path: '/api/operations/pdb-generator', name: 'PDB Generator', icon: '\u1F527' },
        { path: '/api/operations/scheduling-latency', name: 'Scheduling Latency', icon: '\u23F1' },
        { path: '/api/operations/cluster-version-skew', name: 'Version Skew', icon: '\u2195' },
        { path: '/api/operations/node-taint-impact', name: 'Taint Impact', icon: '\u26D4' },
      ],
      'API Server': [
        { path: '/api/operations/api-load', name: 'API Server Load', icon: '\u1F4E6' },
        { path: '/api/operations/api-latency', name: 'API Latency', icon: '\u23F1' },
        { path: '/api/operations/api-access-pattern', name: 'API Access Pattern', icon: '\u1F511' },
        { path: '/api/operations/api-server-slo', name: 'API Server SLO', icon: '\u1F3AF' },
      ],
      'Reliability': [
        { path: '/api/operations/chaos-readiness', name: 'Chaos Readiness', icon: '\u1F4A5' },
        { path: '/api/operations/throttle-risk', name: 'Throttle Risk', icon: '\u1F4A7' },
        { path: '/api/operations/pod-evictions', name: 'Pod Evictions', icon: '\u26A0' },
        { path: '/api/operations/mttr', name: 'MTTR', icon: '\u23F1' },
        { path: '/api/operations/probes', name: 'Health Probes', icon: '\u1FA78' },
      ],
      'Phase & Lifecycle': [
        { path: '/api/operations/pod-phase-timeline', name: 'Phase Timeline', icon: '\u23F1' },
        { path: '/api/operations/image-gc-pressure', name: 'Image GC Pressure', icon: '\u1F4BE' },
        { path: '/api/operations/controller-reconcile', name: 'Controller Reconcile', icon: '\u1F501' },
        { path: '/api/operations/node-maint-window', name: 'Maint Window', icon: '\u1F6E0' },
        { path: '/api/operations/resource-leak-detector', name: 'Resource Leaks', icon: '\u1F4B8' },
        { path: '/api/operations/log-agg-health', name: 'Log Agg Health', icon: '\u1F4DD' },
        { path: '/api/operations/backup-snapshot-audit', name: 'Backup Audit', icon: '\u1F4BE' },
        { path: '/api/operations/job-success-rate', name: 'Job Success', icon: '\u2705' },
        { path: '/api/operations/event-retention', name: 'Event Volume', icon: '\u1F4CB' },
        { path: '/api/operations/control-plane-health', name: 'Control Plane', icon: '\u1F3D7' },
        { path: '/api/operations/csi-driver-health', name: 'CSI Driver', icon: '\u1F4BF' },
        { path: '/api/operations/cert-renewal-timeline', name: 'Cert Timeline', icon: '\u1F4C5' },
      ],
    },
  },
  'Scalability': {
    color: '#bc8cff',
    icon: '\u1F4C8',
    subcategories: {
      'Cost & Waste': [
        { path: '/api/scalability/cost-waste', name: 'Cost Waste', icon: '\u1F4B0' },
        { path: '/api/scalability/cost-allocation', name: 'Cost Allocation', icon: '\u1F4B0' },
        { path: '/api/scalability/cost-intelligence', name: 'Cost Intelligence', icon: '\u1F9EE' },
        { path: '/api/scalability/cost-anomaly', name: 'Cost Anomaly', icon: '\u26A0' },
        { path: '/api/scalability/idle-waste', name: 'Idle Waste', icon: '\u1F4A9' },
        { path: '/api/scalability/chargeback', name: 'Chargeback', icon: '\u1F4B3' },
        { path: '/api/scalability/unit-economics', name: 'Unit Economics', icon: '\u1F4B0' },
        { path: '/api/scalability/budget-alert', name: 'Budget Alert', icon: '\u26A0' },
      ],
      'Autoscaling': [
        { path: '/api/scalability/hpa-performance', name: 'HPA Performance', icon: '\u2195' },
        { path: '/api/scalability/hpa-behavior', name: 'HPA Behavior', icon: '\u2195' },
        { path: '/api/scalability/autoscale-readiness', name: 'Autoscale Readiness', icon: '\u2195' },
        { path: '/api/scalability/autoscaler-gap', name: 'Autoscaler Gap', icon: '\u2195' },
        { path: '/api/scalability/autoscaling-intel', name: 'Autoscaling Intel', icon: '\u1F9EE' },
        { path: '/api/scalability/vpa-audit', name: 'VPA Audit', icon: '\u2195' },
        { path: '/api/scalability/hpa-cooldown-audit', name: 'HPA Cooldown', icon: '\u2195' },
      ],
      'Resource Efficiency': [
        { path: '/api/scalability/alloc-efficiency', name: 'Alloc Efficiency', icon: '\u2696' },
        { path: '/api/scalability/overcommit', name: 'Overcommit', icon: '\u26A0' },
        { path: '/api/scalability/overcommit-risk', name: 'Overcommit Risk', icon: '\u26A0' },
        { path: '/api/scalability/right-size-engine', name: 'Right-Size Engine', icon: '\u1F4CF' },
        { path: '/api/scalability/request-accuracy', name: 'Request Accuracy', icon: '\u1F3AF' },
        { path: '/api/scalability/request-intelligence', name: 'Request Intel', icon: '\u1F9ED' },
        { path: '/api/scalability/resource-request-saturation', name: 'Request Saturation', icon: '\u1F4CA' },
      ],
      'Node Management': [
        { path: '/api/scalability/node-lifecycle', name: 'Node Lifecycle', icon: '\u1F578' },
        { path: '/api/scalability/node-pool-health', name: 'Node Pool Health', icon: '\u1F4BB' },
        { path: '/api/scalability/node-utilization-deep', name: 'Node Utilization', icon: '\u1F4BB' },
        { path: '/api/scalability/node-life-forecast', name: 'Node Life Forecast', icon: '\u1F4C5' },
        { path: '/api/scalability/node-pool-rightsize', name: 'Node Rightsize', icon: '\u2194' },
        { path: '/api/scalability/node-decomm', name: 'Node Decommission', icon: '\u1F5D1' },
      ],
      'Storage': [
        { path: '/api/scalability/pv-reclaim', name: 'PV Reclaim', icon: '\u1F4BE' },
        { path: '/api/scalability/storage-performance', name: 'Storage Performance', icon: '\u1F4BE' },
        { path: '/api/scalability/storage-tier', name: 'Storage Tier', icon: '\u1F4BE' },
        { path: '/api/scalability/volume-budget', name: 'Volume Budget', icon: '\u1F4BE' },
        { path: '/api/scalability/storage-forecast', name: 'Storage Forecast', icon: '\u1F4C8' },
        { path: '/api/scalability/storage-orphan', name: 'Storage Orphan', icon: '\u1F9F9' },
      ],
      'Scheduling & Density': [
        { path: '/api/scalability/scheduling-intel', name: 'Scheduling Intel', icon: '\u1F9ED' },
        { path: '/api/scalability/scheduler-fairness', name: 'Scheduler Fairness', icon: '\u2696' },
        { path: '/api/scalability/binpack-efficiency', name: 'Binpack', icon: '\u1F4E6' },
        { path: '/api/scalability/density-balance', name: 'Density Balance', icon: '\u2696' },
        { path: '/api/scalability/pod-density', name: 'Pod Density', icon: '\u1F4CF' },
        { path: '/api/scalability/fragmentation', name: 'Fragmentation', icon: '\u1F9F9' },
        { path: '/api/scalability/pod-affinity-spread', name: 'Affinity Spread', icon: '\u1F4CD' },
      ],
      'HA & DR': [
        { path: '/api/scalability/dr-readiness', name: 'DR Readiness', icon: '\u1F6E1' },
        { path: '/api/scalability/cluster-fault-tolerance', name: 'Fault Tolerance', icon: '\u1F6E1' },
        { path: '/api/scalability/pod-disruption-tolerance', name: 'Disruption Tolerance', icon: '\u1F6E1' },
        { path: '/api/scalability/eviction-risk', name: 'Eviction Risk', icon: '\u26A0' },
        { path: '/api/scalability/node-failure-blast', name: 'Node Failure Blast', icon: '\u1F4A5' },
        { path: '/api/scalability/ha-audit', name: 'HA Audit', icon: '\u1F6E1' },
      ],
      'Capacity & Forecast': [
        { path: '/api/scalability/capacity-headroom', name: 'Capacity Headroom', icon: '\u1F4CF' },
        { path: '/api/scalability/capacity-plan', name: 'Capacity Plan', icon: '\u1F4CB' },
        { path: '/api/scalability/capacity-forecast-deep', name: 'Capacity Forecast', icon: '\u1F4C8' },
        { path: '/api/scalability/cluster-pod-limit', name: 'Pod Limit', icon: '\u1F4CF' },
        { path: '/api/scalability/pdb-gap-analysis', name: 'PDB Gap', icon: '\u26A0' },
        { path: '/api/scalability/topology-spread-violation', name: 'Topo Spread', icon: '\u1F4CD' },
        { path: '/api/scalability/overcommit-deep', name: 'Overcommit Deep', icon: '\u1F4CA' },
        { path: '/api/scalability/resource-forecast', name: 'Resource Forecast', icon: '\u1F52E' },
        { path: '/api/scalability/bottleneck-predictor', name: 'Bottleneck Predictor', icon: '\u1F9ED' },
      ],
      'Quota & Multi-Tenant': [
        { path: '/api/scalability/quota-utilization', name: 'Quota Utilization', icon: '\u1F4CA' },
        { path: '/api/scalability/quota-saturation', name: 'Quota Saturation', icon: '\u26A0' },
        { path: '/api/scalability/quota-generator', name: 'Quota Generator', icon: '\u1F527' },
        { path: '/api/scalability/tenant-pressure', name: 'Tenant Pressure', icon: '\u1F3E2' },
        { path: '/api/scalability/namespace-isolation', name: 'NS Isolation', icon: '\u1F6E1' },
        { path: '/api/scalability/ns-consumption', name: 'NS Consumption', icon: '\u1F4CA' },
        { path: '/api/scalability/namespace-budget-enforce', name: 'Budget Enforce', icon: '\u1F4B0' },
      ],
      'Cleanup & Sustainability': [
        { path: '/api/scalability/orphan-cleanup', name: 'Orphan Cleanup', icon: '\u1F9F9' },
        { path: '/api/scalability/image-cleanup', name: 'Image Cleanup', icon: '\u1F9F9' },
        { path: '/api/scalability/green-computing', name: 'Green Computing', icon: '\u1F7E2' },
        { path: '/api/scalability/carbon-footprint', name: 'Carbon Footprint', icon: '\u1F7E2' },
        { path: '/api/scalability/resource-waste-deep', name: 'Waste Deep', icon: '\u1F4B8' },
      ],
      'Pressure & Capacity Forecast': [
        { path: '/api/scalability/mem-pressure-forecast', name: 'Mem Pressure Forecast', icon: '\u1F4CA' },
        { path: '/api/scalability/scale-concurrency', name: 'Scale Concurrency', icon: '\u2195' },
        { path: '/api/scalability/disruption-window', name: 'Disruption Window', icon: '\u1F6E1' },
        { path: '/api/scalability/request-efficiency', name: 'Request Efficiency', icon: '\u2696' },
        { path: '/api/scalability/bin-packing-score', name: 'Bin-Packing Score', icon: '\u1F4E6' },
        { path: '/api/scalability/multi-zone-ha', name: 'Multi-Zone HA', icon: '\u1F30D' },
        { path: '/api/scalability/hpa-effectiveness-v2', name: 'HPA Effective', icon: '\u1F4C9' },
        { path: '/api/scalability/scheduling-latency-v2', name: 'Sched Latency', icon: '\u23F1' },
        { path: '/api/scalability/capacity-headroom-v2', name: 'Capacity Headroom', icon: '\u1F4CF' },
        { path: '/api/scalability/burst-capacity', name: 'Burst Capacity', icon: '\u26A1' },
        { path: '/api/scalability/elasticity-index', name: 'Elasticity Index', icon: '\u1F4C8' },
        { path: '/api/scalability/scale-bottleneck', name: 'Scale Bottleneck', icon: '\u1F6A7' },
        { path: '/api/scalability/api-throttle-risk', name: 'API Throttle', icon: '\u1F525' },
        { path: '/api/scalability/pod-density-opt', name: 'Pod Density', icon: '\u1F4E6' },
        { path: '/api/scalability/overcommit-forecast', name: 'Overcommit', icon: '\u1F4C9' },
      ],
    },
  },
  'Product': {
    color: '#58a6ff',
    icon: '\u1F4C2',
    subcategories: {
      'Service & Traffic': [
        { path: '/api/product/service-connectivity', name: 'Service Connectivity', icon: '\u1F517' },
        { path: '/api/product/service-catalog', name: 'Service Catalog', icon: '\u1F4C2' },
        { path: '/api/product/service-dependency-map', name: 'Service Dependencies', icon: '\u1F50D' },
        { path: '/api/product/service-topology', name: 'Service Topology', icon: '\u1F5FA' },
        { path: '/api/product/traffic-flow', name: 'Traffic Flow', icon: '\u1F500' },
        { path: '/api/product/traffic-spike-guard', name: 'Traffic Spike Guard', icon: '\u1F6A8' },
        { path: '/api/product/east-west-traffic', name: 'East-West Traffic', icon: '\u2194' },
      ],
      'Mesh & Gateway': [
        { path: '/api/product/mesh-health', name: 'Service Mesh Health', icon: '\u1F575' },
        { path: '/api/product/svc-mesh-readiness', name: 'Mesh Readiness', icon: '\u1F310' },
        { path: '/api/product/mesh-injection', name: 'Mesh Injection', icon: '\u1F500' },
        { path: '/api/product/ingress-health', name: 'Ingress Health', icon: '\u1F310' },
        { path: '/api/product/api-gateway-health', name: 'API Gateway', icon: '\u1F6A7' },
        { path: '/api/product/ingress-conflict', name: 'Ingress Conflict', icon: '\u26A0' },
      ],
      'Endpoints': [
        { path: '/api/product/endpoint-dns-health', name: 'Endpoint & DNS', icon: '\u1F310' },
        { path: '/api/product/endpoint-health-deep', name: 'Endpoint Health Deep', icon: '\u2713' },
        { path: '/api/product/endpoint-mismatch', name: 'Endpoint Mismatch', icon: '\u26A0' },
        { path: '/api/product/endpoint-slice', name: 'Endpoint Slices', icon: '\u1F4CA' },
      ],
      'Workload Health': [
        { path: '/api/product/workload-criticality', name: 'Workload Criticality', icon: '\u26A0' },
        { path: '/api/product/workload-efficiency', name: 'Workload Efficiency', icon: '\u2696' },
        { path: '/api/product/workload-fingerprint', name: 'Workload Fingerprint', icon: '\u1F194' },
        { path: '/api/product/canary-health', name: 'Canary Health', icon: '\u1F4AB' },
        { path: '/api/product/reliability-scorecard', name: 'Reliability Scorecard', icon: '\u1F4CB' },
        { path: '/api/product/golden-signals', name: 'Golden Signals', icon: '\u1F4A1' },
      ],
      'Config & Labels': [
        { path: '/api/product/configmap-size', name: 'ConfigMap Size', icon: '\u1F4C1' },
        { path: '/api/product/config-audit', name: 'Config Audit', icon: '\u1F4DC' },
        { path: '/api/product/secret-mount-audit', name: 'Secret Mount', icon: '\u1F511' },
        { path: '/api/product/label-propagation', name: 'Label Propagation', icon: '\u1F3F7' },
        { path: '/api/product/cronjob-orphan-audit', name: 'CronJob Orphan', icon: '\u23F0' },
        { path: '/api/product/env-var-drift-detect', name: 'Env Var Drift', icon: '\u1F500' },
        { path: '/api/product/dns-record-audit', name: 'DNS Record Audit', icon: '\u1F310' },
        { path: '/api/product/workload-startup-profile', name: 'Startup Profile', icon: '\u1F680' },
        { path: '/api/product/config-warmstart', name: 'Config Warmstart', icon: '\u23F1' },
        { path: '/api/product/label-hygiene', name: 'Label Hygiene', icon: '\u1F3F7' },
        { path: '/api/product/ownership-map', name: 'Ownership Map', icon: '\u1F464' },
      ],
      'Scheduling & Placement': [
        { path: '/api/product/placement-score', name: 'Placement Score', icon: '\u1F4CD' },
        { path: '/api/product/topology-spread', name: 'Topology Spread', icon: '\u1F5FA' },
        { path: '/api/product/replica-distribution', name: 'Replica Distribution', icon: '\u1F4CA' },
        { path: '/api/product/affinity-conflict', name: 'Affinity Conflict', icon: '\u26A0' },
        { path: '/api/product/taint-toleration', name: 'Taint/Toleration', icon: '\u26D4' },
        { path: '/api/product/antiaffinity-ha', name: 'HA Readiness', icon: '\u1F6E1' },
      ],
      'Storage & PVC': [
        { path: '/api/product/pvc-health', name: 'PVC Health', icon: '\u1F4BE' },
        { path: '/api/product/pv-access', name: 'PV Access', icon: '\u1F4BE' },
        { path: '/api/product/config-mount-risk', name: 'Config Mount Risk', icon: '\u26A0' },
        { path: '/api/product/pvc-io-health', name: 'PVC I/O Health', icon: '\u1F4BE' },
      ],
      'API Governance': [
        { path: '/api/product/api-version-governance', name: 'API Version', icon: '\u1F4C4' },
        { path: '/api/product/api-deprecation', name: 'API Deprecation', icon: '\u26A0' },
        { path: '/api/product/slo-compliance', name: 'SLO Compliance', icon: '\u1F3AF' },
        { path: '/api/product/priority-class-audit', name: 'Priority Class', icon: '\u26A1' },
      ],
      'Network & Exposure': [
        { path: '/api/product/service-exposure-map', name: 'Service Exposure', icon: '\u1F310' },
        { path: '/api/product/workload-interdependency', name: 'Interdependency', icon: '\u1F517' },
        { path: '/api/product/dns-resolution-health', name: 'DNS Health', icon: '\u1F310' },
        { path: '/api/product/storage-class-audit', name: 'Storage Class', icon: '\u1F4BE' },
        { path: '/api/product/cost-attribution', name: 'Cost Attribution', icon: '\u1F4B0' },
        { path: '/api/product/quota-forecast', name: 'Quota Forecast', icon: '\u1F4C9' },
        { path: '/api/product/mesh-readiness-deep', name: 'Mesh Ready', icon: '\u1F310' },
        { path: '/api/product/env-secret-leak', name: 'Secret Leak', icon: '\u1F510' },
        { path: '/api/product/probe-coverage-gap', name: 'Probe Gap', icon: '\u1F50C' },
        { path: '/api/product/gpu-audit', name: 'GPU Audit', icon: '\u1F3AE' },
        { path: '/api/product/limit-range-audit', name: 'LimitRange', icon: '\u1F4CF' },
        { path: '/api/product/tenant-isolation', name: 'Tenant Iso', icon: '\u1F3D7' },
        { path: '/api/product/resource-share', name: 'Resource Share', icon: '\u2696' },
      ],
    },
  },
  'Deployment': {
    color: '#3fb950',
    icon: '\u1F680',
    subcategories: {
      'GitOps & Helm': [
        { path: '/api/deployment/helm-health', name: 'Helm Health', icon: '\u1F4E6' },
        { path: '/api/deployment/helm-drift-monitor', name: 'Helm Drift', icon: '\u1F4E6' },
        { path: '/api/deployment/gitops-audit', name: 'GitOps Audit', icon: '\u1F4C1' },
        { path: '/api/deployment/gitops-sync-deep', name: 'GitOps Sync', icon: '\u1F504' },
      ],
      'Rollout & Progressive': [
        { path: '/api/deployment/progressive-delivery', name: 'Progressive Delivery', icon: '\u1F4C8' },
        { path: '/api/deployment/rollout-health', name: 'Rollout Health', icon: '\u2728' },
        { path: '/api/deployment/update-strategy', name: 'Update Strategy', icon: '\u1F504' },
        { path: '/api/deployment/surge-capacity', name: 'Surge Capacity', icon: '\u26A1' },
      ],
      'Rollback Safety': [
        { path: '/api/deployment/rollback-risk', name: 'Rollback Risk', icon: '\u21A9' },
        { path: '/api/deployment/rollback-safety', name: 'Rollback Safety', icon: '\u21A9' },
        { path: '/api/deployment/rollback-simulator', name: 'Rollback Simulator', icon: '\u21A9' },
      ],
      'Image Management': [
        { path: '/api/deployment/image-hygiene', name: 'Image Hygiene', icon: '\u1F4F7' },
        { path: '/api/deployment/image-freshness', name: 'Image Freshness', icon: '\u1F34E' },
        { path: '/api/deployment/image-pull-latency', name: 'Image Pull Latency', icon: '\u1F4F7' },
        { path: '/api/deployment/image-pull-audit', name: 'Image Pull Audit', icon: '\u1F4F7' },
      ],
      'Readiness & Gates': [
        { path: '/api/deployment/preflight-check', name: 'Preflight Check', icon: '\u2705' },
        { path: '/api/deployment/readiness-gate', name: 'Readiness Gate', icon: '\u2705' },
        { path: '/api/deployment/deploy-window', name: 'Deploy Window', icon: '\u1F4C5' },
        { path: '/api/deployment/change-freeze', name: 'Change Freeze', icon: '\u2744' },
      ],
      'Sidecar & Quota': [
        { path: '/api/deployment/sidecar-injection-audit', name: 'Sidecar Injection', icon: '\u1F916' },
        { path: '/api/deployment/resource-quota-drift', name: 'Quota Drift', icon: '\u2696' },
        { path: '/api/deployment/resource-limit-coverage', name: 'Limit Coverage', icon: '\u2696' },
        { path: '/api/deployment/ephemeral-storage-quota', name: 'Ephemeral Storage', icon: '\u1F4BE' },
      ],
      'DORA Metrics': [
        { path: '/api/deployment/dora-metrics', name: 'DORA Metrics', icon: '\u1F4C8' },
        { path: '/api/deployment/deploy-frequency', name: 'Deploy Frequency', icon: '\u1F4C8' },
        { path: '/api/deployment/deploy-heatmap', name: 'Deploy Heatmap', icon: '\u1F525' },
      ],
      'Probe Health': [
        { path: '/api/deployment/probe-compliance', name: 'Probe Compliance', icon: '\u1FA78' },
        { path: '/api/deployment/probe-generator', name: 'Probe Generator', icon: '\u1F527' },
        { path: '/api/deployment/probe-timeout-audit', name: 'Probe Timeout', icon: '\u23F1' },
        { path: '/api/deployment/init-container-health', name: 'Init Container', icon: '\u1F9E9' },
        { path: '/api/deployment/rollout-blocker-detect', name: 'Rollout Blocker', icon: '\u26D4' },
        { path: '/api/deployment/termination-grace-audit', name: 'Termination Grace', icon: '\u23F3' },
        { path: '/api/deployment/max-surge-audit', name: 'Max Surge', icon: '\u2191' },
        { path: '/api/deployment/graceful-shutdown', name: 'Graceful Shutdown', icon: '\u1F6D1' },
      ],
      'Config & Drift': [
        { path: '/api/deployment/config-sync', name: 'Config Sync', icon: '\u1F504' },
        { path: '/api/deployment/config-snapshot', name: 'Config Snapshot', icon: '\u1F4F8' },
        { path: '/api/deployment/revision-drift', name: 'Revision Drift', icon: '\u1F501' },
        { path: '/api/deployment/revision-history-hygiene', name: 'Revision History', icon: '\u1F4DC' },
        { path: '/api/deployment/env-config-drift', name: 'Env Config Drift', icon: '\u1F501' },
        { path: '/api/deployment/immutable-config-audit', name: 'Immutable Config', icon: '\u1F512' },
      ],
      'Reproducibility & Compliance': [
        { path: '/api/deployment/deploy-reproducibility', name: 'Reproducibility', icon: '\u1F50D' },
        { path: '/api/deployment/update-compliance-deep', name: 'Update Compliance', icon: '\u2705' },
        { path: '/api/deployment/restart-policy-deep', name: 'Restart Policy', icon: '\u1F501' },
        { path: '/api/deployment/graceful-shutdown-audit', name: 'Graceful Shutdown', icon: '\u1F6D1' },
        { path: '/api/deployment/rollout-speed', name: 'Rollout Speed', icon: '\u23F1' },
        { path: '/api/deployment/deploy-conflict', name: 'Deploy Conflict', icon: '\u26A0' },
        { path: '/api/deployment/image-consistency', name: 'Image Consist', icon: '\u1F4F7' },
        { path: '/api/deployment/config-reload-readiness', name: 'Config Reload', icon: '\u1F504' },
        { path: '/api/deployment/deploy-freeze-status', name: 'Deploy Freeze', icon: '\u2744' },
        { path: '/api/deployment/manifest-drift', name: 'Manifest Drift', icon: '\u1F503' },
        { path: '/api/deployment/preflight-check', name: 'Pre-Flight', icon: '\u2708' },
        { path: '/api/deployment/helm-health', name: 'Helm Health', icon: '\u2693' },
      ],
    },
  },
  'Documentation': {
    color: '#8b949e',
    icon: '\u1F4DA',
    subcategories: {
      'Overview': [
        { path: '/api/docs/platform-scorecard', name: 'Platform Scorecard', icon: '\u1F4CB' },
        { path: '/api/docs/exec-dashboard', name: 'Executive Dashboard', icon: '\u1F4BC' },
        { path: '/api/docs/platform-maturity', name: 'Platform Maturity', icon: '\u1F3AF' },
        { path: '/api/docs/resource-inventory', name: 'Resource Inventory', icon: '\u1F4C2' },
        { path: '/api/docs/platform-risk-heatmap', name: 'Risk Heatmap', icon: '\u1F525' },
      ],
      'Maturity & Playbooks': [
        { path: '/api/docs/workload-maturity-matrix', name: 'Maturity Matrix', icon: '\u1F3AF' },
        { path: '/api/docs/incident-playbook', name: 'Incident Playbook', icon: '\u1F691' },
        { path: '/api/docs/tech-debt-radar', name: 'Tech Debt Radar', icon: '\u1F4A1' },
        { path: '/api/docs/sre-scorecard', name: 'SRE Scorecard', icon: '\u1F3AF' },
        { path: '/api/docs/compliance-crosswalk', name: 'Compliance Crosswalk', icon: '\u1F4DC' },
        { path: '/api/docs/cost-optimization-roadmap', name: 'Cost Roadmap', icon: '\u1F4B0' },
        { path: '/api/docs/security-posture-trend', name: 'Security Posture', icon: '\u1F6E1' },
        { path: '/api/docs/capacity-planning-report', name: 'Capacity Report', icon: '\u1F4CF' },
      ],
      'API Docs': [
        { path: '/api/docs/api-coverage-map', name: 'API Coverage Map', icon: '\u1F5FA' },
        { path: '/api/docs/api-explorer', name: 'API Explorer', icon: '\u1F50D' },
        { path: '/api/docs/api-quality', name: 'API Quality', icon: '\u1F50D' },
        { path: '/api/docs/api-coverage-gap', name: 'Coverage Gap', icon: '\u1F50D' },
        { path: '/api/docs/api-governance-score', name: 'API Governance', icon: '\u1F4DC' },
      ],
      'Operations Docs': [
        { path: '/api/docs/action-priority-matrix', name: 'Action Priority Matrix', icon: '\u1F4CB' },
        { path: '/api/docs/oncall-readiness', name: 'Oncall Readiness', icon: '\u1F6E1' },
        { path: '/api/docs/runbook-coverage', name: 'Runbook Coverage', icon: '\u1F4D6' },
        { path: '/api/docs/upgrade-planner', name: 'Upgrade Planner', icon: '\u2B06' },
        { path: '/api/docs/training-readiness', name: 'Training Readiness', icon: '\u1F4DA' },
        { path: '/api/docs/backup-compliance-deep', name: 'Backup Compliance', icon: '\u1F4BE' },
        { path: '/api/docs/label-taxonomy-standard', name: 'Label Taxonomy', icon: '\u1F3F7' },
        { path: '/api/docs/change-impact-brief', name: 'Change Impact', icon: '\u1F4C8' },
        { path: '/api/docs/ownership-registry', name: 'Ownership Registry', icon: '\u1F465' },
        { path: '/api/docs/release-note-gen', name: 'Release Notes', icon: '\u1F4DD' },
        { path: '/api/docs/incident-postmortem', name: 'Postmortem', icon: '\u1F691' },
        { path: '/api/docs/cluster-runbook-gen', name: 'Cluster Runbook', icon: '\u1F4D6' },
        { path: '/api/docs/api-drift-detector', name: 'API Drift', icon: '\u1F500' },
        { path: '/api/docs/resource-topology-doc', name: 'Topology Map', icon: '\u1F5FA' },
        { path: '/api/docs/compliance-report', name: 'Compliance Report', icon: '\u1F4DC' },
        { path: '/api/docs/slo-handbook', name: 'SLO Handbook', icon: '\u1F4CA' },
        { path: '/api/docs/cluster-faq', name: 'Cluster FAQ', icon: '\u2753' },
        { path: '/api/docs/dr-plan-gen', name: 'DR Plan', icon: '\u1F6E0' },
        { path: '/api/docs/adr-generator', name: 'ADR Gen', icon: '\u1F4D1' },
        { path: '/api/docs/migration-checklist', name: 'Migration Checklist', icon: '\u2702' },
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
          <div style="font-size:12px;font-weight:600;color:#8b949e;text-transform:uppercase;letter-spacing:0.5px;margin-bottom:8px;padding-bottom:4px;border-bottom:1px solid #21262d;">
            ${subcat} <span style="color:#484f58;font-weight:400;">(${endpoints.length})</span>
          </div>
          <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(240px,1fr));gap:8px;">
      `;
      for (const ep of endpoints) {
        const cardId = btoa(ep.path).replace(/=/g, '');
        dimHtml += `
          <div class="audit-card" data-name="${ep.name.toLowerCase()}" data-dim="${dim}" 
               id="audit-card-${cardId}" 
               style="border:1px solid #30363d;border-radius:6px;padding:10px;cursor:pointer;background:#0d1117;transition:border-color 0.2s;"
               onclick="window.loadAuditDetail('${ep.path}','${ep.name.replace(/'/g, '')}')">
            <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:4px;">
              <span style="font-size:12px;font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;">${ep.icon} ${ep.name}</span>
              <span class="audit-score" id="score-${cardId}" style="font-size:11px;font-weight:700;padding:1px 6px;border-radius:3px;background:#21262d;color:#8b949e;flex-shrink:0;">--</span>
            </div>
            <div class="audit-status" id="status-${cardId}" style="font-size:10px;color:#8b949e;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;">Loading...</div>
          </div>
        `;
      }
      dimHtml += `</div></div>`;
    }

    dimHtml += `</div></div>`;
  }
  document.getElementById('audit-dimensions').innerHTML = dimHtml;

  // Fetch all endpoints
  for (const [dim, info] of Object.entries(AUDIT_STRUCTURE)) {
    for (const endpoints of Object.values(info.subcategories)) {
      for (const ep of endpoints) {
        const cardId = btoa(ep.path).replace(/=/g, '');
        fetchJSON(ep.path)
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
              if (score >= 80) {
                scoreEl.style.background = '#1a3a2a'; scoreEl.style.color = '#3fb950';
              } else if (score >= 60) {
                scoreEl.style.background = '#3a3a1a'; scoreEl.style.color = '#d29922';
              } else if (score >= 40) {
                scoreEl.style.background = '#3a2a1a'; scoreEl.style.color = '#f0883e';
              } else {
                scoreEl.style.background = '#3a1a1a'; scoreEl.style.color = '#f85149';
              }
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
      }
    }
  }

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
