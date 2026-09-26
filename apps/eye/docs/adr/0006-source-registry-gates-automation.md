# ADR-0006: The source registry gates automation, and fails closed

## Status

Accepted

## Context

Several of the most valuable sources for Córdoba — SAIH Guadalquivir, the
INFOCA active-fire viewer, e-distribución's outage map — publish real
information through a public web viewer with no documented reuse contract.
Behind each viewer there is usually a JSON endpoint that is trivial to call.

Calling it is how projects end up in the well-known failure mode: it worked
because we called `/internal/v3/mapData` until they blocked us. Worse, the
question of whether we *may* is decided implicitly, by whoever wrote the
adapter, at the moment they wrote it.

That decision is not a coding decision. It needs to be visible, reviewable and
made once.

## Decision

`configs/sources.yaml` is the registry, and it is the legal contract of the
project. Nothing is fetched that is not declared there.

Each entry declares an `access` kind and an `automation` gate:

| `access` | Meaning |
|---|---|
| `documented_api` | Published machine-readable contract — CKAN, DATEX II, GTFS-RT, REST |
| `documented_download` | Published file resource with stable terms |
| `public_html` | Public page, no documented reuse contract |
| `undocumented_backend` | Internal endpoint behind a public viewer |

| `automation` | Meaning |
|---|---|
| `enabled` | The scheduler may poll it |
| `review_terms` | Valuable, but the terms are unclear — held out until a human resolves it |
| `manual_link` | eye links to the viewer and ingests nothing |
| `disabled` | Never fetched automatically |

`AutomationStatus.Pollable()` returns true for `enabled` and for nothing else,
including unknown values: the gate **fails closed**. `Source.Validate()`
additionally refuses any entry that pairs `enabled` with
`undocumented_backend`, so that combination cannot be committed by accident.

A held source is not hidden. `eye status` shows it as held, with the reason,
next to the sources that are running.

## Consequences

**Positive:**
- Whether eye may automate a source is answered in review, in one file, by
  people — not implicitly by an adapter author at 2am.
- A typo in the automation field silences a source rather than unleashing it.
- The registry doubles as the project's licence inventory.

**Negative:**
- Genuinely useful data sits unused while a licence question is open. That is
  the intended trade: the project stays publishable and auditable.
- Someone has to do the legal legwork per source.

## Alternatives considered

| Alternative | Why it was rejected |
|---|---|
| Decide per adapter, in code | Makes the most consequential decision in the project invisible to review. |
| Default to enabled, disable on complaint | Optimises for the wrong outcome, and "we stopped once they noticed" is not a defence. |
| Fetch anything reachable | Not a technical limitation but the boundary that lets the repository exist in public. |
