# System overview — cordvba

See [`docs/product/vision.md`](../product/vision.md) for what CORDVBA is and
the epistemic labels referenced below.

## Components

| Component | Path | Language | Responsibility | Persistence |
|---|---|---|---|---|
| eye | `apps/eye` | Go | Observe: acquire, normalise and keep history/provenance for public Córdoba data. | SQLite + raw cache (private to eye). |
| twin | `services/twin` | Python | Model: derived state, forecasts, counterfactual simulations. | Its own store for model metadata, training runs, predictions and simulation results. |
| intelligence | `services/intelligence` | Python | Understand: retrieval, RAG, tool calling, query planning, agent orchestration, answer synthesis, citations, evaluation. | Likely PostgreSQL + pgvector (not decided; not introduced by this PR). |
| api | `apps/api` | Not decided | Compose: BFF, application layer, composition gateway; the only backend web talks to. | Shares a PostgreSQL server with intelligence/twin only via separate databases/schemas, never shared tables. |
| web | `apps/web` | TypeScript (likely) | Explore: map, timeline, ask, forecast, simulate. | None of its own; reads through api. |

## Dependency graph

No reverse dependencies. Each arrow is "depends on":

```
web ---> api ---> eye
           |  \--> twin
           |  \--> intelligence ---> eye
           \-----> twin              \-> twin
```

Read as: `web` depends only on `api`. `api` depends on `eye`, `twin` and
`intelligence`, but never on their internals. `intelligence` depends on
`eye` and `twin`. `twin` depends on `eye`. `eye` depends on nobody.

## Boundary rules

1. Nothing outside `apps/eye/` reads eye's SQLite store or raw cache; only
   eye touches its own persistence.
2. Nothing outside `apps/eye/` imports eye's `internal` Go packages, or the
   import path `github.com/FullFran/cordvba/apps/eye/internal`. Consumers
   talk to eye only over its public HTTP API (`/openapi.json`).
3. `eye` depends on nobody else in this repository: no import of
   `services/`, `packages/` or `apps/api`/`apps/web` code.
4. `twin` and `intelligence` never write to eye; they are read-only
   consumers of eye's public API.
5. `api` composes `eye`, `twin` and `intelligence` through their public
   contracts only; it never reaches into another component's internals or
   persistence.
6. `web` talks only to `api`. It never calls `eye`, `twin` or
   `intelligence` directly, and intelligence never drives the browser
   directly — see the `ui_actions` flow below.

## Layout criterion

Where does a new thing go?

- **`apps/`** — components with their own entry point for people or
  operators. eye is also a standalone product (CLI/TUI/console); api is the
  product's public backend; web is the UI.
- **`services/`** — internal computation services reachable only by other
  components, never by the browser.
- **`packages/`** — libraries and contracts, never deployed alone.
- **`infra/`** — how things run: compose, deployment, observability.
- **`experiments/`** — notebooks and prototypes. Never imported by
  production code, never deployed.

## Persistence ownership

Each component accesses only its own persistence. eye's SQLite store and
raw cache are private to eye. A shared physical PostgreSQL server is
acceptable for intelligence, twin and api, but sharing a server never means
sharing tables: each gets its own database or schema (e.g. `cordvba_app`,
`intelligence`, `twin`).

## Request flows

### A natural-language question

```
web --Ask--> api --POST /ask--> intelligence
                                   |-- plans, then calls eye and/or twin
                                   |-- eye:  OBSERVED / PUBLISHED facts
                                   |-- twin: PREDICTED / SIMULATED outputs
                                   `-- synthesises an answer + citations
                                       + epistemic labels + ui_actions
             api validates ui_actions (e.g. map.layer.enable) before
             the response reaches web; the model never drives the
             browser directly.
```

### A simulation request

```
web --Simulate--> api --POST /simulate--> twin
                                             |-- runs the counterfactual
                                             |-- labels the result SIMULATED
                                             `-- returns model, model_version,
                                                 generated_at, valid_from,
                                                 valid_until, input_snapshot,
                                                 uncertainty
             api returns twin's result to web unmodified in substance;
             web renders it at the Model or Experience visualisation
             level, never as observed Data.
```
