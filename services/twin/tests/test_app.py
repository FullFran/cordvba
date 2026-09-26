"""FastAPI endpoint contract tests, offline: eye is faked through
dependency overrides so no test performs a live HTTP request.
"""

from conftest import make_ica_record, make_metar_record
from fastapi.testclient import TestClient

from twin.app import create_app, get_eye_client


class _FakeEyeClient:
    def __init__(self, metar_records, ica_records):
        self._metar_records = metar_records
        self._ica_records = ica_records

    def records(self, *, source, topic, limit=None, **_kwargs):
        if source == "metar-cordoba":
            return self._metar_records[: limit or len(self._metar_records)]
        if source == "miteco-ica":
            return self._ica_records[: limit or len(self._ica_records)]
        return []


def _make_client(metar_records=None, ica_records=None) -> TestClient:
    app = create_app()
    fake = _FakeEyeClient(
        metar_records if metar_records is not None else [make_metar_record()],
        ica_records
        if ica_records is not None
        else [
            make_ica_record(
                station="14021009", index=4, category="desfavorable", name="AVDA. AL-NASIR"
            )
        ],
    )
    app.dependency_overrides[get_eye_client] = lambda: fake
    return TestClient(app)


def test_health_returns_ok():
    client = _make_client()

    response = client.get("/health")

    assert response.status_code == 200
    assert response.json() == {"status": "ok"}


def test_state_environment_returns_weather_and_air_quality():
    client = _make_client()

    response = client.get("/state/environment")

    assert response.status_code == 200
    body = response.json()
    assert body["weather"]["air_temperature"]["label"] == "OBSERVED"
    assert body["weather"]["relative_humidity"]["label"] == "INFERRED"
    assert body["air_quality"]["city_state"]["label"] == "INFERRED"
    assert body["air_quality"]["stations"][0]["id"] == "14021009"


def test_forecast_air_quality_uses_the_requested_horizons():
    client = _make_client()

    response = client.get("/forecast/air-quality", params={"horizons": "1,3,6"})

    assert response.status_code == 200
    body = response.json()
    assert body["horizons_h"] == [1, 3, 6]
    assert len(body["stations"][0]["forecast"]) == 3
    assert all(
        point["value"]["label"] == "PREDICTED" for point in body["stations"][0]["forecast"]
    )


def test_simulate_environment_returns_observed_and_simulated():
    client = _make_client()

    response = client.post(
        "/simulate/environment",
        json={"temperature_delta_c": 3, "humidity_delta_pct": 10, "wind_factor": 0.5},
    )

    assert response.status_code == 200
    body = response.json()
    assert body["observed"]["air_temperature"]["label"] == "OBSERVED"
    assert body["simulated"]["air_temperature"]["label"] == "SIMULATED"


def test_simulate_environment_rejects_out_of_bounds_scenario_with_422():
    client = _make_client()

    response = client.post(
        "/simulate/environment",
        json={"temperature_delta_c": 99, "humidity_delta_pct": 0, "wind_factor": 1},
    )

    assert response.status_code == 422
