# Data ethics and reuse

This document is operational, not decorative. If you are adding a source or a
feature, the answer to "may we?" is here.

The binding decisions are [ADR-0006](../adr/0006-source-registry-gates-automation.md)
and [ADR-0007](../adr/0007-ethical-boundary.md).

## The one-line version

**eye observes public systems and phenomena. It does not build profiles of
people.**

## eye is a personal project, and some licences depend on that

This matters more than it looks. Several sources are licensed in a way that
works for personal use and breaks for anything else:

| Source | Terms |
|---|---|
| DGT camera **images** | Reproduction permitted *"a no ser que sea para uso personal y privado"* — i.e. only for personal and private use |
| OpenSky | Distinguishes personal and non-profit use from commercial use |
| Much of the Córdoba CKAN | Licence **unspecified**, which is not permission |

A personal `eye`, rendering a frame in its owner's terminal, is inside all of
these. The same code feeding a product, a client deliverable or a public service
is not — and that is a change of licence, not a change of scale.

So: **if eye ever feeds something commercial, the registry gets re-read source
by source first, not afterwards.** That is also why `eye serve` must never proxy
camera images: serving them to someone else is precisely the line these terms
draw.

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

**DGT publishes still images. It publishes no video, and there is none to find.**

Audited 2026-08-28 against the live feed. Every one of the 1948 devices in
`DevicePublication/camaras_datex2_v37.xml` is `typeOfDevice: camera` and carries
exactly one `<fse:deviceUrl>`, and every one of those 1948 URLs ends in `.jpg`.
The feed contains no `rtsp://`, no `rtmp://`, no `.m3u8` and no `.mp4`. The
public DGT viewer references no video endpoint either.

*(This corrects an earlier claim in this document, which said DGT documented no
image endpoint at all. It documents 1948 of them. The correction matters in the
direction that makes eye more capable, which is exactly when it is worth being
accurate.)*

### Why there is no meaningful stream, even by polling

The obvious next thought is to poll the JPEG fast enough to approximate video.
Measured across a sample of 13 cameras, the images were between **205 and 1160
seconds old** — a refresh of roughly five to twenty minutes. `Cache-Control` on
the CDN is `max-age=120`. One sampled camera had not updated in **53 days**.

So polling faster buys nothing but rate limits. A DGT camera is a periodic
still, not a slow video, and eye should present it as what it is.

That last figure is the important one operationally: a frame must always be
shown with its `Last-Modified` age. Rendering a 53-day-old image as the current
state of a road would be precisely the kind of lie this project exists not to
tell.

### Reuse terms are narrower than the metadata's

Two different licences are in play and they must not be conflated:

| | Terms |
|---|---|
| The DATEX II **metadata** (positions, road, PK) | Free of charge under the NAP dataset licence |
| The **images** on `etraffic.dgt.es` | DGT portal content under its [aviso legal](https://www.dgt.es/contenido/aviso-legal/) |

The aviso legal states that unauthorised reproduction, distribution,
commercialisation or transformation is an infringement **"a no ser que sea para
uso personal y privado"** — except for personal and private use.

Read plainly: rendering a frame in your own terminal is personal use.
Republishing those frames, serving them from `eye serve`, or building a product
on them is not, absent authorisation. The registry records that distinction, and
`eye serve` must never proxy camera images.

Córdoba's municipal CKAN publishes only `name` plus coordinates — no media field
of any kind, and no declared licence.

### The pipeline, unchanged

```
GET JPEG
  ↓ validate Content-Type and max size
  ↓ RAM cache, 30–120 s
  ↓ display (Kitty Graphics / chafa), with the Last-Modified age
  ↓ expire
```

There is no persistence branch in that flow, and none is to be added. The
existence of 1948 reachable image URLs does not widen the boundary; it is the
case the boundary was written for.

### Showing a frame outside the terminal

`eye camera --open` hands the frame to the system image viewer, and every such
viewer opens a *file*. That is a genuine tension with "RAM only", so it is
resolved explicitly rather than quietly:

- The frame is written to a **memory-backed filesystem** — `XDG_RUNTIME_DIR` or
  `/dev/shm`, both tmpfs. Nothing reaches a disk.
- If neither exists, eye **refuses**. It does not fall back to `/tmp`, which on
  many systems is a real directory on real storage.
- eye **blocks until the viewer closes**, then deletes the file. That is what
  makes "the frame exists only while you are looking at it" literally true
  rather than a promise.
- The directory is `0700`. Images of public roads are still not for every other
  account on the machine.

`eye camera --window` avoids the question entirely: it opens a fresh graphical
terminal and re-runs the command there, so the frame is drawn with the Kitty
graphics protocol and never becomes a file at all.

Both paths are covered by tests that assert no image reaches the data
directory, and that the tmpfs file is gone afterwards.

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
