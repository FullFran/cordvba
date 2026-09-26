import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ValueTile } from "./ValueTile";
import type { Value } from "../types/environment";

const temperature: Value = {
  value: 26.0,
  unit: "Cel",
  label: "OBSERVED",
  at: "2026-09-26T09:00:00Z",
  provenance: {
    source: "metar-cordoba",
    record_id: "rec_metar_0900",
    publisher: "NOAA Aviation Weather Center",
    licence: "us-government-public-domain",
    source_url: "https://aviationweather.gov/api/data/metar",
    observed_at: "2026-09-26T09:00:00Z",
    fetched_at: "2026-09-26T09:07:12Z",
    quality: "preliminary",
  },
  model: null,
};

describe("ValueTile", () => {
  it("renders the name, value, unit, epistemic label and age", () => {
    render(
      <ValueTile name="Temperature" value={temperature} now={new Date("2026-09-26T09:40:00Z")} />,
    );

    expect(screen.getByText("Temperature")).toBeInTheDocument();
    expect(screen.getByText(/26/)).toBeInTheDocument();
    // the contract's raw unit code ("Cel") is never shown to a person (issue 119)
    expect(screen.getByText(/°C/)).toBeInTheDocument();
    expect(screen.queryByText(/Cel/)).not.toBeInTheDocument();
    expect(screen.getByText(/Observed/)).toBeInTheDocument();
    expect(screen.getByText(/40 min ago/)).toBeInTheDocument();
  });

  it("calls onSelect with the value when clicked, to open its provenance", async () => {
    const onSelect = vi.fn();
    render(<ValueTile name="Temperature" value={temperature} onSelect={onSelect} />);

    await userEvent.click(screen.getByRole("button", { name: /Temperature/ }));

    expect(onSelect).toHaveBeenCalledWith(temperature);
  });

  it("renders extra descriptive text (e.g. an air-quality category) when given one", () => {
    render(<ValueTile name="City air quality" value={temperature} extra="poor" />);

    expect(screen.getByText(/\(poor\)/)).toBeInTheDocument();
  });

  it("renders displayValue instead of the default formatValue rendering, when given one", () => {
    const windDirection: Value = { ...temperature, value: 250, unit: "deg" };
    render(
      <ValueTile name="Wind direction" value={windDirection} displayValue="WSW 250°" />,
    );

    expect(screen.getByText("WSW 250°")).toBeInTheDocument();
    expect(screen.queryByText(/250 deg/)).not.toBeInTheDocument();
  });

  it("renders an em dash, never the word 'null', for a value the source never reported", () => {
    const missing: Value = { ...temperature, value: null };
    render(<ValueTile name="Temperature" value={missing} />);

    expect(screen.getByText("—")).toBeInTheDocument();
    expect(screen.queryByText(/null/i)).not.toBeInTheDocument();
  });
});
