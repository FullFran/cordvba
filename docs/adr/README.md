# Architecture Decision Records

An ADR captures a significant technical decision: what was decided, why, and
what it costs. ADRs are append-only — they are never rewritten, only superseded
by a new one.

## When to write one

Write an ADR when the decision is:

- **Hard to reverse** — storage engine, architectural pattern, dependency policy.
- **Not obvious in six months** — why this option and not the natural alternative.
- **Binding on everyone** — layer conventions, retention policy, ethical scope.

Do not write one for routine or easily reversible decisions.

## Index

| ADR | Title | Status |
|---|---|---|
| [0001](./0001-single-binary-several-modes.md) | Ship eye as a single binary with several modes | Accepted |
| [0002](./0002-hexagonal-per-domain-layout.md) | Keep the hexagonal per-domain layout | Accepted |
| [0003](./0003-sqlite-without-cgo.md) | Store observations in SQLite, without CGO | Accepted |
| [0004](./0004-dependency-budget.md) | Spend dependencies from a fixed budget | Accepted |
| [0005](./0005-entity-and-record-are-separate.md) | Separate Entity from Record, and require provenance on both | Accepted |
| [0006](./0006-source-registry-gates-automation.md) | The source registry gates automation, and fails closed | Accepted |
| [0007](./0007-ethical-boundary.md) | eye observes public systems, never people | Accepted |
| [0008](./0008-undocumented-personal-sources.md) | Allow undocumented sources, gated twice, for personal use only | Accepted |
| [0009](./0009-identity-before-fusion.md) | Identity before fusion, and a change is an observation | Accepted |
| [0010](./0010-three-surfaces-one-model.md) | Give eye three surfaces over one model, and buy none of them | Accepted |

## How to add one

1. Copy `template.md` to `NNNN-kebab-title.md` (next number in sequence).
2. Fill in every section.
3. Add the row to the index above.
4. Ship the ADR in the same PR that implements the decision.

## How to supersede one

Write a new ADR describing the new decision. In the old one, change the status
to `Superseded by ADR-NNNN`. Never edit the original's content.
