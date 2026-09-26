import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { ProvenancePanel } from "./ProvenancePanel";
import type { Value } from "../types/environment";

const observedValue: Value = {
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

const derivedValue: Value = {
  value: 23.7,
  unit: "Cel",
  label: "INFERRED",
  at: "2026-09-26T09:00:00Z",
  provenance: null,
  model: {
    name: "bom-apparent-temperature",
    version: "1",
    reference: "Australian Bureau of Meteorology apparent temperature (Steadman 1994), with wind",
    generated_at: "2026-09-26T09:40:00Z",
    valid_from: "2026-09-26T09:00:00Z",
    valid_until: "2026-09-26T09:00:00Z",
    input_snapshot: ["rec_metar_0900"],
    uncertainty: "shade value; no solar radiation term",
  },
};

describe("ProvenancePanel", () => {
  it("shows source, publisher, licence, observed_at, fetched_at and quality for an observed value", () => {
    render(<ProvenancePanel value={observedValue} />);

    expect(screen.getByText("metar-cordoba")).toBeInTheDocument();
    expect(screen.getByText("NOAA Aviation Weather Center")).toBeInTheDocument();
    expect(screen.getByText("us-government-public-domain")).toBeInTheDocument();
    expect(screen.getByText("2026-09-26T09:00:00Z")).toBeInTheDocument();
    expect(screen.getByText("2026-09-26T09:07:12Z")).toBeInTheDocument();
    expect(screen.getByText("preliminary")).toBeInTheDocument();
  });

  it("shows model name, version, reference and uncertainty for a derived value, and no provenance fields", () => {
    render(<ProvenancePanel value={derivedValue} />);

    expect(screen.getByText("bom-apparent-temperature")).toBeInTheDocument();
    expect(screen.getByText("1")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Australian Bureau of Meteorology apparent temperature (Steadman 1994), with wind",
      ),
    ).toBeInTheDocument();
    expect(screen.getByText("shade value; no solar radiation term")).toBeInTheDocument();
    expect(screen.queryByText("metar-cordoba")).not.toBeInTheDocument();
  });
});
