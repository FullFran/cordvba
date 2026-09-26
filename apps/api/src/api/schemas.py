"""Response models for cordvba's public v1 environment endpoints.

These must match packages/contracts/environment/v1/*.example.json
field-for-field: that directory is the canonical contract both api and
apps/web test against (see its README.md). apps/api/tests/test_schemas.py
loads those examples and validates them against these models.
"""

from __future__ import annotations

from datetime import datetime
from typing import Literal

from pydantic import BaseModel, Field

Label = Literal["OBSERVED", "INFERRED", "PREDICTED", "SIMULATED"]


class Provenance(BaseModel):
    source: str
    record_id: str
    publisher: str
    licence: str
    source_url: str
    observed_at: datetime
    fetched_at: datetime
    quality: str


class Model(BaseModel):
    name: str
    version: str
    reference: str
    generated_at: datetime
    valid_from: datetime
    valid_until: datetime
    input_snapshot: list[str]
    uncertainty: str


class Value(BaseModel):
    """Every number or category the page shows. `category`/`category_source`/
    `due_to` apply only to air-quality values; `horizon_h` only to a
    forecast point on the timeline. All four are omitted from the response
    (not serialized as null) when they do not apply.
    """

    value: float | int | str | None
    unit: str
    label: Label
    at: datetime
    category: str | None = None
    category_source: str | None = None
    due_to: str | None = None
    horizon_h: int | None = None
    provenance: Provenance | None = None
    model: Model | None = None


class Place(BaseModel):
    name: str
    lat: float
    lon: float


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


class AirQualityStation(BaseModel):
    id: str
    name: str
    lat: float
    lon: float
    index: Value


class AirQuality(BaseModel):
    city_state: Value
    stations: list[AirQualityStation]


class AirQualityForecastSummary(BaseModel):
    model: Model
    horizons_h: list[int]


class Forecast(BaseModel):
    air_quality: AirQualityForecastSummary


class EnvironmentResponse(BaseModel):
    generated_at: datetime
    place: Place
    weather: Weather
    air_quality: AirQuality
    forecast: Forecast


class TimelineEntity(BaseModel):
    id: str
    name: str


class TimelineSeries(BaseModel):
    variable: str
    unit: str
    entity: TimelineEntity
    points: list[Value]


class TimelineResponse(BaseModel):
    generated_at: datetime
    now: datetime
    series: list[TimelineSeries]


class SimulateRequest(BaseModel):
    """Bounds per packages/contracts/environment/v1/README.md, 'Scenario
    bounds'. Anything outside raises a pydantic ValidationError, which
    FastAPI turns into a 422 (AC-3).
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
    scenario: SimulateRequest
    model: Model
    observed: SimulateValues
    simulated: SimulateValues


class SourceEntry(BaseModel):
    id: str
    publisher: str
    licence: str
    attribution: str
    source_url: str
    last_ok: datetime
    state: str


class SourcesResponse(BaseModel):
    sources: list[SourceEntry]


PulseKind = Literal["warning", "fire", "quake", "event", "headline", "bulletin"]
PulseLabel = Literal["OBSERVED", "PUBLISHED"]
PulseSeverity = Literal["none", "info", "low", "moderate", "high", "critical"]


class Position(BaseModel):
    lat: float
    lon: float


class PulseProvenance(BaseModel):
    """Deliberately smaller than environment/v1's Provenance: enough to
    trust and cite an item (who published it, under what licence, how
    fresh) without exposing eye's own record shape.
    """

    source: str
    publisher: str
    licence: str
    fetched_at: datetime


class PulseItem(BaseModel):
    """One thing happening around Cordoba. `position`/`area` are both
    optional and never both filled: a source gives coordinates, a named
    place, or (a national BOE bulletin) neither. See
    packages/contracts/pulse/v1/README.md for the OBSERVED/PUBLISHED split.
    """

    kind: PulseKind
    id: str
    title: str
    observed_at: datetime
    valid_from: datetime | None = None
    valid_until: datetime | None = None
    position: Position | None = None
    area: str | None = None
    severity: PulseSeverity
    url: str | None = None
    label: PulseLabel
    provenance: PulseProvenance
    attribution: str


class PulseResponse(BaseModel):
    generated_at: datetime
    window_hours: int
    items: list[PulseItem]
