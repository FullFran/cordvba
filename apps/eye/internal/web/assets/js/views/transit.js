// TRANSIT — buses and trains, as boards rather than as tables of JSON.
//
// The bus arrivals endpoint is a live passthrough over an undocumented personal
// source. eye refuses to serve it unless the operator opted in AND the
// deployment is authenticated, so a 403 here is the gate working, not a bug.
// This view says that in words, because an "Error 403" dialog would teach the
// operator that the console is broken when in fact it is behaving correctly.

import { el, fill, byId, notice, debounce } from '../dom.js';
import * as api from '../api.js';
import * as basemap from '../basemap.js';
import { ageMs, clock, count, duration, stamp } from '../format.js';

const ARRIVALS_MS = 30000;

// AUCORSA's own board moves about every half minute. Anything older than three
// of those is not a live arrival any more, whatever the endpoint calls it.
const STALE_MS = 90000;

let arrivalsTimer = 0;
let arrivalsPaused = false;
let selectedStop = null;
let miniMap = null;
let miniMarker = null;
let searchDebounced = null;
let wired = false;
let linesLoaded = false;

export function mount() {
  if (!wired) {
    wire();
    wired = true;
  }

  if (!linesLoaded) {
    loadLines();
    linesLoaded = true;
  }

  window.setTimeout(() => {
    ensureMiniMap();
    if (miniMap) miniMap.invalidateSize();
  }, 0);

  if (selectedStop) startArrivals();
}

export function unmount() {
  window.clearInterval(arrivalsTimer);
  arrivalsTimer = 0;
  if (searchDebounced) searchDebounced.cancel();
}

function wire() {
  searchDebounced = debounce(searchStops, 300);

  byId('stop-search').addEventListener('input', () => searchDebounced());
  byId('stop-search').addEventListener('keydown', (event) => {
    if (event.key === 'Enter') searchStops();
  });
  byId('stop-search-go').addEventListener('click', () => searchStops());

  byId('arrivals-pause').addEventListener('click', () => {
    arrivalsPaused = !arrivalsPaused;
    const button = byId('arrivals-pause');
    button.setAttribute('aria-pressed', String(arrivalsPaused));
    button.textContent = arrivalsPaused ? 'RESUME' : 'PAUSE';
    if (!arrivalsPaused) loadArrivals();
  });

  byId('station-go').addEventListener('click', () => loadDepartures());
  byId('station-input').addEventListener('keydown', (event) => {
    if (event.key === 'Enter') loadDepartures();
  });

  for (const tab of document.querySelectorAll('#view-transit .tab')) {
    tab.addEventListener('click', () => selectTab(tab.dataset.tab));
  }
}

function selectTab(name) {
  for (const tab of document.querySelectorAll('#view-transit .tab')) {
    tab.classList.toggle('is-on', tab.dataset.tab === name);
  }
  byId('pane-bus').hidden = name !== 'bus';
  byId('pane-train').hidden = name !== 'train';

  if (name === 'train' && !byId('departures-board').childElementCount) loadDepartures();
  if (miniMap) window.setTimeout(() => miniMap.invalidateSize(), 0);
}

// ---------- buses: stops ----------

async function searchStops() {
  const text = byId('stop-search').value.trim();
  const results = byId('stop-results');

  if (text.length < 2) {
    fill(results, el('p', { class: 'hint', text: 'type at least two characters' }));
    return;
  }

  try {
    const payload = await api.get('/v1/transit/stops', { text, limit: 24 });
    const stops = api.listFrom(payload, 'stops', 'entities');

    if (stops.length === 0) {
      fill(results, el('p', { class: 'hint', text: `no stop matches “${text}”` }));
      return;
    }

    fill(results, stops.map((stop) => {
      const code = stopCode(stop);
      const chip = el('button', { type: 'button', class: 'chip' },
        el('span', { text: stopName(stop) }), ' ',
        el('span', { class: 'chip-code', text: code || '?' }));
      chip.addEventListener('click', () => selectStop(stop, chip));
      return chip;
    }));
  } catch (err) {
    fill(results, el('p', { class: 'hint', text: describe(err) }));
  }
}

/**
 * The pole number the arrivals endpoint wants.
 *
 * The registry stores it in the entity payload; the transit endpoint may
 * surface it flattened. Both are accepted, and the id tail is the last resort,
 * because a stop chip that silently picks the wrong number is worse than one
 * that shows a question mark.
 */
function stopCode(stop) {
  const payload = stop.payload || {};
  const direct = api.pick(stop, 'stop_id', 'code', 'pole', 'local_key')
    || api.pick(payload, 'stop_id', 'code', 'pole');
  if (direct) return String(direct);

  const id = String(stop.id || '');
  const tail = id.split(':').pop();
  return /^\d+$/.test(tail) ? tail : '';
}

function stopName(stop) {
  return String(api.pick(stop, 'name', 'title') || stop.id || 'unnamed stop');
}

function positionOf(item) {
  const p = item && item.position;
  if (p && Number.isFinite(p.lat) && Number.isFinite(p.lon)) return [p.lat, p.lon];
  return null;
}

function selectStop(stop, chip) {
  selectedStop = stop;
  for (const other of document.querySelectorAll('#stop-results .chip')) other.classList.remove('is-on');
  if (chip) chip.classList.add('is-on');

  showOnMiniMap(positionOf(stop), stopName(stop),
    positionOf(stop)
      ? null
      : 'AUCORSA publishes its stop directory without coordinates, so this stop has no position to show. '
        + 'eye records that absence rather than guessing one.');

  startArrivals();
}

function startArrivals() {
  window.clearInterval(arrivalsTimer);
  loadArrivals();
  arrivalsTimer = window.setInterval(() => {
    if (!arrivalsPaused) loadArrivals();
  }, ARRIVALS_MS);
}

async function loadArrivals() {
  if (!selectedStop) return;

  const code = stopCode(selectedStop);
  const board = byId('arrivals-board');

  if (!code) {
    notice(byId('arrivals-notice'),
      'This stop has no pole number in the registry, and the arrivals endpoint is addressed by pole number.',
      'error');
    fill(board);
    return;
  }

  try {
    const payload = await api.get('/v1/transit/arrivals', { stop: code });
    notice(byId('arrivals-notice'), '');
    renderArrivals(payload);
  } catch (err) {
    fill(board);
    byId('arrivals-stale').hidden = true;
    byId('arrivals-stamp').textContent = 'not available';
    notice(byId('arrivals-notice'), err instanceof api.ApiError && err.status === 403
      ? personalSourceExplanation(err)
      : describe(err), err instanceof api.ApiError && err.status === 403 ? 'warn' : 'error');
  }
}

function renderArrivals(payload) {
  const arrivals = api.listFrom(payload, 'arrivals');
  const board = byId('arrivals-board');

  if (arrivals.length === 0) {
    fill(board, el('p', { class: 'board-empty', text: 'no arrivals reported for this stop right now' }));
    byId('arrivals-stamp').textContent = `checked ${stamp(new Date())}`;
    byId('arrivals-stale').hidden = true;
    return;
  }

  const newest = arrivals.reduce((best, a) => {
    const t = ageMs(a.observed_at);
    return t < best ? t : best;
  }, Infinity);

  // Stale means the data behind the countdown stopped moving. It is shown as a
  // badge and not as an absence, because a frozen board that looks live is the
  // single most misleading thing a transit display can do.
  const stale = payload.stale === true || newest > STALE_MS;
  byId('arrivals-stale').hidden = !stale;
  byId('arrivals-stamp').textContent = Number.isFinite(newest)
    ? `observed ${duration(newest)} ago · refreshed every ${ARRIVALS_MS / 1000}s`
    : `checked ${stamp(new Date())}`;

  fill(board, arrivals
    .slice()
    .sort((a, b) => Number(a.eta_minutes ?? 999) - Number(b.eta_minutes ?? 999))
    .map((arrival) => {
      const eta = Number(api.pick(arrival, 'eta_minutes', 'eta', 'minutes'));
      const value = Number.isFinite(eta) ? (eta <= 0 ? 'due' : `${eta} min`) : '—';
      const etaNode = el('div', { class: 'board-eta', text: value });
      if (Number.isFinite(eta) && eta <= 2) etaNode.classList.add('soon');

      return el('div', { class: 'board-row' },
        el('div', { class: 'board-line', text: String(arrival.line ?? '?') }),
        el('div', {},
          el('div', { class: 'board-dest', text: String(arrival.destination || 'destination not stated') }),
          el('div', {
            class: 'board-sub',
            text: `${arrival.stop_name || arrival.stop_id || ''} · observed ${stamp(arrival.observed_at)}`,
          })),
        etaNode);
    }));
}

/**
 * The 403 the redistribution gate returns, explained.
 *
 * ADR-0007 and ADR-0008: an undocumented endpoint a public site calls to render
 * public information is a reasonable thing to read for oneself, and is not a
 * contract anybody may redistribute. So it is gated twice.
 */
function personalSourceExplanation(err) {
  return el('div', {},
    el('h3', { text: 'PERSONAL SOURCE NOT SERVED' }),
    el('p', {
      text: 'Live bus arrivals come from an undocumented AUCORSA endpoint. It is readable for '
        + 'personal use and is not redistributable, so eye refuses to serve it unless the '
        + 'deployment is both opted in and private.',
    }),
    el('p', { text: 'Both of these must hold:' }),
    el('ul', {},
      el('li', {}, el('code', { text: 'EYE_ALLOW_PERSONAL_SOURCES' }), ' is set to a truthy value'),
      el('li', {}, el('code', { text: 'EYE_API_TOKEN' }), ' is set, so the deployment is authenticated')),
    err && err.detail ? el('p', { text: `The API added: ${err.detail}` }) : null,
    el('p', { text: 'Everything else in this section works without the gate: lines, service hours and train departures are public.' }));
}

// ---------- buses: lines ----------

async function loadLines() {
  try {
    const payload = await api.get('/v1/transit/lines');
    const lines = api.listFrom(payload, 'lines');
    notice(byId('lines-notice'), '');

    if (lines.length === 0) {
      fill(byId('lines-body'), el('tr', {},
        el('td', { colSpan: 4, class: 'hint', text: 'no lines stored yet — run a collection first' })));
      return;
    }

    fill(byId('lines-body'), lines.map((line) => el('tr', {},
      el('td', { class: 'board-line nowrap', text: String(api.pick(line, 'code', 'line') || '?') }),
      el('td', { text: String(api.pick(line, 'name', 'title') || '—') }),
      el('td', { class: 'num', text: count(api.pick(line, 'stops', 'stop_count') ?? 0) }),
      el('td', { text: serviceHours(line) }))));
  } catch (err) {
    fill(byId('lines-body'));
    notice(byId('lines-notice'), describe(err), 'error');
  }
}

function serviceHours(line) {
  const hours = api.pick(line, 'service_hours', 'schedule', 'hours');
  if (Array.isArray(hours)) return hours.join(' · ');
  return hours ? String(hours) : 'not published';
}

// ---------- trains ----------

async function loadDepartures() {
  const station = byId('station-input').value.trim() || 'CORDOBA';
  const board = byId('departures-board');

  try {
    const payload = await api.get('/v1/transit/departures', { station, limit: 25 });
    const departures = api.listFrom(payload, 'departures');
    notice(byId('departures-notice'), '');

    byId('departures-stamp').textContent = `${departures.length} services · loaded ${stamp(new Date())}`;

    if (departures.length === 0) {
      fill(board, el('p', { class: 'board-empty', text: `nothing stored for “${station}”` }));
    } else {
      fill(board, departures.map(departureRow));
    }

    locateStation(station);
  } catch (err) {
    fill(board);
    byId('departures-stamp').textContent = 'not available';
    notice(byId('departures-notice'), describe(err), 'error');
  }
}

function departureRow(departure) {
  const delay = Number(api.pick(departure, 'delay_minutes', 'delay', 'delay_min') ?? 0);
  const delayed = Number.isFinite(delay) && delay > 0;
  const when = api.pick(departure, 'departure_time', 'scheduled', 'time', 'observed_at');

  const row = el('div', { class: 'board-row' },
    el('div', { class: 'board-line', text: clock(when) }),
    el('div', {},
      el('div', { class: 'board-dest', text: String(api.pick(departure, 'destination', 'title') || 'destination not stated') }),
      el('div', {
        class: 'board-sub',
        text: [
          api.pick(departure, 'train', 'train_number', 'line', 'service'),
          api.pick(departure, 'product', 'type'),
          `observed ${stamp(departure.observed_at)}`,
        ].filter(Boolean).join(' · '),
      })),
    el('div', { class: delayed ? 'board-eta board-delay' : 'board-eta', text: delayed ? `+${delay}′` : 'on time' }));

  if (delayed) row.classList.add('is-delayed');
  return row;
}

/**
 * Put the station on the small map when eye actually knows where it is.
 * It usually does not, and saying so is the correct answer.
 */
async function locateStation(station) {
  try {
    const payload = await api.get('/v1/entities', { text: station, limit: 20 });
    const match = api.listFrom(payload, 'entities').find((entity) => positionOf(entity));
    if (match) {
      showOnMiniMap(positionOf(match), String(match.title || station), null);
      return;
    }
  } catch {
    /* fall through to the honest note */
  }
  showOnMiniMap(null, station, `eye holds no coordinates for “${station}”, so there is nothing to place on the map.`);
}

// ---------- mini map ----------

function ensureMiniMap() {
  const canvas = byId('transit-map');
  if (miniMap || !basemap.available()) {
    if (!basemap.available()) fill(canvas, basemap.unavailableNotice());
    return;
  }
  miniMap = basemap.createMap(canvas, { zoom: 14 });
}

function showOnMiniMap(point, label, note) {
  byId('transit-map-note').textContent = note || (point ? `${label} · ${point[0].toFixed(5)}, ${point[1].toFixed(5)}` : label);

  ensureMiniMap();
  if (!miniMap) return;

  if (miniMarker) {
    miniMap.removeLayer(miniMarker);
    miniMarker = null;
  }
  if (!point) return;

  miniMarker = window.L.circleMarker(point, basemap.markerStyle('bus_stop', 2)).addTo(miniMap);
  miniMarker.bindTooltip(label, { direction: 'top' });
  miniMap.setView(point, 16);
}

function describe(err) {
  if (err instanceof api.ApiError) {
    if (err.status === 401) return 'The API refused the request: a token is required.';
    if (err.status === 0) return `The API is unreachable: ${err.detail}`;
    return `The API answered ${err.status}: ${err.detail || err.code || 'no detail'}`;
  }
  return String(err && err.message ? err.message : err);
}
