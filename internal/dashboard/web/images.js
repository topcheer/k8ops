// images.js — Container Image Inventory page for k8ops dashboard

import { escapeHtml, fetchJSON } from './modules/utils.js';

export async function loadImages() {
  const container = document.getElementById('imagesContent');
  if (container) container.innerHTML = '<div class="loading">Loading image inventory...</div>';

  try {
    const data = await fetchJSON('/api/images');
    renderImages(data);
  } catch (err) {
    if (container) {
      container.innerHTML = '<div class="empty-state">Failed to load: ' + escapeHtml(err.message) + '</div>';
    }
  }
}

function renderImages(data) {
  const container = document.getElementById('imagesContent');
  if (!container) return;

  const items = data.items || [];
  const summary = data.summary || {};

  // Summary cards
  const summaryHtml = `
    <div class="stat-card" style="min-width:110px;">
      <div class="stat-value">${summary.totalImages || 0}</div>
      <div class="stat-label">Unique Images</div>
    </div>
    <div class="stat-card" style="min-width:110px;border-left:3px solid ${(summary.usingLatestTag || 0) > 0 ? '#d29922' : '#3fb950'};">
      <div class="stat-value" style="color:${(summary.usingLatestTag || 0) > 0 ? '#d29922' : '#3fb950'};">${summary.usingLatestTag || 0}</div>
      <div class="stat-label">Using :latest</div>
    </div>
    <div class="stat-card" style="min-width:110px;border-left:3px solid ${(summary.withoutLimits || 0) > 0 ? '#f85149' : '#3fb950'};">
      <div class="stat-value" style="color:${(summary.withoutLimits || 0) > 0 ? '#f85149' : '#3fb950'};">${summary.withoutLimits || 0}</div>
      <div class="stat-label">No Limits</div>
    </div>
    <div class="stat-card" style="min-width:110px;border-left:3px solid ${(summary.withoutRequests || 0) > 0 ? '#d29922' : '#3fb950'};">
      <div class="stat-value" style="color:${(summary.withoutRequests || 0) > 0 ? '#d29922' : '#3fb950'};">${summary.withoutRequests || 0}</div>
      <div class="stat-label">No Requests</div>
    </div>
    <div class="stat-card" style="min-width:100px;">
      <div class="stat-value">${summary.uniqueRegistries || 0}</div>
      <div class="stat-label">Registries</div>
    </div>`;

  // Table rows
  const rows = items.map(img => {
    const tagColor = img.tag === 'latest' ? '#d29922' : '#8b949e';
    const limitsIcon = img.hasLimits ? '<span style="color:#3fb950;">&#10003;</span>' : '<span style="color:#f85149;">&#10007;</span>';
    const reqIcon = img.hasRequests ? '<span style="color:#3fb950;">&#10003;</span>' : '<span style="color:#d29922;">&#10007;</span>';
    const regColor = img.registry === 'docker.io' ? '#8b949e' : '#58a6ff';

    return `<tr>
      <td style="font-family:monospace;font-size:12px;">
        <span style="color:${regColor};">${escapeHtml(img.registry)}</span>/<span style="color:#c9d1d9;">${escapeHtml(img.repo)}</span>
        <span style="color:${tagColor};">:${escapeHtml(img.tag)}</span>
      </td>
      <td style="text-align:center;font-weight:600;">${img.usedByCount}</td>
      <td style="font-size:12px;color:#8b949e;">${escapeHtml(img.namespaces.join(', '))}</td>
      <td style="text-align:center;">${limitsIcon}</td>
      <td style="text-align:center;">${reqIcon}</td>
      <td style="font-size:11px;color:#8b949e;">${escapeHtml(img.pullPolicy || '-')}</td>
    </tr>`;
  }).join('');

  container.innerHTML = `
    <div style="display:flex;gap:12px;flex-wrap:wrap;margin-bottom:16px;">${summaryHtml}</div>
    <div style="display:flex;gap:12px;align-items:center;margin-bottom:16px;">
      <input type="text" id="imgSearch" class="search-input" placeholder="Filter images..." oninput="filterImages()" style="flex:1;">
      <button onclick="loadImages()" class="btn-secondary">Refresh</button>
    </div>
    <table class="data-table" id="imgTable">
      <thead><tr>
        <th>Image</th>
        <th style="width:70px;">Used By</th>
        <th style="width:200px;">Namespaces</th>
        <th style="width:60px;">Limits</th>
        <th style="width:60px;">Requests</th>
        <th style="width:120px;">Pull Policy</th>
      </tr></thead>
      <tbody>${rows}</tbody>
    </table>`;
}

export function filterImages() {
  const search = (document.getElementById('imgSearch')?.value || '').toLowerCase();
  const rows = document.querySelectorAll('#imgTable tbody tr');
  rows.forEach(row => {
    const text = row.textContent.toLowerCase();
    row.style.display = text.includes(search) ? '' : 'none';
  });
}
