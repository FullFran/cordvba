/**
 * Fills `{token}` placeholders in a dictionary string, e.g.
 * `format("{n} min ago", { n: 3 })` -> `"3 min ago"`. Kept this simple
 * (string templates, not a full ICU MessageFormat) because every
 * placeholder here is already a locale-formatted number by the time it
 * reaches this function — the thing that actually needs `Intl` (digit
 * shape, decimal separator) is resolved before templating, not by it.
 */
export function formatTemplate(template: string, values: Record<string, string | number>): string {
  return template.replace(/\{(\w+)\}/g, (match, key: string) =>
    Object.prototype.hasOwnProperty.call(values, key) ? String(values[key]) : match,
  );
}
