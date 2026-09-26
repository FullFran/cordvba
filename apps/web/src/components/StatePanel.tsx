import { useLocale } from "../i18n/LocaleContext";
import { formatAirQualityCategory, formatWindDirection } from "../lib/format";
import { ValueTile } from "./ValueTile";
import type { EnvironmentResponse, Value } from "../types/environment";

export interface StatePanelProps {
  environment: EnvironmentResponse;
  now?: Date;
  onSelect?: (value: Value) => void;
}

/**
 * Temperature, humidity, wind, apparent temperature and city air-quality
 * state, each with its epistemic label and age (issue #102).
 */
export function StatePanel({ environment, now, onSelect }: StatePanelProps) {
  const { weather, air_quality } = environment;
  const { locale, t } = useLocale();

  return (
    <section className="state-panel" aria-label={t.state.ariaLabel}>
      <ValueTile
        name={t.state.temperature}
        value={weather.air_temperature}
        now={now}
        onSelect={onSelect}
      />
      <ValueTile
        name={t.state.humidity}
        value={weather.relative_humidity}
        now={now}
        onSelect={onSelect}
      />
      <ValueTile name={t.state.windSpeed} value={weather.wind_speed} now={now} onSelect={onSelect} />
      <ValueTile
        name={t.state.windDirection}
        value={weather.wind_direction}
        displayValue={
          typeof weather.wind_direction.value === "number"
            ? formatWindDirection(weather.wind_direction.value, locale)
            : undefined
        }
        now={now}
        onSelect={onSelect}
      />
      <ValueTile
        name={t.state.apparentTemperature}
        value={weather.apparent_temperature}
        now={now}
        onSelect={onSelect}
      />
      <ValueTile
        name={t.state.cityAirQuality}
        value={air_quality.city_state}
        extra={formatAirQualityCategory(air_quality.city_state.category, locale)}
        now={now}
        onSelect={onSelect}
      />
    </section>
  );
}
