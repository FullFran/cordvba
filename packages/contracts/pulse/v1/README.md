# Pulse contract, v1

The payload `apps/api` serves for the city's pulse (#141): what is happening
around Córdoba right now, drawn from every eye source that says something
timely — a weather warning, a thermal anomaly, an earthquake, a UCO event, a
local headline, a BOE bulletin. `apps/api` owns the endpoint and its
`/openapi.json` is the formal schema; the file here is the canonical
**example** both sides test against, exactly like
[`environment/v1`](../../environment/v1/README.md):

- `apps/api` tests assert that a real response has exactly this shape.
- `apps/web` uses it as a fixture, so the pulse rail can be built before the
  endpoint is deployed.

A breaking change to this shape goes in a new `v2/` folder; v1 stays until the
web has moved.

| Endpoint | Example |
|---|---|
| `GET /v1/pulse?hours=24` | [`pulse.example.json`](./pulse.example.json) |

## Response

```
{ generated_at, window_hours, items: [Item, ...] }
```

`generated_at` is when api composed the response. `window_hours` echoes the
`hours` query parameter that was actually applied. `items` is newest first by
`observed_at`, bounded to 50. A source with nothing to report in the window
contributes no items; it is never padded or listed as empty.

## Item

| Field | Type | Meaning |
|---|---|---|
| `kind` | string | `warning`, `fire`, `quake`, `event`, `headline` or `bulletin` |
| `id` | string | eye's own record id, opaque |
| `title` | string | the source's own title, in its own language — never translated |
| `observed_at` | RFC 3339 | when the thing was observed, detected or (for an editorial item) discovered by eye |
| `valid_from` | RFC 3339 or absent | when a warning takes effect, or an event starts |
| `valid_until` | RFC 3339 or absent | when a warning stops applying |
| `position` | `{lat, lon}` or absent | present when the source gives coordinates (a fire pixel, an epicentre) |
| `area` | string or absent | a named place instead of coordinates (an AEMET warning zone); an item with neither has no reliable geography (a national BOE bulletin) |
| `severity` | string | `none`, `info`, `low`, `moderate`, `high` or `critical` — eye's own cross-source scale (see `apps/eye/internal/observation/domain/record.go`), rendered as a name instead of leaking its internal 0–5 encoding |
| `url` | string or `null` | a link to the source's own item, when it publishes one; `null` for a source with no per-item page (NASA FIRMS is raw detections, not articles) |
| `label` | `OBSERVED` or `PUBLISHED` | see below |
| `provenance` | `{source, publisher, licence, fetched_at}` | `source` is eye's source id; the rest is enough to trust and cite the item without exposing eye's internal URL or token |
| `attribution` | string | ready to print next to the item |

`position` and `area` are both optional and never both required: an item can
have neither (a national bulletin), one or the other, never both filled from
the same source.

### `label`: OBSERVED vs PUBLISHED

Two epistemic classes, not four (environment/v1's `OBSERVED` /
`INFERRED` / `PREDICTED` / `SIMULATED` do not apply here — pulse carries no
derived values):

- **OBSERVED** — `warning`, `fire`, `quake`. An authority or a sensor detected
  a real physical condition. `observed_at` is when that happened.
- **PUBLISHED** — `event`, `headline`, `bulletin`. Somebody published a
  statement. For `event`, `observed_at` is when eye discovered the listing
  (its own fetch time — the UCO feed carries no publication date, only the
  event's start, which lives in `valid_from`); the event itself may be well
  outside the `hours` window. For `headline` and `bulletin`, `observed_at` is
  the publisher's own dateline.

## Sources and Córdoba scope

Only redistributable sources appear (`apps/api` checks `GET /v1/sources`'
`redistributable` flag before including anything, never a hardcoded
allowlist that could go stale):

| Source id | `kind` | Córdoba scope |
|---|---|---|
| `aemet-warnings` | `warning` | the registry's own `zones` option, three AEMET zones (issue #142) |
| `nasa-firms` | `fire` | api applies a Córdoba-province bounding box, below |
| `ign-seismic` | `quake` | api applies the same bounding box; eye's own feed is nationwide and carries no province code to filter on upstream |
| `uco-events` | `event` | the feed itself is Córdoba-only |
| `cordopolis` | `headline` | the feed itself is Córdoba-only |
| `eldiadecordoba` | `headline` | the feed itself is Córdoba-only |
| `boe` | `bulletin` | national in scope, kept because a Córdoba-relevant decree is still worth surfacing; carries neither `position` nor `area` rather than a false one |

`nasa-firms` and `ign-seismic` are both national/continental feeds narrowed by
coordinates, not by a place code: a fire pixel or an epicentre carries only a
latitude and longitude, never a station code the way `miteco-ica` does
(`province: "14"`, #98). The bounding box api applies is
**west −5.55, south 37.25, east −4.05, north 38.72** — the union of the three
real AEMET warning-zone polygons Córdoba province is made of (Sierra y
Pedroches, Campiña cordobesa, Subbética cordobesa), read from the recorded CAP
fixtures at `apps/eye/testdata/aemet/cap/*.xml` and `apps/eye/testdata/meteoalarm/spain-warnings.xml`,
not a guess and not the wider box `nasa-firms`'s own registry entry already
uses for "Córdoba province and its approaches". Like any rectangle over a
non-rectangular province, it can still admit a point just across the real
border — the same approximation issue #98 found and fixed for `miteco-ica` by
switching to a station-code filter, unavailable here because neither a FIRMS
pixel nor an IGN epicentre carries one.

## Bounds

`hours` (query parameter) accepts 1–72; anything outside returns 422. Whatever
window comes back is bounded to 50 items regardless of `hours`.

## Example values

The example uses one item per `kind`, two for `headline` to show two
different publishers under the same kind. The `aemet-warnings`, `boe` and
`nasa-firms` items are built from real recorded fixtures
(`apps/eye/testdata/meteoalarm/spain-warnings.xml`,
`apps/eye/testdata/rss/boe.xml`, and the Córdoba detection asserted in
`apps/eye/internal/provider/infrastructure/firms/firms_test.go`) with their
dates shifted to fall inside the example's 24-hour window. The `ign-seismic`
item is illustrative: eye's recorded seismic fixture
(`apps/eye/testdata/rss/ign-sismologia.xml`) has no earthquake anywhere near
Córdoba, and that same fixture's entries carry no `pubDate` at all — in
production an `ign-seismic` item's `observed_at` may equal its `fetched_at`
until IGN's feed publishes one.
