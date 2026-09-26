import { describe, expect, it } from "vitest";

import {
  formatAirQualityCategory,
  formatCardinalDirection,
  formatClockTime,
  formatDegrees,
  formatValue,
  formatWindDirection,
} from "./format";

describe("formatValue", () => {
  it("formats a Celsius value with the degree symbol, not the raw unit code (issue 119)", () => {
    expect(formatValue(33, "Cel")).toBe("33 °C");
  });

  it("formats a fractional Celsius value without inventing precision the source never had", () => {
    expect(formatValue(23.7, "Cel")).toBe("23.7 °C");
  });

  it("formats a percentage with a space before the % sign", () => {
    expect(formatValue(41.7, "%")).toBe("41.7 %");
  });

  it("rounds a wind speed in m/s to one decimal", () => {
    expect(formatValue(2.57222, "m/s")).toBe("2.6 m/s");
  });

  it("keeps a wind speed that already has one decimal unchanged", () => {
    expect(formatValue(4.1, "m/s")).toBe("4.1 m/s");
  });

  it("formats hPa unchanged", () => {
    expect(formatValue(1013, "hPa")).toBe("1013 hPa");
  });

  it("renders a null value as an em dash, never as the literal word 'null'", () => {
    expect(formatValue(null, "Cel")).toBe("—");
  });

  it("passes an unrecognised unit through unchanged rather than throwing", () => {
    expect(formatValue(5, "widgets")).toBe("5 widgets");
  });

  it("formats a string value (e.g. a category code) by returning it with its unit suffix", () => {
    expect(formatValue("ok", "status")).toBe("ok status");
  });
});

describe("formatValue: es-ES locale (issue 124, AC-4)", () => {
  it("uses a decimal comma instead of a decimal point", () => {
    expect(formatValue(23.7, "Cel", "es")).toBe("23,7 °C");
  });

  it("rounds a wind speed to one decimal, comma-separated", () => {
    expect(formatValue(2.57222, "m/s", "es")).toBe("2,6 m/s");
  });

  it("never groups a 4-digit reading by thousands (es-ES groups with a dot, which would misread as a decimal)", () => {
    expect(formatValue(1013, "hPa", "es")).toBe("1013 hPa");
  });
});

describe("formatWindDirection", () => {
  it("combines the 16-point cardinal direction with the rounded degree value (issue 119: 'E 100°')", () => {
    expect(formatWindDirection(100)).toBe("E 100°");
  });

  it("formats the contract example direction (250°, WSW)", () => {
    expect(formatWindDirection(250)).toBe("WSW 250°");
  });

  it("rounds a fractional degree before display", () => {
    expect(formatWindDirection(99.6)).toBe("E 100°");
  });

  it("renders a null direction as an em dash", () => {
    expect(formatWindDirection(null)).toBe("—");
  });
});

describe("formatCardinalDirection", () => {
  it("maps 0 degrees to N", () => {
    expect(formatCardinalDirection(0)).toBe("N");
  });

  it("wraps 360 degrees back to N", () => {
    expect(formatCardinalDirection(360)).toBe("N");
  });

  it("maps every 16-point boundary to its compass label", () => {
    const expected: Array<[number, string]> = [
      [0, "N"],
      [22.5, "NNE"],
      [45, "NE"],
      [67.5, "ENE"],
      [90, "E"],
      [112.5, "ESE"],
      [135, "SE"],
      [157.5, "SSE"],
      [180, "S"],
      [202.5, "SSW"],
      [225, "SW"],
      [247.5, "WSW"],
      [270, "W"],
      [292.5, "WNW"],
      [315, "NW"],
      [337.5, "NNW"],
    ];
    for (const [deg, label] of expected) {
      expect(formatCardinalDirection(deg)).toBe(label);
    }
  });
});

describe("formatAirQualityCategory", () => {
  it("passes single-word categories through unchanged", () => {
    expect(formatAirQualityCategory("good")).toBe("good");
    expect(formatAirQualityCategory("fair")).toBe("fair");
    expect(formatAirQualityCategory("moderate")).toBe("moderate");
    expect(formatAirQualityCategory("poor")).toBe("poor");
  });

  it("turns the underscore-joined categories into words (issue 119)", () => {
    expect(formatAirQualityCategory("very_poor")).toBe("very poor");
    expect(formatAirQualityCategory("extremely_poor")).toBe("extremely poor");
  });

  it("uses MITECO's own Spanish wording in the es locale (issue 124)", () => {
    expect(formatAirQualityCategory("good", "es")).toBe("buena");
    expect(formatAirQualityCategory("moderate", "es")).toBe("regular");
    expect(formatAirQualityCategory("very_poor", "es")).toBe("muy desfavorable");
    expect(formatAirQualityCategory("extremely_poor", "es")).toBe("extremadamente desfavorable");
  });
});

describe("formatCardinalDirection / formatWindDirection: es locale (issue 124)", () => {
  it("uses the Spanish 16-point compass (O instead of W, SO instead of SW, …)", () => {
    expect(formatCardinalDirection(250, "es")).toBe("OSO");
    expect(formatCardinalDirection(270, "es")).toBe("O");
    expect(formatCardinalDirection(315, "es")).toBe("NO");
  });

  it("combines the Spanish cardinal with the degree value", () => {
    expect(formatWindDirection(250, "es")).toBe("OSO 250°");
  });
});

describe("formatDegrees (issue #139: the sun widget's altitude, no space before °)", () => {
  it("rounds to the nearest whole degree", () => {
    expect(formatDegrees(42.3)).toBe("42°");
    expect(formatDegrees(42.6)).toBe("43°");
  });

  it("has no space before the ° sign, matching formatWindDirection's convention", () => {
    expect(formatDegrees(0)).toBe("0°");
  });
});

describe("formatClockTime (issue #139: sunrise/solar-noon/sunset in Córdoba's own local time)", () => {
  it("formats a fixed UTC instant in Europe/Madrid time (CEST, UTC+2, in late September)", () => {
    expect(formatClockTime(new Date("2026-09-26T05:57:00Z"))).toBe("07:57");
  });

  it("returns an em dash for a null date (a polar day/night with no sunrise/sunset), not 'Invalid Date'", () => {
    expect(formatClockTime(null)).toBe("—");
  });
});
