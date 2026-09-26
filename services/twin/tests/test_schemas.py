"""The Value/Model/Provenance shapes twin serializes must match
packages/contracts/environment/v1/README.md field-for-field, since api
composes its own contract responses partly by forwarding these.
"""

from twin.schemas import IcaValue, Model, Value, ValueProvenance


def test_observed_value_serializes_with_provenance_and_no_model():
    value = Value(
        value=26.0,
        unit="Cel",
        label="OBSERVED",
        at="2026-09-26T09:00:00Z",
        provenance=ValueProvenance(
            source="metar-cordoba",
            record_id="rec_metar_0900",
            publisher="NOAA Aviation Weather Center",
            licence="us-government-public-domain",
            source_url="https://aviationweather.gov/api/data/metar",
            observed_at="2026-09-26T09:00:00Z",
            fetched_at="2026-09-26T09:07:12Z",
            quality="preliminary",
        ),
        model=None,
    )

    dumped = value.model_dump(mode="json")

    assert dumped["label"] == "OBSERVED"
    assert dumped["provenance"]["publisher"] == "NOAA Aviation Weather Center"
    assert dumped["provenance"]["licence"] == "us-government-public-domain"
    assert dumped["model"] is None


def test_inferred_value_serializes_with_model_and_no_provenance():
    value = Value(
        value=41.7,
        unit="%",
        label="INFERRED",
        at="2026-09-26T09:00:00Z",
        provenance=None,
        model=Model(
            name="magnus-relative-humidity",
            version="1",
            reference="Alduchov & Eskridge (1996), Magnus approximation",
            generated_at="2026-09-26T09:40:00Z",
            valid_from="2026-09-26T09:00:00Z",
            valid_until="2026-09-26T09:00:00Z",
            input_snapshot=["rec_metar_0900"],
            uncertainty="derived from reported temperature and dew point",
        ),
    )

    dumped = value.model_dump(mode="json")

    assert dumped["provenance"] is None
    assert dumped["model"]["name"] == "magnus-relative-humidity"
    assert dumped["model"]["uncertainty"] == "derived from reported temperature and dew point"


def test_ica_value_adds_category_fields_alongside_the_base_value_shape():
    value = IcaValue(
        value=4,
        unit="ica",
        label="OBSERVED",
        at="2026-09-26T08:00:00Z",
        category="poor",
        category_source="desfavorable",
        due_to="NO2",
        provenance=ValueProvenance(
            source="miteco-ica",
            record_id="rec_ica_14021009",
            publisher="MITECO",
            licence="CC-BY-4.0",
            source_url="https://ica.miteco.es/datos/ica-ultima-hora.csv",
            observed_at="2026-09-26T08:00:00Z",
            fetched_at="2026-09-26T09:35:40Z",
            quality="official",
        ),
        model=None,
    )

    dumped = value.model_dump(mode="json")

    assert dumped["category"] == "poor"
    assert dumped["category_source"] == "desfavorable"
    assert dumped["due_to"] == "NO2"
    assert dumped["unit"] == "ica"


def test_value_accepts_a_null_value_for_a_station_with_no_data():
    value = Value(value=None, unit="ica", label="OBSERVED", at="2026-09-26T08:00:00Z")

    assert value.model_dump(mode="json")["value"] is None
