import { useEffect, useId, useRef, useState } from "react";
import type maplibregl from "maplibre-gl";

import { prefersReducedMotion } from "../lib/motion";
import { windVelocity } from "../map/wind";

export interface WindParticlesProps {
  map: maplibregl.Map;
  directionDeg: number | null;
  speedMs: number | string | null;
  label: string;
  toggleLabel: string;
}

const PARTICLE_COUNT = 140;

interface Particle {
  x: number;
  y: number;
}

/**
 * An animated particle field showing the single METAR station's wind
 * (issue #119): explicitly illustrative, one station, not spatially
 * varied across the city, and labelled as such whenever it is on.
 * Frozen to one static frame under `prefers-reduced-motion`, and
 * toggleable so a visitor can turn it off entirely.
 */
export function WindParticles({ map, directionDeg, speedMs, label, toggleLabel }: WindParticlesProps) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const [enabled, setEnabled] = useState(true);
  const toggleId = useId();

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || !enabled) {
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

  return (
    <div className="wind-particles">
      <canvas ref={canvasRef} className="wind-particles__canvas" aria-hidden="true" />
      <div className="wind-particles__controls">
        <label htmlFor={toggleId} className="wind-particles__toggle">
          <input
            id={toggleId}
            type="checkbox"
            checked={enabled}
            onChange={(event) => setEnabled(event.target.checked)}
          />
          {toggleLabel}
        </label>
        {enabled ? <p className="wind-particles__label">{label}</p> : null}
      </div>
    </div>
  );
}
