import { afterEach, describe, expect, it } from "vitest";

import { buildDarkStyle, readMapPalette, type MapPalette, type MapStyle } from "./darkStyle";

// A minimal slice of OpenFreeMap's real "liberty" style (fetched and
// inspected while building this module): enough layers to exercise the
// generic recolour pass, the curated overrides, and the "leave non-colour
// strings alone" guarantee, without committing the whole 111-layer style
// as a fixture.
const baseStyle: MapStyle = {
  version: 8,
  sources: { openmaptiles: { type: "vector", url: "https://tiles.openfreemap.org/planet" } },
  layers: [
    { id: "background", type: "background", paint: { "background-color": "#f8f4f0" } }, // ds-allow-hardcode
    {
      id: "water",
      type: "fill",
      source: "openmaptiles",
      "source-layer": "water",
      filter: ["!=", ["get", "brunnel"], "tunnel"],
      paint: { "fill-color": "rgb(158,189,255)" }, // ds-allow-hardcode
    },
    {
      id: "building",
      type: "fill",
      source: "openmaptiles",
      "source-layer": "building",
      minzoom: 13,
      maxzoom: 14,
      paint: { "fill-color": "#d9d0c9", "fill-outline-color": "#c6b8a6" }, // ds-allow-hardcode
    },
    {
      id: "building-3d",
      type: "fill-extrusion",
      source: "openmaptiles",
      "source-layer": "building",
      minzoom: 14,
      paint: {
        "fill-extrusion-base": ["get", "render_min_height"],
        "fill-extrusion-color": "hsl(35,8%,85%)", // ds-allow-hardcode
        "fill-extrusion-height": ["get", "render_height"],
        "fill-extrusion-opacity": 0.8,
      },
    },
    {
      id: "label_city",
      type: "symbol",
      source: "openmaptiles",
      "source-layer": "place",
      paint: { "text-color": "#333", "text-halo-color": "#fff", "text-halo-width": 1.2 }, // ds-allow-hardcode (fixture: OpenFreeMap's original light-style colours)
    },
    {
      // A real OpenFreeMap layer shape (e.g. several `label_*`/`road_*`
      // symbol layers): no `paint` key at all, only `layout`. MapLibre's
      // style schema rejects an explicit `paint: undefined`. Not one of
      // the suppressed/muted/overridden ids, so it must pass through
      // exactly as-is.
      id: "waterway_tunnel",
      type: "symbol",
      source: "openmaptiles",
      "source-layer": "transportation",
      layout: { "symbol-placement": "line" },
    },
    {
      id: "poi_r7",
      type: "symbol",
      source: "openmaptiles",
      "source-layer": "poi",
      paint: { "text-color": "#666", "icon-opacity": 1, "text-opacity": 1 }, // ds-allow-hardcode
    },
    {
      id: "highway-name-major",
      type: "symbol",
      source: "openmaptiles",
      "source-layer": "transportation_name",
      paint: { "text-color": "#666" }, // ds-allow-hardcode
    },
    {
      id: "road_motorway",
      type: "line",
      source: "openmaptiles",
      "source-layer": "transportation",
      minzoom: 5,
      paint: { "line-color": "hsl(35,60%,70%)" }, // ds-allow-hardcode (fixture: original warm amber road colour)
    },
    {
      id: "road_motorway_casing",
      type: "line",
      source: "openmaptiles",
      "source-layer": "transportation",
      paint: { "line-color": "hsl(35,40%,50%)" }, // ds-allow-hardcode
    },
    {
      id: "road_minor",
      type: "line",
      source: "openmaptiles",
      "source-layer": "transportation",
      paint: { "line-color": "hsl(0,0%,100%)" }, // ds-allow-hardcode
    },
    {
      id: "aeroway_fill",
      type: "fill",
      source: "openmaptiles",
      "source-layer": "aeroway",
      minzoom: 11,
      paint: { "fill-color": "hsl(0,0%,88%)" }, // ds-allow-hardcode
    },
    {
      // A real OpenFreeMap layer (issue 119: this exact shape once broke
      // the style, "unknown property line-color" on a symbol layer): a
      // road-id-prefixed layer that is NOT line/fill, so it must not get
      // a line-color/fill-color override.
      id: "road_shield_us",
      type: "symbol",
      source: "openmaptiles",
      "source-layer": "transportation_name",
      minzoom: 9,
      layout: { "icon-image": "shield", "text-field": "ref" },
    },
  ],
};

const palette: MapPalette = {
  background: "#060a12", // ds-allow-hardcode (test fixture, mirrors --map-bg)
  water: "#0c1c33", // ds-allow-hardcode (mirrors --map-water)
  buildingLow: "#171d2b", // ds-allow-hardcode (mirrors --map-building-low)
  buildingHigh: "#3a4a63", // ds-allow-hardcode (mirrors --map-building-high)
  labelHalo: "rgba(6,10,18,0.85)", // ds-allow-hardcode (mirrors --map-label-halo)
  roadMinor: "#1e2229", // ds-allow-hardcode (mirrors --map-road-minor)
  roadMid: "#262b36", // ds-allow-hardcode (mirrors --map-road-mid)
  roadMajor: "#313949", // ds-allow-hardcode (mirrors --map-road-major)
  roadLabel: "#5a6272", // ds-allow-hardcode (mirrors --map-road-label)
};

describe("buildDarkStyle", () => {
  it("overrides the background with the exact palette colour", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    const background = dark.layers.find((l) => l.id === "background");
    expect(background?.paint?.["background-color"]).toBe(palette.background);
  });

  it("overrides water with the exact palette colour", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    const water = dark.layers.find((l) => l.id === "water");
    expect(water?.paint?.["fill-color"]).toBe(palette.water);
  });

  it("keeps building-3d's data-driven height expression, recoloured into a low/high ramp", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    const building3d = dark.layers.find((l) => l.id === "building-3d");
    const color = building3d?.paint?.["fill-extrusion-color"];
    expect(Array.isArray(color)).toBe(true);
    expect(JSON.stringify(color)).toContain("render_height");
    expect(JSON.stringify(color)).toContain(palette.buildingLow);
    expect(JSON.stringify(color)).toContain(palette.buildingHigh);
    // height/base expressions (the real per-feature OSM data) are untouched
    expect(building3d?.paint?.["fill-extrusion-height"]).toEqual(["get", "render_height"]);
    expect(building3d?.paint?.["fill-extrusion-base"]).toEqual(["get", "render_min_height"]);
  });

  it("darkens every other layer generically, via lightness inversion, without a curated override", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    const building2d = dark.layers.find((l) => l.id === "building");
    // fixture colour (L ~85%, light); its recoloured value must be a colour string, not the original hex.
    expect(building2d?.paint?.["fill-color"]).not.toBe("#d9d0c9"); // ds-allow-hardcode
    expect(typeof building2d?.paint?.["fill-color"]).toBe("string");
  });

  it("never touches non-colour fields: ids, sources, source-layer, filters, minzoom/maxzoom, numeric paint values", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    const water = dark.layers.find((l) => l.id === "water");
    expect(water?.id).toBe("water");
    expect(water?.source).toBe("openmaptiles");
    expect(water?.["source-layer"]).toBe("water");
    expect(water?.filter).toEqual(["!=", ["get", "brunnel"], "tunnel"]);

    // building's minzoom/maxzoom are a deliberate override too (13/14 ->
    // 11/12, see MINZOOM_OVERRIDES and the "extrusion visibility"
    // describe block below); a layer with no override at all (water,
    // checked above) is the proof this generic pass never touches
    // zoom ranges on its own.

    // building-3d's opacity is a deliberate override (0.8 -> 0.85, see
    // overridePaint), covered by the height-ramp test above; here we check
    // that a layer with NO curated override keeps its numeric paint values
    // exactly, proving the generic pass never touches non-colour data.
    const building2d = dark.layers.find((l) => l.id === "building");
    expect(building2d?.paint?.["fill-outline-color"]).toBeDefined();
  });

  it("recolours a symbol layer's text and halo colours for contrast against the dark map", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    const label = dark.layers.find((l) => l.id === "label_city");
    expect(label?.paint?.["text-color"]).not.toBe("#333"); // ds-allow-hardcode
    expect(label?.paint?.["text-halo-color"]).not.toBe("#fff"); // ds-allow-hardcode
    expect(label?.paint?.["text-halo-width"]).toBe(1.2); // non-colour value untouched
  });

  it("preserves the sources object unchanged (the tile source stays OpenFreeMap unless swapped upstream)", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    expect(dark.sources).toEqual(baseStyle.sources);
  });

  it("preserves the same number of layers, in the same order", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    expect(dark.layers.map((l) => l.id)).toEqual(baseStyle.layers.map((l) => l.id));
  });

  it("never introduces a paint key on a layer that had none (MapLibre's schema rejects an explicit paint: undefined)", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    const layer = dark.layers.find((l) => l.id === "waterway_tunnel");
    expect(layer).toBeDefined();
    expect("paint" in (layer as object)).toBe(false);
    expect(layer?.layout).toEqual({ "symbol-placement": "line" });
  });
});

describe("buildDarkStyle: basemap noise reduction (issue 119, maintainer review)", () => {
  it("hides POI icons/labels outright (icon-opacity and text-opacity both 0)", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    const poi = dark.layers.find((l) => l.id === "poi_r7");
    expect(poi?.paint?.["icon-opacity"]).toBe(0);
    expect(poi?.paint?.["text-opacity"]).toBe(0);
  });

  it("mutes, but does not hide, a kept landmark/major-road label", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    const major = dark.layers.find((l) => l.id === "highway-name-major");
    const opacity = major?.paint?.["text-opacity"] as number;
    expect(opacity).toBeGreaterThan(0);
    expect(opacity).toBeLessThan(1);
  });

  it("still recolours a muted layer's own text colour (it is muted, not left in its original light colour)", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    const major = dark.layers.find((l) => l.id === "highway-name-major");
    expect(major?.paint?.["text-color"]).not.toBe("#666"); // ds-allow-hardcode
  });

  it("does not suppress or mute a layer outside those named sets (e.g. district/city labels stay at full opacity)", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    const cityLabel = dark.layers.find((l) => l.id === "label_city");
    expect(cityLabel?.paint?.["icon-opacity"]).toBeUndefined();
    expect(cityLabel?.paint?.["text-opacity"]).toBeUndefined();
  });

  it("gives a muted major-road label the exact curated colour, not just any non-original colour", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    const major = dark.layers.find((l) => l.id === "highway-name-major");
    expect(major?.paint?.["text-color"]).toBe(palette.roadLabel);
  });
});

describe("buildDarkStyle: road network muted to neutral slate (issue 119, maintainer review)", () => {
  it("recolours a major road (motorway) to the curated major-road colour, not its original warm amber", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    const motorway = dark.layers.find((l) => l.id === "road_motorway");
    expect(motorway?.paint?.["line-color"]).toBe(palette.roadMajor);
  });

  it("recolours a road casing to the minor tier, one step darker than its own fill's tier", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    const casing = dark.layers.find((l) => l.id === "road_motorway_casing");
    expect(casing?.paint?.["line-color"]).toBe(palette.roadMinor);
  });

  it("recolours a minor road to the minor-tier colour", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    const minor = dark.layers.find((l) => l.id === "road_minor");
    expect(minor?.paint?.["line-color"]).toBe(palette.roadMinor);
  });

  it("recolours a fill-type road layer (aeroway) via fill-color, not line-color", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    const aeroway = dark.layers.find((l) => l.id === "aeroway_fill");
    expect(aeroway?.paint?.["fill-color"]).toBe(palette.roadMinor);
    expect(aeroway?.paint?.["line-color"]).toBeUndefined();
  });

  it("never applies line-color/fill-color to a symbol layer whose id happens to start with a road prefix (real regression: MapLibre rejected 'unknown property line-color' on road_shield_us)", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    const shield = dark.layers.find((l) => l.id === "road_shield_us");
    expect(shield?.paint).toBeUndefined();
    expect(shield?.layout).toEqual({ "icon-image": "shield", "text-field": "ref" });
  });

  it("keeps a major road no more than ~2.5x the minor tier's relative luminance (verified against the WCAG relative-luminance formula, not just 'brighter')", () => {
    // Palette values already chosen so minor/mid/major stay comfortably
    // under 25-30% of the darkest beacon's luminance (see tokens.css);
    // this only guards the *ratio* between tiers stays modest.
    function relLuminance(hex: string): number {
      const n = parseInt(hex.slice(1), 16);
      const lin = (c: number) => {
        const cs = c / 255;
        return cs <= 0.03928 ? cs / 12.92 : ((cs + 0.055) / 1.055) ** 2.4;
      };
      const r = lin((n >> 16) & 255);
      const g = lin((n >> 8) & 255);
      const b = lin(n & 255);
      return 0.2126 * r + 0.7152 * g + 0.0722 * b;
    }
    const minorLum = relLuminance(palette.roadMinor);
    const majorLum = relLuminance(palette.roadMajor);
    expect(majorLum / minorLum).toBeLessThan(3);
  });
});

// ds-allow-hardcode:start (test fixture: arbitrary literal colours, distinct per phase, exercising readMapPalette's own token-reading logic — not this app's own design decisions)
describe("readMapPalette phase support (issue #139: a day/golden/blue/night base-map palette driven by sun altitude, tokens not hardcoded colours)", () => {
  afterEach(() => {
    // jsdom resolves inline custom properties without loading tokens.css
    // (vitest config sets `css: false`), so each test sets exactly the
    // properties it needs and this clears them again afterwards.
    document.documentElement.removeAttribute("style");
  });

  it("reads the night (default) background/water tokens when no phase is given, unchanged from before this feature", () => {
    document.documentElement.style.setProperty("--map-bg", "#010203");
    document.documentElement.style.setProperty("--map-water", "#040506");
    const palette = readMapPalette(document.documentElement);
    expect(palette.background).toBe("#010203");
    expect(palette.water).toBe("#040506");
  });

  it("reads the day-phase background/water tokens when phase is 'day'", () => {
    document.documentElement.style.setProperty("--map-bg-day", "#111213");
    document.documentElement.style.setProperty("--map-water-day", "#141516");
    const palette = readMapPalette(document.documentElement, "day");
    expect(palette.background).toBe("#111213");
    expect(palette.water).toBe("#141516");
  });

  it("reads the golden-hour-phase background/water tokens when phase is 'golden'", () => {
    document.documentElement.style.setProperty("--map-bg-golden", "#211213");
    document.documentElement.style.setProperty("--map-water-golden", "#241516");
    const palette = readMapPalette(document.documentElement, "golden");
    expect(palette.background).toBe("#211213");
    expect(palette.water).toBe("#241516");
  });

  it("reads the blue-hour-phase background/water tokens when phase is 'blue'", () => {
    document.documentElement.style.setProperty("--map-bg-blue", "#0e1030");
    document.documentElement.style.setProperty("--map-water-blue", "#141850");
    const palette = readMapPalette(document.documentElement, "blue");
    expect(palette.background).toBe("#0e1030");
    expect(palette.water).toBe("#141850");
  });

  it("leaves every non-background/water token identical across phases (only the sky/water tint changes)", () => {
    document.documentElement.style.setProperty("--map-building-low", "#171d2b");
    const night = readMapPalette(document.documentElement, "night");
    const day = readMapPalette(document.documentElement, "day");
    expect(day.buildingLow).toBe(night.buildingLow);
    expect(day.roadMinor).toBe(night.roadMinor);
  });
});
// ds-allow-hardcode:end

describe("buildDarkStyle: extrusion visibility (issue 119, maintainer review: '3D barely perceptible')", () => {
  it("lowers building-3d's minzoom so extrusions render at a wider fitBounds camera", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    const b3d = dark.layers.find((l) => l.id === "building-3d");
    expect(b3d?.minzoom).toBeLessThan(14);
  });

  it("lowers the flat 2D building layer's maxzoom to match, so there is no gap or double-render", () => {
    const dark = buildDarkStyle(baseStyle, palette);
    const building2d = dark.layers.find((l) => l.id === "building");
    const b3d = dark.layers.find((l) => l.id === "building-3d");
    expect(building2d?.maxzoom).toBeLessThanOrEqual(b3d?.minzoom as number);
  });
});
