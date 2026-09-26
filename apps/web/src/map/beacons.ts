import { DICTIONARIES } from "../i18n/dictionaries";
import { formatTemplate } from "../i18n/template";
import type { Locale } from "../i18n/types";
import { formatAirQualityCategory, formatValue, formatWindDirection } from "../lib/format";
import { windBearingTo } from "./wind";

/**
 * A shape per ICA category, softest (a circle) for "good" and sharpest (a
 * star) for "extremely poor" — so severity reads even without colour
 * (issue #119, AC-2). Applied as a CSS class; the actual clip-path lives
 * in styles.css next to the rest of `.beacon`.
 */
export type BeaconShape = "circle" | "rounded-square" | "square" | "triangle" | "diamond" | "star";

const SHAPES: Record<string, BeaconShape> = {
  good: "circle",
  fair: "rounded-square",
  moderate: "square",
  poor: "triangle",
  very_poor: "diamond",
  extremely_poor: "star",
};

export function categoryShape(category: string): BeaconShape {
  return SHAPES[category] ?? "circle";
}

/**
 * The air-quality beacon's breathing-ring animation duration, in
 * milliseconds (issue #140): slower for good air, faster for poor —
 * "the air is calm" vs. "the air is agitated," a second, colour-independent
 * severity channel alongside `categoryShape` and the category/index text.
 * An unrecognised category falls back to the "moderate" rate rather than
 * throwing or breathing at 0ms.
 */
const BREATHING_DURATION_MS: Record<string, number> = {
  good: 3600,
  fair: 2800,
  moderate: 2200,
  poor: 1700,
  very_poor: 1300,
  extremely_poor: 900,
};

export function breathingDurationMs(category: string): number {
  return BREATHING_DURATION_MS[category] ?? BREATHING_DURATION_MS["moderate"]!;
}

/**
 * The wind arrow's on-screen rotation, in degrees (issue #138 — supersedes
 * issue #102's original "rotate straight to the reported degree" decision,
 * see DESIGN.md's decision log): it must point where the wind blows TO,
 * not the meteorological direction it is reported FROM, and — since the
 * arrow is a plain DOM element in fixed screen space, not something
 * MapLibre rotates with the map canvas — the map's own current bearing has
 * to be subtracted back out, or rotating the map would silently rotate
 * the arrow's *meaning* along with it. `null` (no reading) stays a neutral
 * 0°, regardless of bearing: there is no direction to correct.
 */
export function windArrowRotation(deg: number | null, mapBearingDeg: number = 0): number {
  if (deg === null) {
    return 0;
  }
  return ((windBearingTo(deg) - mapBearingDeg) % 360 + 360) % 360;
}

export interface AirQualityBeaconSpec {
  kind: "air-quality";
  name: string;
  category: string;
  index: number | string | null;
  highlighted: boolean;
  /** The pollutant responsible for the current index (issue #140), e.g. "O3", "NO2" — the contract's `due_to`. Shown as text, not just implied by colour/category. */
  pollutant?: string;
}

export interface WindBeaconSpec {
  kind: "wind";
  name: string;
  directionDeg: number | null;
  speedMs: number | string | null;
  /** The map's current bearing (issue #138), so the arrow — a plain DOM element, not map-rotated — can be corrected to still point the right way on screen. Defaults to 0. */
  mapBearingDeg?: number;
}

export type BeaconSpec = AirQualityBeaconSpec | WindBeaconSpec;

function el(tag: string, className: string, text?: string): HTMLElement {
  const node = document.createElement(tag);
  node.className = className;
  if (text !== undefined) {
    node.textContent = text;
  }
  return node;
}

/**
 * Builds the DOM element for one map beacon (issue #119): a glow + rings
 * (CSS, `.beacon__ring`) around a shape distinct per category, with the
 * category and value as visible text — never colour alone (AC-2). Used as
 * a MapLibre `Marker`'s HTML element, so it stays a plain DOM builder
 * rather than a React component (MapLibre owns this element's lifecycle,
 * not React's). Locale-aware (issue #124): defaults to English so every
 * pre-existing call site/test keeps working unchanged.
 */
export function createBeaconElement(spec: BeaconSpec, locale: Locale = "en"): HTMLElement {
  const t = DICTIONARIES[locale];
  const root = el("div", "beacon");
  const innerRing = el("span", "beacon__ring");
  const outerRing = el("span", "beacon__ring beacon__ring--outer");
  root.appendChild(innerRing);
  root.appendChild(outerRing);

  if (spec.kind === "air-quality") {
    const shape = categoryShape(spec.category);
    // A pill, not a fixed small circle (issue #119, maintainer review:
    // "untruncated labels"): "extremely poor"/"desfavorable" never fits a
    // ~44px circle at a legible size. The severity shape lives in its own
    // small `.beacon__shape` glyph; the pill itself is sized by its text.
    root.classList.add(`beacon--${spec.category}`);
    if (spec.highlighted) {
      root.classList.add("beacon--highlighted");
    }
    // The breathing ring (issue #140): slower for good air, faster for
    // poor — a third, colour-independent severity channel. Set here as an
    // inline longhand rather than the `.beacon__ring` stylesheet rule's
    // `animation` shorthand, so `prefers-reduced-motion`'s `animation: none`
    // override (same stylesheet, a higher-specificity media query) still
    // wins outright — this only ever overrides the *duration*, never
    // whether it animates at all.
    const durationMs = `${breathingDurationMs(spec.category)}ms`;
    innerRing.style.animationDuration = durationMs;
    outerRing.style.animationDuration = durationMs;

    const categoryWord = formatAirQualityCategory(spec.category, locale);
    const indexText = spec.index === null ? t.map.unknownIndex : String(spec.index);
    root.setAttribute(
      "aria-label",
      spec.pollutant
        ? formatTemplate(t.map.beaconAirQualityWithPollutant, {
            name: spec.name,
            category: categoryWord,
            index: indexText,
            pollutant: spec.pollutant,
          })
        : formatTemplate(t.map.beaconAirQuality, { name: spec.name, category: categoryWord, index: indexText }),
    );

    root.appendChild(el("span", `beacon__shape beacon__shape--${shape}`));
    const core = el("span", "beacon__core");
    core.appendChild(el("span", "beacon__index", spec.index === null ? "—" : String(spec.index)));
    core.appendChild(el("span", "beacon__category", categoryWord));
    if (spec.pollutant) {
      core.appendChild(el("span", "beacon__pollutant", spec.pollutant));
    }
    root.appendChild(core);
    return root;
  }

  root.classList.add("beacon--wind");
  root.setAttribute(
    "aria-label",
    formatTemplate(t.map.beaconWind, {
      name: spec.name,
      direction: formatWindDirection(spec.directionDeg, locale),
      speed: formatValue(spec.speedMs, "m/s", locale),
    }),
  );

  const arrow = el("span", "beacon__arrow", "↑");
  arrow.style.transform = `rotate(${windArrowRotation(spec.directionDeg, spec.mapBearingDeg ?? 0)}deg)`;
  root.appendChild(arrow);

  const core = el("span", "beacon__core beacon__core--wind");
  core.appendChild(el("span", "beacon__direction", formatWindDirection(spec.directionDeg, locale)));
  core.appendChild(el("span", "beacon__speed", formatValue(spec.speedMs, "m/s", locale)));
  root.appendChild(core);

  return root;
}

/**
 * The airport weather beacon, when it falls outside the visible map
 * viewport (issue #119, maintainer review): a small arrow clamped to the
 * viewport edge, rotated toward the true position, with its distance —
 * instead of the beacon simply disappearing off-screen.
 */
export function createEdgeIndicatorElement(
  name: string,
  distanceKm: number,
  angleDeg: number,
  locale: Locale = "en",
): HTMLElement {
  const t = DICTIONARIES[locale];
  const root = el("div", "beacon beacon--edge beacon--wind");
  const arrow = el("span", "beacon__arrow", "↑");
  arrow.style.transform = `rotate(${angleDeg + 90}deg)`; // arrow glyph points up (-90deg in atan2 terms); align it to the ray
  root.appendChild(arrow);
  root.appendChild(el("span", "beacon__edge-distance", `${Math.round(distanceKm)} km`));
  root.setAttribute(
    "aria-label",
    formatTemplate(t.map.edgeIndicator, { name, distance: Math.round(distanceKm) }),
  );
  return root;
}
