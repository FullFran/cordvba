# cordvba

CORDVBA is an experimental, personal, non-commercial computational model of
the city of Córdoba (Spain). Not a chatbot: the city is the interface. It
integrates observable reality, documentary knowledge, current city state,
prediction, simulation, natural-language interaction, geospatial and
temporal exploration, and clearly-labelled illustrative renderings. See
[`docs/product/vision.md`](./docs/product/vision.md) for the full picture
and [`docs/architecture/system-overview.md`](./docs/architecture/system-overview.md)
for how the components fit together.

## Components

| Component | Status | Responsibility |
|---|---|---|
| [`apps/eye`](./apps/eye/README.md) | Active | The city data plane: a local OSINT gateway that assembles a live model of Córdoba from public sources only, shipped as a single Go binary with no mandatory services. |
| [`apps/api`](./apps/api/README.md) | Planned | The product backend — the only backend web talks to. |
| [`apps/web`](./apps/web/README.md) | Planned | Makes the city explorable: map, timeline, ask, forecast, simulate. |
| [`services/intelligence`](./services/intelligence/README.md) | Planned | Understands questions and builds answers from eye and twin. |
| [`services/twin`](./services/twin/README.md) | Planned | Models what the city may do next: derived state, forecasts, simulations. |

## Building and testing

```bash
make eye     # build eye (apps/eye)
make test    # run every component's tests
make ci      # run the full local CI pipeline for every component
```

Each component also builds, tests and runs standalone from its own directory
— see its own README for the details.

## Licence

The code in this repository is released under the [MIT License](./LICENSE).

Data that eye collects keeps the licence of the source that published it (for
example MITECO's air-quality index under CC BY 4.0, NOAA's METAR reports in the
public domain). Each source's licence and attribution requirements are recorded
in [`apps/eye/configs/sources.yaml`](./apps/eye/configs/sources.yaml); anything
that redistributes eye's data must honour them.
