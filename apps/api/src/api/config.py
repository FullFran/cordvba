"""Configuration, read from the environment in exactly one place. Nothing
else in api calls os.environ directly. The eye token and both internal
base URLs are server-side only; the browser never sees them (see
apps/api/README.md).
"""

from __future__ import annotations

import os
from dataclasses import dataclass

_REQUIRED_VARS = ("EYE_BASE_URL", "EYE_API_TOKEN", "TWIN_BASE_URL", "WEB_ORIGIN")


@dataclass(frozen=True)
class Settings:
    eye_base_url: str
    eye_api_token: str
    twin_base_url: str
    web_origin: str
    eye_timeout_seconds: float = 10.0
    twin_timeout_seconds: float = 10.0


def load_settings() -> Settings:
    """Read EYE_BASE_URL, EYE_API_TOKEN, TWIN_BASE_URL and WEB_ORIGIN, or
    raise a clear error naming the missing variable.
    """
    values = {}
    for name in _REQUIRED_VARS:
        value = os.environ.get(name)
        if not value:
            raise RuntimeError(f"{name} is not set")
        values[name] = value

    return Settings(
        eye_base_url=values["EYE_BASE_URL"],
        eye_api_token=values["EYE_API_TOKEN"],
        twin_base_url=values["TWIN_BASE_URL"],
        web_origin=values["WEB_ORIGIN"],
    )
