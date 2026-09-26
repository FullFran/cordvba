import { useLocale } from "../i18n/LocaleContext";
import { symbolForLabel } from "../lib/labels";
import type { Label as EpistemicLabel } from "../types/environment";

export interface LabelProps {
  label: EpistemicLabel;
}

/**
 * Renders an epistemic label (OBSERVED / PUBLISHED / INFERRED / PREDICTED /
 * SIMULATED) as text plus a symbol, so it never depends on colour alone
 * (issue #102, AC-2). The wire value (`label`, and the CSS hook derived
 * from it) is the machine token and never changes with locale (issue #124:
 * "the API contract does not change"); only the displayed word is
 * translated, via the dictionary's `epistemicLabels`.
 */
export function Label({ label }: LabelProps) {
  const { t } = useLocale();

  return (
    <span className={`label label--${label.toLowerCase()}`}>
      <span aria-hidden="true">{symbolForLabel(label)}</span> {t.epistemicLabels[label]}
    </span>
  );
}
