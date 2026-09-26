"""MITECO's ICA index (1-6) and its category mapping. See
packages/contracts/environment/v1/README.md, "Air-quality categories".

MITECO publishes an index computed from a subset of pollutants as the same
category multiplied by ten (10, 20, ... 60) rather than dropping it; eye
keeps that raw value and flags it `partial: true`. The rank is the same
either way, so twin normalizes it back to 1-6 before mapping to a category.
"""

from __future__ import annotations

from dataclasses import dataclass

#: index -> (English slug, MITECO's own Spanish wording).
_CATEGORY_TABLE: dict[int, tuple[str, str]] = {
    1: ("good", "buena"),
    2: ("fair", "razonablemente buena"),
    3: ("moderate", "regular"),
    4: ("poor", "desfavorable"),
    5: ("very_poor", "muy desfavorable"),
    6: ("extremely_poor", "extremadamente desfavorable"),
}

_PARTIAL_MULTIPLE = 10


@dataclass(frozen=True)
class Category:
    category: str
    category_source: str


def normalize_index(raw_index: int) -> int:
    """Map a raw ICA index (1-6, or 10-60 for the partial encoding) back to
    its 1-6 rank. Raises ValueError for anything eye's own dictionary does
    not explain (including 0, which means "no data" and must be filtered
    out by the caller before this is reached).
    """
    if 1 <= raw_index <= 6:
        return raw_index
    if raw_index % _PARTIAL_MULTIPLE == 0:
        candidate = raw_index // _PARTIAL_MULTIPLE
        if 1 <= candidate <= 6:
            return candidate
    raise ValueError(f"ICA index {raw_index} is outside the known 1-6 (or partial) encoding")


def to_category(normalized_index: int) -> Category:
    """Map an already-normalized 1-6 index to its category slug and MITECO's
    own wording.
    """
    try:
        category, category_source = _CATEGORY_TABLE[normalized_index]
    except KeyError as exc:
        raise ValueError(
            f"{normalized_index} is not a normalized ICA index (expected 1-6)"
        ) from exc
    return Category(category=category, category_source=category_source)
