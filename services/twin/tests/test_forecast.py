"""twin.forecast: a persistence baseline per city station. The forecast
value always equals the last observed one; every point is labelled
PREDICTED with model metadata and an honest uncertainty string (never a
confidence number).
"""

from conftest import make_ica_record

from twin.forecast import air_quality_forecast


def test_persistence_forecast_repeats_the_last_observed_index_per_horizon():
    records = [
        make_ica_record(
            station="14021009",
            index=4,
            category="desfavorable",
            due_to="PM10",
            name="AVDA. AL-NASIR",
            observed_at="2026-09-26T08:00:00Z",
        )
    ]

    forecast = air_quality_forecast(records, horizons=[1, 3, 6])

    assert forecast.horizons_h == [1, 3, 6]
    assert len(forecast.stations) == 1
    station = forecast.stations[0]
    assert station.id == "14021009"
    assert station.last_observed.value == 4
    assert len(station.forecast) == 3
    for point, expected_horizon in zip(station.forecast, [1, 3, 6], strict=True):
        assert point.horizon_h == expected_horizon
        assert point.value.value == 4
        assert point.value.category == "poor"
        assert point.value.label == "PREDICTED"
        assert point.value.model.name == "persistence"
        assert point.value.model.uncertainty == "baseline, not evaluated"
        assert point.value.provenance is None


def test_persistence_forecast_never_states_a_confidence_number():
    records = [make_ica_record(station="14021006", index=2, category="razonablemente buena")]

    forecast = air_quality_forecast(records, horizons=[1])

    point = forecast.stations[0].forecast[0]
    # uncertainty is free text; nothing here should look like a numeric score.
    assert isinstance(point.value.model.uncertainty, str)
    assert point.value.model.uncertainty == "baseline, not evaluated"


def test_forecast_ignores_stations_outside_cordoba_city():
    records = [make_ica_record(station="13071014", index=0, index_reported=False)]

    forecast = air_quality_forecast(records, horizons=[1])

    assert forecast.stations == []
