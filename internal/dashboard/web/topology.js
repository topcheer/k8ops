// --- Cluster Topology Visualization ---
// Draws an SVG showing nodes → pods with health status colors
import { escapeHtml, truncateText, podPhaseColor, barColor, timeAgo } from './modules/utils.js';

export async function loadTopology() {
  const container = document.getElementById('topologyContainer');
  if (!container) return;

  try {
    const [nodesRes, podsRes] = await Promise.all([
      fetchJSON('/api/nodes').catch(() => ({ items: [] })),
      fetchJSON('/api/pods').catch(() => ({ items: [] }))
    ]);

    const nodes = nodesRes.items || [];
    const pods = podsRes.items || [];

    if (nodes.length === 0) {
      container.innerHTML = '<div class="empty">No nodes found</div>';
      return;
    }

    // Group pods by node
    const podsByNode = {};
    const unassignedPods = [];
    for (const p of pods) {
      if (p.node) {
        if (!podsByNode[p.node]) podsByNode[p.node] = [];
        podsByNode[p.node].push(p);
      } else {
        unassignedPods.push(p);
      }
    }

    // Layout: each node is a column
    const nodeWidth = 200;
    const nodeGap = 40;
    const podHeight = 22;
    const podGap = 3;
    const headerHeight = 70;
    const svgMargin = 20;

    // Calculate column heights
    let maxPodsInNode = 0;
    for (const n of nodes) {
      const count = (podsByNode[n.name] || []).length;
      if (count > maxPodsInNode) maxPodsInNode = count;
    }
    if (unassignedPods.length > maxPodsInNode) maxPodsInNode = unassignedPods.length;

    const totalWidth = (nodes.length + (unassignedPods.length > 0 ? 1 : 0)) * (nodeWidth + nodeGap) + svgMargin * 2;
    const colHeight = Math.max(maxPodsInNode * (podHeight + podGap), 200);
    const totalHeight = headerHeight + colHeight + svgMargin * 2;

    let svg = '<svg class="topology-svg" viewBox="0 0 ' + totalWidth + ' ' + totalHeight + '" preserveAspectRatio="xMidYMin meet" style="width:100%;min-width:' + totalWidth + 'px;">';

    // Draw each node column
    let x = svgMargin;
    for (const n of nodes) {
      const nodePods = podsByNode[n.name] || [];
      const isReady = n.status === 'Ready';
      const nodeColor = isReady ? '#3fb950' : '#f85149';
      const cpuPct = Math.round(n.cpuRequestedPct || 0);
      const memPct = Math.round(n.memRequestedPct || 0);

      // Node box
      svg += '<rect x="' + x + '" y="' + svgMargin + '" width="' + nodeWidth + '" height="' + headerHeight + '" rx="8" fill="#161b22" stroke="' + nodeColor + '" stroke-width="2"/>';
      // Node name
      svg += '<text x="' + (x + 10) + '" y="' + (svgMargin + 20) + '" fill="#c9d1d9" font-size="13" font-weight="600">' + escapeHtml(truncateText(n.name, 22)) + '</text>';
      // Role tag
      svg += '<text x="' + (x + 10) + '" y="' + (svgMargin + 38) + '" fill="#8b949e" font-size="11">' + escapeHtml(n.role) + ' / ' + escapeHtml(n.os) + '/' + escapeHtml(n.arch) + '</text>';
      // Resource bars
      svg += '<rect x="' + (x + 10) + '" y="' + (svgMargin + 46) + '" width="80" height="6" rx="3" fill="#21262d"/>';
      svg += '<rect x="' + (x + 10) + '" y="' + (svgMargin + 46) + '" width="' + (cpuPct * 0.8) + '" height="6" rx="3" fill="' + barColor(cpuPct) + '"/>';
      svg += '<text x="' + (x + 95) + '" y="' + (svgMargin + 52) + '" fill="#8b949e" font-size="10">CPU ' + cpuPct + '%</text>';
      svg += '<rect x="' + (x + 10) + '" y="' + (svgMargin + 56) + '" width="80" height="6" rx="3" fill="#21262d"/>';
      svg += '<rect x="' + (x + 10) + '" y="' + (svgMargin + 56) + '" width="' + (memPct * 0.8) + '" height="6" rx="3" fill="' + barColor(memPct) + '"/>';
      svg += '<text x="' + (x + 95) + '" y="' + (svgMargin + 62) + '" fill="#8b949e" font-size="10">MEM ' + memPct + '%</text>';

      // Pods
      let py = svgMargin + headerHeight + 10;
      for (const p of nodePods) {
        const phase = p.phase || 'Unknown';
        const phaseColor = podPhaseColor(phase);
        const isCrash = p.restarts > 3;

        svg += '<g class="topo-pod-group" onclick="showPodDetail(\'' + escapeHtml(p.namespace) + '\',\'' + escapeHtml(p.name) + '\')">';
        svg += '<rect x="' + x + '" y="' + py + '" width="' + nodeWidth + '" height="' + podHeight + '" rx="4" fill="#0d1117" stroke="' + phaseColor + '" stroke-width="1" class="topo-pod-rect' + (isCrash ? ' topo-pod-crash' : '') + '"/>';
        // Phase indicator dot
        svg += '<circle cx="' + (x + 10) + '" cy="' + (py + podHeight/2) + '" r="4" fill="' + phaseColor + '"/>';
        // Pod name
        svg += '<text x="' + (x + 22) + '" y="' + (py + 14) + '" fill="#c9d1d9" font-size="11" font-family="monospace">' + escapeHtml(truncateText(p.name, 24)) + '</text>';
        // Namespace
        svg += '<text x="' + (x + 22) + '" y="' + (py + podHeight - 4) + '" fill="#484f58" font-size="9">' + escapeHtml(truncateText(p.namespace, 20)) + '</text>';
        // Restart indicator
        if (isCrash) {
          svg += '<text x="' + (x + nodeWidth - 24) + '" y="' + (py + 14) + '" fill="#f85149" font-size="11" font-weight="600">' + p.restarts + 'x</text>';
        }
        svg += '</g>';
        py += podHeight + podGap;
      }

      // Pod count summary
      if (nodePods.length > 0) {
        svg += '<text x="' + x + '" y="' + (py + 12) + '" fill="#3fb950" font-size="10">' + nodePods.length + ' pods</text>';
      }

      x += nodeWidth + nodeGap;
    }

    // Unassigned pods
    if (unassignedPods.length > 0) {
      svg += '<rect x="' + x + '" y="' + svgMargin + '" width="' + nodeWidth + '" height="' + headerHeight + '" rx="8" fill="#161b22" stroke="#8b949e" stroke-width="2" stroke-dasharray="4"/>';
      svg += '<text x="' + (x + 10) + '" y="' + (svgMargin + 28) + '" fill="#8b949e" font-size="13" font-weight="600">Unassigned</text>';
      svg += '<text x="' + (x + 10) + '" y="' + (svgMargin + 48) + '" fill="#484f58" font-size="11">Pending / Unscheduled</text>';
      let py = svgMargin + headerHeight + 10;
      for (const p of unassignedPods) {
        const phaseColor = podPhaseColor(p.phase);
        svg += '<rect x="' + x + '" y="' + py + '" width="' + nodeWidth + '" height="' + podHeight + '" rx="4" fill="#0d1117" stroke="' + phaseColor + '" stroke-width="1"/>';
        svg += '<circle cx="' + (x + 10) + '" cy="' + (py + podHeight/2) + '" r="4" fill="' + phaseColor + '"/>';
        svg += '<text x="' + (x + 22) + '" y="' + (py + 14) + '" fill="#c9d1d9" font-size="11" font-family="monospace">' + escapeHtml(truncateText(p.name, 24)) + '</text>';
        svg += '<text x="' + (x + 22) + '" y="' + (py + podHeight - 4) + '" fill="#484f58" font-size="9">' + escapeHtml(truncateText(p.namespace, 20)) + '</text>';
        py += podHeight + podGap;
      }
    }

    // Legend
    svg += '<g transform="translate(' + svgMargin + ', ' + (totalHeight - 20) + ')">';
    svg += '<circle cx="0" cy="0" r="4" fill="#3fb950"/><text x="10" y="4" fill="#8b949e" font-size="10">Running</text>';
    svg += '<circle cx="70" cy="0" r="4" fill="#d29922"/><text x="80" y="4" fill="#8b949e" font-size="10">Pending</text>';
    svg += '<circle cx="150" cy="0" r="4" fill="#f85149"/><text x="160" y="4" fill="#8b949e" font-size="10">Failed</text>';
    svg += '<circle cx="220" cy="0" r="4" fill="#8b949e"/><text x="230" y="4" fill="#8b949e" font-size="10">Unknown/Succeeded</text>';
    svg += '</g>';

    svg += '</svg>';

    container.innerHTML = '<div class="topology-wrapper">' + svg + '</div>' +
      '<div class="topology-stats">' +
        '<span class="topo-stat"><span class="status-dot dot-ok"></span> ' + nodes.filter(function(n){return n.status==='Ready'}).length + ' Ready Nodes</span>' +
        '<span class="topo-stat"><span class="status-dot dot-err"></span> ' + nodes.filter(function(n){return n.status!=='Ready'}).length + ' NotReady</span>' +
        '<span class="topo-stat">' + pods.length + ' Pods Total</span>' +
        '<span class="topo-stat">' + pods.filter(function(p){return p.phase==='Running'}).length + ' Running</span>' +
        '<span class="topo-stat">' + pods.filter(function(p){return p.restarts>3}).length + ' CrashLoop</span>' +
      '</div>';
  } catch(e) {
    container.innerHTML = '<div class="empty">Error loading topology: ' + escapeHtml(e.message) + '</div>';
  }
}

export function showPodDetail(ns, name) {
  // Open the log viewer as a quick action
  if (typeof openLogViewer === 'function') {
    openLogViewer(ns, name);
  }
}

// --- Notification Center ---
var _notifSeen = {};
var _notifPollTimer = null;

export function initNotificationCenter() {
  // Load initial state
  fetchNotifications();
  // Poll every 60s
  if (_notifPollTimer) clearInterval(_notifPollTimer);
  _notifPollTimer = setInterval(fetchNotifications, 60000);
}

export async function fetchNotifications() {
  try {
    var data = await fetchJSON('/api/events?limit=20');
    var events = (data.items || []).filter(function(e) { return e.type === 'Warning'; });
    // Also check for not-ready nodes
    var nodesData = await fetchJSON('/api/nodes').catch(function() { return { items: [] }; });
    var notReadyNodes = (nodesData.items || []).filter(function(n) { return n.status !== 'Ready'; });

    var alerts = [];
    for (var i = 0; i < notReadyNodes.length; i++) {
      alerts.push({ severity: 'critical', text: 'Node ' + notReadyNodes[i].name + ' is NotReady', time: new Date().toISOString() });
    }
    for (var i = 0; i < Math.min(events.length, 15); i++) {
      alerts.push({ severity: 'warning', text: events[i].reason + ': ' + events[i].message, object: events[i].object, ns: events[i].namespace, time: events[i].lastTime });
    }

    renderNotifBadge(alerts.length);
    renderNotifList(alerts);
  } catch(e) { /* silent */ }
}

export function renderNotifBadge(count) {
  var badge = document.getElementById('notifBadge');
  if (!badge) return;
  if (count > 0) {
    badge.textContent = count > 99 ? '99+' : count;
    badge.style.display = 'inline';
    badge.classList.add('notif-pulse');
  } else {
    badge.style.display = 'none';
    badge.classList.remove('notif-pulse');
  }
}

export function renderNotifList(alerts) {
  var list = document.getElementById('notifList');
  if (!list) return;
  if (alerts.length === 0) {
    list.innerHTML = '<div class="notif-empty">All clear. No active alerts.</div>';
    return;
  }
  list.innerHTML = alerts.map(function(a) {
    var sevClass = a.severity === 'critical' ? 'notif-critical' : 'notif-warn';
    var icon = a.severity === 'critical' ? '\u25CF' : '\u26A0';
    return '<div class="notif-item ' + sevClass + '">' +
      '<span class="notif-item-icon">' + icon + '</span>' +
      '<div class="notif-item-body">' +
        '<div class="notif-item-text">' + escapeHtml(truncateText(a.text, 120)) + '</div>' +
        (a.ns ? '<div class="notif-item-meta">' + escapeHtml(a.ns) + (a.object ? ' / ' + escapeHtml(a.object) : '') + '</div>' : '') +
        '<div class="notif-item-time">' + timeAgo(a.time) + '</div>' +
      '</div>' +
    '</div>';
  }).join('');
}

export function toggleNotifPanel() {
  var panel = document.getElementById('notifPanel');
  if (!panel) return;
  panel.style.display = panel.style.display === 'none' ? 'block' : 'none';
}

// Close notif panel when clicking outside
document.addEventListener('click', function(e) {
  var bell = document.getElementById('notifBell');
  var panel = document.getElementById('notifPanel');
  if (!bell || !panel) return;
  if (!bell.contains(e.target)) {
    panel.style.display = 'none';
  }
});

// Init on page load
document.addEventListener('DOMContentLoaded', function() {
  setTimeout(function() { window.initNotificationCenter(); }, 2000);
});
