// MAP — the viewport is the query.
//
// Every pan sends a bbox and gets back only what is inside it, because a map
// that downloads the whole store to draw a street corner stops being usable at
// exactly the moment the store becomes interesting.
//
// Clicking a feature opens the record with its provenance attached. GeoJSON
// properties carry a summary; the publisher, the source URL and fetched_at
// live on the record itself, so the panel goes and gets it. When it cannot,
// it says so rather than leaving the fields blank and hoping.

import { el, fill, byId, notice, debounce } from '../dom.js';
import * as api from '../api.js';
import * as basemap from '../basemap.js';
import { provenanceBlock, severityLabel, stamp, ellipsis } from '../format.js';
import { jsonBlock } from '../highlight.js';

// A viewport holds a few hundred readable things. Past that the honest move is
// to say the view is truncated, not to draw ten thousand overlapping dots.
const FEATURE_LIMIT = 600;

const GLYPHS = {
  incident: '▲',
  roadwork: '■',
  camera: '◉',
  bus_stop: '●',
  bus_line: '━',
  train: '▬',
  aircraft: '▶',
  river_level: '▼',
  gauge: '▼',
  weather: '☁',
  fire: '◆',
  thermal_anomaly: '◆',
  air_quality: '○',
  event: '★',
};

let map = null;
let layers = new Map();      // kind -> { group, colour, count, on }
let hidden = new Set();      // kinds the operator switched off
let iconCache = new Map();
let recordCache = new Map(); // "source|kind" -> Promise<Map<id, record>>
let refresh = null;
let inFlight = null;

export function mount(section) {
  const canvas = byId('map-canvas');

  if (!basemap.available()) {
    fill(canvas, basemap.unavailableNotice());
    return;
  }

  if (!map) {
    map = basemap.createMap(canvas);
    refresh = debounce(load, 350);
    map.on('moveend zoomend', () => refresh());
    byId('map-show-entities').addEventListener('change', () => load());
  }

  // Leaflet measured a hidden container if the section was closed; ask it to
  // measure again now that it is on screen.
  window.setTimeout(() => {
    map.invalidateSize();
    load();
  }, 0);

  section.dataset.mounted = 'true';
}

export function unmount() {
  if (refresh) refresh.cancel();
  if (inFlight) inFlight.abort();
  inFlight = null;
}

// ---------- loading ----------

async function load() {
  if (!map) return;

  if (inFlight) inFlight.abort();
  inFlight = new AbortController();
  const signal = inFlight.signal;

  const bbox = basemap.bboxOf(map);
  const wantEntities = byId('map-show-entities').checked;
  byId('map-stamp').textContent = 'loading…';

  try {
    const requests = [api.get('/v1/geojson', { bbox, limit: FEATURE_LIMIT }, { signal })];
    if (wantEntities) {
      requests.push(api.get('/v1/entities.geojson', { bbox, limit: FEATURE_LIMIT }, { signal }));
    }

    const collections = await Promise.all(requests);
    notice(byId('map-error'), '', 'error');

    const features = [];
    collections.forEach((collection, index) => {
      for (const feature of (collection && collection.features) || []) {
        feature.__dataset = index === 0 ? 'records' : 'entities';
        features.push(feature);
      }
    });

    draw(features);

    const truncated = collections.some((c) => ((c && c.features) || []).length >= FEATURE_LIMIT);
    byId('map-stamp').textContent =
      `${features.length} features${truncated ? ` (capped at ${FEATURE_LIMIT} per layer)` : ''} · ${stamp(new Date())}`;
  } catch (err) {
    if (err && err.name === 'AbortError') return;
    byId('map-stamp').textContent = 'failed';
    notice(byId('map-error'), describe(err), 'error');
  }
}

function describe(err) {
  if (err instanceof api.ApiError) {
    if (err.status === 401) return 'The API refused the request: a token is required.';
    return `The API answered ${err.status}: ${err.detail || err.code || 'no detail'}`;
  }
  return String(err && err.message ? err.message : err);
}

// ---------- drawing ----------

function draw(features) {
  for (const layer of layers.values()) map.removeLayer(layer.group);
  layers = new Map();

  for (const feature of features) {
    const point = pointOf(feature);
    if (!point) continue;

    const props = feature.properties || {};
    const kind = String(props.kind || 'unknown');
    const layer = layerFor(kind);
    layer.count += 1;

    const marker = window.L.marker(point, { icon: iconFor(kind), keyboard: false, riseOnHover: true });
    // Colour through the CSSOM. A style attribute would need 'unsafe-inline'.
    marker.on('add', () => {
      const node = marker.getElement();
      if (!node) return;
      node.style.color = basemap.colourFor(kind);
      node.style.textShadow = `0 0 7px ${basemap.colourFor(kind)}`;
    });

    marker.bindTooltip(ellipsis(props.title || props.id || kind, 70), { direction: 'top', opacity: 0.9 });
    marker.on('click', () => showFeature(feature));
    layer.group.addLayer(marker);
  }

  for (const [kind, layer] of layers) {
    if (!hidden.has(kind)) layer.group.addTo(map);
  }
  renderLayerSwitcher();
}

function layerFor(kind) {
  let layer = layers.get(kind);
  if (!layer) {
    layer = { group: window.L.layerGroup(), colour: basemap.colourFor(kind), count: 0 };
    layers.set(kind, layer);
  }
  return layer;
}

function iconFor(kind) {
  if (!iconCache.has(kind)) {
    const glyph = GLYPHS[String(kind).toLowerCase()] || '●';
    iconCache.set(kind, window.L.divIcon({
      className: 'eye-glyph',
      // Constant markup from the table above; no data is interpolated here.
      html: `<span class="glyph">${glyph}</span>`,
      iconSize: [18, 18],
      iconAnchor: [9, 9],
    }));
  }
  return iconCache.get(kind);
}

function pointOf(feature) {
  const g = feature && feature.geometry;
  if (!g) return null;
  if (g.type === 'Point' && Array.isArray(g.coordinates)) {
    const [lon, lat] = g.coordinates;
    return Number.isFinite(lat) && Number.isFinite(lon) ? [lat, lon] : null;
  }
  // Anything with an extent is represented by its centre for now; the record
  // panel still shows the real geometry.
  try {
    const bounds = window.L.geoJSON(g).getBounds();
    return bounds.isValid() ? bounds.getCenter() : null;
  } catch {
    return null;
  }
}

function renderLayerSwitcher() {
  const node = byId('map-layers');
  if (layers.size === 0) {
    fill(node, el('p', { class: 'hint', text: 'nothing in this viewport' }));
    return;
  }

  const rows = [...layers.entries()]
    .sort((a, b) => b[1].count - a[1].count)
    .map(([kind, layer]) => {
      const box = el('input', { type: 'checkbox', checked: !hidden.has(kind) });
      box.addEventListener('change', () => {
        if (box.checked) {
          hidden.delete(kind);
          layer.group.addTo(map);
        } else {
          hidden.add(kind);
          map.removeLayer(layer.group);
        }
      });

      return el('label', { class: 'layer-row' },
        box,
        basemap.swatch(kind),
        el('span', { text: kind }),
        el('span', { class: 'layer-count', text: String(layer.count) }));
    });

  fill(node, rows);
}

// ---------- feature panel ----------

async function showFeature(feature) {
  const props = feature.properties || {};
  const panel = byId('map-feature');

  const summary = el('dl', { class: 'kv' });
  for (const [term, value] of [
    ['ID', props.id],
    ['SOURCE', props.source],
    ['KIND', props.kind],
    ['TOPIC', props.topic],
    ['SEVERITY', props.severity === undefined ? undefined : severityLabel(props.severity)],
    ['OBSERVED AT', props.observed_at ? stamp(props.observed_at) : undefined],
    ['LICENCE', props.license],
  ]) {
    if (value === undefined || value === null || value === '') continue;
    summary.append(el('dt', { text: term }), el('dd', { text: String(value) }));
  }

  // eye stores camera metadata and never a frame, so there is nothing here to
  // display even when the record is about a camera. Saying so is cheaper than
  // letting somebody wonder where the picture went.
  const cameraNote = String(props.kind || '') === 'camera'
    ? el('p', { class: 'hint', text: 'Camera metadata only: eye never stores, displays or proxies camera images.' })
    : null;

  fill(panel,
    el('h3', { class: 'feature-title', text: props.title || props.id || 'feature' }),
    summary,
    cameraNote,
    el('p', { class: 'hint', text: 'fetching the full record…' }));

  const record = await lookupRecord(feature).catch(() => null);

  const tail = record
    ? [
      provenanceBlock(record),
      el('details', {}, el('summary', { text: 'raw record' }), jsonBlock(record)),
    ]
    : [
      el('div', { class: 'notice' },
        el('h3', { text: 'PARTIAL PROVENANCE' }),
        el('p', {
          text: 'Only the map summary is available for this feature: the full record was not '
            + 'found in the /v1 window this panel queried. Publisher, source URL and fetched_at '
            + 'are shown blank rather than guessed.',
        })),
      el('details', {}, el('summary', { text: 'raw feature' }), jsonBlock(feature)),
    ];

  fill(panel,
    el('h3', { class: 'feature-title', text: props.title || props.id || 'feature' }),
    summary,
    cameraNote,
    ...tail);
}

/**
 * Fetch the record behind a feature.
 *
 * The contract has no lookup by id, so this pulls the feature's own source and
 * kind and indexes them once. It is a cache keyed on the pair, which keeps a
 * busy viewport to one request per layer rather than one per click.
 */
function lookupRecord(feature) {
  const props = feature.properties || {};
  if (!props.id || !props.source) return Promise.resolve(null);

  const dataset = feature.__dataset === 'entities' ? 'entities' : 'records';
  const key = `${dataset}|${props.source}|${props.kind || ''}`;

  if (!recordCache.has(key)) {
    const path = dataset === 'entities' ? '/v1/entities' : '/v1/records';
    const promise = api.get(path, { source: props.source, kind: props.kind, limit: 1000 })
      .then((payload) => {
        const index = new Map();
        for (const item of api.listFrom(payload, dataset)) index.set(item.id, item);
        return index;
      })
      .catch(() => new Map());
    recordCache.set(key, promise);
    // The index is a snapshot; drop it so a long session does not serve
    // provenance from an hour ago.
    window.setTimeout(() => recordCache.delete(key), 120000);
  }

  return recordCache.get(key).then((index) => index.get(props.id) || null);
}
