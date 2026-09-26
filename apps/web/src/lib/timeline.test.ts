import { describe, expect, it } from "vitest";

import { pointHorizonLabel, pointLabel, splitTimeline, textureForLabel } from "./timeline";
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

describe("pointLabel (polish: always the clock time — see pointHorizonLabel for the secondary '+Nh' text)", () => {
  // The historical "+1h before now/ahora" confusion: persistence horizons
  // anchor on the last ICA observation, which is published with a 1-2h
  // lag, so a "+1h" prediction's own `at` can still fall before `now` and
  // render in the timeline's *past* section — showing only "+1h" there,
  // with no clock time, read as nonsensical ("a future-sounding label in
  // the past list"). Always showing the clock time first fixes the
  // meaning regardless of which section it lands in; the horizon becomes
  // secondary, explanatory text instead of the only label.
  it("labels an observed point by its time of day, never '+nullh' (issue 119)", () => {
    expect(pointLabel(point("2026-09-26T08:00:00Z", "OBSERVED"))).toBe("08:00");
  });

  it("still guards against an explicit null horizon_h, which JSON can carry even though TypeScript cannot", () => {
    const withNullHorizon = {
      ...point("2026-09-26T08:00:00Z", "OBSERVED"),
      horizon_h: null,
    } as unknown as TimelinePoint;
    expect(pointLabel(withNullHorizon)).toBe("08:00");
  });

  it("labels a predicted point by its clock time too, not just its forecast horizon", () => {
    expect(pointLabel(point("2026-09-26T10:00:00Z", "PREDICTED", 1))).toBe("10:00");
  });

  it("labels a predicted point with horizon_h 0 by its clock time as well", () => {
    expect(pointLabel(point("2026-09-26T09:00:00Z", "PREDICTED", 0))).toBe("09:00");
  });
});

describe("pointHorizonLabel (polish: the forecast horizon as secondary text, alongside the clock time)", () => {
  it("returns null for an observed point (no horizon at all)", () => {
    expect(pointHorizonLabel(point("2026-09-26T08:00:00Z", "OBSERVED"))).toBeNull();
  });

  it("returns null for an explicit null horizon_h, same as a missing one", () => {
    const withNullHorizon = {
      ...point("2026-09-26T08:00:00Z", "OBSERVED"),
      horizon_h: null,
    } as unknown as TimelinePoint;
    expect(pointHorizonLabel(withNullHorizon)).toBeNull();
  });

  it("returns '+1h' for a predicted point one hour out", () => {
    expect(pointHorizonLabel(point("2026-09-26T10:00:00Z", "PREDICTED", 1))).toBe("+1h");
  });

  it("returns '+0h' for horizon_h 0, rather than treating it as falsy/missing", () => {
    expect(pointHorizonLabel(point("2026-09-26T09:00:00Z", "PREDICTED", 0))).toBe("+0h");
  });
});

describe("textureForLabel", () => {
  it("gives observed and published points a solid texture", () => {
    expect(textureForLabel("OBSERVED")).toBe("solid");
    expect(textureForLabel("PUBLISHED")).toBe("solid");
  });

  it("gives inferred points an outline texture", () => {
    expect(textureForLabel("INFERRED")).toBe("outline");
  });

  it("gives predicted points a dotted texture", () => {
    expect(textureForLabel("PREDICTED")).toBe("dotted");
  });

  it("gives simulated points a hatched texture", () => {
    expect(textureForLabel("SIMULATED")).toBe("hatched");
  });

  it("gives every label a distinct texture, so none rely on colour alone", () => {
    const labels = ["OBSERVED", "PUBLISHED", "INFERRED", "PREDICTED", "SIMULATED"] as const;
    const textures = new Set(labels.map(textureForLabel));
    // OBSERVED and PUBLISHED intentionally share "solid" (both are facts, not
    // model output); the other three each get their own texture.
    expect(textures.size).toBe(4);
  });
});
