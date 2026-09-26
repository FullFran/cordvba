# ADR-0001: Ship eye as a single binary with several modes

## Status

Accepted

## Context

The research that seeded this project proposed a two-binary gateway: `eyed`
running a scheduler and serving REST + SSE, and `eye` as an HTTP client. That
architecture is correct for the destination — a web map, a Cesium globe, a
mobile client — but it front-loads network configuration, CORS, authentication
and API versioning before a single provider adapter exists.

The explicit design constraint for this project is that it must be **light by
design**. Light does not mean simplistic: the domain architecture (providers →
normalize → Entity/Record → fusion) is sound and is kept in full. What gets
made light is the *deployment*.

## Decision

`eye` ships as one binary with several modes:

| Mode | What it does |
|---|---|
| `eye status`, `eye query`, `eye events`, `eye radar` | Answer from the local store |
| `eye daemon` | Run the scheduler in a loop |
| `eye serve` | Expose HTTP + SSE — opt-in, never required |

All modes are infrastructure adapters over the same application layer. There is
nothing to deploy but the build output.

## Consequences

**Positive:**
- Deployment is copying one file. No compose file, no service to supervise, no
  port to open unless the user asks for one.
- `eye serve` keeps the door open for a web or Cesium client without any
  provider being rewritten.
- The HTTP layer cannot quietly become the centre of the system, because
  nothing else depends on it.

**Negative:**
- A long-running `daemon` and a short-lived `status` share one process image, so
  the binary carries code some invocations never use. At this scale that costs
  a few hundred kilobytes.
- Multi-machine deployments (ingest here, query there) need `eye serve` to be
  built out before they are possible. That is deliberate: it is not a v0.1
  problem.

## Alternatives considered

| Alternative | Why it was rejected |
|---|---|
| Two binaries (`eyed` + `eye`) with REST/SSE from day one | Contradicts the "light by design" constraint and multiplies v0.1 work before any provider exists. |
| Pure CLI driven by systemd timers | Delegates the scheduler to the OS, losing jitter, backoff and circuit breaking, and cannot express a 10-second ADS-B cadence. |
