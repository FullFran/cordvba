"""FastAPI endpoint contract tests, offline: eye and twin are faked
through dependency overrides so no test performs a live HTTP request.
Also asserts no response leaks eye's token or either internal base URL
(AC-4).
"""

import json

from conftest import make_eye_source, make_metar_record
from fastapi.testclient import TestClient

from api.app import create_app, get_eye_client, get_twin_client

SECRET_TOKEN = "s3cr3t-eye-token"
EYE_URL = "http://eye.internal.invalid:8080"
TWIN_URL = "http://twin.internal.invalid:8100"

STATE = {
    "generated_at": "2026-09-26T09:40:00Z",
    "weather": {
        "station": {"id": "LEBA", "name": "Córdoba Airport", "lat": 37.842, "lon": -4.8488},
        "air_temperature": {
            "value": 26.0,
            "unit": "Cel",
            "label": "OBSERVED",
            "at": "2026-09-26T09:00:00Z",
            "provenance": None,
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
            "model": None,
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
            "model": None,
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
            "model": None,
        },
        "stations": [],
    },
}

FORECAST = {"generated_at": "2026-09-26T09:40:00Z", "horizons_h": [1, 3, 6], "stations": []}

SIMULATE_RESPONSE = {
    "generated_at": "2026-09-26T09:41:00Z",
    "scenario": {"temperature_delta_c": 3, "humidity_delta_pct": 10, "wind_factor": 0.5},
    "model": {
        "name": "thermal-scenario",
        "version": "1",
        "reference": "observed state with the scenario deltas applied",
        "generated_at": "2026-09-26T09:41:00Z",
        "valid_from": "2026-09-26T09:00:00Z",
        "valid_until": "2026-09-26T09:00:00Z",
        "input_snapshot": ["rec_metar_0900"],
        "uncertainty": "counterfactual; not a forecast",
    },
    "observed": {
        "air_temperature": {
            "value": 26.0,
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
            "model": None,
        },
        "wind_speed": {
            "value": 4.1,
            "unit": "m/s",
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
            "model": None,
        },
    },
    "simulated": {
        "air_temperature": {
            "value": 29.0,
            "unit": "Cel",
            "label": "SIMULATED",
            "at": "2026-09-26T09:00:00Z",
            "provenance": None,
            "model": None,
        },
        "relative_humidity": {
            "value": 51.7,
            "unit": "%",
            "label": "SIMULATED",
            "at": "2026-09-26T09:00:00Z",
            "provenance": None,
            "model": None,
        },
        "wind_speed": {
            "value": 2.05,
            "unit": "m/s",
            "label": "SIMULATED",
            "at": "2026-09-26T09:00:00Z",
            "provenance": None,
            "model": None,
        },
        "apparent_temperature": {
            "value": 30.4,
            "unit": "Cel",
            "label": "SIMULATED",
            "at": "2026-09-26T09:00:00Z",
            "provenance": None,
            "model": None,
        },
    },
}


class _FakeEyeClient:
    def __init__(self, records=None, sources=None):
        self._records = records if records is not None else [make_metar_record()]
        self._sources = sources if sources is not None else [
            make_eye_source(source_id="metar-cordoba", license_="us-government-public-domain"),
            make_eye_source(source_id="miteco-ica", license_="CC-BY-4.0"),
        ]

    def records(self, **kwargs):
        return self._records

    def sources(self):
        return self._sources


class _FakeTwinClient:
    def state_environment(self):
        return STATE

    def forecast_air_quality(self, horizons):
        return FORECAST

    def simulate_environment(self, scenario):
        return SIMULATE_RESPONSE


def _make_client() -> TestClient:
    app = create_app()
    app.dependency_overrides[get_eye_client] = lambda: _FakeEyeClient()
    app.dependency_overrides[get_twin_client] = lambda: _FakeTwinClient()
    return TestClient(app)


def test_health_returns_ok():
    client = _make_client()

    response = client.get("/health")

    assert response.status_code == 200
    assert response.json() == {"status": "ok"}


def test_v1_environment_returns_the_composed_shape():
    client = _make_client()

    response = client.get("/v1/environment")

    assert response.status_code == 200
    body = response.json()
    assert body["place"]["name"] == "Córdoba"
    assert body["weather"]["air_temperature"]["value"] == 26.0
    assert body["forecast"]["air_quality"]["horizons_h"] == [1, 3, 6]


def test_v1_environment_timeline_returns_a_series_list():
    client = _make_client()

    response = client.get(
        "/v1/environment/timeline", params={"hours_back": 12, "horizons": "1,3,6"}
    )

    assert response.status_code == 200
    body = response.json()
    variables = {s["variable"] for s in body["series"]}
    assert "air_temperature" in variables


def test_v1_environment_simulate_returns_observed_and_simulated():
    client = _make_client()

    response = client.post(
        "/v1/environment/simulate",
        json={"temperature_delta_c": 3, "humidity_delta_pct": 10, "wind_factor": 0.5},
    )

    assert response.status_code == 200
    body = response.json()
    assert body["simulated"]["air_temperature"]["label"] == "SIMULATED"


def test_v1_environment_simulate_rejects_out_of_bounds_scenario_with_422():
    client = _make_client()

    response = client.post(
        "/v1/environment/simulate",
        json={"temperature_delta_c": 99, "humidity_delta_pct": 0, "wind_factor": 1},
    )

    assert response.status_code == 422


def test_v1_sources_returns_only_the_configured_redistributable_sources():
    client = _make_client()

    response = client.get("/v1/sources")

    assert response.status_code == 200
    ids = {s["id"] for s in response.json()["sources"]}
    assert ids == {"metar-cordoba", "miteco-ica"}
    assert "attribution" in response.json()["sources"][0]


def test_openapi_json_describes_every_endpoint():
    client = _make_client()

    response = client.get("/openapi.json")

    assert response.status_code == 200
    paths = response.json()["paths"]
    for expected in (
        "/health",
        "/v1/environment",
        "/v1/environment/timeline",
        "/v1/environment/simulate",
        "/v1/sources",
    ):
        assert expected in paths


def test_no_response_contains_eyes_token_or_either_internal_url(monkeypatch):
    monkeypatch.setenv("EYE_BASE_URL", EYE_URL)
    monkeypatch.setenv("EYE_API_TOKEN", SECRET_TOKEN)
    monkeypatch.setenv("TWIN_BASE_URL", TWIN_URL)
    monkeypatch.setenv("WEB_ORIGIN", "https://cordvba.example")

    client = _make_client()

    for response in (
        client.get("/v1/environment"),
        client.get("/v1/environment/timeline"),
        client.get("/v1/sources"),
        client.post(
            "/v1/environment/simulate",
            json={"temperature_delta_c": 0, "humidity_delta_pct": 0, "wind_factor": 1},
        ),
        client.get("/openapi.json"),
    ):
        text = json.dumps(response.json())
        assert SECRET_TOKEN not in text
        assert EYE_URL not in text
        assert TWIN_URL not in text
