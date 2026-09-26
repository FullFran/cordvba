import { describe, expect, it } from "vitest";

import { pointLabel, splitTimeline, textureForLabel } from "./timeline";
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

describe("pointLabel", () => {
  it("labels an observed point (no horizon_h) by its time of day, never '+nullh' (issue 119)", () => {
    expect(pointLabel(point("2026-09-26T08:00:00Z", "OBSERVED"))).toBe("08:00");
  });

  it("still guards against an explicit null horizon_h, which JSON can carry even though TypeScript cannot", () => {
    const withNullHorizon = {
      ...point("2026-09-26T08:00:00Z", "OBSERVED"),
      horizon_h: null,
    } as unknown as TimelinePoint;
    expect(pointLabel(withNullHorizon)).toBe("08:00");
  });

  it("labels a predicted point by its forecast horizon", () => {
    expect(pointLabel(point("2026-09-26T10:00:00Z", "PREDICTED", 1))).toBe("+1h");
  });

  it("labels horizon_h 0 as '+0h' rather than falling back to the clock time", () => {
    expect(pointLabel(point("2026-09-26T09:00:00Z", "PREDICTED", 0))).toBe("+0h");
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
