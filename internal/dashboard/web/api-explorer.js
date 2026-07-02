// api-explorer.js — Interactive API Explorer page for k8ops dashboard

import { escapeHtml, fetchJSON } from './modules/utils.js';

let apiSpec = null;
let apiTagOrder = [];
let apiTagGroups = {};

export async function loadAPIDocs() {
  const container = document.getElementById('apiExplorerContent');
  if (container) container.innerHTML = '<div class="loading">Loading API specification...</div>';

  try {
    const data = await fetchJSON('/api/docs');
    apiSpec = data.spec;
    apiTagOrder = data.tagOrder || [];
    apiTagGroups = data.tagGroups || {};
    renderAPIDocs(data);
  } catch (err) {
    if (container) {
      container.innerHTML = '<div class="empty-state">Failed to load API docs: ' + escapeHtml(err.message) + '</div>';
    }
  }
}

function methodColor(method) {
  const colors = {
    get: '#58a6ff', post: '#3fb950', put: '#d29922',
    delete: '#f85149', patch: '#db6d28',
  };
  return colors[method] || '#8b949e';
}

function renderAPIDocs(data) {
  const container = document.getElementById('apiExplorerContent');
  if (!container) return;

  // Build tag filter dropdown
  const tagOptions = apiTagOrder.map(t => `<option value="${escapeHtml(t)}">${escapeHtml(t)}</option>`).join('');

  let html = `
    <div style="display:flex;gap:12px;flex-wrap:wrap;margin-bottom:16px;align-items:center;">
      <div class="stat-card" style="min-width:100px;">
        <div class="stat-value">${data.endpointCount}</div>
        <div class="stat-label">Endpoints</div>
      </div>
      <div class="stat-card" style="min-width:100px;">
        <div class="stat-value">${apiTagOrder.length}</div>
        <div class="stat-label">Categories</div>
      </div>
      <div style="flex:1;min-width:200px;">
        <input type="text" id="apiSearchInput" class="search-input" placeholder="Search endpoints..."
          oninput="filterAPIEndpoints()" value="" style="width:100%;">
      </div>
      <select id="apiTagFilter" class="ns-selector" onchange="filterAPIEndpoints()">
        <option value="">All Categories</option>
        ${tagOptions}
      </select>
      <button onclick="downloadOpenAPISpec()" class="btn-secondary" title="Download OpenAPI 3.0 JSON">
        Download Spec
      </button>
    </div>

    <div id="apiEndpointList">`;

  // Render grouped by tag
  for (const tag of apiTagOrder) {
    const ops = apiTagGroups[tag];
    if (!ops || ops.length === 0) continue;

    html += `<div class="api-tag-group" data-tag="${escapeHtml(tag)}">
      <div class="api-tag-header" onclick="toggleAPITag(this)">
        <span class="api-tag-name">${escapeHtml(tag)}</span>
        <span class="api-tag-count">${ops.length}</span>
        <span class="api-toggle-icon">&#9660;</span>
      </div>
      <div class="api-tag-body">`;

    for (const op of ops) {
      const color = methodColor(op.method);
      html += `<div class="api-endpoint" data-search="${escapeHtml((op.method + ' ' + op.path + ' ' + op.summary + ' ' + (op.description||'')).toLowerCase())}">
        <div class="api-endpoint-header" onclick="toggleAPIDetail(this)">
          <span class="api-method-badge" style="background:${color}22;color:${color};">${op.method.toUpperCase()}</span>
          <code class="api-path">${escapeHtml(op.path)}</code>
          <span class="api-summary">${escapeHtml(op.summary)}</span>
          <span class="api-toggle-icon">&#9654;</span>
        </div>
        <div class="api-endpoint-detail" style="display:none;">
          ${op.description ? `<p style="color:#8b949e;font-size:13px;margin:8px 0;">${escapeHtml(op.description)}</p>` : ''}
          <div class="api-actions">
            <button class="btn-try" onclick="tryAPIEndpoint('${escapeHtml(op.method)}','${escapeHtml(op.path)}')">Try it</button>
            <button class="btn-copy" onclick="copyAPIPath('${escapeHtml(op.path)}')">Copy path</button>
          </div>
        </div>
      </div>`;
    }

    html += `</div></div>`;
  }

  html += `</div>
    <!-- Try-it modal -->
    <div id="apiTryModal" class="chat-overlay" style="display:none;">
      <div class="chat-header">
        <div class="title" id="apiTryTitle">Try API</div>
        <button onclick="closeAPITryModal()" class="btn-secondary" style="color:#f85149;">Close</button>
      </div>
      <div class="chat-body" style="padding:16px;">
        <div id="apiTryBody"></div>
      </div>
    </div>`;

  container.innerHTML = html;
}

export function toggleAPITag(headerEl) {
  const body = headerEl.nextElementSibling;
  if (body.style.display === 'none') {
    body.style.display = '';
    headerEl.querySelector('.api-toggle-icon').innerHTML = '&#9660;';
  } else {
    body.style.display = 'none';
    headerEl.querySelector('.api-toggle-icon').innerHTML = '&#9654;';
  }
}

export function toggleAPIDetail(headerEl) {
  const detail = headerEl.nextElementSibling;
  if (detail.style.display === 'none') {
    detail.style.display = '';
    headerEl.querySelector('.api-toggle-icon').innerHTML = '&#9660;';
  } else {
    detail.style.display = 'none';
    headerEl.querySelector('.api-toggle-icon').innerHTML = '&#9654;';
  }
}

export function filterAPIEndpoints() {
  const search = (document.getElementById('apiSearchInput')?.value || '').toLowerCase();
  const tagFilter = document.getElementById('apiTagFilter')?.value || '';

  document.querySelectorAll('.api-tag-group').forEach(group => {
    const tag = group.dataset.tag;
    const tagMatch = !tagFilter || tag === tagFilter;
    if (!tagMatch) {
      group.style.display = 'none';
      return;
    }

    let visibleCount = 0;
    group.querySelectorAll('.api-endpoint').forEach(ep => {
      const searchData = ep.dataset.search || '';
      if (!search || searchData.includes(search)) {
        ep.style.display = '';
        visibleCount++;
      } else {
        ep.style.display = 'none';
      }
    });

    // Hide tag group if no visible endpoints
    group.style.display = visibleCount > 0 ? '' : 'none';
  });
}

export function tryAPIEndpoint(method, path) {
  const modal = document.getElementById('apiTryModal');
  const body = document.getElementById('apiTryBody');
  const title = document.getElementById('apiTryTitle');
  if (!modal || !body) return;

  title.textContent = method.toUpperCase() + ' ' + path;
  modal.style.display = 'flex';

  // Build the try-it form
  const hasBody = method === 'post' || method === 'put' || method === 'patch';

  body.innerHTML = `
    <div style="max-width:800px;margin:0 auto;">
      <div style="margin-bottom:16px;">
        <label style="display:block;font-size:13px;color:#8b949e;margin-bottom:6px;">Path Parameters (replace {placeholders})</label>
        <input type="text" id="apiTryPath" class="search-input" value="${escapeHtml(path)}" style="width:100%;font-family:monospace;">
      </div>
      <div style="margin-bottom:16px;">
        <label style="display:block;font-size:13px;color:#8b949e;margin-bottom:6px;">Query Parameters (key=value&...)</label>
        <input type="text" id="apiTryQuery" class="search-input" placeholder="namespace=default&limit=10" style="width:100%;">
      </div>
      ${hasBody ? `
      <div style="margin-bottom:16px;">
        <label style="display:block;font-size:13px;color:#8b949e;margin-bottom:6px;">Request Body (JSON)</label>
        <textarea id="apiTryBody2" class="yaml-editor" style="min-height:120px;font-family:monospace;font-size:13px;"
          placeholder='{"key": "value"}'>{}</textarea>
      </div>` : ''}
      <div style="margin-bottom:16px;">
        <button onclick="executeAPITry('${escapeHtml(method)}')" class="btn-primary" style="padding:8px 24px;">Send Request</button>
      </div>
      <div id="apiTryResult" style="margin-top:16px;"></div>
    </div>`;
}

export async function executeAPITry(method) {
  const pathInput = document.getElementById('apiTryPath');
  const queryInput = document.getElementById('apiTryQuery');
  const bodyInput = document.getElementById('apiTryBody2');
  const resultDiv = document.getElementById('apiTryResult');
  if (!pathInput || !resultDiv) return;

  let url = pathInput.value.trim();
  const query = queryInput?.value.trim();
  if (query) url += '?' + query;

  resultDiv.innerHTML = '<div class="loading">Sending request...</div>';

  const startTime = Date.now();
  try {
    const opts = { method: method.toUpperCase(), headers: {} };
    if (bodyInput && (method === 'post' || method === 'put' || method === 'patch')) {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = bodyInput.value;
    }

    const res = await fetch(url, opts);
    const elapsed = Date.now() - startTime;
    const text = await res.text();
    const statusColor = res.ok ? '#3fb950' : '#f85149';

    let formatted = text;
    try {
      formatted = JSON.stringify(JSON.parse(text), null, 2);
    } catch (_) { /* keep raw text */ }

    resultDiv.innerHTML = `
      <div style="margin-bottom:8px;">
        <span style="background:${statusColor}22;color:${statusColor};padding:2px 10px;border-radius:4px;font-weight:600;">${res.status} ${res.statusText}</span>
        <span style="color:#8b949e;font-size:12px;margin-left:8px;">${elapsed}ms</span>
      </div>
      <pre style="background:var(--bg-primary);border:1px solid var(--border-color);border-radius:8px;padding:16px;overflow:auto;max-height:400px;font-size:13px;line-height:1.5;">${escapeHtml(formatted)}</pre>`;
  } catch (err) {
    resultDiv.innerHTML = `<div style="color:#f85149;">Error: ${escapeHtml(err.message)}</div>`;
  }
}

export function closeAPITryModal() {
  const modal = document.getElementById('apiTryModal');
  if (modal) modal.style.display = 'none';
}

export function copyAPIPath(path) {
  navigator.clipboard.writeText(path).then(() => {
    // Brief visual feedback
  }).catch(() => {});
}

export function downloadOpenAPISpec() {
  if (!apiSpec) return;
  const blob = new Blob([JSON.stringify(apiSpec, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = 'k8ops-openapi.json';
  a.click();
  URL.revokeObjectURL(url);
}
