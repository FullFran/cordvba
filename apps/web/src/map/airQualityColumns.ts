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

const MIN_HEIGHT_M = 25;
const HEIGHT_PER_INDEX_M = 18;
/** A missing/non-numeric index still draws a modest column rather than vanishing (this app's standing "never silently blank" rule) — the same height a category-1 ("good") reading would get. */
const FALLBACK_HEIGHT_M = MIN_HEIGHT_M + HEIGHT_PER_INDEX_M;

/** The column's height in metres for a given ICA index (1-6); monotonically increasing, nothing interpolated between stations. */
export function icaColumnHeightM(index: number | string | null): number {
  if (typeof index !== "number" || !Number.isFinite(index)) {
    return FALLBACK_HEIGHT_M;
  }
  return MIN_HEIGHT_M + index * HEIGHT_PER_INDEX_M;
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

const DISC_RADIUS_M = 14;
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
      coordinates: [discPolygonCoordinates({ lat: station.lat, lon: station.lon }, DISC_RADIUS_M, DISC_POINTS)],
    },
  };
}
