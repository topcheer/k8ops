// core.js — Core navigation, command palette, theme, namespace switcher
import { API, escapeHtml, fetchJSON, showToast } from './modules/utils.js';

// ============================
// Tab Navigation
// ============================
export function showTab(name, btn) {
  document.querySelectorAll('[id^="tab-"]').forEach(el => el.classList.add('hidden'));
  document.getElementById('tab-' + name).classList.remove('hidden');
  document.querySelectorAll('.sidebar-nav button').forEach(b => b.classList.remove('active'));
  if (btn) btn.classList.add('active');
  if (location.hash !== '#' + name) {
    history.replaceState(null, '', '#' + name);
  }
  if (name === 'overview') window.loadOverview();
  if (name === 'topology') window.loadTopology();
  if (name === 'diagnostics') window.loadDiagnostics();
  if (name === 'remediations') window.loadRemediations();
  if (name === 'optimizations') window.loadOptimizations();
  if (name === 'nodes') window.loadNodes(false);
  if (name === 'events') window.loadEvents();
  if (name === 'pods') window.loadPods(false);
  if (name === 'resources') window.loadResources(false);
  if (name === 'crds') window.loadCRDs(false);
  if (name === 'audit') window.loadAudit();
  if (name === 'settings') window.loadSettings();
  if (name === 'rbac') window.loadRBAC();
  if (name === 'security') window.loadSecurityAudit();
  if (name === 'api') window.loadAPIDocs();
  if (name === 'namespaces') window.loadNamespaceRanking();
  if (name === 'capacity') window.loadCapacity();
  if (name === 'compliance') window.loadCompliance();
  if (name === 'cost') window.loadCost();
}

export function initTabFromHash() {
  const hash = location.hash.replace('#', '');
  if (hash === 'chat') {
    window.openChatOverlay();
    return;
  }
  const validTabs = ['overview','diagnostics','remediations','optimizations','nodes','events','pods','resources','crds','audit','settings','rbac','cost','topology'];
  if (hash && validTabs.includes(hash)) {
    const btn = document.querySelector('.sidebar-nav button[onclick*="' + hash + '"]');
    showTab(hash, btn);
  } else {
    window.loadOverview();
  }
}

// Handle browser back/forward
window.addEventListener('hashchange', function() {
  var hash = location.hash.replace('#', '');
  var chatEl = document.getElementById('chatOverlay');
  if (hash === 'chat' && chatEl && !chatEl.classList.contains('active')) {
    window.openChatOverlay();
  } else if (hash !== 'chat' && chatEl && chatEl.classList.contains('active')) {
    chatEl.classList.remove('active');
  }
});

// ============================
// Command Palette (Ctrl+K)
// ============================
const cmdItems = [
  { icon: '\u25A0', label: 'Overview', category: 'Navigate', action: () => showTab('overview') },
  { icon: '\u26A0', label: 'Diagnostics', category: 'Navigate', action: () => showTab('diagnostics') },
  { icon: '\u2699', label: 'Remediations', category: 'Navigate', action: () => showTab('remediations') },
  { icon: '\u25B2', label: 'Optimizations', category: 'Navigate', action: () => showTab('optimizations') },
  { icon: '\uD83D\uDCB0', label: 'Cost Analysis', category: 'Navigate', action: () => showTab('cost') },
  { icon: '\u25CE', label: 'Nodes', category: 'Navigate', action: () => showTab('nodes') },
  { icon: '\u26A0', label: 'Events', category: 'Navigate', action: () => showTab('events') },
  { icon: '\u25A5', label: 'Pods', category: 'Navigate', action: () => showTab('pods') },
  { icon: '\u2715', label: 'Resources', category: 'Navigate', action: () => showTab('resources') },
  { icon: '\u2699', label: 'Custom Resources (CRDs)', category: 'Navigate', action: () => showTab('crds') },
  { icon: '\u2630', label: 'Audit Log', category: 'Navigate', action: () => showTab('audit') },
  { icon: '\uD83D\uDD12', label: 'Access Control (RBAC)', category: 'Admin', action: () => showTab('rbac') },
  { icon: '\u2699', label: 'Settings', category: 'Navigate', action: () => showTab('settings') },
  { icon: '\uD83D\uDCAC', label: 'Open AI Chat', category: 'Action', action: () => window.openChatOverlay() },
  { icon: '\uD83D\uDD0D', label: 'Run Diagnostics', category: 'Action', action: () => { showTab('diagnostics'); } },
  { icon: '\uD83D\uDCCA', label: 'View Cluster Cost', category: 'Action', action: () => showTab('cost') },
];

let cmdSelectedIdx = 0;
let cmdFiltered = [];

export function openCmdPalette() {
  document.getElementById('cmdPalette').style.display = 'flex';
  const input = document.getElementById('cmdInput');
  input.value = '';
  input.focus();
  cmdFilter('');
}

export function closeCmdPalette() {
  document.getElementById('cmdPalette').style.display = 'none';
}

export function cmdFilter(q) {
  q = q.toLowerCase().trim();
  if (!q) {
    cmdFiltered = cmdItems.slice();
  } else {
    cmdFiltered = cmdItems.filter(item =>
      item.label.toLowerCase().includes(q) || item.category.toLowerCase().includes(q)
    );
  }
  cmdSelectedIdx = 0;
  renderCmdResults();
}

function renderCmdResults() {
  const container = document.getElementById('cmdResults');
  if (cmdFiltered.length === 0) {
    container.innerHTML = '<div style="padding:20px;color:#484f58;text-align:center;">No results found</div>';
    return;
  }
  container.innerHTML = cmdFiltered.map((item, i) =>
    '<div class="cmd-item' + (i === cmdSelectedIdx ? ' selected' : '') + '" onclick="cmdSelect(' + i + ')">' +
    '<span class="cmd-item-icon">' + item.icon + '</span>' +
    '<span class="cmd-item-label">' + escapeHtml(item.label) + '</span>' +
    '<span class="cmd-item-category">' + escapeHtml(item.category) + '</span>' +
    '</div>'
  ).join('');
}

export function cmdMove(delta) {
  if (cmdFiltered.length === 0) return;
  cmdSelectedIdx = (cmdSelectedIdx + delta + cmdFiltered.length) % cmdFiltered.length;
  const items = document.querySelectorAll('.cmd-item');
  items.forEach((el, i) => {
    if (i === cmdSelectedIdx) el.classList.add('selected');
    else el.classList.remove('selected');
  });
  const sel = items[cmdSelectedIdx];
  if (sel) sel.scrollIntoView({ block: 'nearest' });
}

export function cmdSelect(idx) {
  if (idx === undefined) idx = cmdSelectedIdx;
  const item = cmdFiltered[idx];
  if (!item) return;
  closeCmdPalette();
  item.action();
}

// Global keyboard shortcut: Ctrl+K / Cmd+K
document.addEventListener('keydown', function(e) {
  if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
    e.preventDefault();
    const palette = document.getElementById('cmdPalette');
    if (palette.style.display === 'none' || !palette.style.display) {
      openCmdPalette();
    } else {
      closeCmdPalette();
    }
    return;
  }
  if (e.key === 'Escape') {
    const palette = document.getElementById('cmdPalette');
    if (palette && palette.style.display === 'flex') {
      closeCmdPalette();
      return;
    }
    const helpOverlay = document.getElementById('kbdHelpOverlay');
    if (helpOverlay && helpOverlay.style.display === 'flex') {
      helpOverlay.style.display = 'none';
      return;
    }
  }
  const palette = document.getElementById('cmdPalette');
  if (palette && palette.style.display === 'flex') {
    if (e.key === 'ArrowDown') { e.preventDefault(); cmdMove(1); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); cmdMove(-1); }
    else if (e.key === 'Enter') { e.preventDefault(); cmdSelect(); }
    return;
  }

  // --- Vim-style keyboard shortcuts ---
  // Skip if user is typing in an input/textarea
  const tag = e.target.tagName;
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || e.target.isContentEditable) return;
  // Skip if chat overlay is open
  const chatOverlay = document.getElementById('chatOverlay');
  if (chatOverlay && chatOverlay.classList.contains('active')) return;
  // Skip modifier combos
  if (e.ctrlKey || e.metaKey || e.altKey) return;

  // '/' → focus first search input on current tab
  if (e.key === '/') {
    e.preventDefault();
    const activeTab = document.querySelector('[id^="tab-"]:not(.hidden)');
    if (activeTab) {
      const search = activeTab.querySelector('.search-input, input[type="text"]');
      if (search) { search.focus(); return; }
    }
    // Fallback: namespace selector
    const nsFilter = document.getElementById('nsFilter');
    if (nsFilter) nsFilter.focus();
    return;
  }

  // '?' → show keyboard shortcut help
  if (e.key === '?') {
    e.preventDefault();
    const help = document.getElementById('kbdHelpOverlay');
    if (help) help.style.display = help.style.display === 'flex' ? 'none' : 'flex';
    return;
  }

  // 'g' prefix → wait for next key for tab navigation
  if (e.key === 'g' && !_gPrefix) {
    _gPrefix = true;
    setTimeout(function() { _gPrefix = false; }, 800);
    return;
  }
  if (_gPrefix) {
    _gPrefix = false;
    const gMap = {
      'o': 'overview', 'd': 'diagnostics', 'r': 'remediations',
      'x': 'optimizations', 'n': 'nodes', 'p': 'pods',
      'e': 'events', 's': 'resources', 'c': 'crds',
      'a': 'audit', 'u': 'rbac', 't': 'cost', 'g': 'settings',
    };
    const tabName = gMap[e.key];
    if (tabName) {
      e.preventDefault();
      const btn = document.querySelector('.sidebar-nav button[onclick*="' + tabName + '"]');
      showTab(tabName, btn);
    }
    return;
  }

  // 'j'/'k' → navigate table rows
  if (e.key === 'j' || e.key === 'k') {
    const activeTab = document.querySelector('[id^="tab-"]:not(.hidden)');
    if (!activeTab) return;
    const rows = activeTab.querySelectorAll('tbody tr');
    if (rows.length === 0) return;
    e.preventDefault();
    let currentIdx = -1;
    rows.forEach(function(r, i) {
      if (r.classList.contains('kbd-selected')) currentIdx = i;
      r.classList.remove('kbd-selected');
    });
    if (e.key === 'j') currentIdx = currentIdx < rows.length - 1 ? currentIdx + 1 : 0;
    else currentIdx = currentIdx > 0 ? currentIdx - 1 : rows.length - 1;
    rows[currentIdx].classList.add('kbd-selected');
    rows[currentIdx].scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    return;
  }

  // 'r' → refresh current tab
  if (e.key === 'r') {
    e.preventDefault();
    const activeBtn = document.querySelector('.sidebar-nav button.active');
    if (activeBtn) {
      const match = activeBtn.getAttribute('onclick').match(/showTab\('([^']+)'/);
      if (match) {
        const name = match[1];
        if (name === 'overview') window.loadOverview();
        else if (name === 'nodes') window.loadNodes();
        else if (name === 'pods') window.loadPods();
        else if (name === 'events') window.loadEvents();
        else if (name === 'audit') window.loadAudit();
      }
    }
    return;
  }
});

var _gPrefix = false;

// Wire up input listener when DOM is ready
document.addEventListener('DOMContentLoaded', function() {
  const input = document.getElementById('cmdInput');
  if (input) {
    input.addEventListener('input', function() { cmdFilter(this.value); });
  }
  loadNamespaceOptions();
  initTheme();
});

// ============================
// Theme Toggle (Dark/Light)
// ============================
function initTheme() {
  const saved = localStorage.getItem('k8ops_theme') || 'dark';
  applyTheme(saved);
}

function applyTheme(theme) {
  if (theme === 'light') {
    document.documentElement.setAttribute('data-theme', 'light');
    const btn = document.getElementById('themeToggle');
    if (btn) btn.innerHTML = '&#9728; Light';
  } else {
    document.documentElement.removeAttribute('data-theme');
    const btn = document.getElementById('themeToggle');
    if (btn) btn.innerHTML = '&#9790; Dark';
  }
}

export function toggleTheme() {
  const current = document.documentElement.getAttribute('data-theme') === 'light' ? 'light' : 'dark';
  const next = current === 'light' ? 'dark' : 'light';
  applyTheme(next);
  localStorage.setItem('k8ops_theme', next);
  const btn = document.getElementById('themeToggle');
  if (btn) btn.innerHTML = next === 'light' ? '\u2600 Light' : '\u263E Dark';
  showToast('Theme: ' + next, 'info', 1500);
}

// ============================
// Namespace Switcher
// ============================
let _currentNamespace = '';

export function getCurrentNamespace() {
  return _currentNamespace;
}

async function loadNamespaceOptions() {
  try {
    const data = await fetchJSON('/api/cluster/overview');
    const nsData = await fetchJSON('/api/resources?resource=namespaces&listOnly=true').catch(() => null);
    const sel = document.getElementById('nsFilter');
    if (!sel) return;
    const current = sel.value;
    sel.innerHTML = '<option value="">All Namespaces</option>';
    let names = [];
    if (nsData && nsData.items) {
      names = nsData.items.map(function(i) { return i.name || (i.metadata ? i.metadata.name : ''); }).filter(Boolean).sort();
    }
    for (var i = 0; i < names.length; i++) {
      var opt = document.createElement('option');
      opt.value = names[i];
      opt.textContent = names[i];
      sel.appendChild(opt);
    }
    sel.value = current;
    const saved = localStorage.getItem('k8ops_ns');
    if (saved && !current) {
      sel.value = saved;
      _currentNamespace = saved;
    }
  } catch(e) { /* silent fail */ }
}

export function onNsChange() {
  const sel = document.getElementById('nsFilter');
  _currentNamespace = sel.value;
  localStorage.setItem('k8ops_ns', _currentNamespace);
  showToast('Namespace: ' + (_currentNamespace || 'all'), 'info', 2000);
  const activeTab = document.querySelector('.sidebar-nav button.active');
  if (activeTab) {
    const tabName = activeTab.getAttribute('onclick');
    if (tabName) {
      const match = tabName.match(/showTab\('([^']+)'/);
      if (match) {
        const name = match[1];
        if (name === 'overview') window.loadOverview();
        else if (name === 'nodes') window.loadNodes();
        else if (name === 'events') window.loadEvents();
        else if (name === 'pods') window.loadPods();
        else if (name === 'resources') window.loadResources && window.loadResources();
      }
    }
  }
}

// ============================
// Table Search Filter
// ============================
export function filterTable(containerId, inputId) {
  var q = (document.getElementById(inputId) || {}).value;
  if (!q) {
    q = '';
  }
  q = q.toLowerCase().trim();
  var container = document.getElementById(containerId);
  if (!container) return;
  var rows = container.querySelectorAll('tbody tr');
  for (var i = 0; i < rows.length; i++) {
    var text = rows[i].textContent.toLowerCase();
    rows[i].style.display = q && text.indexOf(q) === -1 ? 'none' : '';
  }
  var shown = 0;
  for (var i = 0; i < rows.length; i++) {
    if (rows[i].style.display !== 'none') shown++;
  }
  var badge = container.querySelector('.filter-count');
  if (!badge) {
    badge = document.createElement('div');
    badge.className = 'filter-count';
    container.insertBefore(badge, container.firstChild);
  }
  if (q) {
    badge.textContent = shown + ' / ' + rows.length + ' matching';
    badge.style.display = 'block';
  } else {
    badge.style.display = 'none';
  }
}

// --- Notification Center ---
var _notifItems = [];
var _notifMaxItems = 50;
var _notifPolling = null;

export function toggleNotifPanel() {
  var panel = document.getElementById('notifPanel');
  if (!panel) return;
  panel.style.display = panel.style.display === 'none' ? 'block' : 'none';
  if (panel.style.display === 'block') {
    // Clear unread badge when opened
    var badge = document.getElementById('notifBadge');
    if (badge) badge.classList.remove('notif-pulse');
  }
}

export function initNotificationCenter(alerts) {
  // Reset and populate from initial alerts
  _notifItems = [];
  var list = document.getElementById('notifList');
  if (!list) return;

  if (alerts && alerts.length) {
    for (var i = 0; i < alerts.length; i++) {
      addNotification(alerts[i]);
    }
  } else {
    list.innerHTML = '<div class="notif-empty">No alerts. Cluster is healthy.</div>';
    updateNotifBadge();
  }
}

export function addNotification(alert) {
  if (!alert) return;

  var list = document.getElementById('notifList');
  if (!list) return;

  // Remove empty placeholder
  var empty = list.querySelector('.notif-empty');
  if (empty) empty.remove();

  _notifItems.unshift(alert);
  if (_notifItems.length > _notifMaxItems) _notifItems.pop();

  // Determine icon and severity class
  var sev = alert.severity || 'info';
  var icon = '\u2139'; // info
  var cls = 'notif-info';
  if (sev === 'critical') { icon = '\u26A0'; cls = 'notif-critical'; }
  else if (sev === 'warning') { icon = '\u26A0'; cls = 'notif-warn'; }
  else if (sev === 'success') { icon = '\u2713'; cls = 'notif-success'; }

  var item = document.createElement('div');
  item.className = 'notif-item ' + cls;
  var timeStr = alert.time ? timeAgo(alert.time) : 'just now';
  item.innerHTML =
    '<span class="notif-item-icon">' + icon + '</span>' +
    '<div class="notif-item-body">' +
      '<div class="notif-item-text">' + escapeHtml(alert.text || alert.message || '') + '</div>' +
      '<div class="notif-item-meta">' +
        '<span class="notif-item-time">' + timeStr + '</span>' +
      '</div>' +
    '</div>';
  list.insertBefore(item, list.firstChild);

  // Pulse the badge
  var badge = document.getElementById('notifBadge');
  if (badge) badge.classList.add('notif-pulse');

  updateNotifBadge();
}

function updateNotifBadge() {
  var badge = document.getElementById('notifBadge');
  if (!badge) return;
  if (_notifItems.length > 0) {
    badge.textContent = _notifItems.length > 99 ? '99+' : String(_notifItems.length);
    badge.style.display = 'inline-block';
  } else {
    badge.style.display = 'none';
  }
}

// Auto-poll cluster for alerts every 60s
export function startNotifPolling() {
  if (_notifPolling) clearInterval(_notifPolling);
  pollNotifAlerts(); // immediate
  _notifPolling = setInterval(pollNotifAlerts, 60000);
}

async function pollNotifAlerts() {
  try {
    var data = await fetchJSON('/api/cluster/overview');
    var alerts = [];

    // Check failed diagnostics
    var diags = data.diagnostics || {};
    var failed = (diags.byPhase && diags.byPhase.Failed) || 0;
    if (failed > 0) {
      alerts.push({ severity: 'critical', text: failed + ' failed diagnostic' + (failed > 1 ? 's' : '') + ' need attention', time: new Date().toISOString() });
    }

    // Check pending remediations
    var rems = data.remediations || {};
    var pending = (rems.byPhase && rems.byPhase.Pending) || 0;
    if (pending > 0) {
      alerts.push({ severity: 'warning', text: pending + ' remediation plan' + (pending > 1 ? 's' : '') + ' awaiting approval', time: new Date().toISOString() });
    }

    if (alerts.length > 0) {
      initNotificationCenter(alerts);
    }
  } catch(e) { /* silently ignore */ }
}

// ============================
// Connection Status Monitor
// ============================
var _connOnline = true;
var _connCheckTimer = null;

export function setConnStatus(state) {
  var el = document.getElementById('connStatus');
  if (!el) return;
  var dot = el.querySelector('.conn-dot');
  var label = el.querySelector('.conn-label');
  el.className = 'conn-status';
  if (state === 'online') {
    el.classList.add('conn-online');
    if (label) label.textContent = 'Online';
    if (!_connOnline) {
      showToast('Connection restored', 'success', 3000);
    }
    _connOnline = true;
  } else if (state === 'offline') {
    el.classList.add('conn-offline');
    if (label) label.textContent = 'Offline';
    if (_connOnline) {
      showToast('Connection lost. Retrying...', 'error', 5000);
    }
    _connOnline = false;
  } else if (state === 'reconnecting') {
    el.classList.add('conn-reconnecting');
    if (label) label.textContent = 'Connecting';
    _connOnline = false;
  }
}

// Periodic health check
export function startConnMonitor() {
  if (_connCheckTimer) clearInterval(_connCheckTimer);
  _connCheckTimer = setInterval(function() {
    fetch('/api/version', { signal: AbortSignal.timeout ? AbortSignal.timeout(5000) : undefined })
      .then(function(r) {
        if (r.ok) setConnStatus('online');
        else setConnStatus('reconnecting');
      })
      .catch(function() {
        setConnStatus('offline');
      });
  }, 15000); // check every 15s
}

// Also monitor window online/offline events
window.addEventListener('online', function() { setConnStatus('reconnecting'); });
window.addEventListener('offline', function() { setConnStatus('offline'); });
