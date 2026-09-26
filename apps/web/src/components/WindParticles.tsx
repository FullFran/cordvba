import { useEffect, useRef } from "react";
import type maplibregl from "maplibre-gl";

import { prefersReducedMotion } from "../lib/motion";
import { advectParticle } from "../map/wind";
import type { GeoPoint } from "../map/wind";

export interface WindParticlesProps {
  map: maplibregl.Map;
  directionDeg: number | null;
  speedMs: number | string | null;
  /** Whether the layer is switched on; the toggle control itself lives in `MapView`'s merged legend (issue #119, maintainer review). */
  enabled: boolean;
}

const PARTICLE_COUNT = 140;
/** Clamps the animation's per-frame time delta (issue #138): a tab returning from the background can report a multi-second gap, which would otherwise fling every particle far past the viewport in one jump. */
const MAX_DT_SECONDS = 0.1;

function randomPointInBounds(bounds: maplibregl.LngLatBounds): GeoPoint {
  const sw = bounds.getSouthWest();
  const ne = bounds.getNorthEast();
  return {
    lng: sw.lng + Math.random() * (ne.lng - sw.lng),
    lat: sw.lat + Math.random() * (ne.lat - sw.lat),
  };
}

function toGeoBounds(bounds: maplibregl.LngLatBounds) {
  const sw = bounds.getSouthWest();
  const ne = bounds.getNorthEast();
  return { west: sw.lng, east: ne.lng, south: sw.lat, north: ne.lat };
}

/**
 * The canvas half of the illustrative wind layer (issue #119; geographic
 * advection fixed in issue #138): driven by the single METAR reading.
 * Particles are stored and stepped in longitude/latitude
 * (`src/map/wind.ts`'s pure `advectParticle`), exactly like a drifting
 * parcel of air, and only ever turned into a screen position through the
 * live `map.project()` at draw time — never through this component's own
 * trigonometry. That is the fix for the original bug: the field now stays
 * geographically correct (and re-projects itself automatically) whatever
 * the map's current bearing, pitch, pan or zoom is, instead of moving in
 * fixed screen-space pixels that silently ignored all four. Frozen to one
 * static frame under `prefers-reduced-motion`, which still re-projects
 * (but never re-advects) on `move` so a rotated/panned camera never leaves
 * a stale frame on screen. Its honesty label and on/off toggle live in
 * `MapView`'s merged legend, not here (issue #119, maintainer review).
 */
export function WindParticles({ map, directionDeg, speedMs, enabled }: WindParticlesProps) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) {
      return undefined;
    }
    const ctx = canvas.getContext("2d");
    if (!ctx) {
      return undefined;
    }

    const container = map.getContainer();
    let particles: GeoPoint[] = [];

    function draw() {
      canvas!.width = container.clientWidth;
      canvas!.height = container.clientHeight;
      ctx!.clearRect(0, 0, canvas!.width, canvas!.height);
      if (!enabled) {
        return;
      }
      ctx!.fillStyle = "rgba(125, 211, 252, 0.75)";
      for (const particle of particles) {
        const { x, y } = map.project([particle.lng, particle.lat]);
        ctx!.beginPath();
        ctx!.arc(x, y, 1.4, 0, Math.PI * 2);
        ctx!.fill();
      }
    }

    if (!enabled) {
      draw(); // clears the canvas
      map.on("resize", draw);
      return () => {
        map.off("resize", draw);
      };
    }

    particles = Array.from({ length: PARTICLE_COUNT }, () => randomPointInBounds(map.getBounds()));
    draw();

    if (prefersReducedMotion()) {
      // One static frame; no rAF loop, but still re-project (never
      // re-advect) whenever the camera moves, rotates or tilts, so the
      // frame never goes stale relative to the current view (issue #138).
      map.on("move", draw);
      map.on("resize", draw);
      return () => {
        map.off("move", draw);
        map.off("resize", draw);
      };
    }

    let lastTimeMs = performance.now();
    let frameHandle = 0;
    function step(nowMs: number) {
      const dtSeconds = Math.min(MAX_DT_SECONDS, Math.max(0, (nowMs - lastTimeMs) / 1000));
      lastTimeMs = nowMs;
      const bounds = toGeoBounds(map.getBounds());
      particles = particles.map((particle) => advectParticle(particle, directionDeg, speedMs, dtSeconds, bounds));
      draw();
      frameHandle = requestAnimationFrame(step);
    }
    frameHandle = requestAnimationFrame(step);

    return () => {
      cancelAnimationFrame(frameHandle);
    };
  }, [map, enabled, directionDeg, speedMs]);

  return <canvas ref={canvasRef} className="wind-particles__canvas" aria-hidden="true" />;
}
