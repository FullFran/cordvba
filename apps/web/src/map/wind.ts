/**
 * Pure velocity math for the illustrative wind particle field (issue #119).
 * Kept separate from the canvas-driving component so the trigonometry is
 * unit-testable without a canvas, an animation frame, or a mocked
 * MapLibre map.
 */
export interface WindVelocity {
  vx: number;
  vy: number;
}

const FALLBACK_SPEED_MS = 3;
/** Pixels/frame-tick per m/s, at scale 1; purely visual, not to scale with the map projection (this layer is illustrative, never a to-scale simulation). */
const SPEED_SCALE = 4;

export function windVelocity(
  directionDeg: number | null,
  speedMs: number | string | null,
  scale: number = SPEED_SCALE,
): WindVelocity {
  const speed = typeof speedMs === "number" && Number.isFinite(speedMs) ? speedMs : FALLBACK_SPEED_MS;
  const angleRad = ((directionDeg ?? 0) * Math.PI) / 180;
  return {
    vx: -Math.sin(angleRad) * speed * scale,
    vy: Math.cos(angleRad) * speed * scale,
  };
}
