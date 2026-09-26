"""Pure, cited meteorological functions. No I/O, no eye_client, no FastAPI:
these are unit-testable in isolation and reused by both the observed-state
and the scenario endpoints.
"""

from __future__ import annotations

import math

#: Knots to metres per second (1 kt = 1852 m / 3600 s).
KNOTS_TO_MS = 0.514444


def knots_to_ms(knots: float) -> float:
    """Convert wind speed from knots to metres per second."""
    return knots * KNOTS_TO_MS


def relative_humidity_pct(temperature_c: float, dewpoint_c: float) -> float:
    """Relative humidity from temperature and dew point, Magnus approximation.

    Reference: Alduchov & Eskridge (1996).
    RH = 100 * exp(17.625*Td / (243.04+Td)) / exp(17.625*T / (243.04+T))
    """
    numerator = math.exp(17.625 * dewpoint_c / (243.04 + dewpoint_c))
    denominator = math.exp(17.625 * temperature_c / (243.04 + temperature_c))
    return 100.0 * numerator / denominator


def apparent_temperature_c(
    temperature_c: float, relative_humidity_pct_: float, wind_ms: float
) -> float:
    """Apparent temperature, Australian Bureau of Meteorology formula
    (Steadman 1994), with wind.

    e = RH/100 * 6.105 * exp(17.27*Ta / (237.7+Ta))
    AT = Ta + 0.33*e - 0.70*ws - 4.00
    """
    vapour_pressure = (
        relative_humidity_pct_
        / 100.0
        * 6.105
        * math.exp(17.27 * temperature_c / (237.7 + temperature_c))
    )
    return temperature_c + 0.33 * vapour_pressure - 0.70 * wind_ms - 4.00
