// security.js — Security Audit page for k8ops dashboard

import { escapeHtml, fetchJSON } from './modules/utils.js';

let securityData = null;

export async function loadSecurityAudit() {
  const container = document.getElementById('secFindings');
  if (container) container.innerHTML = '<div class="loading">Scanning cluster for security issues...</div>';

  try {
    const data = await fetchJSON('/api/security/audit');
    securityData = data;
    renderSecuritySummary(data.summary || {});
    renderSecurityFindings(data.findings || []);
    updateCategoryFilter(data.findings || []);
  } catch (err) {
    if (container) container.innerHTML = '<div class="empty-state">Failed to load security audit: ' + escapeHtml(err.message) + '</div>';
  }
}

function renderSecuritySummary(summary) {
  const el = document.getElementById('secSummary');
  if (!el) return;

  const severities = [
    { key: 'critical', label: 'Critical', color: '#f85149' },
    { key: 'high',     label: 'High',     color: '#db6d28' },
    { key: 'medium',   label: 'Medium',   color: '#d29922' },
    { key: 'low',      label: 'Low',      color: '#58a6ff' },
    { key: 'info',     label: 'Info',     color: '#8b949e' },
  ];

  const cards = severities.map(s => {
    const count = summary[s.key] || 0;
    const active = count > 0;
    return `
      <div class="stat-card ${active ? '' : 'stat-muted'}" style="border-left: 3px solid ${s.color};">
        <div class="stat-value" style="color: ${active ? s.color : '#484f58'};">${count}</div>
        <div class="stat-label">${s.label}</div>
      </div>`;
  }).join('');

  const total = summary.total || 0;
  const scoreClass = total === 0 ? 'sec-score-good' :
                     (summary.critical || 0) + (summary.high || 0) > 5 ? 'sec-score-bad' :
                     'sec-score-warn';

  el.innerHTML = `
    <div style="display:flex;gap:12px;flex-wrap:wrap;margin-bottom:16px;">
      <div class="stat-card" style="border-left:3px solid #8b949e;min-width:120px;">
        <div class="stat-value">${total}</div>
        <div class="stat-label">Total Findings</div>
      </div>
      ${cards}
    </div>
    <div class="${scoreClass}" style="padding:10px 16px;border-radius:8px;margin-bottom:16px;font-size:14px;">
      ${total === 0
        ? 'No security issues found. Your cluster looks well-configured.'
        : (summary.critical || 0) > 0
          ? `Found ${summary.critical} critical issue(s) requiring immediate attention.`
          : (summary.high || 0) > 0
            ? `Found ${summary.high} high-severity issue(s). Review recommended.`
            : 'Minor issues found. Review and remediate as time allows.'}
    </div>`;
}

function updateCategoryFilter(findings) {
  const sel = document.getElementById('secCategoryFilter');
  if (!sel) return;
  const cats = [...new Set(findings.map(f => f.category))].sort();
  const current = sel.value;
  sel.innerHTML = '<option value="">All Categories</option>' +
    cats.map(c => `<option value="${escapeHtml(c)}">${escapeHtml(c)}</option>`).join('');
  sel.value = current;
}

function renderSecurityFindings(findings) {
  const container = document.getElementById('secFindings');
  if (!container) return;

  if (findings.length === 0) {
    container.innerHTML = `
      <div style="text-align:center;padding:40px;color:#8b949e;">
        <div style="font-size:48px;margin-bottom:12px;">&#9989;</div>
        <div style="font-size:18px;">No security findings</div>
        <div style="font-size:13px;margin-top:8px;">Your cluster passed the security audit.</div>
      </div>`;
    return;
  }

  const sevColors = {
    critical: '#f85149', high: '#db6d28', medium: '#d29922',
    low: '#58a6ff', info: '#8b949e',
  };

  const rows = findings.map(f => {
    const color = sevColors[f.severity] || '#8b949e';
    return `
      <tr class="sec-row" data-severity="${f.severity}" data-category="${escapeHtml(f.category)}" data-resource="${escapeHtml(f.resource)}">
        <td style="white-space:nowrap;">
          <span style="background:${color}22;color:${color};padding:2px 8px;border-radius:4px;font-size:11px;font-weight:600;text-transform:uppercase;">${f.severity}</span>
        </td>
        <td style="white-space:nowrap;color:#8b949e;font-size:13px;">${escapeHtml(f.category)}</td>
        <td style="font-size:13px;font-family:monospace;">${escapeHtml(f.resource)}</td>
        <td style="font-size:13px;">${escapeHtml(f.detail)}</td>
        <td style="font-size:12px;color:#8b949e;">${escapeHtml(f.fix || '')}</td>
      </tr>`;
  }).join('');

  container.innerHTML = `
    <table class="data-table" id="secTable">
      <thead>
        <tr>
          <th style="width:80px;">Severity</th>
          <th style="width:120px;">Category</th>
          <th style="width:250px;">Resource</th>
          <th>Issue</th>
          <th style="width:300px;">Recommendation</th>
        </tr>
      </thead>
      <tbody>${rows}</tbody>
    </table>`;
}

export function filterSecurityFindings() {
  const sev = document.getElementById('secSeverityFilter')?.value || '';
  const cat = document.getElementById('secCategoryFilter')?.value || '';
  const search = (document.getElementById('secSearch')?.value || '').toLowerCase();

  const rows = document.querySelectorAll('#secTable tbody tr');
  let visible = 0;
  rows.forEach(row => {
    const matchSev = !sev || row.dataset.severity === sev;
    const matchCat = !cat || row.dataset.category === cat;
    const matchSearch = !search ||
      (row.dataset.resource || '').toLowerCase().includes(search) ||
      row.textContent.toLowerCase().includes(search);
    if (matchSev && matchCat && matchSearch) {
      row.style.display = '';
      visible++;
    } else {
      row.style.display = 'none';
    }
  });

  // Show/hide "no results" message
  const tbody = document.querySelector('#secTable tbody');
  if (tbody && visible === 0) {
    let msg = document.getElementById('secNoResults');
    if (!msg) {
      msg = document.createElement('tr');
      msg.id = 'secNoResults';
      msg.innerHTML = '<td colspan="5" style="text-align:center;padding:24px;color:#8b949e;">No findings match the current filter.</td>';
      tbody.appendChild(msg);
    }
    msg.style.display = '';
  } else {
    const msg = document.getElementById('secNoResults');
    if (msg) msg.style.display = 'none';
  }
}
