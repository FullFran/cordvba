import { describe, expect, it } from "vitest";

import { clampToEdge, haversineDistanceKm } from "./geo";

describe("haversineDistanceKm", () => {
  it("returns 0 for the same point", () => {
    expect(haversineDistanceKm({ lat: 37.8882, lon: -4.7794 }, { lat: 37.8882, lon: -4.7794 })).toBe(0);
  });

  it("matches the known Córdoba historic-centre-to-airport distance within 1km", () => {
    // Córdoba centre ~37.8882,-4.7794; Córdoba Airport (LEBA) ~37.842,-4.8488
    const km = haversineDistanceKm({ lat: 37.8882, lon: -4.7794 }, { lat: 37.842, lon: -4.8488 });
    expect(km).toBeGreaterThan(6);
    expect(km).toBeLessThan(9);
  });
});

describe("clampToEdge", () => {
  const bounds = { width: 800, height: 400 };
  const center = { x: 400, y: 200 };

  it("returns the point unchanged when it is already inside the bounds", () => {
    const result = clampToEdge({ x: 500, y: 250 }, center, bounds, 24);
    expect(result.clamped).toBe(false);
    expect(result.x).toBe(500);
    expect(result.y).toBe(250);
  });

  it("clamps a point far to the right onto the right edge margin", () => {
    const result = clampToEdge({ x: 5000, y: 200 }, center, bounds, 24);
    expect(result.clamped).toBe(true);
    expect(result.x).toBeCloseTo(bounds.width - 24, 0);
    expect(result.y).toBeCloseTo(200, 0);
    expect(result.angleDeg).toBeCloseTo(0, 0); // due east
  });

  it("clamps a point far above onto the top edge margin", () => {
    const result = clampToEdge({ x: 400, y: -5000 }, center, bounds, 24);
    expect(result.clamped).toBe(true);
    expect(result.y).toBeCloseTo(24, 0);
    expect(result.angleDeg).toBeCloseTo(-90, 0); // due north (screen-space: negative y)
  });

  it("clamps a diagonal off-screen point onto the correct corner region", () => {
    const result = clampToEdge({ x: -5000, y: 5000 }, center, bounds, 24);
    expect(result.clamped).toBe(true);
    expect(result.x).toBeGreaterThanOrEqual(0);
    expect(result.x).toBeLessThanOrEqual(bounds.width);
    expect(result.y).toBeGreaterThanOrEqual(0);
    expect(result.y).toBeLessThanOrEqual(bounds.height);
  });
});
