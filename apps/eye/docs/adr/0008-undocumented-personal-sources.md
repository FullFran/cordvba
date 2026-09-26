# ADR-0008: Allow undocumented sources, gated twice, for personal use only

## Status

Accepted. Amends [ADR-0006](./0006-source-registry-gates-automation.md), which
stands otherwise.

## Context

ADR-0006 made the registry fail closed and refused `undocumented_backend`
outright. That was the right default and it has already paid for itself: the
Córdoba BOP is a React application over an internal API, and eye correctly
declines to scrape it.

But "undocumented" and "forbidden" turned out to be two different things, and
collapsing them costs real data. RENFE publishes live train positions for
Cercanías and not for the AV/LD network that serves Córdoba. ADIF has that
information — its own station boards show it — and publishes nothing. The
national catalogue carries only network geometry and statistics.

eye is a personal project ([README](../../README.md)). An endpoint a public site
calls to render public information is a reasonable thing for a person to read
for themselves, and refusing on principle is not caution, it is just less data.

What the original rule was actually protecting is worth keeping, though:

- The registry is a **legal contract**, and its value comes from being able to
  say precisely what eye reads and under what terms.
- Results must be **reproducible**: somebody else running eye should get what
  the registry says they will.
- eye must never **redistribute** what it was not entitled to redistribute.

## Decision

Add a distinct access kind, `undocumented_personal`, gated twice over.

| | |
|---|---|
| **Registry gate** | The entry must say `access: undocumented_personal`, and must carry `notes` explaining what the endpoint is. A source nobody can explain is one nobody can review — `Source.Validate()` enforces it. |
| **Machine gate** | The operator must set `EYE_ALLOW_PERSONAL_SOURCES=1`. The registry saying `enabled` is deliberately not enough: this is a decision about somebody's own machine, so the machine has to say so too. |
| **No redistribution** | `Source.Redistributable()` returns false, and the API filters those records out of every response. The rule lives in one place because a rule spread across handlers is one that gets forgotten in the next one. |
| **Labelled everywhere** | `eye sources` and `eye status` show them as what they are, never as ordinary sources. |

`undocumented_backend` keeps its original meaning and its outright refusal. The
distinction between the two is the operator's explicit, recorded decision.

## What this does not license

Three things stay refused, and they are different from "undocumented":

- **Bypassing an access control.** An endpoint behind authentication we do not
  own stays out, whatever it returns.
- **Evading a block.** A service returning `403` to an identified client has
  asked automated clients not to call it. eye identifies itself in every
  request precisely so an operator can see who we are; spoofing a browser to
  get past that contradicts the property that makes eye defensible. ADIF is
  currently in this position.
- **Personal data.** Unchanged by this ADR, and not negotiable — see
  [ADR-0007](./0007-ethical-boundary.md).

## Consequences

**Positive:**
- Data eye can genuinely use stops being refused on a technicality.
- The distinction is now a mechanism rather than a judgement call made in a
  pull request at midnight.
- `eye serve` cannot leak a personal source even by accident.

**Negative:**
- Two source kinds where there was one, and a runtime flag to remember.
- A result that depends on a personal source is not reproducible by anyone
  else, which is exactly why those records never leave the machine.
- These sources will break without warning. That is the deal, and the registry
  says so.

## Alternatives considered

| Alternative | Why it was rejected |
|---|---|
| Keep refusing everything undocumented | Costs real data for no benefit on a personal project, and the reproducibility argument is answered by not redistributing rather than by not reading. |
| Allow them with only the registry gate | The registry is committed to a repository. A machine-level opt-in is what keeps "I read this on my laptop" from becoming "eye reads this for everyone who clones it". |
| Allow them and serve them like anything else | Redistribution is the part that actually creates exposure, both legal and factual. |
