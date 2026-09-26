/**
 * Pure geometry helpers for the airport edge indicator (issue #119): when
 * the airport weather beacon's true screen position falls outside the map
 * viewport, an arrow clamped to the viewport edge points toward it instead
 * of the beacon silently disappearing off-screen.
 */

export interface LatLon {
  lat: number;
  lon: number;
}

const EARTH_RADIUS_KM = 6371;

function toRad(deg: number): number {
  return (deg * Math.PI) / 180;
}

/** Great-circle distance between two lat/lon points, in kilometres. */
export function haversineDistanceKm(a: LatLon, b: LatLon): number {
  const dLat = toRad(b.lat - a.lat);
  const dLon = toRad(b.lon - a.lon);
  const lat1 = toRad(a.lat);
  const lat2 = toRad(b.lat);

  const sinDLat = Math.sin(dLat / 2);
  const sinDLon = Math.sin(dLon / 2);
  const h = sinDLat * sinDLat + Math.cos(lat1) * Math.cos(lat2) * sinDLon * sinDLon;
  return 2 * EARTH_RADIUS_KM * Math.asin(Math.min(1, Math.sqrt(h)));
}

export interface ScreenPoint {
  x: number;
  y: number;
}

export interface ScreenBounds {
  width: number;
  height: number;
}

export interface EdgeResult extends ScreenPoint {
  /** Whether the point had to be moved (it was outside the bounds). */
  clamped: boolean;
  /** Direction from `center` to the true point, in degrees (screen-space: 0 = east, -90 = north, atan2 convention). */
  angleDeg: number;
}

/**
 * If `point` already falls within `bounds` (inset by `margin`), returns it
 * unchanged. Otherwise projects it onto the ray from `center` through
 * `point`, clamped to the inset rectangle — the standard "off-screen
 * radar arrow" construction.
 */
export function clampToEdge(
  point: ScreenPoint,
  center: ScreenPoint,
  bounds: ScreenBounds,
  margin: number,
): EdgeResult {
  const angleDeg = (Math.atan2(point.y - center.y, point.x - center.x) * 180) / Math.PI;

  const minX = margin;
  const maxX = bounds.width - margin;
  const minY = margin;
  const maxY = bounds.height - margin;

  const inside = point.x >= minX && point.x <= maxX && point.y >= minY && point.y <= maxY;
  if (inside) {
    return { x: point.x, y: point.y, clamped: false, angleDeg };
  }

  const dx = point.x - center.x;
  const dy = point.y - center.y;

  // Scale factor to hit whichever inset edge the ray crosses first.
  const scales: number[] = [];
  if (dx > 0) scales.push((maxX - center.x) / dx);
  if (dx < 0) scales.push((minX - center.x) / dx);
  if (dy > 0) scales.push((maxY - center.y) / dy);
  if (dy < 0) scales.push((minY - center.y) / dy);
  const scale = scales.length > 0 ? Math.min(...scales.filter((s) => s > 0)) : 0;

  const x = Math.min(maxX, Math.max(minX, center.x + dx * scale));
  const y = Math.min(maxY, Math.max(minY, center.y + dy * scale));

  return { x, y, clamped: true, angleDeg };
}
