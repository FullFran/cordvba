// DASHBOARD — what the store holds and whether the sources are still answering.
//
// Two honesty rules shape this file:
//
//   * An indicator only animates when the underlying number really changed.
//     A dashboard that pulses on every poll teaches people to ignore it, and
//     then it stops working as an alarm.
//   * Source health is derived from stored timestamps, and the thresholds that
//     turn a timestamp into a colour are printed next to the legend rather
//     than hidden in this code.

import { el, fill, byId, notice } from '../dom.js';
import * as api from '../api.js';
import { age, ageMs, bytes, count, stamp } from '../format.js';

const REFRESH_MS = 15000;

// A source that is polled and has not succeeded within the hour is stale.
// Every enabled source in the registry polls far more often than that, so this
// is a deliberately forgiving line rather than a per-source calculation the
// registry does not expose.
const STALE_MS = 60 * 60 * 1000;

let timer = 0;
let paused = false;
let previous = {};

export function mount() {
  byId('dash-pause').addEventListener('click', togglePause);
  refresh();
  schedule();
}

export function unmount() {
  window.clearInterval(timer);
  timer = 0;
  // Forget the last numbers, so a remount is not read as a change.
  previous = {};
  byId('dash-pause').removeEventListener('click', togglePause);
}

function schedule() {
  window.clearInterval(timer);
  timer = window.setInterval(() => {
    if (!paused) refresh();
  }, REFRESH_MS);
}

function togglePause() {
  paused = !paused;
  const button = byId('dash-pause');
  button.setAttribute('aria-pressed', String(paused));
  button.textContent = paused ? 'RESUME' : 'PAUSE';
  if (!paused) refresh();
}

async function refresh() {
  try {
    const [health, stats, sources] = await Promise.all([
      api.get('/health'),
      api.get('/v1/stats').catch(emptyStats),
      api.get('/v1/sources').catch(() => []),
    ]);

    notice(byId('dash-error'), '', 'error');
    renderCounters(health, stats);
    renderBars('bars-topic', stats.by_topic);
    renderBars('bars-source', stats.by_source);
    renderBars('bars-kind', stats.by_kind);
    renderHealth(api.listFrom(sources, 'sources'));
    renderWindow(stats);
    byId('dash-stamp').textContent = stamp(new Date());
  } catch (err) {
    notice(byId('dash-error'), describe(err), 'error');
  }
}

function emptyStats() {
  return { by_topic: {}, by_source: {}, by_kind: {} };
}

function describe(err) {
  if (err instanceof api.ApiError) {
    if (err.status === 401) return 'The API refused the request: a token is required.';
    if (err.status === 0) return `The API is unreachable: ${err.detail}`;
    return `The API answered ${err.status}: ${err.detail || err.code || 'no detail'}`;
  }
  return String(err && err.message ? err.message : err);
}

// ---------- counters ----------

function renderCounters(health, stats) {
  const cards = [
    ['RECORDS', count(health.records), 'observations stored'],
    ['ENTITIES', count(health.entities), 'things tracked'],
    ['SOURCES', count(health.sources), 'registry entries'],
    ['STORE', bytes(stats.store_bytes), 'on disk'],
    ['UPTIME', uptime(health.uptime_s), 'this process'],
  ];

  fill(byId('dash-counters'), cards.map(([label, value, note]) => {
    const number = el('div', { class: 'counter-value', text: value });
    // Only flash when the value moved. Silence is information too.
    if (previous[label] !== undefined && previous[label] !== value) number.classList.add('changed');
    previous[label] = value;

    return el('div', { class: 'counter' },
      el('div', { class: 'counter-label', text: label }),
      number,
      el('div', { class: 'counter-note', text: note }));
  }));
}

function uptime(seconds) {
  const s = Number(seconds);
  if (!Number.isFinite(s)) return '—';
  if (s < 3600) return `${Math.floor(s / 60)}m`;
  if (s < 86400) return `${Math.floor(s / 3600)}h`;
  return `${Math.floor(s / 86400)}d`;
}

// ---------- bars ----------

function renderBars(nodeId, buckets) {
  const node = byId(nodeId);
  const entries = Object.entries(buckets || {})
    .filter(([, value]) => Number.isFinite(Number(value)))
    .sort((a, b) => Number(b[1]) - Number(a[1]))
    .slice(0, 14);

  if (entries.length === 0) {
    fill(node, el('p', { class: 'hint', text: 'no counts reported' }));
    return;
  }

  const max = Math.max(...entries.map(([, value]) => Number(value)));
  fill(node, entries.map(([name, value]) => {
    const fillBar = el('div', { class: 'bar-fill' });
    // Set through the CSSOM, not a style attribute: the policy forbids one and
    // permits the other, and the difference is not cosmetic.
    fillBar.style.width = `${max > 0 ? (Number(value) / max) * 100 : 0}%`;

    return el('div', { class: 'bar-row' },
      el('div', { class: 'bar-name', title: name, text: name }),
      el('div', { class: 'bar-track' }, fillBar),
      el('div', { class: 'bar-count', text: count(value) }));
  }));
}

// ---------- source health ----------

/**
 * Turn stored health into a colour.
 *
 * Fails towards amber rather than green: a source that has never reported is
 * not healthy, it is unproven.
 */
export function healthOf(source, now = Date.now()) {
  const automated = String(source.automation || '') === 'enabled';
  if (!automated) return { dot: 'dot-off', why: 'not automated' };
  if (Number(source.consecutive_errors || 0) > 0 || source.last_error) {
    return { dot: 'dot-bad', why: 'last attempt failed' };
  }
  if (!source.last_success) return { dot: 'dot-warn', why: 'never fetched' };
  if (ageMs(source.last_success, now) > STALE_MS) return { dot: 'dot-warn', why: 'stale' };
  return { dot: 'dot-ok', why: 'fresh' };
}

function renderHealth(sources) {
  const body = byId('health-body');
  if (sources.length === 0) {
    fill(body, el('tr', {}, el('td', { colSpan: 6, class: 'hint', text: 'the registry returned nothing' })));
    return;
  }

  const rows = sources
    .slice()
    .sort((a, b) => String(a.id).localeCompare(String(b.id)))
    .map((source) => {
      const state = healthOf(source);
      return el('tr', {},
        el('td', { class: 'nowrap' },
          el('span', { class: `dot ${state.dot}`, title: state.why }), ' ',
          el('span', { text: source.id })),
        el('td', { class: 'nowrap', text: source.automation || '—' }),
        el('td', { class: 'nowrap', title: stamp(source.last_success), text: age(source.last_success) }),
        el('td', { class: 'num', text: count(source.records ?? 0) }),
        el('td', { class: 'num', text: count(source.consecutive_errors ?? 0) }),
        el('td', { text: source.last_error || '—' }));
    });

  fill(body, rows);
}

function renderWindow(stats) {
  fill(byId('dash-window'),
    el('dt', { text: 'OLDEST OBSERVATION' }), el('dd', { text: stats.oldest ? stamp(stats.oldest) : '—' }),
    el('dt', { text: 'NEWEST OBSERVATION' }), el('dd', { text: stats.newest ? stamp(stats.newest) : '—' }),
    el('dt', { text: 'STORE SIZE' }), el('dd', { text: bytes(stats.store_bytes) }),
    el('dt', { text: 'REFRESH' }), el('dd', { text: `every ${REFRESH_MS / 1000}s while this view is open` }));
}
