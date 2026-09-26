"""Record/Provenance/Source models parse eye's real payload shapes.

Fixtures under tests/fixtures/ are real responses recorded from a deployed
eye instance (NOAA METAR data is US public domain; MITECO ICA data is
CC BY 4.0).
"""

import json
from datetime import datetime
from pathlib import Path

import pytest
from eye_client.models import Provenance, Record, Source
from pydantic import ValidationError

FIXTURES = Path(__file__).parent / "fixtures"


def _load(name: str) -> dict:
    return json.loads((FIXTURES / name).read_text())


def test_record_parses_metar_fixture_with_typed_provenance():
    raw = _load("records_metar.json")["records"][0]

    record = Record.model_validate(raw)

    assert record.id == raw["id"]
    assert record.source == "metar-cordoba"
    assert record.topic == "weather"
    assert record.local_key == "LEBA"
    assert record.dedupe_key == raw["dedupe_key"]
    assert record.expires_at is not None
    assert isinstance(record.provenance, Provenance)
    assert record.provenance.publisher == "NOAA Aviation Weather Center"
    assert record.provenance.license == "us-government-public-domain"
    assert record.provenance.source_url == raw["provenance"]["source_url"]
    assert record.provenance.raw_hash == raw["provenance"]["raw_hash"]


def test_record_keeps_unknown_payload_fields():
    raw = _load("records_metar.json")["records"][0]

    record = Record.model_validate(raw)

    # payload is a passthrough dict: nothing is dropped or rejected.
    assert record.payload["temperature_c"] == 26 or record.payload["temperature_c"] == 28
    assert "raw" in record.payload


def test_record_tolerates_a_brand_new_unknown_top_level_field():
    raw = _load("records_metar.json")["records"][0] | {"a_field_eye_added_tomorrow": "x"}

    record = Record.model_validate(raw)

    assert record.model_extra.get("a_field_eye_added_tomorrow") == "x"


def test_record_entity_id_defaults_to_none_when_absent():
    raw = _load("records_ica.json")["records"][0]
    assert "entity_id" not in raw

    record = Record.model_validate(raw)

    assert record.entity_id is None


def test_ica_record_exposes_index_reported_false_for_stations_without_data():
    raw = _load("records_ica.json")["records"][0]

    record = Record.model_validate(raw)

    assert record.payload["index_reported"] is False
    assert record.payload["index"] == 0


def test_record_parses_valid_from_and_valid_until_as_datetimes():
    """A warning (aemet-warnings) or any other forecast/scheduled-window
    record carries valid_from/valid_until on eye's own Record type, as
    served over HTTP. This payload mirrors the real Campina cordobesa
    warning in apps/eye's recorded MeteoAlarm fixture, mapped through
    eye's own aemet warnings adapter, not a live eye response (no deployed
    instance carries a Cordoba warning on demand).
    """
    raw = {
        "id": "aemet-warnings:2.49.0.0.724.0.ES.260902093610.611402ATTA041941770",
        "source": "aemet-warnings",
        "kind": "weather_warning",
        "topic": "weather",
        "title": "Orange High-temperature Warning issued for Spain - Campiña cordobesa",
        "description": "Severe high-temperature warning · Campiña cordobesa",
        "observed_at": "2026-09-02T09:36:10Z",
        "fetched_at": "2026-09-26T09:40:00Z",
        "valid_from": "2026-09-04T11:00:00Z",
        "valid_until": "2026-09-04T18:59:59Z",
        "quality": "official",
        "severity": 4,
        "confidence": 1,
        "dedupe_key": "aemet-warnings:2.49.0.0.724.0.ES.260902093610.611402ATTA041941770",
        "payload": {
            "issuing_authority": "AEMET. Agencia Estatal de Meteorología",
            "relayed_by": "meteoalarm.org",
            "zone": "ES079",
            "area": "Campiña cordobesa",
        },
        "provenance": {
            "publisher": "AEMET. Agencia Estatal de Meteorologia",
            "source_url": "https://feeds.meteoalarm.org/feeds/meteoalarm-legacy-atom-spain",
            "license": "cc-by-4.0-equivalent-meteoalarm-terms",
            "fetched_at": "2026-09-26T09:40:00Z",
            "raw_hash": "hash",
        },
    }

    record = Record.model_validate(raw)

    assert isinstance(record.valid_from, datetime)
    assert isinstance(record.valid_until, datetime)
    assert record.valid_from < record.valid_until


def test_record_valid_from_and_valid_until_default_to_none_when_absent():
    raw = _load("records_metar.json")["records"][0]
    assert "valid_from" not in raw and "valid_until" not in raw

    record = Record.model_validate(raw)

    assert record.valid_from is None
    assert record.valid_until is None


def test_source_parses_registry_fixture():
    raw = _load("sources.json")["sources"][0]

    source = Source.model_validate(raw)

    assert source.id == "metar-cordoba"
    assert source.redistributable is True
    assert source.license == "us-government-public-domain"


def test_provenance_rejects_a_missing_required_field():
    with pytest.raises(ValidationError):
        Provenance.model_validate({"publisher": "MITECO"})
