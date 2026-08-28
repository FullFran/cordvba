# ADR-0005: Separate Entity from Record, and require provenance on both

## Status

Accepted

## Context

The obvious first data model is one table of "things eye saw". It survives
about two sources. An aircraft and the position of that aircraft at 12:01:03
are not the same kind of fact: one is an inventory entry that persists, the
other is a measurement that expires. A camera and a reading from near that
camera differ the same way.

There is a second trap in the timestamps. If a record carries a single `time`
field, nobody can later tell whether a reading is thirteen minutes old because
AEMET published it thirteen minutes ago or because our scheduler was late. Both
answers matter and they have different fixes.

## Decision

Two types, both in `observation/domain`:

- **`Entity`** — something persistent: a camera, a gauge, a route, a venue.
  Carries `FirstSeen` / `LastSeen`, which describe eye's observation window, not
  the lifetime of the real thing.
- **`Record`** — one observation or change. Carries `ObservedAt` (when the
  source says it happened) and `FetchedAt` (when eye retrieved it), always as
  separate fields. Their difference is the source latency.

Both carry a mandatory `Provenance`: publisher, source URL, licence,
`FetchedAt`, and the SHA-256 of the exact raw payload the observation was
normalized from. `Provenance.Validate()` rejects an incomplete one, so an
observation that cannot be traced back to a public source cannot be stored.

An undeclared licence is recorded as `"unspecified"`, never as an empty string
and never guessed. Records also carry `Quality` — `official`, `validated`,
`preliminary` or `inferred` — which preserves the publisher's own claim about
its data. When SAIH states that its automatic readings are not cross-checked,
that warning reaches the UI instead of dissolving during normalization.

`Record.Confidence` is **eye's** confidence, never the source's. NASA FIRMS
publishes its own `confidence` field; that is detection-product metadata and
lives in `Payload`.

## Consequences

**Positive:**
- Inventory and time series get the retention policies they each need: camera
  positions kept indefinitely, ADS-B positions expired in 24–72 hours.
- `source_latency = FetchedAt - ObservedAt` becomes a first-class metric, so eye
  cannot advertise stale data as live.
- Any alert can be replayed from the bytes it came from.
- Licence questions are answerable per record, which is what makes the project
  publishable.

**Negative:**
- Adapters must do entity resolution rather than emitting a flat list.
- Two tables, two lifecycles, more code.

## Alternatives considered

| Alternative | Why it was rejected |
|---|---|
| One `Record` type for everything | Produces a table that is neither an inventory nor a time series, and forces one retention policy onto both. |
| Provenance as an optional side table | Optional provenance is absent provenance. Making it a required struct is what keeps the guarantee true. |
| A single `time` field | Makes source latency unmeasurable and lets eye present old data as live. |
