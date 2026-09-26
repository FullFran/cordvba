"""Configuration, read from the environment in exactly one place. Nothing
else in twin calls os.environ directly.
"""

from __future__ import annotations

import os
from dataclasses import dataclass


@dataclass(frozen=True)
class Settings:
    eye_base_url: str
    eye_api_token: str
    eye_timeout_seconds: float = 10.0


def load_settings() -> Settings:
    """Read EYE_BASE_URL and EYE_API_TOKEN, or raise a clear error."""
    eye_base_url = os.environ.get("EYE_BASE_URL")
    if not eye_base_url:
        raise RuntimeError("EYE_BASE_URL is not set")

    eye_api_token = os.environ.get("EYE_API_TOKEN")
    if not eye_api_token:
        raise RuntimeError("EYE_API_TOKEN is not set")

    timeout_raw = os.environ.get("EYE_TIMEOUT_SECONDS", "10.0")
    try:
        eye_timeout_seconds = float(timeout_raw)
    except ValueError as exc:
        raise RuntimeError(f"EYE_TIMEOUT_SECONDS is not a number: {timeout_raw!r}") from exc

    return Settings(
        eye_base_url=eye_base_url,
        eye_api_token=eye_api_token,
        eye_timeout_seconds=eye_timeout_seconds,
    )
