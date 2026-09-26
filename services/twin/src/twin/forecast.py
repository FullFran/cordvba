"""A persistence baseline for air quality: the forecast equals the last
observed index, per station, for each requested horizon. See
packages/contracts/environment/v1/README.md and issue #100's "Alternatives
Considered" for why persistence (not a trained model) is honest here: eye's
history starts 2026-09-26, so there is no training data yet.
"""

from __future__ import annotations

from datetime import UTC, datetime, timedelta

from eye_client import Record

from twin.schemas import AirQualityForecast, ForecastPoint, IcaValue, Model, StationForecast
from twin.state import city_air_quality_stations, ica_value_from_record, station_name


def _persistence_model(now: datetime, at: datetime, record_id: str) -> Model:
    return Model(
        name="persistence",
        version="1",
        reference="naive baseline: the forecast equals the last observed index",
        generated_at=now,
        valid_from=at,
        valid_until=at,
        input_snapshot=[record_id],
        uncertainty="baseline, not evaluated",
    )


def air_quality_forecast(
    ica_records: list[Record], horizons: list[int], now: datetime | None = None
) -> AirQualityForecast:
    now = now or datetime.now(UTC)
    city_records = city_air_quality_stations(ica_records)

    stations = []
    for record in city_records:
        last_observed = ica_value_from_record(record)
        points = [
            ForecastPoint(
                horizon_h=horizon,
                value=IcaValue(
                    value=last_observed.value,
                    unit=last_observed.unit,
                    label="PREDICTED",
                    at=(at := last_observed.at + timedelta(hours=horizon)),
                    category=last_observed.category,
                    category_source=last_observed.category_source,
                    due_to=None,
                    provenance=None,
                    model=_persistence_model(now, at, record.id),
                ),
            )
            for horizon in horizons
        ]
        stations.append(
            StationForecast(
                id=record.local_key or record.payload["station"],
                name=station_name(record),
                lat=record.position.lat if record.position else 0.0,
                lon=record.position.lon if record.position else 0.0,
                last_observed=last_observed,
                forecast=points,
            )
        )

    return AirQualityForecast(generated_at=now, horizons_h=horizons, stations=stations)
