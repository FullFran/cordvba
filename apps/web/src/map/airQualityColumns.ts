/**
 * Air quality as columns of light (issue #140): each city station becomes
 * a small extruded disc whose height encodes its ICA index (1-6) —
 * literally, not just by colour, so the encoding survives even without
 * colour vision or a display that renders the category hue correctly.
 * Three independent features, one per station; there is deliberately no
 * shared/merged surface between them (DESIGN.md §7's standing rule — see
 * the legend line this layer carries).
 */
import { destinationPoint, type LatLon } from "./geo";

/**
 * Hero scale (parent review, 2026-09-26): the original 18-25m-per-level
 * stub was invisible at the app's initial `fitBounds` camera — a real
 * city block's buildings (`--map-building-*`, tens of metres) already
 * dwarfed it. `--color-aq-*`'s own columns need to visibly out-scale the
 * skyline they rise over, not blend into its noise floor, so each ICA
 * level now reads as ~280m — comfortably inside the requested 250-300m
 * band and tall enough to be the frame's clear focal point at zoom ~14.
 * A missing/non-numeric index is still drawn, at the same height a
 * category-1 ("good") reading would get, rather than vanishing.
 */
export const HEIGHT_PER_INDEX_M = 280;
const FALLBACK_HEIGHT_M = HEIGHT_PER_INDEX_M;

/** The column's height in metres for a given ICA index (1-6); monotonically increasing, nothing interpolated between stations. */
export function icaColumnHeightM(index: number | string | null): number {
  if (typeof index !== "number" || !Number.isFinite(index)) {
    return FALLBACK_HEIGHT_M;
  }
  return index * HEIGHT_PER_INDEX_M;
}

/**
 * A closed ring of `pointCount` vertices at `radiusM` metres around
 * `center` (issue #140: the column's small circular footprint) —
 * `[lon, lat]` pairs, GeoJSON's own coordinate order, first point
 * repeated at the end as GeoJSON polygons require.
 */
export function discPolygonCoordinates(
  center: LatLon,
  radiusM: number,
  pointCount: number,
): [number, number][] {
  const ring: [number, number][] = [];
  for (let i = 0; i < pointCount; i++) {
    const bearingDeg = (360 * i) / pointCount;
    const point = destinationPoint(center, bearingDeg, radiusM);
    ring.push([point.lon, point.lat]);
  }
  ring.push(ring[0]!);
  return ring;
}

/** Hero scale (parent review): 14m was a sliver at city zoom; 130m reads as a real footprint under a ~280-1680m column without the three discs' footprints ever touching (stations are hundreds of metres apart). */
export const AQ_COLUMN_RADIUS_M = 130;
const DISC_POINTS = 16;

export interface StationColumnInput {
  lat: number;
  lon: number;
  category: string;
  index: number | string | null;
}

/** One station's column, as a GeoJSON Feature ready for a `fill-extrusion` layer's source (`fill-extrusion-height` reads `properties.height`, colour reads `properties.category`). */
export function stationColumnFeature(station: StationColumnInput): GeoJSON.Feature<GeoJSON.Polygon> {
  return {
    type: "Feature",
    properties: {
      category: station.category,
      height: icaColumnHeightM(station.index),
    },
    geometry: {
      type: "Polygon",
      coordinates: [
        discPolygonCoordinates({ lat: station.lat, lon: station.lon }, AQ_COLUMN_RADIUS_M, DISC_POINTS),
      ],
    },
  };
}

/**
 * The same station point as a `Point` feature (parent review) — for the
 * breathing glow's `circle` layer, which needs a point geometry, not the
 * column's own polygon footprint. `opacity`/`radiusScale` carry the
 * current animation frame's values (the caller drives the breathing;
 * this only shapes the feature), defaulting to a calm, static mid-point
 * for a call site that never animates (e.g. a unit test).
 */
export function stationGlowFeature(
  station: StationColumnInput,
  opacity: number = 0.3,
  radiusScale: number = 1,
): GeoJSON.Feature<GeoJSON.Point> {
  return {
    type: "Feature",
    properties: { category: station.category, opacity, radiusScale },
    geometry: { type: "Point", coordinates: [station.lon, station.lat] },
  };
}
