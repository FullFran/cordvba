import { INTL_TAG } from "../i18n/dictionary";
import { DICTIONARIES } from "../i18n/dictionaries";
import { formatTemplate } from "../i18n/template";
import type { Locale } from "../i18n/types";

function n(value: number, locale: Locale): string {
  return new Intl.NumberFormat(INTL_TAG[locale], { useGrouping: false }).format(value);
}

/**
 * Formats the age of a value's `at` timestamp relative to `now`, for the
 * state panel (issue #102: "each with its label ... and age"), in the
 * given locale (issue #124, AC-4). A future `at` (a prediction not yet
 * due) is shown as "in X h" instead of a negative age.
 */
export function formatAge(at: string, now: Date = new Date(), locale: Locale = "en"): string {
  const diffMs = now.getTime() - new Date(at).getTime();
  const t = DICTIONARIES[locale].age;

  if (diffMs < 0) {
    const hoursAhead = Math.round(-diffMs / (60 * 60 * 1000));
    return formatTemplate(t.inH, { n: n(hoursAhead, locale) });
  }

  const totalMinutes = Math.floor(diffMs / (60 * 1000));
  if (totalMinutes < 1) {
    return t.justNow;
  }
  if (totalMinutes < 60) {
    return formatTemplate(t.minAgo, { n: n(totalMinutes, locale) });
  }

  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  return minutes === 0
    ? formatTemplate(t.hAgo, { n: n(hours, locale) })
    : formatTemplate(t.hMinAgo, { h: n(hours, locale), m: n(minutes, locale) });
}
