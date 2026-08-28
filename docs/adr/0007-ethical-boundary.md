# ADR-0007: eye observes public systems, never people

## Status

Accepted

## Context

The tools `eye` assembles — camera inventories, vehicle positions, event
schedules, spatio-temporal correlation — are the same tools a surveillance
system is built from. The difference is not technical capability. It is where
the boundary is drawn, and whether it is written down before the capability
exists rather than after.

Under Spanish and EU law the image of an identifiable person is personal data;
the AEPD requires necessity, proportionality and minimisation in video
surveillance, and systematic recording of public thoroughfares for security
purposes is generally reserved to the authorities. Independently of the law,
a project that can answer "where was this person at 13:32" is a different
project from this one.

## Decision

eye observes **public systems and phenomena**. It does not build profiles of
people.

Permitted:

- Position and metadata of public cameras, as an inventory.
- Consuming documented public interfaces: DATEX II, GTFS, CKAN, AEMET, FIRMS.
- Caching public payloads, subject to their licence.
- Correlating fire, wind, hydrology, traffic and events.
- Rendering a frame from an explicitly public image endpoint, in RAM, for
  30–120 seconds, with no persistence.

Out of scope, permanently:

- Face recognition, or any persistent identity index.
- Licence plate reading or indexing.
- Per-person tracking, or per-vehicle trails retained by default.
- Scanning for undocumented RTSP endpoints, enumerating cameras, or bypassing
  any access control.
- Archiving urban video.
- Presenting an eye inference as an official 112, INFOCA or AEMET notice.

Three mechanisms enforce this rather than merely stating it:

1. Camera media is RAM-only with a TTL. There is no persistence path in the
   code for it.
2. Movement data (ADS-B, GTFS-RT) carries `ExpiresAt` and a default retention
   of 24–72 hours. Long-term storage is aggregate: flights per hour, delay
   distributions, incidents per segment.
3. Every inference is `Quality: inferred` and carries a disclaimer field.

## Consequences

**Positive:**
- The repository can be public, audited and contributed to.
- The scope is small enough to actually finish.
- Explicit limits are what make individual features safe to build.

**Negative:**
- Some questions eye could technically answer, it will not.
- Movement history is short by default, so some analyses need aggregates
  designed up front instead of ad-hoc queries over raw trails.

## Alternatives considered

| Alternative | Why it was rejected |
|---|---|
| Leave ethics to a README paragraph | Prose does not stop a retention path from being added. Mechanisms do. |
| Decide per feature as it comes up | The boundary has to exist before the capability, or it is written by whoever wants the feature most. |
