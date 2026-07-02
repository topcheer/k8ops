// --- Overview ---
import { escapeHtml, fetchJSON, isForbidden, renderForbidden, badge, timeAgo, showToast } from './modules/utils.js';

const _sparklineHistory = { pods: [], warnings: [], events: [] };
const MAX_SPARK_POINTS = 30;

export async function loadOverview() {
  // Show loading skeletons immediately
  const cardsEl = document.getElementById('overviewCards');
  const detailsEl = document.getElementById('overviewDetails');
  if (cardsEl && !cardsEl.innerHTML.trim()) {
    cardsEl.innerHTML = '<div class="cards">' +
      Array(5).fill('<div class="skeleton skeleton-card"></div>').join('') +
      '</div>';
  }
  if (detailsEl && !detailsEl.innerHTML.trim()) {
    detailsEl.innerHTML = '<div class="detail-panel">' +
      '<h3>Node Resource Utilization</h3>' +
      Array(3).fill('<div class="skeleton skeleton-row"></div>').join('') +
      '</div>';
  }

  try {
    const [data, nodesData] = await Promise.all([
      fetchJSON('/api/cluster/overview'),
      fetchJSON('/api/nodes').catch(() => ({ items: [] }))
    ]);
    const cards = document.getElementById('overviewCards');
    const nodes = data.nodes || {};
    const diags = data.diagnostics || {};
    const rems = data.remediations || {};

    // Track sparkline history
    _sparklineHistory.pods.push(nodes.ready || 0);
    _sparklineHistory.warnings.push(data.recentWarnings || 0);
    if (_sparklineHistory.pods.length > MAX_SPARK_POINTS) _sparklineHistory.pods.shift();
    if (_sparklineHistory.warnings.length > MAX_SPARK_POINTS) _sparklineHistory.warnings.shift();

    cards.innerHTML = `
      <div class="card ${nodes.notReady > 0 ? 'warn' : 'ok'}">
        <div class="label">Nodes</div>
        <div class="value">${nodes.ready || 0}<span style="font-size:16px;color:var(--text-muted);">/${nodes.total || 0}</span></div>
        <div class="sub">${nodes.notReady || 0} Not Ready</div>
        ${sparklineSvg(_sparklineHistory.pods, '#3fb950')}
      </div>
      <div class="card ${(data.pods?.failed || 0) > 0 ? 'warn' : 'ok'}">
        <div class="label">Pods</div>
        <div class="value">${data.pods?.running || 0}<span style="font-size:16px;color:var(--text-muted);">/${data.pods?.total || 0}</span></div>
        <div class="sub">${data.pods?.failed || 0} Failed, ${data.pods?.pending || 0} Pending</div>
      </div>
      <div class="card info">
        <div class="label">Namespaces</div>
        <div class="value">${data.namespaces || 0}</div>
      </div>
      <div class="card ${diags.byPhase?.Failed > 0 ? 'warn' : 'info'}">
        <div class="label">Diagnostics</div>
        <div class="value">${diags.total || 0}</div>
        <div class="sub">${diags.byPhase?.Completed || 0} completed</div>
      </div>
      <div class="card ${rems.byPhase?.Failed > 0 ? 'err' : rems.byPhase?.Completed > 0 ? 'ok' : 'info'}">
        <div class="label">Remediations</div>
        <div class="value">${rems.total || 0}</div>
        <div class="sub">${rems.byPhase?.Completed || 0} completed</div>
      </div>
      <div class="card ${data.recentWarnings > 10 ? 'warn' : ''}">
        <div class="label">Recent Warnings</div>
        <div class="value">${data.recentWarnings || 0}</div>
        ${sparklineSvg(_sparklineHistory.warnings, data.recentWarnings > 10 ? 'var(--accent-red)' : 'var(--accent-yellow)')}
      </div>
      ${healthScoreGauge(nodes, diags, data.recentWarnings || 0, data.pods)}
    `;

    // Node resource utilization bars
    const nodesHtml = (nodesData.items || []).map(n => {
      const cpuPct = n.cpuRequestedPct || 0;
      const memPct = n.memRequestedPct || 0;
      const podPct = n.podCapacity > 0 ? (n.podCount / n.podCapacity * 100) : 0;
      const cpuColor = cpuPct > 80 ? '#f85149' : cpuPct > 60 ? '#d29922' : '#3fb950';
      const memColor = memPct > 80 ? '#f85149' : memPct > 60 ? '#d29922' : '#3fb950';
      const podColor = podPct > 80 ? '#f85149' : podPct > 60 ? '#d29922' : '#3fb950';
      return `<div class="node-resource-row">
        <div class="node-resource-name">
          <span class="status-dot ${n.status === 'Ready' ? 'dot-ok' : 'dot-err'}"></span>
          ${escapeHtml(n.name)}
          <span class="node-role-tag">${escapeHtml(n.role)}</span>
        </div>
        <div class="resource-bars">
          <div class="resource-bar-group">
            <span class="resource-label">CPU</span>
            <div class="resource-bar-track"><div class="resource-bar-fill" style="width:${cpuPct}%;background:${cpuColor};"></div></div>
            <span class="resource-value">${cpuPct.toFixed(0)}%</span>
          </div>
          <div class="resource-bar-group">
            <span class="resource-label">MEM</span>
            <div class="resource-bar-track"><div class="resource-bar-fill" style="width:${memPct}%;background:${memColor};"></div></div>
            <span class="resource-value">${memPct.toFixed(0)}%</span>
          </div>
          <div class="resource-bar-group">
            <span class="resource-label">POD</span>
            <div class="resource-bar-track"><div class="resource-bar-fill" style="width:${podPct}%;background:${podColor};"></div></div>
            <span class="resource-value">${n.podCount}/${n.podCapacity}</span>
          </div>
        </div>
      </div>`;
    }).join('');

    document.getElementById('overviewDetails').innerHTML = `
      <div class="detail-panel">
        <h3>Node Resource Utilization</h3>
        ${nodesHtml || '<div class="empty">No nodes data</div>'}
      </div>
      <div class="detail-panel" style="margin-top:16px;">
        <h3>Cluster Info</h3>
        <div class="kv"><span class="k">Version</span><code>${escapeHtml(data.clusterVersion || 'unknown')}</code></div>
        <div class="kv"><span class="k">Node Status</span>${nodes.ready || 0} Ready, ${nodes.notReady || 0} Not Ready</div>
        <div class="kv"><span class="k">Diagnostics</span>${renderPhaseBadges(diags.byPhase)}</div>
        <div class="kv"><span class="k">Remediations</span>${renderPhaseBadges(rems.byPhase)}</div>
      </div>
    `;
    document.getElementById('version').textContent = data.clusterVersion || 'k8ops Dashboard';
  } catch(e) {
    document.getElementById('overviewCards').innerHTML = `<div class="card err"><div class="label">Error</div><div class="value" style="font-size:14px;">${escapeHtml(e.message)}</div></div>`;
  }

  // Load recent events feed
  loadRecentEvents();
  // Load cost overview panel
  loadCostOverview();
}

async function loadRecentEvents() {
  const container = document.getElementById('recentEventsPanel');
  if (!container) return;
  try {
    const data = await fetchJSON('/api/events?limit=10');
    if (!data.items || !data.items.length) {
      container.innerHTML = '<div class="empty" style="padding:16px;">No recent events</div>';
      return;
    }
    container.innerHTML = '<div class="events-feed">' + data.items.slice(0, 10).map(function(e) {
      var sev = (e.type || '').toLowerCase();
      var dotColor = sev === 'warning' ? 'var(--accent-red)' : 'var(--accent-green)';
      var bgClass = sev === 'warning' ? 'event-row-warn' : 'event-row-norm';
      return '<div class="event-row ' + bgClass + '">' +
        '<span class="event-dot" style="background:' + dotColor + ';"></span>' +
        '<span class="event-type">' + escapeHtml(e.type || '') + '</span>' +
        '<span class="event-reason">' + escapeHtml(e.reason || '') + '</span>' +
        '<span class="event-msg" title="' + escapeHtml(e.message || '') + '">' + escapeHtml((e.message || '').substring(0, 80)) + (e.message && e.message.length > 80 ? '...' : '') + '</span>' +
        '<span class="event-time">' + escapeHtml(e.lastTimestamp || e.firstTimestamp || '') + '</span>' +
      '</div>';
    }).join('') + '</div>';
  } catch(e) {
    container.innerHTML = '<div class="empty">Failed to load events</div>';
  }
}

function renderPhaseBadges(byPhase) {
  if (!byPhase || typeof byPhase !== 'object') return '<span style="color:var(--text-muted);">-</span>';
  var phases = Object.keys(byPhase);
  if (!phases.length) return '<span style="color:var(--text-muted);">-</span>';
  var colors = {
    'Completed': 'var(--accent-green)',
    'Failed': 'var(--accent-red)',
    'Pending': 'var(--accent-yellow)',
    'Running': 'var(--accent-blue)',
  };
  return phases.map(function(p) {
    var c = colors[p] || 'var(--text-muted)';
    return '<span style="display:inline-block;padding:2px 8px;border-radius:10px;font-size:11px;background:' + c + '20;color:' + c + ';margin-right:4px;">' + p + ': ' + byPhase[p] + '</span>';
  }).join('');
}

// ============================
// Cluster Health Score Gauge
// ============================
export function healthScoreGauge(nodes, diags, warnings, pods) {
  // Calculate score: 0-100
  let score = 100;

  // Node readiness: -20 per not-ready node (capped)
  if (nodes.total > 0) {
    const nodeRatio = nodes.ready / nodes.total;
    score -= (1 - nodeRatio) * 30;
  }

  // Warnings: -1 per warning (capped at 20)
  score -= Math.min(warnings, 20);

  // Failed diagnostics: -5 per failed (capped at 25)
  const failedDiags = (diags.byPhase && diags.byPhase.Failed) || 0;
  score -= Math.min(failedDiags * 5, 25);

  // Failed remediations: -3 per failed (capped at 15)
  // (handled implicitly through diagnostics)

  score = Math.max(0, Math.min(100, Math.round(score)));

  // Determine status
  let status, color, label;
  if (score >= 85) { status = 'healthy'; color = 'var(--accent-green)'; label = 'Healthy'; }
  else if (score >= 60) { status = 'warning'; color = 'var(--accent-yellow)'; label = 'Warning'; }
  else { status = 'critical'; color = 'var(--accent-red)'; label = 'Critical'; }

  // SVG donut gauge: radius=28, circumference=2*PI*28≈176
  const r = 28;
  const circumference = 2 * Math.PI * r;
  const offset = circumference * (1 - score / 100);

  return `
    <div class="card health-score-card ${status}" style="display:flex;align-items:center;gap:12px;">
      <div class="health-gauge">
        <svg width="72" height="72" viewBox="0 0 72 72">
          <circle cx="36" cy="36" r="${r}" fill="none" stroke="var(--border-default)" stroke-width="6"/>
          <circle cx="36" cy="36" r="${r}" fill="none" stroke="${color}" stroke-width="6"
            stroke-dasharray="${circumference}" stroke-dashoffset="${offset}"
            stroke-linecap="round" transform="rotate(-90 36 36)"
            style="transition:stroke-dashoffset 0.6s ease;"/>
          <text x="36" y="40" text-anchor="middle" font-size="20" font-weight="700" fill="${color}">${score}</text>
        </svg>
      </div>
      <div>
        <div class="label">Cluster Health</div>
        <div class="value" style="font-size:18px;color:${color};">${label}</div>
        <div class="sub">${nodes.ready || 0}/${nodes.total || 0} nodes &middot; ${warnings} warnings</div>
      </div>
    </div>`;
}

export function sparklineSvg(data, color) {
  if (!data || data.length < 2) return '';
  const w = 100, h = 24;
  const min = Math.min(...data), max = Math.max(...data);
  const range = max - min || 1;
  const step = w / (data.length - 1);
  const points = data.map((v, i) => {
    const x = i * step;
    const y = h - ((v - min) / range) * (h - 4) - 2;
    return x.toFixed(1) + ',' + y.toFixed(1);
  }).join(' ');
  return `<svg class="sparkline" viewBox="0 0 ${w} ${h}" preserveAspectRatio="none">
    <polyline points="${points}" fill="none" stroke="${color}" stroke-width="1.5" />
  </svg>`;
}

// --- Cost / FinOps ---
// Implemented below: loadCostOverview, loadCost, loadCostSummary, loadCostRecommendations

// --- Diagnostics History ---
export async function loadDiagnostics() {
  const container = document.getElementById('diagnosticsTable');
  try {
    const statusFilter = document.getElementById('diagStatusFilter')?.value || '';
    const url = '/api/diagnostics/history' + (statusFilter ? '?status=' + encodeURIComponent(statusFilter) : '');
    const data = await fetchJSON(url);
    if (!data.items?.length) { container.innerHTML = '<div class="empty">No diagnostic reports found' + (statusFilter ? ' for status: ' + statusFilter : '') + '</div>'; return; }
    container.innerHTML = `<table>
      <thead><tr><th>ID</th><th>Namespace</th><th>Status</th><th>Summary</th><th>Age</th><th>Details</th></tr></thead>
      <tbody>${data.items.map(d => `<tr>
        <td><code>${escapeHtml(d.name || d.id)}</code></td>
        <td>${escapeHtml(d.namespace)}</td>
        <td>${badge(d.phase || d.status)}</td>
        <td style="max-width:350px;color:var(--text-muted);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">${escapeHtml(d.summary || '-')}</td>
        <td>${timeAgo(d.createdAt || d.created)}</td>
        <td><a href="javascript:void(0)" onclick="viewDiagnostic('${escapeHtml(d.namespace)}','${escapeHtml(d.name || d.id)}')" style="color:var(--accent-blue);text-decoration:none;">View Report</a></td>
      </tr>`).join('')}</tbody>
    </table>`;
  } catch(e) {
    if (isForbidden(e)) renderForbidden(container);
    else container.innerHTML = `<div class="empty">Error: ${escapeHtml(e.message)}</div>`;
  }
}

export async function viewDiagnostic(ns, name) {
  const overlay = document.getElementById('diagDetailOverlay');
  const body = document.getElementById('diagDetailBody');
  document.getElementById('diagDetailTitle').textContent = name;
  overlay.classList.add('active');
  body.innerHTML = '<div class="loading">Loading report...</div>';
  try {
    const data = await fetchJSON(`/api/diagnostics/${ns}/${name}`);
    body.innerHTML = `<div class="md" style="font-size:14px;">${window.renderMarkdown ? window.renderMarkdown(data.markdown) : escapeHtml(data.markdown || '')}</div>`;
  } catch(e) {
    body.innerHTML = `<div class="error">Failed to load: ${escapeHtml(e.message)}</div>`;
  }
}

export function closeDiagDetail() {
  document.getElementById('diagDetailOverlay').classList.remove('active');
}

// --- Remediations ---
export async function loadRemediations() {
  const container = document.getElementById('remediationsTable');
  try {
    const data = await fetchJSON('/api/remediations');
    if (!data.items?.length) { container.innerHTML = '<div class="empty">No remediation plans found</div>'; return; }
    container.innerHTML = `<table>
      <thead><tr><th>Name</th><th>Namespace</th><th>Phase</th><th>Mode</th><th>Actions</th><th>Diagnostic Ref</th><th>Summary</th><th>Approved By</th><th>Age</th></tr></thead>
      <tbody>${data.items.map(r => `<tr>
        <td>${escapeHtml(r.name)}</td>
        <td>${escapeHtml(r.namespace)}</td>
        <td>${badge(r.phase)}</td>
        <td><code>${escapeHtml(r.mode)}</code></td>
        <td>${escapeHtml(r.actions)}${r.phase === 'Pending' ? `
          <br>
          <button class="btn-sm btn-approve" onclick="approveRemediation('${escapeHtml(r.namespace)}','${escapeHtml(r.name)}')">&#10003; Approve</button>
          <button class="btn-sm btn-reject" onclick="rejectRemediation('${escapeHtml(r.namespace)}','${escapeHtml(r.name)}')">&#10007; Reject</button>` : ''}</td>
        <td>${escapeHtml(r.diagnosticRef) || '-'}</td>
        <td style="max-width:300px;color:#8b949e;">${escapeHtml(r.summary) || '-'}</td>
        <td>${escapeHtml(r.approvedBy) || '-'}</td>
        <td>${timeAgo(r.created)}</td>
      </tr>`).join('')}</tbody>
    </table>`;
  } catch(e) {
    if (isForbidden(e)) renderForbidden(container);
    else container.innerHTML = `<div class="empty">Error: ${escapeHtml(e.message)}</div>`;
  }
}

// --- Optimizations ---
export async function loadOptimizations() {
  const container = document.getElementById('optimizationsTable');
  try {
    const data = await fetchJSON('/api/optimizations');
    if (!data.items?.length) { container.innerHTML = '<div class="empty">No optimization suggestions found</div>'; return; }
    container.innerHTML = `<table>
      <thead><tr><th>Name</th><th>Namespace</th><th>Phase</th><th>Scope</th><th>Suggestions</th><th>Est. Savings</th><th>Summary</th><th>Age</th></tr></thead>
      <tbody>${data.items.map(o => `<tr>
        <td>${escapeHtml(o.name)}</td>
        <td>${escapeHtml(o.namespace)}</td>
        <td>${badge(o.phase)}</td>
        <td>${escapeHtml(o.scope)}</td>
        <td>${escapeHtml(o.suggestions)}</td>
        <td>${escapeHtml(o.estimatedSavings) || '-'}</td>
        <td style="max-width:300px;color:#8b949e;">${escapeHtml(o.summary) || '-'}</td>
        <td>${timeAgo(o.created)}</td>
      </tr>`).join('')}</tbody>
    </table>`;
  } catch(e) {
    if (isForbidden(e)) renderForbidden(container);
    else container.innerHTML = `<div class="empty">Error: ${escapeHtml(e.message)}</div>`;
  }
}

// --- Cost / FinOps ---
export async function loadCostOverview() {
  const panel = document.getElementById('costPanel');
  if (!panel) return;
  try {
    const summary = await fetchJSON('/api/cost/summary');
    if (!summary || !summary.namespaces?.length) {
      panel.innerHTML = '';
      return;
    }
    const total = summary.totalMonthlyCostUSD || 0;
    const top3 = summary.namespaces.slice(0, 3);
    panel.innerHTML = `
      <div class="detail-panel">
        <h3>Cost Overview</h3>
        <div class="kv"><span class="k">Total Monthly Cost</span><code style="color:#3fb950;font-size:16px;">$${total.toFixed(2)}</code></div>
        <div class="kv"><span class="k">Total Pods</span>${summary.totalPods || 0}</div>
        <div class="kv"><span class="k">CPU (requested)</span>${(summary.totalCPURequestedCores || 0).toFixed(2)} cores</div>
        <div class="kv"><span class="k">RAM (requested)</span>${(summary.totalRAMRequestedGB || 0).toFixed(1)} GB</div>
        <div class="kv"><span class="k">Pricing</span>$${summary.pricing?.cpuPricePerCore || '?'}/core + $${summary.pricing?.ramPricePerGB || '?'}/GB per month</div>
        <div class="kv"><span class="k">Top Namespaces</span>${top3.map(n => `${n.namespace} ($${n.monthlyCostUSD.toFixed(0)}, ${n.percentage}%)`).join(', ')}</div>
      </div>`;
  } catch(e) {
    panel.innerHTML = '';
  }
}

export async function loadCost() {
  try {
    await Promise.all([loadCostSummary(), loadCostRecommendations()]);
  } catch(e) { /* ignore */ }
}

export async function loadCostSummary() {
  const cardsEl = document.getElementById('costSummaryCards');
  const nsEl = document.getElementById('costNamespaces');
  if (!cardsEl || !nsEl) return;

  try {
    const summary = await fetchJSON('/api/cost/summary');

    cardsEl.innerHTML = `
      <div class="cards" style="display:grid;grid-template-columns:repeat(auto-fill,minmax(220px,1fr));gap:16px;margin-bottom:24px;">
        <div class="card ok">
          <div class="label">Total Monthly Cost</div>
          <div class="value">$${(summary.totalMonthlyCostUSD || 0).toFixed(2)}</div>
          <div class="sub">${(summary.totalAnnualCostUSD || (summary.totalMonthlyCostUSD * 12)).toFixed(0)}/yr projected</div>
        </div>
        <div class="card info">
          <div class="label">Pods Costed</div>
          <div class="value">${summary.totalPods || 0}</div>
        </div>
        <div class="card info">
          <div class="label">CPU Requested</div>
          <div class="value">${(summary.totalCPURequestedCores || 0).toFixed(2)}</div>
          <div class="sub">cores</div>
        </div>
        <div class="card info">
          <div class="label">RAM Requested</div>
          <div class="value">${(summary.totalRAMRequestedGB || 0).toFixed(1)}</div>
          <div class="sub">GB</div>
        </div>
      </div>`;

    if (!summary.namespaces?.length) {
      nsEl.innerHTML = '<div class="empty">No cost data available</div>';
      return;
    }

    nsEl.innerHTML = `
      <div class="detail-panel">
        <h3>Cost by Namespace</h3>
        <table>
          <thead><tr><th>Namespace</th><th>Pods</th><th>CPU (cores)</th><th>RAM (GB)</th><th>Monthly Cost</th><th>Share</th></tr></thead>
          <tbody>${summary.namespaces.map(ns => {
            const barColor = ns.percentage > 50 ? '#f85149' : ns.percentage > 25 ? '#d29922' : '#3fb950';
            return `<tr>
              <td>${ns.namespace}</td>
              <td>${ns.pods}</td>
              <td>${ns.cpuRequestedCores.toFixed(3)}</td>
              <td>${ns.ramRequestedGB.toFixed(3)}</td>
              <td style="color:#3fb950;font-weight:600;">$${ns.monthlyCostUSD.toFixed(2)}</td>
              <td>
                <div style="display:flex;align-items:center;gap:8px;">
                  <div style="flex:1;height:6px;background:#21262d;border-radius:3px;overflow:hidden;">
                    <div style="width:${ns.percentage}%;height:100%;background:${barColor};border-radius:3px;"></div>
                  </div>
                  <span style="color:#8b949e;font-size:12px;min-width:40px;">${ns.percentage}%</span>
                </div>
              </td>
            </tr>`;
          }).join('')}</tbody>
        </table>
      </div>`;
  } catch(e) {
    if (isForbidden(e)) { renderForbidden(cardsEl); renderForbidden(nsEl); }
    else {
      cardsEl.innerHTML = `<div class="card err"><div class="label">Error</div><div class="value" style="font-size:14px;">${escapeHtml(e.message)}</div></div>`;
      nsEl.innerHTML = '';
    }
  }
}

export async function loadCostRecommendations() {
  const recEl = document.getElementById('costRecommendations');
  if (!recEl) return;

  try {
    const data = await fetchJSON('/api/cost/recommendations');

    if (!data.recommendations?.length) {
      recEl.innerHTML = `
        <div class="detail-panel">
          <h3>Right-Sizing Recommendations</h3>
          <div class="empty">No over-provisioned resources detected. Cluster is well-sized.</div>
        </div>`;
      return;
    }

    recEl.innerHTML = `
      <div class="detail-panel">
        <h3>Right-Sizing Recommendations &mdash; $${(data.totalPotentialSavingsUSD || 0).toFixed(2)}/mo potential savings</h3>
        <table>
          <thead><tr><th>Workload</th><th>Container</th><th>Current (req)</th><th>Limit</th><th>Recommended</th><th>Monthly Savings</th><th>% Saved</th><th>Reason</th></tr></thead>
          <tbody>${data.recommendations.map(r => `<tr>
            <td>${r.workload}</td>
            <td>${r.container}</td>
            <td>${r.current.cpuCores.toFixed(3)} core / ${r.current.ramGB.toFixed(3)} GB</td>
            <td style="color:#d29922;">${r.limit.cpuCores.toFixed(3)} core / ${r.limit.ramGB.toFixed(3)} GB</td>
            <td style="color:#58a6ff;">${r.recommended.cpuCores.toFixed(3)} core / ${r.recommended.ramGB.toFixed(3)} GB</td>
            <td style="color:#3fb950;font-weight:600;">$${r.monthlySavingsUSD.toFixed(2)}</td>
            <td>${r.savingsPercent.toFixed(1)}%</td>
            <td style="max-width:300px;color:#8b949e;font-size:12px;">${r.reason}</td>
          </tr>`).join('')}</tbody>
        </table>
      </div>`;
  } catch(e) {
    if (isForbidden(e)) renderForbidden(recEl);
    else recEl.innerHTML = `<div class="detail-panel"><h3>Right-Sizing Recommendations</h3><div class="empty">Error: ${escapeHtml(e.message)}</div></div>`;
  }
}

// --- Remediation Approve/Reject ---
export async function approveRemediation(namespace, name) {
  if (!confirm(`Approve remediation plan "${name}"?`)) return;
  try {
    const res = await fetchJSON(`/api/remediation/${namespace}/${name}/approve`, { method: 'POST' });
    showToast('Approved: ' + (res.status || 'OK'), 'success');
    loadRemediations();
  } catch(e) {
    showToast('Error: ' + (e.message || 'Unknown error'), 'error');
  }
}

export async function rejectRemediation(namespace, name) {
  if (!confirm(`Reject remediation plan "${name}"?`)) return;
  try {
    const res = await fetchJSON(`/api/remediation/${namespace}/${name}/reject`, { method: 'POST' });
    showToast('Rejected: ' + (res.status || 'OK'), 'success');
    loadRemediations();
  } catch(e) {
    showToast('Error: ' + (e.message || 'Unknown error'), 'error');
  }
}

