import { DICTIONARIES } from "../i18n/dictionaries";
import { formatTemplate } from "../i18n/template";
import type { Locale } from "../i18n/types";
import { formatAirQualityCategory, formatValue, formatWindDirection } from "../lib/format";

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

/** Wraps a wind-direction degree value into [0, 360); null (no reading) becomes 0. */
export function windArrowRotation(deg: number | null): number {
  if (deg === null) {
    return 0;
  }
  return ((deg % 360) + 360) % 360;
}

export interface AirQualityBeaconSpec {
  kind: "air-quality";
  name: string;
  category: string;
  index: number | string | null;
  highlighted: boolean;
}

export interface WindBeaconSpec {
  kind: "wind";
  name: string;
  directionDeg: number | null;
  speedMs: number | string | null;
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
  root.appendChild(el("span", "beacon__ring"));
  root.appendChild(el("span", "beacon__ring beacon__ring--outer"));

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
    const categoryWord = formatAirQualityCategory(spec.category, locale);
    root.setAttribute(
      "aria-label",
      formatTemplate(t.map.beaconAirQuality, {
        name: spec.name,
        category: categoryWord,
        index: spec.index === null ? t.map.unknownIndex : String(spec.index),
      }),
    );

    root.appendChild(el("span", `beacon__shape beacon__shape--${shape}`));
    const core = el("span", "beacon__core");
    core.appendChild(el("span", "beacon__index", spec.index === null ? "—" : String(spec.index)));
    core.appendChild(el("span", "beacon__category", categoryWord));
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
  arrow.style.transform = `rotate(${windArrowRotation(spec.directionDeg)}deg)`;
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
