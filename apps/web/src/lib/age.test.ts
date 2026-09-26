import { describe, expect, it } from "vitest";

import { formatAge } from "./age";

describe("formatAge", () => {
  const now = new Date("2026-09-26T09:40:00Z");

  it("formats an age under a minute as 'just now'", () => {
    expect(formatAge("2026-09-26T09:39:45Z", now)).toBe("just now");
  });

  it("formats an age in whole minutes", () => {
    expect(formatAge("2026-09-26T09:37:00Z", now)).toBe("3 min ago");
  });

  it("formats an age of an hour or more in hours and minutes", () => {
    expect(formatAge("2026-09-26T08:10:00Z", now)).toBe("1 h 30 min ago");
  });

  it("formats a future time (a prediction not yet due) as 'in Xh'", () => {
    expect(formatAge("2026-09-26T12:40:00Z", now)).toBe("in 3 h");
  });
});
