/**
 * Sun position for Córdoba (issue #139 — "Córdoba bajo el sol"): the
 * timeline's selected time becomes a real astronomical fact, driving the
 * map's lighting, its day/dusk/night palette and the historic centre's
 * inferred building shadows.
 *
 * Backed by `suncalc` (BSD-2-Clause, ~4.6 KB gzipped as the single
 * `suncalc.cjs` module this app imports — see the PR/report for the exact
 * numbers): a small, widely used, from-scratch implementation of the same
 * family of solar-position formulas NOAA's own solar calculator uses.
 * `suncalc@2.0.2` already returns azimuth as a north-based, clockwise
 * compass bearing in degrees and altitude in degrees above the horizon —
 * both used here unchanged, with no radian/south-based conversion (that
 * was suncalc's pre-2.0 API).
 */
import * as SunCalc from "suncalc";

export interface SunPosition {
  /** Compass bearing the sun is in, 0-360, 0 = north, clockwise. */
  azimuthDeg: number;
  /** Degrees above the horizon; negative when the sun has set. */
  altitudeDeg: number;
}

export interface SunTimes {
  sunrise: Date | null;
  solarNoon: Date;
  sunset: Date | null;
}

function wrapDeg(deg: number): number {
  return ((deg % 360) + 360) % 360;
}

/** The sun's azimuth/altitude at `date` for the given coordinates. */
export function getSunPosition(date: Date, latDeg: number, lonDeg: number): SunPosition {
  const position = SunCalc.getPosition(date, latDeg, lonDeg);
  return {
    azimuthDeg: wrapDeg(position.azimuth),
    altitudeDeg: position.altitude,
  };
}

/** Sunrise, solar noon and sunset for `date`'s calendar day at the given coordinates — the sun widget's three headline times (issue #139). */
export function getSunTimes(date: Date, latDeg: number, lonDeg: number): SunTimes {
  const times = SunCalc.getTimes(date, latDeg, lonDeg);
  return {
    sunrise: times.sunrise ?? null,
    solarNoon: times.solarNoon,
    sunset: times.sunset ?? null,
  };
}

/**
 * Four phases, not three (parent review: "the sun is barely noticeable
 * ... noon vs dusk differ by a faint tint"). A flat day/dusk/night split
 * treated a wide, visually distinct range of low-sun lighting as one
 * "dusk" bucket; photographers already have names — and a strong colour
 * language — for exactly this range: golden hour (warm amber, low but
 * still well above the horizon) and blue hour (cool blue, straddling the
 * horizon itself). Splitting them out gives the map's lighting two more
 * genuinely different states to move through, not just a fixed endpoint
 * pair.
 */
export type SunPhase = "day" | "golden" | "blue" | "night";

/** Below this altitude, daylight starts reading as low-angle golden-hour light. */
const GOLDEN_HOUR_MAX_ALTITUDE_DEG = 20;
/** Below this altitude (still comfortably above civil twilight), the light has crossed into the cooler blue-hour band. */
const BLUE_HOUR_MAX_ALTITUDE_DEG = 2;
/** Civil twilight's own bound: past this, the sun contributes negligible direct light — genuinely night. */
const NIGHT_MAX_ALTITUDE_DEG = -6;

/** Categorises an altitude into the four lighting phases this app's palette, light and building-shadow layer key off. */
export function sunPhase(altitudeDeg: number): SunPhase {
  if (altitudeDeg <= NIGHT_MAX_ALTITUDE_DEG) {
    return "night";
  }
  if (altitudeDeg <= BLUE_HOUR_MAX_ALTITUDE_DEG) {
    return "blue";
  }
  if (altitudeDeg <= GOLDEN_HOUR_MAX_ALTITUDE_DEG) {
    return "golden";
  }
  return "day";
}
