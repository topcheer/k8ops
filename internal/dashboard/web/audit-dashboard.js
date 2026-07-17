// audit-dashboard.js — Unified Audit Dashboard showing all audit endpoints by dimension
import { escapeHtml, fetchJSON } from './modules/utils.js';

// All audit endpoints organized by dimension
const AUDIT_ENDPOINTS = {
  'Product': [
    { path: '/api/product/init-container-audit', name: 'Init Container Reliability', icon: '\u2699' },
    { path: '/api/product/cronjob-schedule', name: 'CronJob Schedule Conflicts', icon: '\u23F0' },
    { path: '/api/product/endpoint-dns-health', name: 'Endpoint & DNS Health', icon: '\u1F310' },
    { path: '/api/product/external-secret-health', name: 'External Secrets Health', icon: '\u1F511' },
    { path: '/api/product/mesh-health', name: 'Service Mesh & mTLS', icon: '\u1F575' },
    { path: '/api/product/hpa-gap', name: 'HPA Target Gap', icon: '\u2195' },
    { path: '/api/product/label-hygiene', name: 'Label Hygiene', icon: '\u1F3F7' },
    { path: '/api/product/orphaned-resources', name: 'Orphaned Resources', icon: '\u1F4E6' },
    { path: '/api/product/pvc-health', name: 'PVC Health', icon: '\u1F4BE' },
    { path: '/api/product/qos-priority', name: 'QoS & Priority', icon: '\u26A0' },
    { path: '/api/product/service-connectivity', name: 'Service Connectivity', icon: '\u1F517' },
    { path: '/api/product/configmap-size', name: 'ConfigMap Size Pressure', icon: '\u1F4C1' },
    { path: '/api/product/workload-criticality', name: 'Workload Criticality', icon: '\u26A0' },
    { path: '/api/product/env-var-audit', name: 'Env Var Audit', icon: '\u1F527' },
    { path: '/api/product/placement-score', name: 'Placement Score', icon: '\u1F4CD' },
    { path: '/api/product/runtime-class', name: 'Runtime Class', icon: '\u2699' },
    { path: '/api/product/service-catalog', name: 'Service Catalog', icon: '\u1F4C2' },
    { path: '/api/product/service-dependency-map', name: 'Service Dependency Map', icon: '\u1F50D' },
    { path: '/api/product/label-score', name: 'Label Hygiene Score', icon: '\u1F3F7' },
    { path: '/api/product/namespace-quota-map', name: 'Namespace Quota Map', icon: '\u1F4CA' },
    { path: '/api/product/ingress-health', name: 'Ingress Health', icon: '\u1F310' },
    { path: '/api/product/network-policy', name: 'Network Policy Audit', icon: '\u1F6E1' },
    { path: '/api/product/slo-compliance', name: 'SLO Compliance', icon: '\u1F3AF' },
    { path: '/api/product/backup-compliance', name: 'Backup Compliance', icon: '\u1F4BE' },
    { path: '/api/product/ownership-map', name: 'Ownership Map', icon: '\u1F464' },
    { path: '/api/product/api-version-governance', name: 'API Version Governance', icon: '\u1F4C4' },
  ],
  'Deployment': [
    { path: '/api/deployment/startup-latency', name: 'Startup Latency', icon: '\u23F1' },
    { path: '/api/deployment/progressive-delivery', name: 'Progressive Delivery', icon: '\u1F4C8' },
    { path: '/api/deployment/rs-staleness', name: 'ReplicaSet Staleness', icon: '\u1F501' },
    { path: '/api/deployment/surge-risk', name: 'Surge & Rolling Risk', icon: '\u26A0' },
    { path: '/api/deployment/helm-health', name: 'Helm & GitOps', icon: '\u1F4E6' },
    { path: '/api/deployment/replica-availability', name: 'Replica Availability', icon: '\u2713' },
    { path: '/api/deployment/image-hygiene', name: 'Image Hygiene', icon: '\u1F4F7' },
    { path: '/api/deployment/rollout-health', name: 'Rollout Health', icon: '\u2728' },
    { path: '/api/deployment/probe-audit', name: 'Probe Compliance', icon: '\u1FA78' },
    { path: '/api/deployment/config-sync', name: 'Config Sync', icon: '\u1F504' },
    { path: '/api/deployment/upgrade-impact', name: 'Upgrade Impact Simulator', icon: '\u2B06' },
    { path: '/api/deployment/deploy-window', name: 'Deploy Window', icon: '\u1F4C5' },
    { path: '/api/deployment/change-freeze', name: 'Change Freeze', icon: '\u2744' },
    { path: '/api/deployment/gitops-audit', name: 'GitOps Audit', icon: '\u1F4C1' },
    { path: '/api/deployment/gitops-sync-deep', name: 'GitOps Sync Deep', icon: '\u1F504' },
    { path: '/api/deployment/rollback-risk', name: 'Rollback Risk', icon: '\u21A9' },
    { path: '/api/deployment/image-freshness', name: 'Image Freshness', icon: '\u1F34E' },
    { path: '/api/deployment/readiness-gate', name: 'Readiness Gate', icon: '\u2705' },
    { path: '/api/deployment/dora-metrics', name: 'DORA Metrics', icon: '\u1F4C8' },
    { path: '/api/deployment/release-gate', name: 'Release Gate', icon: '\u1F6E1' },
    { path: '/api/deployment/deploy-risk', name: 'Deploy Risk', icon: '\u26A0' },
    { path: '/api/deployment/config-snapshot', name: 'Config Snapshot', icon: '\u1F4F8' },
    { path: '/api/deployment/probe-generator', name: 'Probe Generator', icon: '\u1F527' },
    { path: '/api/deployment/update-strategy-auditor', name: 'Update Strategy Auditor', icon: '\u1F504' },
    { path: '/api/deployment/ephemeral-storage', name: 'Ephemeral Storage', icon: '\u1F4BE' },
    { path: '/api/deployment/probe-compliance', name: 'Probe Compliance', icon: '\u1FA78' },
    { path: '/api/deployment/graceful-shutdown', name: 'Graceful Shutdown', icon: '\u1F6D1' },
    { path: '/api/deployment/update-strategy', name: 'Update Strategy', icon: '\u1F504' },
    { path: '/api/deployment/revision-history', name: 'Revision History', icon: '\u1F4DC' },
  ],
  'Operations': [
    { path: '/api/operations/metrics-pipeline', name: 'Metrics Pipeline', icon: '\u1F4CA' },
    { path: '/api/operations/grafana-health', name: 'Grafana Dashboards', icon: '\u1F4C4' },
    { path: '/api/operations/audit-log-health', name: 'Audit Log Pipeline', icon: '\u1F4DD' },
    { path: '/api/operations/prom-health', name: 'Prometheus Rules', icon: '\u1F525' },
    { path: '/api/operations/alertmanager-health', name: 'Alertmanager Health', icon: '\u1F514' },
    { path: '/api/operations/api-load', name: 'API Server Load', icon: '\u1F4E6' },
    { path: '/api/operations/kubelet-health', name: 'Kubelet Health', icon: '\u1F3E2' },
    { path: '/api/operations/etcd-health', name: 'Etcd Health', icon: '\u1F50C' },
    { path: '/api/operations/chaos-readiness', name: 'Chaos Readiness', icon: '\u1F4A5' },
    { path: '/api/operations/signal-correlation', name: 'Signal Correlation', icon: '\u1F50D' },
    { path: '/api/operations/api-access-pattern', name: 'API Access Pattern', icon: '\u1F511' },
    { path: '/api/operations/restart-pattern', name: 'Restart Pattern', icon: '\u1F501' },
    { path: '/api/operations/cni-health', name: 'CNI Health', icon: '\u1F310' },
    { path: '/api/operations/kube-proxy-health', name: 'Kube-Proxy Health', icon: '\u1F50C' },
    { path: '/api/operations/drain-impact', name: 'Drain Impact', icon: '\u1F6A6' },
    { path: '/api/operations/resource-topology', name: 'Resource Topology', icon: '\u1F5FA' },
    { path: '/api/operations/pod-health-index', name: 'Pod Health Index', icon: '\u1F493' },
    { path: '/api/operations/obs-coverage', name: 'Observability Coverage', icon: '\u1F441' },
    { path: '/api/operations/obs-cardinality', name: 'Obs Cardinality', icon: '\u1F4CF' },
    { path: '/api/operations/node-pressure', name: 'Node Pressure', icon: '\u26A0' },
    { path: '/api/operations/pdb-audit', name: 'PDB Audit', icon: '\u1F6E1' },
    { path: '/api/operations/throttle-risk', name: 'Throttle Risk', icon: '\u1F4A7' },
    { path: '/api/operations/event-storm', name: 'Event Storm', icon: '\u26A1' },
    { path: '/api/operations/incident-correlation', name: 'Incident Correlation', icon: '\u1F50D' },
    { path: '/api/operations/pdb-generator', name: 'PDB Generator', icon: '\u1F527' },
    { path: '/api/operations/probe-latency', name: 'Probe Latency', icon: '\u23F1' },
    { path: '/api/operations/endpoint-probe', name: 'Endpoint Probe', icon: '\u1F50D' },
    { path: '/api/operations/health-trend', name: 'Health Trend', icon: '\u1F4C8' },
    { path: '/api/operations/restart-analyzer', name: 'Restart Analyzer', icon: '\u1F501' },
  ],
  'Security': [
    { path: '/api/security/sa-token-audit', name: 'SA Token Rotation', icon: '\u1F511' },
    { path: '/api/security/pss-scorecard', name: 'PSS Compliance', icon: '\u1F6E1' },
    { path: '/api/security/kyverno-compliance', name: 'Kyverno Policies', icon: '\u1F4DC' },
    { path: '/api/security/image-vuln', name: 'Image Vulnerability', icon: '\u26A0' },
    { path: '/api/security/opa-compliance', name: 'OPA/Gatekeeper', icon: '\u1F6AB' },
    { path: '/api/security/sec-drift', name: 'Security Context Drift', icon: '\u1F50D' },
    { path: '/api/security/cert-expiry', name: 'Certificate Expiry', icon: '\u1F510' },
    { path: '/api/security/secret-scan', name: 'Secret Scan', icon: '\u1F576' },
    { path: '/api/security/attack-surface', name: 'Attack Surface', icon: '\u1F575' },
    { path: '/api/security/secret-rotation-v2', name: 'Secret Rotation v2', icon: '\u1F504' },
    { path: '/api/security/cert-inventory', name: 'Cert Inventory', icon: '\u1F4DC' },
    { path: '/api/security/audit-policy', name: 'Audit Policy', icon: '\u1F4DD' },
    { path: '/api/security/seccomp-audit', name: 'Seccomp Audit', icon: '\u1F6E1' },
    { path: '/api/security/mac-audit', name: 'MAC Audit', icon: '\u1F512' },
    { path: '/api/security/compliance-map', name: 'Compliance Map', icon: '\u1F4CB' },
    { path: '/api/security/compliance-posture', name: 'Compliance Posture', icon: '\u1F4DC' },
    { path: '/api/security/policy-drift', name: 'Policy Drift', icon: '\u1F50D' },
    { path: '/api/security/policy-governance', name: 'Policy Governance', icon: '\u1F4DC' },
    { path: '/api/security/audit-trail', name: 'Audit Trail', icon: '\u1F4DD' },
    { path: '/api/security/supply-chain', name: 'Supply Chain Security', icon: '\u1F4E6' },
    { path: '/api/security/hardening-score', name: 'Hardening Score', icon: '\u1F6E1' },
    { path: '/api/security/fix-plan', name: 'Security Fix Plan', icon: '\u1F527' },
    { path: '/api/security/secret-exposure', name: 'Secret Exposure', icon: '\u1F441' },
    { path: '/api/security/netpol-generator', name: 'NetworkPolicy Generator', icon: '\u1F6E1' },
    { path: '/api/security/env-leak-scanner', name: 'Env Leak Scanner', icon: '\u1F576' },
    { path: '/api/security/trust-chain', name: 'Trust Chain', icon: '\u1F512' },
    { path: '/api/security/blast-radius', name: 'Blast Radius', icon: '\u1F4A5' },
    { path: '/api/security/net-policy-effectiveness', name: 'Net Policy Effectiveness', icon: '\u1F50D' },
  ],
  'Scalability': [
    { path: '/api/scalability/pv-reclaim', name: 'PV Reclaim & Waste', icon: '\u1F4BE' },
    { path: '/api/scalability/alloc-efficiency', name: 'Alloc Efficiency', icon: '\u2696' },
    { path: '/api/scalability/hpa-performance', name: 'HPA Performance', icon: '\u2195' },
    { path: '/api/scalability/cost-waste', name: 'Cost Waste', icon: '\u1F4B0' },
    { path: '/api/scalability/node-lifecycle', name: 'Node Lifecycle', icon: '\u1F578' },
    { path: '/api/scalability/node-pool-health', name: 'Node Pool Health', icon: '\u1F4BB' },
    { path: '/api/scalability/tenant-pressure', name: 'Tenant Pressure', icon: '\u1F3E2' },
    { path: '/api/scalability/overcommit', name: 'Resource Overcommit', icon: '\u26A0' },
    { path: '/api/scalability/unit-economics', name: 'Unit Economics', icon: '\u1F4B0' },
    { path: '/api/scalability/green-computing', name: 'Green Computing', icon: '\u1F7E2' },
    { path: '/api/scalability/commit-optimizer', name: 'Commit Optimizer', icon: '\u1F4C8' },
    { path: '/api/scalability/hpa-behavior', name: 'HPA Behavior', icon: '\u2195' },
    { path: '/api/scalability/density-balance', name: 'Density Balance', icon: '\u2696' },
    { path: '/api/scalability/volume-budget', name: 'Volume Budget', icon: '\u1F4BE' },
    { path: '/api/scalability/scaling-simulator', name: 'Scaling Simulator', icon: '\u1F4C8' },
    { path: '/api/scalability/cost-allocation', name: 'Cost Allocation', icon: '\u1F4B0' },
    { path: '/api/scalability/budget-alert', name: 'Budget Alert', icon: '\u26A0' },
    { path: '/api/scalability/cost-intelligence', name: 'Cost Intelligence', icon: '\u1F9EE' },
    { path: '/api/scalability/chargeback', name: 'Chargeback', icon: '\u1F4B3' },
    { path: '/api/scalability/idle-waste', name: 'Idle Waste', icon: '\u1F4A9' },
    { path: '/api/scalability/spot-readiness', name: 'Spot Readiness', icon: '\u26A1' },
    { path: '/api/scalability/dr-readiness', name: 'DR Readiness', icon: '\u1F6E1' },
    { path: '/api/scalability/quota-utilization', name: 'Quota Utilization', icon: '\u1F4CA' },
    { path: '/api/scalability/ip-cidr-utilization', name: 'IP CIDR Utilization', icon: '\u1F310' },
    { path: '/api/scalability/fragmentation', name: 'Fragmentation', icon: '\u1F9F9' },
    { path: '/api/scalability/scheduling-intel', name: 'Scheduling Intel', icon: '\u1F9ED' },
    { path: '/api/scalability/ns-consumption', name: 'NS Consumption', icon: '\u1F4CA' },
    { path: '/api/scalability/capacity-forecast-deep', name: 'Capacity Forecast', icon: '\u1F4C8' },
    { path: '/api/scalability/request-accuracy', name: 'Request Accuracy', icon: '\u1F3AF' },
    { path: '/api/scalability/orphan-cleanup', name: 'Orphan Cleanup', icon: '\u1F9F9' },
    { path: '/api/scalability/cost-anomaly', name: 'Cost Anomaly', icon: '\u26A0' },
    { path: '/api/scalability/right-size-engine', name: 'Right Size Engine', icon: '\u1F4CF' },
    { path: '/api/scalability/storage-performance', name: 'Storage Performance', icon: '\u1F4BE' },
    { path: '/api/scalability/quota-generator', name: 'Quota Generator', icon: '\u1F527' },
    { path: '/api/scalability/image-cleanup', name: 'Image Cleanup', icon: '\u1F9F9' },
    { path: '/api/scalability/storage-tier', name: 'Storage Tier', icon: '\u1F4BE' },
  ],
  'Documentation': [
    { path: '/api/docs/platform-scorecard', name: 'Platform Scorecard', icon: '\u1F4CB' },
    { path: '/api/docs/resource-inventory', name: 'Resource Inventory', icon: '\u1F4C2' },
    { path: '/api/docs/training-readiness', name: 'Training Readiness', icon: '\u1F4DA' },
    { path: '/api/docs/exec-dashboard', name: 'Executive Dashboard', icon: '\u1F4BC' },
    { path: '/api/docs/platform-maturity', name: 'Platform Maturity', icon: '\u1F3AF' },
    { path: '/api/docs/platform-changelog', name: 'Platform Changelog', icon: '\u1F4DD' },
    { path: '/api/docs/api-quality', name: 'API Quality', icon: '\u1F50D' },
    { path: '/api/docs/api-coverage-map', name: 'API Coverage Map', icon: '\u1F5FA' },
    { path: '/api/docs/api-explorer', name: 'API Explorer', icon: '\u1F50D' },
    { path: '/api/docs/cluster-maturity', name: 'Cluster Maturity', icon: '\u1F3AF' },
    { path: '/api/docs/platform-insights', name: 'Platform Insights', icon: '\u1F4A1' },
    { path: '/api/docs/action-priority-matrix', name: 'Action Priority Matrix', icon: '\u1F4CB' },
  ],
};

const DIMENSION_COLORS = {
  'Product': '#58a6ff',
  'Deployment': '#3fb950',
  'Operations': '#d29922',
  'Security': '#f85149',
  'Scalability': '#bc8cff',
  'Documentation': '#8b949e',
};

window.loadAuditDashboard = function() {
  const container = document.getElementById('audit-dashboard-content');
  if (!container) return;

  container.innerHTML = '<div style="text-align:center;padding:40px;color:#8b949e;">Loading audit data...</div>';

  // Render overall summary cards
  let html = `
    <div style="margin-bottom:24px;">
      <h2 style="margin:0 0 8px 0;font-size:18px;">Audit Dashboard</h2>
      <p style="margin:0;color:#8b949e;font-size:13px;">Real-time health scores across all audit dimensions. Click any card for details.</p>
    </div>
    <div id="audit-summary-grid" style="display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:12px;margin-bottom:24px;"></div>
    <div id="audit-dimensions"></div>
  `;
  container.innerHTML = html;

  // Fetch all endpoints in parallel
  const allAudits = [];
  let completed = 0;
  const totalEndpoints = Object.values(AUDIT_ENDPOINTS).flat().length;

  if (totalEndpoints === 0) {
    container.innerHTML = '<div style="text-align:center;padding:40px;color:#8b949e;">No audit endpoints configured</div>';
    return;
  }

  // Render dimension sections
  let dimHtml = '';
  for (const [dim, endpoints] of Object.entries(AUDIT_ENDPOINTS)) {
    if (endpoints.length === 0) continue;
    const color = DIMENSION_COLORS[dim] || '#8b949e';
    dimHtml += `
      <div class="audit-dim-section" style="margin-bottom:24px;">
        <div style="display:flex;align-items:center;gap:8px;margin-bottom:12px;">
          <div style="width:12px;height:12px;border-radius:3px;background:${color};"></div>
          <h3 style="margin:0;font-size:15px;color:${color};">${dim}</h3>
          <span style="color:#8b949e;font-size:12px;">${endpoints.length} audits</span>
        </div>
        <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(280px,1fr));gap:10px;" id="audit-dim-${dim}">
    `;
    for (const ep of endpoints) {
      dimHtml += `
        <div class="audit-card" id="audit-card-${btoa(ep.path).replace(/=/g,'')}" 
             style="border:1px solid #30363d;border-radius:6px;padding:12px;cursor:pointer;background:#161b22;transition:border-color 0.2s;"
             onclick="window.loadAuditDetail('${ep.path}','${ep.name.replace(/'/g,'')}')">
          <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:6px;">
            <span style="font-size:13px;font-weight:600;">${ep.icon} ${ep.name}</span>
            <span class="audit-score" id="score-${btoa(ep.path).replace(/=/g,'')}" style="font-size:12px;font-weight:700;padding:2px 8px;border-radius:4px;background:#21262d;color:#8b949e;">--</span>
          </div>
          <div class="audit-status" id="status-${btoa(ep.path).replace(/=/g,'')}" style="font-size:11px;color:#8b949e;">Loading...</div>
        </div>
      `;
    }
    dimHtml += `</div></div>`;
  }
  document.getElementById('audit-dimensions').innerHTML = dimHtml;

  // Fetch each endpoint
  for (const [dim, endpoints] of Object.entries(AUDIT_ENDPOINTS)) {
    for (const ep of endpoints) {
      const cardId = btoa(ep.path).replace(/=/g, '');
      fetchJSON(ep.path)
        .then(data => {
          const score = data.healthScore !== undefined ? data.healthScore : null;
          const scoreEl = document.getElementById('score-' + cardId);
          const statusEl = document.getElementById('status-' + cardId);
          if (scoreEl && score !== null) {
            scoreEl.textContent = score;
            if (score >= 80) {
              scoreEl.style.background = '#1a3a2a';
              scoreEl.style.color = '#3fb950';
            } else if (score >= 60) {
              scoreEl.style.background = '#3a3a1a';
              scoreEl.style.color = '#d29922';
            } else if (score >= 40) {
              scoreEl.style.background = '#3a2a1a';
              scoreEl.style.color = '#f0883e';
            } else {
              scoreEl.style.background = '#3a1a1a';
              scoreEl.style.color = '#f85149';
            }
          }
          if (statusEl) {
            // Try to get summary stats
            let summary = data.summary || {};
            let parts = [];
            if (summary.totalCronJobs !== undefined) parts.push(`${summary.totalCronJobs} jobs`);
            if (summary.totalDashboards !== undefined) parts.push(`${summary.totalDashboards} dashboards`);
            if (summary.totalPolicies !== undefined) parts.push(`${summary.totalPolicies} policies`);
            if (summary.totalContainers !== undefined) parts.push(`${summary.totalContainers} containers`);
            if (summary.totalServices !== undefined) parts.push(`${summary.totalServices} services`);
            if (summary.totalDeployments !== undefined) parts.push(`${summary.totalDeployments} deployments`);
            if (summary.totalPVs !== undefined) parts.push(`${summary.totalPVs} PVs`);
            if (summary.totalHPAs !== undefined) parts.push(`${summary.totalHPAs} HPAs`);
            if (summary.totalSAs !== undefined) parts.push(`${summary.totalSAs} SAs`);
            if (summary.exporterPodCount !== undefined) parts.push(`${summary.exporterPodCount} exporters`);
            if (summary.totalReplicaSets !== undefined) parts.push(`${summary.totalReplicaSets} RS`);
            if (parts.length === 0 && data.recommendations) {
              statusEl.textContent = data.recommendations[0] || 'OK';
              if (statusEl.textContent.length > 80) statusEl.textContent = statusEl.textContent.substring(0, 80) + '...';
            } else {
              statusEl.textContent = parts.join(', ');
            }
          }
        })
        .catch(() => {
          const statusEl = document.getElementById('status-' + cardId);
          if (statusEl) statusEl.textContent = 'Failed to load';
        });
    }
  }
};

window.loadAuditDetail = function(path, name) {
  const container = document.getElementById('audit-dashboard-content');
  if (!container) return;

  container.innerHTML = `<div style="text-align:center;padding:40px;color:#8b949e;">Loading ${name}...</div>`;

  fetchJSON(path)
    .then(data => {
      let html = `
        <div style="margin-bottom:16px;">
          <button onclick="window.loadAuditDashboard()" class="btn-secondary" style="margin-bottom:12px;">&#8592; Back to Audit Dashboard</button>
          <h2 style="margin:0 0 4px 0;font-size:18px;">${name}</h2>
          <div style="display:flex;gap:12px;align-items:center;margin-top:8px;">
      `;

      if (data.healthScore !== undefined) {
        const score = data.healthScore;
        const color = score >= 80 ? '#3fb950' : score >= 60 ? '#d29922' : score >= 40 ? '#f0883e' : '#f85149';
        html += `<span style="font-size:28px;font-weight:700;color:${color};">${score}</span><span style="color:#8b949e;font-size:13px;">/ 100 Health Score</span>`;
      }
      html += `</div></div>`;

      // Summary
      if (data.summary) {
        html += '<div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(180px,1fr));gap:10px;margin-bottom:20px;">';
        for (const [key, val] of Object.entries(data.summary)) {
          if (typeof val === 'boolean') {
            html += `<div style="border:1px solid #30363d;border-radius:6px;padding:10px;background:#161b22;"><div style="font-size:11px;color:#8b949e;">${key}</div><div style="font-size:14px;font-weight:600;color:${val ? '#3fb950' : '#f85149'};">${val ? 'Yes' : 'No'}</div></div>`;
          } else if (typeof val === 'string' && val.length < 30) {
            html += `<div style="border:1px solid #30363d;border-radius:6px;padding:10px;background:#161b22;"><div style="font-size:11px;color:#8b949e;">${key}</div><div style="font-size:14px;font-weight:600;">${val}</div></div>`;
          } else if (typeof val === 'number') {
            html += `<div style="border:1px solid #30363d;border-radius:6px;padding:10px;background:#161b22;"><div style="font-size:11px;color:#8b949e;">${key}</div><div style="font-size:18px;font-weight:700;">${val}</div></div>`;
          }
        }
        html += '</div>';
      }

      // Issues
      if (data.issues && data.issues.length > 0) {
        html += '<h3 style="font-size:14px;margin:16px 0 8px 0;">Issues (' + data.issues.length + ')</h3><div style="max-height:300px;overflow-y:auto;">';
        for (const issue of data.issues.slice(0, 50)) {
          const color = issue.severity === 'critical' ? '#f85149' : issue.severity === 'warning' ? '#d29922' : '#58a6ff';
          html += `<div style="border-left:3px solid ${color};padding:8px 12px;margin-bottom:6px;background:#161b22;border-radius:0 4px 4px 0;"><span style="font-size:11px;color:${color};font-weight:600;text-transform:uppercase;">${issue.severity}</span> <span style="font-size:13px;">${issue.message}</span></div>`;
        }
        html += '</div>';
      }

      // Recommendations
      if (data.recommendations && data.recommendations.length > 0) {
        html += '<h3 style="font-size:14px;margin:16px 0 8px 0;">Recommendations</h3>';
        for (const rec of data.recommendations) {
          html += `<div style="padding:8px 12px;margin-bottom:4px;background:#161b22;border-radius:4px;font-size:13px;">&#128161; ${rec}</div>`;
        }
      }

      container.innerHTML = html;
    })
    .catch(err => {
      container.innerHTML = `<div style="text-align:center;padding:40px;color:#f85149;">Failed to load: ${err.message}</div>`;
    });
};