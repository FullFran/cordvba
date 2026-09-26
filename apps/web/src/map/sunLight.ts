/**
 * MapLibre's `fill-extrusion` light spec, driven by the sun's real
 * position (issue #139) instead of a fixed default light. `position`'s
 * azimuthal angle is the sun's own compass azimuth; its polar angle is
 * measured from the zenith (so directly overhead is 0°, the horizon is
 * 90°) — the geometric complement of altitude. Intensity dims toward the
 * horizon and through the night, but never to zero, so extruded buildings
 * never go fully flat/black; it climbs back with the sun the next
 * morning. The colour is never invented here — it is always one of the
 * day/dusk/night light-tint tokens (`tokens.css`), passed in by the
 * caller.
 */
import type { SunPosition } from "../lib/sun";

export interface MapLight {
  anchor: "map";
  color: string;
  intensity: number;
  /** [radial distance, azimuthal angle (0-360, from north, clockwise), polar angle (0-180, from zenith)] — MapLibre's own `light.position` shape. */
  position: [number, number, number];
}

const MIN_INTENSITY = 0.15;
const MAX_INTENSITY = 0.6;
/** MapLibre's own suggested default radial distance for a directional-feeling light. */
const RADIAL_DISTANCE = 1.15;

function toRad(deg: number): number {
  return (deg * Math.PI) / 180;
}

export function sunLight(sun: SunPosition, colorHex: string): MapLight {
  const polar = Math.max(0, Math.min(180, 90 - sun.altitudeDeg));
  const daylightFactor = Math.max(0, Math.sin(toRad(sun.altitudeDeg)));
  const intensity = MIN_INTENSITY + (MAX_INTENSITY - MIN_INTENSITY) * daylightFactor;

  return {
    anchor: "map",
    color: colorHex,
    intensity,
    position: [RADIAL_DISTANCE, sun.azimuthDeg, polar],
  };
}
