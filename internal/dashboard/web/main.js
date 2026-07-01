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

// Bridge all exports to window so inline onclick="fn()" handlers continue working.
// This is a transitional measure — future versions will replace inline handlers
// with addEventListener and progressively remove window exposure.
const allModules = [utils, core, overview, nodes, pods, chat, resources, topology, rbac, providers];
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
});
