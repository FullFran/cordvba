// SOURCES — the registry as a page.
//
// The notes field is the reason this view exists. configs/sources.yaml is
// mostly reasoning: why a feed is not automated, which licence applies, what
// the publisher declared. Rendering the registry without its notes would turn
// an argument into a list of switches.

import { el, fill, byId, notice } from '../dom.js';
import * as api from '../api.js';
import { age, count, stamp } from '../format.js';
import { healthOf } from './dashboard.js';

let sources = [];
let wired = false;

export function mount() {
  if (!wired) {
    for (const id of ['s-topic', 's-automation']) byId(id).addEventListener('change', render);
    byId('s-text').addEventListener('input', render);
    wired = true;
  }
  load();
}

export function unmount() {
  // Static content; nothing to stop.
}

async function load() {
  try {
    const payload = await api.get('/v1/sources');
    sources = api.listFrom(payload, 'sources');
    notice(byId('sources-error'), '', 'error');
    byId('sources-stamp').textContent = `${sources.length} entries · loaded ${stamp(new Date())}`;
    fillFilters();
    render();
  } catch (err) {
    sources = [];
    fill(byId('sources-cards'));
    byId('sources-stamp').textContent = 'not loaded';
    notice(byId('sources-error'), describe(err), 'error');
  }
}

function describe(err) {
  if (err instanceof api.ApiError) {
    if (err.status === 401) return 'The API refused the request: a token is required.';
    if (err.status === 0) return `The API is unreachable: ${err.detail}`;
    return `The API answered ${err.status}: ${err.detail || err.code || 'no detail'}`;
  }
  return String(err && err.message ? err.message : err);
}

function fillFilters() {
  fillSelect('s-topic', unique(sources.map((s) => s.topic)));
  fillSelect('s-automation', unique(sources.map((s) => s.automation)));
}

function unique(values) {
  return [...new Set(values.filter(Boolean).map(String))].sort();
}

function fillSelect(id, values) {
  const select = byId(id);
  const chosen = select.value;
  fill(select, el('option', { value: '', text: 'all' }), values.map((v) => el('option', { value: v, text: v })));
  select.value = values.includes(chosen) ? chosen : '';
}

function render() {
  const topic = byId('s-topic').value;
  const automation = byId('s-automation').value;
  const text = byId('s-text').value.trim().toLowerCase();

  const shown = sources.filter((source) => {
    if (topic && source.topic !== topic) return false;
    if (automation && source.automation !== automation) return false;
    if (!text) return true;
    return [source.id, source.authority, source.notes, source.format, source.license, source.url]
      .filter(Boolean)
      .some((field) => String(field).toLowerCase().includes(text));
  });

  const node = byId('sources-cards');
  if (shown.length === 0) {
    fill(node, el('p', { class: 'hint', text: 'no source matches this filter' }));
    return;
  }

  fill(node, shown
    .slice()
    .sort((a, b) => String(a.id).localeCompare(String(b.id)))
    .map(card));
}

function card(source) {
  const state = healthOf(source);
  const personal = String(source.access || '') === 'undocumented_personal';

  const tags = el('div', {},
    tag(source.topic, 'tag'),
    tag(source.format, 'tag'),
    tag(source.access, personal ? 'tag tag-warn' : 'tag'),
    tag(source.automation, source.automation === 'enabled' ? 'tag tag-on' : 'tag tag-warn'),
    tag(source.pollable ? 'pollable' : 'not polled', source.pollable ? 'tag tag-on' : 'tag tag-off'),
    tag(source.license, source.license === 'unspecified' ? 'tag tag-warn' : 'tag'));

  const health = el('dl', { class: 'kv' },
    el('dt', { text: 'STATUS' }),
    el('dd', {}, el('span', { class: `dot ${state.dot}` }), ` ${state.why}`),
    el('dt', { text: 'LAST SUCCESS' }),
    el('dd', { title: stamp(source.last_success), text: age(source.last_success) }),
    el('dt', { text: 'RECORDS' }),
    el('dd', { text: count(source.records ?? 0) }),
    el('dt', { text: 'CONSECUTIVE ERRORS' }),
    el('dd', { text: count(source.consecutive_errors ?? 0) }));

  if (source.last_error) {
    health.append(el('dt', { text: 'LAST ERROR' }), el('dd', { text: String(source.last_error) }));
  }

  return el('div', { class: 'card' },
    el('h3', { text: source.id || 'unnamed source' }),
    el('p', { class: 'card-auth', text: source.authority || 'authority not stated' }),
    tags,
    source.url ? el('a', { href: source.url, target: '_blank', rel: 'noreferrer noopener', text: source.url }) : null,
    health,
    personal
      ? el('p', { class: 'card-notes', text: 'Personal source: read for the operator’s own use, never redistributed by eye serve.' })
      : null,
    source.notes ? el('p', { class: 'card-notes', text: source.notes }) : null);
}

function tag(value, className) {
  if (!value) return null;
  return el('span', { class: className, text: String(value) });
}
