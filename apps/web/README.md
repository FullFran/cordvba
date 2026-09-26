# web

**Status:** v0.1 environment twin page (issue #102), redesigned as a dark
3D city twin with a bilingual ES/EN switch (issues #119, #124). Map,
timeline, state and scenario panels for weather and air quality. See
[`.agents/DESIGN.md`](./.agents/DESIGN.md) for the design system (tokens,
components, motion, accessibility baselines, decision log) — it is
binding.

## Responsibility

Makes the city explorable. Main surfaces: Map (traffic, buses, events,
roadworks, weather, air quality, pollen, environment, infrastructure,
predictions, simulations as layers), Timeline (past = eye history, now,
future = twin forecasts/simulations, distinguished visually and not only by
colour, e.g. ● observed, ◌ predicted, ◇ simulated), Ask, Forecast,
Simulate, Explore. v0.1 ships the environment slice only: current state,
+1/+3/+6 h forecast and a thermal what-if scenario.

## Design (issue #119)

- **Map:** MapLibre GL JS over [OpenFreeMap](https://openfreemap.org)'s
  `liberty` vector-tile style (no API key), recoloured dark at runtime by
  `src/map/darkStyle.ts` (generic lightness inversion + curated overrides
  for background/water/buildings/labels — see `MapPalette` and
  `buildDarkStyle`). Pitched over the historic centre so the real 3D
  building extrusions (`render_height`/`render_min_height`, present for
  ~42% of Córdoba's OSM buildings) read as a skyline; the rest use a
  default height and are deliberately muted in the colour ramp. Required
  attribution ("OpenFreeMap © OpenMapTiles Data from OpenStreetMap") is in
  the footer. Leaflet was removed.
- **Beacons, not a surface:** the three city air-quality stations and the
  airport weather station are individual beacons (`src/map/beacons.ts`) —
  glow + rings, a shape per ICA category, and the value/category as text.
  There is deliberately no interpolated air-quality surface: three stations
  cannot support one.
- **Illustrative wind:** an animated canvas particle field
  (`WindParticles.tsx`, math in `src/map/wind.ts`) driven by the single
  METAR reading. Always labelled "Illustrative wind: one station, not
  spatially varied" while on, toggleable, and frozen to one static frame
  under `prefers-reduced-motion`.
- **Timeline textures:** past/now/future points carry a texture
  (`src/lib/timeline.ts`'s `textureForLabel`: solid/outline/dotted/hatched)
  in addition to their symbol, so observed/inferred/predicted/simulated
  are never colour-only.
- **Dark, always:** the page has exactly one appearance
  (`color-scheme: dark`, explicit opaque backgrounds throughout) and never
  inherits the visitor's browser/OS theme. Verified with
  `scripts/check-color-scheme-isolation.mjs` (see below).
- **Resilient network states:** `src/hooks/useEnvironmentTwin.ts` fetches
  `environment`/`timeline`/`sources` independently, so one endpoint being
  down never blanks data that arrived fine. Loading shows a skeleton, not a
  blank page; a failure shows a styled panel with a Retry button and
  retries automatically with backoff (2s/4s/8s) before waiting for a
  manual retry.

## Language (issue #124)

Spanish by default, English on demand — the same `ff_locale` localStorage
key and ES|EN segmented-control pattern as the host site
(`fullfran.com`), so a language chosen on one site carries into the other.
`src/i18n/` holds the `Locale` type, the typed `en`/`es` dictionaries (every
visible string; `dictionary.test.ts` fails if either locale is missing a
key), and `LocaleContext`/`useLocale()`. Numbers, wind-direction cardinals
and relative ages format through `Intl` per locale (`es-ES` decimal comma,
`en-GB`); air-quality categories use MITECO's own Spanish wording in
Spanish. The API contract is unchanged — only display text is translated.

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
| `VITE_BASE_PATH` | `/` | Vite's `base` (`vite.config.ts`, via the pure `resolveBase` in `src/lib/base.ts`). Set it when the built assets are served from a sub-path, e.g. `/cordvba/` for the GitHub Pages deploy below. |

## Testing

```bash
pnpm test        # vitest run, once
pnpm test:watch  # vitest, watch mode
```

Strict TDD: every test was written and run to a failing (RED) state before
its implementation. No test performs a live HTTP request — `fetch` is
mocked, and fixture-mode tests import the contract JSONs directly.

## Manual design/QA checks

These are not part of `pnpm test` (they need a real browser, so they are
not wired into the vitest+jsdom unit suite) but are checked into the repo
as reviewable scripts:

```bash
# 1. Build and serve the fixture-mode build:
VITE_USE_FIXTURES=1 pnpm build && pnpm preview

# 2. With any Playwright install (does not need to be a project dependency —
#    require() resolves it via NODE_PATH):
NODE_PATH=<path-to-a-playwright-install>/node_modules \
  node scripts/check-color-scheme-isolation.mjs http://localhost:4173/
```

`check-color-scheme-isolation.mjs` asserts the page's background is opaque
and pixel-identical whether the browser reports `colorScheme: 'dark'` or
`'light'` — the isolation this app must have from the visitor's own
browser/OS theme (see `.agents/DESIGN.md` §6).

## Type-checking and building

```bash
pnpm typecheck
pnpm build                       # against a real API
VITE_USE_FIXTURES=1 pnpm build   # a static, API-free build using fixtures
```

### Building for a sub-path

Set `VITE_BASE_PATH` to build assets that resolve correctly when the page
is served from a sub-path rather than the domain root, e.g. behind
`https://www.fullfran.com/cordvba/`:

```bash
VITE_BASE_PATH=/cordvba/ pnpm build
```

Every script, stylesheet and asset URL emitted into `dist/index.html` and
the built JS is then rooted at `/cordvba/`. The API base URL is unrelated
and stays absolute (`VITE_API_BASE_URL`), since Pages only serves static
files and the real API runs elsewhere (see the Pages workflow below).

## GitHub Pages deploy

`.github/workflows/pages.yml` builds and deploys the page to GitHub Pages
on every push to `main` that touches `apps/web/**`, `packages/contracts/**`
or the workflow file itself, and on manual dispatch. It builds with
`VITE_BASE_PATH: /cordvba/` and `VITE_API_BASE_URL` set from the repository
variable `CORDVBA_PUBLIC_API_BASE_URL` (Settings → Secrets and variables →
Actions → Variables), so no host name is committed. Set that variable to
the deployed API's public base URL (the Funnel origin) before the workflow
can produce a working deploy; the job fails with a clear error if it is
empty. Pages must be enabled for the repository with the "GitHub Actions"
source.

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
