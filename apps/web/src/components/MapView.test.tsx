import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import environmentFixture from "@contracts/environment/v1/environment.example.json";
import type { SunPosition } from "../lib/sun";
import type { EnvironmentResponse } from "../types/environment";

const environment = environmentFixture as unknown as EnvironmentResponse;
/** A fixed midday sun (issue #139) so every pre-existing test keeps its original shadows/lighting-agnostic behaviour; the feature's own behaviour is covered in the describe block below. */
const DEFAULT_SUN: SunPosition = { azimuthDeg: 180, altitudeDeg: 50 };

interface FakeMapInstance {
  options: Record<string, unknown>;
  remove: ReturnType<typeof vi.fn>;
}
interface FakeMarkerInstance {
  element: HTMLElement;
  setLngLat: ReturnType<typeof vi.fn>;
  addTo: ReturnType<typeof vi.fn>;
  remove: ReturnType<typeof vi.fn>;
}

const mapInstances: FakeMapInstance[] = [];
const markerInstances: FakeMarkerInstance[] = [];

vi.mock("maplibre-gl", () => {
  class FakeMarker implements FakeMarkerInstance {
    element: HTMLElement;
    setLngLat = vi.fn().mockReturnThis();
    addTo = vi.fn().mockReturnThis();
    remove = vi.fn();
    constructor(opts: { element: HTMLElement }) {
      this.element = opts.element;
      markerInstances.push(this);
    }
  }

  class FakeMap implements FakeMapInstance {
    options: Record<string, unknown>;
    addControl = vi.fn();
    remove = vi.fn();
    on = vi.fn();
    off = vi.fn();
    fitBounds = vi.fn();
    flyTo = vi.fn();
    project = vi.fn().mockReturnValue({ x: 400, y: 200 });
    // Matches the page's own initial bearing (issue #138's `INITIAL_BEARING`).
    getBearing = vi.fn(() => -35);
    getBounds = vi.fn(() => ({
      getSouthWest: () => ({ lng: -4.82, lat: 37.86 }),
      getNorthEast: () => ({ lng: -4.74, lat: 37.92 }),
    }));
    getContainer = vi.fn(() => {
      const div = document.createElement("div");
      // jsdom's clientWidth/clientHeight are read-only getters; shadow them
      // with own properties instead of assigning (which throws).
      Object.defineProperty(div, "clientWidth", { value: 800, configurable: true });
      Object.defineProperty(div, "clientHeight", { value: 400, configurable: true });
      return div;
    });
    // Sun-driven lighting/shadows (issue #139): the style is already
    // "loaded" so effects apply immediately instead of waiting on a
    // "load" event this fake never fires. `sources`/`layers` are real
    // per-instance registries (not blanket stubs) so a test can add a
    // source and then observe `getSource(...).setData` being called on
    // it, the same round-trip MapView.tsx itself relies on.
    sources = new Map<string, { setData: ReturnType<typeof vi.fn> }>();
    layers = new Set<string>();
    isStyleLoaded = vi.fn(() => true);
    setPaintProperty = vi.fn();
    setLight = vi.fn();
    getSource = vi.fn((id: string) => this.sources.get(id));
    getLayer = vi.fn((id: string) => (this.layers.has(id) ? { id } : undefined));
    addSource = vi.fn((id: string) => {
      this.sources.set(id, { setData: vi.fn() });
    });
    addLayer = vi.fn((layer: { id: string }) => {
      this.layers.add(layer.id);
    });
    queryRenderedFeatures = vi.fn(() => [] as unknown[]);
    constructor(options: Record<string, unknown>) {
      this.options = options;
      mapInstances.push(this);
    }
  }

  class FakeNavigationControl {}

  class FakeLngLatBounds {
    points: [number, number][];
    constructor(a: [number, number], b: [number, number]) {
      this.points = [a, b];
    }
    extend(point: [number, number]) {
      this.points.push(point);
      return this;
    }
    getCenter() {
      const lons = this.points.map((p) => p[0]);
      const lats = this.points.map((p) => p[1]);
      return { lng: (Math.min(...lons) + Math.max(...lons)) / 2, lat: (Math.min(...lats) + Math.max(...lats)) / 2 };
    }
  }

  const maplibregl = {
    Map: FakeMap,
    Marker: FakeMarker,
    NavigationControl: FakeNavigationControl,
    LngLatBounds: FakeLngLatBounds,
  };
  return {
    default: maplibregl,
    Map: FakeMap,
    Marker: FakeMarker,
    NavigationControl: FakeNavigationControl,
    LngLatBounds: FakeLngLatBounds,
  };
});

vi.mock("../map/darkStyle", () => ({
  loadDarkStyle: vi.fn().mockResolvedValue({ version: 8, sources: {}, layers: [] }),
  OPENFREEMAP_STYLE_URL: "https://tiles.openfreemap.org/styles/liberty",
  // Issue #139's sun-driven lighting reads this at runtime; the real
  // implementation is covered by its own darkStyle.test.ts, so this only
  // needs to return a plausible-shaped palette here.
  // ds-allow-hardcode:start (test fixture, mirrors tokens.css's --map-* values)
  readMapPalette: vi.fn(() => ({
    background: "#060a12",
    water: "#0c1c33",
    buildingLow: "#171d2b",
    buildingHigh: "#3a4a63",
    labelHalo: "rgba(6,10,18,0.85)",
    roadMinor: "#1e2229",
    roadMid: "#262b36",
    roadMajor: "#313949",
    roadLabel: "#5a6272",
  })),
  // ds-allow-hardcode:end
}));

import { MapView, resolveFitPadding } from "./MapView";

describe("MapView (issue 119: MapLibre dark 3D map + beacons)", () => {
  afterEach(() => {
    mapInstances.length = 0;
    markerInstances.length = 0;
  });

  it("has an accessible label and shows the honesty caption immediately, before the map style loads", () => {
    render(<MapView environment={environment} sun={DEFAULT_SUN} />);

    expect(screen.getByLabelText(/Map of Córdoba/i)).toBeInTheDocument();
    expect(screen.getByText(/no interpolated surface/i)).toBeInTheDocument();
  });

  it("creates the MapLibre map pitched over Córdoba once the dark style resolves", async () => {
    render(<MapView environment={environment} sun={DEFAULT_SUN} />);

    await waitFor(() => expect(mapInstances).toHaveLength(1));

    const [instance] = mapInstances;
    expect(instance?.options.pitch).toBeGreaterThanOrEqual(45);
    expect(instance?.options.pitch).toBeLessThanOrEqual(60);
    const center = instance?.options.center as { lng: number; lat: number };
    expect(typeof center.lng).toBe("number");
    expect(typeof center.lat).toBe("number");
  });

  it("fits the camera to the city stations plus the historic centre, not a fixed zoom (issue 119, maintainer review)", async () => {
    render(<MapView environment={environment} sun={DEFAULT_SUN} />);

    await waitFor(() => expect(mapInstances).toHaveLength(1));
    const instance = mapInstances[0] as unknown as { fitBounds: ReturnType<typeof vi.fn> };
    expect(instance.fitBounds).toHaveBeenCalledTimes(1);
    const [, fitOptions] = instance.fitBounds.mock.calls[0] as [unknown, Record<string, unknown>];
    expect(fitOptions.padding).toBeDefined();
  });

  it("creates one beacon marker per air-quality station plus one for the airport weather station", async () => {
    render(<MapView environment={environment} sun={DEFAULT_SUN} />);

    await waitFor(() =>
      expect(markerInstances).toHaveLength(environment.air_quality.stations.length + 1),
    );
  });

  it("gives each air-quality beacon marker an element carrying the category as visible text (AC-2: never colour alone)", async () => {
    render(<MapView environment={environment} sun={DEFAULT_SUN} />);

    await waitFor(() => expect(markerInstances.length).toBeGreaterThan(0));

    const categories = environment.air_quality.stations.map((s) => s.index.category);
    const beaconTexts = markerInstances.map((m) => m.element.textContent ?? "");
    for (const category of categories) {
      expect(beaconTexts.some((text) => text.includes(category))).toBe(true);
    }
  });

  it("removes the map on unmount", async () => {
    const { unmount } = render(<MapView environment={environment} sun={DEFAULT_SUN} />);
    await waitFor(() => expect(mapInstances).toHaveLength(1));

    unmount();

    expect(mapInstances[0]?.remove).toHaveBeenCalled();
  });

  it("shows the wind toggle and its label immediately, before the map style even loads (issue 119, maintainer review: label must be visible on every breakpoint)", () => {
    render(<MapView environment={environment} sun={DEFAULT_SUN} />);

    expect(screen.getByRole("checkbox")).toBeInTheDocument();
    expect(screen.getAllByText(/Illustrative wind/i).length).toBeGreaterThan(0);
  });

  it("merges the wind label and the extrusion-heights caption into one legend block, so neither can cover the other", () => {
    const { container } = render(<MapView environment={environment} sun={DEFAULT_SUN} />);

    const legends = container.querySelectorAll(".map-legend");
    expect(legends).toHaveLength(1);
    expect(legends[0]?.textContent).toContain("Illustrative wind");
    expect(legends[0]?.textContent).toMatch(/no interpolated surface/i);
  });

  it("keeps the caption visible even after the wind layer is switched off", async () => {
    render(<MapView environment={environment} sun={DEFAULT_SUN} />);

    await userEvent.click(screen.getByRole("checkbox"));

    expect(screen.queryByText(/Illustrative wind: /i)).not.toBeInTheDocument();
    expect(screen.getByText(/no interpolated surface/i)).toBeInTheDocument();
  });

  it("rotates the wind arrow to where the wind blows TO, corrected for the map's current bearing, and re-corrects it on rotate (issue #138)", async () => {
    render(<MapView environment={environment} sun={DEFAULT_SUN} />);
    await waitFor(() => expect(mapInstances).toHaveLength(1));
    // The FakeMarker mock never appends its element into the DOM (that's
    // MapLibre's own job in the real library), so the airport beacon's
    // element is reached through the marker instance itself, the same way
    // the "carries the category as visible text" test above does.
    await waitFor(() => expect(markerInstances.length).toBeGreaterThan(0));
    const airportBeacon = markerInstances[markerInstances.length - 1]!.element;
    const arrow = () => airportBeacon.querySelector<HTMLElement>(".beacon__arrow");

    const instance = mapInstances[0] as unknown as {
      getBearing: ReturnType<typeof vi.fn>;
      on: ReturnType<typeof vi.fn>;
    };

    // Fixture wind_direction is 250° (from the WSW) -> blows TO 70°;
    // mocked initial bearing -35° -> screen rotation 70 - (-35) = 105°.
    expect(arrow()?.style.transform).toContain("105deg");

    instance.getBearing.mockReturnValue(90);
    const rotateHandler = instance.on.mock.calls.find((call) => call[0] === "rotate")?.[1] as () => void;
    rotateHandler?.();

    // 70 - 90 = -20 -> wraps to 340.
    expect(arrow()?.style.transform).toContain("340deg");
  });

  it("shows an edge-clamped distance indicator for the airport when it projects outside the viewport", async () => {
    const { container } = render(<MapView environment={environment} sun={DEFAULT_SUN} />);
    await waitFor(() => expect(mapInstances).toHaveLength(1));
    const instance = mapInstances[0] as unknown as { project: ReturnType<typeof vi.fn>; on: ReturnType<typeof vi.fn> };

    // Move the airport's projected position far outside the 800x400 container.
    instance.project.mockReturnValue({ x: 5000, y: 200 });
    // Re-run whichever handler was registered for "move".
    const moveHandler = instance.on.mock.calls.find((call) => call[0] === "move")?.[1] as () => void;
    moveHandler?.();

    const edge = container.querySelector<HTMLElement>(".beacon--edge");
    expect(edge).not.toBeNull();
    expect(edge?.style.display).toBe("flex");
    expect(edge?.textContent).toContain("km");
  });
});

describe("MapView: sun-driven lighting and building shadows (issue #139)", () => {
  afterEach(() => {
    mapInstances.length = 0;
    markerInstances.length = 0;
  });

  it("applies the sun's position as the map's light and the background/water palette", async () => {
    render(<MapView environment={environment} sun={{ azimuthDeg: 200, altitudeDeg: 40 }} />);
    await waitFor(() => expect(mapInstances).toHaveLength(1));
    const instance = mapInstances[0] as unknown as {
      setPaintProperty: ReturnType<typeof vi.fn>;
      setLight: ReturnType<typeof vi.fn>;
    };

    expect(instance.setPaintProperty).toHaveBeenCalledWith("background", "background-color", expect.any(String));
    expect(instance.setPaintProperty).toHaveBeenCalledWith("water", "fill-color", expect.any(String));
    expect(instance.setLight).toHaveBeenCalledWith(
      expect.objectContaining({ position: [expect.any(Number), 200, 50] }), // polar = 90 - altitude
    );
  });

  it("adds the building-shadows source/layer and populates it from queried building-3d features when the sun is up", async () => {
    const squareRing: [number, number][] = [
      [-4.78, 37.89],
      [-4.779, 37.89],
      [-4.779, 37.888],
      [-4.78, 37.888],
    ];
    render(<MapView environment={environment} sun={{ azimuthDeg: 200, altitudeDeg: 40 }} />);
    await waitFor(() => expect(mapInstances).toHaveLength(1));
    const instance = mapInstances[0] as unknown as {
      queryRenderedFeatures: ReturnType<typeof vi.fn>;
      getSource: ReturnType<typeof vi.fn>;
      addSource: ReturnType<typeof vi.fn>;
      addLayer: ReturnType<typeof vi.fn>;
    };
    instance.queryRenderedFeatures.mockReturnValue([
      { properties: { render_height: 20 }, geometry: { type: "Polygon", coordinates: [squareRing] } },
    ]);

    // Re-trigger the effect's own recompute the same way a camera move would.
    const onCalls = (mapInstances[0] as unknown as { on: ReturnType<typeof vi.fn> }).on.mock.calls;
    const moveEndHandler = onCalls.find((call) => call[0] === "moveend")?.[1] as () => void;
    moveEndHandler?.();

    expect(instance.addSource).toHaveBeenCalledWith("building-shadows", expect.objectContaining({ type: "geojson" }));
    expect(instance.addLayer).toHaveBeenCalledWith(expect.objectContaining({ id: "building-shadows-layer" }));
    const source = instance.getSource("building-shadows") as { setData: ReturnType<typeof vi.fn> };
    const lastCall = source.setData.mock.calls[source.setData.mock.calls.length - 1];
    expect(lastCall![0].features.length).toBeGreaterThan(0);
  });

  it("hides the building-shadows layer once the sun is at or below the horizon (AC-3)", async () => {
    render(<MapView environment={environment} sun={{ azimuthDeg: 200, altitudeDeg: -5 }} />);
    await waitFor(() => expect(mapInstances).toHaveLength(1));
    const instance = mapInstances[0] as unknown as { getSource: ReturnType<typeof vi.fn> };

    const source = instance.getSource("building-shadows") as { setData: ReturnType<typeof vi.fn> };
    const lastCall = source.setData.mock.calls[source.setData.mock.calls.length - 1];
    expect(lastCall![0].features).toEqual([]);
  });

  it("shows the shadow-honesty legend line only while the sun is up", () => {
    const { rerender } = render(<MapView environment={environment} sun={{ azimuthDeg: 200, altitudeDeg: 40 }} />);
    expect(screen.getByText(/inferred/i)).toBeInTheDocument();

    rerender(<MapView environment={environment} sun={{ azimuthDeg: 200, altitudeDeg: -5 }} />);
    expect(screen.queryByText(/inferred/i)).not.toBeInTheDocument();
  });
});

describe("MapView: air quality as columns of light (issue #140)", () => {
  afterEach(() => {
    mapInstances.length = 0;
    markerInstances.length = 0;
  });

  it("adds one column feature per station — never an interpolated surface between them", async () => {
    render(<MapView environment={environment} sun={{ azimuthDeg: 200, altitudeDeg: 40 }} />);
    await waitFor(() => expect(mapInstances).toHaveLength(1));
    const instance = mapInstances[0] as unknown as {
      addSource: ReturnType<typeof vi.fn>;
      addLayer: ReturnType<typeof vi.fn>;
      getSource: ReturnType<typeof vi.fn>;
    };

    expect(instance.addSource).toHaveBeenCalledWith("air-quality-columns", expect.objectContaining({ type: "geojson" }));
    expect(instance.addLayer).toHaveBeenCalledWith(
      expect.objectContaining({ id: "air-quality-columns-layer", type: "fill-extrusion" }),
    );
    const source = instance.getSource("air-quality-columns") as { setData: ReturnType<typeof vi.fn> };
    const lastCall = source.setData.mock.calls[source.setData.mock.calls.length - 1];
    expect(lastCall![0].features).toHaveLength(environment.air_quality.stations.length);
  });

  it("renders the column hero-scale: vertical gradient, high opacity (parent review)", async () => {
    render(<MapView environment={environment} sun={{ azimuthDeg: 200, altitudeDeg: 40 }} />);
    await waitFor(() => expect(mapInstances).toHaveLength(1));
    const instance = mapInstances[0] as unknown as { addLayer: ReturnType<typeof vi.fn> };

    const columnsCall = instance.addLayer.mock.calls.find((call) => call[0]?.id === "air-quality-columns-layer");
    expect(columnsCall).toBeDefined();
    const paint = columnsCall![0].paint;
    expect(paint["fill-extrusion-vertical-gradient"]).toBe(true);
    expect(paint["fill-extrusion-opacity"]).toBeGreaterThanOrEqual(0.85);
  });

  it("adds a breathing glow circle layer, blurred, at the same source points as the columns (parent review)", async () => {
    render(<MapView environment={environment} sun={{ azimuthDeg: 200, altitudeDeg: 40 }} />);
    await waitFor(() => expect(mapInstances).toHaveLength(1));
    const instance = mapInstances[0] as unknown as {
      addLayer: ReturnType<typeof vi.fn>;
      addSource: ReturnType<typeof vi.fn>;
      getSource: ReturnType<typeof vi.fn>;
    };

    expect(instance.addSource).toHaveBeenCalledWith("air-quality-glow", expect.objectContaining({ type: "geojson" }));
    const glowCall = instance.addLayer.mock.calls.find((call) => call[0]?.id === "air-quality-glow-layer");
    expect(glowCall).toBeDefined();
    expect(glowCall![0].type).toBe("circle");
    expect(glowCall![0].paint["circle-blur"]).toBeGreaterThan(0);

    const source = instance.getSource("air-quality-glow") as { setData: ReturnType<typeof vi.fn> };
    await waitFor(() => expect(source.setData).toHaveBeenCalled());
    const lastCall = source.setData.mock.calls[source.setData.mock.calls.length - 1];
    const [feature] = lastCall![0].features;
    expect(feature.geometry.type).toBe("Point");
    expect(typeof feature.properties.opacity).toBe("number");
    expect(typeof feature.properties.radiusScale).toBe("number");
  });

  it("shows the columns-are-not-interpolated legend line, always (not conditional on the sun)", () => {
    render(<MapView environment={environment} sun={{ azimuthDeg: 200, altitudeDeg: -5 }} />);
    expect(screen.getByText(/nothing is interpolated between stations/i)).toBeInTheDocument();
  });

  it("shows the pollutant responsible on the air-quality beacon marker", async () => {
    render(<MapView environment={environment} sun={{ azimuthDeg: 200, altitudeDeg: 40 }} />);
    await waitFor(() => expect(markerInstances.length).toBeGreaterThan(0));

    const dueTo = environment.air_quality.stations.find((s) => s.index.due_to)?.index.due_to;
    expect(dueTo).toBeTruthy();
    expect(markerInstances.some((m) => m.element.textContent?.includes(dueTo!))).toBe(true);
  });
});

describe("resolveFitPadding (polish: the mobile beacon-clipping bug)", () => {
  // The desktop padding's right:340 accounts for the fixed-position state
  // dock, which only exists at >=48rem (768px) — DESIGN.md §3. Below that,
  // the HUD reverts to normal document flow above/below the map, so
  // applying that same padding squeezed a 390px-wide viewport's usable
  // fitBounds area down to ~2px, zooming out to fit the whole country
  // instead of Córdoba (the reported bug: "the left beacon is cut at the
  // screen edge").
  it("uses the generous HUD-aware padding at/above the 768px breakpoint", () => {
    const padding = resolveFitPadding(1440);
    expect(padding.right).toBeGreaterThan(200);
  });

  it("uses a small, symmetric padding below the 768px breakpoint — no fixed-position HUD to clear", () => {
    const padding = resolveFitPadding(390);
    expect(padding.right).toBeLessThan(100);
    expect(padding.left).toBeLessThan(100);
    // Symmetric, unlike desktop's HUD-shaped asymmetry: nothing on mobile
    // singles out one edge over another.
    expect(padding.left).toBe(padding.right);
    expect(padding.top).toBe(padding.bottom);
  });

  it("leaves enough room on a 390px-wide viewport for an actual map area to fit bounds into", () => {
    const padding = resolveFitPadding(390);
    expect(padding.left + padding.right).toBeLessThan(390 * 0.5);
  });
});

describe("MapView: historic-centre camera preset (parent review: 'shadows are invisible at city zoom')", () => {
  afterEach(() => {
    mapInstances.length = 0;
    markerInstances.length = 0;
  });

  it("flies to the historic centre at a close, steeply pitched zoom when the preset is chosen", async () => {
    render(<MapView environment={environment} sun={{ azimuthDeg: 200, altitudeDeg: 40 }} />);
    await waitFor(() => expect(mapInstances).toHaveLength(1));
    const instance = mapInstances[0] as unknown as { flyTo: ReturnType<typeof vi.fn> };

    await userEvent.click(screen.getByRole("button", { name: /historic/i }));

    expect(instance.flyTo).toHaveBeenCalledTimes(1);
    const [options] = instance.flyTo.mock.calls[0]!;
    expect(options.zoom).toBeGreaterThanOrEqual(16);
    expect(options.pitch).toBeGreaterThanOrEqual(55);
    expect(options.center).toEqual([-4.7794, 37.8789]); // the Mezquita / historic-centre point
  });

  it("flies back to the station overview via fitBounds when 'overview' is chosen again", async () => {
    render(<MapView environment={environment} sun={{ azimuthDeg: 200, altitudeDeg: 40 }} />);
    await waitFor(() => expect(mapInstances).toHaveLength(1));
    const instance = mapInstances[0] as unknown as {
      fitBounds: ReturnType<typeof vi.fn>;
      flyTo: ReturnType<typeof vi.fn>;
    };
    instance.fitBounds.mockClear(); // clear the initial-mount call

    await userEvent.click(screen.getByRole("button", { name: /historic/i }));
    await userEvent.click(screen.getByRole("button", { name: /overview|general/i }));

    expect(instance.fitBounds).toHaveBeenCalledTimes(1);
  });

  it("marks the active preset with aria-pressed, toggle-group style", async () => {
    render(<MapView environment={environment} sun={{ azimuthDeg: 200, altitudeDeg: 40 }} />);
    await waitFor(() => expect(mapInstances).toHaveLength(1));

    const overviewButton = screen.getByRole("button", { name: /overview|general/i });
    const historicButton = screen.getByRole("button", { name: /historic/i });
    expect(overviewButton).toHaveAttribute("aria-pressed", "true");
    expect(historicButton).toHaveAttribute("aria-pressed", "false");

    await userEvent.click(historicButton);

    expect(overviewButton).toHaveAttribute("aria-pressed", "false");
    expect(historicButton).toHaveAttribute("aria-pressed", "true");
  });
});

describe("MapView: collapsible legend (parent review: 'the bottom-left legend is clipped by the timeline bar')", () => {
  afterEach(() => {
    mapInstances.length = 0;
    markerInstances.length = 0;
  });

  it("renders the legend body as a native <details>/<summary> disclosure", () => {
    render(<MapView environment={environment} sun={{ azimuthDeg: 200, altitudeDeg: 40 }} />);

    const details = document.querySelector(".map-legend__disclosure");
    expect(details?.tagName.toLowerCase()).toBe("details");
    expect(screen.getByText(/legend|leyenda/i)).toBeInTheDocument();
  });

  it("toggles open/closed when the summary is activated", async () => {
    render(<MapView environment={environment} sun={{ azimuthDeg: 200, altitudeDeg: 40 }} />);

    const summary = screen.getByText(/legend|leyenda/i);
    const details = summary.closest("details")!;
    const initiallyOpen = details.hasAttribute("open");

    await userEvent.click(summary);

    expect(details.hasAttribute("open")).toBe(!initiallyOpen);
  });

  it("keeps the camera-preset controls visible regardless of the legend's open/closed state", async () => {
    render(<MapView environment={environment} sun={{ azimuthDeg: 200, altitudeDeg: 40 }} />);

    const summary = screen.getByText(/legend|leyenda/i);
    const details = summary.closest("details")!;
    if (details.hasAttribute("open")) {
      await userEvent.click(summary); // force closed
    }

    expect(screen.getByRole("button", { name: /historic/i })).toBeVisible();
  });
});
