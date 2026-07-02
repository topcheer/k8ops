// compliance.js — CIS Benchmark Compliance page for k8ops dashboard

import { escapeHtml, fetchJSON } from './modules/utils.js';

export async function loadCompliance() {
  const container = document.getElementById('complianceContent');
  if (container) container.innerHTML = '<div class="loading">Running CIS compliance scan...</div>';

  try {
    const data = await fetchJSON('/api/security/compliance');
    renderCompliance(data);
  } catch (err) {
    if (container) {
      container.innerHTML = '<div class="empty-state">Failed to load: ' + escapeHtml(err.message) + '</div>';
    }
  }
}

function statusIcon(status) {
  switch (status) {
    case 'pass': return '<span style="color:#3fb950;">&#10003;</span>';
    case 'fail': return '<span style="color:#f85149;">&#10007;</span>';
    case 'warn': return '<span style="color:#d29922;">&#9888;</span>';
    default: return '<span style="color:#8b949e;">&#9644;</span>';
  }
}

function renderCompliance(data) {
  const container = document.getElementById('complianceContent');
  if (!container) return;

  const checks = data.checks || [];
  const summary = data.summary || {};
  const score = data.score || 0;

  // Score gauge
  const scoreColor = score >= 80 ? '#3fb950' : score >= 50 ? '#d29922' : '#f85149';

  // Summary cards
  const summaryCards = `
    <div class="stat-card" style="min-width:120px;border-left:3px solid ${scoreColor};">
      <div class="stat-value" style="color:${scoreColor};font-size:28px;">${score}%</div>
      <div class="stat-label">Compliance Score</div>
    </div>
    <div class="stat-card" style="min-width:100px;">
      <div class="stat-value">${summary.total || 0}</div>
      <div class="stat-label">Total Checks</div>
    </div>
    <div class="stat-card" style="min-width:100px;border-left:3px solid #3fb950;">
      <div class="stat-value" style="color:#3fb950;">${summary.pass || 0}</div>
      <div class="stat-label">Passed</div>
    </div>
    <div class="stat-card" style="min-width:100px;border-left:3px solid #d29922;">
      <div class="stat-value" style="color:#d29922;">${summary.warn || 0}</div>
      <div class="stat-label">Warnings</div>
    </div>
    <div class="stat-card" style="min-width:100px;border-left:3px solid #f85149;">
      <div class="stat-value" style="color:#f85149;">${summary.fail || 0}</div>
      <div class="stat-label">Failed</div>
    </div>`;

  // Check rows grouped by category
  let currentCategory = '';
  const rows = checks.map(c => {
    let html = '';
    if (c.category !== currentCategory) {
      currentCategory = c.category;
      html += `<tr class="compliance-category-row"><td colspan="4">${escapeHtml(c.category)}</td></tr>`;
    }
    const statusBadge = c.status === 'pass' ? 'comp-badge-pass' :
                        c.status === 'fail' ? 'comp-badge-fail' : 'comp-badge-warn';
    html += `<tr>
      <td style="font-family:monospace;color:#8b949e;font-size:12px;">CIS ${escapeHtml(c.id)}</td>
      <td>${escapeHtml(c.title)}</td>
      <td><span class="${statusBadge}">${c.status.toUpperCase()}</span></td>
      <td style="font-size:12px;color:#8b949e;">${escapeHtml(c.remediation)}</td>
    </tr>`;
    return html;
  }).join('');

  container.innerHTML = `
    <div style="display:flex;gap:12px;flex-wrap:wrap;margin-bottom:16px;align-items:center;">
      ${summaryCards}
    </div>
    <div style="margin-bottom:16px;">
      <span style="color:#8b949e;font-size:13px;">Benchmark: ${escapeHtml(data.benchmark || 'CIS')}</span>
      <span style="color:#8b949e;font-size:13px;margin-left:16px;">Scanned: ${escapeHtml(data.scannedAt || '')}</span>
    </div>
    <div style="display:flex;gap:8px;margin-bottom:16px;">
      <button onclick="downloadComplianceReport()" class="btn-primary">Download Report (.txt)</button>
      <button onclick="loadCompliance()" class="btn-secondary">Re-scan</button>
    </div>
    <table class="data-table" id="complianceTable">
      <thead><tr>
        <th style="width:80px;">Control ID</th>
        <th>Check</th>
        <th style="width:80px;">Status</th>
        <th style="width:300px;">Remediation</th>
      </tr></thead>
      <tbody>${rows}</tbody>
    </table>`;
}

export function downloadComplianceReport() {
  window.open('/api/security/compliance/report', '_blank');
}
