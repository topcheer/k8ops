// namespaces.js — Namespace Resource Ranking page for k8ops dashboard

import { escapeHtml, fetchJSON } from './modules/utils.js';

let nsRankingData = null;

export async function loadNamespaceRanking() {
  const container = document.getElementById('nsRankingContent');
  if (container) container.innerHTML = '<div class="loading">Loading namespace resource data...</div>';

  try {
    const data = await fetchJSON('/api/namespaces/ranking');
    nsRankingData = data;
    renderNSRanking(data);
  } catch (err) {
    if (container) {
      container.innerHTML = '<div class="empty-state">Failed to load: ' + escapeHtml(err.message) + '</div>';
    }
  }
}

function formatCores(m) {
  if (m >= 1000) return (m / 1000).toFixed(2) + ' cores';
  return m + 'm';
}

function pctBar(pct) {
  const color = pct > 80 ? '#f85149' : pct > 60 ? '#d29922' : '#3fb950';
  return `<div style="display:flex;align-items:center;gap:8px;">
    <div style="width:80px;height:6px;background:rgba(139,148,158,0.2);border-radius:3px;overflow:hidden;">
      <div style="width:${Math.min(pct, 100)}%;height:100%;background:${color};border-radius:3px;"></div>
    </div>
    <span style="font-size:12px;color:${color};">${pct.toFixed(1)}%</span>
  </div>`;
}

function renderNSRanking(data) {
  const container = document.getElementById('nsRankingContent');
  if (!container) return;

  const items = data.items || [];
  const summary = data.summary || {};

  // Summary cards
  const summaryHtml = `
    <div style="display:flex;gap:12px;flex-wrap:wrap;margin-bottom:16px;">
      <div class="stat-card" style="min-width:110px;">
        <div class="stat-value">${summary.totalNamespaces || 0}</div>
        <div class="stat-label">Namespaces</div>
      </div>
      <div class="stat-card" style="min-width:110px;">
        <div class="stat-value">${summary.totalPods || 0}</div>
        <div class="stat-label">Total Pods</div>
      </div>
      <div class="stat-card" style="min-width:130px;">
        <div class="stat-value" style="font-size:18px;">${formatCores(summary.totalCPURequestM || 0)}</div>
        <div class="stat-label">CPU Requested</div>
      </div>
      <div class="stat-card" style="min-width:130px;">
        <div class="stat-value" style="font-size:18px;">${(summary.totalMemRequestMB || 0)} MB</div>
        <div class="stat-label">Memory Requested</div>
      </div>
      <div class="stat-card" style="min-width:130px;">
        <div class="stat-value" style="font-size:18px;">${formatCores(summary.clusterAllocatableCPU || 0)}</div>
        <div class="stat-label">Cluster Allocatable</div>
      </div>
    </div>`;

  // Table rows
  const rows = items.map((ns, i) => {
    return `<tr style="cursor:pointer;" onclick="showNamespaceDetail('${escapeHtml(ns.name)}')">
      <td style="font-weight:600;color:#58a6ff;">${escapeHtml(ns.name)}</td>
      <td>${ns.podCount}</td>
      <td style="font-family:monospace;">${formatCores(ns.cpuRequestMcores)}</td>
      <td>${pctBar(ns.cpuRequestPct)}</td>
      <td style="font-family:monospace;">${formatCores(ns.cpuLimitMcores)}</td>
      <td style="font-family:monospace;">${ns.memRequestMB} MB</td>
      <td>${pctBar(ns.memRequestPct)}</td>
      <td style="font-family:monospace;">${ns.memLimitMB > 0 ? ns.memLimitMB + ' MB' : '-'}</td>
      <td>${ns.pvcCount > 0 ? ns.pvcCount + ' (' + ns.pvcStorageGB.toFixed(1) + ' GB)' : '-'}</td>
    </tr>`;
  }).join('');

  container.innerHTML = `
    ${summaryHtml}
    <input type="text" id="nsSearchInput" class="search-input" placeholder="Filter namespaces..."
      oninput="filterNSTable()" style="margin-bottom:12px;width:100%;">
    <table class="data-table" id="nsRankingTable">
      <thead>
        <tr>
          <th style="width:160px;">Namespace</th>
          <th style="width:60px;">Pods</th>
          <th style="width:100px;">CPU Req</th>
          <th style="width:140px;">CPU %</th>
          <th style="width:100px;">CPU Limit</th>
          <th style="width:100px;">Mem Req</th>
          <th style="width:140px;">Mem %</th>
          <th style="width:100px;">Mem Limit</th>
          <th style="width:120px;">PVCs</th>
        </tr>
      </thead>
      <tbody>${rows}</tbody>
    </table>`;
}

export function filterNSTable() {
  const search = (document.getElementById('nsSearchInput')?.value || '').toLowerCase();
  const rows = document.querySelectorAll('#nsRankingTable tbody tr');
  rows.forEach(row => {
    const name = row.cells[0]?.textContent?.toLowerCase() || '';
    row.style.display = name.includes(search) ? '' : 'none';
  });
}

export async function showNamespaceDetail(ns) {
  const overlay = document.getElementById('diagDetailOverlay');
  const title = document.getElementById('diagDetailTitle');
  const body = document.getElementById('diagDetailBody');
  if (!overlay || !body) return;

  title.textContent = 'Namespace: ' + ns;
  overlay.style.display = 'flex';
  body.innerHTML = '<div class="loading">Loading namespace details...</div>';

  try {
    const res = await fetch('/api/namespaces/' + encodeURIComponent(ns) + '/detail');
    if (!res.ok) {
      body.innerHTML = '<div class="empty-state">Failed to load details (HTTP ' + res.status + ')</div>';
      return;
    }
    const data = await res.json();

    let html = '<div style="max-width:800px;margin:0 auto;">';

    // Quotas
    html += '<h3 style="color:#58a6ff;margin-bottom:8px;">Resource Quotas</h3>';
    if (data.quotas && data.quotas.length > 0) {
      html += '<table class="data-table" style="margin-bottom:20px;"><thead><tr><th>Resource</th><th>Used</th><th>Hard Limit</th></tr></thead><tbody>';
      for (const q of data.quotas) {
        const allKeys = new Set([...Object.keys(q.hard || {}), ...Object.keys(q.used || {})]);
        for (const key of allKeys) {
          html += `<tr>
            <td style="font-family:monospace;">${escapeHtml(key)}</td>
            <td style="font-family:monospace;color:#d29922;">${escapeHtml((q.used||{})[key] || '0')}</td>
            <td style="font-family:monospace;color:#3fb950;">${escapeHtml((q.hard||{})[key] || '-')}</td>
          </tr>`;
        }
      }
      html += '</tbody></table>';
    } else {
      html += '<p style="color:#8b949e;margin-bottom:20px;">No ResourceQuotas set for this namespace.</p>';
    }

    // LimitRanges
    html += '<h3 style="color:#58a6ff;margin-bottom:8px;">Limit Ranges</h3>';
    if (data.limitRanges && data.limitRanges.length > 0) {
      for (const lr of data.limitRanges) {
        html += `<div style="margin-bottom:12px;padding:12px;border:1px solid var(--border-color);border-radius:8px;">`;
        html += `<div style="font-weight:600;margin-bottom:8px;">${escapeHtml(lr.name)}</div>`;
        html += '<table class="data-table"><thead><tr><th>Type</th><th>Default</th><th>Default Request</th><th>Max</th><th>Min</th></tr></thead><tbody>';
        for (const lim of (lr.limits || [])) {
          html += `<tr>
            <td>${escapeHtml(lim.type || '-')}</td>
            <td style="font-family:monospace;">${escapeHtml(lim.default_cpu || '')} ${escapeHtml(lim.default_memory || '')}</td>
            <td style="font-family:monospace;">${escapeHtml(lim.defaultRequest_cpu || '')} ${escapeHtml(lim.defaultRequest_memory || '')}</td>
            <td style="font-family:monospace;">${escapeHtml(lim.max_cpu || '')} ${escapeHtml(lim.max_memory || '')}</td>
            <td style="font-family:monospace;">${escapeHtml(lim.min_cpu || '')} ${escapeHtml(lim.min_memory || '')}</td>
          </tr>`;
        }
        html += '</tbody></table></div>';
      }
    } else {
      html += '<p style="color:#8b949e;margin-bottom:20px;">No LimitRanges set for this namespace.</p>';
    }

    // Recent warnings
    html += '<h3 style="color:#f85149;margin-bottom:8px;">Recent Warnings</h3>';
    if (data.recentWarnings && data.recentWarnings.length > 0) {
      html += '<table class="data-table"><thead><tr><th>Reason</th><th>Object</th><th>Count</th><th>Last Seen</th></tr></thead><tbody>';
      for (const e of data.recentWarnings) {
        html += `<tr>
          <td style="color:#f85149;">${escapeHtml(e.reason || '')}</td>
          <td style="font-family:monospace;">${escapeHtml(e.object || '')}</td>
          <td>${e.count || 0}</td>
          <td style="color:#8b949e;">${escapeHtml(e.lastTime || '')}</td>
        </tr>`;
      }
      html += '</tbody></table>';
    } else {
      html += '<p style="color:#3fb950;">No recent warnings.</p>';
    }

    html += '</div>';
    body.innerHTML = html;
  } catch (err) {
    body.innerHTML = '<div class="empty-state">Error: ' + escapeHtml(err.message) + '</div>';
  }
}
