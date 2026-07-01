// --- Auth: check current user ---
import { escapeHtml, fetchJSON, badge, timeAgo } from './modules/utils.js';

export async function checkCurrentUser() {
  try {
    // Check if auth is enabled first, skip silently if not
    const statusRes = await fetch('/api/auth/status');
    if (!statusRes.ok) return;
    const authStatus = await statusRes.json();
    if (!authStatus.enabled) return;

    const res = await fetch('/api/auth/me');
    if (res.ok) {
      const data = await res.json();
      if (data.user) {
        document.getElementById('userMenu').classList.add('show');
        const u = data.user;
        document.getElementById('userInfo').textContent = u.display_name || u.username;
        document.getElementById('userRole').textContent = u.role;
        // Show Access Control tab for admins
        if (u.role === 'admin') {
          const btn = document.getElementById('rbacTabBtn');
          if (btn) btn.style.display = 'flex';
        }
      }
    }
  } catch(e) {}
}

export function logout() {
  fetch('/api/auth/logout', {method: 'POST'}).finally(() => {
    window.location.href = '/login.html';
  });
}

// Initial load handled by main.js DOMContentLoaded

// --- Pods ---
export async function loadPods(forceRefresh) {
  const container = document.getElementById('podsTable');
  try {
    const params = new URLSearchParams();
    if (forceRefresh) params.set('refresh', 'true');
    const ns = getCurrentNamespace();
    if (ns) params.set('namespace', ns);
    const qs = params.toString();
    const data = await fetchJSON('/api/pods' + (qs ? '?' + qs : ''));
    if (!data.items?.length) { container.innerHTML = '<div class="empty">No pods found' + (ns ? ' in ' + ns : '') + '</div>'; return; }
    container.innerHTML = `<table>
      <thead><tr><th>Name</th><th>Namespace</th><th>Phase</th><th>Node</th><th>Restarts</th><th>Age</th><th>Actions</th></tr></thead>
      <tbody>${data.items.map(p => `<tr>
        <td style="color:#58a6ff;font-family:monospace;">${escapeHtml(p.name)}</td>
        <td>${escapeHtml(p.namespace)}</td>
        <td>${badge(p.phase)}</td>
        <td style="cursor:pointer;color:#58a6ff;" onclick="viewNodePods('${escapeHtml(p.node)}')">${escapeHtml(p.node) || '-'}</td>
        <td>${p.restarts > 5 ? '<span style="color:#f85149;">'+p.restarts+'</span>' : p.restarts}</td>
        <td>${escapeHtml(p.age)}</td>
        <td>
          <button onclick="openLogViewer('${escapeHtml(p.namespace)}','${escapeHtml(p.name)}')" class="btn-secondary" style="font-size:12px;padding:4px 10px;">Logs</button>
          <button onclick="openTerminal('${escapeHtml(p.namespace)}','${escapeHtml(p.name)}')" class="btn-secondary" style="font-size:12px;padding:4px 10px;">Terminal</button>
          <button onclick="viewYAML('pods','${escapeHtml(p.namespace)}','${escapeHtml(p.name)}')" class="btn-secondary" style="font-size:12px;padding:4px 10px;">YAML</button>
        </td>
      </tr>`).join('')}</tbody>
    </table>`;
  } catch(e) { container.innerHTML = `<div class="empty">Error: ${escapeHtml(e.message)}</div>`; }
}

// --- Audit ---
export async function loadAudit() {
  const container = document.getElementById('auditTable');
  const statsDiv = document.getElementById('auditStats');
  try {
    const severity = document.getElementById('auditSeverity') ? document.getElementById('auditSeverity').value : '';
    const params = new URLSearchParams({ limit: '100' });
    if (severity) params.set('severity', severity);
    const [auditData, statsData] = await Promise.all([
      fetchJSON('/api/audit?' + params.toString()),
      fetchJSON('/api/audit/stats')
    ]);
    // Stats cards
    if (statsDiv) {
      const s = statsData;
      const bySev = s.bySeverity || {};
      statsDiv.innerHTML = '<div class="cards">' +
        '<div class="card info"><div class="label">Total Events</div><div class="value">' + (s.total||0) + '</div></div>' +
        '<div class="card ok"><div class="label">Successful</div><div class="value">' + (s.successCount||0) + '</div></div>' +
        '<div class="card err"><div class="label">Failed</div><div class="value">' + (s.failureCount||0) + '</div></div>' +
        '<div class="card ' + ((bySev.critical||0) > 0 ? 'err' : '') + '"><div class="label">Critical</div><div class="value" style="color:#f85149;">' + (bySev.critical||0) + '</div></div>' +
        '<div class="card ' + ((bySev.warning||0) > 0 ? 'warn' : '') + '"><div class="label">Warnings</div><div class="value" style="color:#d29922;">' + (bySev.warning||0) + '</div></div>' +
        '</div>';
    }

    if (!auditData.items || !auditData.items.length) { container.innerHTML = '<div class="empty">No audit events found</div>'; return; }
    container.innerHTML = '<table>' +
      '<thead><tr><th>Time</th><th>Severity</th><th>Action</th><th>Target</th><th>Actor</th><th>Success</th><th>Duration</th></tr></thead>' +
      '<tbody>' + auditData.items.map(function(e) {
        var sevClass = 'audit-sev-info';
        if (e.severity === 'critical') sevClass = 'audit-sev-critical';
        else if (e.severity === 'error') sevClass = 'audit-sev-error';
        else if (e.severity === 'warning') sevClass = 'audit-sev-warning';
        return '<tr>' +
          '<td style="font-size:11px;color:#8b949e;white-space:nowrap;font-family:monospace;">' + (e.timestamp ? new Date(e.timestamp).toLocaleString() : '-') + '</td>' +
          '<td><span class="audit-sev-badge ' + sevClass + '">' + escapeHtml(e.severity || 'info') + '</span></td>' +
          '<td><strong>' + escapeHtml(e.action || '-') + '</strong></td>' +
          '<td style="max-width:220px;color:#58a6ff;font-family:monospace;font-size:11px;">' + escapeHtml(e.target || '-') + '</td>' +
          '<td>' + escapeHtml(e.actor || '-') + '</td>' +
          '<td>' + (e.success ? '<span style="color:#3fb950;">yes</span>' : '<span style="color:#f85149;">no</span>') + '</td>' +
          '<td style="color:#8b949e;">' + escapeHtml(e.duration || '-') + '</td>' +
        '</tr>';
      }).join('') + '</tbody>' +
    '</table>';
  } catch(e) { container.innerHTML = '<div class="empty">Error: ' + escapeHtml(e.message) + '</div>'; }
}
// Auto-refresh overview every 30s
setInterval(() => {
  if (location.hash === '#overview' || location.hash === '') window.loadOverview();
}, 30000);

