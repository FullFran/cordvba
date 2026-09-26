import { describe, expect, it } from "vitest";

import { DICTIONARIES } from "./dictionaries";
import { formatTemplate } from "./template";

/**
 * Flattens a nested dictionary to leaf paths (each a segment array, e.g.
 * `["header", "title"]`) and their values. Segment arrays, not dot-joined
 * strings: a dictionary value can itself legitimately contain a literal
 * "." (e.g. the licence code "CC-BY-4.0" used as an object key in
 * `footer.licences`), which would make a joined-then-re-split string
 * ambiguous.
 */
function flattenEntries(value: unknown, prefix: string[] = []): Array<{ path: string[]; value: unknown }> {
  if (Array.isArray(value)) {
    // Treated as one leaf (e.g. `cardinals`): its own length-mismatch is
    // checked separately, not key-by-key.
    return [{ path: prefix, value }];
  }
  if (value !== null && typeof value === "object") {
    return Object.entries(value as Record<string, unknown>).flatMap(([key, v]) =>
      flattenEntries(v, [...prefix, key]),
    );
  }
  return [{ path: prefix, value }];
}

function flattenKeys(value: unknown): string[] {
  return flattenEntries(value).map((e) => e.path.join("."));
}

describe("dictionaries: every key exists in both locales (issue 124, AC-1)", () => {
  const esKeys = flattenKeys(DICTIONARIES.es).sort();
  const enKeys = flattenKeys(DICTIONARIES.en).sort();

  it("es and en expose exactly the same set of keys", () => {
    expect(esKeys).toEqual(enKeys);
  });

  it("neither locale has an empty string for any key (a missing translation, not just a missing key)", () => {
    for (const locale of ["es", "en"] as const) {
      for (const { path, value } of flattenEntries(DICTIONARIES[locale])) {
        expect(typeof value === "string" ? value.length > 0 : true, `${locale}.${path.join(".")} must not be empty`).toBe(
          true,
        );
      }
    }
  });

  it("both locales' compass arrays have exactly 16 points", () => {
    expect(DICTIONARIES.es.cardinals).toHaveLength(16);
    expect(DICTIONARIES.en.cardinals).toHaveLength(16);
  });

  it("uses MITECO's own Spanish ICA wording, not a re-translation (issue 124)", () => {
    expect(DICTIONARIES.es.airQualityCategories).toEqual({
      good: "buena",
      fair: "razonablemente buena",
      moderate: "regular",
      poor: "desfavorable",
      very_poor: "muy desfavorable",
      extremely_poor: "extremadamente desfavorable",
    });
  });

  it("displays epistemic labels as Observado/Inferido/Predicho/Simulado in Spanish (issue 124)", () => {
    expect(DICTIONARIES.es.epistemicLabels.OBSERVED).toBe("Observado");
    expect(DICTIONARIES.es.epistemicLabels.INFERRED).toBe("Inferido");
    expect(DICTIONARIES.es.epistemicLabels.PREDICTED).toBe("Predicho");
    expect(DICTIONARIES.es.epistemicLabels.SIMULATED).toBe("Simulado");
  });
});

describe("formatTemplate", () => {
  it("substitutes a single placeholder", () => {
    expect(formatTemplate("{n} min ago", { n: 3 })).toBe("3 min ago");
  });

  it("substitutes multiple placeholders", () => {
    expect(formatTemplate("{h} h {m} min ago", { h: 1, m: 30 })).toBe("1 h 30 min ago");
  });

  it("leaves an unmatched placeholder token untouched rather than throwing", () => {
    expect(formatTemplate("{missing} thing", {})).toBe("{missing} thing");
  });
});
