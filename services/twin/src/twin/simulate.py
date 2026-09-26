"""A thermal what-if scenario: apply bounded deltas to the observed state
and recompute apparent temperature. Never mutates the observed values; the
response carries observed and simulated side by side. See
packages/contracts/environment/v1/simulate.response.example.json.
"""

from __future__ import annotations

from datetime import UTC, datetime

from eye_client import Record

from twin.formulas import apparent_temperature_c, knots_to_ms, relative_humidity_pct
from twin.schemas import Model, ScenarioRequest, SimulateResponse, SimulateValues, Value
from twin.state import provenance_from_record

_BOM_REFERENCE = "Australian Bureau of Meteorology apparent temperature (Steadman 1994), with wind"


def _clamp(value: float, low: float, high: float) -> float:
    return max(low, min(high, value))


def simulate_environment(
    metar_record: Record, scenario: ScenarioRequest, now: datetime | None = None
) -> SimulateResponse:
    now = now or datetime.now(UTC)
    at = metar_record.observed_at
    payload = metar_record.payload

    temperature_c = float(payload["temperature_c"])
    dewpoint_c = float(payload["dewpoint_c"])
    wind_ms = knots_to_ms(float(payload["wind_speed_kt"]))
    rh = relative_humidity_pct(temperature_c, dewpoint_c)
    observed_at_c = apparent_temperature_c(temperature_c, rh, wind_ms)

    provenance = provenance_from_record(metar_record)

    humidity_model = Model(
        name="magnus-relative-humidity",
        version="1",
        reference="Alduchov & Eskridge (1996), Magnus approximation",
        generated_at=now,
        valid_from=at,
        valid_until=at,
        input_snapshot=[metar_record.id],
        uncertainty="derived from reported temperature and dew point",
    )
    apparent_temperature_model = Model(
        name="bom-apparent-temperature",
        version="1",
        reference=_BOM_REFERENCE,
        generated_at=now,
        valid_from=at,
        valid_until=at,
        input_snapshot=[metar_record.id],
        uncertainty="shade value; no solar radiation term",
    )

    observed = SimulateValues(
        air_temperature=Value(
            value=temperature_c, unit="Cel", label="OBSERVED", at=at, provenance=provenance
        ),
        relative_humidity=Value(
            value=round(rh, 1), unit="%", label="INFERRED", at=at, model=humidity_model
        ),
        wind_speed=Value(
            value=round(wind_ms, 2), unit="m/s", label="OBSERVED", at=at, provenance=provenance
        ),
        apparent_temperature=Value(
            value=round(observed_at_c, 1),
            unit="Cel",
            label="INFERRED",
            at=at,
            model=apparent_temperature_model,
        ),
    )

    simulated_temperature_c = temperature_c + scenario.temperature_delta_c
    simulated_rh = _clamp(rh + scenario.humidity_delta_pct, 0.0, 100.0)
    simulated_wind_ms = wind_ms * scenario.wind_factor
    simulated_at_c = apparent_temperature_c(
        simulated_temperature_c, simulated_rh, simulated_wind_ms
    )

    def scenario_model(reference: str) -> Model:
        return Model(
            name="thermal-scenario",
            version="1",
            reference=reference,
            generated_at=now,
            valid_from=at,
            valid_until=at,
            input_snapshot=[metar_record.id],
            uncertainty="counterfactual; not a forecast",
        )

    simulated = SimulateValues(
        air_temperature=Value(
            value=round(simulated_temperature_c, 1),
            unit="Cel",
            label="SIMULATED",
            at=at,
            model=scenario_model("observed value + temperature_delta_c"),
        ),
        relative_humidity=Value(
            value=round(simulated_rh, 1),
            unit="%",
            label="SIMULATED",
            at=at,
            model=scenario_model("derived value + humidity_delta_pct, clamped to 0-100"),
        ),
        wind_speed=Value(
            value=round(simulated_wind_ms, 2),
            unit="m/s",
            label="SIMULATED",
            at=at,
            model=scenario_model("observed value * wind_factor"),
        ),
        apparent_temperature=Value(
            value=round(simulated_at_c, 1),
            unit="Cel",
            label="SIMULATED",
            at=at,
            model=Model(
                name="bom-apparent-temperature",
                version="1",
                reference=_BOM_REFERENCE,
                generated_at=now,
                valid_from=at,
                valid_until=at,
                input_snapshot=[metar_record.id],
                uncertainty="counterfactual; not a forecast",
            ),
        ),
    )

    top_level_model = Model(
        name="thermal-scenario",
        version="1",
        reference=(
            "observed state with the scenario deltas applied; apparent temperature "
            "per the Australian Bureau of Meteorology formula"
        ),
        generated_at=now,
        valid_from=at,
        valid_until=at,
        input_snapshot=[metar_record.id],
        uncertainty="counterfactual; not a forecast",
    )

    return SimulateResponse(
        generated_at=now,
        scenario=scenario,
        model=top_level_model,
        observed=observed,
        simulated=simulated,
    )
