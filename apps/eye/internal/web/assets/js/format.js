// Formatting helpers.
//
// The rule this file exists to enforce: a timestamp is never shown alone.
// eye keeps observed_at and fetched_at apart on purpose, and their difference
// is the source latency. Collapsing them would let the console sell
// thirteen-minute-old data as "live".

import { el } from './dom.js';

const SEVERITY = ['none', 'info', 'low', 'moderate', 'high', 'critical'];

/** Absolute UTC stamp, always to the second, never localised away. */
export function stamp(value) {
  const date = toDate(value);
  if (!date) return '—';
  return date.toISOString().replace('T', ' ').slice(0, 19) + 'Z';
}

/** Short UTC clock, for boards where the date is implied. */
export function clock(value) {
  const date = toDate(value);
  return date ? date.toISOString().slice(11, 16) : '—';
}

/** Relative age of a timestamp, phrased as elapsed time. */
export function age(value, now = Date.now()) {
  const date = toDate(value);
  if (!date) return 'never';
  return duration(now - date.getTime()) + ' ago';
}

/** Signed, human duration for a millisecond span. */
export function duration(ms) {
  if (!Number.isFinite(ms)) return '—';
  const sign = ms < 0 ? '-' : '';
  const s = Math.round(Math.abs(ms) / 1000);
  if (s < 60) return `${sign}${s}s`;
  if (s < 3600) return `${sign}${Math.floor(s / 60)}m ${s % 60}s`;
  if (s < 86400) return `${sign}${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`;
  return `${sign}${Math.floor(s / 86400)}d ${Math.floor((s % 86400) / 3600)}h`;
}

/**
 * Source latency: how stale the observation already was when eye fetched it.
 * A negative value means the source dated it in the future, which is worth
 * showing rather than clamping away.
 */
export function latency(observedAt, fetchedAt) {
  const observed = toDate(observedAt);
  const fetched = toDate(fetchedAt);
  if (!observed || !fetched) return '—';
  return duration(fetched.getTime() - observed.getTime());
}

/** Milliseconds since a timestamp, or Infinity when there is none. */
export function ageMs(value, now = Date.now()) {
  const date = toDate(value);
  return date ? now - date.getTime() : Infinity;
}

export function severityLabel(value) {
  const n = Number(value);
  return Number.isInteger(n) && SEVERITY[n] ? `${n} ${SEVERITY[n]}` : String(value ?? '—');
}

/** Thousands separator without pulling in Intl formatting surprises. */
export function count(value) {
  const n = Number(value);
  return Number.isFinite(n) ? n.toLocaleString('en-US') : '—';
}

export function bytes(value) {
  const n = Number(value);
  if (!Number.isFinite(n) || n < 0) return '—';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let size = n;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return `${unit === 0 ? size : size.toFixed(1)} ${units[unit]}`;
}

/** Trim a string for a cell without pretending the rest does not exist. */
export function ellipsis(value, max = 120) {
  const text = String(value ?? '');
  return text.length > max ? `${text.slice(0, max - 1)}…` : text;
}

function toDate(value) {
  if (!value) return null;
  const date = value instanceof Date ? value : new Date(value);
  return Number.isNaN(date.getTime()) ? null : date;
}

/**
 * The provenance block: publisher, licence, source URL, and both timestamps
 * with the gap between them spelled out.
 *
 * Nothing in this console renders a value without one of these nearby. That is
 * the whole reason eye exists, and it is not a place to save vertical space.
 *
 * @param {Object} item a record or an entity
 * @returns {HTMLElement}
 */
export function provenanceBlock(item) {
  const p = (item && item.provenance) || {};
  const observed = item && (item.observed_at || item.first_seen);
  const fetched = (item && item.fetched_at) || p.fetched_at || (item && item.last_seen);

  const rows = [
    ['PUBLISHER', p.publisher || 'not stated'],
    ['LICENCE', p.license || 'unspecified'],
    ['OBSERVED AT', stamp(observed)],
    ['FETCHED AT', stamp(fetched)],
    ['SOURCE LATENCY', latency(observed, fetched)],
  ];

  const list = el('dl', { class: 'kv' });
  for (const [term, value] of rows) {
    list.append(el('dt', { text: term }), el('dd', { text: value }));
  }

  list.append(el('dt', { text: 'SOURCE URL' }));
  list.append(el('dd', {}, p.source_url
    ? el('a', { href: p.source_url, target: '_blank', rel: 'noreferrer noopener', text: p.source_url })
    : 'not stated'));

  if (p.raw_hash) {
    list.append(el('dt', { text: 'RAW SHA-256' }), el('dd', { text: p.raw_hash }));
  }

  return el('div', { class: 'prov' }, el('h3', { text: 'PROVENANCE' }), list);
}
