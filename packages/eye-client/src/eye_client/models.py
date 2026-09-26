"""Pydantic models for the subset of eye's OpenAPI schema this client uses.

Every model keeps unknown fields (``extra="allow"``) instead of rejecting
them, so that eye can add fields to its responses without breaking every
consumer at once. See packages/eye-client/README.md; field names mirror
eye's own domain types (Record, Entity, Source) as served over HTTP,
never eye's internal Go packages directly.
"""

from __future__ import annotations

from datetime import datetime
from typing import Any

from pydantic import BaseModel, ConfigDict


class _Lenient(BaseModel):
    """Base model that keeps fields eye may add later instead of rejecting them."""

    model_config = ConfigDict(extra="allow")


class Point(_Lenient):
    lat: float
    lon: float


class Provenance(_Lenient):
    """Where an observation came from. Field names mirror eye's own JSON,
    including the American spelling of ``license``.
    """

    publisher: str
    source_url: str
    license: str
    fetched_at: datetime
    raw_hash: str


class Record(_Lenient):
    """A single observation, as returned by ``GET /v1/records``."""

    id: str
    source: str
    kind: str
    topic: str
    title: str
    description: str | None = None
    observed_at: datetime
    fetched_at: datetime
    valid_from: datetime | None = None
    valid_until: datetime | None = None
    position: Point | None = None
    quality: str
    severity: int
    confidence: float
    entity_id: str | None = None
    local_key: str | None = None
    dedupe_key: str | None = None
    expires_at: datetime | None = None
    payload: dict[str, Any] = {}
    provenance: Provenance


class Entity(_Lenient):
    """Something that persists across observations, as returned by
    ``GET /v1/entities``.
    """

    id: str
    source: str
    kind: str
    topic: str
    title: str
    description: str | None = None
    position: Point | None = None
    first_seen: datetime
    last_seen: datetime
    payload: dict[str, Any] = {}
    provenance: Provenance


class Source(_Lenient):
    """A registry entry, as returned by ``GET /v1/sources``."""

    id: str
    authority: str
    topic: str
    license: str
    access: str
    automation: str
    pollable: bool
    redistributable: bool
    url: str
