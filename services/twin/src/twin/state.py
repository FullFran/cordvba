"""Builds the observed + inferred environment state from eye's records.
Pure functions over eye_client.Record: no HTTP here (see twin.eye_reads for
the fetching side). See services/twin/README.md and
packages/contracts/environment/v1/README.md.
"""

from __future__ import annotations

from datetime import UTC, datetime

from eye_client import Record

from twin.formulas import apparent_temperature_c, knots_to_ms, relative_humidity_pct
from twin.ica import normalize_index, to_category
from twin.schemas import (
    AirQuality,
    EnvironmentState,
    IcaValue,
    Station,
    StationRef,
    Value,
    ValueProvenance,
    Weather,
)

#: Córdoba city's ICA station codes all start with this prefix. Everything
#: else (the rest of Córdoba province, or another province entirely) is
#: ignored here even before eye's own province filter (#98) lands.
CITY_STATION_PREFIX = "14021"


def provenance_from_record(record: Record) -> ValueProvenance:
    return ValueProvenance(
        source=record.source,
        record_id=record.id,
        publisher=record.provenance.publisher,
        licence=record.provenance.license,
        source_url=record.provenance.source_url,
        observed_at=record.observed_at,
        fetched_at=record.fetched_at,
        quality=record.quality,
    )


def station_name(record: Record) -> str:
    return record.title.split(":", 1)[0].strip()


def city_air_quality_stations(records: list[Record]) -> list[Record]:
    """Keep only Córdoba-city stations (code starting with 14021) that
    reported data, one record per station (the newest; eye returns records
    newest first).
    """
    seen: set[str] = set()
    kept: list[Record] = []
    for record in records:
        station = record.payload.get("station") or record.local_key
        if not station or not station.startswith(CITY_STATION_PREFIX):
            continue
        if not record.payload.get("index_reported"):
            continue
        if station in seen:
            continue
        seen.add(station)
        kept.append(record)
    return kept


def ica_value_from_record(record: Record) -> IcaValue:
    normalized = normalize_index(record.payload["index"])
    category = to_category(normalized)
    return IcaValue(
        value=normalized,
        unit="ica",
        label="OBSERVED",
        at=record.observed_at,
        category=category.category,
        category_source=category.category_source,
        due_to=record.payload.get("due_to") or None,
        provenance=provenance_from_record(record),
        model=None,
    )


def build_weather(metar_record: Record):
    payload = metar_record.payload
    temperature_c = float(payload["temperature_c"])
    dewpoint_c = float(payload["dewpoint_c"])
    wind_speed_ms = knots_to_ms(float(payload["wind_speed_kt"]))
    generated_at = datetime.now(UTC)

    provenance = provenance_from_record(metar_record)
    at = metar_record.observed_at

    air_temperature = Value(
        value=temperature_c, unit="Cel", label="OBSERVED", at=at, provenance=provenance
    )
    dew_point = Value(
        value=dewpoint_c, unit="Cel", label="OBSERVED", at=at, provenance=provenance
    )
    wind_speed = Value(
        value=wind_speed_ms, unit="m/s", label="OBSERVED", at=at, provenance=provenance
    )
    wind_direction = Value(
        value=payload["wind_direction_deg"],
        unit="deg",
        label="OBSERVED",
        at=at,
        provenance=provenance,
    )

    rh = relative_humidity_pct(temperature_c, dewpoint_c)
    relative_humidity = Value(
        value=round(rh, 1),
        unit="%",
        label="INFERRED",
        at=at,
        provenance=None,
        model={
            "name": "magnus-relative-humidity",
            "version": "1",
            "reference": "Alduchov & Eskridge (1996), Magnus approximation",
            "generated_at": generated_at,
            "valid_from": at,
            "valid_until": at,
            "input_snapshot": [metar_record.id],
            "uncertainty": "derived from reported temperature and dew point",
        },
    )

    at_value = apparent_temperature_c(temperature_c, rh, wind_speed_ms)
    apparent_temperature = Value(
        value=round(at_value, 1),
        unit="Cel",
        label="INFERRED",
        at=at,
        provenance=None,
        model={
            "name": "bom-apparent-temperature",
            "version": "1",
            "reference": (
                "Australian Bureau of Meteorology apparent temperature (Steadman 1994), "
                "with wind"
            ),
            "generated_at": generated_at,
            "valid_from": at,
            "valid_until": at,
            "input_snapshot": [metar_record.id],
            "uncertainty": "shade value; no solar radiation term",
        },
    )

    station = StationRef(
        id=metar_record.payload.get("icao") or metar_record.local_key or metar_record.id,
        name=metar_record.payload.get("station") or metar_record.title,
        lat=metar_record.position.lat if metar_record.position else 0.0,
        lon=metar_record.position.lon if metar_record.position else 0.0,
    )

    return Weather(
        station=station,
        air_temperature=air_temperature,
        dew_point=dew_point,
        relative_humidity=relative_humidity,
        wind_speed=wind_speed,
        wind_direction=wind_direction,
        apparent_temperature=apparent_temperature,
    )


def build_air_quality(ica_records: list[Record]) -> AirQuality:
    city_records = city_air_quality_stations(ica_records)
    stations = [
        Station(
            id=record.local_key or record.payload["station"],
            name=station_name(record),
            lat=record.position.lat if record.position else 0.0,
            lon=record.position.lon if record.position else 0.0,
            index=ica_value_from_record(record),
        )
        for record in city_records
    ]

    if stations:
        worst = max(stations, key=lambda s: s.index.value)
        generated_at = datetime.now(UTC)
        at = worst.index.at
        city_state = IcaValue(
            value=worst.index.value,
            unit="ica",
            label="INFERRED",
            at=at,
            category=worst.index.category,
            category_source=worst.index.category_source,
            provenance=None,
            model={
                "name": "worst-city-station",
                "version": "1",
                "reference": "maximum ICA index among Córdoba city stations reporting data",
                "generated_at": generated_at,
                "valid_from": at,
                "valid_until": at,
                "input_snapshot": [record.id for record in city_records],
                "uncertainty": "three stations; not representative of every street",
            },
        )
    else:
        city_state = IcaValue(value=None, unit="ica", label="INFERRED", at=datetime.now(UTC))

    return AirQuality(city_state=city_state, stations=stations)


def environment_state(*, metar_record: Record, ica_records: list[Record]) -> EnvironmentState:
    return EnvironmentState(
        generated_at=datetime.now(UTC),
        weather=build_weather(metar_record),
        air_quality=build_air_quality(ica_records),
    )
