import type { SimulateRequest } from "../types/environment";

/**
 * UI slider ranges from issue #102: temperature +0..+5 °C, humidity
 * -20..+20 points, wind 20..100 %. These are a subset of the API's
 * accepted bounds (packages/contracts/environment/v1/README.md:
 * temperature_delta_c in [-10, 10], humidity_delta_pct in [-50, 50],
 * wind_factor in [0, 2]), so a request built from them never triggers a
 * 422.
 */
export const SCENARIO_BOUNDS = {
  temperatureDeltaC: { min: 0, max: 5 },
  humidityDeltaPct: { min: -20, max: 20 },
  windPct: { min: 20, max: 100 },
} as const;

export interface ScenarioSliders {
  temperatureDeltaC: number;
  humidityDeltaPct: number;
  /** Wind speed as a percentage of the observed value (20–100 %). */
  windPct: number;
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

/**
 * Builds the POST /v1/environment/simulate request body from the scenario
 * sliders, clamping to the declared UI bounds and converting the wind
 * percentage slider into the API's multiplicative wind_factor.
 */
export function buildSimulateRequest(sliders: ScenarioSliders): SimulateRequest {
  const temperature_delta_c = clamp(
    sliders.temperatureDeltaC,
    SCENARIO_BOUNDS.temperatureDeltaC.min,
    SCENARIO_BOUNDS.temperatureDeltaC.max,
  );
  const humidity_delta_pct = clamp(
    sliders.humidityDeltaPct,
    SCENARIO_BOUNDS.humidityDeltaPct.min,
    SCENARIO_BOUNDS.humidityDeltaPct.max,
  );
  const windPct = clamp(sliders.windPct, SCENARIO_BOUNDS.windPct.min, SCENARIO_BOUNDS.windPct.max);

  return {
    temperature_delta_c,
    humidity_delta_pct,
    wind_factor: windPct / 100,
  };
}
