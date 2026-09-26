# evals

**Status:** planned, no code yet.

## Responsibility

Shared AI evaluation: retrieval, RAG, tool selection, agent behaviour,
citation correctness, answer faithfulness — the kinds of evaluation useful
to more than one AI-facing component.

## Must not

- Cover forecasting evaluation — that stays inside
  [`services/twin`](../../services/twin/README.md), which owns its own
  model metrics.
- Contain product or business logic.

## May depend on

Whatever fixtures or contracts it evaluates against, read-only. See
[`docs/architecture/system-overview.md`](../../docs/architecture/system-overview.md).

## Data

Evaluation fixtures and datasets only; no production persistence.
