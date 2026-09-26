"""Composes eye's raw records, across every pulse source, into the public
v1 pulse response. api never imports eye's internal code (see
docs/architecture/system-overview.md); it only reshapes eye_client's typed
Record/Source, as returned over HTTP. See
packages/contracts/pulse/v1/README.md.
"""

from __future__ import annotations

from datetime import UTC, datetime

from eye_client import Record, Source

from api.schemas import Position, PulseItem, PulseProvenance, PulseResponse

#: A response never carries more than this many items, whatever `hours` asked
#: for (packages/contracts/pulse/v1/README.md, "Bounds").
MAX_ITEMS = 50

#: Cordoba province bounding box (west, south, east, north): the union of the
#: three real AEMET warning-zone polygons Cordoba province is made of (Sierra
#: y Pedroches, Campina cordobesa, Subbetica cordobesa), read from
#: apps/eye/testdata/aemet/cap/*.xml and
#: apps/eye/testdata/meteoalarm/spain-warnings.xml — not a guess. `fire` and
#: `quake` records carry only a latitude and longitude, never a place code the
#: way `miteco-ica` does (`province: "14"`, #98), so a bounding box is the
#: only filter available for those two kinds.
CORDOBA_BBOX = (-5.55, 37.25, -4.05, 38.72)

#: Which pulse `kind` each eye source id feeds. A record from a source not
#: listed here is never surfaced, even if eye returns one (defence in depth
#: against a future source reusing an id by accident).
_KIND_FOR_SOURCE: dict[str, str] = {
    "aemet-warnings": "warning",
    "nasa-firms": "fire",
    "ign-seismic": "quake",
    "uco-events": "event",
    "cordopolis": "headline",
    "eldiadecordoba": "headline",
    "boe": "bulletin",
}

#: OBSERVED: a sensor or an authority detected a real physical condition.
#: PUBLISHED: somebody published a statement. See
#: packages/contracts/pulse/v1/README.md, "label: OBSERVED vs PUBLISHED".
_LABEL_FOR_KIND: dict[str, str] = {
    "warning": "OBSERVED",
    "fire": "OBSERVED",
    "quake": "OBSERVED",
    "event": "PUBLISHED",
    "headline": "PUBLISHED",
    "bulletin": "PUBLISHED",
}

#: eye's own cross-source severity scale (its observation domain's Record
#: type), rendered as a name so the contract never leaks its internal 0-5
#: encoding.
_SEVERITY_NAMES: dict[int, str] = {
    0: "none",
    1: "info",
    2: "low",
    3: "moderate",
    4: "high",
    5: "critical",
}

#: The eye source ids GET /v1/pulse asks eye for. api.app fetches exactly
#: these and no others.
PULSE_SOURCE_IDS: tuple[str, ...] = tuple(_KIND_FOR_SOURCE)

#: One line naming the publisher and licence, ready to render next to an
#: item. Keyed by eye source id, same shape as api.compose's
#: _SOURCE_ATTRIBUTIONS for the environment contract.
_PULSE_ATTRIBUTIONS: dict[str, str] = {
    "aemet-warnings": (
        "Weather warning: AEMET, relayed by MeteoAlarm.org (terms equivalent to CC BY 4.0)"
    ),
    "nasa-firms": "Fire detection: NASA FIRMS (NASA open data)",
    "ign-seismic": "Earthquake: Instituto Geográfico Nacional (official citation required)",
    "uco-events": "Event: Universidad de Córdoba",
    "cordopolis": "News: Cordópolis (eldiario.es)",
    "eldiadecordoba": "News: El Día de Córdoba (Grupo Joly)",
    "boe": "Official bulletin: Agencia Estatal Boletín Oficial del Estado",
}


def _within_cordoba_bbox(record: Record) -> bool:
    if record.position is None:
        return False
    west, south, east, north = CORDOBA_BBOX
    return west <= record.position.lon <= east and south <= record.position.lat <= north


def _area(record: Record) -> str | None:
    value = record.payload.get("area")
    return value if isinstance(value, str) and value else None


def _url(record: Record) -> str | None:
    """A link to the source's own item, when it publishes one. AEMET's
    warning payload carries the meteoalarm.org document; every rss-sourced
    kind (event, headline, bulletin, quake) carries the article/event/CAP
    link under `link`. NASA FIRMS publishes raw detections with no per-item
    page, so a fire item's url is always None.
    """
    for key in ("cap_document", "link"):
        value = record.payload.get(key)
        if isinstance(value, str) and value:
            return value
    return None


def _pulse_item(record: Record, kind: str) -> PulseItem:
    position = (
        Position(lat=record.position.lat, lon=record.position.lon) if record.position else None
    )
    return PulseItem(
        kind=kind,
        id=record.id,
        title=record.title,
        observed_at=record.observed_at,
        valid_from=record.valid_from,
        valid_until=record.valid_until,
        position=position,
        area=_area(record),
        severity=_SEVERITY_NAMES.get(record.severity, "info"),
        url=_url(record),
        label=_LABEL_FOR_KIND[kind],
        provenance=PulseProvenance(
            source=record.source,
            publisher=record.provenance.publisher,
            licence=record.provenance.license,
            fetched_at=record.fetched_at,
        ),
        attribution=_PULSE_ATTRIBUTIONS[record.source],
    )


def pulse_from_eye(
    records: list[Record],
    sources: list[Source],
    *,
    window_hours: int,
    now: datetime | None = None,
) -> PulseResponse:
    """Build `GET /v1/pulse`'s response from eye's raw records across every
    pulse source: only redistributable, recognised sources; `fire` and
    `quake` narrowed to `CORDOBA_BBOX`; newest first by `observed_at`;
    bounded to `MAX_ITEMS`.
    """
    now = now or datetime.now(UTC)
    redistributable_ids = {source.id for source in sources if source.redistributable}

    items: list[PulseItem] = []
    for record in records:
        kind = _KIND_FOR_SOURCE.get(record.source)
        if kind is None or record.source not in redistributable_ids:
            continue
        if kind in ("fire", "quake") and not _within_cordoba_bbox(record):
            continue
        items.append(_pulse_item(record, kind))

    items.sort(key=lambda item: item.observed_at, reverse=True)
    return PulseResponse(generated_at=now, window_hours=window_hours, items=items[:MAX_ITEMS])
