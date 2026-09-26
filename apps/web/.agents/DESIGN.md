# DESIGN.md — CORDVBA web (apps/web)

Binding for `apps/web`. If an implementation conflicts with this file, the
implementation is wrong — surface the conflict rather than silently picking a
side. Superseded entries are marked, not deleted.

## 1. Intent & North Star

**"Void Signal."** CORDVBA's web page is a data console, not a form over a
road map. The city sits muted in a dark void; every piece of *live,
verifiable* data — a beacon, the timeline's now-marker, a focus ring —
glows against it. Nothing glows just because it can: luminance is reserved
for signals, never decoration. This follows the shared pattern across the
research references (Digital Twin Victoria, Helsinki 3D, Madrid's gemelo
digital, kepler.gl/deck.gl showcases, earth.nullschool.net, windy.com): dark
base, extruded 3D buildings at a low pitch, data as luminous signals over a
muted city, motion that shows weather happening, a prominent time scrubber,
minimal chrome, tabular numerals.

**Voice:** clinical, honest, slightly cold. This is an instrument reading a
city, not a marketing page. Every value states its epistemic status
(observed/inferred/predicted/simulated) as plainly as its number. A
simulation is never allowed to look like an observation.

**Anti-references:** no purple-gradient glassmorphism, no skeuomorphic globe,
no "smart city" stock-photo aesthetic, no cheerful onboarding copy. Google
Maps' light, retail-friendly warmth is the wrong register for this project.

## 2. Tokens

Single source: `src/styles/tokens.css` (CSS custom properties on `:root`).
Every component stylesheet reads a `var(--…)`; a stray hex/rgb/hsl literal
in component CSS is a `lint_hardcodes` failure, not a style choice (a small,
explicitly commented `ds-allow-hardcode` exception exists only in
`src/map/darkStyle.ts`'s runtime CSS-variable fallback and in test fixtures
that model *someone else's* colours — OpenFreeMap's original light style —
never this app's own design decisions).

| Token | Value | Role |
|---|---|---|
| `--color-bg` | `#1a1b26` | Page background — always opaque, never inherited from the browser theme; matches the host site's (fullfran.com) Tokyo-Night-derived background exactly, for continuity (see Decision Log) |
| `--color-bg-elevated` | `#1f2335` | Panels, cards, the provenance dialog |
| `--color-text` | `#e7ebf3` | Primary text — always explicit, never `inherit` |
| `--color-text-muted` | `#97a3ba` | Secondary text, ages, captions |
| `--color-accent` | `#2dd4bf` | The one luminous colour for *live* signals: predicted-label texture, focus, beacons' base glow |
| `--color-focus-ring` | `#7dd3fc` | Keyboard focus outline |
| `--texture-simulated` | `#f5a524` (amber) | Reserved *exclusively* for SIMULATED — a counterfactual, never real — so it never competes with the teal live-signal accent |
| `--color-aq-good` … `--color-aq-extremely-poor` | 6 hues | Air-quality category hint; never the only encoding (shape + text always carry it too) |
| `--map-bg`, `--map-water`, `--map-building-low/-high`, `--map-label-halo` | dark navy/green/grey | The MapLibre style's curated overrides (`src/map/darkStyle.ts`) |
| `--font-sans` | `"IBM Plex Sans", -apple-system, …` | UI text |
| `--font-mono` | `"IBM Plex Mono", ui-monospace, …` | Numeric readouts, the network-error reason |
| `--space-1`…`--space-8` | 4px-based scale | Spacing |
| `--radius-sm/md/lg/full` | 4/8/12px/pill | Radius |
| `--duration-fast/base/slow` | 150/300/900ms | Motion — all zeroed under `prefers-reduced-motion` in one place |
| `--shadow-panel` | two-layer shadow | Elevation |

Typography carries `font-variant-numeric: tabular-nums` globally (`body`),
so every number in a `ValueTile` lines up vertically without depending on a
specific font's OpenType features.

## 3. Layout Primitives

The map IS the page (issue #119, maintainer review), not a map in a box
above a grid of cards. Two layout modes, one breakpoint at `48rem` (768px):

- **≥48rem (desktop-class):** `.app__stage` is `position: fixed; inset: 0`
  — full-bleed, 100vh × 100vw. `.app__hud`'s children (`.app__panel--*`)
  are each individually `position: fixed`, laid out as a restrained HUD:
  header overlay top-left, a compact single-column state dock top-right
  (`19rem` wide — the 2-column `.state-panel` grid is overridden back to
  one column here, since that rule is keyed to *viewport* width and this
  dock's width is fixed regardless of viewport), the timeline scrubber a
  full-width bar along the bottom edge, source attribution bottom-left,
  and a collapsible "Scenario" drawer under the state dock (closed by
  default — keeps the HUD from clutter until asked for). `fitBounds`
  padding (`MapView.tsx`'s `FIT_PADDING`) keeps the initial camera from
  ever framing a station behind these panels.
- **<48rem (mobile):** everything reverts to normal document flow:
  `.app__stage` is a static `60vh` block on top, `.app__panel`s stack
  below it in order (header, state, scrubber, footer), matching the
  reviewer's explicit mobile spec.
- Panels are opaque `--color-bg-elevated` surfaces with a `1px
  --color-border` and `--shadow-panel` — deliberately not
  glassmorphism/blur (banned in §7 unless recorded as a decision, and it
  is not).
- The map stage (`.map-view__stage`) fills its container (`height: 100%`
  inside `.app__stage`, a `22rem` fallback elsewhere); the MapLibre canvas
  and the wind-particle canvas are both absolutely positioned to fill it
  (`inset: 0`). Sizing this from a container with no explicit height once
  rendered at a near-empty fallback size — see Decision Log.
- The honesty caption (extrusion heights / no interpolated surface) and
  the illustrative-wind label are small absolutely-positioned overlay
  chips inside the map stage, not layout-height-consuming block text —
  the map still fills the frame.
- Verified usable at 390×844 (AC-6): no horizontal overflow, the state
  dock collapses to normal-flow single-column cards, the map stays
  legible and every beacon's label stays untruncated.

## 4. Component Patterns

- **Header** (`Header.tsx`) — title + 3 honesty lines + the ES|EN
  `LocaleSwitch` (a two-button `role="group"`, each with `aria-pressed`).
  Always rendered, even during loading/error, so the page's identity is
  never blank.
- **MapView** (`MapView.tsx`) — MapLibre GL over OpenFreeMap's `liberty`
  style, recoloured dark and decluttered of basemap noise by
  `src/map/darkStyle.ts` (POI/transit icons and minor road/place labels
  hidden outright — `SUPPRESS_IDS`; a few kept landmark/major-road labels
  muted to 55% opacity — `MUTE_IDS` — so beacons and wind stay the only
  saturated elements). Pitched 55°, bearing -17°, and — since real station
  coordinates can differ from the fixtures — the initial camera always
  `fitBounds`es the three city stations plus the historic centre, with
  HUD-aware padding (`FIT_PADDING`), rather than a fixed center/zoom.
  Beacons (`src/map/beacons.ts`) are pills, not fixed small circles (a
  category word like "extremely poor"/"desfavorable" never fit a ~44px
  circle): a glow + two pulsing rings (CSS, frozen under reduced motion),
  a small `.beacon__shape` severity icon (circle→star, softest→sharpest)
  separate from the text, and the index/category as literal text — never
  colour alone (AC-2). No interpolated air-quality surface, ever. When the
  airport beacon's true position falls outside the viewport, a
  `clampToEdge`-positioned arrow + distance (`src/map/geo.ts`) replaces it
  instead of it silently vanishing. A caption states the two honesty notes
  verbatim ("Extrusion heights: OpenStreetMap, approximate…"; "Beacons
  show individual stations only — no interpolated surface") as a small
  overlay chip, not layout-height text.
- **WindParticles** (`WindParticles.tsx`) — an illustrative canvas particle
  field over the map, driven by the single METAR reading
  (`src/map/wind.ts`'s pure velocity math). Always shows its honesty label
  when on ("Illustrative wind: one station, not spatially varied"),
  toggleable, frozen to one static frame under `prefers-reduced-motion`.
- **TimelineView** (`TimelineView.tsx`) — past/now/future points as real
  `<button>`s (keyboard-native, unlike a custom slider widget would need to
  be), each carrying its epistemic symbol (●/■/▲/◌/◇) and a texture class
  (`textureForLabel`: solid/outline/dotted/hatched) so the four kinds never
  rely on colour alone.
- **StatePanel / ScenarioPanel / ValueTile** — every value routes through
  `formatValue`/`formatWindDirection`/`formatAirQualityCategory` (locale +
  unit aware) and `Label` (locale-aware epistemic word). `ValueTile`'s
  `displayValue` prop overrides the default unit-suffix rendering for wind
  direction's cardinal+degree pair.
- **ProvenancePanel** — field *labels* are translated; the provenance data
  itself (ids, ISO timestamps, licence identifiers) is not.
- **Skeleton / LoadingStatus** — shimmering placeholder blocks
  (`aria-hidden`) plus a `role="status"` live-region announcement; frozen
  (no shimmer) under reduced motion.
- **NetworkErrorPanel** — "Live data is unreachable right now." + the raw
  reason in small mono text + a Retry button (`role="alert"`). Replaces the
  old bare "Could not load CORDVBA: <message>" line.
- **Footer** — per-source attribution + age, plus the new required
  OpenFreeMap/OpenMapTiles/OpenStreetMap credit line.

## 5. Motion System

- Curve: `--ease-standard: cubic-bezier(0.2, 0, 0, 1)`.
- Durations: `--duration-fast` (150ms, focus/hover), `--duration-base`
  (300ms, panel transitions), `--duration-slow` (900ms, skeleton shimmer /
  beacon pulse period).
- **`prefers-reduced-motion: reduce` strategy:** all three duration tokens
  collapse to `0ms` in one `@media` block in `tokens.css` — a component
  that reads `var(--duration-*)` gets reduced motion for free. Two effects
  need an explicit code check because CSS variables can't stop a
  `requestAnimationFrame` loop: `WindParticles` renders exactly one static
  frame and never starts its rAF loop; `.beacon__ring`'s `animation` is set
  to `none` directly (a CSS keyframe animation, not a transition, so the
  duration-token trick alone doesn't reach it).
- Nothing animates without a stated reason. The beacon pulse means "this is
  live"; the skeleton shimmer means "this is loading"; the wind particles
  mean "this is what the wind is doing." No entrance animation exists for
  its own sake.

## 6. Accessibility Baselines

- **WCAG level:** AA (2.0/2.1 A+AA verified via axe-core, 0 violations —
  see the audit block in the PR/report).
- **Contrast:** every text/background pair in `tokens.css` targets ≥4.5:1
  (verified with `contrast.py`, see audit block); AQ category colours are
  a hint only, never load-bearing for comprehension.
- **Focus:** a single `--color-focus-ring` (`#7dd3fc`), 3px outline, 2px
  offset, on every interactive element (`button:focus-visible`,
  `input:focus-visible`, `a:focus-visible`) — no component defines its own.
- **Keyboard:**
  - The locale switch, timeline points, value tiles, scenario sliders and
    the provenance close button are all real `<button>`/`<input>`
    elements — keyboard-native, no custom `tabindex` choreography needed.
  - MapLibre's own canvas ships `tabindex="0"` and arrow-key pan built in.
  - Verified: Tab reaches the ES/EN switch; Enter toggles `aria-pressed`
    and the whole page's language, without a reload.
- **Colour independence (AC-2):** epistemic labels (texture + symbol +
  word), AQ categories (shape + word), timeline points (texture + symbol)
  — every one of the app's status encodings has a second, non-colour
  channel.
- **Dark-only, always:** `color-scheme: dark` (a single value, not `light
  dark`) plus explicit, opaque backgrounds on `html`/`body`/`#root` and on
  every surface/marker/popup. Verified with
  `scripts/check-color-scheme-isolation.mjs`: identical, opaque background
  whether the browser reports `colorScheme: 'dark'` or `'light'`.

## 7. Anti-patterns (banned here, and why)

- **An interpolated air-quality surface.** Three stations cannot support
  one; it would manufacture false precision between them (issue #102,
  reaffirmed #119 AC-2).
- **Colour as the only encoding for status.** Every status channel in this
  app (epistemic label, AQ category, timeline kind) also carries a
  distinct shape/texture/word.
- **A raw contract unit code on screen** (`Cel`, `deg`, `ica`…). These are
  wire-format tokens for machines; `formatValue`/`formatWindDirection`/
  `formatAirQualityCategory` are the only place a number becomes text.
- **The literal string "null"** where a value is genuinely absent — always
  an em dash.
- **`+nullh`** on an observed timeline point (the historical bug this issue
  fixed): `pointLabel` checks `typeof horizon_h === "number"`, not
  `!== undefined`, because real JSON can carry an explicit `null` even
  where the TypeScript type says it cannot.
- **Inheriting the browser/OS colour scheme.** This app has exactly one
  appearance. See §6.
- **A blank page while loading, or a bare `Error: <message>` line on
  failure.** Always a skeleton or a named, actionable error state.
- **Purple-gradient glassmorphism**, decorative motion with no stated
  reason, or any accent colour introduced without updating this file.
- **Un-cited machine-token drift into UI copy.** A dictionary key exists
  for every visible string (issue #124); a component must not fall back to
  a bare English literal.

## 8. Decision Log

- **2026-09-26 — Direction: "Void Signal" (design-shotgun, 3 directions
  considered).** *Void Signal* (chosen): void-black surfaces, one luminous
  teal accent reserved for live signals, texture-first status encoding.
  *Tactical Console* (rejected): flatter, more utilitarian/grid-heavy;
  read as generic ops-dashboard chrome rather than "artistic city twin,"
  under-delivering on the issue's explicit ask. *Aurora Drift* (rejected):
  two accent hues plus soft gradients behind panels; violates the
  anti-slop rule of one accent used with restraint, and risked confusing
  "predicted" (teal) against a second decorative hue.
- **2026-09-26 — MapLibre GL + OpenFreeMap, Leaflet removed.** Real 3D
  building extrusions (`render_height`/`render_min_height`, ~42% of
  Córdoba's OSM buildings have levels; the rest use a default height and
  are deliberately muted in `darkStyle.ts`'s colour ramp) are only
  available from vector tiles; Leaflet/raster OSM tiles cannot render
  them. `OPENFREEMAP_STYLE_URL` is the single swap point if OpenFreeMap's
  no-SLA public service needs replacing with a self-hosted Protomaps
  PMTiles style later.
- **2026-09-26 — Dark-style transform: generic lightness inversion +
  curated overrides, not a hand-authored style.** OpenFreeMap's `liberty`
  style is uniformly light, so inverting every colour's HSL lightness
  (`src/map/color.ts`) turns nearly all 111 layers dark in one pass;
  background, water, the 3D buildings and city labels get an exact,
  curated colour on top (`overridePaint` in `darkStyle.ts`). Trade-off:
  a handful of minor layers (road casings, some POI icons) may read as
  merely "inverted" rather than hand-tuned — acceptable for a v0.1 within
  the time available; a fully bespoke style is future work.
- **2026-09-26 — Typography: system font stack + `tabular-nums`, not
  bundled Inter/IBM Plex.** IBM Plex Sans + IBM Plex Mono is the named,
  confirmed-licensable pairing (SIL OFL 1.1, self-hostable) and is listed
  first in the font stack so a visitor who already has it gets it, but
  this change does not bundle the font binaries. Reason: scope/time
  budget for this change, and avoiding an unrequested asset-pipeline
  addition (licence files, font-loading strategy, FOUT handling).
  `font-variant-numeric: tabular-nums` gets tabular digits from any
  fallback system font without a download or a third-party request. This
  is a documented gap, not a silent one: self-hosting the named pairing is
  the natural next step if the maintainer wants it.
- **2026-09-26 — Wind arrow rotation kept as issue #102's original
  convention.** The arrow rotates directly to the reported degree value
  (no 180° flip). Meteorological arrow conventions vary by source and the
  brief did not ask for a change here; only the missing cardinal-direction
  *label* was a named defect (issue #119).
- **2026-09-26 — Beacon shape ramp: circle (good) → star (extremely
  poor).** Rounder = softer = better; more vertices = sharper = worse. An
  intuitive, colour-independent severity gradient (AC-2), rather than six
  arbitrary shapes.
- **2026-09-26 — Resilient network states (maintainer report,
  browser-verified bug).** Replaced the original `Promise.all` + single
  `error`/`null` state (fails all three resources together, shows a raw
  `Error: <message>` string) with `useEnvironmentTwin`: three independent
  resource states (loading/success/error) so one endpoint being down never
  blanks data that arrived fine, a shimmering skeleton instead of a blank
  page, a styled `NetworkErrorPanel` with a Retry button, and automatic
  retry with 2s/4s/8s backoff before giving up and waiting for a manual
  retry.
- **2026-09-26 — Dark-only regardless of the browser/OS theme
  (maintainer report, browser-verified bug).** `color-scheme: dark`
  (single value) plus explicit opaque backgrounds on `html`/`body`/`#root`
  and every surface, replacing `color-scheme: light dark` and implicit/
  inherited colours. Verified with
  `scripts/check-color-scheme-isolation.mjs` against both emulated
  browser themes.
- **2026-09-26 — i18n: ES default, EN on demand, `ff_locale` shared with
  fullfran.com (issue #124).** Mirrors the host site's own
  `App.tsx`/`Layout.tsx` pattern exactly (same key, same
  `Locale = 'es' | 'en'`, same segmented control shape with
  `aria-pressed`) because the page is served under
  `www.fullfran.com/cordvba/` and the two should feel like one property.
  `useLocale()` outside a `<LocaleProvider>` defaults to English rather
  than throwing, specifically so the many component tests written before
  #124 existed keep passing unwrapped — documented trade-off, not an
  oversight.
- **2026-09-26 — Palette alignment with the host site: background
  adopted exactly, accent deliberately not.** Superseded by the entry
  below: on maintainer re-review, `--color-bg`/`--color-bg-elevated`/
  `--color-border*` were re-derived to the host site's (fullfran.com)
  exact Tokyo Night values (`#1a1b26` background) for real pixel-level
  continuity across `www.fullfran.com/cordvba/`, not just "near-identical
  in spirit". Every text/background contrast pair was re-verified against
  the new base (`contrast.py`; all still pass AA, one non-blocking AAA
  miss on muted text, same as before the change). `#7aa2f7` (the host's
  accent) is still **not** reused as CORDVBA's data-accent colour: it
  would collide with `--color-accent` (teal), the one hue this app
  reserves exclusively for live map/timeline signals (Von Restorff — one
  accent, used with restraint). The locale switch already reused the
  host's exact control shape/interaction pattern (a separate, earlier
  decision).
- **2026-09-26 — Full-bleed HUD layout, not "map in a box above a grid of
  cards" (maintainer review).** The single-column, `max-width: 64rem`
  page (§3, superseded) read as a form, not a city twin. Replaced with a
  full-bleed map (`.app__stage`, `position: fixed; inset: 0` at ≥48rem)
  and a restrained HUD of opaque token-surface panels
  (`.app__panel--*`) — never glassmorphism/blur, which the output
  discipline (§7) bans unless recorded as a decision, and it is not.
  Below 48rem the map becomes a static 60vh block and every panel reverts
  to normal document flow, per the reviewer's explicit mobile spec. The
  scenario panel — the one HUD element large enough to compete with the
  map — became a closed-by-default collapsible drawer under the state
  dock rather than a permanently docked panel, to keep the default view
  restrained; the feature is one tap away, not removed.
- **2026-09-26 — Basemap noise reduction: an explicit suppress/mute list,
  not "recolour everything and hope" (maintainer review).**
  `darkStyle.ts`'s `SUPPRESS_IDS` (POI icons, transit-stop icons, minor
  place/road labels, one-way arrows: `icon-opacity`/`text-opacity` forced
  to `0`) and `MUTE_IDS` (kept landmark/major-road/district labels, at
  55% opacity: prominent enough to orient by, muted enough not to compete
  with a beacon) are named allow/deny lists rather than a zoom-level or
  layer-type heuristic, so which basemap elements are noise is a
  reviewable, explicit decision instead of an emergent side effect.
- **2026-09-26 — Beacons are pills, not fixed-size shapes (maintainer
  review: "untruncated labels").** The first pass made the *entire*
  beacon body a category-shaped silhouette (a 44px circle, square,
  triangle…) with the index/category text packed inside it — "extremely
  poor"/"desfavorable" (12+ characters) never fit that at a legible size
  and rendered visibly cramped/overlapping. Split the two jobs apart: a
  `.beacon__shape` glyph (a small solid icon, still circle→star,
  softest→sharpest) carries the colour-independent shape encoding
  (AC-2), and the pill around it is sized by its own text content instead
  of by the shape.
- **2026-09-26 — Camera framing: `fitBounds` over real station
  coordinates, never a fixed center/zoom (maintainer review).** A fixed
  camera constant risked framing the wrong area entirely against the live
  API's real station coordinates (confirmed to differ in shape only
  slightly from the fixtures, see below) or a future station reshuffle.
  `MapView.tsx` always fits the three city stations' actual coordinates
  plus the historic centre, with padding wide enough to clear the HUD
  panels (`FIT_PADDING`), so this holds regardless of which stations the
  API currently reports.
- **2026-09-26 — Airport beacon: an edge-clamped arrow + distance when
  off-screen, not a silently vanished marker (maintainer review).** The
  airport is far enough from the three city stations that fitting all
  four in one frame would zoom out past the point building extrusions
  are visible at all. Kept the tight station-framed camera and added
  `src/map/geo.ts`'s `clampToEdge` (a standard "off-screen radar arrow"
  construction): when `map.project()` places the airport beacon outside
  the viewport, a small arrow rotated toward its true bearing plus its
  distance (`haversineDistanceKm`) takes over, positioned via a plain
  DOM element (not a MapLibre `Marker`, which cannot be clamped) updated
  on the map's `move`/`resize` events.
- **2026-09-26 — Real-data check: fixtures + a direct curl of the live
  API, not a browser call from localhost (maintainer review, "do not
  change the API").** `https://cordvba.fullfran.com/api`'s CORS policy
  returns 400 on a preflight from an arbitrary localhost origin (verified
  with `curl -X OPTIONS`), so a real browser `pnpm preview` session
  cannot call it directly — expected, and out of scope to change per the
  instruction. Verified instead with direct `curl` against
  `/v1/environment` and `/v1/sources` (both 200, real MITECO/METAR data)
  and confirmed the fixture-mode UI end to end. One finding worth
  recording: the live payload sends `"category": null, "due_to": null,
  "horizon_h": null` explicitly on ordinary (non-timeline) `Value`
  objects — confirming, on real data, that `pointLabel`'s
  `typeof horizon_h === "number"` guard (issue #119's original "+nullh"
  fix) is not a hypothetical edge case but the API's actual behaviour.
