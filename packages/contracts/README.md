# contracts

**Status:** planned, no code yet.

## Responsibility

Shares contracts, not implementations: OpenAPI references, JSON Schema,
event schemas, UI-action schemas, simulation-request schemas, shared by
more than one component.

## Must not

- Contain eye's own OpenAPI spec or its Go domain — eye keeps owning both;
  this package references them, it does not move them.
- Contain business logic or runtime code.

## May depend on

Nothing in this repository; other components depend on it, not the other
way around. See
[`docs/architecture/system-overview.md`](../../docs/architecture/system-overview.md).

## Data

None. Schemas and references only.
