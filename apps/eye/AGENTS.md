# AGENTS.md — eye

Agent governance for this repository. Read this before making any code change.
Detailed rules live under `.agents/`; this file is the entry point.

## What this project is

A local OSINT gateway for Córdoba: a live model of the city assembled from
public sources only, shipped as a **single Go binary with no mandatory
services**. Not a camera viewer, not a dashboard over a few APIs.

## Architecture at a glance

Hexagonal per domain, adapted from an HTTP service to a CLI.

```
cmd/eye/main.go             composition root — the only main package
internal/<domain>/
  domain/                   types, ports, domain errors — stdlib only
  application/              use cases; depends only on domain interfaces
  infrastructure/           adapters: HTTP clients, SQLite, terminal output
```

Domains: `observation` (Record, Entity — the core), `source` (the registry),
`provider` (adapter ports), `event` (the Córdoba agenda).

`internal/cli` is an infrastructure adapter. So is `eye serve`. **HTTP is never
the centre of the system.**

## Rule index

| File | Concern |
|---|---|
| `.agents/rules/general.md` | Toolchain, PR size, language |
| `.agents/rules/architecture.md` | Layer rules, adapters, errors, context, logging |
| `.agents/rules/commits.md` | Conventional commits, scope enum |
| `.agents/rules/testing.md` | Test runner, TDD order, table-driven, fixtures |
| `.agents/rules/security.md` | Secrets, govulncheck, trivy, CI tiers |
| `.agents/rules/style.md` | gofmt, golangci-lint, naming, file layout |

## Workflow index

| File | When to use |
|---|---|
| `.agents/workflows/add-feature.md` | Adding a domain or a command |
| `.agents/workflows/fix-bug.md` | Reproducing and fixing a bug |
| `.agents/workflows/review-pr.md` | Reviewing an incoming PR |
| `.agents/workflows/release.md` | Tagging and publishing a release |

## Non-negotiables

1. **TDD is mandatory.** RED → GREEN → REFACTOR. No implementation before a
   failing test.
2. **Layer boundaries are strict.** A `domain` package imports the standard
   library and nothing else.
3. **`os.Getenv` is banned** outside `internal/config`.
4. **CGO stays off.** `CGO_ENABLED=0` in the Makefile and in CI. A change that
   requires a C toolchain is a change to [ADR-0003](./docs/adr/0003-sqlite-without-cgo.md).
5. **The dependency budget is three direct dependencies.** Adding a fourth needs
   an ADR that supersedes [ADR-0004](./docs/adr/0004-dependency-budget.md).
6. **No source is fetched unless `configs/sources.yaml` says
   `automation: enabled`.** The gate fails closed. See
   [ADR-0006](./docs/adr/0006-source-registry-gates-automation.md).
7. **Provenance is required, not optional.** Publisher, source URL, licence,
   `FetchedAt`, `RawHash`. `ObservedAt` and `FetchedAt` are never merged.
8. **No test performs a live HTTP request.** Fixtures live in `testdata/`.
9. **The ethical boundary is not negotiable.** No face recognition, no plate
   indexing, no per-person tracking, no probing undocumented endpoints, no video
   archive. See [ADR-0007](./docs/adr/0007-ethical-boundary.md).
10. **`make ci-local` must pass** before any PR is opened.

## Things that look like small decisions and are not

- Copying a source's own `confidence` field into `Record.Confidence`. It is
  product metadata; it belongs in `Payload`.
- Dropping a source's quality caveat during normalization. SAIH states its
  readings are not cross-checked; that must reach the UI.
- Adding a persistence path for camera frames. There isn't one, by design.
- Polling faster than the publisher's declared cadence. It buys rate limits and
  nothing else.
- Calling the JSON endpoint behind a public viewer. That is
  `access: undocumented_backend`, and `Source.Validate()` refuses to schedule it.

## Quick start for agents

```bash
make ci-local      # full local CI — must pass before committing
make test          # go test -race -cover ./...
make lint          # golangci-lint run ./...
make fmt           # gofmt -l -w .
```

## Language

All identifiers, comments, commit messages, documentation and command output
are in **English**. Source names, authority names and place names keep their
original spelling (Córdoba, Diputación, AUCORSA).
