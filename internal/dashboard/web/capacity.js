// capacity.js — Storage & Capacity Planning page for k8ops dashboard

import { escapeHtml, fetchJSON } from './modules/utils.js';

export async function loadCapacity() {
  const container = document.getElementById('capacityContent');
  if (container) container.innerHTML = '<div class="loading">Loading capacity data...</div>';

  try {
    const [storage, planning] = await Promise.all([
      fetchJSON('/api/storage/capacity'),
      fetchJSON('/api/capacity/planning')
    ]);
    renderCapacity(storage, planning);
  } catch (err) {
    if (container) {
      container.innerHTML = '<div class="empty-state">Failed to load: ' + escapeHtml(err.message) + '</div>';
    }
  }
}

function pctColor(pct) {
  if (pct > 80) return '#f85149';
  if (pct > 60) return '#d29922';
  return '#3fb950';
}

function utilBar(pct, label) {
  const color = pctColor(pct);
  return `<div style="display:flex;align-items:center;gap:8px;">
    <div style="width:100px;height:8px;background:rgba(139,148,158,0.2);border-radius:4px;overflow:hidden;">
      <div style="width:${Math.min(pct, 100)}%;height:100%;background:${color};border-radius:4px;transition:width 0.3s;"></div>
    </div>
    <span style="font-size:12px;color:${color};min-width:45px;">${pct.toFixed(0)}%</span>
  </div>`;
}

function renderCapacity(storage, planning) {
  const container = document.getElementById('capacityContent');
  if (!container) return;

  const sSummary = storage.summary || {};
  const pSummary = planning.summary || {};
  const recs = planning.recommendations || [];

  // Storage summary cards
  const storageCards = `
    <div class="stat-card" style="min-width:110px;">
      <div class="stat-value">${sSummary.totalPVCs || 0}</div>
      <div class="stat-label">PVCs</div>
    </div>
    <div class="stat-card" style="min-width:110px;">
      <div class="stat-value" style="color:#3fb950;">${sSummary.bound || 0}</div>
      <div class="stat-label">Bound</div>
    </div>
    <div class="stat-card" style="min-width:110px;">
      <div class="stat-value" style="color:#d29922;">${sSummary.pending || 0}</div>
      <div class="stat-label">Pending</div>
    </div>
    <div class="stat-card" style="min-width:130px;">
      <div class="stat-value" style="font-size:18px;">${(sSummary.totalCapacityGB || 0).toFixed(1)} GB</div>
      <div class="stat-label">Total Capacity</div>
    </div>
    <div class="stat-card" style="min-width:130px;">
      <div class="stat-value" style="font-size:18px;">${(sSummary.totalRequestedGB || 0).toFixed(1)} GB</div>
      <div class="stat-label">Requested</div>
    </div>`;

  // Cluster capacity cards
  const clusterCards = `
    <div class="stat-card" style="min-width:110px;">
      <div class="stat-value">${pSummary.nodeCount || 0}</div>
      <div class="stat-label">Nodes</div>
    </div>
    <div class="stat-card" style="min-width:130px;">
      <div class="stat-value" style="font-size:18px;">${pSummary.totalCPUAllocatable || '-'}</div>
      <div class="stat-label">CPU Allocatable</div>
    </div>
    <div class="stat-card" style="min-width:130px;">
      <div class="stat-value" style="font-size:18px;">${pSummary.totalCPURequested || '-'}</div>
      <div class="stat-label">CPU Requested</div>
    </div>
    <div class="stat-card" style="min-width:120px;border-left:3px solid ${pctColor(pSummary.clusterCPUUtilPct || 0)};">
      <div class="stat-value" style="font-size:18px;color:${pctColor(pSummary.clusterCPUUtilPct || 0)};">${(pSummary.clusterCPUUtilPct || 0).toFixed(0)}%</div>
      <div class="stat-label">CPU Utilization</div>
    </div>
    <div class="stat-card" style="min-width:120px;border-left:3px solid ${pctColor(pSummary.clusterMemUtilPct || 0)};">
      <div class="stat-value" style="font-size:18px;color:${pctColor(pSummary.clusterMemUtilPct || 0)};">${(pSummary.clusterMemUtilPct || 0).toFixed(0)}%</div>
      <div class="stat-label">Mem Utilization</div>
    </div>`;

  // PVC table rows
  const pvcRows = (storage.items || []).map(pvc => {
    const statusColor = pvc.status === 'Bound' ? '#3fb950' : pvc.status === 'Pending' ? '#d29922' : '#f85149';
    return `<tr>
      <td style="font-weight:600;color:#58a6ff;">${escapeHtml(pvc.name)}</td>
      <td>${escapeHtml(pvc.namespace)}</td>
      <td><span style="color:${statusColor};">${escapeHtml(pvc.status)}</span></td>
      <td style="font-family:monospace;">${pvc.capacityGB.toFixed(1)} GB</td>
      <td style="font-family:monospace;">${pvc.requestedGB > 0 ? pvc.requestedGB.toFixed(1) + ' GB' : '-'}</td>
      <td>${escapeHtml(pvc.storageClass || '-')}</td>
      <td>${escapeHtml(pvc.accessMode || '-')}</td>
    </tr>`;
  }).join('');

  // Node capacity table rows
  const nodeRows = (planning.nodes || []).map(n => {
    return `<tr>
      <td style="font-weight:600;color:#58a6ff;">${escapeHtml(n.name)}</td>
      <td>${n.status === 'Ready' ? '<span style="color:#3fb950;">Ready</span>' : '<span style="color:#f85149;">NotReady</span>'}</td>
      <td style="font-family:monospace;">${formatMCores(n.cpuAllocatableM)}</td>
      <td style="font-family:monospace;">${formatMCores(n.cpuRequestedM)}</td>
      <td>${utilBar(n.cpuRequestedPct, 'CPU')}</td>
      <td>${utilBar(n.cpuLimitPct, 'CPU Limit')}</td>
      <td style="font-family:monospace;">${n.memAllocatableGB.toFixed(1)} GB</td>
      <td style="font-family:monospace;">${n.memRequestedGB.toFixed(1)} GB</td>
      <td>${utilBar(n.memRequestedPct, 'Mem')}</td>
      <td>${n.podCount}/${n.podCapacity}</td>
      <td>${utilBar(n.podUsedPct, 'Pods')}</td>
    </tr>`;
  }).join('');

  // Recommendations
  const recHTML = recs.length > 0 ? recs.map(r => {
    const isWarning = r.includes('consider') || r.includes('at risk') || r.includes('approaching') || r.includes('expansion');
    const icon = isWarning ? '<span style="color:#d29922;">&#9888;</span>' : '<span style="color:#3fb950;">&#10003;</span>';
    return `<div style="padding:8px 12px;border-bottom:1px solid rgba(48,54,61,0.3);font-size:13px;">${icon} ${escapeHtml(r)}</div>`;
  }).join('') : '';

  container.innerHTML = `
    <h3 style="margin:0 0 12px 0;">Capacity Planning</h3>
    <div style="display:flex;gap:12px;flex-wrap:wrap;margin-bottom:16px;">${clusterCards}</div>

    <h3 style="margin:20px 0 12px 0;">Node Capacity Analysis</h3>
    <input type="text" id="capNodeSearch" class="search-input" placeholder="Filter nodes..." oninput="filterCapNodes()" style="margin-bottom:8px;width:100%;">
    <table class="data-table" id="capNodeTable" style="margin-bottom:20px;">
      <thead><tr>
        <th>Node</th><th>Status</th><th>CPU Alloc</th><th>CPU Req</th><th>CPU Util</th><th>CPU Limit</th>
        <th>Mem Alloc</th><th>Mem Req</th><th>Mem Util</th><th>Pods</th><th>Pod Density</th>
      </tr></thead>
      <tbody>${nodeRows}</tbody>
    </table>

    <h3 style="margin:20px 0 12px 0;">Storage (PVCs)</h3>
    <div style="display:flex;gap:12px;flex-wrap:wrap;margin-bottom:12px;">${storageCards}</div>
    <input type="text" id="capPVCSearch" class="search-input" placeholder="Filter PVCs..." oninput="filterCapPVCs()" style="margin-bottom:8px;width:100%;">
    <table class="data-table" id="capPVCTable">
      <thead><tr>
        <th>Name</th><th>Namespace</th><th>Status</th><th>Capacity</th><th>Requested</th><th>Storage Class</th><th>Access</th>
      </tr></thead>
      <tbody>${pvcRows}</tbody>
    </table>

    ${recHTML ? `<h3 style="margin:20px 0 12px 0;">Recommendations</h3>
    <div style="border:1px solid var(--border-color);border-radius:8px;overflow:hidden;">${recHTML}</div>` : ''}
  `;
}

export function filterCapNodes() {
  const search = (document.getElementById('capNodeSearch')?.value || '').toLowerCase();
  const rows = document.querySelectorAll('#capNodeTable tbody tr');
  rows.forEach(row => {
    const name = row.cells[0]?.textContent?.toLowerCase() || '';
    row.style.display = name.includes(search) ? '' : 'none';
  });
}

export function filterCapPVCs() {
  const search = (document.getElementById('capPVCSearch')?.value || '').toLowerCase();
  const rows = document.querySelectorAll('#capPVCTable tbody tr');
  rows.forEach(row => {
    const text = (row.cells[0]?.textContent + ' ' + row.cells[1]?.textContent).toLowerCase();
    row.style.display = text.includes(search) ? '' : 'none';
  });
}

function formatMCores(m) {
  if (m >= 1000) return (m / 1000).toFixed(1) + ' cores';
  return m + 'm';
}
