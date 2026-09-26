"""entities() and sources() follow the same shape as records(): typed
models, bearer auth, offline via httpx.MockTransport.
"""

import json
from pathlib import Path

import httpx
from eye_client import Entity, EyeClient, Source

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


def test_entities_sends_bearer_token_and_returns_typed_entities():
    seen = {}
    # Synthetic payload: eye's real fixtures captured in this session had no
    # entities yet, but the shape mirrors eye's own Entity type as served
    # over HTTP (id, source, kind, topic, title, position, first_seen,
    # last_seen, payload, provenance).
    body = {
        "count": 1,
        "entities": [
            {
                "id": "aucorsa:line-3",
                "source": "aucorsa",
                "kind": "bus_line",
                "topic": "transit",
                "title": "Line 3",
                "first_seen": "2026-09-01T00:00:00Z",
                "last_seen": "2026-09-26T09:00:00Z",
                "provenance": {
                    "publisher": "AUCORSA",
                    "source_url": "https://example.invalid/gtfs",
                    "license": "CC-BY-4.0",
                    "fetched_at": "2026-09-26T09:00:00Z",
                    "raw_hash": "abc123",
                },
            }
        ],
    }

    def handler(request: httpx.Request) -> httpx.Response:
        seen["authorization"] = request.headers.get("authorization")
        seen["path"] = request.url.path
        return httpx.Response(200, json=body)

    client = _client_with(handler)

    entities = client.entities(topic="transit")

    assert seen["authorization"] == "Bearer s3cr3t-token"
    assert seen["path"] == "/v1/entities"
    assert len(entities) == 1
    assert isinstance(entities[0], Entity)
    assert entities[0].id == "aucorsa:line-3"


def test_sources_sends_bearer_token_and_returns_typed_sources_no_query_params():
    seen = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen["authorization"] = request.headers.get("authorization")
        seen["path"] = request.url.path
        seen["params"] = dict(request.url.params)
        return httpx.Response(200, json=_load("sources.json"))

    client = _client_with(handler)

    sources = client.sources()

    assert seen["authorization"] == "Bearer s3cr3t-token"
    assert seen["path"] == "/v1/sources"
    assert seen["params"] == {}
    assert len(sources) == 2
    assert all(isinstance(s, Source) for s in sources)
    ids = {s.id for s in sources}
    assert ids == {"metar-cordoba", "miteco-ica"}
