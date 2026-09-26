# web

**Status:** planned, no code yet.

## Responsibility

Makes the city explorable. Main surfaces: Map (traffic, buses, events,
roadworks, weather, air quality, pollen, environment, infrastructure,
predictions, simulations as layers), Timeline (past = eye history, now,
future = twin forecasts/simulations, distinguished visually and not only by
colour, e.g. ● observed, ◌ predicted, ◇ simulated), Ask, Forecast,
Simulate, Explore.

## Must not

- Call `apps/eye`, `services/twin` or `services/intelligence` directly.
- Execute `ui_actions` proposed by intelligence without api having
  validated them first — the model never drives the browser directly.

## May depend on

[`apps/api`](../api/README.md) only. See
[`docs/architecture/system-overview.md`](../../docs/architecture/system-overview.md).

## Data

None of its own; everything comes from api.
