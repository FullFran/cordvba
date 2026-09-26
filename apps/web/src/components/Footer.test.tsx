import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Footer } from "./Footer";
import { LocaleProvider } from "../i18n/LocaleContext";
import type { Source } from "../types/environment";

const sources: Source[] = [
  {
    id: "metar-cordoba",
    publisher: "NOAA Aviation Weather Center",
    licence: "us-government-public-domain",
    attribution: "Weather: NOAA Aviation Weather Center (public domain)",
    source_url: "https://aviationweather.gov/api/data/metar",
    last_ok: "2026-09-26T09:07:12Z",
    state: "live",
  },
  {
    id: "miteco-ica",
    publisher: "MITECO",
    licence: "CC-BY-4.0",
    attribution: "Air quality: Ministerio para la Transición Ecológica y el Reto Demográfico (MITECO), CC BY 4.0",
    source_url: "https://ica.miteco.es/datos/ica-ultima-hora.csv",
    last_ok: "2026-09-26T09:35:40Z",
    state: "live",
  },
];

describe("Footer", () => {
  it("shows attribution for weather, air quality and the map tiles", () => {
    render(<Footer sources={sources} now={new Date("2026-09-26T09:40:00Z")} />);

    expect(screen.getByText(/NOAA Aviation Weather Center \(public domain\)/)).toBeInTheDocument();
    expect(screen.getByText(/MITECO.*CC BY 4\.0/)).toBeInTheDocument();
    // issue 119: the map moved to MapLibre/OpenFreeMap vector tiles, with
    // the required attribution "OpenFreeMap © OpenMapTiles Data from
    // OpenStreetMap" replacing the old bare OSM raster-tile credit.
    expect(screen.getByText(/OpenFreeMap.*OpenMapTiles.*OpenStreetMap/)).toBeInTheDocument();
  });

  it("shows each source's freshness computed from its last_ok timestamp", () => {
    render(<Footer sources={sources} now={new Date("2026-09-26T09:40:00Z")} />);

    // 09:40 - 09:07:12 = 32 min 48s -> floors to 32 min ago
    expect(screen.getByText(/32 min ago/)).toBeInTheDocument();
    // 09:40 - 09:35:40 = 4 min 20s -> floors to 4 min ago
    expect(screen.getByText(/4 min ago/)).toBeInTheDocument();
  });

  it("builds attribution in Spanish, never the API's own English attribution string (issue 124, maintainer review)", () => {
    render(
      <LocaleProvider>
        <Footer sources={sources} now={new Date("2026-09-26T09:40:00Z")} />
      </LocaleProvider>,
    );

    expect(screen.getByText(/Meteorología: NOAA Aviation Weather Center \(dominio público\)/)).toBeInTheDocument();
    expect(screen.getByText(/Calidad del aire: MITECO \(CC BY 4\.0\)/)).toBeInTheDocument();
    expect(screen.queryByText(/Air quality:/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Ministerio para la Transición/)).not.toBeInTheDocument();
  });
});
