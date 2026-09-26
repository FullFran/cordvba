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
 * A timeline point's display label: its forecast horizon when it has one,
 * otherwise its time of day. `horizon_h` is only ever a number on the wire
 * (TypeScript says so), but real JSON has handed us an explicit `null`
 * before, which used to render as the literal "+nullh" (issue #119) — so
 * this checks the runtime type instead of trusting `!== undefined`.
 */
export function pointLabel(point: TimelinePoint): string {
  if (typeof point.horizon_h === "number") {
    return `+${point.horizon_h}h`;
  }
  return point.at.slice(11, 16); // HH:MM
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
