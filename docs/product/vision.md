# Vision — cordvba

## What this is

CORDVBA is an experimental computational model of the city of Córdoba
(Spain): a personal, open, non-commercial project. It is not a chatbot with
a city theme — the city itself is the interface. It integrates observable
reality, documentary knowledge, current city state, prediction, simulation,
natural-language interaction, geospatial and temporal exploration,
scientific visualisation, and clearly-labelled illustrative renderings
derived from simulations.

## What this is not

Not a general-purpose assistant, not a chat product wrapped around an LLM,
not a surveillance tool. There is no face recognition, no plate indexing, no
per-person tracking. A simulation is never presented as an observation, and
an inference is never presented as official data — see the epistemic labels
below.

## Components, in one paragraph each

- **eye** (`apps/eye`, Go) — observes Córdoba. The city data plane: acquires
  public data, normalises it, and keeps history, provenance, quality and
  time semantics. It runs alone, with no LLM, ML, simulation or user state
  inside it.
- **twin** (`services/twin`, Python) — models what the city may do next.
  Derived state, forecasts and counterfactual simulations, each carrying
  its model, version and uncertainty. It only reads eye's public API.
- **intelligence** (`services/intelligence`, Python; name pending) —
  understands questions and builds answers from eye and twin: retrieval,
  RAG, tool calling, query planning, agent orchestration, citations. It
  never turns a simulation into a claimed observation.
- **api** (`apps/api`) — the product backend: the only backend the web
  knows. Composes eye, twin and intelligence into one representation and
  owns product-level concerns (sessions, favourites, rate limiting).
- **web** (`apps/web`) — makes the city explorable: a map, a timeline, and
  ways to ask, forecast and simulate. It talks only to api.

## Visualisation levels

- **Data** — observed values, as recorded by eye.
- **Model** — heatmaps, particle fields, forecast bands, isolines,
  uncertainty bands: the output of twin's models.
- **Experience** — illustrative renderings derived from state or
  simulation. Always labelled "Illustrative simulation"; never confused
  with observed data or real imagery.

## Epistemic labels

Every claim the system produces carries one of five types, and intelligence
must preserve it end to end:

| Label | Meaning |
|---|---|
| `OBSERVED` | Measured or recorded by eye from a public source. |
| `PUBLISHED` | Stated in a document (e.g. an official notice), not measured. |
| `INFERRED` | Derived by intelligence from other claims (e.g. RAG synthesis). |
| `PREDICTED` | A twin forecast, with model, version and uncertainty. |
| `SIMULATED` | A twin counterfactual — "what if" — never a forecast of reality. |

## Indicative roadmap

v0.1 environment twin (weather and air quality: current state, baseline
forecast, what-if) → air-quality ML → pollen → traffic → events and
mobility → intelligence/RAG → multimodal simulations.

This order is indicative, not a commitment: it reflects the smallest
useful twin first, then layers of increasing modelling complexity.
