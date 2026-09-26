# twin

**Status:** planned, no code yet.

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
