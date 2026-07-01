// --- Settings ---
import { escapeHtml, fetchJSON, isForbidden, renderForbidden, truncateText } from './modules/utils.js';

export async function loadSettings() {
  try {
    const status = await fetchJSON('/api/provider/status');
    document.getElementById('providerStatus').innerHTML =
      '<div class="kv"><span class="k">Status</span>' +
        (status.active ? '<span style="color:#3fb950;">Active</span>' : '<span style="color:#f85149;">Inactive</span>') + '</div>' +
      '<div class="kv"><span class="k">Provider</span><code>' + (status.type || '-') + '</code></div>' +
      '<div class="kv"><span class="k">Model</span><code>' + (status.model || '-') + '</code></div>' +
      '<div class="kv"><span class="k">API Key</span>' + (status.hasApiKey ? '<span style="color:#3fb950;">Configured</span>' : '<span style="color:#f85149;">Missing</span>') + '</div>' +
      '<div class="kv"><span class="k">Last Reload</span>' + (status.lastReload ? timeAgo(status.lastReload) : '-') + '</div>';
    if (status.type) document.getElementById('cfgType').value = status.type;
    if (status.model) document.getElementById('cfgModel').value = status.model;
  } catch(e) {
    document.getElementById('providerStatus').innerHTML = '<div class="empty">Error: ' + escapeHtml(e.message) + '</div>';
  }
  // Load account info
  try {
    const data = await fetchJSON('/api/auth/me');
    if (data.user) {
      const u = data.user;
      document.getElementById('accountInfo').innerHTML =
        '<div class="kv"><span class="k">Username</span><code>' + u.username + '</code></div>' +
        '<div class="kv"><span class="k">Display Name</span>' + (u.display_name || '-') + '</div>' +
        '<div class="kv"><span class="k">Email</span>' + (u.email || '-') + '</div>' +
        '<div class="kv"><span class="k">Role</span><span class="badge ' + u.role + '">' + u.role + '</span></div>' +
        '<div class="kv"><span class="k">Auth Provider</span><code>' + u.provider + '</code></div>';
    }
  } catch(e) {}
  window.loadConversations();
}

export async function updateProvider() {
  const cfg = {
    type: document.getElementById('cfgType').value,
    model: document.getElementById('cfgModel').value,
    apiKey: document.getElementById('cfgApiKey').value,
    endpoint: document.getElementById('cfgEndpoint').value,
    maxTokens: 4096, temperature: 0.1,
  };
  if (!cfg.type || !cfg.model || !cfg.apiKey) {
    alert('Provider type, model, and API key are required');
    return;
  }
  try {
    const resp = await fetch('/api/provider/update', {
      method: 'POST', headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(cfg),
    });
    const result = await resp.json();
    if (resp.ok) {
      alert('Provider updated! ' + (result.message || ''));
      window.loadSettings();
    } else {
      alert('Error: ' + (result.error || 'Unknown'));
    }
  } catch(e) { alert('Error: ' + e.message); }
}

// --- Resources Browser ---
export async function loadResources(forceRefresh) {
  const kind = document.getElementById('resKind').value;
  const container = document.getElementById('resourcesTable');
  container.innerHTML = '<div class="loading">Loading ' + kind + '...</div>';

  // Ensure namespace list is loaded
  if (!window._allNamespaces) await loadNamespaceList();

  try {
    // Always fetch all namespaces, filter client-side
    const url = '/api/resources?kind=' + kind + (forceRefresh ? '&refresh=true' : '');
    const data = await fetchJSON(url);
    if (!data.items?.length) {
      container.innerHTML = '<div class="empty">No ' + kind + ' found</div>';
      return;
    }

    // Apply namespace filter
    const selectedNs = getSelectedNs();
    const filtered = selectedNs.size === 0 ? data.items :
      data.items.filter(r => selectedNs.has(r.namespace));

    if (filtered.length === 0) {
      container.innerHTML = '<div class="empty">No ' + kind + ' in selected namespace(s)</div>';
      return;
    }

    const detailKey = filtered[0].detail ? Object.keys(filtered[0].detail)[0] : null;
    container.innerHTML = `<p style="margin-bottom:8px;color:#8b949e;font-size:13px;">Showing ${filtered.length} of ${data.items.length} ${escapeHtml(kind)}</p>
      <table><thead><tr>
      <th>Name</th><th>Namespace</th><th>Ready</th><th>Type</th>
      <th>${detailKey || 'Detail'}</th><th>Age</th><th>Actions</th>
    </tr></thead><tbody>${filtered.map(r => {
      let detail = '';
      if (r.detail) { detail = Object.entries(r.detail).map(([k,v]) => `<span style="color:#8b949e;">${k}:</span> ${escapeHtml(v)}`).join('<br>'); }
      return `<tr>
        <td style="color:#58a6ff;font-family:monospace;">${escapeHtml(r.name)}</td>
        <td>${escapeHtml(r.namespace)}</td>
        <td>${r.ready ? `<span class="badge ${r.ready.includes('/') && r.ready.split('/')[0]==r.ready.split('/')[1] ? 'Ready' : 'NotReady'}">${r.ready}</span>` : ''}</td>
        <td>${r.type || ''}</td>
        <td style="font-size:13px;">${detail}</td>
        <td>${r.age}</td>
        <td><button onclick="viewYAML('${escapeHtml(kind)}','${escapeHtml(r.namespace)}','${escapeHtml(r.name)}')" class="btn-secondary" style="font-size:12px;padding:4px 10px;">YAML</button></td>
      </tr>`;
    }).join('')}</tbody></table>`;
  } catch(e) {
    if (isForbidden(e)) renderForbidden(container);
    else container.innerHTML = '<div class="error">Error: ' + escapeHtml(e.message) + '</div>';
  }
}

// --- Namespace multi-select ---
export async function loadNamespaceList() {
  try {
    const data = await fetchJSON('/api/resources?kind=namespaces');
    window._allNamespaces = (data.items || []).map(n => n.name).sort();
  } catch(e) {
    window._allNamespaces = [];
  }
  renderNsCheckboxes();
}

export function renderNsCheckboxes(filter) {
  const box = document.getElementById('nsCheckboxes');
  const nsList = window._allNamespaces || [];
  const filtered = filter ? nsList.filter(n => n.includes(filter)) : nsList;
  box.innerHTML = filtered.map(ns => {
    const checked = (window._selectedNs && window._selectedNs.has(ns)) ? 'checked' : '';
    return `<label style="display:flex;align-items:center;gap:6px;padding:4px 12px;cursor:pointer;font-size:13px;color:#c9d1d9;" onmouseover="this.style.background='#1c2128'" onmouseout="this.style.background='none'">
      <input type="checkbox" value="${escapeHtml(ns)}" ${checked} onchange="toggleNsSelection('${escapeHtml(ns)}', this.checked)" style="accent-color:#58a6ff;">
      <span>${escapeHtml(ns)}</span>
    </label>`;
  }).join('') || '<div style="padding:12px;color:#8b949e;font-size:13px;">No namespaces found</div>';
}

export function toggleNsDropdown() {
  const list = document.getElementById('nsCheckboxList');
  list.style.display = list.style.display === 'none' ? 'block' : 'none';
  if (list.style.display === 'block') document.getElementById('nsSearch').focus();
}

export function filterNsList() {
  const q = document.getElementById('nsSearch').value.toLowerCase();
  renderNsCheckboxes(q);
}

export function toggleNsSelection(ns, checked) {
  if (!window._selectedNs) window._selectedNs = new Set();
  if (checked) window._selectedNs.add(ns);
  else window._selectedNs.delete(ns);
  updateNsDisplay();
}

export function selectAllNs(all) {
  if (all) {
    window._selectedNs = new Set();
  } else {
    window._selectedNs = new Set(window._allNamespaces || []);
  }
  renderNsCheckboxes(document.getElementById('nsSearch').value.toLowerCase());
  updateNsDisplay();
}

export function getSelectedNs() {
  return window._selectedNs || new Set();
}

export function updateNsDisplay() {
  const sel = getSelectedNs();
  const text = sel.size === 0 ? 'All Namespaces' :
    sel.size <= 2 ? Array.from(sel).join(', ') :
    sel.size + ' Namespaces';
  document.getElementById('nsDisplayText').textContent = text;
}

// Close dropdown when clicking outside
document.addEventListener('click', (e) => {
  const dropdown = document.getElementById('nsDropdown');
  if (dropdown && !dropdown.contains(e.target)) {
    document.getElementById('nsCheckboxList').style.display = 'none';
  }
});

// --- CRD Browser ---
export async function loadCRDs(forceRefresh) {
  const container = document.getElementById('crdList');
  document.getElementById('crdDetail').style.display = 'none';
  container.style.display = 'block';
  container.innerHTML = '<div class="loading">Loading CRDs (counting instances...)...</div>';
  try {
    const url = '/api/crds?with_counts=true' + (forceRefresh ? '&refresh=true' : '');
    const data = await fetchJSON(url);
    if (!data.items?.length) {
      container.innerHTML = '<div class="empty">No CRDs found</div>';
      return;
    }
    window._allCRDs = data.items;
    renderCRDTable();
  } catch(e) { container.innerHTML = '<div style="color:#f85149;">Error: ' + escapeHtml(e.message) + '</div>'; }
}

export function filterCRDs() {
  renderCRDTable();
}

export function renderCRDTable() {
  const container = document.getElementById('crdList');
  const q = (document.getElementById('crdFilter')?.value || '').toLowerCase();
  const all = window._allCRDs || [];

  const filtered = q ? all.filter(c =>
    c.name.toLowerCase().includes(q) ||
    c.group.toLowerCase().includes(q) ||
    c.kind.toLowerCase().includes(q)
  ) : all;

  if (!filtered.length) {
    container.innerHTML = '<div class="empty">No CRDs match "' + escapeHtml(q) + '"</div>';
    return;
  }

  // Sort: count > 0 first (by count desc), then count=0 by name
  filtered.sort((a, b) => {
    if ((b.count || 0) !== (a.count || 0)) return (b.count || 0) - (a.count || 0);
    return a.name.localeCompare(b.name);
  });

  const withInstances = filtered.filter(c => c.count > 0).length;
  container.innerHTML = `<p style="margin-bottom:12px;color:#8b949e;font-size:13px;">
    ${filtered.length} of ${all.length} CRDs${q ? ' matching "' + escapeHtml(q) + '"' : ''} —
    ${withInstances} with active instances</p>
    <table><thead><tr><th>Name</th><th>Group</th><th>Version</th><th>Kind</th><th>Scope</th><th>Instances</th></tr></thead>
    <tbody>${filtered.map(c => {
      const countBadge = c.count > 0
        ? `<span class="badge Ready">${c.count}</span>`
        : `<span style="color:#484f58;font-size:12px;">0</span>`;
      const clickable = c.count > 0;
      return `<tr style="cursor:${clickable?'pointer':'default'};" ${clickable?`onclick="browseCRD('${c.group}','${c.version}','${c.plural}','${c.kind}')"`:''}>
        <td style="color:${clickable?'#58a6ff':'#8b949e'};font-family:monospace;">${escapeHtml(c.name)}</td>
        <td style="font-size:13px;color:#8b949e;">${escapeHtml(c.group)}</td>
        <td><code>${c.version}</code></td>
        <td style="color:${clickable?'#d2a8ff':'#8b949e'};">${c.kind}</td>
        <td><span class="badge ${c.scope==='Namespaced'?'Ready':'Normal'}">${c.scope}</span></td>
        <td style="text-align:center;">${countBadge}</td>
      </tr>`;
    }).join('')}</tbody></table>`;
}

export function showCRDList() {
  document.getElementById('crdDetail').style.display = 'none';
  document.getElementById('crdList').style.display = 'block';
}

export async function browseCRD(group, version, resource, kind) {
  document.getElementById('crdList').style.display = 'none';
  const detail = document.getElementById('crdDetail');
  detail.style.display = 'block';
  document.getElementById('crdDetailTitle').textContent = kind + ' (' + group + '/' + version + ')';
  const tableDiv = document.getElementById('crdResourcesTable');
  tableDiv.innerHTML = '<div class="loading">Loading...</div>';
  try {
    const url = '/api/crd-resources?group=' + group + '&version=' + version + '&resource=' + resource;
    const data = await fetchJSON(url);
    if (!data.items?.length) {
      tableDiv.innerHTML = '<div class="empty">No instances found</div>';
      return;
    }
    const detailKeys = new Set();
    data.items.forEach(i => Object.keys(i.detail || {}).forEach(k => detailKeys.add(k)));
    const dk = Array.from(detailKeys).slice(0, 4);
    tableDiv.innerHTML = `<table><thead><tr><th>Name</th><th>Namespace</th>${dk.map(k=>`<th>${escapeHtml(k)}</th>`).join('')}<th>Age</th><th>Actions</th></tr></thead>
      <tbody>${data.items.map(r => `<tr>
        <td style="color:#58a6ff;font-family:monospace;">${escapeHtml(r.name)}</td>
        <td>${escapeHtml(r.namespace)}</td>
        ${dk.map(k=>`<td style="font-size:13px;">${escapeHtml(r.detail?.[k]||'-')}</td>`).join('')}
        <td>${r.age}</td>
        <td><button onclick="viewYAML('','${escapeHtml(r.namespace)}','${escapeHtml(r.name)}','${escapeHtml(group)}','${escapeHtml(version)}','${escapeHtml(resource)}')" class="btn-secondary" style="font-size:12px;padding:4px 10px;">YAML</button></td>
      </tr>`).join('')}</tbody></table>`;
  } catch(e) { tableDiv.innerHTML = '<div style="color:#f85149;">Error: ' + escapeHtml(e.message) + '</div>'; }
}

// --- Node Pods Drill-down ---
export function closeNodePods() {
  document.getElementById('nodePodsOverlay').classList.remove('active');
}

export async function viewNodePods(nodeName) {
  document.getElementById('nodePodsOverlay').classList.add('active');
  document.getElementById('nodePodsTitle').textContent = nodeName;
  const container = document.getElementById('nodePodsTable');
  container.innerHTML = '<div class="loading">Loading pods...</div>';
  try {
    const data = await fetchJSON('/api/nodes/' + nodeName + '/pods');
    if (!data.pods?.length) {
      container.innerHTML = '<div class="empty">No pods on this node</div>';
      return;
    }
    container.innerHTML = `<table><thead><tr><th>Name</th><th>Namespace</th><th>Status</th><th>Restarts</th><th>IP</th><th>Containers</th><th>Age</th><th>Actions</th></tr></thead>
      <tbody>${data.pods.map(p => `<tr>
        <td style="color:#58a6ff;font-family:monospace;">${escapeHtml(p.name)}</td>
        <td>${escapeHtml(p.namespace)}</td>
        <td><span class="badge ${p.status==='Running'?'Ready':'Warning'}">${p.status}</span></td>
        <td>${p.restarts > 0 ? `<span style="color:#f85149;">${p.restarts}</span>` : '0'}</td>
        <td style="font-size:13px;">${p.ip||'-'}</td>
        <td style="font-size:12px;color:#8b949e;">${(p.containers||[]).join(', ')}</td>
        <td>${p.age}</td>
        <td>
          <button onclick="openLogViewer('${p.namespace}','${p.name}')" class="btn-secondary" style="font-size:12px;padding:4px 10px;">Logs</button>
          <button onclick="openTerminal('${p.namespace}','${p.name}')" class="btn-secondary" style="font-size:12px;padding:4px 10px;">Terminal</button>
          <button onclick="viewYAML('pods','${p.namespace}','${p.name}')" class="btn-secondary" style="font-size:12px;padding:4px 10px;">YAML</button>
        </td>
      </tr>`).join('')}</tbody></table>`;
  } catch(e) { container.innerHTML = '<div style="color:#f85149;">Error: ' + escapeHtml(e.message) + '</div>'; }
}

// --- Log Viewer ---
let logEventSource = null; // legacy compat, unused after v13.4 migration

export function closeLogViewer() {
  document.getElementById('logOverlay').classList.remove('active');
  if (logFetchController) { try { logFetchController.abort(); } catch(e) {} logFetchController = null; }
}

// (old clearLogs removed — newer definition below)

export async function openLogViewer(ns, name) {
  document.getElementById('logOverlay').classList.add('active');
  document.getElementById('logPodName').textContent = ns + '/' + name;
  document.getElementById('logOutput').innerHTML = '';
  logLines = [];
  document.getElementById('logSearch').value = '';

  // Load containers
  try {
    const data = await fetchJSON('/api/pods/' + ns + '/' + name + '/containers');
    const sel = document.getElementById('logContainer');
    sel.innerHTML = (data.containers || []).map((c, i) =>
      `<option value="${c.name}" ${i===0?'selected':''}>${c.name} ${c.ready?'\u2713':'\u2715'}</option>`).join('');
  } catch(e) { /* single container */ }

  openLogViewer._ns = ns;
  openLogViewer._name = name;
  restartLogs();
}

// Log viewer state
let logLines = []; // array of {text, level, matched}
let logFetchController = null;

export function classifyLogLevel(line) {
  const upper = line.toUpperCase();
  if (upper.includes('FATAL') || upper.includes('PANIC')) return 'fatal';
  if (upper.includes('ERROR') || upper.includes('ERR ') || upper.includes('\u2715')) return 'error';
  if (upper.includes('WARN') || upper.includes('WARNING')) return 'warn';
  if (upper.includes('DEBUG') || upper.includes('TRACE')) return 'debug';
  if (upper.includes('\u2713') || upper.includes('SUCCESS')) return 'success';
  return 'info';
}

export function restartLogs() {
  if (logFetchController) { logFetchController.abort(); logFetchController = null; }
  logLines = [];
  document.getElementById('logOutput').innerHTML = '';
  updateLogStatus();

  const ns = openLogViewer._ns;
  const name = openLogViewer._name;
  const container = document.getElementById('logContainer').value;
  const follow = document.getElementById('logFollow').checked;
  const url = '/api/pods/' + ns + '/' + name + '/logs?container=' + encodeURIComponent(container) +
              '&follow=' + follow + '&tailLines=500';

  logFetchController = new AbortController();
  fetch(url, { signal: logFetchController.signal }).then(resp => {
    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    function pump() {
      return reader.read().then(({done, value}) => {
        if (done) return;
        buffer += decoder.decode(value, {stream: true});
        const lines = buffer.split('\n');
        buffer = lines.pop();
        for (const line of lines) {
          if (!line.startsWith('data: ')) continue;
          try {
            const d = JSON.parse(line.substring(6));
            if (d.done) return;
            if (d.line) addLogLine(d.line);
          } catch(e) {}
        }
        return pump();
      });
    }
    return pump();
  }).catch(e => {
    if (e.name !== 'AbortError') {
      addLogLine('[Error: ' + e.message + ']');
    }
  });
}

export function addLogLine(text) {
  const level = classifyLogLevel(text);
  logLines.push({ text, level, matched: true });
  // Only append DOM if under 5000 lines (performance)
  if (logLines.length <= 5000) {
    appendLogLineDom(logLines[logLines.length - 1]);
  }
  updateLogStatus();
}

export function appendLogLineDom(entry) {
  if (!entry.matched) return;
  const filter = document.getElementById('logSearch').value.toLowerCase();
  if (filter && !entry.text.toLowerCase().includes(filter)) return;
  const output = document.getElementById('logOutput');
  const div = document.createElement('div');
  div.className = 'log-line log-' + entry.level;
  div.textContent = entry.text;
  output.appendChild(div);
  if (document.getElementById('logAutoScroll').checked) {
    const sc = document.getElementById('logScrollContainer');
    sc.scrollTop = sc.scrollHeight;
  }
}

export function filterLogs() {
  const filter = document.getElementById('logSearch').value.toLowerCase();
  const output = document.getElementById('logOutput');
  output.innerHTML = '';
  let shown = 0;
  for (const entry of logLines) {
    entry.matched = !filter || entry.text.toLowerCase().includes(filter);
    if (entry.matched && shown < 5000) {
      appendLogLineDom(entry);
      shown++;
    }
  }
  updateLogStatus();
}

export function updateLogStatus() {
  const total = logLines.length;
  const filter = document.getElementById('logSearch').value;
  document.getElementById('logLineCount').textContent = total + ' lines';
  const filteredEl = document.getElementById('logFilteredCount');
  if (filter) {
    const shown = document.querySelectorAll('.log-line').length;
    filteredEl.textContent = '(' + shown + ' shown)';
    filteredEl.style.display = 'inline';
  } else {
    filteredEl.style.display = 'none';
  }
}

export function downloadLogs() {
  const text = logLines.map(l => l.text).join('\n');
  const blob = new Blob([text], { type: 'text/plain' });
  const a = document.createElement('a');
  a.href = URL.createObjectURL(blob);
  a.download = (openLogViewer._ns || 'pod') + '-' + (openLogViewer._name || 'log') + '-' + Date.now() + '.log';
  a.click();
  URL.revokeObjectURL(a.href);
}

export function clearLogs() {
  logLines = [];
  document.getElementById('logOutput').innerHTML = '';
  updateLogStatus();
}

// --- Terminal ---
// --- YAML Viewer ---
export function closeYAMLViewer() {
  document.getElementById('yamlOverlay').classList.remove('active');
  // Reset to view mode
  exitYAMLEdit();
}

export function toggleYAMLEdit() {
  const pre = document.getElementById('yamlOutput');
  const editor = document.getElementById('yamlEditor');
  const editBtn = document.getElementById('yamlEditBtn');
  const applyBtn = document.getElementById('yamlApplyBtn');
  const indicator = document.getElementById('yamlEditIndicator');

  if (editor.style.display === 'none') {
    // Enter edit mode
    editor.value = pre.textContent;
    pre.style.display = 'none';
    editor.style.display = 'block';
    editor.focus();
    editBtn.textContent = 'Cancel';
    applyBtn.style.display = 'inline-block';
    indicator.style.display = 'inline-block';
  } else {
    // Exit edit mode
    exitYAMLEdit();
  }
}

export function exitYAMLEdit() {
  const pre = document.getElementById('yamlOutput');
  const editor = document.getElementById('yamlEditor');
  const editBtn = document.getElementById('yamlEditBtn');
  const applyBtn = document.getElementById('yamlApplyBtn');
  const indicator = document.getElementById('yamlEditIndicator');
  pre.style.display = 'block';
  editor.style.display = 'none';
  editBtn.textContent = 'Edit';
  applyBtn.style.display = 'none';
  indicator.style.display = 'none';
  const result = document.getElementById('yamlApplyResult');
  if (result) result.style.display = 'none';
}

export async function applyYAML() {
  const editor = document.getElementById('yamlEditor');
  const yaml = editor.value;
  if (!yaml.trim()) return;

  const applyBtn = document.getElementById('yamlApplyBtn');
  applyBtn.textContent = 'Applying...';
  applyBtn.disabled = true;

  try {
    const resp = await fetch('/api/yaml/apply', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ yaml: yaml, dryRun: false })
    });
    const data = await resp.json();
    if (!resp.ok) {
      throw new Error(data.error || data.message || 'Apply failed');
    }
    // Success
    const result = document.getElementById('yamlApplyResult');
    result.style.display = 'block';
    result.innerHTML = '<div style="padding:12px 16px;margin:8px;background:#0d2818;border:1px solid #3fb950;border-radius:6px;color:#3fb950;font-size:13px;">' + escapeHtml(data.message || 'Applied successfully') + '</div>';
    applyBtn.textContent = 'Apply';
    applyBtn.disabled = false;
    // Update the view mode with new YAML
    document.getElementById('yamlOutput').textContent = yaml;
    setTimeout(exitYAMLEdit, 1500);
  } catch(e) {
    const result = document.getElementById('yamlApplyResult');
    result.style.display = 'block';
    result.innerHTML = '<div style="padding:12px 16px;margin:8px;background:#2d0d0d;border:1px solid #f85149;border-radius:6px;color:#f85149;font-size:13px;">' + escapeHtml(e.message) + '</div>';
    applyBtn.textContent = 'Apply';
    applyBtn.disabled = false;
  }
}

export function copyYAML() {
  const text = document.getElementById('yamlOutput').textContent;
  navigator.clipboard.writeText(text).then(() => {
    const btn = document.getElementById('yamlCopyBtn');
    btn.textContent = 'Copied!';
    setTimeout(() => btn.textContent = 'Copy', 1500);
  });
}

export async function viewYAML(kind, ns, name, group, version, resource) {
  document.getElementById('yamlOverlay').classList.add('active');
  document.getElementById('yamlTitle').textContent = ns ? ns + '/' + name : name;
  const output = document.getElementById('yamlOutput');
  output.innerHTML = '<span style="color:#8b949e;">Loading YAML...</span>';

  try {
    let url = '/api/yaml?name=' + encodeURIComponent(name);
    if (kind) url += '&kind=' + encodeURIComponent(kind);
    if (ns) url += '&namespace=' + encodeURIComponent(ns);
    if (group) url += '&group=' + encodeURIComponent(group);
    if (version) url += '&version=' + encodeURIComponent(version);
    if (resource) url += '&resource=' + encodeURIComponent(resource);

    const data = await fetchJSON(url);
    output.textContent = data.yaml;
  } catch(e) {
    output.innerHTML = '<span style="color:#f85149;">Error: ' + escapeHtml(e.message) + '</span>';
  }
}

let termNs = '', termName = '';

export function closeTerminal() {
  document.getElementById('termOverlay').classList.remove('active');
  document.getElementById('termInput').value = '';
  document.getElementById('termOutput').innerHTML = '';
}

export async function openTerminal(ns, name) {
  document.getElementById('termOverlay').classList.add('active');
  document.getElementById('termPodName').textContent = ns + '/' + name;
  termNs = ns; termName = name;
  const output = document.getElementById('termOutput');
  output.innerHTML = '<div style="color:#8b949e;">Connected to ' + ns + '/' + name + '. Type a command and press Enter.</div>';

  // Run initial command
  await runCommand('hostname && whoami && pwd');

  // Focus input
  document.getElementById('termInput').focus();
  document.getElementById('termInput').onkeypress = async (e) => {
    if (e.key !== 'Enter') return;
    const cmd = e.target.value.trim();
    if (!cmd) return;
    e.target.value = '';
    await runCommand(cmd);
  };
}

export async function runCommand(cmd) {
  const output = document.getElementById('termOutput');
  output.innerHTML += '<div style="color:#58a6ff;margin-top:8px;">$ ' + escapeHtml(cmd) + '</div>';

  try {
    const resp = await fetch('/api/pods/' + termNs + '/' + termName + '/exec', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({command: cmd}),
    });
    const data = await resp.json();
    if (data.output) {
      const pre = document.createElement('div');
      pre.style.color = data.success ? '#c9d1d9' : '#f85149';
      pre.style.whiteSpace = 'pre-wrap';
      pre.textContent = data.output;
      output.appendChild(pre);
    }
    if (data.error) {
      output.innerHTML += '<div style="color:#f85149;">' + escapeHtml(data.error) + '</div>';
    }
  } catch(e) {
    output.innerHTML += '<div style="color:#f85149;">Error: ' + escapeHtml(e.message) + '</div>';
  }
  output.parentElement.scrollTop = output.parentElement.scrollHeight;
}
