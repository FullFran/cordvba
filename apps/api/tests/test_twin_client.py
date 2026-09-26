"""TwinClient: a small HTTP client for twin's internal endpoints. Offline
via httpx.MockTransport; no test performs a live request.
"""

import json

import httpx
import pytest

from api.twin_client import TwinClient, TwinError


def _client_with(handler) -> TwinClient:
    transport = httpx.MockTransport(handler)
    return TwinClient(base_url="http://twin.internal", timeout=5.0, transport=transport)


def test_state_environment_gets_the_expected_path():
    seen = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen["path"] = request.url.path
        return httpx.Response(200, json={"ok": True})

    client = _client_with(handler)

    result = client.state_environment()

    assert seen["path"] == "/state/environment"
    assert result == {"ok": True}


def test_forecast_air_quality_forwards_horizons_as_a_query_param():
    seen = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen["path"] = request.url.path
        seen["params"] = dict(request.url.params)
        return httpx.Response(200, json={"ok": True})

    client = _client_with(handler)

    client.forecast_air_quality(horizons=[1, 3, 6])

    assert seen["path"] == "/forecast/air-quality"
    assert seen["params"] == {"horizons": "1,3,6"}


def test_simulate_environment_posts_the_scenario_body():
    seen = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen["path"] = request.url.path
        seen["body"] = json.loads(request.content)
        return httpx.Response(200, json={"ok": True})

    client = _client_with(handler)

    scenario = {"temperature_delta_c": 3, "humidity_delta_pct": 10, "wind_factor": 0.5}
    client.simulate_environment(scenario)

    assert seen["path"] == "/simulate/environment"
    assert seen["body"] == scenario


def test_a_5xx_response_raises_twin_error():
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(500, text="internal error")

    client = _client_with(handler)

    with pytest.raises(TwinError):
        client.state_environment()


def test_a_timeout_raises_twin_error():
    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.TimeoutException("timed out", request=request)

    client = _client_with(handler)

    with pytest.raises(TwinError):
        client.state_environment()
