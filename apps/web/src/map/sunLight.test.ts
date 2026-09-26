import { describe, expect, it } from "vitest";

import { sunLight } from "./sunLight";

describe("sunLight (issue #139: MapLibre extrusion light driven by the real sun position)", () => {
  it("points straight down (polar 0) when the sun is directly overhead", () => {
    const light = sunLight({ azimuthDeg: 120, altitudeDeg: 90 }, "#cfe0f5");
    expect(light.position[2]).toBeCloseTo(0, 5);
  });

  it("points level with the horizon (polar 90) when the sun is on the horizon", () => {
    const light = sunLight({ azimuthDeg: 120, altitudeDeg: 0 }, "#cfe0f5");
    expect(light.position[2]).toBeCloseTo(90, 5);
  });

  it("carries the sun's own compass azimuth as the light's azimuthal angle", () => {
    const light = sunLight({ azimuthDeg: 257, altitudeDeg: 30 }, "#cfe0f5");
    expect(light.position[1]).toBeCloseTo(257, 5);
  });

  it("passes the given colour straight through, never inventing one", () => {
    const light = sunLight({ azimuthDeg: 0, altitudeDeg: 30 }, "#f2b783");
    expect(light.color).toBe("#f2b783");
  });

  it("is more intense with the sun high than with the sun low", () => {
    const high = sunLight({ azimuthDeg: 0, altitudeDeg: 70 }, "#cfe0f5");
    const low = sunLight({ azimuthDeg: 0, altitudeDeg: 5 }, "#cfe0f5");
    expect(high.intensity).toBeGreaterThan(low.intensity);
  });

  it("never drops below a floor intensity, even at night (buildings never go fully flat)", () => {
    const night = sunLight({ azimuthDeg: 0, altitudeDeg: -40 }, "#6b7ba8");
    expect(night.intensity).toBeGreaterThan(0);
  });

  it("anchors to the map, so the light direction is geographic, not tied to the viewport", () => {
    expect(sunLight({ azimuthDeg: 0, altitudeDeg: 30 }, "#cfe0f5").anchor).toBe("map");
  });
});
