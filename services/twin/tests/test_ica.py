"""ICA index normalization and category mapping, per
packages/contracts/environment/v1/README.md's table. Real fixtures (station
14021006) publish the index multiplied by ten when computed from a subset
of pollutants (`partial: true`); the normalized category must be the same
either way.
"""

import pytest

from twin.ica import normalize_index, to_category


@pytest.mark.parametrize(
    ("raw_index", "expected"),
    [
        (1, 1),
        (6, 6),
        (10, 1),
        (30, 3),
        (60, 6),
    ],
)
def test_normalize_index_handles_the_times_ten_partial_encoding(raw_index, expected):
    assert normalize_index(raw_index) == expected


@pytest.mark.parametrize("invalid", [0, 7, 15, 61, -1])
def test_normalize_index_rejects_values_outside_the_known_encoding(invalid):
    with pytest.raises(ValueError):
        normalize_index(invalid)


@pytest.mark.parametrize(
    ("index", "category", "category_source"),
    [
        (1, "good", "buena"),
        (2, "fair", "razonablemente buena"),
        (3, "moderate", "regular"),
        (4, "poor", "desfavorable"),
        (5, "very_poor", "muy desfavorable"),
        (6, "extremely_poor", "extremadamente desfavorable"),
    ],
)
def test_to_category_matches_the_contract_table(index, category, category_source):
    result = to_category(index)

    assert result.category == category
    assert result.category_source == category_source


def test_to_category_rejects_an_unnormalized_index():
    with pytest.raises(ValueError):
        to_category(30)
