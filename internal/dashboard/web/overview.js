// --- Overview ---
async function loadOverview() {
  try {
    const data = await fetchJSON('/api/cluster/overview');
    const cards = document.getElementById('overviewCards');
    const nodes = data.nodes || {};
    const diags = data.diagnostics || {};
    const rems = data.remediations || {};
    cards.innerHTML = `
      <div class="card ${nodes.notReady > 0 ? 'warn' : 'ok'}">
        <div class="label">Nodes</div>
        <div class="value">${nodes.ready || 0}<span style="font-size:16px;color:#8b949e;">/${nodes.total || 0}</span></div>
        <div class="sub">${nodes.notReady || 0} Not Ready</div>
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
      </div>
    `;
    document.getElementById('overviewDetails').innerHTML = `
      <div class="detail-panel">
        <h3>Cluster Info</h3>
        <div class="kv"><span class="k">Version</span><code>${data.clusterVersion || 'unknown'}</code></div>
        <div class="kv"><span class="k">Node Status</span>${nodes.ready || 0} Ready, ${nodes.notReady || 0} Not Ready</div>
        <div class="kv"><span class="k">Diagnostics</span>${JSON.stringify(diags.byPhase || {})}</div>
        <div class="kv"><span class="k">Remediations</span>${JSON.stringify(rems.byPhase || {})}</div>
      </div>
    `;
    document.getElementById('version').textContent = data.clusterVersion || 'k8ops Dashboard';
  } catch(e) {
    document.getElementById('overviewCards').innerHTML = `<div class="card err"><div class="label">Error</div><div class="value" style="font-size:14px;">${escapeHtml(e.message)}</div></div>`;
  }

  // Load cost data (independent of cluster overview)
  loadCostPanel();

  document.getElementById('lastUpdate').textContent = 'Updated: ' + new Date().toLocaleTimeString();

  // Load cost overview panel
  loadCostOverview();
}

// --- Cost / FinOps ---
// Implemented below: loadCostOverview, loadCost, loadCostSummary, loadCostRecommendations

// --- Diagnostics History ---
async function loadDiagnostics() {
  const container = document.getElementById('diagnosticsTable');
  try {
    const statusFilter = document.getElementById('diagStatusFilter')?.value || '';
    const url = '/api/diagnostics/history' + (statusFilter ? '?status=' + encodeURIComponent(statusFilter) : '');
    const data = await fetchJSON(url);
    if (!data.items?.length) { container.innerHTML = '<div class="empty">No diagnostic reports found' + (statusFilter ? ' for status: ' + statusFilter : '') + '</div>'; return; }
    container.innerHTML = `<table>
      <thead><tr><th>ID</th><th>Namespace</th><th>Status</th><th>Summary</th><th>Age</th><th>Details</th></tr></thead>
      <tbody>${data.items.map(d => `<tr>
        <td><code>${d.id}</code></td>
        <td>${d.namespace}</td>
        <td>${badge(d.status)}</td>
        <td style="max-width:350px;color:#8b949e;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">${escapeHtml(d.summary || '-')}</td>
        <td>${timeAgo(d.createdAt)}</td>
        <td><a href="javascript:void(0)" onclick="viewDiagnostic('${d.namespace}','${d.id}')" style="color:#58a6ff;text-decoration:none;">View Report</a></td>
      </tr>`).join('')}</tbody>
    </table>`;
  } catch(e) {
    if (isForbidden(e)) renderForbidden(container);
    else container.innerHTML = `<div class="empty">Error: ${escapeHtml(e.message)}</div>`;
  }
}

async function viewDiagnostic(ns, name) {
  const overlay = document.getElementById('diagDetailOverlay');
  const body = document.getElementById('diagDetailBody');
  document.getElementById('diagDetailTitle').textContent = name;
  overlay.classList.add('active');
  body.innerHTML = '<div class="loading">Loading report...</div>';
  try {
    const data = await fetchJSON(`/api/diagnostics/${ns}/${name}`);
    body.innerHTML = `<div class="md" style="font-size:14px;">${renderMarkdown(data.markdown)}</div>`;
  } catch(e) {
    body.innerHTML = `<div class="error">Failed to load: ${escapeHtml(e.message)}</div>`;
  }
}

function closeDiagDetail() {
  document.getElementById('diagDetailOverlay').classList.remove('active');
}

// --- Remediations ---
async function loadRemediations() {
  const container = document.getElementById('remediationsTable');
  try {
    const data = await fetchJSON('/api/remediations');
    if (!data.items?.length) { container.innerHTML = '<div class="empty">No remediation plans found</div>'; return; }
    container.innerHTML = `<table>
      <thead><tr><th>Name</th><th>Namespace</th><th>Phase</th><th>Mode</th><th>Actions</th><th>Diagnostic Ref</th><th>Summary</th><th>Approved By</th><th>Age</th></tr></thead>
      <tbody>${data.items.map(r => `<tr>
        <td>${r.name}</td>
        <td>${r.namespace}</td>
        <td>${badge(r.phase)}</td>
        <td><code>${r.mode}</code></td>
        <td>${r.actions}${r.phase === 'Pending' ? `
          <br>
          <button class="btn-sm btn-approve" onclick="approveRemediation('${r.namespace}','${r.name}')">&#10003; Approve</button>
          <button class="btn-sm btn-reject" onclick="rejectRemediation('${r.namespace}','${r.name}')">&#10007; Reject</button>` : ''}</td>
        <td>${r.diagnosticRef || '-'}</td>
        <td style="max-width:300px;color:#8b949e;">${r.summary || '-'}</td>
        <td>${r.approvedBy || '-'}</td>
        <td>${timeAgo(r.created)}</td>
      </tr>`).join('')}</tbody>
    </table>`;
  } catch(e) {
    if (isForbidden(e)) renderForbidden(container);
    else container.innerHTML = `<div class="empty">Error: ${escapeHtml(e.message)}</div>`;
  }
}

// --- Optimizations ---
async function loadOptimizations() {
  const container = document.getElementById('optimizationsTable');
  try {
    const data = await fetchJSON('/api/optimizations');
    if (!data.items?.length) { container.innerHTML = '<div class="empty">No optimization suggestions found</div>'; return; }
    container.innerHTML = `<table>
      <thead><tr><th>Name</th><th>Namespace</th><th>Phase</th><th>Scope</th><th>Suggestions</th><th>Est. Savings</th><th>Summary</th><th>Age</th></tr></thead>
      <tbody>${data.items.map(o => `<tr>
        <td>${o.name}</td>
        <td>${o.namespace}</td>
        <td>${badge(o.phase)}</td>
        <td>${o.scope}</td>
        <td>${o.suggestions}</td>
        <td>${o.estimatedSavings || '-'}</td>
        <td style="max-width:300px;color:#8b949e;">${o.summary || '-'}</td>
        <td>${timeAgo(o.created)}</td>
      </tr>`).join('')}</tbody>
    </table>`;
  } catch(e) {
    if (isForbidden(e)) renderForbidden(container);
    else container.innerHTML = `<div class="empty">Error: ${escapeHtml(e.message)}</div>`;
  }
}

// --- Cost / FinOps ---
async function loadCostOverview() {
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

async function loadCost() {
  try {
    await Promise.all([loadCostSummary(), loadCostRecommendations()]);
  } catch(e) { /* ignore */ }
}

async function loadCostSummary() {
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

async function loadCostRecommendations() {
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
async function approveRemediation(namespace, name) {
  if (!confirm(`Approve remediation plan "${name}"?`)) return;
  try {
    const res = await fetchJSON(`/api/remediation/${namespace}/${name}/approve`, { method: 'POST' });
    alert(`Approved: ${res.status}`);
    loadRemediations();
  } catch(e) {
    alert('Error: ' + (e.message || 'Unknown error'));
  }
}

async function rejectRemediation(namespace, name) {
  if (!confirm(`Reject remediation plan "${name}"?`)) return;
  try {
    const res = await fetchJSON(`/api/remediation/${namespace}/${name}/reject`, { method: 'POST' });
    alert(`Rejected: ${res.status}`);
    loadRemediations();
  } catch(e) {
    alert('Error: ' + (e.message || 'Unknown error'));
  }
}

