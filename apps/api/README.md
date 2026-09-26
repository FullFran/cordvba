# api

**Status:** v0.1 implemented (issue #101): composes eye and twin into the
public v1 environment endpoints.

## v0.1 endpoints

- `GET /health`, `GET /openapi.json`
- `GET /v1/environment`: now — observed values, derived state and a
  forecast summary from twin, each labelled and carrying provenance or
  model metadata.
- `GET /v1/environment/timeline?hours_back=12&horizons=1,3,6`: observed
  points from eye's history plus predicted points from twin, one ordered
  series per variable.
- `POST /v1/environment/simulate`: validates the scenario (same bounds as
  twin; out of range is a 422), forwards it to twin, returns observed and
  simulated side by side.
- `GET /v1/sources`: publisher, licence and freshness for `metar-cordoba`
  and `miteco-ica` only (the only redistributable v0.1 sources), each with
  an attribution string.

Response models (`apps/api/src/api/schemas.py`) match
`packages/contracts/environment/v1/*.example.json` exactly; tests load
those examples and validate them against the models.

Config: `EYE_BASE_URL`, `EYE_API_TOKEN`, `TWIN_BASE_URL`, `WEB_ORIGIN`
(CORS) — see `api.config`. `WEB_ORIGIN` accepts one origin or a
comma-separated list of origins (e.g.
`https://a.example, https://b.example`); each entry is trimmed and must be
a bare `scheme://host[:port]` with no path, or the process refuses to
start. No response contains eye's token or either internal base URL
(tested).

Run locally: `make api` (root Makefile) or
`docker build -f apps/api/Dockerfile -t cordvba-api .` from the
repository root.

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
