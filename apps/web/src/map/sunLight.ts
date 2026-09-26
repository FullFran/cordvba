/**
 * MapLibre's `fill-extrusion` light spec, driven by the sun's real
 * position (issue #139) instead of a fixed default light. `position`'s
 * azimuthal angle is the sun's own compass azimuth; its polar angle is
 * measured from the zenith (so directly overhead is 0°, the horizon is
 * 90°) — the geometric complement of altitude.
 *
 * Intensity is driven primarily by `phase` (parent review: "noon vs dusk
 * differ by a faint tint... make the lighting dramatic"), not a smooth
 * function of altitude alone: day is near MapLibre's own maximum, golden
 * hour and blue hour each a clear step down, and night genuinely dark —
 * building faces are meant to visibly change as the sun moves through
 * `sunPhase`'s four bands, not fade by a few percent. It never drops to
 * exactly zero, so extruded buildings never go fully flat/black even at
 * the deepest part of the night. The colour is never invented here — it
 * is always one of the day/golden/blue/night light-tint tokens
 * (`tokens.css`), passed in by the caller.
 */
import type { SunPhase, SunPosition } from "../lib/sun";

export interface MapLight {
  anchor: "map";
  color: string;
  intensity: number;
  /** [radial distance, azimuthal angle (0-360, from north, clockwise), polar angle (0-180, from zenith)] — MapLibre's own `light.position` shape. */
  position: [number, number, number];
}

/**
 * Per-phase intensity (parent review: dramatic, not subtle). MapLibre's
 * own valid range is 0-1; day sits near the top so extrusion faces
 * genuinely light up, night sits near the bottom so they genuinely go
 * dark, and golden/blue hour are clear, ordered steps between the two —
 * never a fixed floor that quietly ignores how deep into the night it
 * actually is.
 */
const PHASE_INTENSITY: Record<SunPhase, number> = {
  day: 0.85,
  golden: 0.55,
  blue: 0.28,
  night: 0.06,
};

/** MapLibre's own suggested default radial distance for a directional-feeling light. */
const RADIAL_DISTANCE = 1.15;

export function sunLight(sun: SunPosition, phase: SunPhase, colorHex: string): MapLight {
  const polar = Math.max(0, Math.min(180, 90 - sun.altitudeDeg));

  return {
    anchor: "map",
    color: colorHex,
    intensity: PHASE_INTENSITY[phase],
    position: [RADIAL_DISTANCE, sun.azimuthDeg, polar],
  };
}
