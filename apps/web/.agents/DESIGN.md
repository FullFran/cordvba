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
| `--map-bg`, `--map-water`, `--map-building-low/-high`, `--map-label-halo` | dark navy/green/grey | The MapLibre style's curated overrides (`src/map/darkStyle.ts`); `--map-bg`/`--map-water` double as the **night** phase of the sun-driven palette below |
| `--map-road-minor/-mid/-major`, `--map-road-label` | neutral slate (hue ~220) | Roads/bridges/tunnels/aeroways and their labels — deliberately *not* the basemap's original warm amber, which shared a hue family with the moderate/poor beacons (see Decision Log) |
| `--map-bg-day/-golden/-blue`, `--map-water-day/-golden/-blue` | cool slate / vivid warm amber-brown / deep indigo-blue | The sun-driven day/golden-hour/blue-hour base-map tint (issue #139, parent review: dramatic not faint): `sunPhase(altitude)` selects which of these `readMapPalette` reads; still the Void Signal dark aesthetic at every phase, never a light theme |
| `--map-light-day/-golden/-blue/-night` | `#cfe0f5` / `#ffb066` / `#7b93e0` / `#2a3550` | The extrusion light's colour per sun phase (`src/map/sunLight.ts`), paired with that module's own per-phase intensity (0.85/0.55/0.28/0.06) — read straight through with no literal in JS |
| `--map-shadow-fill` | `rgba(3, 6, 12, 0.4)` | The historic-centre building-shadow layer's translucent fill — a plain dark tint at every phase (a shadow is a shadow); not drawn once the sun is down (AC-3) |
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
  padding (`MapView.tsx`'s `resolveFitPadding`, `DESKTOP_FIT_PADDING` at
  this breakpoint) keeps the initial camera from ever framing a station
  behind these panels.
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
  breakpoint-aware padding (`resolveFitPadding`: the HUD-shaped
  `DESKTOP_FIT_PADDING` at >=768px, a small symmetric `MOBILE_FIT_PADDING`
  below it — see Decision Log's "the left beacon is cut" entry), rather
  than a fixed center/zoom.
  Beacons (`src/map/beacons.ts`) are pills, not fixed small circles (a
  category word like "extremely poor"/"desfavorable" never fit a ~44px
  circle): a glow + two pulsing rings (CSS, frozen under reduced motion),
  a small `.beacon__shape` severity icon (circle→star, softest→sharpest)
  separate from the text, and the index/category as literal text — never
  colour alone (AC-2). Each air-quality pill also names the pollutant
  responsible (issue #140, e.g. "O3"), and its two rings breathe at a
  category-driven rate (`breathingDurationMs`: slower for good air, faster
  for poor) — a fourth, colour-independent severity channel alongside
  shape/text/height. Real 3D "columns of light" (issue #140,
  `src/map/airQualityColumns.ts`), hero-scaled (parent review): height
  ~280m per ICA level (`icaColumnHeightM`, index 1-6 spans 280-1680m),
  130m base radius, `fill-extrusion-vertical-gradient` and 0.9 opacity so
  they are the frame's clear focal point, not a "tiny stub" — plus a
  soft breathing `circle-blur` glow at each base, animated at the same
  per-category rate as the beacon's own rings. One independent GeoJSON
  feature per station, never a shared/merged surface (the columns' own
  legend line states this explicitly; the three stations are hundreds of
  metres apart, so the discs never touch). No interpolated air-quality
  surface, ever. When the
  airport beacon's true position falls outside the viewport, a
  `clampToEdge`-positioned arrow + distance (`src/map/geo.ts`) replaces it
  instead of it silently vanishing. A caption states the two honesty notes
  verbatim ("Extrusion heights: OpenStreetMap, approximate…"; "Beacons
  show individual stations only — no interpolated surface") as a small
  overlay chip, not layout-height text. Sun-driven (issue #139): a
  `sun: SunPosition` prop (computed once in `App.tsx` from the timeline's
  selected time, `src/lib/sun.ts`'s `getSunPosition`) drives three effects
  — the extrusion light and background/water palette via
  `map.setPaintProperty`/`setLight` (`sunPhase` picks one of four dramatic
  phases — day, golden hour, blue hour, night, each a clearly distinct
  colour and intensity, parent review; `src/map/sunLight.ts` computes the
  light spec), a translucent
  `building-shadows` GeoJSON layer recomputed from currently rendered
  `building-3d` footprints projected along the sun's shadow vector
  (`src/map/shadows.ts`'s `buildingShadow`, a convex-hull approximation —
  see Decision Log), and the legend's shadow-honesty line (shown only
  while the sun is up). Shadows recompute on `moveend` (fires once per
  gesture, not per frame — the layer's throttle) and whenever the sun's
  position changes; hidden entirely once the sun is at or below the
  horizon (AC-3).
- **WindParticles** (`WindParticles.tsx`) — an illustrative canvas particle
  field over the map, driven by the single METAR reading. Particles are
  advected in longitude/latitude (`src/map/wind.ts`'s pure geographic
  math, issue #138) and only turned into a screen position through the
  live `map.project()`, so the field stays correct whatever the map's
  current bearing/pitch/pan/zoom is. Always shows its honesty label when
  on ("Illustrative wind: one station, not spatially varied"), toggleable,
  frozen to one static frame (which still re-projects on `move`, never
  re-advects) under `prefers-reduced-motion`.
- **TimelineView** (`TimelineView.tsx`) — past/now/future points as real
  `<button>`s (keyboard-native, unlike a custom slider widget would need to
  be), each carrying its epistemic symbol (●/■/▲/◌/◇) and a texture class
  (`textureForLabel`: solid/outline/dotted/hatched) so the four kinds never
  rely on colour alone. Its primary label is always the clock time
  (`pointLabel`); a forecast horizon (e.g. "+1h") is muted secondary text
  (`pointHorizonLabel`, `.timeline__point-horizon`) — polish fix for a
  "+1h before now" confusion caused by persistence horizons anchoring on a
  1-2h-lagged last observation.
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
- **SunWidget** (`SunWidget.tsx`, issue #139) — sunrise, solar noon,
  sunset (`src/lib/sun.ts`'s `getSunTimes`, shown in Córdoba's own
  `Europe/Madrid` local time regardless of the visitor's browser
  timezone — `formatClockTime`) and the sun's current altitude
  (`formatDegrees`, no space before `°`, matching `formatWindDirection`'s
  existing convention). Lives in the state dock, next to `StatePanel`; a
  polar day/night's missing sunrise/sunset renders as an em dash, never
  "Invalid Date".

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
- **Sun-driven map lighting (issue #139):** a third mechanism, alongside
  the two above, for a case neither covers — a MapLibre *style-level*
  paint-property transition (`style.transition`, set once at map creation
  in `MapView.tsx`), not a CSS variable or a `requestAnimationFrame` loop.
  Scrubbing the timeline calls `setPaintProperty`/`setLight`; this makes
  the sky/water/light ease between values (800ms, `--duration-slow`'s
  spirit) instead of snapping, and is set to `{ duration: 0 }` under
  `prefers-reduced-motion`, read once via `prefersReducedMotion()` at
  creation time (the same helper the other two mechanisms use).

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
  convention.** *(Superseded below, issue #138.)* The arrow rotates
  directly to the reported degree value (no 180° flip). Meteorological
  arrow conventions vary by source and the brief did not ask for a change
  here; only the missing cardinal-direction *label* was a named defect
  (issue #119).
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
- **2026-09-26 — Road network recoloured to neutral slate, verified
  against the WCAG relative-luminance formula, not eyeballed (second
  maintainer review).** OpenFreeMap's `liberty` style paints every road,
  bridge, tunnel and aeroway line/fill in warm amber — the generic
  lightness-inversion pass kept that hue, just darker, so the whole city
  still read as amber/orange and shared a hue family with the moderate/
  poor beacons. `darkStyle.ts`'s `roadColor()` gives roads an exact,
  tiered neutral-slate colour instead (hue ~220, matching
  `--color-border`): minor/mid/major tiers at relative luminance
  0.016/0.024/0.041 (`darkStyle.test.ts` asserts the major:minor ratio
  stays under 3x), comfortably under 25-30% of even the darkest beacon
  colour (`--color-aq-very-poor`, luminance ~0.33). A road-id-prefixed
  layer that is not actually a line/fill (`road_shield_us`, a US-highway
  shield icon; `symbol` type) is explicitly excluded from this treatment —
  a real regression caught in the browser (MapLibre rejected "unknown
  property line-color" on it) before this line landed, now covered by a
  fixture test.
- **2026-09-26 — Wind label + extrusion-heights caption merged into one
  legend block (second maintainer review).** Two separate absolutely-
  positioned boxes could go unnoticed (the wind label, easy to miss next
  to the animating canvas) or overlap each other (both competed for the
  map stage's bottom corners on a narrow viewport). `WindParticles.tsx`
  was reduced to just the canvas; the toggle state moved up into
  `MapView.tsx`, which now owns one `.map-legend` box holding the toggle,
  the wind label (shown only while the layer is on) and the caption
  (always shown) together. This in turn freed the map stage's bottom-left
  corner for `MapView`'s own attribution-panel needs — see the next
  entry.
- **2026-09-26 — Attribution moved from its own HUD corner into the
  state dock (second maintainer review).** Merging the wind/caption
  legend made that box taller, which then collided with the app-level
  Footer panel occupying the same bottom-left corner. Rather than hunt
  for a fifth free corner (there are only four, and the scrubber already
  claims the bottom edge), attribution now renders at the bottom of
  `.app__panel--state` (the state dock), below the Scenario toggle — it
  is low-priority "fine print" content, a natural fit for the bottom of
  an already-scrollable panel rather than its own dedicated screen
  region.
- **2026-09-26 — Attribution built client-side from id/publisher/licence,
  never the API's own `attribution` string (second maintainer review).**
  The contract's `Source.attribution` field is pre-formatted server-side
  in English only; showing it verbatim produced an English sentence
  ("Weather: NOAA...") inside an otherwise-Spanish page. `src/lib/
  attribution.ts`'s `formatSourceAttribution()` instead builds the line
  from `Source.id` (through `footer.sourceKinds`), `Source.publisher`
  (used as-is; it is already a short form like "MITECO", not translated)
  and `Source.licence` (through `footer.licences`), via
  `footer.attributionTemplate`. A source id or licence code with no
  dictionary entry falls back to the raw value (`Source.attribution` for
  an unknown id) rather than showing `undefined` — forward-compatible
  with a source the current dictionaries don't know about yet.
- **2026-09-26 — MapLibre's own zoom/compass control moved to
  bottom-right (second maintainer review).** It defaults into a corner
  MapLibre positions inside the map container; at "top-right" it was
  partly covered by the state-dock HUD panel occupying that same corner.
  Moved to `bottom-right` in `MapView.tsx`, with a
  `.maplibregl-ctrl-bottom-right` CSS offset lifting it above the bottom
  scrubber bar.
- **2026-09-26 — Pitch/bearing increased, and both building layers'
  zoom range lowered (second maintainer review: "3D is barely
  perceptible").** Pitch 55°→58°, bearing -17°→-35° (a bearing close to
  due north showed rooftops more than facades). Independently, since a
  `fitBounds` camera's resulting zoom depends on the live API's actual
  station spread (not just the fixture's), `building`/`building-3d`'s
  minzoom/maxzoom were lowered by two levels each
  (`MINZOOM_OVERRIDES`) so the historic centre still reads as a 3D
  skyline even if that zoom lands below OpenFreeMap's default cutover
  (14), rather than depending on a specific zoom value being true.
- **2026-09-26 — A pre-existing test flakiness fixed in passing:
  `ScenarioPanel.test.tsx`'s third test never passed a fixed `now`, so an
  unmocked wall-clock age (e.g. "4 h 26 min ago") could coincidentally
  contain the same digits as the fixture's "26 °C" observed temperature
  under a loose `/26/` regex match — caught when it actually flaked
  during this round's verification. Fixed with an explicit `now` and
  exact-string assertions ("26 °C", not `/26/`).
- **2026-09-26 — Wind particles and the wind arrow made geographic, not
  screen-space (issue #138, bug report; supersedes the entry above).**
  `WindParticles.tsx` moved particles in fixed canvas pixels computed once
  from the raw wind reading, so the whole field silently ignored the map's
  own -35° initial bearing and 58° pitch, and any rotation the visitor
  applied. Fixed at the source: `src/map/wind.ts` now advects particles in
  longitude/latitude (`geographicWindStep`, `wrapWithinBounds`,
  `advectParticle`), exactly like a real drifting parcel of air, and the
  component only ever turns a particle into a screen position through the
  live `map.project()` at draw time — the map's bearing/pitch/pan/zoom
  live entirely inside that one call, never duplicated by hand. The same
  fix retired issue #102's "rotate straight to the reported degree, no
  flip" convention for the airport's wind-arrow beacon: `windArrowRotation`
  now points the arrow where the wind blows *to* (`direction + 180°`) and
  subtracts the map's current bearing (`MapView.tsx` listens for `rotate`
  and recomputes it), since that arrow is a plain DOM element MapLibre
  never rotates with the map canvas on its own. Both particles and the
  arrow are covered by pure-function tests independent of a live WebGL
  context: `geographicWindStep`/`wrapWithinBounds` for the geometry, and a
  small bearing-aware `project()` mock at 0°/-35°/90° proving the
  geographic step never itself depends on bearing (`wind.test.ts`), plus
  `windArrowRotation`'s own bearing-corrected cases (`beacons.test.ts`).
- **2026-09-26 — Sun library: `suncalc` (BSD-2-Clause), not a hand-rolled
  or heavier alternative (issue #139).** `suncalc@2.0.2`: BSD-2-Clause
  (permissive, compatible with this project's MIT licence), a single
  ~15KB (~4.6KB gzipped) `suncalc.cjs` module with no dependencies of its
  own. Its `getPosition`/`getTimes` already return exactly the shapes this
  app needs (north-based-clockwise azimuth and refraction-corrected
  altitude in plain degrees; sunrise/solarNoon/sunset as `Date`s) — no
  radian or south-based conversion required, unlike its pre-2.0 API. A
  from-scratch NOAA-solar-position-algorithm implementation (Julian
  century ephemeris → equation of time → hour angle → zenith/azimuth,
  the same formulas behind NOAA's own solar calculator) was written
  independently to cross-check it: both agree to within 0.11° at three
  test times for Córdoba, comfortably inside AC-1's 1° budget
  (`sun.test.ts` bakes in the NOAA-side numbers as the reference).
- **2026-09-26 — Sun-driven palette: two tokens change (background,
  water), not the whole basemap (issue #139).** *(4-phase split and
  dramatic intensity superseded below, parent review.)* "A day/dusk/night
  palette for the base map" could have meant recolouring roads/buildings/
  labels too; scoped to just the sky/water tint (`--map-bg-*`/
  `--map-water-*`) so the change stays legible as "the light changed," not
  "the whole city changed colour," and so beacons/roads/labels keep their
  one already carefully-tuned contrast ratio (§8, "Road network
  recoloured…") regardless of time of day.
- **2026-09-26 — Four sun phases (day/golden/blue/night), not three
  (parent review: "the sun is barely noticeable... noon vs dusk differ by
  a faint tint. Make the lighting dramatic").** The original 3-way
  day/dusk/night split (dusk = within ±6° of the horizon) folded two
  visually distinct states — the warm, low-angle light before sunset/
  after sunrise, and the cool light straddling the horizon itself — into
  one blurred "dusk," and a flat intensity floor (0.15) that never dimmed
  further meant "night" only ever read as a slightly darker "day."
  `sunPhase` (`src/lib/sun.ts`) now names both halves of that range with
  photography's own vocabulary: **golden hour** (2°–20° altitude, warm
  amber) and **blue hour** (-6°–2°, cool blue) — genuinely different
  tokens (`--map-bg-golden`/`--map-water-golden` vs `--map-bg-blue`/
  `--map-water-blue`, both new) and genuinely different extrusion-light
  colours (`--map-light-golden: #ffb066` vs `--map-light-blue: #7b93e0`).
  `sunLight`'s intensity is now driven primarily by phase, not a smooth
  `sin(altitude)` curve with a shared floor: day 0.85 (near MapLibre's
  own maximum), golden 0.55, blue 0.28, night 0.06 (never exactly zero,
  so extrusions never go fully flat) — four clearly ordered, clearly
  different steps, each pinned by a test. Still a categorical read, not a
  continuous interpolation (simpler, testable in four fixed cases): the
  map style's own `transition` (800ms, zeroed under
  `prefers-reduced-motion`) already eases every discrete jump, including
  `setLight` calls, so scrubbing the timeline across a phase boundary
  still reads as smooth motion, not a hard cut. Real building faces
  (both the AQ columns and the city's own extruded roofs) visibly
  re-shade with every phase change, since MapLibre's lighting model
  shades every `fill-extrusion` layer from the same `light.color`/
  `intensity`/`position` — no per-layer colour hack needed.
- **2026-09-26 — Building shadows: convex hull of footprint ∪
  shadow-cast translation, not a full silhouette sweep (issue #139).**
  `shadows.ts`'s `buildingShadow` translates every footprint vertex by
  `height / tan(altitude)` along the anti-solar bearing and takes the
  convex hull of the original and translated vertices. Exact for a convex
  footprint (most OSM building footprints); a defensible, cheap
  over-approximation for a concave one, instead of classifying which
  edges are sun-facing (a full extruded-polygon shadow silhouette) —
  named as a trade-off, not hidden, and consistent with the layer's own
  INFERRED label and legend line. Shadow length is clamped
  (`DEFAULT_MAX_SHADOW_LENGTH_M`, 400m) so a near-horizon sun does not
  draw an absurd shadow across the whole map.
- **2026-09-26 — Shadow recompute: throttled by `moveend`, on the main
  thread, not a Web Worker (issue #139, scope/time trade-off, named per
  the issue's own "throttled *or* in a worker" wording).** MapLibre's
  `moveend` event already fires once per camera gesture rather than per
  frame, which is exactly the throttle this layer needs (`query
  RenderedFeatures` + a convex hull per building is not something to run
  60 times a second); a fixed-interval debounce on top would only add
  latency without a measured benefit. `performance.mark`/`measure`
  (`cordvba-shadow-compute`) brackets each recompute so its cost is
  reported (see the PR/report) rather than assumed. A Web Worker would
  need MapLibre's queried GeoJSON features serialised across the worker
  boundary for a computation already cheap enough on the main thread at
  this scale (see the report's timing numbers) — real future work if the
  historic centre's building count grows enough to matter, not a
  currently-measured problem.
- **2026-09-26 — Air quality as columns of light: real 3D extrusion, not
  a taller pill (issue #140, maintainer: "a more artistic representation
  using the 3D city it sits in").** A `fill-extrusion` GeoJSON layer
  (`src/map/airQualityColumns.ts`) sits alongside the existing DOM-marker
  beacon (kept, not replaced): the beacon still carries the accessible
  text/aria-label and now also the pollutant and a category-rate breathing
  ring, while the column is the genuinely new, literally-3D "height =
  index" encoding the issue asked for. Rejected: replacing the beacon
  outright — the existing pill is exactly the kind of "glow + rings +
  shape + text" treatment this file already documents at length (§4,
  Decision Log), and duplicating that work under a new name would have
  cost far more than it added; the column is additive value, not a
  redesign.
- **2026-09-26 — Column footprint: a 16-gon disc via `destinationPoint`,
  not a MapLibre `circle` layer (issue #140).** *(Radius superseded
  below, parent review.)* `circle` layers are always 2D (screen-space
  radius, no `fill-extrusion-height`); a real extruded "column" needs an
  actual small `Polygon` footprint in the source data. `discPolygonCoordinates`
  reuses `geo.ts`'s `destinationPoint` (the same geodesic-offset helper
  issue #139's shadows already introduced) around each station, 16
  vertices — plenty round at the zoom this app frames stations at.
- **2026-09-26 — Column hero scale: ~280m per ICA level, 130m radius, not
  the original ~18-43m/14m "stub" scale (parent review: "tiny orange
  stubs at the fitted zoom").** The original scale was calibrated against
  nothing — it drew *a* column, but a real city block's buildings
  (`--map-building-high`, tens of metres) already out-sized it, so at the
  app's initial `fitBounds` camera (station-framed, zoom ~14) it read as
  a barely-visible sliver rather than the "hero" element the issue asked
  for. `icaColumnHeightM`'s `HEIGHT_PER_INDEX_M` is now 280 (within the
  requested 250-300m band; index 1-6 spans 280-1680m) and
  `AQ_COLUMN_RADIUS_M` is 130 (within the requested 100-150m band, still
  far short of the ~300-600m gaps between real stations, so the three
  discs never touch — DESIGN.md §7's standing "no interpolated surface"
  rule is a geometric fact here, not just a stated intent). Both are
  pinned by a test (`airQualityColumns.test.ts`) asserting the exact
  requested range, so this cannot silently shrink back to invisible.
  `fill-extrusion-vertical-gradient: true` and a raised
  `fill-extrusion-opacity` (0.75 → 0.9) make the taller shape actually
  read as a lit column (darker base, bright top) instead of a flat
  coloured slab.
- **2026-09-26 — Breathing glow: a `circle` layer with `circle-blur`,
  animated by rewriting its own source data every frame, not a
  zoom-expression trick (issue #140 AC, parent review).** MapLibre's
  style expressions forbid using `["zoom"]` as a sub-expression of
  arithmetic operators like `*`, which would otherwise be the natural way
  to scale a data-driven radius by a per-frame "breathing" factor. Three
  stations is cheap enough to just recompute `opacity`/`radiusScale` per
  feature every `requestAnimationFrame` tick and call the glow source's
  own `setData` — the same per-category rate as the DOM beacon's
  `breathingDurationMs` (slower for good air, faster for poor), so both
  readings of "how urgent is this" agree. Frozen to one calm, static
  mid-opacity frame under `prefers-reduced-motion` (no rAF loop started
  at all), matching every other JS-driven animation in this app.
  Drawn *below* the extrusion layer (`map.addLayer(glow, beforeId:
  columns)`) so it reads as a pool of light the column rises out of, not
  a halo painted over its face.
- **2026-09-26 — Column/pill alignment: verified by test, not "fixed" —
  the original "stubs look offset from the pills" was the old scale
  being too small to judge by eye, not a coordinate bug (parent review).**
  `stationColumnFeature`/`stationGlowFeature`/the beacon `Marker` all read
  the identical `station.lat`/`station.lon` — a new test
  (`airQualityColumns.test.ts`) computes the disc's own vertex centroid
  and asserts it lands exactly on the station coordinate. At hero scale
  the column's base now visibly meets its beacon pill (see the PR/report
  screenshots); the remaining, expected effect is ordinary 3D
  perspective at a 58° pitch — a *very* tall column's lit top leans back
  on screen relative to its ground-anchored base and label, the same way
  any real extruded skyscraper would at this camera angle. This was not
  "fixed" by moving the label to a computed top-of-column screen
  position: that needs projecting a 3D point through MapLibre's camera
  transform, which the public API does not expose, and an approximate
  pixel-lift heuristic would drift out of sync the moment pitch, bearing
  or zoom changes (e.g. the new historic-centre camera preset, #139/#140
  follow-up). Documented as a known, camera-angle-dependent optical
  effect rather than silently papered over.
- **2026-09-26 — Timeline label: always the clock time, horizon secondary
  (polish, maintainer report: "+1h shows before now/ahora").** Persistence
  horizons anchor on the last ICA observation, published with a 1-2h lag,
  so a "+1h" prediction's own `at` can still fall before `now` — landing
  it in the timeline's *past* section while showing only "+1h", with no
  clock time to make sense of it there. `pointLabel` now always returns
  the clock time; the horizon moves to a new `pointHorizonLabel`, rendered
  as muted secondary text (`.timeline__point-horizon`) alongside it. Fixes
  the meaning regardless of which section a point lands in, without
  touching `splitTimeline`'s past/future split itself — that split (at
  <= now is past) was already correct; only the label was confusing.
- **2026-09-26 — Mobile `fitBounds` padding: small and symmetric below
  768px, not the desktop HUD padding (polish, maintainer report: "the left
  beacon is cut at the screen edge").** The desktop padding's right: 340
  accounts for the fixed-position state dock, which only exists at
  >=48rem (DESIGN.md §3); below that the HUD reverts to normal document
  flow above/below the map, so applying that same padding on a 390px-wide
  viewport left `fitBounds` under 2px of horizontal room, which zoomed out
  to fit the whole country instead of Córdoba — a beacon "cut at the
  edge" was actually the mild end of a much larger framing bug.
  `resolveFitPadding(viewportWidthPx)` (`MapView.tsx`) picks between
  `DESKTOP_FIT_PADDING` (unchanged) and a new small `MOBILE_FIT_PADDING`
  (40/40/24/24) at the same 768px breakpoint every other layout rule in
  this app already uses.
