import { DICTIONARIES } from "../i18n/dictionaries";
import { formatTemplate } from "../i18n/template";
import type { Locale } from "../i18n/types";
import type { Source } from "../types/environment";

/**
 * Builds a source's attribution line from the contract's own
 * `id`/`publisher`/`licence` fields, translated through the dictionaries
 * (issue #124). The API's `Source.attribution` field is English-only and
 * pre-formatted server-side; showing it verbatim inside the Spanish UI
 * mixed languages mid-sentence. That field is now only a fallback, for a
 * source id this dictionary doesn't yet know about — so a genuinely new
 * source degrades to *something* readable instead of "undefined".
 */
export function formatSourceAttribution(source: Source, locale: Locale = "en"): string {
  const t = DICTIONARIES[locale].footer;
  const kind = t.sourceKinds[source.id];
  if (kind === undefined) {
    return source.attribution;
  }
  const licence = t.licences[source.licence] ?? source.licence;
  return formatTemplate(t.attributionTemplate, { kind, publisher: source.publisher, licence });
}
