"""Typed exceptions for eye_client. Nothing retries silently: every failure
mode eye's HTTP API can produce becomes a distinct, catchable type.
"""

from __future__ import annotations


class EyeClientError(Exception):
    """Base class for every error this client raises."""


class EyeAuthError(EyeClientError):
    """The bearer token was missing or rejected (HTTP 401)."""

    def __init__(self, status_code: int = 401) -> None:
        self.status_code = status_code
        super().__init__(f"eye rejected the request: HTTP {status_code}")


class EyeHTTPError(EyeClientError):
    """Any other 4xx response."""

    def __init__(self, status_code: int, body: str = "") -> None:
        self.status_code = status_code
        self.body = body
        super().__init__(f"eye returned HTTP {status_code}")


class EyeServerError(EyeClientError):
    """A 5xx response: eye's own failure, not the caller's."""

    def __init__(self, status_code: int, body: str = "") -> None:
        self.status_code = status_code
        self.body = body
        super().__init__(f"eye returned a server error: HTTP {status_code}")


class EyeTimeoutError(EyeClientError):
    """The request to eye did not complete within the configured timeout."""

    def __init__(self, timeout: float) -> None:
        self.timeout = timeout
        super().__init__(f"eye did not respond within {timeout}s")
