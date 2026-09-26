import { describe, expect, it } from "vitest";

import { formatSourceAttribution } from "./attribution";
import type { Source } from "../types/environment";

const weatherSource: Source = {
  id: "metar-cordoba",
  publisher: "NOAA Aviation Weather Center",
  licence: "us-government-public-domain",
  attribution: "Weather: NOAA Aviation Weather Center (public domain)",
  source_url: "https://aviationweather.gov/api/data/metar",
  last_ok: "2026-09-26T09:07:12Z",
  state: "live",
};

const airQualitySource: Source = {
  id: "miteco-ica",
  publisher: "MITECO",
  licence: "CC-BY-4.0",
  attribution: "Air quality: Ministerio para la Transición Ecológica y el Reto Demográfico (MITECO), CC BY 4.0",
  source_url: "https://ica.miteco.es/datos/ica-ultima-hora.csv",
  last_ok: "2026-09-26T09:35:40Z",
  state: "live",
};

describe("formatSourceAttribution", () => {
  it("builds the attribution from id/publisher/licence in English, not the API's own attribution string", () => {
    expect(formatSourceAttribution(weatherSource, "en")).toBe(
      "Weather: NOAA Aviation Weather Center (public domain)",
    );
  });

  it("builds the attribution in Spanish (issue 124: never the API's English-only string inside the Spanish UI)", () => {
    expect(formatSourceAttribution(weatherSource, "es")).toBe(
      "Meteorología: NOAA Aviation Weather Center (dominio público)",
    );
    expect(formatSourceAttribution(airQualitySource, "es")).toBe("Calidad del aire: MITECO (CC BY 4.0)");
  });

  it("never contains the API's own raw English kind word when built in Spanish", () => {
    const result = formatSourceAttribution(airQualitySource, "es");
    expect(result).not.toMatch(/Air quality/);
    expect(result).not.toMatch(/Ministerio para la Transición/); // the API's own long-form publisher text, not used here
  });

  it("falls back to the API's own attribution string for an unrecognised source id, rather than showing 'undefined'", () => {
    const unknownSource: Source = { ...weatherSource, id: "some-new-source" };
    expect(formatSourceAttribution(unknownSource, "en")).toBe(unknownSource.attribution);
  });

  it("falls back to the raw licence code for an unrecognised licence", () => {
    const source: Source = { ...weatherSource, licence: "future-licence-v2" };
    expect(formatSourceAttribution(source, "en")).toBe("Weather: NOAA Aviation Weather Center (future-licence-v2)");
  });
});
