# eye-client

**Status:** planned, no code yet.

## Responsibility

A client for eye's HTTP contract, probably Python first since twin and
intelligence are its first consumers. Ideally generated from eye's OpenAPI
spec once it is stable.

## Must not

- Contain business logic — it is a thin client, not a use case.
- Read eye's SQLite store or raw cache directly, or import
  `apps/eye/internal`; it talks to eye's HTTP API only.

## May depend on

[`apps/eye`](../../apps/eye/README.md)'s public HTTP API and OpenAPI spec
only. See
[`docs/architecture/system-overview.md`](../../docs/architecture/system-overview.md).

## Data

None of its own.
