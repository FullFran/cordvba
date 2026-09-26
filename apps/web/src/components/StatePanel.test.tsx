import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import environmentFixture from "@contracts/environment/v1/environment.example.json";

import { StatePanel } from "./StatePanel";
import type { EnvironmentResponse } from "../types/environment";

const environment = environmentFixture as unknown as EnvironmentResponse;

describe("StatePanel", () => {
  it("shows temperature, humidity, wind, apparent temperature and city air quality, each with a label", () => {
    render(<StatePanel environment={environment} now={new Date("2026-09-26T09:40:00Z")} />);

    expect(screen.getByText(/Temperature/)).toBeInTheDocument();
    expect(screen.getByText(/Humidity/)).toBeInTheDocument();
    expect(screen.getAllByText(/Wind/).length).toBeGreaterThan(0);
    expect(screen.getByText(/Apparent temperature/)).toBeInTheDocument();
    expect(screen.getByText(/City air quality/)).toBeInTheDocument();

    // temperature is OBSERVED, humidity is INFERRED (contract examples)
    expect(screen.getAllByText(/Observed/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/Inferred/).length).toBeGreaterThan(0);
  });

  it("calls onSelect with the clicked value, so its provenance can be shown", async () => {
    const { default: userEvent } = await import("@testing-library/user-event");
    const { vi } = await import("vitest");
    const onSelect = vi.fn();
    render(
      <StatePanel
        environment={environment}
        now={new Date("2026-09-26T09:40:00Z")}
        onSelect={onSelect}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: /^Temperature/ }));

    expect(onSelect).toHaveBeenCalledWith(environment.weather.air_temperature);
  });

  it("shows wind direction as a cardinal point plus degrees, not a bare 'deg' code (issue 119)", () => {
    render(<StatePanel environment={environment} now={new Date("2026-09-26T09:40:00Z")} />);

    // contract example: wind_direction.value === 250 -> "WSW 250°"
    expect(screen.getByText("WSW 250°")).toBeInTheDocument();
    expect(screen.queryByText(/250 deg/)).not.toBeInTheDocument();
  });

  it("shows the city air-quality category in words, not the underscore-joined slug", () => {
    const veryPoorEnvironment: EnvironmentResponse = {
      ...environment,
      air_quality: {
        ...environment.air_quality,
        city_state: { ...environment.air_quality.city_state, category: "very_poor" },
      },
    };
    render(<StatePanel environment={veryPoorEnvironment} now={new Date("2026-09-26T09:40:00Z")} />);

    expect(screen.getByText(/\(very poor\)/)).toBeInTheDocument();
    expect(screen.queryByText(/very_poor/)).not.toBeInTheDocument();
  });
});
