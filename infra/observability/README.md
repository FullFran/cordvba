# observability

**Status:** planned, no code yet.

## Responsibility

Cross-component observability: logging, metrics and tracing configuration
that spans more than one component (dashboards, alerting rules).

## Must not

- Contain application or domain code.
- Read a component's persistence directly; it consumes exported
  logs/metrics/traces, not internal state.

## May depend on

Each component's exported telemetry, through whatever format they emit.
See
[`docs/architecture/system-overview.md`](../../docs/architecture/system-overview.md).

## Data

Metrics/log/trace storage configuration only; never product data.
