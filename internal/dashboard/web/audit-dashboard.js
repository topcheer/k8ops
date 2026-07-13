// audit-dashboard.js — Unified Audit Dashboard showing all audit endpoints by dimension
import { apiFetch } from './core.js';

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
  ],
  'Documentation': [],
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
      apiFetch(ep.path)
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

  apiFetch(path)
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