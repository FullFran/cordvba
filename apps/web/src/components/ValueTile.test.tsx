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
    expect(screen.getByText(/Cel/)).toBeInTheDocument();
    expect(screen.getByText(/OBSERVED/)).toBeInTheDocument();
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
});
