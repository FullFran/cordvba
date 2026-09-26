# twin

**Status:** v0.1 implemented (issue #100): environment state, a
persistence air-quality forecast, and a thermal scenario. The other
capability families (traffic, pollen, hydrology, ...) remain planned.

## v0.1 endpoints

- `GET /health`
- `GET /state/environment`: latest METAR reading plus derived relative
  humidity and apparent temperature; latest ICA index per Córdoba-city
  station (`14021*`, `index_reported: true` only) plus the worst-station
  city state.
- `GET /forecast/air-quality?horizons=1,3,6`: a persistence baseline per
  city station (`model.name = "persistence"`,
  `uncertainty = "baseline, not evaluated"`, never a confidence number).
- `POST /simulate/environment`: bounded deltas
  (`temperature_delta_c` in [-10,10], `humidity_delta_pct` in [-50,50],
  `wind_factor` in [0,2]; out of bounds is a 422) applied to the observed
  state; the response shows observed and simulated side by side.

Config: `EYE_BASE_URL`, `EYE_API_TOKEN` (see `twin.config`).
Run locally: `make twin` (root Makefile) or
`docker build -f services/twin/Dockerfile -t cordvba-twin .` from the
repository root.

## Responsibility

Models what the city may do next, across three capability families:
derived state (thermal comfort, air-quality state, traffic/mobility
pressure), forecast (air quality +1h/+6h/+24h, pollen, traffic, hydrology,
bus punctuality, urban heat), and simulation (counterfactuals — a road
closure, +20% traffic, a 42 °C day, a 30k-person event). Every output
states `model`, `model_version`, `generated_at`, `valid_from`,
`valid_until`, `input_snapshot` and `uncertainty`, and is labelled
`PREDICTED` or `SIMULATED` — see
[`docs/product/vision.md`](../../docs/product/vision.md).

## Must not

- Write to eye, ever.
- Present a simulation as a forecast, or a forecast as an observation.
- Be reachable by web or api's users directly outside api's composition.

## May depend on

[`apps/eye`](../../apps/eye/README.md), through its public API only. See
[`docs/architecture/system-overview.md`](../../docs/architecture/system-overview.md).

## Data

Its own store for model metadata, training runs, predictions and
simulation results. Never eye's SQLite store or raw cache. Forecasting
evaluation stays inside this service; shared AI evaluation lives in
[`packages/evals`](../../packages/evals/README.md).
