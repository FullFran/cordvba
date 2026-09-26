// RECORDS — the query explorer.
//
// The console is not a privileged client, and this view is where that stops
// being a claim: "copy as curl" prints the exact request that produced what is
// on screen, so anything here can be reproduced from a shell. The downloads are
// built from the rows already in the browser, so a CSV is the same data and not
// a second, differently-filtered query.

import { el, fill, byId, notice, copyText, download } from '../dom.js';
import * as api from '../api.js';
import { ellipsis, latency, provenanceBlock, severityLabel, stamp } from '../format.js';
import { jsonBlock } from '../highlight.js';

// Rows are rendered in chunks and extended on scroll. The store can answer with
// thousands of records and a table with thousands of live rows in it stops
// scrolling smoothly long before it stops being useful.
const CHUNK = 100;

const FIELDS = {
  topic: 'f-topic',
  source: 'f-source',
  kind: 'f-kind',
  text: 'f-text',
  since: 'f-since',
  until: 'f-until',
  min_severity: 'f-min-severity',
  bbox: 'f-bbox',
  near: 'f-near',
  radius_km: 'f-radius',
  limit: 'f-limit',
};

let rows = [];
let rendered = 0;
let wired = false;

export function mount() {
  if (wired) return;
  wired = true;

  byId('records-run').addEventListener('click', run);
  byId('records-reset').addEventListener('click', reset);
  byId('records-curl').addEventListener('click', copyCurl);
  byId('records-csv').addEventListener('click', () => downloadCsv());
  byId('records-geojson').addEventListener('click', () => downloadGeojson());

  for (const id of Object.values(FIELDS)) {
    byId(id).addEventListener('keydown', (event) => {
      if (event.key === 'Enter') run();
    });
  }

  byId('records-scroll').addEventListener('scroll', onScroll);
}

export function unmount() {
  // Nothing to tear down: this view holds no timers and no live connections.
}

function params() {
  const out = {};
  for (const [name, id] of Object.entries(FIELDS)) {
    const value = byId(id).value.trim();
    if (value) out[name] = value;
  }
  return out;
}

function dataset() {
  return byId('f-dataset').value === 'entities' ? 'entities' : 'records';
}

function path() {
  return dataset() === 'entities' ? '/v1/entities' : '/v1/records';
}

async function run() {
  const stampNode = byId('records-stamp');
  stampNode.textContent = 'running…';

  try {
    const payload = await api.get(path(), params());
    rows = api.listFrom(payload, dataset());
    notice(byId('records-error'), '', 'error');

    byId('records-count').textContent = `${rows.length} rows`;
    stampNode.textContent = `ran ${stamp(new Date())}`;
    renderFirstChunk();
  } catch (err) {
    rows = [];
    renderFirstChunk();
    byId('records-count').textContent = '';
    stampNode.textContent = 'failed';
    notice(byId('records-error'), describe(err), 'error');
  }
}

function reset() {
  for (const id of Object.values(FIELDS)) byId(id).value = '';
  byId('f-limit').value = '200';
  byId('f-min-severity').value = '';
  rows = [];
  renderFirstChunk();
  byId('records-count').textContent = '';
  byId('records-stamp').textContent = 'not run';
  notice(byId('records-error'), '', 'error');
}

function describe(err) {
  if (err instanceof api.ApiError) {
    if (err.status === 401) return 'The API refused the request: a token is required.';
    if (err.status === 400) return `The API rejected the filters: ${err.detail || err.code}`;
    if (err.status === 0) return `The API is unreachable: ${err.detail}`;
    return `The API answered ${err.status}: ${err.detail || err.code || 'no detail'}`;
  }
  return String(err && err.message ? err.message : err);
}

// ---------- table ----------

function renderFirstChunk() {
  rendered = 0;
  fill(byId('records-body'));
  byId('records-scroll').scrollTop = 0;

  if (rows.length === 0) {
    fill(byId('records-body'), el('tr', {},
      el('td', { colSpan: 7, class: 'hint', text: 'no rows' })));
    byId('records-more').hidden = true;
    return;
  }
  appendChunk();
}

function onScroll() {
  const node = byId('records-scroll');
  if (node.scrollTop + node.clientHeight >= node.scrollHeight - 120) appendChunk();
}

function appendChunk() {
  if (rendered >= rows.length) {
    byId('records-more').hidden = true;
    return;
  }

  const body = byId('records-body');
  const next = rows.slice(rendered, rendered + CHUNK);
  for (const item of next) body.append(...rowFor(item));
  rendered += next.length;

  const more = byId('records-more');
  more.hidden = rendered >= rows.length;
  more.textContent = `${rendered} of ${rows.length} rendered — scroll for more`;
}

function rowFor(item) {
  const observed = item.observed_at || item.first_seen;
  const fetched = item.fetched_at || (item.provenance && item.provenance.fetched_at) || item.last_seen;

  const detail = el('tr', { hidden: true },
    el('td', { colSpan: 7, class: 'raw' },
      el('div', {}, provenanceBlock(item)),
      jsonBlock(item)));

  const head = el('tr', { class: 'row-head' },
    el('td', { class: 'nowrap', text: stamp(observed) }),
    el('td', { class: 'nowrap', text: latency(observed, fetched) }),
    el('td', { class: 'nowrap', text: String(item.source || '—') }),
    el('td', { class: 'nowrap', text: String(item.kind || '—') }),
    el('td', { class: 'nowrap', text: String(item.topic || '—') }),
    el('td', { class: 'nowrap', text: item.severity === undefined ? '—' : severityLabel(item.severity) }),
    el('td', { text: ellipsis(item.title || item.id || '', 140) }));

  head.addEventListener('click', () => {
    detail.hidden = !detail.hidden;
    head.classList.toggle('is-open', !detail.hidden);
  });

  return [head, detail];
}

// ---------- exports ----------

async function copyCurl() {
  const command = api.curlFor(path(), params());
  const ok = await copyText(command).catch(() => false);
  byId('records-hint').textContent = ok
    ? 'Copied. The token is referenced as $EYE_API_TOKEN rather than pasted into the command.'
    : `Copy failed; here it is: ${command}`;
}

function downloadCsv() {
  if (rows.length === 0) {
    notice(byId('records-error'), 'Run a query first: the download is built from the rows on screen.', 'error');
    return;
  }

  const columns = ['id', 'source', 'kind', 'topic', 'observed_at', 'fetched_at', 'severity', 'quality',
    'title', 'lat', 'lon', 'publisher', 'license', 'source_url'];

  const lines = [columns.join(',')];
  for (const item of rows) {
    const p = item.provenance || {};
    lines.push(columns.map((column) => csvCell(valueFor(item, p, column))).join(','));
  }

  download(`eye-${dataset()}-${fileStamp()}.csv`, 'text/csv;charset=utf-8', `${lines.join('\r\n')}\r\n`);
}

function valueFor(item, provenance, column) {
  switch (column) {
    case 'lat': return item.position ? item.position.lat : '';
    case 'lon': return item.position ? item.position.lon : '';
    case 'publisher': return provenance.publisher || '';
    case 'license': return provenance.license || '';
    case 'source_url': return provenance.source_url || '';
    case 'fetched_at': return item.fetched_at || provenance.fetched_at || item.last_seen || '';
    case 'observed_at': return item.observed_at || item.first_seen || '';
    default: return item[column] === undefined || item[column] === null ? '' : item[column];
  }
}

function csvCell(value) {
  const text = String(value);
  return /[",\r\n]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text;
}

function downloadGeojson() {
  if (rows.length === 0) {
    notice(byId('records-error'), 'Run a query first: the download is built from the rows on screen.', 'error');
    return;
  }

  const features = [];
  let withoutGeometry = 0;

  for (const item of rows) {
    const geometry = item.geometry
      || (item.position ? { type: 'Point', coordinates: [item.position.lon, item.position.lat] } : null);
    if (!geometry) {
      withoutGeometry += 1;
      continue;
    }

    features.push({
      type: 'Feature',
      id: item.id,
      geometry,
      properties: {
        id: item.id,
        source: item.source,
        kind: item.kind,
        topic: item.topic,
        title: item.title,
        severity: item.severity,
        observed_at: item.observed_at || item.first_seen,
        fetched_at: item.fetched_at || (item.provenance && item.provenance.fetched_at) || item.last_seen,
        provenance: item.provenance,
      },
    });
  }

  const collection = {
    type: 'FeatureCollection',
    features,
    // The file says what it left out. A GeoJSON that quietly drops the rows
    // with no position is a file that misreports the size of the result.
    eye: {
      exported_at: new Date().toISOString(),
      dataset: dataset(),
      rows_in_result: rows.length,
      rows_without_geometry: withoutGeometry,
      request: api.urlFor(path(), params()).toString(),
    },
  };

  download(`eye-${dataset()}-${fileStamp()}.geojson`, 'application/geo+json', JSON.stringify(collection, null, 2));

  if (withoutGeometry > 0) {
    byId('records-hint').textContent =
      `${withoutGeometry} of ${rows.length} rows have no position and are not in the GeoJSON; the count is recorded inside the file.`;
  }
}

function fileStamp() {
  return new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
}
