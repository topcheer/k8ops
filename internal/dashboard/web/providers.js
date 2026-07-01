// --- Auth Provider Icons ---
// Returns an HTML span with icon/emoji for common auth providers.
import { escapeHtml, fetchJSON, showToast } from './modules/utils.js';

export function providerIcon(name, size) {
  size = size || 24;
  const icons = {
    github:    '<svg width="' + size + '" height="' + size + '" viewBox="0 0 24 24" fill="#f0f6fc"><path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12"/></svg>',
    google: '<svg width="' + size + '" height="' + size + '" viewBox="0 0 24 24"><path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"/><path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/><path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/><path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/></svg>',
    microsoft: '<svg width="' + size + '" height="' + size + '" viewBox="0 0 23 23"><path fill="#f25022" d="M1 1h10v10H1z"/><path fill="#7fba00" d="M12 1h10v10H12z"/><path fill="#00a4ef" d="M1 12h10v10H1z"/><path fill="#ffb900" d="M12 12h10v10H12z"/></svg>',
    gitlab: '<svg width="' + size + '" height="' + size + '" viewBox="0 0 24 24" fill="#fc6d26"><path d="M23.955 13.587l-1.347-4.135-2.664-8.197a.455.455 0 00-.867 0L16.416 9.45H7.584L4.923 1.255a.455.455 0 00-.867 0L1.392 9.452.045 13.587a.924.924 0 00.331 1.023L12 23.054l11.624-8.443a.92.92 0 00.331-1.024"/></svg>',
    keycloak: '<svg width="' + size + '" height="' + size + '" viewBox="0 0 24 24"><rect width="24" height="24" rx="4" fill="#4d4d4d"/><text x="12" y="17" text-anchor="middle" fill="#fff" font-size="14" font-family="sans-serif" font-weight="bold">KC</text></svg>',
    okta: '<svg width="' + size + '" height="' + size + '" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9" fill="none" stroke="#007dc1" stroke-width="4"/></svg>',
    auth0: '<svg width="' + size + '" height="' + size + '" viewBox="0 0 24 24" fill="#eb5424"><path d="M21.98 7.448L19.62 0H4.347L2.02 7.448c-1.352 4.312.03 8.87 3.344 11.697L12.005 24l6.635-4.855c3.31-2.828 4.69-7.385 3.34-11.697M12 18.07l-3.766-2.752 3.766-2.752 3.766 2.752z"/></svg>',
    ldap: '<span style="font-size:' + (size * 0.7) + 'px;">🔑</span>',
    oidc: '<span style="font-size:' + (size * 0.7) + 'px;">🔐</span>',
    custom: '<span style="font-size:' + (size * 0.7) + 'px;">⚡</span>',
  };
  // Check if it's an emoji (single char or short string)
  if (name && !icons[name] && name.length <= 2) {
    return '<span style="font-size:' + (size * 0.7) + 'px;">' + name + '</span>';
  }
  return icons[name] || icons.custom;
}

// --- Auth Provider Management ---

let _providerPresets = null;

export async function loadAuthProviders() {
  const container = document.getElementById('authProviderList');
  if (!container) return;

  try {
    // Load presets (cached)
    if (!_providerPresets) {
      const data = await fetchJSON('/api/auth/provider-presets');
      _providerPresets = data.presets || [];
    }

    const data = await fetchJSON('/api/auth/providers');
    const providers = data.providers || [];

    let html = '';

    // Existing providers
    for (const p of providers) {
      const statusBadge = p.enabled
        ? '<span style="color:#3fb950;">● enabled</span>'
        : '<span style="color:#8b949e;">○ disabled</span>';

      const toggleLabel = p.enabled ? 'Disable' : 'Enable';
      const toggleColor = p.enabled ? '#f85149' : '#3fb950';

      // Config summary
      let cfgSummary = '';
      if (p.config) {
        if (p.config_type === 'ldap') {
          cfgSummary = `<span style="color:#8b949e;font-size:11px;font-family:monospace;">${p.config.server || ''} | ${p.config.search_base || ''}</span>`;
        } else if (p.config_type === 'oidc') {
          cfgSummary = `<span style="color:#8b949e;font-size:11px;font-family:monospace;">${p.config.issuer || ''}</span>`;
        }
      }

      html += `<div style="display:flex;align-items:center;gap:12px;padding:12px;border:1px solid #30363d;border-radius:8px;margin-bottom:8px;">
        <div style="flex-shrink:0;">${providerIcon(p.icon, 32)}</div>
        <div style="flex-grow:1;">
          <div style="display:flex;align-items:center;gap:8px;">
            <span style="font-weight:600;font-size:14px;">${escapeHtml(p.display_name)}</span>
            <span style="background:#21262d;border:1px solid #30363d;padding:1px 6px;border-radius:4px;font-size:10px;text-transform:uppercase;color:#8b949e;">${escapeHtml(p.type)}</span>
            ${statusBadge}
          </div>
          <div style="font-size:12px;color:#8b949e;font-family:monospace;">${escapeHtml(p.name)}</div>
          <div>${cfgSummary}</div>
        </div>
        <div style="display:flex;gap:6px;flex-shrink:0;">
          <button onclick="toggleProvider(${p.id}, ${!p.enabled})" class="btn-secondary" style="padding:4px 12px;font-size:11px;color:${toggleColor};">${toggleLabel}</button>
          <button onclick="editProvider(${p.id})" class="btn-secondary" style="padding:4px 12px;font-size:11px;">Edit</button>
          <button onclick="deleteProvider(${p.id}, '${escapeHtml(p.name)}')" class="btn-secondary" style="padding:4px 12px;font-size:11px;color:#f85149;">Delete</button>
        </div>
      </div>`;
    }

    if (providers.length === 0) {
      html += '<div style="text-align:center;padding:24px;color:#8b949e;">No auth providers configured. Add one below.</div>';
    }

    // Add provider form
    html += `<div id="addProviderSection" style="margin-top:16px;padding:16px;border:1px dashed #30363d;border-radius:8px;">
      <div style="font-weight:600;font-size:13px;margin-bottom:12px;color:#58a6ff;">+ Add Auth Provider</div>

      <!-- Preset grid -->
      <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(140px,1fr));gap:8px;margin-bottom:12px;" id="presetGrid">
        ${_providerPresets.map(ps => `
          <div onclick="selectPreset('${ps.key}')" class="preset-card" data-key="${ps.key}"
            style="cursor:pointer;padding:10px 8px;border:1px solid #30363d;border-radius:8px;text-align:center;transition:all 0.2s;background:#0d1117;"
            onmouseover="this.style.borderColor='#58a6ff'"
            onmouseout="if(this.dataset.selected!=='true')this.style.borderColor='#30363d'">
            <div style="margin-bottom:4px;">${providerIcon(ps.icon, 28)}</div>
            <div style="font-size:12px;font-weight:600;">${escapeHtml(ps.display_name)}</div>
            <div style="font-size:10px;color:#8b949e;">${escapeHtml(ps.description)}</div>
          </div>
        `).join('')}
      </div>

      <!-- Config form (shown after preset selection) -->
      <div id="providerConfigForm" style="display:none;margin-top:12px;padding-top:12px;border-top:1px solid #21262d;">
        <div id="presetHelp" style="padding:8px 12px;background:#161b22;border-radius:6px;margin-bottom:12px;font-size:12px;color:#8b949e;border-left:3px solid #58a6ff;display:none;"></div>
        <div style="display:flex;gap:8px;flex-wrap:wrap;align-items:end;">
          <div><label style="font-size:12px;color:#8b949e;">Name (slug) *</label><br>
            <input type="text" id="newProviderName" class="form-input" placeholder="company-ldap" style="width:180px;"></div>
          <div><label style="font-size:12px;color:#8b949e;">Display Name</label><br>
            <input type="text" id="newProviderDisplay" class="form-input" placeholder="Company LDAP" style="width:200px;"></div>
          <div><label style="font-size:12px;color:#8b949e;">Icon</label><br>
            <input type="text" id="newProviderIcon" class="form-input" placeholder="github" style="width:100px;"></div>
          <div id="ldapFields" style="display:none;flex-basis:100%;flex-wrap:wrap;gap:8px;margin-top:8px;">
            <div><label style="font-size:12px;color:#8b949e;">LDAP Server *</label><br>
              <input type="text" id="ldapServer" class="form-input" placeholder="ldap://host:389" style="width:260px;"></div>
            <div><label style="font-size:12px;color:#8b949e;">Bind DN *</label><br>
              <input type="text" id="ldapBindDN" class="form-input" placeholder="cn=admin,dc=example,dc=com" style="width:260px;"></div>
            <div><label style="font-size:12px;color:#8b949e;">Bind Password *</label><br>
              <input type="password" id="ldapBindPW" class="form-input" placeholder="••••••••" style="width:180px;"></div>
            <div><label style="font-size:12px;color:#8b949e;">Search Base *</label><br>
              <input type="text" id="ldapSearchBase" class="form-input" placeholder="dc=example,dc=com" style="width:260px;"></div>
            <div><label style="font-size:12px;color:#8b949e;">Search Filter</label><br>
              <input type="text" id="ldapSearchFilter" class="form-input" placeholder="(uid={username})" style="width:200px;"></div>
            <div style="display:flex;align-items:center;gap:6px;flex-basis:100%;">
              <input type="checkbox" id="ldapSkipTLSVerify" style="width:auto;">
              <label for="ldapSkipTLSVerify" style="font-size:12px;color:#8b949e;">Skip TLS Certificate Verification (insecure)</label>
            </div>
          </div>
          <div id="oidcFields" style="display:none;flex-basis:100%;flex-wrap:wrap;gap:8px;margin-top:8px;">
            <div><label style="font-size:12px;color:#8b949e;">Issuer URL *</label><br>
              <input type="text" id="oidcIssuer" class="form-input" placeholder="https://accounts.google.com" style="width:320px;"></div>
            <div><label style="font-size:12px;color:#8b949e;">Client ID *</label><br>
              <input type="text" id="oidcClientID" class="form-input" placeholder="your-client-id" style="width:260px;"></div>
            <div><label style="font-size:12px;color:#8b949e;">Client Secret *</label><br>
              <input type="password" id="oidcClientSecret" class="form-input" placeholder="••••••••" style="width:200px;"></div>
            <div><label style="font-size:12px;color:#8b949e;">Redirect URL</label><br>
              <input type="text" id="oidcRedirectURL" class="form-input" placeholder="auto-generated" style="width:320px;"></div>
          </div>
          <button onclick="createProvider()" class="btn-primary" style="margin-top:8px;">Create Provider</button>
        </div>
      </div>
    </div>`;

    container.innerHTML = html;
  } catch(e) {
    container.innerHTML = `<div class="error">${escapeHtml(e.message)}</div>`;
  }
}

let _selectedPreset = null;

export function selectPreset(key) {
  _selectedPreset = _providerPresets.find(p => p.key === key);
  if (!_selectedPreset) return;

  // Highlight selected
  document.querySelectorAll('.preset-card').forEach(card => {
    if (card.dataset.key === key) {
      card.dataset.selected = 'true';
      card.style.borderColor = '#58a6ff';
      card.style.background = '#161b22';
    } else {
      card.dataset.selected = 'false';
      card.style.borderColor = '#30363d';
      card.style.background = '#0d1117';
    }
  });

  // Show config form
  const form = document.getElementById('providerConfigForm');
  form.style.display = 'block';

  // Auto-fill name/display/icon
  document.getElementById('newProviderName').value = key === 'custom-oidc' ? '' : key;
  document.getElementById('newProviderDisplay').value = _selectedPreset.display_name;
  document.getElementById('newProviderIcon').value = _selectedPreset.icon;

  // Show help
  const helpDiv = document.getElementById('presetHelp');
  if (_selectedPreset.help) {
    helpDiv.style.display = 'block';
    helpDiv.innerHTML = '<strong>Configuration Guide:</strong> ' + _selectedPreset.help;
  } else {
    helpDiv.style.display = 'none';
  }

  // Show type-specific fields
  const isLDAP = _selectedPreset.type === 'ldap';
  document.getElementById('ldapFields').style.display = isLDAP ? 'flex' : 'none';
  document.getElementById('oidcFields').style.display = !isLDAP ? 'flex' : 'none';

  // Auto-fill OIDC preset values
  if (!isLDAP && _selectedPreset.oidc) {
    document.getElementById('oidcIssuer').value = _selectedPreset.oidc.issuer || '';
  }
}

export async function createProvider() {
  if (!_selectedPreset) { showToast('Select a provider preset first', 'error'); return; }
  const name = document.getElementById('newProviderName').value.trim();
  if (!name) { showToast('Name is required', 'error'); return; }

  const body = {
    name,
    type: _selectedPreset.type,
    display_name: document.getElementById('newProviderDisplay').value.trim(),
    icon: document.getElementById('newProviderIcon').value.trim(),
    enabled: true,
    priority: 0,
  };

  if (_selectedPreset.type === 'ldap') {
    body.ldap = {
      server: document.getElementById('ldapServer').value.trim(),
      bind_dn: document.getElementById('ldapBindDN').value.trim(),
      bind_pw: document.getElementById('ldapBindPW').value,
      search_base: document.getElementById('ldapSearchBase').value.trim(),
      search_filter: document.getElementById('ldapSearchFilter').value.trim() || '(uid={username})',
      start_tls: false,
      skip_tls_verify: document.getElementById('ldapSkipTLSVerify').checked,
    };
  } else {
    body.oidc = {
      issuer: document.getElementById('oidcIssuer').value.trim(),
      client_id: document.getElementById('oidcClientID').value.trim(),
      client_secret: document.getElementById('oidcClientSecret').value,
      redirect_url: document.getElementById('oidcRedirectURL').value.trim(),
    };
  }

  try {
    const res = await fetch('/api/auth/providers', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(body),
    });
    if (!res.ok) { const e = await res.json(); throw new Error(e.error || 'HTTP ' + res.status); }
    _selectedPreset = null;
    loadAuthProviders();
  } catch(e) { showToast('Failed: ' + e.message, 'error'); }
}

export async function toggleProvider(id, enable) {
  try {
    await fetch('/api/auth/providers/' + id, {
      method: 'PATCH',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({enabled: enable}),
    });
    loadAuthProviders();
  } catch(e) { showToast(e.message, 'error'); }
}

export async function deleteProvider(id, name) {
  if (!confirm(`Delete provider "${name}"?`)) return;
  try {
    await fetch('/api/auth/providers/' + id, {method: 'DELETE'});
    loadAuthProviders();
  } catch(e) { showToast(e.message, 'error'); }
}

export function editProvider(id) {
  // For now, just scroll to top. Full edit form can be added later.
  showToast('Edit functionality — provider ID ' + id + '. Use delete + recreate for now.', 'warning');
}
