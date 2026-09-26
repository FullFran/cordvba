// Shared Leaflet setup.
//
// Leaflet arrives from cdnjs as a classic script with subresource integrity, so
// it is a global by the time these modules run. If the CDN is blocked the map
// sections say so instead of throwing: a console that shows a blank rectangle
// and no explanation is worse than one that admits what is missing.

import { el } from './dom.js';

/** Córdoba, roughly the Mezquita. */
export const CORDOBA = [37.8794, -4.7794];

// Standard OpenStreetMap tiles, darkened in CSS rather than fetched dark.
//
// CARTO's keyless dark_all endpoint now stamps every tile with "API KEY
// REQUIRED" — verified by downloading one and looking at it — and eye is not
// going to render somebody else's watermark and call it a basemap. OSM's own
// tiles need no key, and `.leaflet-tile-pane` inverts them into the dark
// palette the rest of this console uses.
//
// OSM's tile usage policy is written for exactly this: a personal deployment
// with honest attribution and no bulk downloading. Respect it.
const TILE_URL = 'https://tile.openstreetmap.org/{z}/{x}/{y}.png';
const TILE_ATTRIBUTION =
  '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors';

/** Whether the Leaflet script actually loaded. */
export function available() {
  return typeof window.L !== 'undefined';
}

/** A stand-in panel for when it did not. */
export function unavailableNotice() {
  return el('div', { class: 'notice notice-error' },
    el('h3', { text: 'MAP LIBRARY NOT LOADED' }),
    el('p', {
      text: 'Leaflet is served from cdnjs.cloudflare.com and did not load. '
        + 'Everything else on this console works offline; only the map needs the CDN.',
    }));
}

/**
 * Create a dark operations map. Attribution is not optional: the tiles are
 * OpenStreetMap's own rendering of OpenStreetMap data, credited by its terms.
 *
 * @param {HTMLElement} container
 * @param {{center?: number[], zoom?: number}} [options]
 */
export function createMap(container, options = {}) {
  const map = window.L.map(container, {
    center: options.center || CORDOBA,
    zoom: options.zoom || 13,
    zoomControl: true,
    preferCanvas: false,
    attributionControl: true,
  });

  window.L.tileLayer(TILE_URL, {
    attribution: TILE_ATTRIBUTION,
    maxZoom: 19,
    // No detectRetina: it doubles the tile requests, and OSM's usage policy
    // is a courtesy this project intends to keep.
  }).addTo(map);

  return map;
}

/** The bbox query parameter for the current viewport, in the API's order. */
export function bboxOf(map) {
  const b = map.getBounds();
  return [b.getWest(), b.getSouth(), b.getEast(), b.getNorth()]
    .map((n) => n.toFixed(5))
    .join(',');
}

// Kinds eye publishes often enough to deserve a colour somebody can learn.
const KIND_COLOURS = {
  incident: '#ff4646',
  roadwork: '#ffb000',
  traffic: '#ffb000',
  camera: '#7a8f86',
  bus_stop: '#00ff6e',
  bus_line: '#00d0ff',
  bus_arrival: '#00ff6e',
  train: '#b98cff',
  train_departure: '#b98cff',
  aircraft: '#35d7ff',
  river_level: '#35d7ff',
  gauge: '#35d7ff',
  weather: '#8fd9ff',
  fire: '#ff6a2b',
  thermal_anomaly: '#ff6a2b',
  air_quality: '#c8ff5a',
  event: '#ff8ad4',
  dataset: '#9aa8a2',
};

const FALLBACK_COLOURS = ['#00ff6e', '#35d7ff', '#ffb000', '#b98cff', '#ff8ad4', '#c8ff5a', '#ff6a2b', '#8fd9ff'];

/** A stable colour per kind, so the same layer looks the same on every load. */
export function colourFor(kind) {
  const key = String(kind || 'unknown').toLowerCase();
  if (KIND_COLOURS[key]) return KIND_COLOURS[key];

  let hash = 0;
  for (let i = 0; i < key.length; i += 1) hash = (hash * 31 + key.charCodeAt(i)) >>> 0;
  return FALLBACK_COLOURS[hash % FALLBACK_COLOURS.length];
}

/**
 * Marker style. Severity drives the radius, so a critical incident is bigger
 * than a routine reading without changing what colour means.
 */
export function markerStyle(kind, severity) {
  const colour = colourFor(kind);
  const sev = Number(severity);
  return {
    radius: 4 + (Number.isFinite(sev) ? Math.max(0, Math.min(5, sev)) : 0),
    color: colour,
    weight: 1.5,
    opacity: 0.95,
    fillColor: colour,
    fillOpacity: 0.35,
  };
}

/** A small round swatch for a legend row. */
export function swatch(kind) {
  const node = el('span', { class: 'layer-swatch' });
  node.style.background = colourFor(kind);
  node.style.boxShadow = `0 0 6px ${colourFor(kind)}`;
  return node;
}
