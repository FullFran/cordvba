import { useEffect, useRef } from "react";
import type maplibregl from "maplibre-gl";

import { prefersReducedMotion } from "../lib/motion";
import { windVelocity } from "../map/wind";

export interface WindParticlesProps {
  map: maplibregl.Map;
  directionDeg: number | null;
  speedMs: number | string | null;
  /** Whether the layer is switched on; the toggle control itself lives in `MapView`'s merged legend (issue #119, maintainer review). */
  enabled: boolean;
}

const PARTICLE_COUNT = 140;

interface Particle {
  x: number;
  y: number;
}

/**
 * The canvas half of the illustrative wind layer (issue #119): driven by
 * the single METAR reading, frozen to one static frame under
 * `prefers-reduced-motion`. Its honesty label and on/off toggle live in
 * `MapView`'s merged legend, not here, so they can never be covered by
 * (or cover) the extrusion-heights caption, and stay visible regardless
 * of whether this canvas is animating.
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
    const resize = () => {
      canvas.width = container.clientWidth;
      canvas.height = container.clientHeight;
    };
    resize();
    map.on("resize", resize);

    if (!enabled) {
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      return () => {
        map.off("resize", resize);
      };
    }

    const { vx, vy } = windVelocity(directionDeg, speedMs);
    let particles: Particle[] = Array.from({ length: PARTICLE_COUNT }, () => ({
      x: Math.random() * canvas.width,
      y: Math.random() * canvas.height,
    }));

    function draw() {
      if (!ctx || !canvas) return;
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      ctx.fillStyle = "rgba(125, 211, 252, 0.75)";
      for (const p of particles) {
        ctx.beginPath();
        ctx.arc(p.x, p.y, 1.4, 0, Math.PI * 2);
        ctx.fill();
      }
    }

    if (prefersReducedMotion()) {
      draw(); // one static frame; no animation loop under reduced motion
      return () => {
        map.off("resize", resize);
      };
    }

    let frameHandle = 0;
    function step() {
      if (!canvas) return;
      particles = particles.map((p) => {
        let x = p.x + vx / 30;
        let y = p.y + vy / 30;
        if (x < 0) x += canvas.width;
        if (x > canvas.width) x -= canvas.width;
        if (y < 0) y += canvas.height;
        if (y > canvas.height) y -= canvas.height;
        return { x, y };
      });
      draw();
      frameHandle = requestAnimationFrame(step);
    }
    frameHandle = requestAnimationFrame(step);

    return () => {
      cancelAnimationFrame(frameHandle);
      map.off("resize", resize);
    };
  }, [map, enabled, directionDeg, speedMs]);

  return <canvas ref={canvasRef} className="wind-particles__canvas" aria-hidden="true" />;
}
