"""api.ica mirrors twin's normalization/category mapping (see
services/twin/src/twin/tests/test_ica.py for the twin-side twin); both
follow packages/contracts/environment/v1/README.md's table.
"""

import pytest

from api.ica import normalize_index, to_category


@pytest.mark.parametrize(("raw_index", "expected"), [(1, 1), (6, 6), (10, 1), (30, 3), (60, 6)])
def test_normalize_index_handles_the_times_ten_partial_encoding(raw_index, expected):
    assert normalize_index(raw_index) == expected


@pytest.mark.parametrize("invalid", [0, 7, 15, 61])
def test_normalize_index_rejects_values_outside_the_known_encoding(invalid):
    with pytest.raises(ValueError):
        normalize_index(invalid)


def test_to_category_matches_the_contract_table():
    result = to_category(4)

    assert result.category == "poor"
    assert result.category_source == "desfavorable"
