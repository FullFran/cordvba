import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import environmentFixture from "@contracts/environment/v1/environment.example.json";
import type { EnvironmentResponse } from "../types/environment";

const environment = environmentFixture as unknown as EnvironmentResponse;

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
    project = vi.fn().mockReturnValue({ x: 400, y: 200 });
    getContainer = vi.fn(() => {
      const div = document.createElement("div");
      // jsdom's clientWidth/clientHeight are read-only getters; shadow them
      // with own properties instead of assigning (which throws).
      Object.defineProperty(div, "clientWidth", { value: 800, configurable: true });
      Object.defineProperty(div, "clientHeight", { value: 400, configurable: true });
      return div;
    });
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
}));

import { MapView } from "./MapView";

describe("MapView (issue 119: MapLibre dark 3D map + beacons)", () => {
  afterEach(() => {
    mapInstances.length = 0;
    markerInstances.length = 0;
  });

  it("has an accessible label and shows the honesty caption immediately, before the map style loads", () => {
    render(<MapView environment={environment} />);

    expect(screen.getByLabelText(/Map of Córdoba/i)).toBeInTheDocument();
    expect(screen.getByText(/no interpolated surface/i)).toBeInTheDocument();
  });

  it("creates the MapLibre map pitched over Córdoba once the dark style resolves", async () => {
    render(<MapView environment={environment} />);

    await waitFor(() => expect(mapInstances).toHaveLength(1));

    const [instance] = mapInstances;
    expect(instance?.options.pitch).toBeGreaterThanOrEqual(45);
    expect(instance?.options.pitch).toBeLessThanOrEqual(60);
    const center = instance?.options.center as { lng: number; lat: number };
    expect(typeof center.lng).toBe("number");
    expect(typeof center.lat).toBe("number");
  });

  it("fits the camera to the city stations plus the historic centre, not a fixed zoom (issue 119, maintainer review)", async () => {
    render(<MapView environment={environment} />);

    await waitFor(() => expect(mapInstances).toHaveLength(1));
    const instance = mapInstances[0] as unknown as { fitBounds: ReturnType<typeof vi.fn> };
    expect(instance.fitBounds).toHaveBeenCalledTimes(1);
    const [, fitOptions] = instance.fitBounds.mock.calls[0] as [unknown, Record<string, unknown>];
    expect(fitOptions.padding).toBeDefined();
  });

  it("creates one beacon marker per air-quality station plus one for the airport weather station", async () => {
    render(<MapView environment={environment} />);

    await waitFor(() =>
      expect(markerInstances).toHaveLength(environment.air_quality.stations.length + 1),
    );
  });

  it("gives each air-quality beacon marker an element carrying the category as visible text (AC-2: never colour alone)", async () => {
    render(<MapView environment={environment} />);

    await waitFor(() => expect(markerInstances.length).toBeGreaterThan(0));

    const categories = environment.air_quality.stations.map((s) => s.index.category);
    const beaconTexts = markerInstances.map((m) => m.element.textContent ?? "");
    for (const category of categories) {
      expect(beaconTexts.some((text) => text.includes(category))).toBe(true);
    }
  });

  it("removes the map on unmount", async () => {
    const { unmount } = render(<MapView environment={environment} />);
    await waitFor(() => expect(mapInstances).toHaveLength(1));

    unmount();

    expect(mapInstances[0]?.remove).toHaveBeenCalled();
  });

  it("shows the illustrative-wind toggle once the map is ready", async () => {
    render(<MapView environment={environment} />);

    expect(await screen.findByRole("checkbox")).toBeInTheDocument();
    expect(screen.getAllByText(/Illustrative wind/i).length).toBeGreaterThan(0);
  });

  it("shows an edge-clamped distance indicator for the airport when it projects outside the viewport", async () => {
    const { container } = render(<MapView environment={environment} />);
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
