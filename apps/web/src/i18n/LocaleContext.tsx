import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";

import { DICTIONARIES } from "./dictionaries";
import type { Dictionary } from "./dictionary";
import { readStoredLocale, writeStoredLocale } from "./storage";
import { DEFAULT_LOCALE, type Locale } from "./types";

export interface LocaleContextValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: Dictionary;
}

/**
 * A component that calls `useLocale()` outside a `<LocaleProvider>` (most
 * existing component tests, which render in isolation) gets the English
 * dictionary and a no-op setter rather than a thrown error. The real app
 * (`main.tsx`) always wraps in a provider; this default only spares every
 * pre-existing test from having to add one just to render at all (issue
 * #124 was built after those tests already existed).
 */
const defaultValue: LocaleContextValue = {
  locale: "en",
  setLocale: () => {
    /* no-op outside a provider */
  },
  t: DICTIONARIES.en,
};

const LocaleContext = createContext<LocaleContextValue>(defaultValue);

function resolveInitialLocale(): Locale {
  if (typeof window === "undefined") {
    return DEFAULT_LOCALE;
  }
  return readStoredLocale(window.localStorage) ?? DEFAULT_LOCALE;
}

export interface LocaleProviderProps {
  children: ReactNode;
}

/**
 * The real locale source for the app (issue #124): defaults to Spanish,
 * adopts whatever the host site (fullfran.com) already stored under
 * `ff_locale` on first load (AC-3), keeps `document.documentElement.lang`
 * in sync, and re-renders every consumer instantly on switch (AC-2) since
 * this is ordinary React context, not a page reload.
 */
export function LocaleProvider({ children }: LocaleProviderProps) {
  const [locale, setLocaleState] = useState<Locale>(resolveInitialLocale);

  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);

  const setLocale = useCallback((next: Locale) => {
    setLocaleState(next);
    writeStoredLocale(next, window.localStorage);
  }, []);

  const value = useMemo<LocaleContextValue>(
    () => ({ locale, setLocale, t: DICTIONARIES[locale] }),
    [locale, setLocale],
  );

  return <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>;
}

export function useLocale(): LocaleContextValue {
  return useContext(LocaleContext);
}
