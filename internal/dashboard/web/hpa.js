// hpa.js — HPA (Horizontal Pod Autoscaler) Visualization page

import { escapeHtml, fetchJSON } from './modules/utils.js';

export async function loadHPA() {
  const container = document.getElementById('hpaContent');
  if (container) container.innerHTML = '<div class="loading">Loading HPA data...</div>';

  try {
    const data = await fetchJSON('/api/hpa');
    renderHPA(data);
  } catch (err) {
    if (container) {
      container.innerHTML = '<div class="empty-state">Failed to load: ' + escapeHtml(err.message) + '</div>';
    }
  }
}

function replicaBar(current, desired, min, max) {
  const range = max - min;
  if (range <= 0) return '';
  const currentPct = ((current - min) / range) * 100;
  const desiredPct = ((desired - min) / range) * 100;
  const isScaling = current !== desired;
  const color = isScaling ? '#d29922' : '#3fb950';

  return `<div style="display:flex;align-items:center;gap:6px;">
    <span style="font-family:monospace;font-size:12px;min-width:60px;">${current}/${max}</span>
    <div style="width:100px;height:8px;background:rgba(139,148,158,0.2);border-radius:4px;overflow:hidden;position:relative;">
      <div style="width:${Math.min(currentPct, 100)}%;height:100%;background:${color};border-radius:4px;"></div>
      ${isScaling ? `<div style="position:absolute;top:0;left:${Math.min(desiredPct, 100)}%;width:2px;height:100%;background:#58a6ff;"></div>` : ''}
    </div>
    ${isScaling ? `<span style="font-size:11px;color:#58a6ff;">&rarr; ${desired}</span>` : ''}
  </div>`;
}

function metricBar(util) {
  const color = util > 100 ? '#f85149' : util > 80 ? '#d29922' : '#3fb950';
  return `<div style="display:flex;align-items:center;gap:6px;">
    <div style="width:60px;height:6px;background:rgba(139,148,158,0.2);border-radius:3px;overflow:hidden;">
      <div style="width:${Math.min(util, 150) / 1.5}%;height:100%;background:${color};border-radius:3px;"></div>
    </div>
    <span style="font-size:11px;color:${color};min-width:40px;">${util.toFixed(0)}%</span>
  </div>`;
}

function renderHPA(data) {
  const container = document.getElementById('hpaContent');
  if (!container) return;

  const items = data.items || [];
  const summary = data.summary || {};

  if (items.length === 0) {
    container.innerHTML = `
      <div style="text-align:center;padding:60px;color:#8b949e;">
        <div style="font-size:48px;margin-bottom:12px;">&#9881;</div>
        <div style="font-size:16px;">No HorizontalPodAutoscalers found</div>
        <div style="font-size:13px;margin-top:8px;">Create an HPA to see autoscaling visualization here.</div>
        <pre style="text-align:left;margin:20px auto;max-width:500px;font-size:12px;background:var(--bg-primary);padding:12px;border-radius:8px;border:1px solid var(--border-color);">
kubectl autoscale deployment nginx \
  --min=2 --max=10 --cpu-percent=70</pre>
      </div>`;
    return;
  }

  // Summary cards
  const summaryHtml = `
    <div class="stat-card" style="min-width:100px;">
      <div class="stat-value">${summary.totalHPAs || 0}</div>
      <div class="stat-label">HPAs</div>
    </div>
    <div class="stat-card" style="min-width:110px;border-left:3px solid #d29922;">
      <div class="stat-value" style="color:#d29922;">${summary.scalingActive || 0}</div>
      <div class="stat-label">Scaling</div>
    </div>
    <div class="stat-card" style="min-width:120px;">
      <div class="stat-value">${summary.currentReplicas || 0}</div>
      <div class="stat-label">Current Replicas</div>
    </div>
    <div class="stat-card" style="min-width:120px;border-left:3px solid #58a6ff;">
      <div class="stat-value" style="color:#58a6ff;">${summary.desiredReplicas || 0}</div>
      <div class="stat-label">Desired Replicas</div>
    </div>`;

  // HPA cards
  const cards = items.map(h => {
    const isScaling = h.scalingActive;
    const cardBorder = isScaling ? '#d29922' : '#30363d';
    const metricsHtml = (h.metrics || []).map(m => {
      return `<div style="display:flex;align-items:center;gap:8px;margin-bottom:6px;font-size:12px;">
        <span style="color:#8b949e;text-transform:uppercase;font-weight:600;min-width:50px;">${escapeHtml(m.type)}</span>
        <span style="min-width:80px;">${escapeHtml(m.name)}</span>
        <span style="color:#8b949e;">target: ${escapeHtml(m.targetValue)}</span>
        ${m.currentValue ? `<span style="color:${m.utilizationPct > 100 ? '#f85149' : '#3fb950'};">now: ${escapeHtml(m.currentValue)}</span>` : '<span style="color:#8b949e;">now: -</span>'}
        ${m.utilizationPct > 0 ? metricBar(m.utilizationPct) : ''}
      </div>`;
    }).join('');

    return `<div style="border:1px solid ${cardBorder};border-radius:8px;padding:14px;margin-bottom:10px;background:var(--bg-secondary);">
      <div style="display:flex;justify-content:space-between;align-items:start;margin-bottom:10px;">
        <div>
          <span style="font-weight:600;color:#58a6ff;">${escapeHtml(h.name)}</span>
          <span style="color:#8b949e;font-size:12px;margin-left:8px;">${escapeHtml(h.namespace)}</span>
          ${isScaling ? '<span style="background:rgba(210,153,34,0.15);color:#d29922;padding:1px 6px;border-radius:4px;font-size:10px;margin-left:8px;">SCALING</span>' : ''}
          <div style="font-size:12px;color:#8b949e;margin-top:4px;">
            &rarr; ${escapeHtml(h.targetKind)}/${escapeHtml(h.targetName)} &middot; age: ${escapeHtml(h.age)}
          </div>
        </div>
      </div>
      <div style="margin-bottom:8px;">
        ${replicaBar(h.currentReplicas, h.desiredReplicas, h.minReplicas, h.maxReplicas)}
      </div>
      ${metricsHtml || '<div style="color:#8b949e;font-size:12px;">No metrics configured</div>'}
    </div>`;
  }).join('');

  container.innerHTML = `${summaryHtml}
    <div style="display:flex;gap:12px;flex-wrap:wrap;margin-bottom:16px;">${summaryHtml}</div>
    <div style="display:flex;gap:12px;align-items:center;margin-bottom:16px;">
      <input type="text" id="hpaSearch" class="search-input" placeholder="Filter HPAs..." oninput="filterHPA()" style="flex:1;">
      <button onclick="loadHPA()" class="btn-secondary">Refresh</button>
    </div>
    <div id="hpaCardList">${cards}</div>`;
}

export function filterHPA() {
  const search = (document.getElementById('hpaSearch')?.value || '').toLowerCase();
  const cards = document.querySelectorAll('#hpaCardList > div');
  cards.forEach(card => {
    const text = card.textContent.toLowerCase();
    card.style.display = text.includes(search) ? '' : 'none';
  });
}
