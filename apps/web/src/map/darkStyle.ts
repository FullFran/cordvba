import { invertColorLightness, isColorString } from "./color";

/**
 * A loose MapLibre style-spec shape: only what this module reads or
 * writes. Deliberately not the full `@maplibre/style-spec` type — every
 * layer here is walked generically (see `deepRecolor`), so a narrower,
 * hand-rolled type is both enough and easier to unit test against a small
 * fixture instead of a real 111-layer style.
 */
export interface MapStyleLayer {
  id: string;
  type: string;
  paint?: Record<string, unknown>;
  [key: string]: unknown;
}

export interface MapStyle {
  version: number;
  sources: Record<string, unknown>;
  layers: MapStyleLayer[];
  [key: string]: unknown;
}

/**
 * The handful of colours this app chooses deliberately rather than by
 * generic inversion (issue #119). Values are read at runtime from the
 * `--map-*` custom properties in `src/styles/tokens.css` via
 * `readMapPalette`, so this module itself never contains a literal colour
 * — tokens.css stays the one source of truth.
 */
export interface MapPalette {
  background: string;
  water: string;
  buildingLow: string;
  buildingHigh: string;
  labelHalo: string;
}

const OVERRIDE_IDS = new Set(["background", "water", "building-3d", "label_city"]);

/**
 * Basemap layers hidden outright (issue #119, maintainer review: "the
 * data must be the only saturated elements"): POI icons/labels (cafés,
 * museums, "camera" tourist-attraction icons — the things that made
 * "Patios de Córdoba" and a camera glyph compete with the beacons),
 * transit stop icons, minor road-name labels and one-way arrows. District/
 * city/state/country labels, the river and major-road names stay (see
 * `MUTE_IDS`).
 */
const SUPPRESS_IDS = new Set([
  "poi_r20",
  "poi_r7",
  "poi_r1",
  "poi_transit",
  "label_village",
  "label_other",
  "highway-name-path",
  "highway-name-minor",
  "road_one_way_arrow",
  "road_one_way_arrow_opposite",
]);

/** Labels kept, but at a low, non-competing opacity: landmark/major labels the reviewer asked to keep "at low contrast". */
const MUTE_IDS = new Set(["highway-name-major", "airport", "label_town"]);
const MUTE_OPACITY = 0.55;

function deepRecolor(value: unknown): unknown {
  if (typeof value === "string") {
    return isColorString(value) ? invertColorLightness(value) : value;
  }
  if (Array.isArray(value)) {
    return value.map(deepRecolor);
  }
  if (value !== null && typeof value === "object") {
    return Object.fromEntries(Object.entries(value).map(([k, v]) => [k, deepRecolor(v)]));
  }
  return value;
}

function overridePaint(layer: MapStyleLayer, palette: MapPalette): Record<string, unknown> | undefined {
  switch (layer.id) {
    case "background":
      return { "background-color": palette.background };
    case "water":
      return { "fill-color": palette.water };
    case "building-3d":
      return {
        // Real per-feature data (~42% of Córdoba's OSM buildings have
        // levels; the brief calls this out explicitly) stays untouched —
        // only the colour ramp is ours. Taller, more-often-real buildings
        // read as more prominent; the many default-height buildings blend
        // into the muted base instead of all shouting at once.
        "fill-extrusion-color": [
          "interpolate",
          ["linear"],
          ["get", "render_height"],
          0,
          palette.buildingLow,
          15,
          palette.buildingLow,
          60,
          palette.buildingHigh,
        ],
        "fill-extrusion-opacity": 0.85,
      };
    case "label_city":
      return { "text-color": palette.buildingHigh, "text-halo-color": palette.labelHalo };
    default:
      return undefined;
  }
}

/**
 * Turns OpenFreeMap's light "liberty" style dark (issue #119): a generic
 * lightness-inversion pass darkens every layer (the base style is
 * uniformly light, so this alone gets most of the way there), then a
 * small set of layers we care about specifically — background, water,
 * the 3D buildings, city labels — get an exact, curated colour instead.
 * Everything that is not a colour (ids, sources, source-layer, filters,
 * zoom ranges, numeric paint values, the height/base expressions that
 * carry OSM's real per-building data) passes through unchanged.
 */
export function buildDarkStyle(base: MapStyle, palette: MapPalette): MapStyle {
  const layers = base.layers.map((layer) => {
    const needsPaint = OVERRIDE_IDS.has(layer.id) || SUPPRESS_IDS.has(layer.id) || MUTE_IDS.has(layer.id);

    // A layer with no `paint` at all (some symbol/background layers) must
    // stay that way: MapLibre's style schema rejects an explicit
    // `paint: undefined` key, which `{ ...layer, paint: undefined }` would
    // otherwise introduce (it did — this broke real OpenFreeMap layers
    // that carry no paint object, e.g. several `label_*` symbol layers).
    if (!layer.paint && !needsPaint) {
      return layer;
    }

    const recoloredPaint = layer.paint ? (deepRecolor(layer.paint) as Record<string, unknown>) : {};

    if (SUPPRESS_IDS.has(layer.id)) {
      return { ...layer, paint: { ...recoloredPaint, "icon-opacity": 0, "text-opacity": 0 } };
    }
    if (MUTE_IDS.has(layer.id)) {
      return { ...layer, paint: { ...recoloredPaint, "text-opacity": MUTE_OPACITY, "icon-opacity": MUTE_OPACITY } };
    }
    if (!OVERRIDE_IDS.has(layer.id)) {
      return { ...layer, paint: recoloredPaint };
    }
    const override = overridePaint(layer, palette);
    return { ...layer, paint: { ...recoloredPaint, ...override } };
  });

  return { ...base, layers };
}

/** OpenFreeMap's public "liberty" style: vector tiles, no API key required (issue #119). */
export const OPENFREEMAP_STYLE_URL = "https://tiles.openfreemap.org/styles/liberty";

/**
 * Reads the `--map-*` design tokens from `:root` (tokens.css), so the
 * MapLibre style — which needs literal colour values, not CSS custom
 * properties — still has exactly one source of truth for its palette.
 */
export function readMapPalette(root: HTMLElement = document.documentElement): MapPalette {
  const style = getComputedStyle(root);
  const read = (name: string, fallback: string) => {
    const value = style.getPropertyValue(name).trim();
    return value.length > 0 ? value : fallback;
  };
  // Fallbacks only, for a browser/test environment that cannot resolve the
  // CSS custom property; tokens.css remains the source of truth whenever
  // getComputedStyle actually resolves it.
  // ds-allow-hardcode:start
  return {
    background: read("--map-bg", "#060a12"),
    water: read("--map-water", "#0c1c33"),
    buildingLow: read("--map-building-low", "#171d2b"),
    buildingHigh: read("--map-building-high", "#3a4a63"),
    labelHalo: read("--map-label-halo", "rgba(6,10,18,0.85)"),
  };
  // ds-allow-hardcode:end
}

/** Fetches OpenFreeMap's style and returns it recoloured dark, ready to hand to `new maplibregl.Map({ style })`. */
export async function loadDarkStyle(root?: HTMLElement): Promise<MapStyle> {
  const response = await fetch(OPENFREEMAP_STYLE_URL);
  if (!response.ok) {
    throw new Error(`failed to load the base map style (status ${response.status})`);
  }
  const base = (await response.json()) as MapStyle;
  return buildDarkStyle(base, readMapPalette(root));
}
