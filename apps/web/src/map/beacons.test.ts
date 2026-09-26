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

describe("windArrowRotation (issue #138: points where the wind blows TO, corrected for the map's own bearing — supersedes issue #102's original 'no flip' convention)", () => {
  it("rotates the arrow to the flow-TO bearing (direction + 180°) when the map bearing is 0", () => {
    expect(windArrowRotation(0)).toBe(180); // from the north -> blows south
    expect(windArrowRotation(250)).toBe(70); // from the WSW -> blows ENE
  });

  it("subtracts the map's current bearing, since the arrow is drawn in fixed screen space, not rotated with the map canvas", () => {
    // 310° wind blows TO 130°; the page's own initial bearing is -35°.
    expect(windArrowRotation(310, -35)).toBeCloseTo(165, 5);
    expect(windArrowRotation(310, 90)).toBeCloseTo(40, 5);
    expect(windArrowRotation(310, 0)).toBeCloseTo(130, 5);
  });

  it("wraps into [0, 360)", () => {
    expect(windArrowRotation(370)).toBe(190);
    expect(windArrowRotation(-10)).toBe(170);
  });

  it("returns 0 for a null direction rather than NaN, regardless of map bearing", () => {
    expect(windArrowRotation(null)).toBe(0);
    expect(windArrowRotation(null, 90)).toBe(0);
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

  it("renders a wind beacon with the arrow pointing where the wind blows TO and formatted speed/direction", () => {
    const el = createBeaconElement({
      kind: "wind",
      name: "Córdoba Airport",
      directionDeg: 250,
      speedMs: 4.1,
    });

    expect(el.textContent).toContain("WSW");
    expect(el.textContent).toContain("4.1");
    const arrow = el.querySelector<HTMLElement>(".beacon__arrow");
    // 250° (from the WSW) blows TO 70° (ENE); no map bearing given, so no correction.
    expect(arrow?.style.transform).toContain("70deg");
  });

  it("corrects the wind arrow for the map's current bearing (issue #138)", () => {
    const el = createBeaconElement({
      kind: "wind",
      name: "Córdoba Airport",
      directionDeg: 250,
      speedMs: 4.1,
      mapBearingDeg: -35,
    });

    const arrow = el.querySelector<HTMLElement>(".beacon__arrow");
    // blows-TO 70°, minus a -35° map bearing = 105°.
    expect(arrow?.style.transform).toContain("105deg");
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
