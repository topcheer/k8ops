// --- Cluster Topology Visualization ---
// Draws namespace-grouped topology with health status colors.
import { escapeHtml, truncateText, podPhaseColor, barColor } from './modules/utils.js';

export async function loadTopology() {
  const container = document.getElementById('topologyContainer');
  if (!container) return;

  try {
    container.innerHTML = '<div class="loading">Loading topology...</div>';

    const [nodesRes, podsRes] = await Promise.all([
      fetch('/api/nodes').then(r => r.json()).catch(() => ({ items: [] })),
      fetch('/api/pods?all=1').then(r => r.json()).catch(() => ({ items: [] }))
    ]);

    const nodes = nodesRes.items || [];
    let pods = podsRes.items || podsRes || [];

    // Handle different response shapes
    if (!Array.isArray(pods) && pods.items) pods = pods.items;
    if (!Array.isArray(pods)) pods = [];

    if (pods.length === 0 && nodes.length === 0) {
      container.innerHTML = '<div style="text-align:center;padding:60px;color:var(--text-muted);">No resources found</div>';
      return;
    }

    // Group pods by namespace
    const nsGroups = {};
    for (const p of pods) {
      const ns = p.namespace || '<unknown>';
      if (!nsGroups[ns]) nsGroups[ns] = [];
      nsGroups[ns].push(p);
    }

    // Sort namespaces: system namespaces last, by pod count desc
    const sortedNs = Object.keys(nsGroups).sort((a, b) => {
      const aSys = a.startsWith('kube-') || a === 'default' || a.includes('cattle') || a.includes('tigera');
      const bSys = b.startsWith('kube-') || b === 'default' || b.includes('cattle') || b.includes('tigera');
      if (aSys && !bSys) return 1;
      if (!aSys && bSys) return -1;
      return nsGroups[b].length - nsGroups[a].length;
    });

    // Calculate cluster stats
    const totalPods = pods.length;
    const runningPods = pods.filter(p => (p.phase || '').toLowerCase() === 'running').length;
    const failedPods = pods.filter(p => {
      const ph = (p.phase || '').toLowerCase();
      return ph === 'failed' || ph === 'unknown';
    }).length;
    const readyNodes = nodes.filter(n => n.ready !== false && !n.conditions?.Ready?.includes('False')).length;

    // Build HTML
    let html = '';

    // Summary bar
    html += '<div class="topo-summary-bar">';
    html += '<div class="topo-stat"><span class="topo-stat-val" style="color:var(--accent-green);">' + readyNodes + '</span><span class="topo-stat-label">Nodes Ready</span></div>';
    html += '<div class="topo-stat"><span class="topo-stat-val" style="color:var(--accent-blue);">' + totalPods + '</span><span class="topo-stat-label">Total Pods</span></div>';
    html += '<div class="topo-stat"><span class="topo-stat-val" style="color:var(--accent-green);">' + runningPods + '</span><span class="topo-stat-label">Running</span></div>';
    html += '<div class="topo-stat"><span class="topo-stat-val" style="color:' + (failedPods > 0 ? 'var(--accent-red)' : 'var(--text-muted)') + ';">' + failedPods + '</span><span class="topo-stat-label">Failed</span></div>';
    html += '<div class="topo-stat"><span class="topo-stat-val" style="color:var(--accent-purple);">' + sortedNs.length + '</span><span class="topo-stat-label">Namespaces</span></div>';
    html += '</div>';

    // Namespace cards grid
    html += '<div class="topo-ns-grid">';
    for (const ns of sortedNs) {
      const nsPods = nsGroups[ns];
      const nsRunning = nsPods.filter(p => (p.phase || '').toLowerCase() === 'running').length;
      const nsFailed = nsPods.filter(p => {
        const ph = (p.phase || '').toLowerCase();
        return ph === 'failed' || ph === 'unknown';
      }).length;
      const isSystem = ns.startsWith('kube-') || ns.includes('cattle') || ns.includes('tigera');

      // Namespace health
      let nsHealth = 'healthy';
      if (nsFailed > 0) nsHealth = 'critical';
      else if (nsRunning < nsPods.length) nsHealth = 'warning';

      html += '<div class="topo-ns-card topo-' + nsHealth + (isSystem ? ' topo-system' : '') + '">';
      html += '<div class="topo-ns-header">';
      html += '<span class="topo-ns-name">' + escapeHtml(ns) + '</span>';
      html += '<span class="topo-ns-badge">' + nsPods.length + ' pods</span>';
      html += '</div>';
      html += '<div class="topo-ns-stats">';
      html += '<span style="color:var(--accent-green);">' + nsRunning + ' running</span>';
      if (nsFailed > 0) html += '<span style="color:var(--accent-red);">' + nsFailed + ' failed</span>';
      if (nsPods.length - nsRunning - nsFailed > 0) {
        html += '<span style="color:var(--accent-yellow);">' + (nsPods.length - nsRunning - nsFailed) + ' other</span>';
      }
      html += '</div>';
      html += '<div class="topo-pod-list">';

      // Show up to 12 pods, sorted by status (failed first)
      const sortedPods = nsPods.sort((a, b) => {
        const aFail = (a.phase || '').toLowerCase() !== 'running' ? 0 : 1;
        const bFail = (b.phase || '').toLowerCase() !== 'running' ? 0 : 1;
        return aFail - bFail;
      });

      const maxShow = 12;
      for (let i = 0; i < Math.min(sortedPods.length, maxShow); i++) {
        const p = sortedPods[i];
        const color = podPhaseColor(p.phase);
        const isCrash = (p.restartCount || 0) > 5;
        html += '<div class="topo-pod-item' + (isCrash ? ' topo-pod-crash' : '') + '" title="' +
          escapeHtml(p.name) + ' | phase=' + (p.phase || '?') + ' | restarts=' + (p.restartCount || 0) + ' | node=' + escapeHtml(p.nodeName || '?') + '">';
        html += '<span class="topo-pod-dot" style="background:' + color + ';"></span>';
        html += '<span class="topo-pod-name">' + escapeHtml(truncateText(p.name.replace(ns + '-', ''), 28)) + '</span>';
        if (isCrash) {
          html += '<span class="topo-pod-warn" title="High restart count: ' + p.restartCount + '">!' + (p.restartCount || '') + '</span>';
        }
        html += '</div>';
      }
      if (sortedPods.length > maxShow) {
        html += '<div class="topo-pod-more">+' + (sortedPods.length - maxShow) + ' more...</div>';
      }

      html += '</div>'; // pod-list
      html += '</div>'; // ns-card
    }
    html += '</div>'; // grid

    // Node resource summary at bottom
    if (nodes.length > 0) {
      html += '<div class="topo-nodes-section">';
      html += '<div class="section-title">Node Resources</div>';
      html += '<div class="topo-nodes-grid">';
      for (const n of nodes) {
        const cpuPct = n.cpuRequestedPct || n.cpuPercent || 0;
        const memPct = n.memoryRequestedPct || n.memoryPercent || 0;
        const isReady = n.ready !== false;
        html += '<div class="topo-node-card' + (isReady ? '' : ' topo-node-down') + '">';
        html += '<div class="topo-node-name">' + escapeHtml(n.name) + '</div>';
        html += '<div class="topo-node-bars">';
        html += '<div class="topo-bar-row"><span class="topo-bar-label">CPU</span><div class="topo-bar-track"><div class="topo-bar-fill" style="width:' + cpuPct + '%;background:' + barColor(cpuPct) + ';"></div></div><span class="topo-bar-val">' + Math.round(cpuPct) + '%</span></div>';
        html += '<div class="topo-bar-row"><span class="topo-bar-label">MEM</span><div class="topo-bar-track"><div class="topo-bar-fill" style="width:' + memPct + '%;background:' + barColor(memPct) + ';"></div></div><span class="topo-bar-val">' + Math.round(memPct) + '%</span></div>';
        html += '</div>';
        html += '</div>';
      }
      html += '</div>';
      html += '</div>';
    }

    container.innerHTML = html;

    // Init notification center if available
    var notReadyNodes = nodes.filter(n => n.ready === false);
    var alerts = [];
    for (var i = 0; i < notReadyNodes.length; i++) {
      alerts.push({ severity: 'critical', text: 'Node ' + notReadyNodes[i].name + ' is NotReady', time: new Date().toISOString() });
    }
    if (failedPods > 0) {
      alerts.push({ severity: 'warning', text: failedPods + ' pods are in Failed/Unknown state', time: new Date().toISOString() });
    }
    if (window.initNotificationCenter) {
      try { window.initNotificationCenter(alerts); } catch(_) {}
    }
  } catch(e) {
    container.innerHTML = '<div style="text-align:center;padding:40px;color:var(--accent-red);">Failed to load topology: ' + escapeHtml(e.message) + '</div>';
  }
}
