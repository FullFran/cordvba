# cordvba

A monorepo for a live model of Córdoba: a city data plane, and the consumers
built on top of it.

## Components

- [`apps/eye`](./apps/eye/README.md) — the city data plane. A local OSINT
  gateway that assembles a live model of Córdoba from public sources only,
  shipped as a single Go binary with no mandatory services.

More components (an API, a web frontend, an intelligence layer, and a
digital-twin worker) will land in a follow-up PR.

## Building and testing

```bash
make eye     # build eye (apps/eye)
make test    # run every component's tests
make ci      # run the full local CI pipeline for every component
```

Each component also builds, tests and runs standalone from its own directory
— see its own README for the details.
