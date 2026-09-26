"""twin.state builds the observed+inferred environment state from eye's
records: latest METAR, city-only ICA stations with reported data, and the
derived humidity/apparent-temperature/worst-city-state values.
"""

import pytest
from conftest import make_ica_record, make_metar_record

from twin.state import build_weather, city_air_quality_stations, environment_state


def test_build_weather_derives_humidity_and_apparent_temperature():
    metar = make_metar_record(temperature_c=26.0, dewpoint_c=12.0, wind_speed_kt=8.0)

    weather = build_weather(metar)

    assert weather.air_temperature.value == 26.0
    assert weather.air_temperature.label == "OBSERVED"
    assert weather.air_temperature.provenance.source == "metar-cordoba"
    assert weather.dew_point.value == 12.0
    assert weather.relative_humidity.label == "INFERRED"
    assert weather.relative_humidity.value == pytest.approx(41.7, abs=0.1)
    assert weather.relative_humidity.provenance is None
    assert weather.relative_humidity.model.name == "magnus-relative-humidity"
    assert weather.wind_speed.value == pytest.approx(4.115, abs=1e-2)
    assert weather.wind_speed.unit == "m/s"
    assert weather.wind_direction.value == 250
    assert weather.apparent_temperature.label == "INFERRED"
    assert weather.apparent_temperature.value == pytest.approx(23.7, abs=0.1)
    assert weather.apparent_temperature.model.name == "bom-apparent-temperature"


def test_city_air_quality_stations_keeps_only_14021_prefixed_reporting_stations():
    records = [
        make_ica_record(station="13071014", index=0, index_reported=False),  # not city
        make_ica_record(station="14021006", index=30, category="regular", due_to="PM10"),
        make_ica_record(station="14021999", index=0, index_reported=False),  # city, no data
        make_ica_record(station="41088001", index=20, category="razonablemente buena"),  # not city
    ]

    city_stations = city_air_quality_stations(records)

    assert [r.local_key for r in city_stations] == ["14021006"]


def test_city_air_quality_stations_keeps_the_newest_record_per_station():
    older = make_ica_record(
        station="14021006",
        index=2,
        observed_at="2026-09-26T07:00:00Z",
        category="razonablemente buena",
    )
    newer = make_ica_record(
        station="14021006", index=3, observed_at="2026-09-26T08:00:00Z", category="regular"
    )

    # eye returns newest first.
    city_stations = city_air_quality_stations([newer, older])

    assert len(city_stations) == 1
    assert city_stations[0].payload["index"] == 3


def test_environment_state_picks_the_worst_city_station_as_city_state():
    metar = make_metar_record()
    ica_records = [
        make_ica_record(station="14021006", index=3, category="regular", name="ASOMADILLA"),
        make_ica_record(station="14021007", index=3, category="regular", name="LEPANTO"),
        make_ica_record(
            station="14021009", index=4, category="desfavorable", name="AVDA. AL-NASIR"
        ),
    ]

    state = environment_state(metar_record=metar, ica_records=ica_records)

    assert state.air_quality.city_state.value == 4
    assert state.air_quality.city_state.category == "poor"
    assert state.air_quality.city_state.label == "INFERRED"
    assert state.air_quality.city_state.model.name == "worst-city-station"
    assert {s.id for s in state.air_quality.stations} == {"14021006", "14021007", "14021009"}
    station_by_id = {s.id: s for s in state.air_quality.stations}
    assert station_by_id["14021009"].name == "AVDA. AL-NASIR"
    assert station_by_id["14021009"].index.category == "poor"
    assert station_by_id["14021009"].index.label == "OBSERVED"

