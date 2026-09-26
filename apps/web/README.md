# web

**Status:** v0.1 environment twin page (issue #102). Map, timeline, state and
scenario panels for weather and air quality.

## Responsibility

Makes the city explorable. Main surfaces: Map (traffic, buses, events,
roadworks, weather, air quality, pollen, environment, infrastructure,
predictions, simulations as layers), Timeline (past = eye history, now,
future = twin forecasts/simulations, distinguished visually and not only by
colour, e.g. ● observed, ◌ predicted, ◇ simulated), Ask, Forecast,
Simulate, Explore. v0.1 ships the environment slice only: current state,
+1/+3/+6 h forecast and a thermal what-if scenario.

## Must not

- Call `apps/eye`, `services/twin` or `services/intelligence` directly.
- Execute `ui_actions` proposed by intelligence without api having
  validated them first — the model never drives the browser directly.

## May depend on

[`apps/api`](../api/README.md) only. See
[`docs/architecture/system-overview.md`](../../docs/architecture/system-overview.md).

## Data

None of its own; everything comes from api, following the shapes in
[`packages/contracts/environment/v1`](../../packages/contracts/environment/v1).

## Types

`src/types/environment.ts` is hand-written from
`packages/contracts/environment/v1/README.md`. It moves to generation from
api's `/openapi.json` once api is deployed — do not hand-edit it after that
point; regenerate it instead.

## Running

Requires Node 24 and pnpm.

```bash
pnpm install

# Against a real API at VITE_API_BASE_URL (default /api):
pnpm dev

# Against the contract's fixture JSONs, so the page runs before api exists:
VITE_USE_FIXTURES=1 pnpm dev
```

### Environment variables

| Variable | Default | Meaning |
|---|---|---|
| `VITE_API_BASE_URL` | `/api` | Base URL the page calls; never eye or the twin directly. |
| `VITE_USE_FIXTURES` | unset | `1` serves `packages/contracts/environment/v1`'s example JSONs instead of calling the API. |

## Testing

```bash
pnpm test        # vitest run, once
pnpm test:watch  # vitest, watch mode
```

Strict TDD: every test was written and run to a failing (RED) state before
its implementation. No test performs a live HTTP request — `fetch` is
mocked, and fixture-mode tests import the contract JSONs directly.

## Type-checking and building

```bash
pnpm typecheck
pnpm build                       # against a real API
VITE_USE_FIXTURES=1 pnpm build   # a static, API-free build using fixtures
```

## Docker

Build from the **monorepo root** (the image needs the contract fixtures
next to it):

```bash
docker build -f apps/web/Dockerfile -t cordvba-web .
docker run -p 8080:80 cordvba-web
```

Multi-stage: `pnpm build` in a `node:24-slim` stage, then the static
`dist/` is served by `nginx:1.27-alpine` (`nginx.conf`) with SPA fallback
(`try_files ... /index.html`) and no secrets baked in beyond the
build-time `VITE_API_BASE_URL`.
