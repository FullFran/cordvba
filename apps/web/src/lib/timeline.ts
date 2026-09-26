import type { TimelinePoint } from "../types/environment";

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
