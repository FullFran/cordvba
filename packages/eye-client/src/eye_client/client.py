"""A thin, typed client for eye's public HTTP API. It moves JSON; it
contains no business logic. See packages/eye-client/README.md.
"""

from __future__ import annotations

from typing import Any

import httpx

from eye_client.exceptions import (
    EyeAuthError,
    EyeHTTPError,
    EyeServerError,
    EyeTimeoutError,
)
from eye_client.models import Entity, Record, Source


class EyeClient:
    """A client bound to one eye deployment.

    The bearer token is kept private and is never included in `repr()`,
    logs, or raised exceptions.
    """

    def __init__(
        self,
        base_url: str,
        token: str,
        timeout: float = 10.0,
        transport: httpx.BaseTransport | None = None,
    ) -> None:
        self._base_url = base_url.rstrip("/")
        self._timeout = timeout
        self._http = httpx.Client(
            base_url=self._base_url,
            timeout=timeout,
            transport=transport,
            headers={"Authorization": f"Bearer {token}"},
        )

    def __repr__(self) -> str:  # pragma: no cover - trivial
        return f"EyeClient(base_url={self._base_url!r}, token=<redacted>)"

    def close(self) -> None:
        self._http.close()

    def __enter__(self) -> EyeClient:
        return self

    def __exit__(self, *exc_info: object) -> None:
        self.close()

    def records(
        self,
        *,
        source: str | None = None,
        topic: str | None = None,
        since: str | None = None,
        limit: int | None = None,
    ) -> list[Record]:
        """`GET /v1/records`. Query params bound `observed_at` (`since`) and
        cap the result set (`limit`, capped at 5000 by eye itself).
        """
        params = self._params(source=source, topic=topic, since=since, limit=limit)
        payload = self._get("/v1/records", params)
        return [Record.model_validate(item) for item in payload["records"]]

    def entities(
        self,
        *,
        source: str | None = None,
        topic: str | None = None,
        since: str | None = None,
        limit: int | None = None,
    ) -> list[Entity]:
        """`GET /v1/entities`: the inventory of things records are about."""
        params = self._params(source=source, topic=topic, since=since, limit=limit)
        payload = self._get("/v1/entities", params)
        return [Entity.model_validate(item) for item in payload["entities"]]

    def sources(self) -> list[Source]:
        """`GET /v1/sources`: the registry and each source's health."""
        payload = self._get("/v1/sources", {})
        return [Source.model_validate(item) for item in payload["sources"]]

    @staticmethod
    def _params(**kwargs: Any) -> dict[str, str]:
        return {key: str(value) for key, value in kwargs.items() if value is not None}

    def _get(self, path: str, params: dict[str, str]) -> dict[str, Any]:
        try:
            response = self._http.get(path, params=params)
        except httpx.TimeoutException as exc:
            raise EyeTimeoutError(self._timeout) from exc

        if response.status_code == 401:
            raise EyeAuthError(response.status_code)
        if response.status_code >= 500:
            raise EyeServerError(response.status_code, response.text)
        if response.status_code >= 400:
            raise EyeHTTPError(response.status_code, response.text)

        return response.json()
