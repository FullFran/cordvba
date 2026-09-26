/**
 * Building-shadow geometry for the historic centre (issue #139): projects
 * each rendered building footprint along the sun's shadow vector by
 * `height / tan(altitude)` — the standard right-triangle relationship
 * between a gnomon's height, the sun's altitude and the shadow it casts.
 *
 * Drawn as one translucent layer, always labelled INFERRED (never a claim
 * of measured fact): a shadow polygon here is the convex hull of a
 * footprint and its shadow-cast translation. That is exact for a convex
 * footprint and a defensible, cheap over-approximation for a concave one
 * — not a full silhouette sweep, which needs to classify each edge as
 * sun-facing or not. Honest about the trade-off, not silent about it (see
 * DESIGN.md).
 */
import { destinationPoint, type LatLon } from "./geo";
import type { SunPosition } from "../lib/sun";

/** A building footprint's vertices; open or closed (first === last or not) — every point here is treated as a vertex. */
export type Footprint = LatLon[];

const DEFAULT_MAX_SHADOW_LENGTH_M = 400;

/** The compass bearing a shadow falls towards: directly away from the sun. */
export function shadowBearing(sunAzimuthDeg: number): number {
  return (sunAzimuthDeg + 180) % 360;
}

/**
 * `height / tan(altitude)`: the shadow a `heightM`-tall gnomon casts when
 * the sun is `altitudeDeg` above the horizon. Clamped to `maxLengthM` so a
 * near-horizon sun never draws an absurdly long shadow, and 0 once the
 * sun is at or below the horizon or the building has no positive height.
 */
export function shadowLengthM(
  heightM: number,
  altitudeDeg: number,
  maxLengthM: number = DEFAULT_MAX_SHADOW_LENGTH_M,
): number {
  if (altitudeDeg <= 0 || heightM <= 0) {
    return 0;
  }
  const length = heightM / Math.tan((altitudeDeg * Math.PI) / 180);
  return Math.min(Math.max(length, 0), maxLengthM);
}

function cross(o: LatLon, a: LatLon, b: LatLon): number {
  return (a.lon - o.lon) * (b.lat - o.lat) - (a.lat - o.lat) * (b.lon - o.lon);
}

/**
 * The convex hull of `points`, via Andrew's monotone chain — used to trace
 * one shadow polygon from a footprint's vertices plus their shadow-cast
 * translation. Degenerates to the input unchanged for fewer than 3 points.
 */
export function convexHull(points: LatLon[]): LatLon[] {
  if (points.length < 3) {
    return points.slice();
  }
  const sorted = [...points].sort((a, b) => a.lon - b.lon || a.lat - b.lat);

  const lower: LatLon[] = [];
  for (const p of sorted) {
    while (lower.length >= 2 && cross(lower[lower.length - 2]!, lower[lower.length - 1]!, p) <= 0) {
      lower.pop();
    }
    lower.push(p);
  }

  const upper: LatLon[] = [];
  for (let i = sorted.length - 1; i >= 0; i--) {
    const p = sorted[i]!;
    while (upper.length >= 2 && cross(upper[upper.length - 2]!, upper[upper.length - 1]!, p) <= 0) {
      upper.pop();
    }
    upper.push(p);
  }

  lower.pop();
  upper.pop();
  return lower.concat(upper);
}

export interface BuildingFootprint {
  footprint: Footprint;
  heightM: number;
}

/**
 * One building's shadow polygon, or `null` when it casts none (sun at or
 * below the horizon, non-positive height, or a degenerate footprint).
 */
export function buildingShadow(
  building: BuildingFootprint,
  sun: SunPosition,
  maxLengthM: number = DEFAULT_MAX_SHADOW_LENGTH_M,
): Footprint | null {
  const length = shadowLengthM(building.heightM, sun.altitudeDeg, maxLengthM);
  if (length <= 0 || building.footprint.length < 3) {
    return null;
  }
  const bearing = shadowBearing(sun.azimuthDeg);
  const translated = building.footprint.map((point) => destinationPoint(point, bearing, length));
  return convexHull([...building.footprint, ...translated]);
}

/** The loose shape this module needs from a queried MapLibre feature's `geometry` — never the full GeoJSON type, so this stays unit-testable with a plain object. */
export interface QueriedGeometry {
  type: string;
  coordinates: unknown;
}

/**
 * Reads a footprint out of a MapLibre `queryRenderedFeatures` result's
 * `geometry` (issue #139): GeoJSON stores rings as `[lon, lat]` pairs, so
 * this is also where that flips to this module's `{lat, lon}` convention.
 * Only the outer ring (a Polygon's first ring, or a MultiPolygon's first
 * polygon's first ring) is used — holes are ignored, an acceptable
 * simplification for an inferred shadow silhouette. Returns `null` for
 * any other geometry type or a degenerate (<3-point) ring, rather than
 * throwing on a feature this layer never expected.
 */
export function footprintFromGeometry(geometry: QueriedGeometry | null | undefined): Footprint | null {
  if (!geometry) {
    return null;
  }

  let ring: unknown;
  if (geometry.type === "Polygon") {
    ring = (geometry.coordinates as unknown[])[0];
  } else if (geometry.type === "MultiPolygon") {
    ring = ((geometry.coordinates as unknown[])[0] as unknown[] | undefined)?.[0];
  } else {
    return null;
  }

  if (!Array.isArray(ring) || ring.length < 3) {
    return null;
  }
  return (ring as [number, number][]).map(([lon, lat]) => ({ lat, lon }));
}

/**
 * The inverse of `footprintFromGeometry`'s coordinate order: a shadow
 * polygon back into GeoJSON's `[lon, lat]` ring shape, closed (first
 * point repeated at the end, as GeoJSON polygons require).
 */
export function shadowToGeoJSONCoordinates(shadow: Footprint): [number, number][][] {
  const ring = shadow.map((point): [number, number] => [point.lon, point.lat]);
  if (ring.length > 0) {
    ring.push(ring[0]!);
  }
  return [ring];
}
