"""Configuration, read from the environment in exactly one place. Nothing
else in api calls os.environ directly. The eye token and both internal
base URLs are server-side only; the browser never sees them (see
apps/api/README.md).
"""

from __future__ import annotations

import os
from dataclasses import dataclass
from urllib.parse import urlsplit

_REQUIRED_VARS = ("EYE_BASE_URL", "EYE_API_TOKEN", "TWIN_BASE_URL", "WEB_ORIGIN")


@dataclass(frozen=True)
class Settings:
    eye_base_url: str
    eye_api_token: str
    twin_base_url: str
    web_origins: list[str]
    eye_timeout_seconds: float = 10.0
    twin_timeout_seconds: float = 10.0


def _is_origin(value: str) -> bool:
    """An origin is `scheme://host[:port]` with no path, query or fragment
    (issue #115, AC-2/AC-5): exactly what CORS `Access-Control-Allow-Origin`
    expects to echo back, nothing a browser could confuse with a full URL.
    """
    parts = urlsplit(value)
    return (
        bool(parts.scheme)
        and bool(parts.netloc)
        and not parts.path
        and not parts.query
        and not parts.fragment
    )


def _parse_web_origins(raw: str) -> list[str]:
    """Parse WEB_ORIGIN as a comma-separated list of one or more origins.
    Each entry is trimmed; an empty entry (e.g. a trailing comma or double
    comma) or one that is not a bare origin raises a clear startup error.
    """
    origins: list[str] = []
    for part in raw.split(","):
        origin = part.strip()
        if not origin:
            raise RuntimeError(
                "WEB_ORIGIN contains an empty origin entry (check for stray or "
                "trailing commas)"
            )
        if not _is_origin(origin):
            raise RuntimeError(
                f"WEB_ORIGIN contains an invalid origin {origin!r}: expected "
                "scheme://host[:port] with no path"
            )
        origins.append(origin)
    return origins


def load_settings() -> Settings:
    """Read EYE_BASE_URL, EYE_API_TOKEN, TWIN_BASE_URL and WEB_ORIGIN, or
    raise a clear error naming the missing variable. WEB_ORIGIN may be a
    single origin or a comma-separated list of origins.
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
        web_origins=_parse_web_origins(values["WEB_ORIGIN"]),
    )
