# ADR-0010: Give eye three surfaces over one model, and buy none of them

## Status

Accepted

## Context

`eye` had one way in: the terminal, one question at a time. That is the right
default and it stays. But three questions kept arriving that a sequence of
subcommands answers badly.

*Where is all of this?* — a list of records with coordinates is not a picture of
a city. *What is the state of everything right now?* — thirty-nine sources whose
health is only visible by running a command that polls them all. *Can something
else read this?* — `eye serve` existed but bound to loopback, authenticated
nobody, and served four endpoints, which is a demo rather than a deployment.

Each of these has an obvious purchase attached: a TUI framework, a charting
library, a front-end toolchain, a web framework. Taken one at a time none looks
unreasonable, which is exactly the failure mode [ADR-0004](./0004-dependency-budget.md)
was written to prevent. And a surface is not a small thing to add: three of them
can easily become three models of Córdoba that disagree.

## Decision

`eye` grows three surfaces — a TUI (`eye tui`), a deployable HTTP API
(`eye serve`), and a web application embedded in that binary — and all three
read the **same** domain model through the same ports. The API is the only one
that crosses a process boundary; the TUI and the web UI are adapters like the
CLI, and neither holds logic the others do not.

None of them costs a dependency. The `go.mod` direct-dependency list is
unchanged by this ADR:

| Surface | What it would normally buy | What it uses instead |
|---|---|---|
| TUI | Bubbletea, tcell, termbox | ANSI escapes and `stty` on `/dev/tty`, on top of the existing `internal/render` |
| TUI map | a plotting library | a Unicode braille canvas (2×4 dots per cell) with a Web Mercator projection |
| Web UI | React, a bundler, `node_modules` | hand-written ES2020 and CSS, served by `go:embed` |
| Web charts | Chart.js, D3 | SVG and CSS written for the four charts that exist |
| Web map | — | Leaflet from a pinned CDN, loaded by the browser, never vendored into the binary |
| API | chi, gin, gorilla | `net/http`'s method-and-pattern `ServeMux`, which has done this since Go 1.22 |

Leaflet is the one thing that is not ours, and it is deliberately the browser's
dependency rather than eye's: it is fetched by the page, it is pinned by
version, and a binary built from this repo still contains no third-party
front-end code. An operator with no internet gets a map with no tiles and every
other surface intact.

The API becomes deployable rather than demonstrable: a bearer token
(`EYE_API_TOKEN`), a refusal to bind publicly without one, per-endpoint
redistribution gating, an OpenAPI document generated from the route table, and
container and systemd units in `deploy/`.

## Consequences

**Positive:**
- One model, three views. A field that appears in the TUI appears in the API
  because both read `observation.Record`, not because someone remembered.
- The binary stays a single artefact with no runtime assets to lose.
- The map, the dashboards and the boards are testable: every draw function takes
  an `io.Writer` and a size, and every handler is exercised over `httptest`.
- The cost of the next surface is visible before it is paid.

**Negative:**
- Terminal input is ours to maintain: raw mode, CSI decoding and SIGWINCH are
  written by hand, and a terminal without `stty` degrades to a non-interactive
  refresh loop.
- The web UI has no type checking and no bundler. It stays small on purpose,
  and it is reviewed as the hand-written code it is.
- Braille rendering needs a font that has the block. Most terminals do; some
  will show boxes.

**Neutral, and stated so it is not discovered later:**
- Publishing an API does not widen what eye may redistribute. Sources marked
  `undocumented_personal` are still filtered out of every collection endpoint,
  and the one live passthrough that reads such a source refuses unless the
  operator has opted in *and* the deployment is authenticated. Camera images are
  never served, by any surface. See [ADR-0007](./0007-ethical-boundary.md) and
  [ADR-0008](./0008-undocumented-personal-sources.md).

## Alternatives considered

| Alternative | Why it was rejected |
|---|---|
| Buy a TUI framework and raise the dependency budget | The budget is the mechanism, not the number. Raising it for the first surface that wants something is how it stops meaning anything. |
| Ship the web UI as a separate repo and a separate deployment | Two artefacts, two versions, and a UI that can disagree with the binary it points at. The whole value of `eye` is that a figure traces back to a source. |
| Serve the web UI from disk rather than `go:embed` | Adds a path to configure, a directory to lose, and a way for the UI and the binary to drift apart. |
| Vendor Leaflet into the binary | Roughly 150 KB of third-party JavaScript we would then be responsible for reviewing and patching, to solve a problem — offline tiles — that vendoring does not actually solve. |
| Skip the TUI and let the web UI be the visual surface | The web UI needs a browser and a server. The terminal is where this tool is actually used, and a map you can open over SSH is worth writing. |
