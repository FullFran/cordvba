import { describe, expect, it } from "vitest";

import { categoryShape, createBeaconElement, createEdgeIndicatorElement, windArrowRotation } from "./beacons";

describe("categoryShape", () => {
  it("gives every ICA category a distinct shape, softest for good and sharpest for extremely poor", () => {
    const categories = ["good", "fair", "moderate", "poor", "very_poor", "extremely_poor"] as const;
    const shapes = categories.map(categoryShape);
    expect(new Set(shapes).size).toBe(categories.length);
    expect(categoryShape("good")).toBe("circle");
    expect(categoryShape("extremely_poor")).toBe("star");
  });

  it("falls back to a generic shape for an unrecognised category, instead of throwing", () => {
    expect(() => categoryShape("unknown-category")).not.toThrow();
  });
});

describe("windArrowRotation", () => {
  it("rotates the arrow to the reported direction in degrees, unchanged (issue 102's convention, kept as-is)", () => {
    expect(windArrowRotation(0)).toBe(0);
    expect(windArrowRotation(250)).toBe(250);
  });

  it("wraps into [0, 360)", () => {
    expect(windArrowRotation(370)).toBe(10);
    expect(windArrowRotation(-10)).toBe(350);
  });

  it("returns 0 for a null direction rather than NaN", () => {
    expect(windArrowRotation(null)).toBe(0);
  });
});

describe("createBeaconElement", () => {
  it("shows the ICA index and category as text inside the beacon, not colour alone", () => {
    const el = createBeaconElement({
      kind: "air-quality",
      name: "ASOMADILLA",
      category: "moderate",
      index: 3,
      highlighted: false,
    });

    expect(el.textContent).toContain("3");
    expect(el.textContent?.toLowerCase()).toContain("moderate");
    expect(el.className).toContain("beacon--moderate");
    expect(el.getAttribute("aria-label")).toMatch(/ASOMADILLA.*moderate/i);
  });

  it("marks a highlighted beacon distinctly (from a selected timeline point)", () => {
    const el = createBeaconElement({
      kind: "air-quality",
      name: "LEPANTO",
      category: "poor",
      index: 4,
      highlighted: true,
    });

    expect(el.className).toContain("beacon--highlighted");
  });

  it("renders category and aria-label text in Spanish when given the es locale (issue 124)", () => {
    const el = createBeaconElement(
      { kind: "air-quality", name: "ASOMADILLA", category: "very_poor", index: 5, highlighted: false },
      "es",
    );

    expect(el.textContent).toContain("muy desfavorable");
    expect(el.getAttribute("aria-label")).toMatch(/calidad del aire/i);
  });

  it("renders a wind beacon with the rotated arrow and formatted speed/direction", () => {
    const el = createBeaconElement({
      kind: "wind",
      name: "Córdoba Airport",
      directionDeg: 250,
      speedMs: 4.1,
    });

    expect(el.textContent).toContain("WSW");
    expect(el.textContent).toContain("4.1");
    const arrow = el.querySelector<HTMLElement>(".beacon__arrow");
    expect(arrow?.style.transform).toContain("250deg");
  });
});

describe("createEdgeIndicatorElement", () => {
  it("shows the rounded distance in km and an aria-label naming the station and distance", () => {
    const el = createEdgeIndicatorElement("Córdoba Airport", 8.3, 0);

    expect(el.textContent).toContain("8 km");
    expect(el.getAttribute("aria-label")).toMatch(/Córdoba Airport.*8 km away/i);
    expect(el.className).toContain("beacon--edge");
  });

  it("localises the label in Spanish", () => {
    const el = createEdgeIndicatorElement("Aeropuerto de Córdoba", 8.3, 0, "es");
    expect(el.getAttribute("aria-label")).toMatch(/a 8 km/i);
  });
});
