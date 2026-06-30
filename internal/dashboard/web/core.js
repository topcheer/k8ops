const API = '';

// escapeHtml escapes HTML special characters to prevent XSS.
// Must be used when inserting any server-side or user-provided data into innerHTML.
function escapeHtml(s) {
  if (s == null) return '';
  return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

async function fetchJSON(url, opts) {
  const res = await fetch(url, opts);
  if (res.status === 401) {
    window.location.href = '/login.html';
    throw new Error('Unauthorized');
  }
  if (res.status === 403) {
    let detail = '';
    try { const e = await res.json(); detail = e.error || e.message || ''; } catch(_) {}
    const err = new Error('FORBIDDEN:' + detail);
    err.status = 403;
    err.detail = detail;
    throw err;
  }
  if (!res.ok) {
    let detail = '';
    try { const e = await res.json(); detail = e.error || e.message || ''; } catch(_) {}
    throw new Error(detail || `HTTP ${res.status}`);
  }
  return res.json();
}

function isForbidden(err) {
  return err && (err.status === 403 || (err.message && err.message.startsWith('FORBIDDEN')));
}

function renderForbidden(container) {
  container.innerHTML = `
    <div style="display:flex;flex-direction:column;align-items:center;justify-content:center;padding:48px 24px;color:#8b949e;">
      <div style="font-size:48px;margin-bottom:16px;">🔒</div>
      <div style="font-size:16px;font-weight:600;color:#f85149;margin-bottom:8px;">权限不足</div>
      <div style="font-size:13px;max-width:400px;text-align:center;line-height:1.6;">
        您的账户没有访问此资源的权限。请联系管理员调整角色或命名空间授权。
      </div>
      <div style="margin-top:16px;padding:8px 16px;background:#161b22;border-radius:6px;font-size:12px;font-family:monospace;color:#8b949e;">
        403 Forbidden
      </div>
    </div>`;
}

function badge(text) {
  return `<span class="badge ${text}">${text}</span>`;
}

function timeAgo(iso) {
  if (!iso) return '-';
  const diff = Date.now() - new Date(iso).getTime();
  const m = Math.floor(diff / 60000);
  if (m < 1) return 'just now';
  if (m < 60) return m + 'm ago';
  const h = Math.floor(m / 60);
  if (h < 24) return h + 'h ago';
  return Math.floor(h / 24) + 'd ago';
}

// --- Tabs with hash persistence ---
function showTab(name, btn) {
  document.querySelectorAll('[id^="tab-"]').forEach(el => el.classList.add('hidden'));
  document.getElementById('tab-' + name).classList.remove('hidden');
  document.querySelectorAll('.sidebar-nav button').forEach(b => b.classList.remove('active'));
  if (btn) btn.classList.add('active');
  // Update URL hash without scrolling
  if (location.hash !== '#' + name) {
    history.replaceState(null, '', '#' + name);
  }
  if (name === 'overview') loadOverview();
  if (name === 'diagnostics') loadDiagnostics();
  if (name === 'remediations') loadRemediations();
  if (name === 'optimizations') loadOptimizations();
  if (name === 'nodes') loadNodes(false);
  if (name === 'events') loadEvents();
  if (name === 'pods') loadPods(false);
  if (name === 'resources') loadResources(false);
  if (name === 'crds') loadCRDs(false);
  if (name === 'audit') loadAudit();
  if (name === 'settings') loadSettings();
  if (name === 'rbac') loadRBAC();
  if (name === 'cost') loadCost();
}

// Restore tab from URL hash on page load
function initTabFromHash() {
  const hash = location.hash.replace('#', '');
  const validTabs = ['overview','diagnostics','remediations','optimizations','nodes','events','pods','resources','crds','audit','settings','rbac'];
  if (hash && validTabs.includes(hash)) {
    const btn = document.querySelector('.sidebar-nav button[onclick*="' + hash + '"]');
    showTab(hash, btn);
  } else {
    loadOverview();
  }
}
