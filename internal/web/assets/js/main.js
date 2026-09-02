// Boot, routing and the token gate.
//
// Routing happens in the fragment, so the server never has to guess whether an
// unknown path is a page or a mistake: it serves five files and 404s the rest.

import { byId, qsa, notice } from './dom.js';
import * as api from './api.js';
import { duration } from './format.js';

import * as dashboard from './views/dashboard.js';
import * as mapView from './views/map.js';
import * as transit from './views/transit.js';
import * as records from './views/records.js';
import * as sources from './views/sources.js';

const VIEWS = {
  dashboard: { module: dashboard, section: 'view-dashboard' },
  map: { module: mapView, section: 'view-map' },
  transit: { module: transit, section: 'view-transit' },
  records: { module: records, section: 'view-records' },
  sources: { module: sources, section: 'view-sources' },
};

const DEFAULT_VIEW = 'dashboard';

let current = null;
let healthTimer = 0;

function routeName() {
  const name = window.location.hash.replace(/^#\/?/, '').split('?')[0];
  return VIEWS[name] ? name : DEFAULT_VIEW;
}

function show(name) {
  if (current === name) return;

  if (current) {
    const previous = VIEWS[current];
    if (previous.module.unmount) previous.module.unmount();
    byId(previous.section).hidden = true;
  }

  current = name;
  const view = VIEWS[name];
  const section = byId(view.section);
  section.hidden = false;

  for (const button of qsa('.rail-link')) {
    button.classList.toggle('is-on', button.dataset.view === name);
  }

  // Mounted after the section is visible: Leaflet measures its container, and
  // a map built inside a hidden element comes out zero-sized.
  view.module.mount(section);
}

// ---------- token gate ----------

function openGate(message) {
  const gate = byId('token-gate');
  gate.hidden = false;
  notice(byId('token-error'), message || '', 'error');
  byId('token-input').focus();
}

function closeGate() {
  byId('token-gate').hidden = true;
  byId('token-input').value = '';
}

function wireGate() {
  const input = byId('token-input');

  byId('token-save').addEventListener('click', async () => {
    const value = input.value.trim();
    if (!value) {
      notice(byId('token-error'), 'Paste the value of EYE_API_TOKEN.', 'error');
      return;
    }

    api.setToken(value);
    try {
      await api.get('/v1/sources');
    } catch (err) {
      if (err instanceof api.ApiError && err.status === 401) {
        api.clearToken();
        notice(byId('token-error'), 'The API refused that token.', 'error');
        return;
      }
      // Any other failure is not the token's fault; let the view report it.
    }

    closeGate();
    refreshSignOut();
    if (current) {
      const view = VIEWS[current];
      if (view.module.unmount) view.module.unmount();
      view.module.mount(byId(view.section));
    }
  });

  input.addEventListener('keydown', (event) => {
    if (event.key === 'Enter') byId('token-save').click();
  });

  byId('sign-out').addEventListener('click', () => {
    api.clearToken();
    refreshSignOut();
    openGate('Token cleared from this browser.');
  });
}

function refreshSignOut() {
  byId('sign-out').hidden = !api.getToken();
}

// ---------- connection state ----------

async function probeHealth() {
  const dot = byId('conn').querySelector('.dot');
  const text = byId('conn-text');
  const meta = byId('conn-version');

  try {
    // /health is open by contract, so this reports the process, not the token.
    const health = await api.get('/health');
    dot.className = 'dot dot-ok';
    text.textContent = `online · up ${duration(Number(health.uptime_s || 0) * 1000)}`;
    meta.textContent = health.version ? `v${health.version}` : '';
  } catch (err) {
    dot.className = 'dot dot-bad';
    text.textContent = err instanceof api.ApiError && err.status ? `api ${err.status}` : 'unreachable';
    meta.textContent = '';
  }
}

// ---------- boot ----------

function boot() {
  wireGate();
  refreshSignOut();

  api.onUnauthorized(() => {
    if (byId('token-gate').hidden) openGate('This deployment requires a token.');
  });

  for (const button of qsa('.rail-link')) {
    button.addEventListener('click', () => {
      window.location.hash = `#/${button.dataset.view}`;
    });
  }

  window.addEventListener('hashchange', () => show(routeName()));

  probeHealth();
  healthTimer = window.setInterval(probeHealth, 60000);
  window.addEventListener('pagehide', () => window.clearInterval(healthTimer));

  show(routeName());
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', boot, { once: true });
} else {
  boot();
}
