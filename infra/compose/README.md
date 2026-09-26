# compose

**Status:** planned, no code yet.

## Responsibility

Local development orchestration for the whole stack: bringing up eye, api,
web, intelligence and twin together through Docker Compose, wired to
`make dev` once it exists.

## Must not

- Contain application or domain code.
- Define empty service containers for components that do not exist yet —
  entries are added only when a component has something to run.

## May depend on

Each component's own Dockerfile/build output. See
[`docs/architecture/system-overview.md`](../../docs/architecture/system-overview.md).

## Data

None; it wires up volumes and networks for components, it does not own
data itself.
