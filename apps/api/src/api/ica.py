"""MITECO's ICA index (1-6) and its category mapping, for api's own
timeline endpoint (it reads eye's raw ICA history directly; twin's HTTP
contract only covers current state and forecast, not full history). This
mirrors services/twin/src/twin/ica.py: both independently follow
packages/contracts/environment/v1/README.md, "Air-quality categories" —
the shared source of truth, not each other.
"""

from __future__ import annotations

from dataclasses import dataclass

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
    if 1 <= raw_index <= 6:
        return raw_index
    if raw_index % _PARTIAL_MULTIPLE == 0:
        candidate = raw_index // _PARTIAL_MULTIPLE
        if 1 <= candidate <= 6:
            return candidate
    raise ValueError(f"ICA index {raw_index} is outside the known 1-6 (or partial) encoding")


def to_category(normalized_index: int) -> Category:
    try:
        category, category_source = _CATEGORY_TABLE[normalized_index]
    except KeyError as exc:
        raise ValueError(
            f"{normalized_index} is not a normalized ICA index (expected 1-6)"
        ) from exc
    return Category(category=category, category_source=category_source)
