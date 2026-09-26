import { describe, expect, it } from "vitest";

import { SCENARIO_BOUNDS, buildSimulateRequest } from "./scenario";

describe("SCENARIO_BOUNDS", () => {
  it("keeps the UI slider ranges from issue 102 within the API's accepted bounds (packages/contracts/environment/v1/README.md)", () => {
    // API: temperature_delta_c in [-10, 10], humidity_delta_pct in [-50, 50], wind_factor in [0, 2]
      expect(SCENARIO_BOUNDS.temperatureDeltaC.min).toBeGreaterThanOrEqual(-10);
      expect(SCENARIO_BOUNDS.temperatureDeltaC.max).toBeLessThanOrEqual(10);
      expect(SCENARIO_BOUNDS.humidityDeltaPct.min).toBeGreaterThanOrEqual(-50);
      expect(SCENARIO_BOUNDS.humidityDeltaPct.max).toBeLessThanOrEqual(50);
  });
});

describe("buildSimulateRequest", () => {
  it("maps the sliders at their minimums to a request within API bounds", () => {
    const request = buildSimulateRequest({
      temperatureDeltaC: 0,
      humidityDeltaPct: -20,
      windPct: 20,
    });

    expect(request).toEqual({
      temperature_delta_c: 0,
      humidity_delta_pct: -20,
      wind_factor: 0.2,
    });
  });

  it("maps the sliders at their maximums to a request within API bounds", () => {
    const request = buildSimulateRequest({
      temperatureDeltaC: 5,
      humidityDeltaPct: 20,
      windPct: 100,
    });

    expect(request).toEqual({
      temperature_delta_c: 5,
      humidity_delta_pct: 20,
      wind_factor: 1,
    });
  });

  it("matches the contract's own request example for its slider values", () => {
    // packages/contracts/environment/v1/simulate.request.example.json
    const request = buildSimulateRequest({
      temperatureDeltaC: 3,
      humidityDeltaPct: 10,
      windPct: 50,
    });

    expect(request).toEqual({
      temperature_delta_c: 3,
      humidity_delta_pct: 10,
      wind_factor: 0.5,
    });
  });

  it("clamps slider input that falls outside the declared UI bounds instead of sending it as-is", () => {
    const request = buildSimulateRequest({
      temperatureDeltaC: 999,
      humidityDeltaPct: -999,
      windPct: 0,
    });

    expect(request.temperature_delta_c).toBe(5);
    expect(request.humidity_delta_pct).toBe(-20);
    expect(request.wind_factor).toBe(0.2);
  });
});
