// modules/utils.js — Shared utility functions for k8ops dashboard
// Single source of truth for common helpers used across all pages.

// API base path (empty string = same origin)
export const API = '';

/**
 * Escape HTML special characters to prevent XSS.
 * Must be used when inserting any server-side or user-provided data into innerHTML.
 */
export function escapeHtml(s) {
  if (s == null) return '';
  return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

/**
 * Fetch JSON with auth redirect and error normalization.
 * - 401 → redirect to login
 * - 403 → throw with FORBIDDEN prefix
 * - other errors → throw with detail message
 */
export async function fetchJSON(url, opts) {
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

/** Check if an error is a 403 Forbidden. */
export function isForbidden(err) {
  return err && (err.status === 403 || (err.message && err.message.startsWith('FORBIDDEN')));
}

/** Render a forbidden access message into a container. */
export function renderForbidden(container) {
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

/** Render a status badge span. */
export function badge(text) {
  return `<span class="badge ${text}">${text}</span>`;
}

/** Human-readable relative time (e.g. "3m ago", "2h ago"). */
export function timeAgo(iso) {
  if (!iso) return '-';
  const diff = Date.now() - new Date(iso).getTime();
  const m = Math.floor(diff / 60000);
  if (m < 1) return 'just now';
  if (m < 60) return m + 'm ago';
  const h = Math.floor(m / 60);
  if (h < 24) return h + 'h ago';
  return Math.floor(h / 24) + 'd ago';
}

/** Truncate text to max length with ellipsis. */
export function truncateText(s, max) {
  if (!s) return '';
  return s.length > max ? s.substring(0, max - 1) + '\u2026' : s;
}

/** Color for a resource utilization percentage (0-100). */
export function barColor(pct) {
  if (pct > 80) return '#f85149';
  if (pct > 60) return '#d29922';
  return '#3fb950';
}

/** Color for a Pod phase. */
export function podPhaseColor(phase) {
  switch((phase||'').toLowerCase()) {
    case 'running': return '#3fb950';
    case 'pending': return '#d29922';
    case 'failed': return '#f85149';
    case 'succeeded': return '#8b949e';
    default: return '#8b949e';
  }
}
