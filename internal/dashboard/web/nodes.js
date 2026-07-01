// --- Nodes ---
import { escapeHtml, fetchJSON, badge, timeAgo } from './modules/utils.js';

export async function loadNodes(forceRefresh) {
  const container = document.getElementById('nodesTable');
  try {
    const data = await fetchJSON('/api/nodes' + (forceRefresh ? '?refresh=true' : ''));
    if (!data.items?.length) { container.innerHTML = '<div class="empty">No nodes found</div>'; return; }
    container.innerHTML = `<table>
      <thead><tr><th>Name</th><th>Status</th><th>Role</th><th>Version</th><th>CPU</th><th>Memory</th><th>OS/Arch</th><th>Conditions</th><th></th></tr></thead>
      <tbody>${data.items.map(n => `<tr>
        <td style="cursor:pointer;color:#58a6ff;" onclick="viewNodePods('${escapeHtml(n.name)}')">${escapeHtml(n.name)}</td>
        <td>${badge(n.status)}</td>
        <td>${escapeHtml(n.role)}</td>
        <td><code>${escapeHtml(n.version)}</code></td>
        <td>${escapeHtml(n.cpu)}</td>
        <td>${escapeHtml(n.memory)}</td>
        <td>${escapeHtml(n.os)}/${escapeHtml(n.arch)}</td>
        <td style="font-size:12px;color:#8b949e;">${Object.entries(n.conditions).map(([k,v])=>escapeHtml(k)+':'+escapeHtml(v)).join(', ')}</td>
        <td><button onclick="viewNodePods('${escapeHtml(n.name)}')" class="btn-secondary" style="font-size:12px;padding:4px 10px;">Pods &rarr;</button></td>
      </tr>`).join('')}</tbody>
    </table>`;
  } catch(e) { container.innerHTML = `<div class="empty">Error: ${escapeHtml(e.message)}</div>`; }
}

// --- Events ---
export async function loadEvents() {
  const container = document.getElementById('eventsTable');
  try {
    const params = new URLSearchParams();
    const ns = getCurrentNamespace();
    if (ns) params.set('namespace', ns);
    if (document.getElementById('warningOnly')?.checked) params.set('warning', 'true');
    const data = await fetchJSON('/api/events?' + params.toString());
    if (!data.items?.length) { container.innerHTML = '<div class="empty">No events found</div>'; return; }
    container.innerHTML = `<table>
      <thead><tr><th>Type</th><th>Reason</th><th>Object</th><th>Namespace</th><th>Message</th><th>Count</th><th>Last Seen</th></tr></thead>
      <tbody>${data.items.map(e => `<tr>
        <td>${badge(e.type)}</td>
        <td><strong>${escapeHtml(e.reason)}</strong></td>
        <td>${escapeHtml(e.object)}</td>
        <td>${escapeHtml(e.namespace)}</td>
        <td style="max-width:400px;color:#8b949e;">${escapeHtml(e.message)}</td>
        <td>${e.count}</td>
        <td>${timeAgo(e.lastTime)}</td>
      </tr>`).join('')}</tbody>
    </table>`;
  } catch(err) { container.innerHTML = `<div class="empty">Error: ${escapeHtml(err.message)}</div>`; }

  // Start live SSE feed if enabled
  toggleLiveEvents();
}

// --- Live Events SSE ---
let liveEventSource = null;

export function toggleLiveEvents() {
  const enabled = document.getElementById('liveEvents')?.checked;
  if (enabled) {
    startLiveEvents();
  } else {
    stopLiveEvents();
  }
}

export function startLiveEvents() {
  stopLiveEvents();
  const feed = document.getElementById('liveEventsFeed');
  if (!feed) return;
  feed.style.display = 'block';

  const params = new URLSearchParams();
  const ns = getCurrentNamespace();
  if (ns) params.set('namespace', ns);
  if (document.getElementById('warningOnly')?.checked) params.set('warning', 'true');

  liveEventSource = new EventSource('/api/events/stream?' + params.toString());
  liveEventSource.onmessage = function(e) {
    try {
      const d = JSON.parse(e.data);
      if (d.error) return;
      prependLiveEvent(d);
    } catch(err) {}
  };
  liveEventSource.addEventListener('reconnect', function() {
    // Server will close; browser will auto-reconnect via EventSource
  });
  liveEventSource.onerror = function() {
    // EventSource auto-reconnects; just keep going
  };
}

export function stopLiveEvents() {
  if (liveEventSource) {
    liveEventSource.close();
    liveEventSource = null;
  }
}

function prependLiveEvent(d) {
  const feed = document.getElementById('liveEventsFeed');
  if (!feed) return;

  const typeClass = d.type === 'Warning' ? 'live-event-warning' : 'live-event-normal';
  const watchBadge = d.watchType === 'ADDED' ? '<span class="live-event-badge live-event-new">NEW</span>' :
                     d.watchType === 'DELETED' ? '<span class="live-event-badge live-event-del">DEL</span>' : '';

  const row = document.createElement('div');
  row.className = 'live-event-row ' + typeClass;
  row.innerHTML =
    '<span class="live-event-time">' + new Date().toLocaleTimeString() + '</span>' +
    watchBadge +
    '<span class="live-event-type type-' + (d.type||'').toLowerCase() + '">' + escapeHtml(d.type || '') + '</span>' +
    '<span class="live-event-reason">' + escapeHtml(d.reason || '') + '</span>' +
    '<span class="live-event-object">' + escapeHtml(d.object || '') + '</span>' +
    '<span class="live-event-ns">' + escapeHtml(d.namespace || '') + '</span>' +
    '<span class="live-event-msg">' + escapeHtml(d.message || '') + '</span>';

  feed.insertBefore(row, feed.firstChild);

  // Keep max 200 live entries
  while (feed.children.length > 200) {
    feed.removeChild(feed.lastChild);
  }
}

export function clearLiveEvents() {
  const feed = document.getElementById('liveEventsFeed');
  if (feed) feed.innerHTML = '';
}

