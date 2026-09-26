import type { Label } from "../types/environment";

/**
 * A text/shape symbol for each epistemic label, so observed / predicted /
 * simulated (and the rest) are distinguishable without relying on colour.
 * OBSERVED, PREDICTED and SIMULATED match the timeline legend in issue #102
 * (● / ◌ / ◇); PUBLISHED and INFERRED get their own distinct shapes.
 */
const SYMBOLS: Record<Label, string> = {
  OBSERVED: "●", // ●
  PUBLISHED: "■", // ■
  INFERRED: "▲", // ▲
  PREDICTED: "◌", // ◌
  SIMULATED: "◇", // ◇
};

export function symbolForLabel(label: Label): string {
  return SYMBOLS[label];
}
