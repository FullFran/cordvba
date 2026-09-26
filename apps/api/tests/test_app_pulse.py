"""FastAPI contract tests for GET /v1/pulse, offline: eye is faked through
a dependency override so no test performs a live HTTP request. See
packages/contracts/pulse/v1/README.md.
"""

from __future__ import annotations

import json

from conftest import make_eye_source
from eye_client import Record
from fastapi.testclient import TestClient

from api.app import create_app, get_eye_client, get_twin_client
from api.pulse import PULSE_SOURCE_IDS

SECRET_TOKEN = "s3cr3t-eye-token"
EYE_URL = "http://eye.internal.invalid:8080"


def _warning_record() -> Record:
    return Record.model_validate(
        {
            "id": "aemet-warnings:1",
            "source": "aemet-warnings",
            "kind": "weather_warning",
            "topic": "weather",
            "title": "Orange High-temperature Warning issued for Spain - Campiña cordobesa",
            "observed_at": "2026-09-25T10:36:10Z",
            "fetched_at": "2026-09-26T09:37:00Z",
            "valid_from": "2026-09-26T11:00:00Z",
            "valid_until": "2026-09-26T19:00:00Z",
            "quality": "official",
            "severity": 4,
            "confidence": 1,
            "dedupe_key": "aemet-warnings:1",
            "payload": {
                "area": "Campiña cordobesa",
                "cap_document": "https://feeds.meteoalarm.org/api/v1/warnings/x",
            },
            "provenance": {
                "publisher": "AEMET. Agencia Estatal de Meteorologia",
                "source_url": "https://feeds.meteoalarm.org/feeds/meteoalarm-legacy-atom-spain",
                "license": "cc-by-4.0-equivalent-meteoalarm-terms",
                "fetched_at": "2026-09-26T09:37:00Z",
                "raw_hash": "hash",
            },
        }
    )


class _FakeEyeClient:
    """Faithful enough to the real EyeClient for these tests: records()
    honours `source`, sources() lists exactly the pulse sources as
    redistributable.
    """

    def __init__(self, records_by_source=None):
        self._records_by_source = records_by_source or {"aemet-warnings": [_warning_record()]}
        self.requested_sources: list[str] = []

    def records(self, *, source=None, topic=None, since=None, limit=None):
        self.requested_sources.append(source)
        return self._records_by_source.get(source, [])

    def sources(self):
        return [
            make_eye_source(source_id=sid, license_="unspecified") for sid in PULSE_SOURCE_IDS
        ]


class _FakeTwinClient:
    def state_environment(self):
        return {}

    def forecast_air_quality(self, horizons):
        return {}

    def simulate_environment(self, scenario):
        return {}


def _make_client(fake_eye: _FakeEyeClient | None = None) -> TestClient:
    app = create_app()
    app.dependency_overrides[get_eye_client] = lambda: fake_eye or _FakeEyeClient()
    app.dependency_overrides[get_twin_client] = lambda: _FakeTwinClient()
    return TestClient(app)


def test_v1_pulse_returns_the_composed_shape():
    client = _make_client()

    response = client.get("/v1/pulse", params={"hours": 24})

    assert response.status_code == 200
    body = response.json()
    assert body["window_hours"] == 24
    assert body["items"][0]["kind"] == "warning"
    assert body["items"][0]["provenance"]["source"] == "aemet-warnings"


def test_v1_pulse_defaults_hours_to_24():
    client = _make_client()

    response = client.get("/v1/pulse")

    assert response.status_code == 200
    assert response.json()["window_hours"] == 24


def test_v1_pulse_queries_eye_for_every_pulse_source():
    fake = _FakeEyeClient()
    client = _make_client(fake)

    client.get("/v1/pulse", params={"hours": 24})

    assert set(fake.requested_sources) == set(PULSE_SOURCE_IDS)


def test_v1_pulse_rejects_hours_below_one_with_422():
    client = _make_client()

    response = client.get("/v1/pulse", params={"hours": 0})

    assert response.status_code == 422


def test_v1_pulse_rejects_hours_above_seventy_two_with_422():
    client = _make_client()

    response = client.get("/v1/pulse", params={"hours": 73})

    assert response.status_code == 422


def test_v1_pulse_is_described_in_openapi_json():
    client = _make_client()

    response = client.get("/openapi.json")

    assert "/v1/pulse" in response.json()["paths"]


def test_v1_pulse_never_leaks_eyes_token_or_base_url(monkeypatch):
    monkeypatch.setenv("EYE_BASE_URL", EYE_URL)
    monkeypatch.setenv("EYE_API_TOKEN", SECRET_TOKEN)
    monkeypatch.setenv("TWIN_BASE_URL", "http://twin.internal.invalid:8100")
    monkeypatch.setenv("WEB_ORIGIN", "https://cordvba.example")

    client = _make_client()

    response = client.get("/v1/pulse")

    text = json.dumps(response.json())
    assert SECRET_TOKEN not in text
    assert EYE_URL not in text
