import { Label } from "./Label";
import type { Value } from "../types/environment";

export interface ProvenancePanelProps {
  value: Value;
  title?: string;
}

/**
 * Shown when a value is clicked (issue #102): for an OBSERVED value, its
 * provenance (source, publisher, licence, observed_at, fetched_at,
 * quality); for a derived value (INFERRED / PREDICTED / SIMULATED), its
 * model (name, version, reference, uncertainty).
 */
export function ProvenancePanel({ value, title }: ProvenancePanelProps) {
  return (
    <dl className="provenance-panel" aria-label={title ?? "Provenance"}>
      <div className="provenance-panel__label">
        <Label label={value.label} />
      </div>

      {value.provenance ? (
        <>
          <dt>Source</dt>
          <dd>{value.provenance.source}</dd>
          <dt>Publisher</dt>
          <dd>{value.provenance.publisher}</dd>
          <dt>Licence</dt>
          <dd>{value.provenance.licence}</dd>
          <dt>Observed at</dt>
          <dd>{value.provenance.observed_at}</dd>
          <dt>Fetched at</dt>
          <dd>{value.provenance.fetched_at}</dd>
          <dt>Quality</dt>
          <dd>{value.provenance.quality}</dd>
        </>
      ) : null}

      {value.model ? (
        <>
          <dt>Model</dt>
          <dd>{value.model.name}</dd>
          <dt>Version</dt>
          <dd>{value.model.version}</dd>
          <dt>Reference</dt>
          <dd>{value.model.reference}</dd>
          <dt>Uncertainty</dt>
          <dd>{value.model.uncertainty}</dd>
        </>
      ) : null}
    </dl>
  );
}
