import { describe, expect, it } from "vitest";

import { sunLight } from "./sunLight";

// ds-allow-hardcode:start (test fixture: arbitrary literal colours exercising that sunLight() passes its given colour straight through — not this app's own design decisions)
describe("sunLight (issue #139/parent review: a dramatic, phase-driven extrusion light, not a faint tint)", () => {
  it("points straight down (polar 0) when the sun is directly overhead", () => {
    const light = sunLight({ azimuthDeg: 120, altitudeDeg: 90 }, "day", "#cfe0f5");
    expect(light.position[2]).toBeCloseTo(0, 5);
  });

  it("points level with the horizon (polar 90) when the sun is on the horizon", () => {
    const light = sunLight({ azimuthDeg: 120, altitudeDeg: 0 }, "blue", "#cfe0f5");
    expect(light.position[2]).toBeCloseTo(90, 5);
  });

  it("carries the sun's own compass azimuth as the light's azimuthal angle", () => {
    const light = sunLight({ azimuthDeg: 257, altitudeDeg: 30 }, "golden", "#cfe0f5");
    expect(light.position[1]).toBeCloseTo(257, 5);
  });

  it("passes the given colour straight through, never inventing one", () => {
    const light = sunLight({ azimuthDeg: 0, altitudeDeg: 30 }, "golden", "#f2b783");
    expect(light.color).toBe("#f2b783");
  });

  it("is strictly stronger at 'day' than 'golden' than 'blue' than 'night' (parent review: 'noon vs dusk differ by a faint tint')", () => {
    const day = sunLight({ azimuthDeg: 0, altitudeDeg: 60 }, "day", "#cfe0f5");
    const golden = sunLight({ azimuthDeg: 0, altitudeDeg: 10 }, "golden", "#ffb066");
    const blue = sunLight({ azimuthDeg: 0, altitudeDeg: 0 }, "blue", "#6f8fd9");
    const night = sunLight({ azimuthDeg: 0, altitudeDeg: -40 }, "night", "#2a3550");
    expect(day.intensity).toBeGreaterThan(golden.intensity);
    expect(golden.intensity).toBeGreaterThan(blue.intensity);
    expect(blue.intensity).toBeGreaterThan(night.intensity);
  });

  it("is dramatically bright at day (near the top of MapLibre's 0-1 intensity range)", () => {
    expect(sunLight({ azimuthDeg: 0, altitudeDeg: 60 }, "day", "#cfe0f5").intensity).toBeGreaterThanOrEqual(0.7);
  });

  it("is genuinely dark at night, not just a slightly dimmer day (parent review: 'a genuinely dark night palette')", () => {
    expect(sunLight({ azimuthDeg: 0, altitudeDeg: -40 }, "night", "#2a3550").intensity).toBeLessThanOrEqual(0.1);
  });

  it("never drops to exactly zero, even at night (buildings never go fully flat)", () => {
    expect(sunLight({ azimuthDeg: 0, altitudeDeg: -80 }, "night", "#2a3550").intensity).toBeGreaterThan(0);
  });

  it("never exceeds MapLibre's own valid intensity range (0-1)", () => {
    for (const phase of ["day", "golden", "blue", "night"] as const) {
      const light = sunLight({ azimuthDeg: 0, altitudeDeg: 45 }, phase, "#ffffff");
      expect(light.intensity).toBeGreaterThan(0);
      expect(light.intensity).toBeLessThanOrEqual(1);
    }
  });

  it("anchors to the map, so the light direction is geographic, not tied to the viewport", () => {
    expect(sunLight({ azimuthDeg: 0, altitudeDeg: 30 }, "day", "#cfe0f5").anchor).toBe("map");
  });
});
// ds-allow-hardcode:end
