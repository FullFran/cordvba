"""apps/api's pydantic response models must match
packages/contracts/environment/v1/*.example.json exactly: these examples
are the canonical shapes both api and web test against (see
packages/contracts/environment/v1/README.md).
"""

import json
from pathlib import Path

from api.schemas import (
    EnvironmentResponse,
    PulseResponse,
    SimulateRequest,
    SimulateResponse,
    SourcesResponse,
    TimelineResponse,
)

CONTRACT_DIR = Path(__file__).parents[3] / "packages" / "contracts" / "environment" / "v1"
PULSE_CONTRACT_DIR = Path(__file__).parents[3] / "packages" / "contracts" / "pulse" / "v1"


def _load(name: str) -> dict:
    return json.loads((CONTRACT_DIR / name).read_text())


def _strip_none(value):
    """Recursively drop keys whose value is None, so that "the field is
    null" and "the field is absent" compare equal: the contract examples
    use both (provenance/model are explicit nulls; category/due_to/
    horizon_h are simply omitted when they do not apply).
    """
    if isinstance(value, dict):
        return {k: _strip_none(v) for k, v in value.items() if v is not None}
    if isinstance(value, list):
        return [_strip_none(item) for item in value]
    return value


def test_environment_example_matches_the_model_exactly():
    raw = _load("environment.example.json")

    model = EnvironmentResponse.model_validate(raw)

    assert model.model_dump(mode="json", exclude_none=True) == _strip_none(raw)


def test_timeline_example_matches_the_model_exactly():
    raw = _load("timeline.example.json")

    model = TimelineResponse.model_validate(raw)

    assert model.model_dump(mode="json", exclude_none=True) == _strip_none(raw)


def test_simulate_request_example_matches_the_model_exactly():
    raw = _load("simulate.request.example.json")

    model = SimulateRequest.model_validate(raw)

    assert model.model_dump(mode="json", exclude_none=True) == _strip_none(raw)


def test_simulate_response_example_matches_the_model_exactly():
    raw = _load("simulate.response.example.json")

    model = SimulateResponse.model_validate(raw)

    assert model.model_dump(mode="json", exclude_none=True) == _strip_none(raw)


def test_sources_example_matches_the_model_exactly():
    raw = _load("sources.example.json")

    model = SourcesResponse.model_validate(raw)

    assert model.model_dump(mode="json", exclude_none=True) == _strip_none(raw)


def _load_pulse(name: str) -> dict:
    return json.loads((PULSE_CONTRACT_DIR / name).read_text())


def test_pulse_example_matches_the_model_exactly():
    raw = _load_pulse("pulse.example.json")

    model = PulseResponse.model_validate(raw)

    assert model.model_dump(mode="json", exclude_none=True) == _strip_none(raw)


def test_pulse_example_covers_every_kind():
    raw = _load_pulse("pulse.example.json")

    kinds = {item["kind"] for item in raw["items"]}

    assert kinds == {"warning", "fire", "quake", "event", "headline", "bulletin"}
