// --- Auth: check current user ---
async function checkCurrentUser() {
  try {
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

function logout() {
  fetch('/api/auth/logout', {method: 'POST'}).finally(() => {
    window.location.href = '/login.html';
  });
}

// Initial load moved to end of index.html (after all scripts loaded)
checkCurrentUser();

// --- Pods ---
async function loadPods(forceRefresh) {
  const container = document.getElementById('podsTable');
  try {
    const data = await fetchJSON('/api/pods' + (forceRefresh ? '?refresh=true' : ''));
    if (!data.items?.length) { container.innerHTML = '<div class="empty">No pods found</div>'; return; }
    container.innerHTML = `<table>
      <thead><tr><th>Name</th><th>Namespace</th><th>Phase</th><th>Node</th><th>Restarts</th><th>Age</th><th>Actions</th></tr></thead>
      <tbody>${data.items.map(p => `<tr>
        <td style="color:#58a6ff;font-family:monospace;">${p.name}</td>
        <td>${p.namespace}</td>
        <td>${badge(p.phase)}</td>
        <td style="cursor:pointer;color:#58a6ff;" onclick="viewNodePods('${p.node}')">${p.node || '-'}</td>
        <td>${p.restarts > 5 ? '<span style="color:#f85149;">'+p.restarts+'</span>' : p.restarts}</td>
        <td>${p.age}</td>
        <td>
          <button onclick="openLogViewer('${p.namespace}','${p.name}')" class="btn-secondary" style="font-size:12px;padding:4px 10px;">Logs</button>
          <button onclick="openTerminal('${p.namespace}','${p.name}')" class="btn-secondary" style="font-size:12px;padding:4px 10px;">Terminal</button>
          <button onclick="viewYAML('pods','${p.namespace}','${p.name}')" class="btn-secondary" style="font-size:12px;padding:4px 10px;">YAML</button>
        </td>
      </tr>`).join('')}</tbody>
    </table>`;
  } catch(e) { container.innerHTML = `<div class="empty">Error: ${escapeHtml(e.message)}</div>`; }
}

// --- Audit ---
async function loadAudit() {
  const container = document.getElementById('auditTable');
  const statsDiv = document.getElementById('auditStats');
  try {
    const [auditData, statsData] = await Promise.all([
      fetchJSON('/api/audit'),
      fetchJSON('/api/audit/stats')
    ]);
    // Stats cards
    if (statsDiv) {
    const s = statsData;
    statsDiv.innerHTML = `<div class="cards">
      <div class="card info"><div class="label">Total Events</div><div class="value">${s.total||0}</div></div>
      <div class="card ok"><div class="label">Successful</div><div class="value">${s.successCount||0}</div></div>
      <div class="card err"><div class="label">Failed</div><div class="value">${s.failureCount||0}</div></div>
      <div class="card warn"><div class="label">Critical</div><div class="value">${(s.bySeverity||{}).critical||0}</div></div>
    </div>`;
    } // end if(statsDiv)

    if (!auditData.items?.length) { container.innerHTML = '<div class="empty">No audit events found</div>'; return; }
    container.innerHTML = `<table>
      <thead><tr><th>Time</th><th>Type</th><th>Severity</th><th>Action</th><th>Target</th><th>Actor</th><th>Success</th><th>Duration</th></tr></thead>
      <tbody>${auditData.items.map(e => `<tr>
        <td style="font-size:12px;">${e.timestamp ? new Date(e.timestamp).toLocaleTimeString() : '-'}</td>
        <td><code>${e.type||'-'}</code></td>
        <td>${badge(e.severity||'info')}</td>
        <td>${e.action||'-'}</td>
        <td style="max-width:200px;color:#8b949e;">${e.target||'-'}</td>
        <td>${e.actor||'-'}</td>
        <td>${e.success ? '<span style="color:#3fb950;">yes</span>' : '<span style="color:#f85149;">no</span>'}</td>
        <td>${e.duration||'-'}</td>
      </tr>`).join('')}</tbody>
    </table>`;
  } catch(e) { container.innerHTML = `<div class="empty">Error: ${escapeHtml(e.message)}</div>`; }
}
// Auto-refresh overview every 30s
setInterval(() => {
  if (location.hash === '#overview' || location.hash === '') loadOverview();
}, 30000);

