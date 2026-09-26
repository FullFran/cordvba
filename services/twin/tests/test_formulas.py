"""Table-driven tests for twin's pure meteorological functions, against the
reference values published in packages/contracts/environment/v1/README.md
(METAR at Córdoba airport, 2026-09-26 09:00Z: 26 °C, dew point 12 °C, 8 kt).
"""

import pytest

from twin.formulas import apparent_temperature_c, knots_to_ms, relative_humidity_pct

TOLERANCE = 0.1


@pytest.mark.parametrize(
    ("temperature_c", "dewpoint_c", "expected_rh_pct"),
    [
        (26.0, 12.0, 41.7),
    ],
)
def test_relative_humidity_matches_the_contract_reference_value(
    temperature_c, dewpoint_c, expected_rh_pct
):
    rh = relative_humidity_pct(temperature_c, dewpoint_c)

    assert rh == pytest.approx(expected_rh_pct, abs=TOLERANCE)


def test_relative_humidity_is_100_when_temperature_equals_dewpoint():
    assert relative_humidity_pct(20.0, 20.0) == pytest.approx(100.0, abs=TOLERANCE)


@pytest.mark.parametrize(
    ("knots", "expected_ms"),
    [
        (8.0, 4.11552),
        (0.0, 0.0),
    ],
)
def test_knots_to_ms(knots, expected_ms):
    assert knots_to_ms(knots) == pytest.approx(expected_ms, abs=1e-3)


@pytest.mark.parametrize(
    ("temperature_c", "rh_pct", "wind_ms", "expected_at_c"),
    [
        # Observed reading: 26 °C, RH 41.7 %, wind 4.1 m/s -> AT 23.7 °C.
        (26.0, 41.7, 4.1, 23.7),
        # Scenario: +3 °C, +10 points of RH, half the wind -> AT 30.4 °C.
        (29.0, 51.7, 2.05, 30.4),
    ],
)
def test_apparent_temperature_matches_the_contract_reference_values(
    temperature_c, rh_pct, wind_ms, expected_at_c
):
    at = apparent_temperature_c(temperature_c, rh_pct, wind_ms)

    assert at == pytest.approx(expected_at_c, abs=TOLERANCE)
