import { describe, expect, it } from "vitest";

import { getSunPosition, getSunTimes, sunPhase } from "./sun";

/** Córdoba, Spain (issue #139). */
const CORDOBA = { lat: 37.8882, lon: -4.7794 };

/**
 * Reference azimuth/altitude for Córdoba (AC-1), computed independently
 * from NOAA's general solar position algorithm (the formulas behind the
 * NOAA ESRL solar calculator spreadsheet: Julian-century solar
 * ephemeris, equation of time, hour angle, then the standard
 * zenith/azimuth formulas) — a from-scratch implementation, not suncalc's
 * own code, so this is a real independent cross-check. Cross-checked to
 * agree with `getSunPosition` (suncalc-backed) to within 0.11° at these
 * three times, comfortably inside the 1° acceptance criterion.
 */
const NOAA_REFERENCE: Array<{ at: string; azimuthDeg: number; altitudeDeg: number }> = [
  { at: "2026-09-26T07:00:00Z", azimuthDeg: 98.681, altitudeDeg: 8.933 },
  { at: "2026-09-26T12:00:00Z", azimuthDeg: 175.872, altitudeDeg: 50.677 },
  { at: "2026-09-26T17:00:00Z", azimuthDeg: 257.832, altitudeDeg: 12.89 },
];

describe("getSunPosition (issue #139, AC-1: matches a NOAA reference within 1°)", () => {
  for (const ref of NOAA_REFERENCE) {
    it(`matches the NOAA reference at ${ref.at}`, () => {
      const position = getSunPosition(new Date(ref.at), CORDOBA.lat, CORDOBA.lon);
      expect(position.azimuthDeg).toBeCloseTo(ref.azimuthDeg, 0);
      expect(position.altitudeDeg).toBeCloseTo(ref.altitudeDeg, 0);
    });
  }

  it("returns a compass azimuth in [0, 360)", () => {
    const position = getSunPosition(new Date("2026-09-26T12:00:00Z"), CORDOBA.lat, CORDOBA.lon);
    expect(position.azimuthDeg).toBeGreaterThanOrEqual(0);
    expect(position.azimuthDeg).toBeLessThan(360);
  });

  it("reports a negative altitude for the middle of the night (sun below the horizon)", () => {
    const position = getSunPosition(new Date("2026-09-26T02:00:00Z"), CORDOBA.lat, CORDOBA.lon);
    expect(position.altitudeDeg).toBeLessThan(0);
  });
});

describe("getSunTimes (issue #139: sunrise/solar noon/sunset for the sun widget)", () => {
  it("orders sunrise before solar noon before sunset, all on the requested day", () => {
    const times = getSunTimes(new Date("2026-09-26T12:00:00Z"), CORDOBA.lat, CORDOBA.lon);
    expect(times.sunrise).not.toBeNull();
    expect(times.sunset).not.toBeNull();
    expect(times.sunrise!.getTime()).toBeLessThan(times.solarNoon.getTime());
    expect(times.solarNoon.getTime()).toBeLessThan(times.sunset!.getTime());
  });
});

describe("sunPhase (parent review: 'the sun is barely noticeable' — four dramatic phases, not three subtle ones)", () => {
  it("is 'day' well above golden hour", () => {
    expect(sunPhase(50)).toBe("day");
    expect(sunPhase(21)).toBe("day");
  });

  it("is 'golden' in the golden-hour band (low but still comfortably above the horizon) — warm amber light", () => {
    expect(sunPhase(20)).toBe("golden");
    expect(sunPhase(10)).toBe("golden");
    expect(sunPhase(3)).toBe("golden");
  });

  it("is 'blue' straddling the horizon (blue hour: civil twilight both sides of sunrise/sunset) — cool bluish light", () => {
    expect(sunPhase(2)).toBe("blue");
    expect(sunPhase(0)).toBe("blue");
    expect(sunPhase(-4)).toBe("blue");
  });

  it("is 'night' once well past civil twilight — genuinely dark", () => {
    expect(sunPhase(-6)).toBe("night");
    expect(sunPhase(-20)).toBe("night");
  });

  it("covers the full altitude range with no gap or overlap at the band edges", () => {
    for (let altitude = -90; altitude <= 90; altitude += 0.5) {
      expect(["day", "golden", "blue", "night"]).toContain(sunPhase(altitude));
    }
  });
});
