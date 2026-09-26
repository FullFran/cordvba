"""api.compose builds the public v1 response shapes from twin's and eye's
raw JSON. api never imports twin's or eye's internal code (see
docs/architecture/system-overview.md): these functions take plain dicts,
as they arrive over HTTP.
"""

from datetime import UTC, datetime

from conftest import make_eye_source

from api.compose import environment_from_twin, sources_from_eye
from api.schemas import EnvironmentResponse, SourcesResponse

TWIN_STATE = {
    "generated_at": "2026-09-26T09:40:00Z",
    "weather": {
        "station": {"id": "LEBA", "name": "Córdoba Airport", "lat": 37.842, "lon": -4.8488},
        "air_temperature": {
            "value": 26.0,
            "unit": "Cel",
            "label": "OBSERVED",
            "at": "2026-09-26T09:00:00Z",
            "provenance": {
                "source": "metar-cordoba",
                "record_id": "rec_metar_0900",
                "publisher": "NOAA Aviation Weather Center",
                "licence": "us-government-public-domain",
                "source_url": "https://aviationweather.gov/api/data/metar",
                "observed_at": "2026-09-26T09:00:00Z",
                "fetched_at": "2026-09-26T09:07:12Z",
                "quality": "preliminary",
            },
            "model": None,
        },
        "dew_point": {
            "value": 12.0,
            "unit": "Cel",
            "label": "OBSERVED",
            "at": "2026-09-26T09:00:00Z",
            "provenance": None,
            "model": None,
        },
        "relative_humidity": {
            "value": 41.7,
            "unit": "%",
            "label": "INFERRED",
            "at": "2026-09-26T09:00:00Z",
            "provenance": None,
            "model": {
                "name": "magnus-relative-humidity",
                "version": "1",
                "reference": "Alduchov & Eskridge (1996), Magnus approximation",
                "generated_at": "2026-09-26T09:40:00Z",
                "valid_from": "2026-09-26T09:00:00Z",
                "valid_until": "2026-09-26T09:00:00Z",
                "input_snapshot": ["rec_metar_0900"],
                "uncertainty": "derived from reported temperature and dew point",
            },
        },
        "wind_speed": {
            "value": 4.1,
            "unit": "m/s",
            "label": "OBSERVED",
            "at": "2026-09-26T09:00:00Z",
            "provenance": None,
            "model": None,
        },
        "wind_direction": {
            "value": 250,
            "unit": "deg",
            "label": "OBSERVED",
            "at": "2026-09-26T09:00:00Z",
            "provenance": None,
            "model": None,
        },
        "apparent_temperature": {
            "value": 23.7,
            "unit": "Cel",
            "label": "INFERRED",
            "at": "2026-09-26T09:00:00Z",
            "provenance": None,
            "model": {
                "name": "bom-apparent-temperature",
                "version": "1",
                "reference": (
                    "Australian Bureau of Meteorology apparent temperature "
                    "(Steadman 1994), with wind"
                ),
                "generated_at": "2026-09-26T09:40:00Z",
                "valid_from": "2026-09-26T09:00:00Z",
                "valid_until": "2026-09-26T09:00:00Z",
                "input_snapshot": ["rec_metar_0900"],
                "uncertainty": "shade value; no solar radiation term",
            },
        },
    },
    "air_quality": {
        "city_state": {
            "value": 4,
            "unit": "ica",
            "label": "INFERRED",
            "at": "2026-09-26T08:00:00Z",
            "category": "poor",
            "category_source": "desfavorable",
            "provenance": None,
            "model": {
                "name": "worst-city-station",
                "version": "1",
                "reference": "maximum ICA index among Córdoba city stations reporting data",
                "generated_at": "2026-09-26T09:40:00Z",
                "valid_from": "2026-09-26T08:00:00Z",
                "valid_until": "2026-09-26T09:00:00Z",
                "input_snapshot": ["rec_ica_14021009"],
                "uncertainty": "three stations; not representative of every street",
            },
        },
        "stations": [
            {
                "id": "14021009",
                "name": "AVDA. AL-NASIR",
                "lat": 37.8926,
                "lon": -4.7801,
                "index": {
                    "value": 4,
                    "unit": "ica",
                    "label": "OBSERVED",
                    "at": "2026-09-26T08:00:00Z",
                    "category": "poor",
                    "category_source": "desfavorable",
                    "due_to": "NO2",
                    "provenance": {
                        "source": "miteco-ica",
                        "record_id": "rec_ica_14021009",
                        "publisher": "MITECO",
                        "licence": "CC-BY-4.0",
                        "source_url": "https://ica.miteco.es/datos/ica-ultima-hora.csv",
                        "observed_at": "2026-09-26T08:00:00Z",
                        "fetched_at": "2026-09-26T09:35:40Z",
                        "quality": "official",
                    },
                    "model": None,
                },
            }
        ],
    },
}

TWIN_FORECAST = {
    "generated_at": "2026-09-26T09:40:00Z",
    "horizons_h": [1, 3, 6],
    "stations": [
        {
            "id": "14021009",
            "name": "AVDA. AL-NASIR",
            "lat": 37.8926,
            "lon": -4.7801,
            "last_observed": TWIN_STATE["air_quality"]["stations"][0]["index"],
            "forecast": [
                {
                    "horizon_h": 1,
                    "value": {
                        "value": 4,
                        "unit": "ica",
                        "label": "PREDICTED",
                        "at": "2026-09-26T09:00:00Z",
                        "category": "poor",
                        "category_source": "desfavorable",
                        "provenance": None,
                        "model": {
                            "name": "persistence",
                            "version": "1",
                            "reference": (
                                "naive baseline: the forecast equals the last observed index"
                            ),
                            "generated_at": "2026-09-26T09:40:00Z",
                            "valid_from": "2026-09-26T09:00:00Z",
                            "valid_until": "2026-09-26T09:00:00Z",
                            "input_snapshot": ["rec_ica_14021009"],
                            "uncertainty": "baseline, not evaluated",
                        },
                    },
                }
            ],
        }
    ],
}


def test_environment_from_twin_builds_a_valid_environment_response():
    now = datetime(2026, 9, 26, 9, 40, tzinfo=UTC)

    result = environment_from_twin(TWIN_STATE, TWIN_FORECAST, now=now)

    assert isinstance(result, EnvironmentResponse)
    assert result.place.name == "Córdoba"
    assert result.weather.air_temperature.value == 26.0
    assert result.air_quality.city_state.value == 4
    assert result.forecast.air_quality.horizons_h == [1, 3, 6]
    assert result.forecast.air_quality.model.name == "persistence"


def test_environment_from_twin_never_invents_a_confidence_number():
    now = datetime(2026, 9, 26, 9, 40, tzinfo=UTC)

    result = environment_from_twin(TWIN_STATE, TWIN_FORECAST, now=now)

    assert isinstance(result.forecast.air_quality.model.uncertainty, str)
    assert "not evaluated" in result.forecast.air_quality.model.uncertainty


def test_sources_from_eye_keeps_only_redistributable_configured_sources():
    sources = [
        make_eye_source(source_id="metar-cordoba", license_="us-government-public-domain"),
        make_eye_source(source_id="miteco-ica", license_="CC-BY-4.0"),
        make_eye_source(source_id="some-other-source", license_="CC-BY-4.0"),
    ]

    result = sources_from_eye(sources)

    assert isinstance(result, SourcesResponse)
    ids = {s.id for s in result.sources}
    assert ids == {"metar-cordoba", "miteco-ica"}


def test_sources_from_eye_includes_the_attribution_string():
    sources = [make_eye_source(source_id="miteco-ica", license_="CC-BY-4.0")]

    result = sources_from_eye(sources)

    entry = result.sources[0]
    assert "MITECO" in entry.attribution
    assert entry.licence == "CC-BY-4.0"
