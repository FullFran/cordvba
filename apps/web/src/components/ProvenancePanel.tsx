import { useLocale } from "../i18n/LocaleContext";
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
 * model (name, version, reference, uncertainty). The provenance data
 * itself (source ids, ISO timestamps, licence identifiers) is not
 * translated; only the field labels are (issue #124).
 */
export function ProvenancePanel({ value, title }: ProvenancePanelProps) {
  const { t } = useLocale();

  return (
    <dl className="provenance-panel" aria-label={title ?? t.provenance.dialogLabel}>
      <div className="provenance-panel__label">
        <Label label={value.label} />
      </div>

      {value.provenance ? (
        <>
          <dt>{t.provenance.source}</dt>
          <dd>{value.provenance.source}</dd>
          <dt>{t.provenance.publisher}</dt>
          <dd>{value.provenance.publisher}</dd>
          <dt>{t.provenance.licence}</dt>
          <dd>{value.provenance.licence}</dd>
          <dt>{t.provenance.observedAt}</dt>
          <dd>{value.provenance.observed_at}</dd>
          <dt>{t.provenance.fetchedAt}</dt>
          <dd>{value.provenance.fetched_at}</dd>
          <dt>{t.provenance.quality}</dt>
          <dd>{value.provenance.quality}</dd>
        </>
      ) : null}

      {value.model ? (
        <>
          <dt>{t.provenance.model}</dt>
          <dd>{value.model.name}</dd>
          <dt>{t.provenance.version}</dt>
          <dd>{value.model.version}</dd>
          <dt>{t.provenance.reference}</dt>
          <dd>{value.model.reference}</dd>
          <dt>{t.provenance.uncertainty}</dt>
          <dd>{value.model.uncertainty}</dd>
        </>
      ) : null}
    </dl>
  );
}
