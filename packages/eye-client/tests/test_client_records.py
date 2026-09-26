"""EyeClient.records() sends the bearer token, forwards query params, and
returns typed Record objects. All HTTP is faked with httpx.MockTransport;
no test performs a live request.
"""

import json
from pathlib import Path

import httpx
from eye_client import EyeClient, Record

FIXTURES = Path(__file__).parent / "fixtures"


def _load(name: str) -> dict:
    return json.loads((FIXTURES / name).read_text())


def _client_with(handler) -> EyeClient:
    transport = httpx.MockTransport(handler)
    return EyeClient(
        base_url="https://eye.example.internal",
        token="s3cr3t-token",
        timeout=5.0,
        transport=transport,
    )


def test_records_sends_bearer_token_and_returns_typed_records():
    seen = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen["authorization"] = request.headers.get("authorization")
        seen["path"] = request.url.path
        return httpx.Response(200, json=_load("records_metar.json"))

    client = _client_with(handler)

    records = client.records()

    assert seen["authorization"] == "Bearer s3cr3t-token"
    assert seen["path"] == "/v1/records"
    assert len(records) == 2
    assert all(isinstance(r, Record) for r in records)
    assert records[0].source == "metar-cordoba"


def test_records_forwards_source_topic_since_and_limit_as_query_params():
    seen = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen["params"] = dict(request.url.params)
        return httpx.Response(200, json={"count": 0, "records": []})

    client = _client_with(handler)

    client.records(source="metar-cordoba", topic="weather", since="2026-09-26T00:00:00Z", limit=100)

    assert seen["params"] == {
        "source": "metar-cordoba",
        "topic": "weather",
        "since": "2026-09-26T00:00:00Z",
        "limit": "100",
    }


def test_records_omits_unset_query_params():
    seen = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen["params"] = dict(request.url.params)
        return httpx.Response(200, json={"count": 0, "records": []})

    client = _client_with(handler)

    client.records()

    assert seen["params"] == {}


def test_records_parses_ica_fixture_end_to_end():
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json=_load("records_ica.json"))

    client = _client_with(handler)

    records = client.records(source="miteco-ica")

    assert len(records) == 13
    assert {r.payload["station"] for r in records if r.payload["station"] == "14021009"}
