"""api.pulse composes eye's raw records, across every pulse source, into
the public v1 pulse response. api never imports eye's internal code (see
docs/architecture/system-overview.md): these functions take eye_client's
typed Record/Source, as returned over HTTP. See
packages/contracts/pulse/v1/README.md.
"""

from __future__ import annotations

from datetime import UTC, datetime, timedelta

from conftest import make_eye_source
from eye_client import Record

from api.pulse import pulse_from_eye
from api.schemas import PulseResponse

NOW = datetime(2026, 9, 26, 9, 40, tzinfo=UTC)


def _record(**overrides) -> Record:
    base = {
        "id": "aemet-warnings:test-1",
        "source": "aemet-warnings",
        "kind": "weather_warning",
        "topic": "weather",
        "title": "Orange High-temperature Warning issued for Spain - Campiña cordobesa",
        "observed_at": "2026-09-25T10:36:10Z",
        "fetched_at": "2026-09-26T09:37:00Z",
        "valid_from": "2026-09-26T11:00:00Z",
        "valid_until": "2026-09-26T19:00:00Z",
        "quality": "official",
        "severity": 4,
        "confidence": 1,
        "dedupe_key": "aemet-warnings:test-1",
        "payload": {
            "area": "Campiña cordobesa",
            "cap_document": "https://feeds.meteoalarm.org/api/v1/warnings/x",
        },
        "provenance": {
            "publisher": "AEMET. Agencia Estatal de Meteorologia",
            "source_url": "https://feeds.meteoalarm.org/feeds/meteoalarm-legacy-atom-spain",
            "license": "cc-by-4.0-equivalent-meteoalarm-terms",
            "fetched_at": "2026-09-26T09:37:00Z",
            "raw_hash": "hash",
        },
    }
    base.update(overrides)
    return Record.model_validate(base)


def test_pulse_from_eye_maps_a_warning_by_area_not_position():
    sources = [
        make_eye_source(
            source_id="aemet-warnings", license_="cc-by-4.0-equivalent-meteoalarm-terms"
        )
    ]

    result = pulse_from_eye([_record()], sources, window_hours=24, now=NOW)

    assert isinstance(result, PulseResponse)
    assert result.window_hours == 24
    assert len(result.items) == 1
    item = result.items[0]
    assert item.kind == "warning"
    assert item.label == "OBSERVED"
    assert item.area == "Campiña cordobesa"
    assert item.position is None
    assert item.severity == "high"
    assert item.url == "https://feeds.meteoalarm.org/api/v1/warnings/x"
    assert item.provenance.source == "aemet-warnings"
    assert item.provenance.licence == "cc-by-4.0-equivalent-meteoalarm-terms"
    assert "AEMET" in item.attribution


def test_pulse_from_eye_maps_an_event_as_published_with_no_location():
    record = _record(
        id="uco-events:test-1",
        source="uco-events",
        kind="event_listing",
        topic="events",
        title="Congreso Internacional",
        observed_at="2026-09-26T09:38:00Z",
        valid_from="2026-10-07T07:00:00Z",
        valid_until=None,
        severity=1,
        payload={"link": "http://eventos.uco.es/event_detail/1.html"},
        provenance={
            "publisher": "Universidad de Cordoba",
            "source_url": "https://eventos.uco.es/rss/next.rss",
            "license": "unspecified",
            "fetched_at": "2026-09-26T09:38:00Z",
            "raw_hash": "hash",
        },
    )
    sources = [make_eye_source(source_id="uco-events", license_="unspecified")]

    result = pulse_from_eye([record], sources, window_hours=24, now=NOW)

    item = result.items[0]
    assert item.kind == "event"
    assert item.label == "PUBLISHED"
    assert item.position is None
    assert item.area is None
    assert item.valid_from is not None
    assert item.url == "http://eventos.uco.es/event_detail/1.html"


def test_pulse_from_eye_keeps_a_fire_inside_the_cordoba_bbox():
    record = _record(
        id="nasa-firms:test-1",
        source="nasa-firms",
        kind="fire_detection",
        topic="fire",
        title="Thermal anomaly, 5.2 MW (VIIRS_SNPP_NRT)",
        observed_at="2026-09-25T14:27:00Z",
        valid_from=None,
        valid_until=None,
        position={"lat": 37.9, "lon": -4.8},
        severity=2,
        payload={"latitude": "37.9", "longitude": "-4.8", "frp": "5.2"},
        provenance={
            "publisher": "NASA FIRMS",
            "source_url": "https://firms.modaps.eosdis.nasa.gov/data/x.csv",
            "license": "nasa-open-data",
            "fetched_at": "2026-09-26T09:33:00Z",
            "raw_hash": "hash",
        },
    )
    sources = [make_eye_source(source_id="nasa-firms", license_="nasa-open-data")]

    result = pulse_from_eye([record], sources, window_hours=24, now=NOW)

    assert len(result.items) == 1
    item = result.items[0]
    assert item.kind == "fire"
    assert item.label == "OBSERVED"
    assert item.position.lat == 37.9
    assert item.position.lon == -4.8
    assert item.severity == "low"
    assert item.url is None


def test_pulse_from_eye_drops_a_quake_outside_the_cordoba_bbox():
    record = _record(
        id="ign-seismic:test-1",
        source="ign-seismic",
        kind="seismic_event",
        topic="geophysics",
        title="-Info.terremoto: 26/08/2026 17:58:16",
        observed_at="2026-09-26T09:00:00Z",
        valid_from=None,
        valid_until=None,
        position={"lat": 28.8273, "lon": -16.3017},  # Canary Islands
        severity=2,
        payload={"link": "http://www.ign.es/x", "magnitude": 2.8},
        provenance={
            "publisher": "Instituto Geografico Nacional",
            "source_url": "https://www.ign.es/ign/RssTools/sismologia.xml",
            "license": "official-cite-ign",
            "fetched_at": "2026-09-26T09:36:00Z",
            "raw_hash": "hash",
        },
    )
    sources = [make_eye_source(source_id="ign-seismic", license_="official-cite-ign")]

    result = pulse_from_eye([record], sources, window_hours=24, now=NOW)

    assert result.items == []


def test_pulse_from_eye_excludes_a_source_eye_reports_as_not_redistributable():
    sources = [
        make_eye_source(
            source_id="aemet-warnings",
            license_="cc-by-4.0-equivalent-meteoalarm-terms",
            redistributable=False,
        )
    ]

    result = pulse_from_eye([_record()], sources, window_hours=24, now=NOW)

    assert result.items == []


def test_pulse_from_eye_ignores_a_record_from_a_source_pulse_does_not_recognise():
    record = _record(id="metar-cordoba:test-1", source="metar-cordoba")
    sources = [make_eye_source(source_id="metar-cordoba", license_="us-government-public-domain")]

    result = pulse_from_eye([record], sources, window_hours=24, now=NOW)

    assert result.items == []


def test_pulse_from_eye_sorts_newest_first_and_bounds_to_fifty():
    sources = [
        make_eye_source(
            source_id="aemet-warnings", license_="cc-by-4.0-equivalent-meteoalarm-terms"
        )
    ]
    records = [
        _record(
            id=f"aemet-warnings:test-{i}",
            observed_at=(NOW - timedelta(hours=i)).isoformat(),
        )
        for i in range(60)
    ]

    result = pulse_from_eye(records, sources, window_hours=72, now=NOW)

    assert len(result.items) == 50
    observed_at_values = [item.observed_at for item in result.items]
    assert observed_at_values == sorted(observed_at_values, reverse=True)
    assert result.items[0].id == "aemet-warnings:test-0"
    assert result.items[-1].id == "aemet-warnings:test-49"
