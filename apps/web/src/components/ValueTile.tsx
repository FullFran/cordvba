import { useLocale } from "../i18n/LocaleContext";
import { formatAge } from "../lib/age";
import { formatValue } from "../lib/format";
import { Label } from "./Label";
import type { Value } from "../types/environment";

export interface ValueTileProps {
  name: string;
  value: Value;
  now?: Date;
  onSelect?: (value: Value) => void;
  /** Extra descriptive text, e.g. an air-quality category. */
  extra?: string;
  /**
   * Overrides the default `formatValue(value.value, value.unit)` rendering,
   * for a value whose display needs more than a unit suffix (e.g. wind
   * direction as a cardinal + degree pair, issue #119).
   */
  displayValue?: string;
}

/**
 * One state-panel or scenario-panel value: its name, number/unit, epistemic
 * label and age. Clicking it opens its provenance (issue #102).
 */
export function ValueTile({ name, value, now, onSelect, extra, displayValue }: ValueTileProps) {
  const { locale } = useLocale();

  return (
    <button type="button" className="value-tile" onClick={() => onSelect?.(value)}>
      <span className="value-tile__name">{name}</span>
      <span className="value-tile__value">
        {displayValue ?? formatValue(value.value, value.unit, locale)}
        {extra ? ` (${extra})` : null}
      </span>
      <Label label={value.label} />
      <span className="value-tile__age">{formatAge(value.at, now, locale)}</span>
    </button>
  );
}
