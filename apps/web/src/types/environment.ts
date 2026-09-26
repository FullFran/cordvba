// Hand-written types for the cordvba environment contract v1
// (packages/contracts/environment/v1). These move to generation from the
// api's /openapi.json once it is deployed — see apps/web/README.md.

export type Label = "OBSERVED" | "PUBLISHED" | "INFERRED" | "PREDICTED" | "SIMULATED";

export interface Provenance {
  source: string;
  record_id: string;
  publisher: string;
  licence: string;
  source_url: string;
  observed_at: string;
  fetched_at: string;
  quality: string;
}

export interface Model {
  name: string;
  version: string;
  reference: string;
  generated_at: string;
  valid_from: string;
  valid_until: string;
  input_snapshot: string[];
  uncertainty: string;
}

export interface Value {
  value: number | string | null;
  unit: string;
  label: Label;
  at: string;
  provenance: Provenance | null;
  model: Model | null;
}

/** A Value that also carries MITECO's ICA air-quality category. */
export interface AirQualityValue extends Value {
  category: string;
  category_source: string;
  due_to?: string;
}

/** A timeline point: a Value that may carry a forecast horizon and/or category. */
export interface TimelinePoint extends Value {
  category?: string;
  category_source?: string;
  due_to?: string;
  horizon_h?: number;
}

export interface Place {
  name: string;
  lat: number;
  lon: number;
}

export interface WeatherStation {
  id: string;
  name: string;
  lat: number;
  lon: number;
}

export interface WeatherBlock {
  station: WeatherStation;
  air_temperature: Value;
  dew_point: Value;
  relative_humidity: Value;
  wind_speed: Value;
  wind_direction: Value;
  apparent_temperature: Value;
}

export interface AirQualityStation {
  id: string;
  name: string;
  lat: number;
  lon: number;
  index: AirQualityValue;
}

export interface AirQualityBlock {
  city_state: AirQualityValue;
  stations: AirQualityStation[];
}

export interface ForecastAirQuality {
  model: Model;
  horizons_h: number[];
}

export interface Forecast {
  air_quality: ForecastAirQuality;
}

/** GET /v1/environment */
export interface EnvironmentResponse {
  generated_at: string;
  place: Place;
  weather: WeatherBlock;
  air_quality: AirQualityBlock;
  forecast: Forecast;
}

export interface TimelineEntity {
  id: string;
  name: string;
}

export interface TimelineSeries {
  variable: string;
  unit: string;
  entity: TimelineEntity;
  points: TimelinePoint[];
}

/** GET /v1/environment/timeline */
export interface TimelineResponse {
  generated_at: string;
  now: string;
  series: TimelineSeries[];
}

/** POST /v1/environment/simulate request body */
export interface SimulateRequest {
  temperature_delta_c: number;
  humidity_delta_pct: number;
  wind_factor: number;
}

export interface SimulateStateBlock {
  air_temperature: Value;
  relative_humidity: Value;
  wind_speed: Value;
  apparent_temperature: Value;
}

/** POST /v1/environment/simulate response */
export interface SimulateResponse {
  generated_at: string;
  scenario: SimulateRequest;
  model: Model;
  observed: SimulateStateBlock;
  simulated: SimulateStateBlock;
}

export interface Source {
  id: string;
  publisher: string;
  licence: string;
  attribution: string;
  source_url: string;
  last_ok: string;
  state: string;
}

/** GET /v1/sources */
export interface SourcesResponse {
  sources: Source[];
}
