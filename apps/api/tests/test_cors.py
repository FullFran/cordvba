"""CORS with a comma-separated WEB_ORIGIN allow-list (issue #115, AC-2):
several configured origins are allowed, everything else is not, and a
single configured origin keeps working exactly as before.
"""

from fastapi.testclient import TestClient

from api.app import create_app, get_eye_client, get_twin_client


class _FakeEyeClient:
    def records(self, **kwargs):
        return []

    def sources(self):
        return []


class _FakeTwinClient:
    def state_environment(self):
        return {
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

    def forecast_air_quality(self, horizons):
        return {"generated_at": "2026-09-26T09:40:00Z", "horizons_h": horizons, "stations": []}

    def simulate_environment(self, scenario):
        raise NotImplementedError


def _make_client() -> TestClient:
    app = create_app()
    app.dependency_overrides[get_eye_client] = lambda: _FakeEyeClient()
    app.dependency_overrides[get_twin_client] = lambda: _FakeTwinClient()
    return TestClient(app)


def _set_required_env(monkeypatch, web_origin: str) -> None:
    monkeypatch.setenv("EYE_BASE_URL", "http://eye.internal.invalid:8080")
    monkeypatch.setenv("EYE_API_TOKEN", "s3cr3t")
    monkeypatch.setenv("TWIN_BASE_URL", "http://twin.internal.invalid:8100")
    monkeypatch.setenv("WEB_ORIGIN", web_origin)


def test_cors_allows_both_configured_origins_on_a_get(monkeypatch):
    _set_required_env(monkeypatch, "https://a.example, https://b.example")
    client = _make_client()

    response_a = client.get("/v1/environment", headers={"Origin": "https://a.example"})
    response_b = client.get("/v1/environment", headers={"Origin": "https://b.example"})

    assert response_a.headers.get("access-control-allow-origin") == "https://a.example"
    assert response_b.headers.get("access-control-allow-origin") == "https://b.example"


def test_cors_allows_both_configured_origins_on_a_preflight(monkeypatch):
    _set_required_env(monkeypatch, "https://a.example, https://b.example")
    client = _make_client()

    response_a = client.options(
        "/v1/environment",
        headers={
            "Origin": "https://a.example",
            "Access-Control-Request-Method": "GET",
        },
    )
    response_b = client.options(
        "/v1/environment",
        headers={
            "Origin": "https://b.example",
            "Access-Control-Request-Method": "GET",
        },
    )

    assert response_a.headers.get("access-control-allow-origin") == "https://a.example"
    assert response_b.headers.get("access-control-allow-origin") == "https://b.example"


def test_cors_rejects_a_third_unconfigured_origin(monkeypatch):
    _set_required_env(monkeypatch, "https://a.example, https://b.example")
    client = _make_client()

    response = client.get("/v1/environment", headers={"Origin": "https://c.example"})

    assert "access-control-allow-origin" not in response.headers


def test_cors_single_configured_origin_is_unchanged(monkeypatch):
    _set_required_env(monkeypatch, "https://cordvba.example")
    client = _make_client()

    allowed = client.get("/v1/environment", headers={"Origin": "https://cordvba.example"})
    rejected = client.get("/v1/environment", headers={"Origin": "https://other.example"})

    assert allowed.headers.get("access-control-allow-origin") == "https://cordvba.example"
    assert "access-control-allow-origin" not in rejected.headers
