import { describe, expect, it } from "vitest";

import {
  buildingShadow,
  convexHull,
  footprintFromGeometry,
  shadowBearing,
  shadowLengthM,
  shadowToGeoJSONCoordinates,
} from "./shadows";

describe("shadowBearing (issue #139: a shadow falls directly away from the sun)", () => {
  it("falls south when the sun is due north (azimuth 0)", () => {
    expect(shadowBearing(0)).toBe(180);
  });

  it("falls east when the sun is due west (azimuth 270)", () => {
    expect(shadowBearing(270)).toBe(90);
  });
});

describe("shadowLengthM (height / tan(altitude), the standard gnomon relationship)", () => {
  it("casts a shadow equal to the height at 45° altitude", () => {
    expect(shadowLengthM(10, 45)).toBeCloseTo(10, 5);
  });

  it("casts a longer shadow the lower the sun gets", () => {
    const high = shadowLengthM(10, 60);
    const low = shadowLengthM(10, 20);
    expect(low).toBeGreaterThan(high);
  });

  it("casts no shadow once the sun is at or below the horizon", () => {
    expect(shadowLengthM(10, 0)).toBe(0);
    expect(shadowLengthM(10, -5)).toBe(0);
  });

  it("casts no shadow for a non-positive height", () => {
    expect(shadowLengthM(0, 30)).toBe(0);
    expect(shadowLengthM(-1, 30)).toBe(0);
  });

  it("clamps an absurdly long near-horizon shadow to the given maximum", () => {
    expect(shadowLengthM(10, 0.5, 400)).toBe(400);
  });
});

describe("convexHull", () => {
  it("returns fewer-than-3-point input unchanged", () => {
    const points = [{ lat: 0, lon: 0 }];
    expect(convexHull(points)).toEqual(points);
  });

  it("keeps exactly the 4 corners of an axis-aligned square", () => {
    const square = [
      { lat: 0, lon: 0 },
      { lat: 0, lon: 1 },
      { lat: 1, lon: 1 },
      { lat: 1, lon: 0 },
    ];
    const hull = convexHull(square);
    expect(hull).toHaveLength(4);
    for (const corner of square) {
      expect(hull.some((p) => p.lat === corner.lat && p.lon === corner.lon)).toBe(true);
    }
  });

  it("excludes a point strictly inside the hull of the others", () => {
    const points = [
      { lat: 0, lon: 0 },
      { lat: 0, lon: 10 },
      { lat: 10, lon: 5 },
      { lat: 3, lon: 5 }, // interior point
    ];
    const hull = convexHull(points);
    expect(hull.some((p) => p.lat === 3 && p.lon === 5)).toBe(false);
    expect(hull).toHaveLength(3);
  });
});

describe("buildingShadow (issue #139: footprint ∪ shadow-cast translation, as a convex hull)", () => {
  const squareFootprint = [
    { lat: 37.8885, lon: -4.7796 },
    { lat: 37.8885, lon: -4.7792 },
    { lat: 37.8881, lon: -4.7792 },
    { lat: 37.8881, lon: -4.7796 },
  ];

  it("extends the footprint away from the sun (sun due north -> shadow extends south)", () => {
    const shadow = buildingShadow({ footprint: squareFootprint, heightM: 20 }, { azimuthDeg: 0, altitudeDeg: 45 });
    expect(shadow).not.toBeNull();
    const originalSouthmost = Math.min(...squareFootprint.map((p) => p.lat));
    const shadowSouthmost = Math.min(...shadow!.map((p) => p.lat));
    expect(shadowSouthmost).toBeLessThan(originalSouthmost);
  });

  it("casts no shadow once the sun has set (AC-3: hidden at night)", () => {
    expect(buildingShadow({ footprint: squareFootprint, heightM: 20 }, { azimuthDeg: 0, altitudeDeg: -1 })).toBeNull();
  });

  it("casts no shadow for a building with no recorded height", () => {
    expect(buildingShadow({ footprint: squareFootprint, heightM: 0 }, { azimuthDeg: 0, altitudeDeg: 45 })).toBeNull();
  });
});

describe("footprintFromGeometry (issue #139: reads a queried MapLibre feature's footprint, [lon,lat] -> {lat,lon})", () => {
  it("reads a Polygon's outer ring", () => {
    const footprint = footprintFromGeometry({
      type: "Polygon",
      coordinates: [
        [
          [-4.78, 37.89],
          [-4.779, 37.89],
          [-4.779, 37.888],
          [-4.78, 37.888],
        ],
      ],
    });
    expect(footprint).toEqual([
      { lat: 37.89, lon: -4.78 },
      { lat: 37.89, lon: -4.779 },
      { lat: 37.888, lon: -4.779 },
      { lat: 37.888, lon: -4.78 },
    ]);
  });

  it("reads a MultiPolygon's first polygon's outer ring", () => {
    const footprint = footprintFromGeometry({
      type: "MultiPolygon",
      coordinates: [
        [
          [
            [-4.78, 37.89],
            [-4.779, 37.89],
            [-4.779, 37.888],
          ],
        ],
      ],
    });
    expect(footprint).toEqual([
      { lat: 37.89, lon: -4.78 },
      { lat: 37.89, lon: -4.779 },
      { lat: 37.888, lon: -4.779 },
    ]);
  });

  it("returns null for an unsupported geometry type (e.g. a Point), rather than throwing", () => {
    expect(footprintFromGeometry({ type: "Point", coordinates: [-4.78, 37.89] })).toBeNull();
  });

  it("returns null for a degenerate ring with fewer than 3 points", () => {
    expect(
      footprintFromGeometry({
        type: "Polygon",
        coordinates: [
          [
            [-4.78, 37.89],
            [-4.779, 37.89],
          ],
        ],
      }),
    ).toBeNull();
  });

  it("returns null for null/undefined geometry", () => {
    expect(footprintFromGeometry(null)).toBeNull();
    expect(footprintFromGeometry(undefined)).toBeNull();
  });
});

describe("shadowToGeoJSONCoordinates (issue #139: {lat,lon} -> a closed GeoJSON Polygon ring)", () => {
  it("converts to [lon,lat] pairs and closes the ring", () => {
    const shadow = [
      { lat: 37.89, lon: -4.78 },
      { lat: 37.89, lon: -4.779 },
      { lat: 37.888, lon: -4.779 },
    ];
    const coords = shadowToGeoJSONCoordinates(shadow);
    expect(coords).toEqual([
      [
        [-4.78, 37.89],
        [-4.779, 37.89],
        [-4.779, 37.888],
        [-4.78, 37.89], // closed: last point repeats the first
      ],
    ]);
  });

  it("returns an empty ring for an empty shadow, rather than throwing", () => {
    expect(shadowToGeoJSONCoordinates([])).toEqual([[]]);
  });
});
