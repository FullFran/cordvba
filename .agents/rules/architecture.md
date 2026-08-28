# Architecture Rules — eye

## Package layout

```
cmd/eye/                    main package; builds the command registry and exits
internal/
  cli/                      argument parsing and output rendering
  config/                   configuration; the ONLY place os.Getenv is called
  version/                  build identity injected via -ldflags
  <domain>/
    domain/                 types, ports, domain errors — stdlib imports only
    application/            use cases; depends only on domain interfaces
    infrastructure/         HTTP clients, SQLite, terminal, filesystem
  scheduler/                jitter, backoff, circuit breaker, host concurrency
  fusion/                   spatio-temporal rule engine
  testutil/                 shared test helpers; never imported by production code
configs/                    sources.yaml (the source registry), rules.yaml
testdata/                   recorded fixtures; tests never hit the network
```

Domains: `observation`, `source`, `provider`, `event`.

## Layer rules

| Layer | May import | Must NOT import |
|---|---|---|
| `domain` | stdlib only | any other project package, any driver, `net/http` |
| `application` | `domain` packages | `infrastructure`, `net/http`, storage drivers |
| `infrastructure` | `application`, `domain`, drivers | (no cycles) |
| `cli` | anything under `internal/` | — |
| `cmd/eye` | `internal/cli` | everything else |

`eye serve` is infrastructure. It has no privilege over `eye status` or
`eye daemon`, and nothing in `application` may know it exists.

## Adding a source provider

1. Add the entry to `configs/sources.yaml` — authority, URL, format, licence,
   `access`, `automation`, `interval`. If the licence cannot be filled in, that
   is the task; the adapter comes later.
2. Record a real response into `testdata/<source>/`.
3. Write the failing test: fixture in, `[]Record` out.
4. Implement in `internal/provider/infrastructure/<source>/`.
5. Map `Provenance` fully. Keep `ObservedAt` and `FetchedAt` distinct.
6. `make ci-local`.

An adapter is the **only** place that may know a wire format exists. Never let
DATEX II, GTFS or CKAN shapes past the normalizer.

## Adding a command

1. Implement `cli.Command` with a `Name`, a `Summary` and a `Run`.
2. `Run` parses its own flags with `flag.NewFlagSet` and writes to the `stdout`
   and `stderr` writers it is given — never to `os.Stdout` directly, or the
   command becomes untestable.
3. Register it in `cli.New()`.
4. Test it through `App.Run` with buffers, asserting the exit code.

Exit codes: `0` success, `1` runtime failure, `2` usage error.

## Error handling

- Domain errors are typed values, not strings: `var ErrInvalidRecord = errors.New(...)`.
- Wrap with context: `fmt.Errorf("poll dgt-incidents: %w", err)`.
- Inspect with `errors.Is` / `errors.As`. Never string matching.
- A provider must not retry internally. Backoff belongs to the scheduler.

## Context propagation

- Every function doing I/O takes `context.Context` as its first parameter.
- Never store a context in a struct field.
- Long-running loops check `ctx.Done()`.

## Storage

- All access goes through the port defined in `observation/domain`.
- SQLite in WAL mode, CGO-free driver. `CGO_ENABLED=0` is not optional.
- Records are append-only. eye does not rewrite history in place.
- Parameterised queries only. SQL lives in the infrastructure adapter.

## HTTP clients

- One shared `*http.Client` with an explicit timeout. Never `http.DefaultClient`.
- Send `If-None-Match` / `If-Modified-Since` and honour `304`.
- Honour `Retry-After`.
- Always `defer resp.Body.Close()`, and cap the read with `io.LimitReader`.

## Logging

- `log/slog` only. No third-party logger — see ADR-0004.
- Structured pairs: `slog.Info("poll complete", "source", id, "records", n)`.
- Log at boundaries: poll start and end, state changes, breaker trips. Not per
  record.
- Never log an API key, a full payload, or a personal identifier.
