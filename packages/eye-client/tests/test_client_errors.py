"""401, other 4xx, 5xx and timeouts raise distinct, typed exceptions.
Nothing retries silently.
"""

import httpx
import pytest
from eye_client import EyeAuthError, EyeClient, EyeHTTPError, EyeServerError, EyeTimeoutError


def _client_with(handler) -> EyeClient:
    transport = httpx.MockTransport(handler)
    return EyeClient(
        base_url="https://eye.example.internal",
        token="s3cr3t-token",
        timeout=5.0,
        transport=transport,
    )


def test_401_raises_eye_auth_error():
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(401, json={"error": "missing or wrong bearer token"})

    client = _client_with(handler)

    with pytest.raises(EyeAuthError) as excinfo:
        client.records()

    assert excinfo.value.status_code == 401


def test_400_raises_eye_http_error_not_auth_error():
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(400, json={"error": "bad limit"})

    client = _client_with(handler)

    with pytest.raises(EyeHTTPError) as excinfo:
        client.records(limit=999999)

    assert excinfo.value.status_code == 400
    assert not isinstance(excinfo.value, EyeAuthError)


def test_500_raises_eye_server_error():
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(500, text="internal error")

    client = _client_with(handler)

    with pytest.raises(EyeServerError) as excinfo:
        client.sources()

    assert excinfo.value.status_code == 500


def test_timeout_raises_eye_timeout_error():
    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.TimeoutException("timed out", request=request)

    client = _client_with(handler)

    with pytest.raises(EyeTimeoutError):
        client.records()


def test_auth_error_message_never_contains_the_token():
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(401, json={"error": "no"})

    client = _client_with(handler)

    with pytest.raises(EyeAuthError) as excinfo:
        client.records()

    assert "s3cr3t-token" not in str(excinfo.value)


def test_client_repr_never_contains_the_token():
    client = _client_with(lambda request: httpx.Response(200, json={"count": 0, "records": []}))

    assert "s3cr3t-token" not in repr(client)
