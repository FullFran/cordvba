import { describe, expect, it } from "vitest";

import { splitTimeline } from "./timeline";
import type { TimelinePoint } from "../types/environment";

function point(at: string, label: TimelinePoint["label"], horizon_h?: number): TimelinePoint {
  return {
    value: 1,
    unit: "Cel",
    label,
    at,
    provenance: null,
    model: null,
    ...(horizon_h === undefined ? {} : { horizon_h }),
  };
}

describe("splitTimeline", () => {
  const now = "2026-09-26T09:40:00Z";

  it("sorts unordered points chronologically", () => {
    const points = [
      point("2026-09-26T09:00:00Z", "OBSERVED"),
      point("2026-09-26T08:00:00Z", "OBSERVED"),
      point("2026-09-26T12:00:00Z", "PREDICTED", 3),
      point("2026-09-26T10:00:00Z", "PREDICTED", 1),
    ];

    const { past, future } = splitTimeline(points, now);

    expect(past.map((p) => p.at)).toEqual(["2026-09-26T08:00:00Z", "2026-09-26T09:00:00Z"]);
    expect(future.map((p) => p.at)).toEqual(["2026-09-26T10:00:00Z", "2026-09-26T12:00:00Z"]);
  });

  it("splits strictly on now: past holds at <= now, future holds at > now", () => {
    const points = [
      point("2026-09-26T09:40:00Z", "OBSERVED"), // exactly now
      point("2026-09-26T09:41:00Z", "PREDICTED", 1),
      point("2026-09-26T09:39:00Z", "OBSERVED"),
    ];

    const { past, future } = splitTimeline(points, now);

    expect(past.map((p) => p.at)).toEqual(["2026-09-26T09:39:00Z", "2026-09-26T09:40:00Z"]);
    expect(future.map((p) => p.at)).toEqual(["2026-09-26T09:41:00Z"]);
  });

  it("returns empty arrays for an empty series without throwing", () => {
    expect(splitTimeline([], now)).toEqual({ past: [], future: [] });
  });
});
