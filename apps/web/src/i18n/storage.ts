import { LOCALES, type Locale } from "./types";

/**
 * The exact key the host site (fullfran.com) uses (issue #124): both live
 * on the same origin, so a language chosen on one carries into the other.
 */
export const LOCALE_STORAGE_KEY = "ff_locale";

export function isLocale(value: unknown): value is Locale {
  return typeof value === "string" && (LOCALES as readonly string[]).includes(value);
}

type ReadableStorage = Pick<Storage, "getItem">;
type WritableStorage = Pick<Storage, "setItem">;

/** Reads the stored locale; `null` if unset, unreadable (private mode, disabled storage) or not one of `Locale`. Never throws. */
export function readStoredLocale(storage: ReadableStorage): Locale | null {
  try {
    const raw = storage.getItem(LOCALE_STORAGE_KEY);
    return isLocale(raw) ? raw : null;
  } catch {
    return null;
  }
}

/** Persists the locale; silently does nothing if storage is unavailable. Never throws. */
export function writeStoredLocale(locale: Locale, storage: WritableStorage): void {
  try {
    storage.setItem(LOCALE_STORAGE_KEY, locale);
  } catch {
    // Storage unavailable (private browsing, quota, disabled): the page
    // still works for this session, it just won't remember the choice.
  }
}
