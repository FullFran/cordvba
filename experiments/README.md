# experiments

**Status:** planned, no code yet.

## Responsibility

Notebooks and prototypes for exploring ideas before they become a
component — model exploration, data exploration, one-off analysis.

## Must not

- Be imported by production code, in any component.
- Be deployed, ever.

## May depend on

Anything, read-only, for exploration purposes; nothing depends on it back.
See
[`docs/architecture/system-overview.md`](../docs/architecture/system-overview.md).

## Data

Whatever local fixtures a given experiment needs; never shared production
data or credentials.
