/**
 * A tiny, dependency-free colour parser/inverter used to turn OpenFreeMap's
 * light "liberty" style into a dark one (issue #119). It only needs to
 * understand the colour forms that style actually uses (hex, rgb[a](),
 * hsl[a]()) — not the full CSS colour grammar.
 */

export interface Hsla {
  h: number;
  s: number;
  l: number;
  a: number;
}

const HEX_RE = /^#([0-9a-f]{3}|[0-9a-f]{4}|[0-9a-f]{6}|[0-9a-f]{8})$/i;
const RGB_RE = /^rgba?\(\s*([\d.]+)\s*,\s*([\d.]+)\s*,\s*([\d.]+)\s*(?:,\s*([\d.]+)\s*)?\)$/i;
const HSL_RE = /^hsla?\(\s*([\d.]+)\s*,\s*([\d.]+)%\s*,\s*([\d.]+)%\s*(?:,\s*([\d.]+)\s*)?\)$/i;

function rgbToHsl(r: number, g: number, b: number): { h: number; s: number; l: number } {
  const rn = r / 255;
  const gn = g / 255;
  const bn = b / 255;
  const max = Math.max(rn, gn, bn);
  const min = Math.min(rn, gn, bn);
  const l = (max + min) / 2;
  const delta = max - min;

  if (delta === 0) {
    return { h: 0, s: 0, l: l * 100 };
  }

  const s = delta / (1 - Math.abs(2 * l - 1));

  let h: number;
  switch (max) {
    case rn:
      h = ((gn - bn) / delta) % 6;
      break;
    case gn:
      h = (bn - rn) / delta + 2;
      break;
    default:
      h = (rn - gn) / delta + 4;
      break;
  }
  h *= 60;
  if (h < 0) h += 360;

  return { h, s: s * 100, l: l * 100 };
}

function expandHex(hex: string): string {
  if (hex.length === 3 || hex.length === 4) {
    return hex
      .split("")
      .map((c) => c + c)
      .join("");
  }
  return hex;
}

/** Parses a hex / rgb[a]() / hsl[a]() colour string. Returns `null` for anything else (a filter literal, a source-layer name, …). */
export function parseColor(input: string): Hsla | null {
  const trimmed = input.trim();

  const hexMatch = HEX_RE.exec(trimmed);
  if (hexMatch) {
    const hex = expandHex(hexMatch[1] as string);
    const r = parseInt(hex.slice(0, 2), 16);
    const g = parseInt(hex.slice(2, 4), 16);
    const b = parseInt(hex.slice(4, 6), 16);
    const a = hex.length === 8 ? parseInt(hex.slice(6, 8), 16) / 255 : 1;
    return { ...rgbToHsl(r, g, b), a };
  }

  const rgbMatch = RGB_RE.exec(trimmed);
  if (rgbMatch) {
    const [, r, g, b, a] = rgbMatch;
    return { ...rgbToHsl(Number(r), Number(g), Number(b)), a: a === undefined ? 1 : Number(a) };
  }

  const hslMatch = HSL_RE.exec(trimmed);
  if (hslMatch) {
    const [, h, s, l, a] = hslMatch;
    return { h: Number(h), s: Number(s), l: Number(l), a: a === undefined ? 1 : Number(a) };
  }

  return null;
}

/** True for any string `parseColor` can understand; false for a plain identifier (a filter literal, an id, a source-layer name). */
export function isColorString(input: string): boolean {
  return parseColor(input) !== null;
}

export function toHslString({ h, s, l, a }: Hsla): string {
  const hh = Math.round(h * 100) / 100;
  const ss = Math.round(s * 100) / 100;
  const ll = Math.round(l * 100) / 100;
  return a === 1 ? `hsl(${hh},${ss}%,${ll}%)` : `hsla(${hh},${ss}%,${ll}%,${a})`;
}

/**
 * Flips a colour's lightness around the midpoint (`100 - l`), keeping hue,
 * saturation and alpha. OpenFreeMap's "liberty" style is uniformly light
 * (a pale background, pastel roads, light water), so this one transform
 * turns nearly every layer dark in a single pass; a handful of layers we
 * care about specifically (background, water, buildings) get a curated
 * override afterwards in `darkStyle.ts`. Strings that are not colours pass
 * through unchanged.
 */
export function invertColorLightness(input: string): string {
  const parsed = parseColor(input);
  if (!parsed) {
    return input;
  }
  return toHslString({ ...parsed, l: 100 - parsed.l });
}
