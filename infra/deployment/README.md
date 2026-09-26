# deployment

**Status:** planned, no code yet.

## Responsibility

How components run outside local development: deployment manifests,
release wiring per component (eye keeps its standalone binary; container
images per component), environment configuration.

## Must not

- Contain application or domain code.
- Duplicate a component's own build logic — it orchestrates and references
  it, following the same principle as the root
  [`Makefile`](../../Makefile).

## May depend on

Each component's build artefacts. See
[`docs/architecture/system-overview.md`](../../docs/architecture/system-overview.md).

## Data

None.
