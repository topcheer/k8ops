// --- Nodes ---
async function loadNodes(forceRefresh) {
  const container = document.getElementById('nodesTable');
  try {
    const data = await fetchJSON('/api/nodes' + (forceRefresh ? '?refresh=true' : ''));
    if (!data.items?.length) { container.innerHTML = '<div class="empty">No nodes found</div>'; return; }
    container.innerHTML = `<table>
      <thead><tr><th>Name</th><th>Status</th><th>Role</th><th>Version</th><th>CPU</th><th>Memory</th><th>OS/Arch</th><th>Conditions</th><th></th></tr></thead>
      <tbody>${data.items.map(n => `<tr>
        <td style="cursor:pointer;color:#58a6ff;" onclick="viewNodePods('${n.name}')">${n.name}</td>
        <td>${badge(n.status)}</td>
        <td>${n.role}</td>
        <td><code>${n.version}</code></td>
        <td>${n.cpu}</td>
        <td>${n.memory}</td>
        <td>${n.os}/${n.arch}</td>
        <td style="font-size:12px;color:#8b949e;">${Object.entries(n.conditions).map(([k,v])=>k+':'+v).join(', ')}</td>
        <td><button onclick="viewNodePods('${n.name}')" class="btn-secondary" style="font-size:12px;padding:4px 10px;">Pods →</button></td>
      </tr>`).join('')}</tbody>
    </table>`;
  } catch(e) { container.innerHTML = `<div class="empty">Error: ${escapeHtml(e.message)}</div>`; }
}

// --- Events ---
async function loadEvents() {
  const container = document.getElementById('eventsTable');
  try {
    const warning = document.getElementById('warningOnly')?.checked ? 'warning=true' : '';
    const data = await fetchJSON('/api/events?' + warning);
    if (!data.items?.length) { container.innerHTML = '<div class="empty">No events found</div>'; return; }
    container.innerHTML = `<table>
      <thead><tr><th>Type</th><th>Reason</th><th>Object</th><th>Namespace</th><th>Message</th><th>Count</th><th>Last Seen</th></tr></thead>
      <tbody>${data.items.map(e => `<tr>
        <td>${badge(e.type)}</td>
        <td><strong>${e.reason}</strong></td>
        <td>${e.object}</td>
        <td>${e.namespace}</td>
        <td style="max-width:400px;color:#8b949e;">${escapeHtml(e.message)}</td>
        <td>${e.count}</td>
        <td>${timeAgo(e.lastTime)}</td>
      </tr>`).join('')}</tbody>
    </table>`;
  } catch(err) { container.innerHTML = `<div class="empty">Error: ${escapeHtml(err.message)}</div>`; }
}

