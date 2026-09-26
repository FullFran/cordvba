import type { Label, TimelinePoint } from "../types/environment";

/**
 * The four visual textures the timeline scrubber and its ticks use to
 * distinguish epistemic labels without relying on colour alone (issue
 * #119): solid (a fact — observed or published), outline (inferred),
 * dotted (predicted) and hatched (simulated, cross-hatch pattern).
 */
export type Texture = "solid" | "outline" | "dotted" | "hatched";

const TEXTURES: Record<Label, Texture> = {
  OBSERVED: "solid",
  PUBLISHED: "solid",
  INFERRED: "outline",
  PREDICTED: "dotted",
  SIMULATED: "hatched",
};

export function textureForLabel(label: Label): Texture {
  return TEXTURES[label];
}

/**
 * A timeline point's primary display label: always its time of day (issue
 * #119/polish). Persistence horizons anchor on the last ICA observation,
 * which is published with a 1-2h lag, so a "+1h" prediction's own `at` can
 * still fall before `now` — landing it in the timeline's *past* section
 * while showing only a future-sounding "+1h", with no clock time to make
 * sense of it (the historical "+1h before now/ahora" confusion). Always
 * showing the clock time fixes the meaning regardless of which section a
 * point lands in; see `pointHorizonLabel` for the secondary "+Nh" text.
 */
export function pointLabel(point: TimelinePoint): string {
  return point.at.slice(11, 16); // HH:MM
}

/**
 * A timeline point's forecast horizon, as secondary text alongside
 * `pointLabel`'s clock time — `null` for a point with none. `horizon_h` is
 * only ever a number on the wire (TypeScript says so), but real JSON has
 * handed us an explicit `null` before, which used to render as the
 * literal "+nullh" (issue #119) — so this checks the runtime type instead
 * of trusting `!== undefined` or truthiness (`0` is a valid horizon).
 */
export function pointHorizonLabel(point: TimelinePoint): string | null {
  return typeof point.horizon_h === "number" ? `+${point.horizon_h}h` : null;
}

export interface TimelineSplit {
  past: TimelinePoint[];
  future: TimelinePoint[];
}

/**
 * Sorts a timeline series chronologically and splits it around `now`:
 * points at or before `now` are past (observed), points after `now` are
 * future (predicted). Used to render past ● / now / future ◌ on the
 * timeline (issue #102).
 */
export function splitTimeline(points: TimelinePoint[], now: string): TimelineSplit {
  const sorted = [...points].sort((a, b) => a.at.localeCompare(b.at));
  const nowTime = new Date(now).getTime();

  const past: TimelinePoint[] = [];
  const future: TimelinePoint[] = [];

  for (const point of sorted) {
    if (new Date(point.at).getTime() <= nowTime) {
      past.push(point);
    } else {
      future.push(point);
    }
  }

  return { past, future };
}
