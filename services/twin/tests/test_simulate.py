"""twin.simulate: a thermal what-if scenario. It never mutates the observed
state; the response carries observed and simulated side by side, matching
packages/contracts/environment/v1/simulate.response.example.json.
"""

import pytest
from conftest import make_metar_record
from pydantic import ValidationError

from twin.schemas import ScenarioRequest
from twin.simulate import simulate_environment


def test_simulate_matches_the_contract_reference_values():
    metar = make_metar_record(temperature_c=26.0, dewpoint_c=12.0, wind_speed_kt=8.0)
    scenario = ScenarioRequest(temperature_delta_c=3, humidity_delta_pct=10, wind_factor=0.5)

    result = simulate_environment(metar, scenario)

    assert result.observed.air_temperature.value == 26.0
    assert result.observed.air_temperature.label == "OBSERVED"
    assert result.observed.relative_humidity.value == pytest.approx(41.7, abs=0.1)
    assert result.observed.relative_humidity.label == "INFERRED"
    assert result.observed.apparent_temperature.value == pytest.approx(23.7, abs=0.1)

    assert result.simulated.air_temperature.value == pytest.approx(29.0, abs=0.1)
    assert result.simulated.air_temperature.label == "SIMULATED"
    assert result.simulated.relative_humidity.value == pytest.approx(51.7, abs=0.1)
    assert result.simulated.wind_speed.value == pytest.approx(2.05, abs=0.05)
    assert result.simulated.apparent_temperature.value == pytest.approx(30.4, abs=0.1)

    assert result.model.name == "thermal-scenario"
    assert result.model.uncertainty == "counterfactual; not a forecast"
    assert result.scenario.temperature_delta_c == 3


def test_simulate_never_mutates_the_observed_values():
    metar = make_metar_record(temperature_c=26.0, dewpoint_c=12.0, wind_speed_kt=8.0)
    scenario = ScenarioRequest(temperature_delta_c=3, humidity_delta_pct=10, wind_factor=0.5)

    result = simulate_environment(metar, scenario)

    # Calling it again with a no-op scenario must reproduce the same
    # observed numbers: simulate never writes back into the record.
    zero_scenario = ScenarioRequest(temperature_delta_c=0, humidity_delta_pct=0, wind_factor=1)
    second = simulate_environment(metar, zero_scenario)

    assert result.observed.air_temperature.value == second.observed.air_temperature.value
    assert second.simulated.air_temperature.value == second.observed.air_temperature.value


def test_simulate_clamps_humidity_to_0_100():
    metar = make_metar_record(temperature_c=5.0, dewpoint_c=4.0, wind_speed_kt=0.0)
    scenario = ScenarioRequest(temperature_delta_c=0, humidity_delta_pct=50, wind_factor=1)

    result = simulate_environment(metar, scenario)

    assert result.simulated.relative_humidity.value <= 100.0


def test_scenario_request_rejects_out_of_bounds_deltas():
    with pytest.raises(ValidationError):
        ScenarioRequest(temperature_delta_c=11, humidity_delta_pct=0, wind_factor=1)
    with pytest.raises(ValidationError):
        ScenarioRequest(temperature_delta_c=0, humidity_delta_pct=-51, wind_factor=1)
    with pytest.raises(ValidationError):
        ScenarioRequest(temperature_delta_c=0, humidity_delta_pct=0, wind_factor=2.1)
