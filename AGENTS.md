# AGENTS.md — cordvba

Agent governance for the cordvba monorepo root. Every component owns its own
`AGENTS.md` and rules; this file covers only what is true across all of them.
See [`docs/architecture/system-overview.md`](./docs/architecture/system-overview.md)
for the full component table, dependency graph and request flows, and
[`docs/product/vision.md`](./docs/product/vision.md) for what CORDVBA is.

## Layout criterion

Where does a new thing go?

| Kind | Meaning |
|---|---|
| `apps/` | Components with their own entry point for people or operators (eye, api, web). |
| `services/` | Internal computation services reachable only by other components, never by the browser (intelligence, twin). |
| `packages/` | Libraries and contracts, never deployed alone. |
| `infra/` | How things run: compose, deployment, observability. |
| `experiments/` | Notebooks and prototypes; never imported by production code, never deployed. |

Only `apps/eye` has code today. api, web, intelligence and twin are
documentation-only placeholders — see each component's own `README.md` and
`AGENTS.md` once it starts.

## Boundary rules

1. Nothing outside `apps/eye/` reads eye's SQLite store or raw cache.
2. Nothing outside `apps/eye/` imports eye's `internal` Go packages.
   Consumers talk to eye over its HTTP API — see
   [`apps/eye/AGENTS.md`](./apps/eye/AGENTS.md) and the schema it serves at
   `/openapi.json`.
3. eye depends on nobody else in this repository.
4. twin and intelligence never write to eye; they only read its public API.
5. api composes eye, twin and intelligence through their public contracts,
   never their internals or persistence.
6. web talks only to api.

Every component owns its persistence and internals completely; these
boundaries are hexagonal, not a suggestion. `infra/ci/check-boundaries.sh`
enforces rules 1–3 mechanically.

Every component owns its own `AGENTS.md`; add one when a component's code
starts.

## Commits and pull requests

Until a root-level rule set exists, commit and PR conventions for every
component follow
[`apps/eye/.agents/rules/commits.md`](./apps/eye/.agents/rules/commits.md).
