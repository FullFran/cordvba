/**
 * Pure geographic math for the illustrative wind particle field (issue
 * #138 — fixes the bug where particles moved in fixed screen-space pixels
 * and silently ignored the map's own bearing/pitch).
 *
 * Particles are advected here in longitude/latitude, exactly like a real
 * parcel of air would drift, and only ever converted to a screen position
 * by the caller through the live MapLibre `map.project()` — never by this
 * module. That split is the fix: geography stays geography regardless of
 * which way the visitor has rotated or tilted the camera, and the screen
 * position is always whatever the *current* camera says that geographic
 * point projects to.
 *
 * Meteorological convention: `directionDeg` is where the wind comes FROM.
 * The flow — and so the particles — moves TO the opposite compass
 * direction (`directionDeg + 180`), which is why `windBearingTo` exists
 * separately from the raw reading.
 */

export interface GeoPoint {
  lng: number;
  lat: number;
}

/** A lng/lat box, in the shape this module needs from a MapLibre `LngLatBounds` — never the class itself, so this stays unit-testable without a map. */
export interface GeoBounds {
  west: number;
  east: number;
  south: number;
  north: number;
}

export interface WindStep {
  dLng: number;
  dLat: number;
}

const FALLBACK_SPEED_MS = 3;
/** Metres per degree of latitude (WGS84 mean; plenty precise at this display's scale). */
const METERS_PER_DEGREE_LAT = 111_320;
/**
 * Purely illustrative multiplier (see DESIGN.md's "illustrative wind"
 * entry): a real METAR speed (a few m/s) advected at true geographic
 * scale would cross a barely perceptible fraction of a degree per frame.
 * This scales how fast the field appears to flow — it never touches
 * *direction*, which is exactly what issue #138 was about.
 */
const ILLUSTRATIVE_SPEED_SCALE = 60;

function toRad(deg: number): number {
  return (deg * Math.PI) / 180;
}

function wrapDeg(deg: number): number {
  return ((deg % 360) + 360) % 360;
}

/**
 * The compass bearing the wind blows TOWARDS (the flow direction), given
 * the meteorological direction it is reported as blowing FROM. `null` (no
 * reading) is treated as calm air with no preferred direction, same as
 * the previous fallback.
 */
export function windBearingTo(directionDeg: number | null): number {
  return wrapDeg((directionDeg ?? 0) + 180);
}

function resolveSpeed(speedMs: number | string | null): number {
  return typeof speedMs === "number" && Number.isFinite(speedMs) ? speedMs : FALLBACK_SPEED_MS;
}

/**
 * One geographic advection step: a lng/lat delta for `dtSeconds` of the
 * meteorological wind, correcting the longitude component for the
 * shrinking length of a degree of longitude away from the equator
 * (`cos(latitude)`). A 310° wind (reported FROM the WNW) blows TO 130°
 * (south-east): negative `dLat` (south), positive `dLng` (east).
 */
export function geographicWindStep(
  directionDeg: number | null,
  speedMs: number | string | null,
  dtSeconds: number,
  atLatDeg: number,
  scale: number = ILLUSTRATIVE_SPEED_SCALE,
): WindStep {
  const speed = resolveSpeed(speedMs);
  const bearingRad = toRad(windBearingTo(directionDeg));
  const distanceM = speed * scale * dtSeconds;

  const dLat = (distanceM * Math.cos(bearingRad)) / METERS_PER_DEGREE_LAT;
  const metersPerDegreeLng = METERS_PER_DEGREE_LAT * Math.cos(toRad(atLatDeg));
  const dLng = metersPerDegreeLng !== 0 ? (distanceM * Math.sin(bearingRad)) / metersPerDegreeLng : 0;

  return { dLng, dLat };
}

/**
 * Wraps a point back into `bounds` — a torus, not a cliff edge — so a
 * particle that drifts past the current viewport re-enters from the
 * opposite side instead of vanishing. Same spirit as the previous
 * screen-space canvas wrap, now done in geographic space so it keeps
 * working whatever the map's current bearing/pitch/zoom is.
 */
export function wrapWithinBounds(point: GeoPoint, bounds: GeoBounds): GeoPoint {
  const width = bounds.east - bounds.west;
  const height = bounds.north - bounds.south;
  let { lng, lat } = point;

  if (width > 0) {
    while (lng < bounds.west) lng += width;
    while (lng > bounds.east) lng -= width;
  }
  if (height > 0) {
    while (lat < bounds.south) lat += height;
    while (lat > bounds.north) lat -= height;
  }
  return { lng, lat };
}

/** One advect-then-wrap step — the shape `WindParticles.tsx` calls once per particle per frame. */
export function advectParticle(
  point: GeoPoint,
  directionDeg: number | null,
  speedMs: number | string | null,
  dtSeconds: number,
  bounds: GeoBounds,
  scale?: number,
): GeoPoint {
  const step = geographicWindStep(directionDeg, speedMs, dtSeconds, point.lat, scale);
  return wrapWithinBounds({ lng: point.lng + step.dLng, lat: point.lat + step.dLat }, bounds);
}
