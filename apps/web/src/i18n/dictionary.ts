/**
 * Every visible string in the page, typed so a missing translation is a
 * compile error (`satisfies Dictionary` on each locale file) and also
 * caught at runtime by `dictionary.test.ts` (issue #124, AC-1). Machine
 * tokens from the API contract (`OBSERVED`, `good`, …) are never part of
 * this file — only their on-screen text is.
 */
export interface Dictionary {
  header: {
    title: string;
    line1: string;
    line2: string;
    line3: string;
    skipToContent: string;
  };
  localeSwitch: {
    groupLabel: string;
    es: string;
    en: string;
  };
  loading: {
    label: string;
  };
  networkError: {
    message: string;
    retry: string;
  };
  unavailable: {
    timelinePrefix: string;
    attributionPrefix: string;
    mapPrefix: string;
  };
  map: {
    ariaLabel: string;
    caption: string;
    beaconAirQuality: string;
    /** Same as `beaconAirQuality`, plus the pollutant responsible (issue #140), e.g. "due to O3". */
    beaconAirQualityWithPollutant: string;
    beaconWind: string;
    unknownIndex: string;
    edgeIndicator: string;
    /** The air-quality columns layer's honesty line (issue #140): one column per station, height = index, no interpolation. */
    columnsLegend: string;
    /** The camera-preset toggle group's accessible group label (parent review). */
    cameraPresetsLabel: string;
    presetOverview: string;
    presetHistoric: string;
    /** The collapsible legend's <summary> text (parent review: never let it clip the timeline bar). */
    legendSummary: string;
  };
  wind: {
    toggle: string;
    label: string;
  };
  /** The sun widget (issue #139): sunrise/solar-noon/sunset + current altitude, and the building-shadow layer's honesty legend line. */
  sun: {
    ariaLabel: string;
    sunrise: string;
    solarNoon: string;
    sunset: string;
    altitude: string;
    shadowLegend: string;
  };
  timeline: {
    ariaLabel: string;
    now: string;
    scenario: string;
  };
  state: {
    ariaLabel: string;
    temperature: string;
    humidity: string;
    windSpeed: string;
    windDirection: string;
    apparentTemperature: string;
    cityAirQuality: string;
  };
  scenario: {
    ariaLabel: string;
    temperatureLabel: string;
    humidityLabel: string;
    windLabel: string;
    run: string;
    running: string;
    observedHeading: string;
    simulatedHeading: string;
  };
  provenance: {
    dialogLabel: string;
    close: string;
    source: string;
    publisher: string;
    licence: string;
    observedAt: string;
    fetchedAt: string;
    quality: string;
    model: string;
    version: string;
    reference: string;
    uncertainty: string;
  };
  footer: {
    tileAttribution: string;
    updatedPrefix: string;
    /** "{kind}: {publisher} ({licence})" — built client-side from the contract's id/publisher/licence fields (issue #124), never the API's own pre-formatted (English-only) `attribution` string. */
    attributionTemplate: string;
    /** Keyed by `Source.id`; a source id with no entry falls back to the API's raw `attribution` string. */
    sourceKinds: Record<string, string>;
    /** Keyed by `Source.licence`; falls back to the raw licence code if unrecognised. */
    licences: Record<string, string>;
  };
  epistemicLabels: {
    OBSERVED: string;
    PUBLISHED: string;
    INFERRED: string;
    PREDICTED: string;
    SIMULATED: string;
  };
  airQualityCategories: {
    good: string;
    fair: string;
    moderate: string;
    poor: string;
    very_poor: string;
    extremely_poor: string;
  };
  age: {
    justNow: string;
    minAgo: string;
    hAgo: string;
    hMinAgo: string;
    inH: string;
  };
  /** 16-point compass, N first, clockwise — issue #119/#124. */
  cardinals: readonly [
    string,
    string,
    string,
    string,
    string,
    string,
    string,
    string,
    string,
    string,
    string,
    string,
    string,
    string,
    string,
    string,
  ];
}

/** The Intl locale tag each app Locale resolves to (issue #124: es-ES decimal comma, en-GB). */
export const INTL_TAG: Record<"es" | "en", string> = {
  es: "es-ES",
  en: "en-GB",
};
