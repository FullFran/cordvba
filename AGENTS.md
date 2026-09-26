# AGENTS.md — cordvba

Agent governance for the cordvba monorepo root. Every component owns its own
`AGENTS.md` and rules; this file covers only what is true across all of them.

## Layout

```
apps/eye/    the city data plane — a local OSINT gateway that assembles a
             live model of Córdoba from public sources only, shipped as a
             single Go binary with no mandatory services.
```

Only `apps/eye` exists so far. More components (an API, a web frontend, an
intelligence layer, and a digital-twin worker) will be added by a follow-up
PR, each under `apps/<name>` with its own `AGENTS.md`.

## The one boundary that already applies

Nothing outside `apps/eye/` reads eye's SQLite store or raw cache, and nothing
outside `apps/eye/` imports eye's `internal` packages. Consumers talk to eye
over its HTTP API instead — see [`apps/eye/AGENTS.md`](./apps/eye/AGENTS.md)
and the schema it serves at `/openapi.json`. eye owns its persistence and
internals completely; this boundary is hexagonal, not a suggestion.

## Commits and pull requests

Until a root-level rule set exists, commit and PR conventions for every
component follow
[`apps/eye/.agents/rules/commits.md`](./apps/eye/.agents/rules/commits.md).
