# ADR-0009: Identity before fusion, and a change is an observation

## Status

Accepted. Builds on [ADR-0005](./0005-entity-and-record-are-separate.md).

## Context

eye reads 39 sources and returns 39 lists. It cannot say that the DGT incident
on the A-4 and the article the press published about it are the same happening,
and it cannot tell you that anything changed — you have to look, and compare
against what you remember. A dashboard you have to watch is not a dashboard.

The model already anticipated this in two places and neither was ever wired up:

- `Record.EntityID` — declared, and **nothing in the codebase writes it**.
- `Record.DedupeKey` — populated by every adapter, indexed in SQLite, and
  **read by nothing**.

The dedupe keys are worse than unused. All twelve begin with the source ID:

```go
DedupeKey: p.src.ID + ":" + ds.Name + ":" + ds.MetadataModified
```

A key prefixed with the source can only ever match a record against itself, so
cross-source deduplication is impossible by construction. And several bake the
mutable value into the key — `observedAt`, `MetadataModified`, `e.Minutes`, the
line's service hours. When the thing changes, the key changes, so the change
arrives looking like a brand-new fact. That is the exact opposite of what a
change detector needs.

There is a second trap, and it is the one that makes naive versions of this
feature lie. [Issue #24](https://github.com/FullFran/eye/issues/24) proposes
fingerprinting an event as `title + venue + start truncated to 30 minutes`,
while [issue #26](https://github.com/FullFran/eye/issues/26) wants to detect
`TIME -21:30 +22:00`. Those cannot both be true: **any field inside the identity
is a field whose change you can never detect**, because a changed value produces
a different identity and therefore a different thing.

## Decision

Fusion is three layers, and they have to be built in this order because each
needs the one below it. You cannot diff what you cannot pair, and you cannot
pair what you cannot name.

### 1. Identity — two kinds, and they answer different questions

| | Question | Built from |
|---|---|---|
| **`LocalKey`** | Is this the same thing this source told me about last time? | The source's own stable identifier, excluding everything mutable |
| **`Fingerprint`** | Is this the same thing another source is telling me about? | Normalized content: kind, folded title, coarse position, coarse date |

`LocalKey` enables change detection. `Fingerprint` enables cross-source
deduplication. Conflating them is what broke `DedupeKey`.

Both are computed in `observation/domain`, not by adapters. An adapter that
invents its own identity scheme cannot participate in fusion, and twelve
adapters each inventing one is how eye got here.

**Neither may contain a field eye wants to watch.** Identity is built from what
does not change: what the thing *is* and roughly where and when. Title, venue,
position, kind. Never status, never severity, never start time, never the count
of anything.

### 2. Change — a change is itself an observation

A change is emitted as a `Record` of kind `change`, so it lands in the timeline,
is queryable by topic and time, expires under the same retention, and carries
`Provenance` naming the two records it was derived from. No new storage
concept, no second timeline, and it can be replayed from the bytes.

**Absence is not an event.** A record that stops arriving may mean the thing
ended, or that the feed broke, or that our poll failed. eye already knows which:
`SourceState` records the last success, the consecutive errors and the staleness
of every source. So a disappearance is reported **only** when the source that
used to report it polled successfully and did not include it. A source that is
failing or stale produces silence, never a cancellation.

This is the whole difficulty of the feature and it is settled by data eye
already collects rather than by a guess.

### 3. Correlation — links carry their evidence, and assert nothing

Two *different* things being plausibly related — a bus delayed near an incident
— is inference, not observation. It is recorded as a link between two records
with the evidence that produced it attached: the distance, the interval, what
matched. Never as a merged record, never as a new fact, and always inspectable
back to both sides.

Deferred to its own slice, but the model reserves room for it now so it does not
arrive as a retrofit.

## What this does not do

- **It does not merge and discard.** Deduplication groups records; it never
  deletes one. Both keep their provenance, and the canonical view is computed at
  read time. Two publishers disagreeing is information, and a fusion engine that
  resolves it silently destroys the thing that made the disagreement visible.
- **It does not detect a reschedule across days.** Identity buckets by date, so
  a concert postponed three months reads as a disappearance and an appearance,
  not a change. When the source says `postponed`, that status is read directly.
  The limit is stated rather than papered over with a fuzzier bucket, which
  would collapse genuinely different events instead.
- **It does not score confidence in a match with a magic number.** Fuzzy
  matching gets a documented threshold and a scoring function that can be
  inspected, or it does not ship.

## Consequences

**Positive:**
- `EntityID` and `DedupeKey` stop being decoration.
- Change detection becomes possible at all, because identity excludes the
  mutable fields.
- "A source went quiet" and "the thing ended" become distinguishable, using
  health data eye already persists.
- Correlation cannot quietly become fabrication, because a link that carries no
  evidence cannot be written.

**Negative:**
- Every adapter's `DedupeKey` is wrong and has to be re-derived. Twelve of them.
- Two identity concepts where the model had one, and telling them apart is a
  judgement each adapter has to get right.
- Changes stored as records inflate the store: every field change of every
  incident is a row. Retention has to cover them from the start.
