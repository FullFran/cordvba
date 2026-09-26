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

export type SunPhase = "day" | "dusk" | "night";

/** Civil twilight (sun within 6° of the horizon) reads as "dusk"; the rest of the day/night cycle is a plain binary (issue #139: drives the map's day/dusk/night palette and light). */
const CIVIL_TWILIGHT_DEG = 6;

/** Categorises an altitude into the three lighting phases this app's palette and building-shadow layer key off. */
export function sunPhase(altitudeDeg: number): SunPhase {
  if (altitudeDeg > CIVIL_TWILIGHT_DEG) {
    return "day";
  }
  if (altitudeDeg > -CIVIL_TWILIGHT_DEG) {
    return "dusk";
  }
  return "night";
}
