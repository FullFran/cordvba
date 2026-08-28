# Getting started

## Requirements

Go 1.25 or newer. That is the whole list.

`go.mod` pins `toolchain go1.26.6`, which Go downloads on its own the first time
you build. The pin exists because CI resolves its Go version from this file, and
the `go` directive alone selects the `.0` patch — which `govulncheck` reports
with 27 standard-library CVEs. Raise it when the next advisory lands.

No database to install, no Docker, no services to start. `eye` is one static
binary and one SQLite file.

## Build and run

```bash
git clone https://github.com/FullFran/eye.git
cd eye

make build          # → bin/eye
./bin/eye status    # polls every live source and reports the city
./bin/eye help
```

Or run straight from source:

```bash
make run ARGS="news --limit 10"
```

The source registry is compiled into the binary, so `eye` works immediately
after building with nothing else installed. It is resolved in this order:

1. `--registry <path>`, when given
2. `$EYE_CONFIG_DIR/sources.yaml` (default `~/.config/eye/sources.yaml`)
3. `./configs/sources.yaml`, which is what you get inside a checkout
4. the copy compiled into the binary

`eye sources` prints which one was used.

## The commands you will use daily

```bash
make ci-local       # fmt + vet + lint + test + build — must pass before any PR
make test           # go test -race -cover ./...
make cover          # HTML coverage report
make lint           # golangci-lint
make vuln           # govulncheck
make help           # every target
```

`make ci-local` runs the same checks CI does. If it passes locally, CI passes.

## Where things live

Read [the architecture overview](../architecture/overview.md) first — the
package map at the end of it is the fastest way to orient yourself.

The short version: `internal/<domain>/domain` holds types and knows nothing
about the outside world, `application` holds use cases, `infrastructure` holds
everything that talks to a network, a disk or a terminal.

## Running it continuously

`eye daemon` polls every live source on its own interval and persists what it
returns. It does one immediate pass on startup — an operator should have the
full picture within a second, not after a random stagger — then hands over to
the scheduler.

```bash
eye daemon                    # until interrupted
eye daemon --once             # one pass and exit
eye daemon --prune-every 30m  # how often retention is enforced
```

As a user service, if you would rather not supervise it yourself:

```ini
# ~/.config/systemd/user/eye.service
[Unit]
Description=eye — a live model of Cordoba
After=network-online.target

[Service]
ExecStart=%h/.local/bin/eye daemon
Restart=on-failure
RestartSec=30
Environment=EYE_LOG_LEVEL=info

[Install]
WantedBy=default.target
```

```bash
systemctl --user enable --now eye.service
journalctl --user -u eye -f
```

Logs are JSON when stderr is not a terminal, which is what `journalctl` wants.
On a terminal they are plain text.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `EYE_DATA_DIR` | `$XDG_DATA_HOME/eye`, else `~/.local/share/eye` | Store and raw cache location; `--data-dir` overrides it per command |
| `EYE_CONFIG_DIR` | `$XDG_CONFIG_HOME/eye`, else `~/.config/eye` | `sources.yaml`, `rules.yaml` |
| `EYE_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `AEMET_API_KEY` | — | Required by the AEMET provider |
| `FIRMS_MAP_KEY` | — | Required by the NASA FIRMS provider |

API keys are read only in `internal/config`. `os.Getenv` is banned everywhere
else, so that the full set of inputs to the program is knowable from one file.

Copy `.env.example` to `.env` for local development. `.env` is gitignored and
must stay that way.

## Adding a source provider

The order matters, and it is not the order most people reach for.

1. **Register the source first.** Add the entry to `configs/sources.yaml`:
   authority, URL, format, licence, `access`, `automation`, `interval`. If you
   cannot fill in the licence, that is the task — not the adapter.
2. **Record a fixture.** Save a real response under `testdata/<source>/`. Tests
   never hit the network.
3. **Write the failing test.** Fixture in, `[]Record` out.
4. **Implement the adapter** in `internal/provider/infrastructure/<source>/`.
5. **Map provenance properly.** Publisher, source URL, licence, `FetchedAt`,
   `RawHash`. Keep `ObservedAt` and `FetchedAt` distinct. Do not copy a source's
   own confidence score into `Record.Confidence`.
6. **Run `make ci-local`.**

See the [data ethics checklist](../legal/data-ethics.md#adding-a-source-the-checklist)
before step 1.

## Testing

TDD is mandatory: RED → GREEN → REFACTOR. Table-driven tests are the default.
Coverage floor is 80%; the domain packages sit at 100% and should stay there.

```bash
go test -race -cover ./...
```

Never write a test that performs a live HTTP request. Public services are not
our test fixtures, and a test suite that fails because AEMET is having a bad
morning teaches you nothing.

Nor may a test write into the operator's real store. Every command that opens
the store takes `--data-dir`, and the test helpers point it at `t.TempDir()`.
A suite that pollutes `~/.local/share/eye` only reveals itself as surprising
rows in somebody's `eye status`.

Time is injected, not slept through. The scheduler takes a `Clock`, so its
tests exercise hours of behaviour in microseconds.
