"""A small HTTP client for twin's internal endpoints. twin is never reached
by the browser; only api calls it, over plain HTTP inside the deployment
network. See services/twin/README.md for the endpoints this wraps.
"""

from __future__ import annotations

from typing import Any

import httpx


class TwinError(Exception):
    """twin returned an error, or did not respond in time."""


class TwinClient:
    def __init__(
        self, base_url: str, timeout: float = 10.0, transport: httpx.BaseTransport | None = None
    ) -> None:
        self._http = httpx.Client(
            base_url=base_url.rstrip("/"), timeout=timeout, transport=transport
        )

    def close(self) -> None:
        self._http.close()

    def state_environment(self) -> dict[str, Any]:
        return self._get("/state/environment")

    def forecast_air_quality(self, horizons: list[int]) -> dict[str, Any]:
        horizons_param = ",".join(str(h) for h in horizons)
        return self._get("/forecast/air-quality", params={"horizons": horizons_param})

    def simulate_environment(self, scenario: dict[str, Any]) -> dict[str, Any]:
        try:
            response = self._http.post("/simulate/environment", json=scenario)
        except httpx.TimeoutException as exc:
            raise TwinError("twin did not respond in time") from exc
        return self._raise_for_status(response)

    def _get(self, path: str, params: dict[str, str] | None = None) -> dict[str, Any]:
        try:
            response = self._http.get(path, params=params or {})
        except httpx.TimeoutException as exc:
            raise TwinError("twin did not respond in time") from exc
        return self._raise_for_status(response)

    @staticmethod
    def _raise_for_status(response: httpx.Response) -> dict[str, Any]:
        if response.status_code >= 400:
            raise TwinError(f"twin returned HTTP {response.status_code}")
        return response.json()
