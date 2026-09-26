"""The only module in twin that calls eye_client. Isolating the reads here
keeps twin.state / twin.forecast / twin.simulate pure and unit-testable
without HTTP.
"""

from __future__ import annotations

from typing import Protocol

from eye_client import Record


class RecordsSource(Protocol):
    def records(self, **kwargs: object) -> list[Record]: ...


def fetch_latest_metar(client: RecordsSource) -> Record | None:
    """The newest METAR record for Córdoba airport, or None if eye has
    nothing yet.
    """
    records = client.records(source="metar-cordoba", topic="weather", limit=1)
    return records[0] if records else None


def fetch_ica_records(client: RecordsSource, limit: int = 500) -> list[Record]:
    """Every recent ICA record eye holds. Station/city filtering happens in
    twin.state, not here: this module only fetches.
    """
    return client.records(source="miteco-ica", topic="air_quality", limit=limit)
