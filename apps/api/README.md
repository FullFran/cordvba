# api

**Status:** planned, no code yet.

## Responsibility

The product backend: BFF, application layer and composition gateway — the
only backend [`apps/web`](../web/README.md) knows. Composes eye, twin and
intelligence into one representation for the frontend (map, timeline, ask,
forecast, simulate), and later owns product logic: sessions, users,
favourites, preferences, saved simulations, rate limiting, application
cache, feature flags, notifications. It also validates any `ui_actions`
that intelligence proposes before they reach web.

## Must not

- Ingest, train, embed, retrieve or plan — that is intelligence's job.
- Reach into eye's, twin's or intelligence's internals or persistence.
- Let the model (intelligence) drive the browser directly; `ui_actions` are
  validated here first.

## May depend on

[`apps/eye`](../eye/README.md), [`services/twin`](../../services/twin/README.md)
and [`services/intelligence`](../../services/intelligence/README.md),
through their public contracts only. See
[`docs/architecture/system-overview.md`](../../docs/architecture/system-overview.md).

## Data

Its own product database/schema (sessions, favourites, preferences). Never
eye's SQLite store, never twin's or intelligence's stores.
