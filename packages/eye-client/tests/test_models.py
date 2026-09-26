"""Record/Provenance/Source models parse eye's real payload shapes.

Fixtures under tests/fixtures/ are real responses recorded from a deployed
eye instance (NOAA METAR data is US public domain; MITECO ICA data is
CC BY 4.0).
"""

import json
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


def test_source_parses_registry_fixture():
    raw = _load("sources.json")["sources"][0]

    source = Source.model_validate(raw)

    assert source.id == "metar-cordoba"
    assert source.redistributable is True
    assert source.license == "us-government-public-domain"


def test_provenance_rejects_a_missing_required_field():
    with pytest.raises(ValidationError):
        Provenance.model_validate({"publisher": "MITECO"})
