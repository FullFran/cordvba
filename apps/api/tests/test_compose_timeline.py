"""api.compose.timeline_from_eye_and_twin: observed points from eye's
history, plus predicted points from twin's forecast, one ordered series
per variable (see packages/contracts/environment/v1/timeline.example.json).
"""

from datetime import UTC, datetime

from conftest import make_metar_record
from eye_client import Record

from api.compose import timeline_from_eye_and_twin
from api.schemas import TimelineResponse


def make_ica_record(*, observed_at: str, index: int, category: str, due_to: str = "PM10") -> Record:
    record_id = f"miteco-ica:14021009:{observed_at}"
    return Record.model_validate(
        {
            "id": record_id,
            "source": "miteco-ica",
            "kind": "air_quality_index",
            "topic": "air_quality",
            "title": "AVDA. AL-NASIR: calidad del aire desfavorable",
            "observed_at": observed_at,
            "fetched_at": "2026-09-26T09:35:40Z",
            "position": {"lat": 37.8926, "lon": -4.7801},
            "quality": "official",
            "severity": 0,
            "confidence": 1.0,
            "local_key": "14021009",
            "dedupe_key": record_id,
            "payload": {
                "station": "14021009",
                "index": index,
                "index_reported": True,
                "category": category,
                "due_to": due_to,
                "active": True,
                "station_type": "TRAFICO",
            },
            "provenance": {
                "publisher": "MITECO",
                "source_url": "https://ica.miteco.es/datos/ica-ultima-hora.csv",
                "license": "CC-BY-4.0",
                "fetched_at": "2026-09-26T09:35:40Z",
                "raw_hash": "hash",
            },
        }
    )


TWIN_FORECAST_ONE_STATION = {
    "generated_at": "2026-09-26T09:40:00Z",
    "horizons_h": [1, 3],
    "stations": [
        {
            "id": "14021009",
            "name": "AVDA. AL-NASIR",
            "lat": 37.8926,
            "lon": -4.7801,
            "last_observed": {
                "value": 4,
                "unit": "ica",
                "label": "OBSERVED",
                "at": "2026-09-26T08:00:00Z",
                "category": "poor",
                "category_source": "desfavorable",
                "provenance": None,
                "model": None,
            },
            "forecast": [
                {
                    "horizon_h": 1,
                    "value": {
                        "value": 4,
                        "unit": "ica",
                        "label": "PREDICTED",
                        "at": "2026-09-26T09:00:00Z",
                        "category": "poor",
                        "category_source": "desfavorable",
                        "provenance": None,
                        "model": {
                            "name": "persistence",
                            "version": "1",
                            "reference": (
                                "naive baseline: the forecast equals the last observed index"
                            ),
                            "generated_at": "2026-09-26T09:40:00Z",
                            "valid_from": "2026-09-26T09:00:00Z",
                            "valid_until": "2026-09-26T09:00:00Z",
                            "input_snapshot": ["rec_ica_14021009"],
                            "uncertainty": "baseline, not evaluated",
                        },
                    },
                },
                {
                    "horizon_h": 3,
                    "value": {
                        "value": 4,
                        "unit": "ica",
                        "label": "PREDICTED",
                        "at": "2026-09-26T11:00:00Z",
                        "category": "poor",
                        "category_source": "desfavorable",
                        "provenance": None,
                        "model": {
                            "name": "persistence",
                            "version": "1",
                            "reference": (
                                "naive baseline: the forecast equals the last observed index"
                            ),
                            "generated_at": "2026-09-26T09:40:00Z",
                            "valid_from": "2026-09-26T11:00:00Z",
                            "valid_until": "2026-09-26T11:00:00Z",
                            "input_snapshot": ["rec_ica_14021009"],
                            "uncertainty": "baseline, not evaluated",
                        },
                    },
                },
            ],
        }
    ],
}


def test_timeline_orders_metar_points_oldest_first():
    newer = make_metar_record(
        record_id="m2", observed_at="2026-09-26T09:00:00Z", temperature_c=26.0
    )
    older = make_metar_record(
        record_id="m1", observed_at="2026-09-26T08:00:00Z", temperature_c=24.0
    )
    now = datetime(2026, 9, 26, 9, 40, tzinfo=UTC)

    result = timeline_from_eye_and_twin(
        metar_records=[newer, older],
        ica_records=[],
        twin_forecast={"stations": []},
        station_id="14021009",
        now=now,
    )

    assert isinstance(result, TimelineResponse)
    temps = next(s for s in result.series if s.variable == "air_temperature")
    assert [p.at.hour for p in temps.points] == [8, 9]
    assert [p.value for p in temps.points] == [24.0, 26.0]


def test_timeline_combines_observed_and_predicted_ica_points_for_one_station():
    ica = make_ica_record(observed_at="2026-09-26T08:00:00Z", index=4, category="desfavorable")
    now = datetime(2026, 9, 26, 9, 40, tzinfo=UTC)

    result = timeline_from_eye_and_twin(
        metar_records=[],
        ica_records=[ica],
        twin_forecast=TWIN_FORECAST_ONE_STATION,
        station_id="14021009",
        now=now,
    )

    aq = next(s for s in result.series if s.variable == "air_quality_index")
    assert aq.entity.id == "14021009"
    assert aq.entity.name == "AVDA. AL-NASIR"
    labels = [p.label for p in aq.points]
    assert labels == ["OBSERVED", "PREDICTED", "PREDICTED"]
    assert aq.points[0].category == "poor"
    assert aq.points[0].due_to == "PM10"
    assert aq.points[1].horizon_h == 1
    assert aq.points[2].horizon_h == 3


def test_timeline_with_no_ica_history_still_returns_the_forecast_points():
    now = datetime(2026, 9, 26, 9, 40, tzinfo=UTC)

    result = timeline_from_eye_and_twin(
        metar_records=[],
        ica_records=[],
        twin_forecast=TWIN_FORECAST_ONE_STATION,
        station_id="14021009",
        now=now,
    )

    aq = next(s for s in result.series if s.variable == "air_quality_index")
    assert [p.label for p in aq.points] == ["PREDICTED", "PREDICTED"]
