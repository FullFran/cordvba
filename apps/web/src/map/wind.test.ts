import { describe, expect, it } from "vitest";

import { windVelocity } from "./wind";

describe("windVelocity", () => {
  it("scales magnitude with speed", () => {
    const slow = windVelocity(45, 1);
    const fast = windVelocity(45, 4);
    const magnitude = (v: { vx: number; vy: number }) => Math.hypot(v.vx, v.vy);
    expect(magnitude(fast)).toBeCloseTo(magnitude(slow) * 4, 5);
  });

  it("has no horizontal component when the direction is due north/south (0°/180°)", () => {
    expect(windVelocity(0, 5).vx).toBeCloseTo(0, 5);
    expect(windVelocity(180, 5).vx).toBeCloseTo(0, 5);
  });

  it("has no vertical component when the direction is due east/west (90°/270°)", () => {
    expect(windVelocity(90, 5).vy).toBeCloseTo(0, 5);
    expect(windVelocity(270, 5).vy).toBeCloseTo(0, 5);
  });

  it("defaults to a gentle fallback speed when no reading is available, instead of NaN", () => {
    const v = windVelocity(null, null);
    expect(Number.isFinite(v.vx)).toBe(true);
    expect(Number.isFinite(v.vy)).toBe(true);
  });

  it("treats a string value the same as no reading (defends against an unexpected contract shape)", () => {
    const v = windVelocity(90, "calm");
    expect(Number.isFinite(v.vx)).toBe(true);
    expect(Number.isFinite(v.vy)).toBe(true);
  });
});
