import { symbolForLabel } from "../lib/labels";
import type { Label as EpistemicLabel } from "../types/environment";

export interface LabelProps {
  label: EpistemicLabel;
}

/**
 * Renders an epistemic label (OBSERVED / PUBLISHED / INFERRED / PREDICTED /
 * SIMULATED) as text plus a symbol, so it never depends on colour alone
 * (issue #102, AC-2).
 */
export function Label({ label }: LabelProps) {
  return (
    <span className={`label label--${label.toLowerCase()}`}>
      <span aria-hidden="true">{symbolForLabel(label)}</span> {label}
    </span>
  );
}
