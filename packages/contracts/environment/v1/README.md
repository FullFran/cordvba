# Environment contract, v1

The payloads the cordvba API serves to the web for the v0.1 environment twin (#104). `apps/api` owns these endpoints and its `/openapi.json` is the formal schema; the files here are the canonical **examples** both sides test against:

- `apps/api` tests assert that real responses have exactly these shapes.
- `apps/web` uses them as fixtures, so the page can be built before the API is deployed.

A breaking change to any shape goes in a new `v2/` folder; v1 stays until the web has moved.

| Endpoint | Example |
|---|---|
| `GET /v1/environment` | [`environment.example.json`](./environment.example.json) |
| `GET /v1/environment/timeline?hours_back=12&horizons=1,3,6` | [`timeline.example.json`](./timeline.example.json) |
| `POST /v1/environment/simulate` | request [`simulate.request.example.json`](./simulate.request.example.json), response [`simulate.response.example.json`](./simulate.response.example.json) |
| `GET /v1/sources` | [`sources.example.json`](./sources.example.json) |

## Shared shapes

**Value**: every number or category the page shows.

| Field | Type | Meaning |
|---|---|---|
| `value` | number, string or null | null when the source reported no data |
| `unit` | string | `Cel`, `%`, `m/s`, `deg`, `hPa`, `ica` (index 1–6), `category` |
| `label` | string | `OBSERVED`, `INFERRED`, `PREDICTED` or `SIMULATED` |
| `at` | RFC 3339 | the time the value refers to (observation time, or forecast/scenario valid time) |
| `provenance` | Provenance or null | required when `label` is `OBSERVED` |
| `model` | Model or null | required for `INFERRED`, `PREDICTED` and `SIMULATED` |

**Provenance**: where an observed value came from, straight from eye: `source`, `record_id` (opaque), `publisher`, `licence`, `source_url`, `observed_at`, `fetched_at`, `quality`.

**Model**: how a derived value was produced: `name`, `version`, `reference` (citation or URL), `generated_at`, `valid_from`, `valid_until`, `input_snapshot` (record ids), `uncertainty` (free text; never a made-up confidence number).

## Air-quality categories

MITECO's ICA index, 1–6. `category` is an English slug; `category_source` keeps MITECO's wording.

| Index | `category` | `category_source` |
|---|---|---|
| 1 | `good` | buena |
| 2 | `fair` | razonablemente buena |
| 3 | `moderate` | regular |
| 4 | `poor` | desfavorable |
| 5 | `very_poor` | muy desfavorable |
| 6 | `extremely_poor` | extremadamente desfavorable |

## Scenario bounds

`temperature_delta_c` in [-10, 10], `humidity_delta_pct` in [-50, 50] (percentage points, result clamped to 0–100), `wind_factor` in [0, 2]. Anything outside returns 422.

## Example values

The examples use the METAR reading at Córdoba airport on 2026-09-26 09:00Z (26 °C, 8 kt) with an illustrative dew point of 12 °C. Relative humidity uses the Magnus approximation (41.7 %); apparent temperature uses the Australian Bureau of Meteorology formula (23.7 °C observed; 30.4 °C for +3 °C, +10 points of humidity and half the wind).
