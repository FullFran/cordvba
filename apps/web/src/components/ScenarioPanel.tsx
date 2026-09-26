import { useId, useState } from "react";

import { ValueTile } from "./ValueTile";
import { SCENARIO_BOUNDS, buildSimulateRequest, type ScenarioSliders } from "../lib/scenario";
import type { SimulateRequest, SimulateResponse, Value } from "../types/environment";

export interface ScenarioPanelProps {
  onSimulate: (request: SimulateRequest) => Promise<SimulateResponse>;
  now?: Date;
  onSelectValue?: (value: Value) => void;
}

const DEFAULT_SLIDERS: ScenarioSliders = {
  temperatureDeltaC: SCENARIO_BOUNDS.temperatureDeltaC.min,
  humidityDeltaPct: 0,
  windPct: 100,
};

/**
 * Scenario sliders for temperature, humidity and wind (issue #102). Running
 * the scenario shows observed and simulated values side by side, the
 * simulated ones always labelled SIMULATED.
 */
export function ScenarioPanel({ onSimulate, now, onSelectValue }: ScenarioPanelProps) {
  const [sliders, setSliders] = useState<ScenarioSliders>(DEFAULT_SLIDERS);
  const [result, setResult] = useState<SimulateResponse | null>(null);
  const [pending, setPending] = useState(false);

  const temperatureId = useId();
  const humidityId = useId();
  const windId = useId();

  async function runScenario() {
    setPending(true);
    try {
      const request = buildSimulateRequest(sliders);
      const response = await onSimulate(request);
      setResult(response);
    } finally {
      setPending(false);
    }
  }

  return (
    <section className="scenario-panel" aria-label="Scenario">
      <div className="scenario-panel__sliders">
        <label htmlFor={temperatureId}>Temperature (+°C)</label>
        <input
          id={temperatureId}
          type="range"
          min={SCENARIO_BOUNDS.temperatureDeltaC.min}
          max={SCENARIO_BOUNDS.temperatureDeltaC.max}
          step={1}
          value={sliders.temperatureDeltaC}
          onChange={(event) =>
            setSliders((prev) => ({ ...prev, temperatureDeltaC: Number(event.target.value) }))
          }
        />

        <label htmlFor={humidityId}>Humidity (points)</label>
        <input
          id={humidityId}
          type="range"
          min={SCENARIO_BOUNDS.humidityDeltaPct.min}
          max={SCENARIO_BOUNDS.humidityDeltaPct.max}
          step={1}
          value={sliders.humidityDeltaPct}
          onChange={(event) =>
            setSliders((prev) => ({ ...prev, humidityDeltaPct: Number(event.target.value) }))
          }
        />

        <label htmlFor={windId}>Wind (%)</label>
        <input
          id={windId}
          type="range"
          min={SCENARIO_BOUNDS.windPct.min}
          max={SCENARIO_BOUNDS.windPct.max}
          step={1}
          value={sliders.windPct}
          onChange={(event) =>
            setSliders((prev) => ({ ...prev, windPct: Number(event.target.value) }))
          }
        />
      </div>

      <button type="button" onClick={runScenario} disabled={pending}>
        {pending ? "Running scenario…" : "Run scenario"}
      </button>

      {result ? (
        <div className="scenario-panel__result">
          <div className="scenario-panel__observed">
            <h3>Observed</h3>
            <ValueTile
              name="Temperature"
              value={result.observed.air_temperature}
              now={now}
              onSelect={onSelectValue}
            />
            <ValueTile
              name="Humidity"
              value={result.observed.relative_humidity}
              now={now}
              onSelect={onSelectValue}
            />
            <ValueTile
              name="Wind speed"
              value={result.observed.wind_speed}
              now={now}
              onSelect={onSelectValue}
            />
            <ValueTile
              name="Apparent temperature"
              value={result.observed.apparent_temperature}
              now={now}
              onSelect={onSelectValue}
            />
          </div>
          <div className="scenario-panel__simulated">
            <h3>Simulated</h3>
            <ValueTile
              name="Temperature"
              value={result.simulated.air_temperature}
              now={now}
              onSelect={onSelectValue}
            />
            <ValueTile
              name="Humidity"
              value={result.simulated.relative_humidity}
              now={now}
              onSelect={onSelectValue}
            />
            <ValueTile
              name="Wind speed"
              value={result.simulated.wind_speed}
              now={now}
              onSelect={onSelectValue}
            />
            <ValueTile
              name="Apparent temperature"
              value={result.simulated.apparent_temperature}
              now={now}
              onSelect={onSelectValue}
            />
          </div>
        </div>
      ) : null}
    </section>
  );
}
