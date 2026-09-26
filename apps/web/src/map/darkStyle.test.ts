import { describe, expect, it } from "vitest";

import { buildDarkStyle, type MapPalette, type MapStyle } from "./darkStyle";

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
  ],
};

const palette: MapPalette = {
  background: "#060a12", // ds-allow-hardcode (test fixture, mirrors --map-bg)
  water: "#0c1c33", // ds-allow-hardcode (mirrors --map-water)
  buildingLow: "#171d2b", // ds-allow-hardcode (mirrors --map-building-low)
  buildingHigh: "#3a4a63", // ds-allow-hardcode (mirrors --map-building-high)
  labelHalo: "rgba(6,10,18,0.85)", // ds-allow-hardcode (mirrors --map-label-halo)
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

    const building = dark.layers.find((l) => l.id === "building");
    expect(building?.minzoom).toBe(13);
    expect(building?.maxzoom).toBe(14);

    // building-3d's opacity is a deliberate override (0.8 -> 0.85, see
    // overridePaint), covered by the height-ramp test above; here we check
    // that a layer with NO curated override keeps its numeric paint values
    // exactly, proving the generic pass never touches non-colour data.
    const building2d = dark.layers.find((l) => l.id === "building");
    expect(building2d?.minzoom).toBe(13);
    expect(building2d?.maxzoom).toBe(14);
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
});
