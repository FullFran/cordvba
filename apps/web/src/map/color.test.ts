import { describe, expect, it } from "vitest";

import { invertColorLightness, isColorString, parseColor, toHslString } from "./color";

// ds-allow-hardcode:start
// This whole file's job is parsing/inverting literal colour strings, so
// every hex/rgb/hsl literal below is test input data, not a design token.
describe("parseColor", () => {
  it("parses a 6-digit hex colour", () => {
    expect(parseColor("#f8f4f0")).toMatchObject({ h: expect.closeTo(30, 0), s: expect.closeTo(36, 0) });
  });

  it("parses a 3-digit hex colour", () => {
    expect(parseColor("#fff")).toMatchObject({ h: 0, s: 0, l: 100 });
  });

  it("parses an rgb() colour", () => {
    expect(parseColor("rgb(158,189,255)")).not.toBeNull();
  });

  it("parses an rgba() colour and keeps its alpha", () => {
    const parsed = parseColor("rgba(10, 20, 30, 0.5)");
    expect(parsed?.a).toBeCloseTo(0.5);
  });

  it("parses an hsl() colour", () => {
    expect(parseColor("hsl(35,8%,85%)")).toMatchObject({ h: 35, s: 8, l: 85, a: 1 });
  });

  it("returns null for a string that is not a colour, e.g. a filter literal", () => {
    expect(parseColor("tunnel")).toBeNull();
    expect(parseColor("building")).toBeNull();
  });
});

describe("isColorString", () => {
  it("recognises hex, rgb and hsl forms", () => {
    expect(isColorString("#fff")).toBe(true);
    expect(isColorString("rgb(1,2,3)")).toBe(true);
    expect(isColorString("hsl(1,2%,3%)")).toBe(true);
  });

  it("rejects plain identifiers used elsewhere in a style (source-layer names, filter literals)", () => {
    expect(isColorString("water")).toBe(false);
    expect(isColorString("openmaptiles")).toBe(false);
  });
});

describe("invertColorLightness", () => {
  it("turns a near-white background into a near-black one", () => {
    const dark = invertColorLightness("#f8f4f0");
    const parsed = parseColor(dark);
    expect(parsed?.l).toBeLessThan(20);
  });

  it("leaves a mid-lightness colour close to where it started", () => {
    const result = invertColorLightness("hsl(200,50%,50%)");
    const parsed = parseColor(result);
    expect(parsed?.l).toBeCloseTo(50, 0);
  });

  it("preserves hue and saturation, only flipping lightness", () => {
    const result = invertColorLightness("hsl(200,60%,80%)");
    const parsed = parseColor(result);
    expect(parsed?.h).toBeCloseTo(200, 0);
    expect(parsed?.s).toBeCloseTo(60, 0);
    expect(parsed?.l).toBeCloseTo(20, 0);
  });

  it("preserves alpha", () => {
    const result = invertColorLightness("rgba(255,255,255,0.4)");
    const parsed = parseColor(result);
    expect(parsed?.a).toBeCloseTo(0.4);
  });

  it("returns the input unchanged when it is not a recognisable colour", () => {
    expect(invertColorLightness("tunnel")).toBe("tunnel");
  });
});

describe("toHslString", () => {
  it("round-trips a fully opaque colour without an alpha channel in the output", () => {
    expect(toHslString({ h: 200, s: 50, l: 50, a: 1 })).toBe("hsl(200,50%,50%)");
  });

  it("uses hsla() when alpha is not 1", () => {
    expect(toHslString({ h: 200, s: 50, l: 50, a: 0.5 })).toBe("hsla(200,50%,50%,0.5)");
  });
});
// ds-allow-hardcode:end
