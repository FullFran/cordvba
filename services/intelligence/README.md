# intelligence

**Status:** planned, no code yet. Product name pending.

## Responsibility

Understands questions and builds answers from eye and twin: retrieval,
RAG, semantic indexing, reranking, tool calling, query planning, agent
orchestration, answer synthesis, citations, evaluation. Routes, for
example, "history of the Torre de la Calahorra" to RAG over documents,
"when is the next bus?" to an eye tool call, and "how will the air be this
afternoon?" to a twin forecast; complex questions combine several of
these (plan → RAG/eye/twin → synthesis → answer + evidence).

It must preserve the epistemic type of every claim it uses or produces —
`OBSERVED`, `PUBLISHED`, `INFERRED`, `PREDICTED`, `SIMULATED` — see
[`docs/product/vision.md`](../../docs/product/vision.md). It never presents
a simulation as an observation, or an inference as official data.

## Must not

- Write to eye; it only reads eye's public API.
- Drive the browser directly; it proposes `ui_actions` that api validates.
- Be reachable by web directly; only api calls it.

## May depend on

[`apps/eye`](../../apps/eye/README.md) and
[`services/twin`](../twin/README.md), through their public contracts. See
[`docs/architecture/system-overview.md`](../../docs/architecture/system-overview.md).

## Data

Probably PostgreSQL + pgvector, in its own database/schema (not decided,
not introduced by this PR). Never eye's or twin's stores.
