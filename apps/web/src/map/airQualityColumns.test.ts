import { describe, expect, it } from "vitest";

import { discPolygonCoordinates, icaColumnHeightM, stationColumnFeature } from "./airQualityColumns";

describe("icaColumnHeightM (issue #140: column height encodes the ICA index, nothing interpolated)", () => {
  it("gives a taller column to a worse (higher) ICA index", () => {
    expect(icaColumnHeightM(6)).toBeGreaterThan(icaColumnHeightM(1));
  });

  it("is monotonically increasing across the full 1-6 range", () => {
    const heights = [1, 2, 3, 4, 5, 6].map(icaColumnHeightM);
    for (let i = 1; i < heights.length; i++) {
      expect(heights[i]).toBeGreaterThan(heights[i - 1]!);
    }
  });

  it("gives a modest, non-zero height for a missing/non-numeric index, rather than vanishing", () => {
    expect(icaColumnHeightM(null)).toBeGreaterThan(0);
    expect(icaColumnHeightM("unknown")).toBeGreaterThan(0);
  });
});

describe("discPolygonCoordinates (issue #140: a small closed ring around a station, for the extruded column footprint)", () => {
  it("returns a closed ring (first point repeats at the end)", () => {
    const ring = discPolygonCoordinates({ lat: 37.9, lon: -4.78 }, 12, 8);
    expect(ring[0]).toEqual(ring[ring.length - 1]);
  });

  it("returns the requested number of distinct vertices plus the closing point", () => {
    const ring = discPolygonCoordinates({ lat: 37.9, lon: -4.78 }, 12, 16);
    expect(ring).toHaveLength(17);
  });

  it("keeps every vertex within roughly the given radius of the centre", () => {
    const center = { lat: 37.9, lon: -4.78 };
    const radiusM = 12;
    const ring = discPolygonCoordinates(center, radiusM, 16);
    // ~1 degree of latitude is ~111.32km; a generous tolerance covers the flat-earth approximation.
    const maxDegreeSpan = (radiusM / 111_320) * 1.05;
    for (const [lon, lat] of ring) {
      expect(Math.abs(lat - center.lat)).toBeLessThan(maxDegreeSpan);
      expect(Math.abs(lon - center.lon)).toBeLessThan(maxDegreeSpan * 2); // longitude spans more at this latitude
    }
  });
});

describe("stationColumnFeature (issue #140: one GeoJSON Feature per station, never an interpolated surface between them)", () => {
  it("carries the station's ICA-driven height and category as feature properties", () => {
    const feature = stationColumnFeature({
      lat: 37.9,
      lon: -4.78,
      category: "poor",
      index: 4,
    });
    expect(feature.type).toBe("Feature");
    expect(feature.geometry.type).toBe("Polygon");
    expect(feature.properties?.category).toBe("poor");
    expect(feature.properties?.height).toBe(icaColumnHeightM(4));
  });

  it("produces one independent, unconnected polygon per call (never a shared/merged surface)", () => {
    const a = stationColumnFeature({ lat: 37.9, lon: -4.78, category: "good", index: 1 });
    const b = stationColumnFeature({ lat: 37.89, lon: -4.76, category: "good", index: 1 });
    expect(a.geometry.coordinates).not.toEqual(b.geometry.coordinates);
  });
});
