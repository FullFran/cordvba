# ADR-0004: Spend dependencies from a fixed budget

## Status

Accepted

## Context

"Light by design" is a claim that decays silently. A Go project starts with a
clean `go.mod` and, twenty pull requests later, pulls in a CLI framework, a
logging library, an HTTP client wrapper, an assertion library and a YAML
alternative — none of which any single PR could reasonably be blocked over.

Go's standard library already covers most of what `eye` needs: `encoding/xml`
parses DATEX II, `encoding/json` and `archive/zip` cover CKAN and GTFS,
`net/http` does the fetching, `log/slog` does structured logging, `flag` and
`text/tabwriter` do the CLI, and `testing` does the tests.

## Decision

`eye` maintains an explicit dependency budget. The permitted direct
dependencies are:

| Dependency | For | Why the stdlib is not enough |
|---|---|---|
| `modernc.org/sqlite` | the store | No SQL engine in the stdlib. CGO-free — see ADR-0003. |
| `gopkg.in/yaml.v3` | `sources.yaml`, `rules.yaml` | Config is written by humans; JSON has no comments, and the registry is mostly reasoning. |
| `google.golang.org/protobuf` | GTFS-RT | GTFS-RT is protobuf. Hand-rolling it is worse. |

Everything else is standard library. Adding a fourth direct dependency requires
a new ADR that supersedes this one, arguing the case in writing.

Specifically **not** used: a CLI framework (`flag` is enough for a verb-noun
tool), a logging library (`log/slog`), an assertion library (`testing` plus
table-driven tests), an HTTP client wrapper, an ORM.

## Consequences

**Positive:**
- Small binary, fast builds, tiny `govulncheck` surface.
- Upgrades are rare and boring.
- The cost of a dependency becomes visible at the moment it is proposed, which
  is the only moment anyone is willing to argue about it.

**Negative:**
- Some plumbing gets written by hand: flag parsing per subcommand, table
  rendering, retry logic. That code is small, ours, and tested.
- Contributors used to `cobra` or `testify` have a short adjustment.

## Alternatives considered

| Alternative | Why it was rejected |
|---|---|
| No policy, judge per PR | This is exactly how the budget is lost: no individual PR ever looks unreasonable. |
| Zero dependencies | Would mean writing a SQL engine and a protobuf decoder. The budget exists to be spent well, not hoarded. |
