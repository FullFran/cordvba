import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import simulateResponseFixture from "@contracts/environment/v1/simulate.response.example.json";

import { ScenarioPanel } from "./ScenarioPanel";
import { SCENARIO_BOUNDS } from "../lib/scenario";
import type { SimulateResponse } from "../types/environment";

const simulateResponse = simulateResponseFixture as unknown as SimulateResponse;

describe("ScenarioPanel", () => {
  it("renders sliders bounded to the UI ranges from issue 102", () => {
    render(<ScenarioPanel onSimulate={vi.fn()} />);

    const temperature = screen.getByRole("slider", { name: /temperature/i });
    expect(temperature).toHaveAttribute("min", String(SCENARIO_BOUNDS.temperatureDeltaC.min));
    expect(temperature).toHaveAttribute("max", String(SCENARIO_BOUNDS.temperatureDeltaC.max));

    const humidity = screen.getByRole("slider", { name: /humidity/i });
    expect(humidity).toHaveAttribute("min", String(SCENARIO_BOUNDS.humidityDeltaPct.min));
    expect(humidity).toHaveAttribute("max", String(SCENARIO_BOUNDS.humidityDeltaPct.max));

    const wind = screen.getByRole("slider", { name: /wind/i });
    expect(wind).toHaveAttribute("min", String(SCENARIO_BOUNDS.windPct.min));
    expect(wind).toHaveAttribute("max", String(SCENARIO_BOUNDS.windPct.max));
  });

  it("runs the scenario with a request built from the current sliders, within API bounds", async () => {
    const onSimulate = vi.fn().mockResolvedValue(simulateResponse);
    render(<ScenarioPanel onSimulate={onSimulate} />);

    await userEvent.click(screen.getByRole("button", { name: /run scenario/i }));

    expect(onSimulate).toHaveBeenCalledTimes(1);
    const call = onSimulate.mock.calls[0];
    if (!call) {
      throw new Error("onSimulate was not called");
    }
    const [request] = call;
    expect(request.temperature_delta_c).toBeGreaterThanOrEqual(-10);
    expect(request.temperature_delta_c).toBeLessThanOrEqual(10);
    expect(request.humidity_delta_pct).toBeGreaterThanOrEqual(-50);
    expect(request.humidity_delta_pct).toBeLessThanOrEqual(50);
    expect(request.wind_factor).toBeGreaterThanOrEqual(0);
    expect(request.wind_factor).toBeLessThanOrEqual(2);
  });

  it("shows observed and simulated side by side, clearly labelled SIMULATED, after the scenario runs", async () => {
    const onSimulate = vi.fn().mockResolvedValue(simulateResponse);
    render(<ScenarioPanel onSimulate={onSimulate} />);

    await userEvent.click(screen.getByRole("button", { name: /run scenario/i }));

    expect(await screen.findByText(/26/)).toBeInTheDocument(); // observed air_temperature
    expect(screen.getByText(/29/)).toBeInTheDocument(); // simulated air_temperature
    expect(screen.getAllByText(/Simulated/).length).toBeGreaterThan(0);
  });
});
