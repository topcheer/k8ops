// main.js — Entry point for k8ops dashboard
// Imports all modules and bridges exports to window for inline event handler compatibility.
// This file is loaded via <script type="module" src="/main.js"> in index.html.

import * as utils from './modules/utils.js';
import * as core from './core.js';
import * as overview from './overview.js';
import * as nodes from './nodes.js';
import * as pods from './pods.js';
import * as chat from './chat.js';
import * as resources from './resources.js';
import * as topology from './topology.js';
import * as rbac from './rbac.js';
import * as providers from './providers.js';
import * as security from './security.js';

// Bridge all exports to window so inline onclick="fn()" handlers continue working.
// This is a transitional measure — future versions will replace inline handlers
// with addEventListener and progressively remove window exposure.
const allModules = [utils, core, overview, nodes, pods, chat, resources, topology, rbac, providers, security];
for (const mod of allModules) {
  for (const key of Object.keys(mod)) {
    window[key] = mod[key];
  }
}

// Initialize on DOMContentLoaded
document.addEventListener('DOMContentLoaded', function() {
  // Call initTabFromHash to restore tab from URL
  if (typeof core.initTabFromHash === 'function') {
    core.initTabFromHash();
  }
  // Check current user
  if (typeof pods.checkCurrentUser === 'function') {
    pods.checkCurrentUser();
  }
  // Check provider configuration
  checkProviderConfig();
  // Start notification center polling
  if (typeof core.startNotifPolling === 'function') {
    core.startNotifPolling();
  }
  // Start connection status monitor
  if (typeof core.startConnMonitor === 'function') {
    core.startConnMonitor();
  }
});

// Check if AI provider is configured, show banner if not
async function checkProviderConfig() {
  try {
    const res = await fetch('/api/provider/status');
    if (!res.ok) return;
    const data = await res.json();
    if (!data.active || !data.hasApiKey) {
      showProviderBanner(data);
    }
  } catch(e) { /* silent */ }
}

function showProviderBanner(data) {
  // Avoid duplicate banners
  if (document.getElementById('providerBanner')) return;
  const banner = document.createElement('div');
  banner.id = 'providerBanner';
  banner.className = 'provider-banner';
  banner.innerHTML = `
    <span class="provider-banner-icon">\u26A0</span>
    <span class="provider-banner-text">
      AI Provider ${data.active ? 'API Key' : 'is not'} configured. AI Chat, Diagnostics, and Optimization features are unavailable.
    </span>
    <button class="provider-banner-btn" onclick="showTab('settings')">Configure Now</button>
    <button class="provider-banner-close" onclick="this.parentElement.remove()">&times;</button>
  `;
  // Insert at top of the app container
  const app = document.querySelector('.app-container') || document.body;
  app.insertBefore(banner, app.firstChild);
}
