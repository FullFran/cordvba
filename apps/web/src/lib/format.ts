/**
 * Display formatting for contract `Value`s (issue #119, locale-aware since
 * issue #124): the contract's `unit` field is a UCUM-ish code meant for
 * machines (`Cel`, `%`, `m/s`, `deg`, `hPa`, `ica`), not for display. This
 * module is the single place that turns a code + number into the text a
 * person reads, so no component prints a raw unit code, an unrounded
 * float, a wrong-locale decimal separator, or the literal string "null"
 * again.
 */
import { INTL_TAG } from "../i18n/dictionary";
import { DICTIONARIES } from "../i18n/dictionaries";
import type { Locale } from "../i18n/types";

const UNIT_DISPLAY: Record<string, string> = {
  Cel: "°C",
  "%": "%",
  hPa: "hPa",
};

/** Wind speed and a few other measurements read better rounded to 1 decimal. */
const ONE_DECIMAL_UNITS = new Set(["m/s"]);

function formatNumber(value: number, locale: Locale, options?: Intl.NumberFormatOptions): string {
  // A data-console readout (temperature, pressure, an index) is never
  // grouped by thousands — grouping is for large sums of money or
  // populations, and it would misread as a decimal in `es-ES`, where the
  // grouping separator IS a dot.
  return new Intl.NumberFormat(INTL_TAG[locale], { useGrouping: false, ...options }).format(value);
}

/**
 * Formats a contract `Value.value` + `Value.unit` pair for display, in the
 * given locale (`es-ES` decimal comma, `en-GB`, issue #124). `null` (the
 * source reported no data) renders as an em dash, never the literal word
 * "null". Units with no display mapping pass through unchanged rather than
 * throwing, so an unexpected future unit degrades gracefully.
 */
export function formatValue(value: number | string | null, unit: string, locale: Locale = "en"): string {
  if (value === null) {
    return "—";
  }

  if (typeof value === "number") {
    const displayUnit = UNIT_DISPLAY[unit] ?? unit;
    const numberStr = ONE_DECIMAL_UNITS.has(unit)
      ? formatNumber(value, locale, { minimumFractionDigits: 1, maximumFractionDigits: 1 })
      : formatNumber(value, locale);
    return `${numberStr} ${displayUnit}`;
  }

  return `${value} ${unit}`;
}

/** Maps a wind-direction degree value to its 16-point compass label, in the given locale (N/NE/…/NNW in English, N/NE/…/NNO in Spanish). */
export function formatCardinalDirection(deg: number, locale: Locale = "en"): string {
  const normalized = ((deg % 360) + 360) % 360;
  // Provably 0..15 (the % 16 above), but `cardinals` is a 16-tuple indexed
  // by a non-literal number, so noUncheckedIndexedAccess still widens the
  // result to `string | undefined` — the "N" fallback is unreachable.
  const index = Math.round(normalized / 22.5) % 16;
  return DICTIONARIES[locale].cardinals[index] ?? DICTIONARIES[locale].cardinals[0];
}

/**
 * Combines the cardinal direction with the rounded degree value, e.g.
 * "E 100°" (issue #119): a bare `250 deg` told nobody which way the wind
 * blew without doing trigonometry in their head.
 */
export function formatWindDirection(deg: number | string | null, locale: Locale = "en"): string {
  if (deg === null || typeof deg !== "number") {
    return "—";
  }
  const rounded = Math.round(deg);
  return `${formatCardinalDirection(deg, locale)} ${rounded}°`;
}

/**
 * A rounded degree value with no space before the `°` (e.g. "42°"),
 * matching `formatWindDirection`'s existing convention rather than
 * `formatValue`'s "number space unit" shape (which would read as the
 * ungainly "42 °").
 */
export function formatDegrees(value: number): string {
  return `${Math.round(value)}°`;
}

/**
 * Córdoba's own local clock time (issue #139's sun widget: sunrise, solar
 * noon, sunset — all facts about a place, shown in that place's own time
 * regardless of the visitor's browser timezone, the same reasoning
 * `Intl.DateTimeFormat` needs an explicit `timeZone` for). `null` (a
 * polar day/night with no sunrise/sunset) renders as an em dash, never
 * "Invalid Date".
 */
export function formatClockTime(date: Date | null, locale: Locale = "en"): string {
  if (date === null) {
    return "—";
  }
  return new Intl.DateTimeFormat(INTL_TAG[locale], {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
    timeZone: "Europe/Madrid",
  }).format(date);
}

/**
 * MITECO's ICA category slug as words, in the given locale (issue #119,
 * #124): English words for `en`, MITECO's own Spanish wording for `es`
 * (`category_source` on the API value also keeps that Spanish wording,
 * verbatim, for the provenance panel — this is the same words, used for
 * the on-map/on-tile category text instead).
 */
export function formatAirQualityCategory(category: string, locale: Locale = "en"): string {
  const words = DICTIONARIES[locale].airQualityCategories as Record<string, string>;
  return words[category] ?? category.replace(/_/g, " ");
}
