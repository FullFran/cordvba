import { invertColorLightness, isColorString } from "./color";
import type { SunPhase } from "../lib/sun";

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
  minzoom?: number;
  maxzoom?: number;
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
  roadMinor: string;
  roadMid: string;
  roadMajor: string;
  roadLabel: string;
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

/**
 * Roads, bridges, tunnels and aeroway lines (issue #119, maintainer
 * review): OpenFreeMap's `liberty` style paints these in warm amber, the
 * same hue family as the "moderate"/"poor" air-quality beacons. Generic
 * lightness inversion alone keeps that hue, just darker — still visibly
 * amber. These get an exact, curated neutral-slate colour instead, tiered
 * by prominence so major roads stay only slightly brighter than minor
 * ones, and casings (an outline behind the fill) render one tier down
 * from their own fill.
 */
const ROAD_ID_PATTERN = /^(road_|bridge_|tunnel_|aeroway_)/;

/**
 * True only for a `line`/`fill` road-ish layer. The id pattern alone is
 * not enough: `road_shield_us` (a US-highway shield icon) and
 * `road_one_way_arrow[_opposite]` (already suppressed above) are
 * `symbol` layers that also start with `road_`, and MapLibre's style
 * schema rejects `line-color`/`fill-color` on a layer type that doesn't
 * paint with them — this broke real OpenFreeMap layers once already.
 */
function isRoadLayer(layer: MapStyleLayer): boolean {
  return ROAD_ID_PATTERN.test(layer.id) && (layer.type === "line" || layer.type === "fill");
}

function roadColor(id: string, palette: MapPalette): string {
  if (id.includes("_casing")) {
    return palette.roadMinor;
  }
  if (/motorway|trunk_primary|runway/.test(id)) {
    return palette.roadMajor;
  }
  if (/secondary_tertiary|_link|taxiway/.test(id)) {
    return palette.roadMid;
  }
  return palette.roadMinor;
}

/**
 * The building layers' zoom range (issue #119, maintainer review: "3D is
 * barely perceptible at the initial zoom"). OpenFreeMap's own cutover is
 * 2D fill 13-14, extrusion from 14; lowered by two levels so the historic
 * centre still reads as 3D at a `fitBounds` camera that (depending on how
 * spread out the live API's real station coordinates are) might land
 * below z14, without waiting on a specific zoom value to be true.
 */
const MINZOOM_OVERRIDES: Record<string, { minzoom?: number; maxzoom?: number }> = {
  building: { minzoom: 11, maxzoom: 12 },
  "building-3d": { minzoom: 12 },
};

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
 * uniformly light, so this alone gets most of the way there); background,
 * water, the 3D buildings and city labels get an exact, curated colour;
 * roads/bridges/tunnels/aeroways get a tiered neutral-slate colour instead
 * of their original warm amber; basemap noise (POI/transit icons, minor
 * labels) is hidden or muted. Everything that is not a colour (ids,
 * sources, source-layer, filters, zoom ranges, numeric paint values, the
 * height/base expressions that carry OSM's real per-building data) passes
 * through unchanged.
 */
export function buildDarkStyle(base: MapStyle, palette: MapPalette): MapStyle {
  const layers = base.layers.map((layer) => {
    const zoomOverride = MINZOOM_OVERRIDES[layer.id];
    const withZoom = zoomOverride ? { ...layer, ...zoomOverride } : layer;

    const isRoad = isRoadLayer(layer);
    const needsPaint = OVERRIDE_IDS.has(layer.id) || SUPPRESS_IDS.has(layer.id) || MUTE_IDS.has(layer.id) || isRoad;

    // A layer with no `paint` at all (some symbol/background layers) must
    // stay that way: MapLibre's style schema rejects an explicit
    // `paint: undefined` key, which `{ ...layer, paint: undefined }` would
    // otherwise introduce (it did — this broke real OpenFreeMap layers
    // that carry no paint object, e.g. several `label_*` symbol layers).
    if (!withZoom.paint && !needsPaint) {
      return withZoom;
    }

    const recoloredPaint = withZoom.paint ? (deepRecolor(withZoom.paint) as Record<string, unknown>) : {};

    if (SUPPRESS_IDS.has(layer.id)) {
      return { ...withZoom, paint: { ...recoloredPaint, "icon-opacity": 0, "text-opacity": 0 } };
    }
    if (isRoad) {
      const colorProp = layer.type === "fill" ? "fill-color" : "line-color";
      return { ...withZoom, paint: { ...recoloredPaint, [colorProp]: roadColor(layer.id, palette) } };
    }
    if (MUTE_IDS.has(layer.id)) {
      const labelOverride = layer.id === "highway-name-major" ? { "text-color": palette.roadLabel } : {};
      return {
        ...withZoom,
        paint: { ...recoloredPaint, "text-opacity": MUTE_OPACITY, "icon-opacity": MUTE_OPACITY, ...labelOverride },
      };
    }
    if (!OVERRIDE_IDS.has(layer.id)) {
      return { ...withZoom, paint: recoloredPaint };
    }
    const override = overridePaint(layer, palette);
    return { ...withZoom, paint: { ...recoloredPaint, ...override } };
  });

  return { ...base, layers };
}

/** OpenFreeMap's public "liberty" style: vector tiles, no API key required (issue #119). */
export const OPENFREEMAP_STYLE_URL = "https://tiles.openfreemap.org/styles/liberty";

/**
 * Reads the `--map-*` design tokens from `:root` (tokens.css), so the
 * MapLibre style — which needs literal colour values, not CSS custom
 * properties — still has exactly one source of truth for its palette.
 *
 * `phase` (issue #139: sun-driven day/golden/blue/night lighting) selects
 * which background/water tokens to read — `--map-bg`/`--map-water` for
 * `night` (the app's original, unchanged default), and
 * `--map-bg-{day,golden,blue}`/`--map-water-{day,golden,blue}` for the
 * other three phases. Every other token (buildings, roads, label halo)
 * stays the same regardless of phase: only the sky/water tint changes
 * with the sun, not the whole basemap.
 */
export function readMapPalette(
  root: HTMLElement = document.documentElement,
  phase: SunPhase = "night",
): MapPalette {
  const style = getComputedStyle(root);
  const read = (name: string, fallback: string) => {
    const value = style.getPropertyValue(name).trim();
    return value.length > 0 ? value : fallback;
  };
  const phaseSuffix = phase === "night" ? "" : `-${phase}`;
  // Fallbacks only, for a browser/test environment that cannot resolve the
  // CSS custom property; tokens.css remains the source of truth whenever
  // getComputedStyle actually resolves it.
  // ds-allow-hardcode:start
  return {
    background: read(`--map-bg${phaseSuffix}`, "#060a12"),
    water: read(`--map-water${phaseSuffix}`, "#0c1c33"),
    buildingLow: read("--map-building-low", "#171d2b"),
    buildingHigh: read("--map-building-high", "#3a4a63"),
    labelHalo: read("--map-label-halo", "rgba(6,10,18,0.85)"),
    roadMinor: read("--map-road-minor", "#1e2229"),
    roadMid: read("--map-road-mid", "#262b36"),
    roadMajor: read("--map-road-major", "#313949"),
    roadLabel: read("--map-road-label", "#5a6272"),
  };
  // ds-allow-hardcode:end
}

/**
 * Fetches OpenFreeMap's style and returns it recoloured dark, ready to
 * hand to `new maplibregl.Map({ style })`. `phase` (issue #139) seeds the
 * *initial* background/water palette with the correct day/golden/blue/
 * night tint from the very first frame, instead of always starting at "night"
 * and waiting for the first `setPaintProperty` call to correct it; every
 * later phase change is applied live by that same runtime call, not by
 * recreating the style.
 */
export async function loadDarkStyle(root?: HTMLElement, phase: SunPhase = "night"): Promise<MapStyle> {
  const response = await fetch(OPENFREEMAP_STYLE_URL);
  if (!response.ok) {
    throw new Error(`failed to load the base map style (status ${response.status})`);
  }
  const base = (await response.json()) as MapStyle;
  return buildDarkStyle(base, readMapPalette(root, phase));
}
