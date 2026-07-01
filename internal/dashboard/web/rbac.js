// --- User Management ---
import { escapeHtml, fetchJSON } from './modules/utils.js';

export async function loadUsers() {
  const container = document.getElementById('userList');
  if (!container) return;
  try {
    const data = await fetchJSON('/api/admin/users');
    const users = data.users || data;
    if (!Array.isArray(users) || !users.length) {
      container.innerHTML = '<div class="empty">No users found</div>';
      return;
    }
    container.innerHTML = `<table class="data-table" style="width:100%;border-collapse:collapse;">
      <thead><tr style="border-bottom:1px solid #30363d;">
        <th style="text-align:left;padding:8px;">Username</th>
        <th style="text-align:left;padding:8px;">Auth Source</th>
        <th style="text-align:left;padding:8px;">Display Name</th>
        <th style="text-align:left;padding:8px;">Role</th>
        <th style="text-align:left;padding:8px;">Namespaces</th>
        <th style="text-align:left;padding:8px;">Actions</th>
      </tr></thead>
      <tbody>
        ${users.map(u => `<tr style="border-bottom:1px solid #21262d;">
          <td style="padding:8px;font-weight:600;">${u.username}</td>
          <td style="padding:8px;">${sourceBadge(u.provider)}</td>
          <td style="padding:8px;color:#8b949e;">${u.display_name || '-'}</td>
          <td style="padding:8px;">${roleBadge(u.role)}</td>
          <td style="padding:8px;font-size:13px;color:#8b949e;">${u.allowed_namespaces || (u.role.startsWith('ns-') ? '<span style="color:#f85149;">not set</span>' : '-')}</td>
          <td style="padding:8px;white-space:nowrap;">
            ${u.role === 'admin' && u.username === 'admin' ? '<span style="color:#8b949e;font-size:12px;">protected</span>' : `
              <button onclick="editUserRole(${u.id}, '${u.username}', '${u.role}', '${u.allowed_namespaces || ''}')" class="btn-secondary" style="padding:4px 10px;font-size:12px;">Edit Role</button>
              <button onclick="deleteUser(${u.id}, '${u.username}')" class="btn-secondary" style="padding:4px 10px;font-size:12px;color:#f85149;">Delete</button>
            `}
          </td>
        </tr>`).join('')}
      </tbody>
    </table>`;
  } catch(e) {
    container.innerHTML = `<div class="error">${escapeHtml(e.message)}</div>`;
  }
}

export function sourceBadge(provider) {
  const styles = {
    'local':  '<span style="padding:2px 10px;border-radius:12px;font-size:12px;background:#1f6feb1a;color:#58a6ff;border:1px solid #1f6feb33;">Local</span>',
    'ldap':   '<span style="padding:2px 10px;border-radius:12px;font-size:12px;background:#2386361a;color:#3fb950;border:1px solid #23863633;">LDAP</span>',
    'oidc':   '<span style="padding:2px 10px;border-radius:12px;font-size:12px;background:#a371f71a;color:#bc8cff;border:1px solid #a371f733;">OIDC/SSO</span>',
  };
  return styles[provider] || `<span style="padding:2px 10px;border-radius:12px;font-size:12px;background:#30363d;color:#8b949e;">${provider}</span>`;
}

export function roleBadge(role) {
  const colors = { 'admin': '#f85149', 'operator': '#d29922', 'viewer': '#58a6ff', 'ns-admin': '#f0883e', 'ns-viewer': '#3fb950' };
  const c = colors[role] || '#8b949e';
  return `<span style="color:${c};font-weight:600;">${role}</span>`;
}

// showCreateUserForm now defined below with dynamic role loading
export function hideCreateUserForm() { document.getElementById('userCreateForm').style.display = 'none'; }

let _userRoleCache = null;
export async function loadUserRoles() {
  if (!_userRoleCache) {
    try {
      const data = await fetchJSON('/api/rbac/role-defs');
      _userRoleCache = data.items || [];
    } catch(e) {
      _userRoleCache = [];
    }
  }
  return _userRoleCache;
}

export async function showCreateUserForm() {
  document.getElementById('userCreateForm').style.display = 'block';
  // Populate role dropdown dynamically
  const select = document.getElementById('newRole');
  const roles = await loadUserRoles();
  select.innerHTML = roles.map(r => {
    const scopeTag = r.scope === 'namespace' ? ' [NS]' : '';
    const builtinTag = r.builtin ? '' : ' (custom)';
    const desc = r.description || r.displayName || '';
    return `<option value="${r.name}">${r.name}${scopeTag}${builtinTag}${desc ? ' — ' + desc : ''}</option>`;
  }).join('');
}

export function toggleNsField() {
  const role = document.getElementById('newRole').value;
  // Show namespace field for ns-* roles or namespace-scoped custom roles
  const show = role.startsWith('ns-') || role.includes('-ns');
  document.getElementById('newNsRow').style.display = show ? 'block' : 'none';
}

export async function createUser() {
  const username = document.getElementById('newUsername').value.trim();
  const password = document.getElementById('newPassword').value;
  const role = document.getElementById('newRole').value;
  if (!username || !password) { alert('Username and password are required'); return; }
  const ns = role.startsWith('ns-') ? document.getElementById('newNamespaces').value.trim() : '';
  try {
    const res = await fetch('/api/admin/users', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        username, password,
        email: document.getElementById('newEmail').value.trim(),
        display_name: document.getElementById('newDisplayName').value.trim(),
        role, allowed_namespaces: ns,
      })
    });
    if (!res.ok) { const e = await res.json(); throw new Error(e.error || 'HTTP ' + res.status); }
    hideCreateUserForm();
    document.getElementById('newUsername').value = '';
    document.getElementById('newPassword').value = '';
    document.getElementById('newEmail').value = '';
    document.getElementById('newDisplayName').value = '';
    loadUsers();
  } catch(e) { alert('Failed to create user: ' + e.message); }
}

export function editUserRole(id, username, currentRole, currentNs) {
  const role = prompt(`Role for ${username}:\n(admin, operator, viewer, ns-admin, ns-viewer)`, currentRole);
  if (!role || role === currentRole) return;
  let ns = currentNs;
  if (role.startsWith('ns-')) {
    ns = prompt(`Allowed namespaces for ${username} (comma-separated):`, currentNs || 'default');
    if (ns === null) return;
  }
  fetch('/api/admin/users/' + id, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ role, allowed_namespaces: ns }),
  }).then(r => r.json()).then(() => loadUsers()).catch(e => alert('Update failed: ' + e.message));
}

export async function deleteUser(id, username) {
  if (!confirm(`Delete user "${username}"? This also removes their namespace RoleBindings.`)) return;
  try {
    await fetch('/api/admin/users/' + id, { method: 'DELETE' });
    loadUsers();
  } catch(e) { alert('Delete failed: ' + e.message); }
}

// --- Auth Provider Configuration ---


export async function loadAuthConfig() {
  try {
    const cfg = await fetchJSON('/api/admin/auth-config');
    const drSelect = document.getElementById('defaultRole');
    if (drSelect) {
      const roles = await loadUserRoles();
      drSelect.innerHTML = roles.map(r => {
        const sel = r.name === cfg.default_role ? 'selected' : '';
        return `<option value="${r.name}" ${sel}>${r.name}${r.description ? " — " + r.description : ""}</option>`;
      }).join('');
    }
    const drNs = document.getElementById('defaultAllowedNs');
    if (drNs) drNs.value = cfg.default_allowed_namespaces || '';
  } catch(e) { console.error('loadAuthConfig:', e); }
}

export async function saveAuthConfig() {
  const body = {
    default_role: document.getElementById('defaultRole')?.value || '',
    default_allowed_namespaces: document.getElementById('defaultAllowedNs')?.value || '',
  };
  try {
    const res = await fetch('/api/admin/auth-config', {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(body),
    });
    if (!res.ok) { const e = await res.json(); throw new Error(e.error || 'HTTP ' + res.status); }
    loadAuthConfig();
  } catch(e) { alert('Failed: ' + e.message); }
}
// --- Shared data cache ---

let _nsCache = null;
let _crCache = null;
let _subjectsCache = null;

export async function getNamespaces() {
  if (!_nsCache) {
    const d = await fetchJSON('/api/rbac/namespaces');
    _nsCache = d.items || [];
  }
  return _nsCache;
}

export async function getClusterRolesForDropdown() {
  if (!_crCache) {
    const d = await fetchJSON('/api/rbac/clusterroles');
    _crCache = (d.items || []).map(r => r.name);
  }
  return _crCache;
}

export async function getSubjects() {
  if (!_subjectsCache) {
    const d = await fetchJSON('/api/rbac/subjects');
    _subjectsCache = d.items || [];
  }
  return _subjectsCache;
}

// --- Role Mapping (k8ops role -> multiple K8s ClusterRoles/Roles) ---

let _roleMappingData = null;

export async function loadRoleMappings() {
  const container = document.getElementById('roleMappingList');
  if (!container) return;
  try {
    const data = await fetchJSON('/api/rbac/role-mapping');
    _roleMappingData = data;
    const items = data.items || [];
    const available = data.availableRoles || [];
    const scopeLabel = { 'cluster': '集群级', 'namespace': '命名空间级' };
    const scopeColor = { 'cluster': '#f85149', 'namespace': '#3fb950' };

    let html = '';

    for (const m of items) {
      const builtinTag = m.builtin
        ? '<span style="background:#2386361a;color:#3fb950;padding:2px 8px;border-radius:8px;font-size:11px;border:1px solid #23863633;">内置</span>'
        : '<span style="background:#a371f71a;color:#bc8cff;padding:2px 8px;border-radius:8px;font-size:11px;border:1px solid #a371f733;">自定义</span>';

      // Bindings list
      const bindingRows = (m.bindings || []).map(b => {
        const nsTag = b.namespace ? `<span style="color:#58a6ff;">@${b.namespace}</span>` : '';
        return `<div style="display:flex;align-items:center;gap:8px;padding:4px 0;">
          <span style="background:#21262d;border:1px solid #30363d;padding:2px 8px;border-radius:4px;font-size:12px;font-family:monospace;">${b.kind}</span>
          <span style="font-size:13px;color:#c9d1d9;">${b.name}</span>
          ${nsTag}
          <button onclick="removeRoleMapping('${m.name}','${b.kind}','${b.name}','${b.namespace || ''}')" class="btn-secondary" style="padding:2px 8px;font-size:11px;color:#f85149;margin-left:auto;">×</button>
        </div>`;
      }).join('') || '<span style="color:#8b949e;font-size:12px;font-style:italic;">无绑定</span>';

      // Add binding form (inline, per role)
      const addFormId = `addBinding-${m.name}`;

      html += `<div style="border:1px solid #30363d;border-radius:8px;margin-bottom:12px;overflow:hidden;">
        <div style="display:flex;align-items:center;gap:10px;padding:12px 16px;background:#161b22;">
          <span style="font-weight:700;font-size:14px;">${roleBadge(m.name)}</span>
          ${builtinTag}
          <span style="font-family:monospace;font-size:12px;color:#8b949e;">${m.group}</span>
          <span style="color:${scopeColor[m.scope]};font-size:12px;margin-left:auto;">${scopeLabel[m.scope]}</span>
          ${m.displayName ? `<span style="color:#8b949e;font-size:12px;">${m.displayName}</span>` : ''}
          ${!m.builtin ? `<button onclick="deleteCustomRole('${m.name}')" class="btn-secondary" style="padding:2px 8px;font-size:11px;color:#f85149;">删除角色</button>` : ''}
        </div>
        <div style="padding:12px 16px;">
          <div style="margin-bottom:8px;font-size:12px;color:#8b949e;font-weight:600;">K8s 绑定 (权限取并集):</div>
          <div style="margin-bottom:10px;">${bindingRows}</div>
          <div id="${addFormId}" style="display:flex;gap:6px;align-items:center;flex-wrap:wrap;padding-top:6px;border-top:1px solid #21262d;">
            <select id="${addFormId}-kind" class="form-input" style="width:auto;font-size:12px;" onchange="onAddBindingKindChange('${m.name}')">
              <option value="ClusterRole">ClusterRole</option>
              <option value="Role">Role</option>
            </select>
            <select id="${addFormId}-name" class="form-input" style="width:auto;font-size:12px;">
              ${available.map(r => `<option value="${r}">${r}</option>`).join('')}
            </select>
            <select id="${addFormId}-ns" class="form-input" style="width:auto;font-size:12px;display:none;">
              ${(_nsCache || []).map(ns => `<option value="${ns}">${ns}</option>`).join('')}
            </select>
            <button onclick="addRoleMapping('${m.name}')" class="btn-primary" style="padding:4px 12px;font-size:12px;">+ 添加绑定</button>
          </div>
        </div>
      </div>`;
    }

    // Custom role creation form
    html += `<div style="margin-top:16px;padding:16px;border:1px dashed #30363d;border-radius:8px;">
      <div style="font-weight:600;font-size:13px;margin-bottom:10px;color:#58a6ff;">+ 新建自定义角色</div>
      <div style="display:flex;gap:8px;flex-wrap:wrap;align-items:end;">
        <div><label style="font-size:12px;color:#8b949e;">角色名 *</label><br>
          <input type="text" id="newCustomRoleName" class="form-input" placeholder="devops" style="width:160px;"></div>
        <div><label style="font-size:12px;color:#8b949e;">显示名</label><br>
          <input type="text" id="newCustomRoleDisplay" class="form-input" placeholder="DevOps Engineer" style="width:200px;"></div>
        <div><label style="font-size:12px;color:#8b949e;">作用域</label><br>
          <select id="newCustomRoleScope" class="form-input" style="width:auto;">
            <option value="cluster">集群级</option>
            <option value="namespace">命名空间级</option>
          </select></div>
        <div><label style="font-size:12px;color:#8b949e;">描述</label><br>
          <input type="text" id="newCustomRoleDesc" class="form-input" placeholder="CI/CD 部署权限" style="width:200px;"></div>
        <button onclick="createCustomRole()" class="btn-primary" style="font-size:12px;">创建角色</button>
      </div>
      <div style="margin-top:6px;color:#8b949e;font-size:11px;">创建后，角色自动生成 impersonation group <code>k8ops:&lt;角色名&gt;</code>，然后在上方添加 K8s 绑定。</div>
    </div>`;

    container.innerHTML = html;
  } catch(e) {
    container.innerHTML = `<div class="error">${escapeHtml(e.message)}</div>`;
  }
}

export function onAddBindingKindChange(roleName) {
  const kindSel = document.getElementById(`addBinding-${roleName}-kind`);
  const nsSel = document.getElementById(`addBinding-${roleName}-ns`);
  if (kindSel.value === 'Role') {
    nsSel.style.display = '';
  } else {
    nsSel.style.display = 'none';
  }
}

export async function addRoleMapping(roleName) {
  const kind = document.getElementById(`addBinding-${roleName}-kind`).value;
  const name = document.getElementById(`addBinding-${roleName}-name`).value;
  const nsSel = document.getElementById(`addBinding-${roleName}-ns`);
  const ns = kind === 'Role' ? nsSel.value : '';

  try {
    const res = await fetch('/api/rbac/role-mapping', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ roleName, k8sRoleKind: kind, k8sRoleName: name, namespace: ns }),
    });
    if (!res.ok) { const e = await res.json(); throw new Error(e.error || 'HTTP ' + res.status); }
    loadRoleMappings();
  } catch(e) { alert('Failed: ' + e.message); }
}

export async function removeRoleMapping(roleName, kind, name, ns) {
  try {
    const params = new URLSearchParams({ role: roleName, kind, name });
    if (ns) params.set('namespace', ns);
    await fetch('/api/rbac/role-mapping?' + params, { method: 'DELETE' });
    loadRoleMappings();
  } catch(e) { alert('Failed: ' + e.message); }
}

export async function createCustomRole() {
  const name = document.getElementById('newCustomRoleName').value.trim();
  if (!name) { alert('角色名必填'); return; }
  const displayName = document.getElementById('newCustomRoleDisplay').value.trim();
  const scope = document.getElementById('newCustomRoleScope').value;
  const desc = document.getElementById('newCustomRoleDesc').value.trim();
  try {
    const res = await fetch('/api/rbac/role-defs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, displayName, scope, description: desc }),
    });
    if (!res.ok) { const e = await res.json(); throw new Error(e.error || 'HTTP ' + res.status); }
    document.getElementById('newCustomRoleName').value = '';
    document.getElementById('newCustomRoleDisplay').value = '';
    document.getElementById('newCustomRoleDesc').value = '';
    loadRoleMappings();
  } catch(e) { alert('Failed: ' + e.message); }
}

export async function deleteCustomRole(name) {
  if (!confirm(`删除自定义角色 "${name}"？所有绑定也会一并删除。`)) return;
  try {
    await fetch('/api/rbac/role-defs?name=' + name, { method: 'DELETE' });
    loadRoleMappings();
  } catch(e) { alert('Failed: ' + e.message); }
}

// --- ClusterRole Management ---

export async function loadClusterRoles() {
  const container = document.getElementById('clusterRoleList');
  if (!container) return;
  _crCache = null;
  try {
    const data = await fetchJSON('/api/rbac/clusterroles');
    const roles = data.items || [];
    container.innerHTML = `<table class="data-table" style="width:100%;border-collapse:collapse;">
      <thead><tr style="border-bottom:1px solid #30363d;">
        <th style="text-align:left;padding:8px;">Name</th>
        <th style="text-align:left;padding:8px;">Rules</th>
        <th style="text-align:left;padding:8px;">Bound Groups</th>
        <th style="text-align:left;padding:8px;">Actions</th>
      </tr></thead>
      <tbody>
        ${roles.map(r => `<tr style="border-bottom:1px solid #21262d;">
          <td style="padding:8px;font-weight:600;">${r.name}</td>
          <td style="padding:8px;font-size:12px;color:#8b949e;">${r.ruleCount} rule(s)</td>
          <td style="padding:8px;">${(r.bindings || []).map(b => `<span style="background:#21262d;border:1px solid #30363d;padding:2px 8px;border-radius:12px;font-size:12px;margin:2px;display:inline-block;">${b}</span>`).join('') || '<span style="color:#8b949e;">-</span>'}</td>
          <td style="padding:8px;white-space:nowrap;">
            <button onclick="editRoleRules('${r.name}')" class="btn-secondary" style="padding:4px 10px;font-size:12px;">Edit Rules</button>
            <button onclick="viewRoleYAML('${r.name}')" class="btn-secondary" style="padding:4px 10px;font-size:12px;">YAML</button>
            ${r.name.startsWith('k8ops-role-') ? '<span style="color:#8b949e;font-size:11px;margin-left:4px;">system</span>' : `<button onclick="deleteClusterRole('${r.name}')" class="btn-secondary" style="padding:4px 10px;font-size:12px;color:#f85149;">Delete</button>`}
          </td>
        </tr>`).join('')}
      </tbody>
    </table>`;
  } catch(e) {
    container.innerHTML = `<div class="error">${escapeHtml(e.message)}</div>`;
  }
}

export function showCreateRoleForm() { document.getElementById('roleCreateForm').style.display = 'block'; }
export function hideCreateRoleForm() { document.getElementById('roleCreateForm').style.display = 'none'; }

export async function createClusterRole() {
  const name = document.getElementById('newRoleName').value.trim();
  if (!name) { alert('Name is required'); return; }
  try {
    const res = await fetch('/api/rbac/clusterroles', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name }),
    });
    if (!res.ok) { const e = await res.json(); throw new Error(e.error || 'HTTP ' + res.status); }
    hideCreateRoleForm();
    document.getElementById('newRoleName').value = '';
    await loadClusterRoles();
    editRoleRules(name);
  } catch(e) { alert('Failed: ' + e.message); }
}

export async function deleteClusterRole(name) {
  if (!confirm(`Delete ClusterRole "${name}"?`)) return;
  try {
    await fetch('/api/rbac/clusterroles/' + name, { method: 'DELETE' });
    loadClusterRoles();
  } catch(e) { alert('Delete failed: ' + e.message); }
}

export async function viewRoleYAML(name) {
  try {
    const data = await fetchJSON('/api/rbac/clusterroles/' + name + '/yaml');
    const overlay = document.getElementById('yamlOverlay');
    document.getElementById('yamlTitle').textContent = 'ClusterRole: ' + name;
    document.getElementById('yamlOutput').textContent = data.yaml || '';
    overlay.classList.add('active');
  } catch(e) { alert('Failed: ' + e.message); }
}

// --- Namespace Role Management ---

export async function loadNsRoles() {
  const container = document.getElementById('nsRoleList');
  if (!container) return;
  try {
    const data = await fetchJSON('/api/rbac/roles');
    const roles = data.items || [];
    if (!roles.length) {
      container.innerHTML = '<div class="empty">No namespace roles found</div>';
      return;
    }
    container.innerHTML = `<table class="data-table" style="width:100%;border-collapse:collapse;">
      <thead><tr style="border-bottom:1px solid #30363d;">
        <th style="text-align:left;padding:8px;">Name</th>
        <th style="text-align:left;padding:8px;">Namespace</th>
        <th style="text-align:left;padding:8px;">Rules</th>
        <th style="text-align:left;padding:8px;">Actions</th>
      </tr></thead>
      <tbody>
        ${roles.map(r => `<tr style="border-bottom:1px solid #21262d;">
          <td style="padding:8px;font-weight:600;">${r.name}</td>
          <td style="padding:8px;"><span style="color:#58a6ff;">${r.namespace}</span></td>
          <td style="padding:8px;font-size:12px;color:#8b949e;">${r.ruleCount} rule(s)</td>
          <td style="padding:8px;white-space:nowrap;">
            <button onclick="editNsRoleRules('${r.namespace}','${r.name}')" class="btn-secondary" style="padding:4px 10px;font-size:12px;">Edit Rules</button>
            <button onclick="deleteNsRole('${r.namespace}','${r.name}')" class="btn-secondary" style="padding:4px 10px;font-size:12px;color:#f85149;">Delete</button>
          </td>
        </tr>`).join('')}
      </tbody>
    </table>`;
  } catch(e) {
    container.innerHTML = `<div class="error">${escapeHtml(e.message)}</div>`;
  }
}

export async function showCreateNsRoleForm() {
  document.getElementById('nsRoleCreateForm').style.display = 'block';
  const select = document.getElementById('newNsRoleNs');
  select.innerHTML = '<option value="">Select namespace...</option>';
  const namespaces = await getNamespaces();
  namespaces.forEach(ns => {
    select.innerHTML += `<option value="${escapeHtml(ns)}">${escapeHtml(ns)}</option>`;
  });
}
export function hideCreateNsRoleForm() { document.getElementById('nsRoleCreateForm').style.display = 'none'; }

export async function createNsRole() {
  const name = document.getElementById('newNsRoleName').value.trim();
  const ns = document.getElementById('newNsRoleNs').value;
  if (!name || !ns) { alert('Name and namespace are required'); return; }
  try {
    const res = await fetch('/api/rbac/roles', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, namespace: ns }),
    });
    if (!res.ok) { const e = await res.json(); throw new Error(e.error || 'HTTP ' + res.status); }
    hideCreateNsRoleForm();
    document.getElementById('newNsRoleName').value = '';
    await loadNsRoles();
    editNsRoleRules(ns, name);
  } catch(e) { alert('Failed: ' + e.message); }
}

export async function deleteNsRole(ns, name) {
  if (!confirm(`Delete Role "${name}" in "${ns}"?`)) return;
  try {
    await fetch(`/api/rbac/roles/${ns}/${name}`, { method: 'DELETE' });
    loadNsRoles();
  } catch(e) { alert('Delete failed: ' + e.message); }
}

// --- RoleBinding Management ---

export async function loadRoleBindings() {
  const container = document.getElementById('roleBindingList');
  if (!container) return;
  try {
    const data = await fetchJSON('/api/rbac/rolebindings');
    const items = data.items || [];
    if (!items.length) {
      container.innerHTML = '<div class="empty">No namespace RoleBindings found</div>';
      return;
    }
    container.innerHTML = `<table class="data-table" style="width:100%;border-collapse:collapse;">
      <thead><tr style="border-bottom:1px solid #30363d;">
        <th style="text-align:left;padding:8px;">Name</th>
        <th style="text-align:left;padding:8px;">Namespace</th>
        <th style="text-align:left;padding:8px;">Subject</th>
        <th style="text-align:left;padding:8px;">Role</th>
        <th style="text-align:left;padding:8px;">Actions</th>
      </tr></thead>
      <tbody>
        ${items.map(b => `<tr style="border-bottom:1px solid #21262d;">
          <td style="padding:8px;font-weight:600;">${b.name}</td>
          <td style="padding:8px;"><span style="color:#58a6ff;">${b.namespace}</span></td>
          <td style="padding:8px;font-size:12px;">${b.subjectKind}: ${b.subjectName}</td>
          <td style="padding:8px;">${b.roleKind}/${b.roleName}</td>
          <td style="padding:8px;">
            ${b.managed ? `<button onclick="deleteRoleBinding('${b.namespace}','${b.name}')" class="btn-secondary" style="padding:4px 10px;font-size:12px;color:#f85149;">Delete</button>` : '<span style="color:#8b949e;font-size:12px;">system</span>'}
          </td>
        </tr>`).join('')}
      </tbody>
    </table>`;
  } catch(e) {
    container.innerHTML = `<div class="error">${escapeHtml(e.message)}</div>`;
  }
}

export async function showCreateBindingForm() {
  document.getElementById('bindingCreateForm').style.display = 'block';
  const nsSelect = document.getElementById('newBindingNamespaces');
  nsSelect.innerHTML = '';
  const namespaces = await getNamespaces();
  namespaces.forEach(ns => {
    nsSelect.innerHTML += `<option value="${escapeHtml(ns)}">${escapeHtml(ns)}</option>`;
  });
  await updateSubjectOptions();
  await updateRoleNameOptions();
}

export function hideCreateBindingForm() { document.getElementById('bindingCreateForm').style.display = 'none'; }

export async function updateSubjectOptions() {
  const kind = document.getElementById('newBindingSubjectKind').value;
  const select = document.getElementById('newBindingSubjectName');
  select.innerHTML = '';
  const staticGroups = ['k8ops:admin', 'k8ops:operator', 'k8ops:viewer'];
  if (kind === 'Group') {
    staticGroups.forEach(g => {
      select.innerHTML += `<option value="${escapeHtml(g)}">${escapeHtml(g)}</option>`;
    });
  }
  const subjects = await getSubjects();
  subjects.filter(s => s.kind === kind).forEach(s => {
    if (!staticGroups.includes(s.name)) {
      select.innerHTML += `<option value="${escapeHtml(s.name)}">${escapeHtml(s.name)}</option>`;
    }
  });
}

export async function updateRoleNameOptions() {
  const kind = document.getElementById('newBindingRoleKind').value;
  const select = document.getElementById('newBindingRoleName');
  select.innerHTML = '';
  if (kind === 'ClusterRole') {
    const roles = await getClusterRolesForDropdown();
    roles.forEach(r => {
      select.innerHTML += `<option value="${escapeHtml(r)}">${escapeHtml(r)}</option>`;
    });
  } else {
    const nsSelect = document.getElementById('newBindingNamespaces');
    const selected = Array.from(nsSelect.selectedOptions).map(o => o.value);
    if (selected.length > 0) {
      const d = await fetchJSON('/api/rbac/roles?namespace=' + encodeURIComponent(selected[0]));
      (d.items || []).forEach(r => {
        select.innerHTML += `<option value="${escapeHtml(r.name)}">${escapeHtml(r.name)}</option>`;
      });
    } else {
      select.innerHTML = '<option value="">Select namespace first...</option>';
    }
  }
}

export async function createRoleBinding() {
  const name = document.getElementById('newBindingName').value.trim();
  const nsSelect = document.getElementById('newBindingNamespaces');
  const namespaces = Array.from(nsSelect.selectedOptions).map(o => o.value);
  const subjectKind = document.getElementById('newBindingSubjectKind').value;
  const subjectName = document.getElementById('newBindingSubjectName').value;
  const roleKind = document.getElementById('newBindingRoleKind').value;
  const roleName = document.getElementById('newBindingRoleName').value;

  if (!name || !namespaces.length || !subjectName || !roleName) {
    alert('Binding name, at least one namespace, subject, and role are required');
    return;
  }

  try {
    const res = await fetch('/api/rbac/rolebindings', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name, namespaces,
        subject_kind: subjectKind,
        subject_name: subjectName,
        role_kind: roleKind,
        role_name: roleName,
      }),
    });
    if (!res.ok) { const e = await res.json(); throw new Error(e.error || 'HTTP ' + res.status); }
    const data = await res.json();
    hideCreateBindingForm();
    document.getElementById('newBindingName').value = '';
    loadRoleBindings();
    if (data.errors && data.errors.length > 0) {
      alert('Created in: ' + data.created.join(', ') + '\nErrors: ' + data.errors.join('; '));
    }
  } catch(e) { alert('Failed: ' + e.message); }
}

export async function deleteRoleBinding(ns, name) {
  if (!confirm(`Delete RoleBinding "${name}" in "${ns}"?`)) return;
  try {
    await fetch(`/api/rbac/rolebindings/${ns}/${name}`, { method: 'DELETE' });
    loadRoleBindings();
  } catch(e) { alert('Delete failed: ' + e.message); }
}

// --- WYSIWYG Role Rule Editor ---

let _editingRoleName = null;
let _apiResourcesCache = null;
let _currentRules = [];

export async function editRoleRules(name) {
  _editingRoleName = name;
  document.getElementById('roleEditorOverlay').classList.add('active');
  document.getElementById('roleEditorName').textContent = name;
  document.getElementById('roleEditorTitle').textContent = 'Edit: ' + name;

  try {
    const [resRes, rulesRes] = await Promise.all([
      fetchJSON('/api/rbac/api-resources'),
      fetchJSON('/api/rbac/clusterroles/' + name + '/rules'),
    ]);
    _apiResourcesCache = resRes.items || [];
    _currentRules = rulesRes.rules || [];
    renderRoleEditor();
  } catch(e) {
    document.getElementById('roleEditorBody').innerHTML = '<div class="error">' + escapeHtml(e.message) + '</div>';
  }
}

// Namespace-scoped roles also use clusterrole rules API for now (same ClusterRole underneath via RoleBinding)
export async function editNsRoleRules(ns, name) {
  // For ns roles we read the Role resource rules directly
  _editingRoleName = name;
  document.getElementById('roleEditorOverlay').classList.add('active');
  document.getElementById('roleEditorName').textContent = ns + '/' + name;
  document.getElementById('roleEditorTitle').textContent = 'Edit: ' + ns + '/' + name;

  try {
    // Fetch api resources + the Role's current rules via clusterroles endpoint (reuse)
    const [resRes] = await Promise.all([
      fetchJSON('/api/rbac/api-resources'),
    ]);
    _apiResourcesCache = resRes.items || [];
    // Namespace Roles don't have a dedicated rules API yet, start empty
    _currentRules = [];
    renderRoleEditor();
  } catch(e) {
    document.getElementById('roleEditorBody').innerHTML = '<div class="error">' + escapeHtml(e.message) + '</div>';
  }
}

export function closeRoleEditor() {
  document.getElementById('roleEditorOverlay').classList.remove('active');
  _editingRoleName = null;
}

export function buildResourceTree() {
  const groups = {};
  for (const res of _apiResourcesCache) {
    const g = res.group || 'core';
    if (!groups[g]) groups[g] = { isCore: g === 'core', isCRD: res.crd, resources: [] };
    groups[g].resources.push(res);
  }
  return groups;
}

export function isVerbChecked(group, resource, verb) {
  for (const rule of _currentRules) {
    if (!ruleGroupsMatch(rule.apiGroups, group)) continue;
    if (!ruleResourcesMatch(rule.resources, resource)) continue;
    if (ruleVerbsMatch(rule.verbs, verb)) return true;
  }
  return false;
}

export function ruleGroupsMatch(ruleGroups, group) {
  if (!ruleGroups) return false;
  for (const g of ruleGroups) {
    if (g === '*' || g === group || (group === 'core' && g === '')) return true;
  }
  return false;
}

export function ruleResourcesMatch(ruleResources, resource) {
  if (!ruleResources) return false;
  for (const r of ruleResources) {
    if (r === '*' || r === resource) return true;
  }
  return false;
}

export function ruleVerbsMatch(ruleVerbs, verb) {
  if (!ruleVerbs) return false;
  for (const v of ruleVerbs) {
    if (v === '*' || v === verb) return true;
  }
  return false;
}

const ALL_VERBS = ['get', 'list', 'watch', 'create', 'update', 'patch', 'delete', 'deletecollection'];

export function renderRoleEditor() {
  const container = document.getElementById('roleEditorBody');
  const groups = buildResourceTree();
  const groupNames = Object.keys(groups).sort((a, b) => {
    if (a === 'core') return -1;
    if (b === 'core') return 1;
    if (groups[a].isCRD && !groups[b].isCRD) return 1;
    if (!groups[a].isCRD && groups[b].isCRD) return -1;
    return a.localeCompare(b);
  });

  let html = `<div style="margin-bottom:16px;">
    <span style="color:#8b949e;font-size:13px;">勾选复选框配置权限，保存时自动生成 PolicyRule。</span>
  </div>`;

  html += `<div style="display:flex;gap:8px;margin-bottom:16px;padding:8px 12px;background:#161b22;border-radius:6px;flex-wrap:wrap;">
    ${ALL_VERBS.map(v => `<span style="font-size:11px;padding:2px 8px;border-radius:4px;background:#21262d;color:#8b949e;">${v}</span>`).join('')}
  </div>`;

  for (const group of groupNames) {
    const g = groups[group];
    const groupLabel = group === 'core' ? 'Core API (v1)' : group;
    const badge = g.isCRD
      ? '<span style="background:#a371f71a;color:#bc8cff;padding:2px 8px;border-radius:8px;font-size:11px;border:1px solid #a371f733;">CRD</span>'
      : '<span style="background:#2386361a;color:#3fb950;padding:2px 8px;border-radius:8px;font-size:11px;border:1px solid #23863633;">builtin</span>';

    html += `<div style="margin-bottom:16px;border:1px solid #30363d;border-radius:8px;overflow:hidden;">
      <div style="display:flex;align-items:center;gap:8px;padding:10px 16px;background:#161b22;cursor:pointer;" onclick="toggleGroup(this)">
        <span style="color:#58a6ff;font-weight:600;font-size:14px;">${groupLabel}</span>
        ${badge}
        <span style="color:#8b949e;font-size:12px;margin-left:auto;">${g.resources.length} resources</span>
        <span style="color:#8b949e;">▶</span>
      </div>
      <div class="group-body" style="display:none;">
        <table style="width:100%;border-collapse:collapse;">
          <thead>
            <tr style="border-bottom:1px solid #30363d;">
              <th style="text-align:left;padding:6px 12px;font-size:12px;color:#8b949e;width:25%;">Resource</th>
              <th style="text-align:left;padding:6px 12px;font-size:12px;color:#8b949e;width:10%;">Kind</th>
              <th style="text-align:left;padding:6px 12px;font-size:12px;color:#8b949e;width:8%;">Scoped</th>
              ${ALL_VERBS.map(v => `<th style="text-align:center;padding:6px 4px;font-size:11px;color:#8b949e;">${v}</th>`).join('')}
            </tr>
          </thead>
          <tbody>
            ${g.resources.map(res => `<tr style="border-bottom:1px solid #21262d;" onmouseover="this.style.background='#161b22'" onmouseout="this.style.background=''">
              <td style="padding:5px 12px;font-size:13px;font-weight:500;">${res.name}</td>
              <td style="padding:5px 12px;font-size:12px;color:#8b949e;">${res.kind}</td>
              <td style="padding:5px 12px;font-size:12px;color:${res.namespaced ? '#3fb950' : '#d29922'};">${res.namespaced ? 'ns' : 'cluster'}</td>
              ${ALL_VERBS.map(v => {
                const checked = isVerbChecked(group, res.name, v) ? 'checked' : '';
                return `<td style="text-align:center;padding:5px 4px;">
                  <input type="checkbox" data-group="${group}" data-resource="${res.name}" data-verb="${v}" ${checked}
                    style="cursor:pointer;accent-color:#58a6ff;">
                </td>`;
              }).join('')}
            </tr>`).join('')}
          </tbody>
        </table>
      </div>
    </div>`;
  }

  container.innerHTML = html;
}

export function toggleGroup(headerEl) {
  const body = headerEl.nextElementSibling;
  if (body.style.display === 'none') {
    body.style.display = '';
    headerEl.querySelector('span:last-child').textContent = '▼';
  } else {
    body.style.display = 'none';
    headerEl.querySelector('span:last-child').textContent = '▶';
  }
}

export function selectAllVerbs(checked) {
  document.querySelectorAll('#roleEditorBody input[type=checkbox]').forEach(cb => cb.checked = checked);
}

export async function saveRoleRules() {
  const checkboxMap = {};
  document.querySelectorAll('#roleEditorBody input[type=checkbox]:checked').forEach(cb => {
    const key = cb.dataset.group + '|' + cb.dataset.resource;
    if (!checkboxMap[key]) checkboxMap[key] = new Set();
    checkboxMap[key].add(cb.dataset.verb);
  });

  const byGroup = {};
  for (const [key, verbs] of Object.entries(checkboxMap)) {
    const [group, resource] = key.split('|');
    if (!byGroup[group]) byGroup[group] = [];
    byGroup[group].push({ resources: resource, verbs: Array.from(verbs) });
  }

  const rules = [];
  for (const [group, items] of Object.entries(byGroup)) {
    const byVerbs = {};
    for (const item of items) {
      const verbKey = item.verbs.slice().sort().join(',');
      if (!byVerbs[verbKey]) byVerbs[verbKey] = [];
      byVerbs[verbKey].push(item.resources);
    }
    for (const [verbKey, resources] of Object.entries(byVerbs)) {
      rules.push({
        apiGroups: [group === 'core' ? '' : group],
        resources: resources,
        verbs: verbKey.split(','),
      });
    }
  }

  try {
    const res = await fetch('/api/rbac/clusterroles/' + _editingRoleName + '/rules', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ rules }),
    });
    if (!res.ok) { const e = await res.json(); throw new Error(e.error || 'HTTP ' + res.status); }
    closeRoleEditor();
    loadClusterRoles();
  } catch(e) {
    alert('Failed to save: ' + e.message);
  }
}

// --- RBAC Tab Loader ---

export function loadRBAC() {
  _nsCache = null;
  _crCache = null;
  _subjectsCache = null;
  _userRoleCache = null;
  _roleMappingData = null;
  loadUsers();
  loadRoleMappings();
  loadClusterRoles();
  loadNsRoles();
  loadRoleBindings();
  loadAuthConfig();
  loadAuthProviders();
}
