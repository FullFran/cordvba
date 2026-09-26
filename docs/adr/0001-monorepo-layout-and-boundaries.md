# ADR-0001: Monorepo layout and component boundaries

## Status

Accepted

## Context

eye owned the entire repository root before this change: its module path,
CI, release-please and Dockerfile all lived at the top level. Issue #72
adds four more components — api, web, intelligence and twin — that need to
start development in parallel, without waiting on each other and without a
structural reorganisation happening under them mid-flight.

PR 1 (#97) moved eye into `apps/eye` as a pure rename, preserving its
history and standalone build. This ADR records the layout and the
boundaries the remaining components must follow from their first commit,
so four teams of agents can build independently without producing
conflicting assumptions about who owns what.

## Decision

The repository is a monorepo with four top-level kinds of directory —
`apps/`, `services/`, `packages/`, `infra/` (plus `experiments/` for
prototypes) — under the layout criterion recorded in
[`docs/architecture/system-overview.md`](../architecture/system-overview.md).
Six boundary rules, also recorded there, govern how components may depend
on each other: no reverse dependencies, no component reaches into another
component's persistence or internals, and web talks only to api. Each
component owns its own `AGENTS.md`; root `AGENTS.md` covers only what is
true across all of them.

## Consequences

**Positive:**
- api, web, intelligence and twin can be built by separate teams of agents
  without a later restructuring, because the layout and boundaries are
  decided before any of their code exists.
- The layout criterion gives a direct answer to "where does a new thing
  go?", reducing bikeshedding as new components are added.
- eye's existing boundary (no one touches its store or internals) is
  restated as one instance of a repository-wide rule, not a special case.

**Negative:**
- Components other than eye start as documentation-only directories with
  no code, which can look like unfinished scaffolding until their teams
  begin work.
- A rule enforced only by review is easy to violate by accident; this is
  mitigated by the automated boundary check added in the same PR
  (`infra/ci/check-boundaries.sh`), though it currently only covers eye's
  side of the boundary.

## Alternatives considered

| Alternative | Why it was rejected |
|---|---|
| Separate repositories per component | Shared contracts (`packages/contracts`) and coordinated releases would require extra tooling; a monorepo keeps atomic cross-component changes possible without it. |
| No documented layout criterion, decide ad hoc per component | Four teams working in parallel would each answer "where does this go?" differently, producing exactly the inconsistency this ADR exists to prevent. |
| Enforce boundaries by code review only, no automated check | Reviewer attention does not scale with four parallel teams; a mechanical check catches accidental violations review would miss. |
