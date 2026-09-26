/**
 * The same two-locale switch as the host site (fullfran.com) — issue
 * #124. Kept identical (`'es' | 'en'`, Spanish default) so the choice
 * persisted under the shared `ff_locale` localStorage key means the same
 * thing on both origins.
 */
export type Locale = "es" | "en";

export const LOCALES: readonly Locale[] = ["es", "en"];

export const DEFAULT_LOCALE: Locale = "es";
