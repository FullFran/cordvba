"""The shared Value/Provenance/Model shapes, mirroring
packages/contracts/environment/v1/README.md ("Shared shapes"). twin's own
responses use these directly so that api's composition step stays a
forward, not a translation.
"""

from __future__ import annotations

from datetime import datetime
from typing import Literal

from pydantic import BaseModel, Field

Label = Literal["OBSERVED", "INFERRED", "PREDICTED", "SIMULATED"]


class ValueProvenance(BaseModel):
    """Where an observed value came from. Required when a Value's label is
    OBSERVED. Field is spelled `licence` (British) to match the contract,
    even though eye's own JSON spells it `license`.
    """

    source: str
    record_id: str
    publisher: str
    licence: str
    source_url: str
    observed_at: datetime
    fetched_at: datetime
    quality: str


class Model(BaseModel):
    """How a derived value was produced. Required for INFERRED, PREDICTED
    and SIMULATED values. `uncertainty` is free text, never a made-up
    confidence number.
    """

    name: str
    version: str
    reference: str
    generated_at: datetime
    valid_from: datetime
    valid_until: datetime
    input_snapshot: list[str]
    uncertainty: str


class Value(BaseModel):
    """Every number or category the page shows."""

    value: float | int | str | None
    unit: str
    label: Label
    at: datetime
    provenance: ValueProvenance | None = None
    model: Model | None = None


class IcaValue(Value):
    """An ICA index value, with the category MITECO derives from it."""

    category: str | None = None
    category_source: str | None = None
    due_to: str | None = None


class StationRef(BaseModel):
    id: str
    name: str
    lat: float
    lon: float


class Weather(BaseModel):
    station: StationRef
    air_temperature: Value
    dew_point: Value
    relative_humidity: Value
    wind_speed: Value
    wind_direction: Value
    apparent_temperature: Value


class Station(BaseModel):
    id: str
    name: str
    lat: float
    lon: float
    index: IcaValue


class AirQuality(BaseModel):
    city_state: IcaValue
    stations: list[Station]


class EnvironmentState(BaseModel):
    generated_at: datetime
    weather: Weather
    air_quality: AirQuality


class ForecastPoint(BaseModel):
    horizon_h: int
    value: IcaValue


class StationForecast(BaseModel):
    id: str
    name: str
    lat: float
    lon: float
    last_observed: IcaValue
    forecast: list[ForecastPoint]


class AirQualityForecast(BaseModel):
    generated_at: datetime
    horizons_h: list[int]
    stations: list[StationForecast]


class ScenarioRequest(BaseModel):
    """Bounds per packages/contracts/environment/v1/README.md, 'Scenario
    bounds'. Anything outside raises a pydantic ValidationError, which
    apps/api turns into a 422.
    """

    temperature_delta_c: float = Field(ge=-10, le=10)
    humidity_delta_pct: float = Field(ge=-50, le=50)
    wind_factor: float = Field(ge=0, le=2)


class SimulateValues(BaseModel):
    air_temperature: Value
    relative_humidity: Value
    wind_speed: Value
    apparent_temperature: Value


class SimulateResponse(BaseModel):
    generated_at: datetime
    scenario: ScenarioRequest
    model: Model
    observed: SimulateValues
    simulated: SimulateValues
