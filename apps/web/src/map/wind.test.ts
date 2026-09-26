import { describe, expect, it } from "vitest";

import { advectParticle, geographicWindStep, windBearingTo, wrapWithinBounds } from "./wind";

const CORDOBA_LAT = 37.8882;

describe("windBearingTo (issue #138: meteorological FROM vs. flow TO)", () => {
  it("flips the reported FROM direction to the TO bearing the flow actually moves along", () => {
    expect(windBearingTo(310)).toBeCloseTo(130, 5);
    expect(windBearingTo(0)).toBeCloseTo(180, 5);
    expect(windBearingTo(180)).toBeCloseTo(0, 5);
  });

  it("treats a missing reading as calm from the north, not NaN", () => {
    expect(Number.isFinite(windBearingTo(null))).toBe(true);
  });
});

describe("geographicWindStep (issue #138: advect in lng/lat, not screen-space pixels)", () => {
  it("moves a 310° (from the WNW) wind's particle south-east in lng/lat", () => {
    const step = geographicWindStep(310, 5, 1, CORDOBA_LAT);
    expect(step.dLat).toBeLessThan(0); // south
    expect(step.dLng).toBeGreaterThan(0); // east
  });

  it("moves a due-north-from wind's particle due south, with no east/west drift", () => {
    const step = geographicWindStep(0, 5, 1, CORDOBA_LAT);
    expect(step.dLat).toBeLessThan(0);
    expect(step.dLng).toBeCloseTo(0, 8);
  });

  it("moves a due-east-from wind's particle due west, with no north/south drift", () => {
    const step = geographicWindStep(90, 5, 1, CORDOBA_LAT);
    expect(step.dLng).toBeLessThan(0);
    expect(step.dLat).toBeCloseTo(0, 8);
  });

  it("scales the step's magnitude with speed and elapsed time", () => {
    const slow = geographicWindStep(310, 5, 1, CORDOBA_LAT);
    const fast = geographicWindStep(310, 10, 1, CORDOBA_LAT);
    const doubleTime = geographicWindStep(310, 5, 2, CORDOBA_LAT);
    const magnitude = (s: { dLng: number; dLat: number }) => Math.hypot(s.dLng, s.dLat);
    expect(magnitude(fast)).toBeCloseTo(magnitude(slow) * 2, 6);
    expect(magnitude(doubleTime)).toBeCloseTo(magnitude(slow) * 2, 6);
  });

  it("defaults to a gentle fallback speed instead of NaN when no reading is available", () => {
    const step = geographicWindStep(null, null, 1, CORDOBA_LAT);
    expect(Number.isFinite(step.dLng)).toBe(true);
    expect(Number.isFinite(step.dLat)).toBe(true);
  });

  it("treats a string value the same as no reading (defends against an unexpected contract shape)", () => {
    const step = geographicWindStep(90, "calm", 1, CORDOBA_LAT);
    expect(Number.isFinite(step.dLng)).toBe(true);
    expect(Number.isFinite(step.dLat)).toBe(true);
  });

  it("shrinks the longitude delta at higher latitudes (cos(lat) correction for converging meridians)", () => {
    const atEquator = geographicWindStep(90, 5, 1, 0);
    const atCordoba = geographicWindStep(90, 5, 1, CORDOBA_LAT);
    expect(Math.abs(atCordoba.dLng)).toBeGreaterThan(Math.abs(atEquator.dLng));
  });
});

describe("wrapWithinBounds (issue #138: geographic torus wrap, replacing the old canvas-pixel wrap)", () => {
  const bounds = { west: -5, east: -4, south: 37, north: 38 };

  it("leaves a point already inside the bounds unchanged", () => {
    expect(wrapWithinBounds({ lng: -4.5, lat: 37.5 }, bounds)).toEqual({ lng: -4.5, lat: 37.5 });
  });

  it("wraps a point past the east edge back in from the west", () => {
    expect(wrapWithinBounds({ lng: -3.9, lat: 37.5 }, bounds).lng).toBeCloseTo(-4.9, 5);
  });

  it("wraps a point past the west edge back in from the east", () => {
    expect(wrapWithinBounds({ lng: -5.2, lat: 37.5 }, bounds).lng).toBeCloseTo(-4.2, 5);
  });

  it("wraps a point past the north edge back in from the south", () => {
    expect(wrapWithinBounds({ lng: -4.5, lat: 38.2 }, bounds).lat).toBeCloseTo(37.2, 5);
  });
});

describe("advectParticle (issue #138: one advect-then-wrap step)", () => {
  it("advects then wraps in a single call, never producing NaN", () => {
    const bounds = { west: -5, east: -4, south: 37, north: 38 };
    const result = advectParticle({ lng: -4.5, lat: 37.5 }, 310, 5, 1, bounds);
    expect(Number.isFinite(result.lng)).toBe(true);
    expect(Number.isFinite(result.lat)).toBe(true);
  });
});

/**
 * Projection-aware check (issue #138): the geographic step above must come
 * out identical whatever the map's current bearing is — only the *screen*
 * position, produced by `map.project()`, should rotate with it. This is
 * exactly the shape of the original bug: the old implementation baked
 * screen-space trigonometry into the step itself, so rotating the map
 * (bearing -35°, the page's own initial value) left the drawn flow off by
 * the bearing. A small bearing-aware mock stands in for MapLibre's real
 * `project()` (which needs a live WebGL context, out of reach here).
 */
function angularDiff(a: number, b: number): number {
  const d = Math.abs(a - b) % 360;
  return d > 180 ? 360 - d : d;
}

function normalizeDeg(deg: number): number {
  return ((deg % 360) + 360) % 360;
}

function mockRotatingMap(bearingDeg: number, atLatDeg: number) {
  const rad = (bearingDeg * Math.PI) / 180;
  // Locally conformal (isotropic metres, like a real Mercator projection is
  // at any given point) rather than a naive equirectangular scale — the
  // same cos(lat) correction `geographicWindStep` itself applies to lng,
  // so a real 90°-apart pair of directions still projects 90° apart here.
  const metersPerDegreeLng = 111_320 * Math.cos((atLatDeg * Math.PI) / 180);
  const metersPerDegreeLat = 111_320;
  return {
    project([lng, lat]: [number, number]) {
      // MapLibre's bearing convention: "up" on screen is the compass
      // direction the bearing names, so a world vector is rotated by
      // -bearing to land on screen.
      const worldX = lng * metersPerDegreeLng;
      const worldY = -lat * metersPerDegreeLat; // screen y grows downward; latitude grows north
      return {
        x: worldX * Math.cos(-rad) - worldY * Math.sin(-rad),
        y: worldX * Math.sin(-rad) + worldY * Math.cos(-rad),
      };
    },
  };
}

describe("map.project() carries the bearing; the geographic step never does (issue #138)", () => {
  it.each([0, -35, 90])(
    "at map bearing %d°, the projected screen bearing equals the flow's compass bearing minus the map bearing",
    (bearing) => {
      const origin = { lng: -4.78, lat: 37.89 };
      const step = geographicWindStep(310, 5, 1, origin.lat);
      const advected = { lng: origin.lng + step.dLng, lat: origin.lat + step.dLat };

      const map = mockRotatingMap(bearing, origin.lat);
      const p0 = map.project([origin.lng, origin.lat]);
      const p1 = map.project([advected.lng, advected.lat]);

      const screenBearing = normalizeDeg((Math.atan2(p1.x - p0.x, -(p1.y - p0.y)) * 180) / Math.PI);
      const expectedScreenBearing = normalizeDeg(windBearingTo(310) - bearing);

      expect(screenBearing).toBeCloseTo(expectedScreenBearing, 0);
    },
  );

  it("produces a different screen direction per bearing for the same wind — proving direction is not baked in screen-space (the original bug)", () => {
    const origin = { lng: -4.78, lat: 37.89 };
    const step = geographicWindStep(310, 5, 1, origin.lat);
    const advected = { lng: origin.lng + step.dLng, lat: origin.lat + step.dLat };

    const screenBearings = [0, -35, 90].map((bearing) => {
      const map = mockRotatingMap(bearing, origin.lat);
      const p0 = map.project([origin.lng, origin.lat]);
      const p1 = map.project([advected.lng, advected.lat]);
      return normalizeDeg((Math.atan2(p1.x - p0.x, -(p1.y - p0.y)) * 180) / Math.PI);
    });

    expect(angularDiff(screenBearings[0]!, screenBearings[1]!)).toBeGreaterThan(30);
    expect(angularDiff(screenBearings[0]!, screenBearings[2]!)).toBeGreaterThan(30);
  });
});
