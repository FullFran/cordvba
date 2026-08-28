# Data ethics and reuse

This document is operational, not decorative. If you are adding a source or a
feature, the answer to "may we?" is here.

The binding decisions are [ADR-0006](../adr/0006-source-registry-gates-automation.md)
and [ADR-0007](../adr/0007-ethical-boundary.md).

## The one-line version

**eye observes public systems and phenomena. It does not build profiles of
people.**

## Reachable is not the same as reusable

A resource being visible from the internet does not mean it may be copied,
stored and republished without conditions. In Spain the reuse of public sector
information is regulated, and many bodies apply general conditions or
attribution licences. `datos.gob.es` notes that the state's general conditions
approximate CC BY 4.0 — but each dataset keeps its own licence and provenance,
and several Córdoba CKAN datasets declare none at all.

That is why `Provenance.License` is a required field and why "no licence
declared" is stored as `unspecified` rather than left blank or assumed
permissive. An unknown licence is a fact worth keeping.

## The decision table

| Action | Verdict |
|---|---|
| Show the position of a public camera | Allowed |
| Consume a documented public API (DATEX, GTFS, CKAN, AEMET, FIRMS) | Allowed |
| Cache a public payload | Allowed, subject to its licence |
| Correlate fire, wind, hydrology and traffic | Allowed |
| Alert on official incidents and closures | Allowed |
| Render a frame from an explicitly public image endpoint | Allowed, RAM only, 30–120 s TTL |
| Archive urban video | Avoid |
| Infer identity from faces | Out of scope, permanently |
| Index licence plates | Out of scope, permanently |
| Probe for undocumented RTSP endpoints | Never |
| Bypass authentication | Never |
| Reuse OpenSky commercially while ignoring its terms | Never |
| Present an eye inference as a 112 or INFOCA notice | Never |

## Camera media

The current state of CCTV is worth stating precisely, because it is routinely
assumed to be otherwise.

```
CAMERA ASSET
    id + position + road metadata
              │
              ├──────────────────┐
              ▼                  ▼
       STILL IMAGE URL      VIDEO STREAM
        JPEG / WebP          HLS / RTSP
```

For DGT, only the first block is officially confirmed. The `Cámaras DGT DATEX2
v3.7` dataset publishes an XML resource with an XSD, classified as static road
data with hourly updates. It documents **no** JPEG, HLS or RTSP endpoint. The
Córdoba CKAN publishes camera identification and location in CSV, GeoJSON and
PDF, with no media field and no declared licence.

So eye integrates the **inventory**. If an authority later publishes a media URL
with terms that permit reuse, the adapter is small and the pipeline is already
specified:

```
GET JPEG
  ↓ validate Content-Type and max size
  ↓ RAM cache, 30–120 s
  ↓ display (Kitty Graphics / chafa)
  ↓ expire
```

There is no persistence branch in that flow, and none is to be added.

## Retention

| Data | Retention |
|---|---|
| Camera frames | RAM only, 30–120 s. Never written to disk. |
| ADS-B positions | 24–72 h raw; aggregates thereafter |
| GTFS-RT vehicle positions | 24–72 h raw; delay distributions thereafter |
| Road incidents | 90 days detailed, aggregates indefinitely |
| Weather, hydrology, air quality | Years — they are environmental series |
| Public documents (BOE, BOP, PLACSP) | Indefinite |
| Raw payloads | Kept as evidence for anything an inference depends on |

Long-term movement storage is aggregate by design:

```
kept:     flights_per_hour, train_delay_distribution, incidents_per_segment
not kept: perpetual per-vehicle trails
```

## Rate limiting

"Real time" does not mean "poll every five seconds". FIRMS latency is orbital,
not network — polling faster changes nothing. MITECO ICA is hourly. DGT declares
one hour for its camera inventory. AUCORSA refreshes its GTFS up to every 24
hours.

`configs/sources.yaml` records both `interval` (our engineering decision) and
`published_every` (the publisher's declared cadence). Polling faster than the
latter buys nothing but rate limits.

## Adding a source: the checklist

- [ ] Is there a **documented** public interface? If it is an endpoint found in
      DevTools behind a viewer, the answer is no.
- [ ] What licence does the publisher declare? Record it verbatim; use
      `unspecified` when there is none.
- [ ] What cadence does the publisher declare? Set `interval` at or above it.
- [ ] Does the source state anything about data quality? Preserve it as
      `Quality` — SAIH's "not cross-checked" must reach the UI.
- [ ] Does this feed contain personal data? If so, stop and reconsider the scope.
- [ ] Set `automation` honestly. `review_terms` is a normal, respectable state,
      not a failure.

## Attribution

Every alert resolves to its evidence: which source, how old, under what licence,
and a link back to the original. This is not politeness. It is the property that
makes an inference checkable, and it is the difference between a data fusion
centre and a rumour with a monospace font.
