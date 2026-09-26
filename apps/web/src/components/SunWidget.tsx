import { useLocale } from "../i18n/LocaleContext";
import { formatClockTime, formatDegrees } from "../lib/format";

export interface SunWidgetProps {
  sunrise: Date | null;
  solarNoon: Date;
  sunset: Date | null;
  altitudeDeg: number;
}

/**
 * Sunrise, solar noon, sunset and the sun's current altitude for Córdoba
 * (issue #139) — the small HUD fact behind the map's sun-driven lighting,
 * palette and building shadows, so that is legible as astronomy, not just
 * an ambient effect.
 *
 * A single flowing row of inline items (parent review: "compact the sun
 * widget into one row, so nothing is ever clipped"), not four stacked
 * block lines — text wraps naturally if the dock is ever too narrow,
 * instead of a fixed-height grid that only grows the state dock's own
 * overflow.
 */
export function SunWidget({ sunrise, solarNoon, sunset, altitudeDeg }: SunWidgetProps) {
  const { locale, t } = useLocale();

  const items: Array<{ label: string; value: string }> = [
    { label: t.sun.sunrise, value: formatClockTime(sunrise, locale) },
    { label: t.sun.solarNoon, value: formatClockTime(solarNoon, locale) },
    { label: t.sun.sunset, value: formatClockTime(sunset, locale) },
    { label: t.sun.altitude, value: formatDegrees(altitudeDeg) },
  ];

  return (
    <section className="sun-widget" aria-label={t.sun.ariaLabel}>
      {items.map((item) => (
        <span className="sun-widget__item" key={item.label}>
          <span className="sun-widget__label">{item.label}</span>{" "}
          <span className="sun-widget__value">{item.value}</span>
        </span>
      ))}
    </section>
  );
}
