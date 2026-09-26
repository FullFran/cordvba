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
 */
export function SunWidget({ sunrise, solarNoon, sunset, altitudeDeg }: SunWidgetProps) {
  const { locale, t } = useLocale();

  return (
    <section className="sun-widget" aria-label={t.sun.ariaLabel}>
      <p className="sun-widget__line">
        <span className="sun-widget__label">{t.sun.sunrise}</span>
        <span className="sun-widget__value">{formatClockTime(sunrise, locale)}</span>
      </p>
      <p className="sun-widget__line">
        <span className="sun-widget__label">{t.sun.solarNoon}</span>
        <span className="sun-widget__value">{formatClockTime(solarNoon, locale)}</span>
      </p>
      <p className="sun-widget__line">
        <span className="sun-widget__label">{t.sun.sunset}</span>
        <span className="sun-widget__value">{formatClockTime(sunset, locale)}</span>
      </p>
      <p className="sun-widget__line">
        <span className="sun-widget__label">{t.sun.altitude}</span>
        <span className="sun-widget__value">{formatDegrees(altitudeDeg)}</span>
      </p>
    </section>
  );
}
