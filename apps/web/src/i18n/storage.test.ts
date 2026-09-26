import { describe, expect, it } from "vitest";

import { LOCALE_STORAGE_KEY, isLocale, readStoredLocale, writeStoredLocale } from "./storage";

function fakeStorage(initial: Record<string, string> = {}) {
  const store = { ...initial };
  return {
    getItem: (key: string) => (key in store ? store[key]! : null),
    setItem: (key: string, value: string) => {
      store[key] = value;
    },
    _store: store,
  };
}

function throwingStorage(): { getItem: () => never; setItem: () => never } {
  return {
    getItem: () => {
      throw new Error("SecurityError: storage disabled");
    },
    setItem: () => {
      throw new Error("QuotaExceededError");
    },
  };
}

describe("isLocale", () => {
  it("accepts 'es' and 'en'", () => {
    expect(isLocale("es")).toBe(true);
    expect(isLocale("en")).toBe(true);
  });

  it("rejects anything else, including a stray value the host site never writes", () => {
    expect(isLocale("fr")).toBe(false);
    expect(isLocale(null)).toBe(false);
    expect(isLocale(undefined)).toBe(false);
    expect(isLocale(1)).toBe(false);
  });
});

describe("readStoredLocale", () => {
  it("reads a value the host site wrote under the shared ff_locale key (issue 124, AC-3)", () => {
    const storage = fakeStorage({ [LOCALE_STORAGE_KEY]: "en" });
    expect(readStoredLocale(storage)).toBe("en");
  });

  it("returns null when nothing is stored", () => {
    expect(readStoredLocale(fakeStorage())).toBeNull();
  });

  it("returns null for a stored value that is not a known locale, instead of throwing", () => {
    const storage = fakeStorage({ [LOCALE_STORAGE_KEY]: "fr" });
    expect(readStoredLocale(storage)).toBeNull();
  });

  it("returns null, never throws, when storage itself throws (private browsing, disabled storage)", () => {
    expect(() => readStoredLocale(throwingStorage())).not.toThrow();
    expect(readStoredLocale(throwingStorage())).toBeNull();
  });
});

describe("writeStoredLocale", () => {
  it("writes under the shared ff_locale key", () => {
    const storage = fakeStorage();
    writeStoredLocale("en", storage);
    expect(storage._store[LOCALE_STORAGE_KEY]).toBe("en");
  });

  it("never throws when storage is unavailable", () => {
    expect(() => writeStoredLocale("es", throwingStorage())).not.toThrow();
  });
});
