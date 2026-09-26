"""Composes eye's and twin's raw JSON into the public v1 response shapes.
api never imports eye's or twin's internal code (see
docs/architecture/system-overview.md); it only reshapes the plain dicts
they return over HTTP, plus eye_client's typed models for the /v1/sources
endpoint (eye_client is the shared, public contract for eye's HTTP API,
not an internal import).
"""

from __future__ import annotations

from datetime import UTC, datetime
from typing import Any

from eye_client import Record, Source

from api.ica import normalize_index, to_category
from api.schemas import (
    AirQuality,
    AirQualityForecastSummary,
    EnvironmentResponse,
    Forecast,
    Model,
    Place,
    Provenance,
    SourceEntry,
    SourcesResponse,
    TimelineEntity,
    TimelineResponse,
    TimelineSeries,
    Value,
    Weather,
)

#: Córdoba's city centre point, shown alongside the observed/derived state.
CITY_PLACE = Place(name="Córdoba", lat=37.8882, lon=-4.7794)

#: Only these sources are redistributable in v0.1 (AC: only metar-cordoba
#: and miteco-ica, with an attribution string). See
#: packages/contracts/environment/v1/sources.example.json.
_SOURCE_ATTRIBUTIONS = {
    "metar-cordoba": "Weather: NOAA Aviation Weather Center (public domain)",
    "miteco-ica": (
        "Air quality: Ministerio para la Transición Ecológica y el Reto Demográfico "
        "(MITECO), CC BY 4.0"
    ),
}


def environment_from_twin(
    state: dict[str, Any], forecast: dict[str, Any], now: datetime | None = None
) -> EnvironmentResponse:
    """Build `GET /v1/environment`'s response from twin's
    `GET /state/environment` and `GET /forecast/air-quality`.
    """
    now = now or datetime.now(UTC)

    weather = Weather.model_validate(state["weather"])
    air_quality = AirQuality.model_validate(state["air_quality"])
    forecast_summary = Forecast(air_quality=_air_quality_forecast_summary(forecast))

    return EnvironmentResponse(
        generated_at=now,
        place=CITY_PLACE,
        weather=weather,
        air_quality=air_quality,
        forecast=forecast_summary,
    )


def _air_quality_forecast_summary(forecast: dict[str, Any]) -> AirQualityForecastSummary:
    """twin reports one persistence model per station per horizon; here
    they collapse into a single summary model spanning every horizon, with
    every contributing station's last observed record in `input_snapshot`.
    """
    horizons_h: list[int] = forecast.get("horizons_h", [])
    stations = forecast.get("stations", [])

    input_snapshot = [
        record_id
        for station in stations
        if (provenance := station.get("last_observed", {}).get("provenance"))
        for record_id in [provenance.get("record_id")]
        if record_id
    ]

    generated_at = forecast.get("generated_at")
    valid_from = min(
        (station["last_observed"]["at"] for station in stations if station.get("last_observed")),
        default=generated_at,
    )
    last_horizon_points = [
        point["value"]["at"]
        for station in stations
        for point in station.get("forecast", [])
    ]
    valid_until = max(last_horizon_points, default=generated_at)

    model = Model(
        name="persistence",
        version="1",
        reference="naive baseline: the forecast equals the last observed index",
        generated_at=generated_at,
        valid_from=valid_from,
        valid_until=valid_until,
        input_snapshot=input_snapshot,
        uncertainty="baseline, not evaluated",
    )
    return AirQualityForecastSummary(model=model, horizons_h=horizons_h)


def sources_from_eye(sources: list[Source]) -> SourcesResponse:
    """`GET /v1/sources`: only the redistributable sources the v0.1 page
    shows, each with its attribution string.
    """
    entries = []
    for source in sources:
        attribution = _SOURCE_ATTRIBUTIONS.get(source.id)
        if attribution is None:
            continue
        extra = source.model_extra or {}
        entries.append(
            SourceEntry(
                id=source.id,
                publisher=source.authority,
                licence=source.license,
                attribution=attribution,
                source_url=source.url,
                last_ok=extra.get("last_success"),
                state="live" if extra.get("consecutive_errors", 0) == 0 else "degraded",
            )
        )
    return SourcesResponse(sources=entries)


def _metar_point(record: Record) -> Value:
    return Value(
        value=record.payload["temperature_c"],
        unit="Cel",
        label="OBSERVED",
        at=record.observed_at,
        provenance=_provenance(record),
    )


def _ica_observed_point(record: Record) -> Value:
    normalized = normalize_index(record.payload["index"])
    category = to_category(normalized)
    return Value(
        value=normalized,
        unit="ica",
        label="OBSERVED",
        at=record.observed_at,
        category=category.category,
        category_source=category.category_source,
        due_to=record.payload.get("due_to") or None,
        provenance=_provenance(record),
    )


def _provenance(record: Record) -> Provenance:
    return Provenance(
        source=record.source,
        record_id=record.id,
        publisher=record.provenance.publisher,
        licence=record.provenance.license,
        source_url=record.provenance.source_url,
        observed_at=record.observed_at,
        fetched_at=record.fetched_at,
        quality=record.quality,
    )


def timeline_from_eye_and_twin(
    *,
    metar_records: list[Record],
    ica_records: list[Record],
    twin_forecast: dict[str, Any],
    station_id: str,
    now: datetime | None = None,
) -> TimelineResponse:
    """Build `GET /v1/environment/timeline`'s response: observed points
    from eye's history plus predicted points from twin's persistence
    forecast, one ordered series per variable. `station_id` selects which
    Córdoba-city ICA station the `air_quality_index` series follows.
    """
    now = now or datetime.now(UTC)

    metar_series = TimelineSeries(
        variable="air_temperature",
        unit="Cel",
        entity=TimelineEntity(id="LEBA", name="Córdoba Airport"),
        points=[
            _metar_point(record)
            for record in sorted(metar_records, key=lambda r: r.observed_at)
        ],
    )

    matching_station = next(
        (s for s in twin_forecast.get("stations", []) if s.get("id") == station_id), None
    )
    station_name = matching_station["name"] if matching_station else station_id

    observed_points = [
        _ica_observed_point(record)
        for record in sorted(ica_records, key=lambda r: r.observed_at)
    ]
    predicted_points = [
        Value.model_validate(point["value"] | {"horizon_h": point["horizon_h"]})
        for point in (matching_station["forecast"] if matching_station else [])
    ]

    aq_series = TimelineSeries(
        variable="air_quality_index",
        unit="ica",
        entity=TimelineEntity(id=station_id, name=station_name),
        points=observed_points + predicted_points,
    )

    return TimelineResponse(generated_at=now, now=now, series=[metar_series, aq_series])
