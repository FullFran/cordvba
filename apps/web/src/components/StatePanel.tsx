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

  return (
    <section className="state-panel" aria-label="Current state">
      <ValueTile name="Temperature" value={weather.air_temperature} now={now} onSelect={onSelect} />
      <ValueTile name="Humidity" value={weather.relative_humidity} now={now} onSelect={onSelect} />
      <ValueTile name="Wind speed" value={weather.wind_speed} now={now} onSelect={onSelect} />
      <ValueTile
        name="Wind direction"
        value={weather.wind_direction}
        now={now}
        onSelect={onSelect}
      />
      <ValueTile
        name="Apparent temperature"
        value={weather.apparent_temperature}
        now={now}
        onSelect={onSelect}
      />
      <ValueTile
        name="City air quality"
        value={air_quality.city_state}
        extra={air_quality.city_state.category}
        now={now}
        onSelect={onSelect}
      />
    </section>
  );
}
