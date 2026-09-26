"""Typed Python client for eye's public HTTP API.

This package moves JSON between eye and its consumers (the twin, the API).
It contains no business logic: parsing and validation only.
"""

from eye_client.client import EyeClient
from eye_client.exceptions import (
    EyeAuthError,
    EyeClientError,
    EyeHTTPError,
    EyeServerError,
    EyeTimeoutError,
)
from eye_client.models import Entity, Provenance, Record, Source

__all__ = [
    "EyeClient",
    "EyeClientError",
    "EyeAuthError",
    "EyeHTTPError",
    "EyeServerError",
    "EyeTimeoutError",
    "Record",
    "Provenance",
    "Entity",
    "Source",
]
