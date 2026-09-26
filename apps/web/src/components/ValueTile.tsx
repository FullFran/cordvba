import { formatAge } from "../lib/age";
import { Label } from "./Label";
import type { Value } from "../types/environment";

export interface ValueTileProps {
  name: string;
  value: Value;
  now?: Date;
  onSelect?: (value: Value) => void;
  /** Extra descriptive text, e.g. an air-quality category. */
  extra?: string;
}

/**
 * One state-panel or scenario-panel value: its name, number/unit, epistemic
 * label and age. Clicking it opens its provenance (issue #102).
 */
export function ValueTile({ name, value, now, onSelect, extra }: ValueTileProps) {
  return (
    <button type="button" className="value-tile" onClick={() => onSelect?.(value)}>
      <span className="value-tile__name">{name}</span>
      <span className="value-tile__value">
        {value.value === null ? "—" : value.value} {value.unit}
        {extra ? ` (${extra})` : null}
      </span>
      <Label label={value.label} />
      <span className="value-tile__age">{formatAge(value.at, now)}</span>
    </button>
  );
}
