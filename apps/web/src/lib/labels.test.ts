import { describe, expect, it } from "vitest";

import { symbolForLabel } from "./labels";

describe("symbolForLabel", () => {
  it("gives every epistemic label a distinct, non-empty symbol", () => {
    const labels = ["OBSERVED", "PUBLISHED", "INFERRED", "PREDICTED", "SIMULATED"] as const;
    const symbols = labels.map((label) => symbolForLabel(label));

    for (const symbol of symbols) {
      expect(symbol.length).toBeGreaterThan(0);
    }
    expect(new Set(symbols).size).toBe(labels.length);
  });

  it("uses the symbols the timeline spec fixes for observed, predicted and simulated", () => {
    expect(symbolForLabel("OBSERVED")).toBe("●"); // ●
    expect(symbolForLabel("PREDICTED")).toBe("◌"); // ◌
    expect(symbolForLabel("SIMULATED")).toBe("◇"); // ◇
  });
});
