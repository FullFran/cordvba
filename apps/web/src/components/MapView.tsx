import maplibregl from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
import { useEffect, useId, useRef, useState } from "react";

import { useLocale } from "../i18n/LocaleContext";
import { prefersReducedMotion } from "../lib/motion";
import { formatAirQualityCategory } from "../lib/format";
import { sunPhase, type SunPosition } from "../lib/sun";
import { stationColumnFeature, stationGlowFeature } from "../map/airQualityColumns";
import { breathingDurationMs, createBeaconElement, createEdgeIndicatorElement, windArrowRotation } from "../map/beacons";
import { loadDarkStyle, readMapPalette } from "../map/darkStyle";
import { clampToEdge, haversineDistanceKm } from "../map/geo";
import { buildingShadow, footprintFromGeometry, shadowToGeoJSONCoordinates } from "../map/shadows";
import { sunLight } from "../map/sunLight";
import type { EnvironmentResponse } from "../types/environment";
import { WindParticles } from "./WindParticles";

/** The historic centre of Córdoba, folded into the initial camera bounds alongside the stations (issue #119). */
const CORDOBA_HISTORIC_CENTRE: [number, number] = [-4.7794, 37.8789];
/**
 * Pitch/bearing (issue #119, maintainer review: "3D is barely
 * perceptible"): a steeper pitch and a bearing away from due north/east
 * shows building facades, not just rooftops from directly above.
 */
const INITIAL_PITCH = 58;
const INITIAL_BEARING = -35;
/**
 * The "Casco histórico / Historic centre" camera preset (parent review:
 * "shadows are invisible at city zoom"). The station-framed overview
 * camera is deliberately wide (it has to fit three spread-out stations
 * plus the centre); building shadows a few tens of metres long simply
 * do not read at that scale. This preset flies close over the Mezquita
 * — `CORDOBA_HISTORIC_CENTRE` — steeply pitched, where a shadow spans a
 * meaningful fraction of the frame.
 */
const HISTORIC_PRESET_ZOOM = 16.5;
const HISTORIC_PRESET_PITCH = 60;
type CameraPreset = "overview" | "historic";
/** Generous, HUD-aware padding (issue #119, maintainer review): the state dock (right), scrubber (bottom) and header overlay (top) must never cover a station. Only applies at/above the `MOBILE_BREAKPOINT_PX` — see `resolveFitPadding`. */
const DESKTOP_FIT_PADDING = { top: 180, bottom: 150, left: 48, right: 340 };
/**
 * Small, symmetric padding for <48rem (polish, maintainer report: "the
 * left beacon is cut at the screen edge"). Below that breakpoint the HUD
 * reverts to normal document flow above/below the map (DESIGN.md §3) — no
 * fixed-position panel sits inside the map's own viewport to clear, so
 * the desktop padding's asymmetric, HUD-shaped margins (right: 340 alone
 * eating most of a 390px-wide screen) have nothing left to justify them
 * and only starved `fitBounds` of room, zooming out to fit the whole
 * country instead of Córdoba.
 */
const MOBILE_FIT_PADDING = { top: 40, bottom: 40, left: 24, right: 24 };
/** Matches DESIGN.md §3's one layout breakpoint, 48rem at the default 16px root font size. */
const MOBILE_BREAKPOINT_PX = 768;

/** Picks the desktop or mobile `fitBounds` padding for the given viewport width (polish: see `MOBILE_FIT_PADDING`'s own comment for why these must differ). */
export function resolveFitPadding(viewportWidthPx: number): typeof DESKTOP_FIT_PADDING {
  return viewportWidthPx >= MOBILE_BREAKPOINT_PX ? DESKTOP_FIT_PADDING : MOBILE_FIT_PADDING;
}

const EDGE_MARGIN = 40;

const SHADOW_SOURCE_ID = "building-shadows";
const SHADOW_LAYER_ID = "building-shadows-layer";
const AQ_COLUMNS_SOURCE_ID = "air-quality-columns";
const AQ_COLUMNS_LAYER_ID = "air-quality-columns-layer";
const AQ_GLOW_SOURCE_ID = "air-quality-glow";
const AQ_GLOW_LAYER_ID = "air-quality-glow-layer";
/** Flat pixel radius (parent review: "a soft glow at the base"); `circle-radius` is screen pixels, not metres, so this does not scale with the column's real-world 130m footprint. */
const AQ_GLOW_BASE_RADIUS_PX = 34;
const AQ_GLOW_MIN_OPACITY = 0.18;
const AQ_GLOW_MAX_OPACITY = 0.5;
const AQ_GLOW_MID_OPACITY = (AQ_GLOW_MIN_OPACITY + AQ_GLOW_MAX_OPACITY) / 2;
const EMPTY_FEATURE_COLLECTION: GeoJSON.FeatureCollection = { type: "FeatureCollection", features: [] };

/**
 * The ICA category → token-colour `match` expression shared by the
 * air-quality columns and their glow (parent review: same category,
 * same colour, in both layers). Reads `--color-aq-*` at call time, never
 * a literal in this module — see `readMapPalette`'s own convention.
 */
function aqCategoryColorMatch(): maplibregl.ExpressionSpecification {
  const style = getComputedStyle(document.documentElement);
  const read = (name: string, fallback: string) => style.getPropertyValue(name).trim() || fallback;
  // ds-allow-hardcode:start (runtime CSS-variable fallback, same convention as darkStyle.ts's readMapPalette)
  return [
    "match",
    ["get", "category"],
    "good",
    read("--color-aq-good", "#34d399"),
    "fair",
    read("--color-aq-fair", "#a3e635"),
    "moderate",
    read("--color-aq-moderate", "#facc15"),
    "poor",
    read("--color-aq-poor", "#fb923c"),
    "very_poor",
    read("--color-aq-very-poor", "#f87171"),
    "extremely_poor",
    read("--color-aq-extremely-poor", "#e879f9"),
    read("--color-aq-moderate", "#facc15"),
  ];
  // ds-allow-hardcode:end
}
/** Reads the tokens.css light-tint variable for a given sun phase (issue #139); falls back to a sane default so a missing token never breaks `map.setLight`. */
// ds-allow-hardcode:start (runtime CSS-variable fallback, same convention as darkStyle.ts's readMapPalette)
const LIGHT_TOKEN_FALLBACK: Record<ReturnType<typeof sunPhase>, string> = {
  day: "#cfe0f5",
  golden: "#ffb066",
  blue: "#7b93e0",
  night: "#2a3550",
};
// ds-allow-hardcode:end

export interface MapViewProps {
  environment: EnvironmentResponse;
  /** The station id to visually call out, e.g. from a selected timeline point. */
  highlightStationId?: string;
  /** The sun's position at the timeline's selected time (issue #139), computed once by `App.tsx` from the same `getSunPosition` call the sun widget uses — drives this map's lighting, day/dusk/night palette and building shadows. */
  sun: SunPosition;
}

/**
 * Córdoba as a dark 3D city twin (issue #119): MapLibre GL over OpenFreeMap
 * vector tiles, recoloured dark and decluttered of POI/transit noise
 * (`src/map/darkStyle.ts`), pitched over the historic centre so the real
 * building extrusions read as a skyline. The initial camera fits the three
 * city stations plus the historic centre (never a fixed center/zoom, since
 * real station coordinates can differ from the fixtures). City air-quality
 * stations and the airport weather station are beacons: glow, rings,
 * category/value as text and shape, never an interpolated surface (AC-2).
 * When the airport falls outside the viewport, an edge-clamped arrow +
 * distance replaces it rather than it silently vanishing. The illustrative
 * wind particle field lives alongside it, wired to the single METAR
 * reading.
 */
export function MapView({ environment, highlightStationId, sun }: MapViewProps) {
  const { locale, t } = useLocale();
  const stageRef = useRef<HTMLDivElement | null>(null);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [map, setMap] = useState<maplibregl.Map | null>(null);
  const [mapError, setMapError] = useState<string | null>(null);
  const [windEnabled, setWindEnabled] = useState(true);
  const windToggleId = useId();
  const initialSunRef = useRef(sun);
  const overviewBoundsRef = useRef<maplibregl.LngLatBounds | null>(null);
  const [cameraPreset, setCameraPreset] = useState<CameraPreset>("overview");

  useEffect(() => {
    const container = containerRef.current;
    if (!container) {
      return undefined;
    }
    let cancelled = false;
    let instance: maplibregl.Map | undefined;

    const bounds = environment.air_quality.stations.reduce(
      (b, s) => b.extend([s.lon, s.lat]),
      new maplibregl.LngLatBounds(CORDOBA_HISTORIC_CENTRE, CORDOBA_HISTORIC_CENTRE),
    );
    overviewBoundsRef.current = bounds;

    loadDarkStyle(undefined, sunPhase(initialSunRef.current.altitudeDeg))
      .then((style) => {
        if (cancelled) {
          return;
        }
        // A default paint-property transition (issue #139): scrubbing the
        // timeline calls `setPaintProperty`/`setLight` below, and without
        // this the sky/water/light snap instantly on every tick. Zeroed
        // under `prefers-reduced-motion`, same as every other animation in
        // this app (see DESIGN.md §5).
        const styleWithTransition = {
          ...style,
          transition: { duration: prefersReducedMotion() ? 0 : 800, delay: 0 },
        };
        instance = new maplibregl.Map({
          container,
          style: styleWithTransition as unknown as maplibregl.StyleSpecification,
          center: bounds.getCenter(),
          zoom: 14,
          pitch: INITIAL_PITCH,
          bearing: INITIAL_BEARING,
          attributionControl: false,
        });
        instance.fitBounds(bounds, {
          padding: resolveFitPadding(container.clientWidth),
          pitch: INITIAL_PITCH,
          bearing: INITIAL_BEARING,
          duration: prefersReducedMotion() ? 0 : 1200,
        });
        // bottom-right, not top-right (issue #119, maintainer review): the
        // state dock HUD panel already occupies the top-right corner on
        // desktop and was covering this control. styles.css offsets
        // `.maplibregl-ctrl-bottom-right` above the bottom scrubber bar.
        instance.addControl(new maplibregl.NavigationControl({ visualizePitch: true }), "bottom-right");
        setMap(instance);
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setMapError(err instanceof Error ? err.message : "failed to load the map style");
        }
      });

    return () => {
      cancelled = true;
      instance?.remove();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- created once; station coordinates arrive with the first environment payload and camera framing shouldn't re-fly on every re-render.
  }, []);

  // City air-quality station beacons.
  useEffect(() => {
    if (!map) {
      return undefined;
    }

    const markers = environment.air_quality.stations.map((station) => {
      const element = createBeaconElement(
        {
          kind: "air-quality",
          name: station.name,
          category: station.index.category,
          index: station.index.value,
          highlighted: station.id === highlightStationId,
          pollutant: station.index.due_to,
        },
        locale,
      );
      return new maplibregl.Marker({ element }).setLngLat([station.lon, station.lat]).addTo(map);
    });

    return () => {
      markers.forEach((marker) => marker.remove());
    };
  }, [map, environment.air_quality.stations, highlightStationId, locale]);

  // Air quality as columns of light (issue #140): one small extruded disc
  // per city station, its height encoding the ICA index and its colour
  // following the category tokens — never an interpolated surface between
  // the three stations (DESIGN.md §7's standing rule; see the legend line
  // below). Static per-station data (unlike the sun layers above, this
  // does not depend on `atTime`), so it is only rebuilt when the stations
  // themselves change.
  useEffect(() => {
    if (!map) {
      return undefined;
    }

    function ensureColumnsLayer() {
      if (!map) return;
      if (!map.getSource(AQ_COLUMNS_SOURCE_ID)) {
        map.addSource(AQ_COLUMNS_SOURCE_ID, { type: "geojson", data: EMPTY_FEATURE_COLLECTION });
      }
      if (!map.getLayer(AQ_COLUMNS_LAYER_ID)) {
        const colorMatch = aqCategoryColorMatch();
        map.addLayer({
          id: AQ_COLUMNS_LAYER_ID,
          type: "fill-extrusion",
          source: AQ_COLUMNS_SOURCE_ID,
          paint: {
            "fill-extrusion-color": colorMatch,
            "fill-extrusion-height": ["get", "height"],
            "fill-extrusion-base": 0,
            // Hero scale (parent review): opaque enough that the column
            // reads as the frame's focal point, not a translucent haze;
            // the vertical gradient (darker base, lit top) is what makes
            // a flat-shaded extrusion actually read as a "column of
            // light" rather than a plain coloured block.
            "fill-extrusion-opacity": 0.9,
            "fill-extrusion-vertical-gradient": true,
          },
        });
      }
      if (!map.getSource(AQ_GLOW_SOURCE_ID)) {
        map.addSource(AQ_GLOW_SOURCE_ID, { type: "geojson", data: EMPTY_FEATURE_COLLECTION });
      }
      if (!map.getLayer(AQ_GLOW_LAYER_ID)) {
        const colorMatch = aqCategoryColorMatch();
        map.addLayer(
          {
            id: AQ_GLOW_LAYER_ID,
            type: "circle",
            source: AQ_GLOW_SOURCE_ID,
            paint: {
              "circle-color": colorMatch,
              "circle-radius": ["*", AQ_GLOW_BASE_RADIUS_PX, ["get", "radiusScale"]],
              "circle-opacity": ["get", "opacity"],
              "circle-blur": 1,
            },
          },
          // Drawn below the extrusion layer, so the glow reads as a soft
          // pool of light the column rises out of, not a halo painted
          // over its face.
          AQ_COLUMNS_LAYER_ID,
        );
      }
    }

    function updateColumns() {
      if (!map) return;
      const source = map.getSource(AQ_COLUMNS_SOURCE_ID) as maplibregl.GeoJSONSource | undefined;
      if (!source) return;
      const features = environment.air_quality.stations.map((station) =>
        stationColumnFeature({
          lat: station.lat,
          lon: station.lon,
          category: station.index.category,
          index: station.index.value,
        }),
      );
      source.setData({ type: "FeatureCollection", features });
    }

    // The breathing glow (issue #140 AC, parent review: "a soft glow ...
    // that breathes"): slower for good air, faster for poor — the exact
    // same per-category rate as the DOM beacon's own breathing rings
    // (`breathingDurationMs`), so the two readings of "how urgent is
    // this" agree. Animated by rewriting the glow source's own feature
    // properties every frame (three tiny point features — cheap), not by
    // an unsupported zoom-expression trick; frozen to one calm static
    // frame under `prefers-reduced-motion`.
    function glowFeaturesAt(nowMs: number): GeoJSON.FeatureCollection {
      const features = environment.air_quality.stations.map((station) => {
        const category = station.index.category;
        let opacity = AQ_GLOW_MID_OPACITY;
        let radiusScale = 1;
        if (!prefersReducedMotion()) {
          const durationMs = breathingDurationMs(category);
          const phase = (nowMs % durationMs) / durationMs;
          const wave = (Math.sin(phase * 2 * Math.PI) + 1) / 2; // 0..1
          opacity = AQ_GLOW_MIN_OPACITY + wave * (AQ_GLOW_MAX_OPACITY - AQ_GLOW_MIN_OPACITY);
          radiusScale = 1 + wave * 0.25;
        }
        return stationGlowFeature(
          { lat: station.lat, lon: station.lon, category, index: station.index.value },
          opacity,
          radiusScale,
        );
      });
      return { type: "FeatureCollection", features };
    }

    let glowFrameHandle = 0;
    function stepGlow(nowMs: number) {
      if (!map) return;
      const source = map.getSource(AQ_GLOW_SOURCE_ID) as maplibregl.GeoJSONSource | undefined;
      if (source) {
        source.setData(glowFeaturesAt(nowMs));
      }
      if (!prefersReducedMotion()) {
        glowFrameHandle = requestAnimationFrame(stepGlow);
      }
    }

    function setUpAndStart() {
      ensureColumnsLayer();
      updateColumns();
      stepGlow(performance.now());
    }

    if (map.isStyleLoaded()) {
      setUpAndStart();
    } else {
      map.once("load", setUpAndStart);
    }

    return () => {
      cancelAnimationFrame(glowFrameHandle);
    };
  }, [map, environment.air_quality.stations]);

  // The airport weather beacon, always at its true position; a manually
  // positioned edge indicator (not a MapLibre Marker, which cannot be
  // clamped) takes over visually whenever that position falls outside the
  // viewport (issue #119, maintainer review).
  useEffect(() => {
    if (!map) {
      return undefined;
    }
    const stage = stageRef.current;
    if (!stage) {
      return undefined;
    }

    const airport = environment.weather.station;
    const windDirectionDeg =
      typeof environment.weather.wind_direction.value === "number"
        ? environment.weather.wind_direction.value
        : null;

    const beaconElement = createBeaconElement(
      {
        kind: "wind",
        name: airport.name,
        directionDeg: windDirectionDeg,
        speedMs: environment.weather.wind_speed.value,
        mapBearingDeg: map.getBearing(),
      },
      locale,
    );
    const marker = new maplibregl.Marker({ element: beaconElement }).setLngLat([airport.lon, airport.lat]).addTo(map);

    // The arrow is a plain DOM element, not something MapLibre rotates with
    // the map canvas, so it must be re-corrected every time the visitor
    // rotates the map (issue #138) — otherwise a bearing change silently
    // rotates the arrow's *meaning* along with the view.
    function updateWindArrowRotation() {
      if (!map) return;
      const arrow = beaconElement.querySelector<HTMLElement>(".beacon__arrow");
      if (arrow) {
        arrow.style.transform = `rotate(${windArrowRotation(windDirectionDeg, map.getBearing())}deg)`;
      }
    }
    map.on("rotate", updateWindArrowRotation);

    const distanceKm = haversineDistanceKm(
      { lat: CORDOBA_HISTORIC_CENTRE[1], lon: CORDOBA_HISTORIC_CENTRE[0] },
      { lat: airport.lat, lon: airport.lon },
    );
    const edgeElement = createEdgeIndicatorElement(airport.name, distanceKm, 0, locale);
    edgeElement.style.position = "absolute";
    edgeElement.style.display = "none";
    stage.appendChild(edgeElement);

    function updateEdgeIndicator() {
      if (!map) return;
      const container = map.getContainer();
      const point = map.project([airport.lon, airport.lat]);
      const center = { x: container.clientWidth / 2, y: container.clientHeight / 2 };
      const result = clampToEdge(
        point,
        center,
        { width: container.clientWidth, height: container.clientHeight },
        EDGE_MARGIN,
      );

      if (!result.clamped) {
        edgeElement.style.display = "none";
        return;
      }
      edgeElement.style.display = "flex";
      edgeElement.style.left = `${result.x}px`;
      edgeElement.style.top = `${result.y}px`;
      edgeElement.style.transform = "translate(-50%, -50%)";
      const arrow = edgeElement.querySelector<HTMLElement>(".beacon__arrow");
      if (arrow) {
        arrow.style.transform = `rotate(${result.angleDeg + 90}deg)`;
      }
    }

    updateEdgeIndicator();
    map.on("move", updateEdgeIndicator);
    map.on("resize", updateEdgeIndicator);

    return () => {
      map.off("move", updateEdgeIndicator);
      map.off("resize", updateEdgeIndicator);
      map.off("rotate", updateWindArrowRotation);
      marker.remove();
      edgeElement.remove();
    };
  }, [map, environment.weather, locale]);

  // Sun-driven lighting and day/dusk/night base-map palette (issue #139).
  // Cheap enough to run directly whenever the sun's position changes
  // (a handful of setPaintProperty/setLight calls); no throttling needed
  // here, unlike the shadow recompute below.
  useEffect(() => {
    if (!map) {
      return undefined;
    }

    function applyLighting() {
      if (!map) return;
      const phase = sunPhase(sun.altitudeDeg);
      const palette = readMapPalette(document.documentElement, phase);
      map.setPaintProperty("background", "background-color", palette.background);
      map.setPaintProperty("water", "fill-color", palette.water);

      const style = getComputedStyle(document.documentElement);
      const lightColor =
        style.getPropertyValue(`--map-light-${phase}`).trim() || LIGHT_TOKEN_FALLBACK[phase];
      map.setLight(sunLight(sun, phase, lightColor));
    }

    if (map.isStyleLoaded()) {
      applyLighting();
    } else {
      map.once("load", applyLighting);
    }

    return () => {
      map.off("load", applyLighting);
    };
  }, [map, sun.azimuthDeg, sun.altitudeDeg]);

  // Historic-centre building shadows (issue #139): an inferred, translucent
  // layer projecting each currently rendered building-3d footprint along
  // the sun's shadow vector (`src/map/shadows.ts`). Recomputed whenever the
  // sun's position changes and whenever the camera settles ("moveend" —
  // which, unlike "move", fires once per gesture rather than every frame,
  // the throttle this layer needs) — see DESIGN.md for the trade-offs
  // (main-thread, convex-hull approximation) this scope accepted. Hidden
  // once the sun is at or below the horizon (AC-3).
  useEffect(() => {
    if (!map) {
      return undefined;
    }

    function ensureShadowLayer() {
      if (!map) return;
      if (!map.getSource(SHADOW_SOURCE_ID)) {
        map.addSource(SHADOW_SOURCE_ID, { type: "geojson", data: EMPTY_FEATURE_COLLECTION });
      }
      if (!map.getLayer(SHADOW_LAYER_ID)) {
        const shadowFill =
          getComputedStyle(document.documentElement).getPropertyValue("--map-shadow-fill").trim() ||
          "rgba(3, 6, 12, 0.6)"; // ds-allow-hardcode (runtime CSS-variable fallback, same convention as darkStyle.ts's readMapPalette)
        map.addLayer({
          id: SHADOW_LAYER_ID,
          type: "fill",
          source: SHADOW_SOURCE_ID,
          paint: { "fill-color": shadowFill, "fill-opacity": 1 },
        });
      }
    }

    function recomputeShadows() {
      if (!map) return;
      const source = map.getSource(SHADOW_SOURCE_ID) as maplibregl.GeoJSONSource | undefined;
      if (!source) return;

      if (sun.altitudeDeg <= 0) {
        source.setData(EMPTY_FEATURE_COLLECTION);
        return;
      }

      performance.mark("cordvba-shadow-compute-start");
      const rendered = map.queryRenderedFeatures(undefined, { layers: ["building-3d"] });
      const features: GeoJSON.Feature[] = [];
      for (const feature of rendered) {
        const heightM = Number(feature.properties?.["render_height"]) || 0;
        const footprint = footprintFromGeometry(feature.geometry as { type: string; coordinates: unknown });
        if (!footprint) continue;
        const shadow = buildingShadow({ footprint, heightM }, sun);
        if (!shadow) continue;
        features.push({
          type: "Feature",
          properties: {},
          geometry: { type: "Polygon", coordinates: shadowToGeoJSONCoordinates(shadow) },
        });
      }
      source.setData({ type: "FeatureCollection", features });
      performance.mark("cordvba-shadow-compute-end");
      performance.measure(
        "cordvba-shadow-compute",
        "cordvba-shadow-compute-start",
        "cordvba-shadow-compute-end",
      );
    }

    function setUpAndRecompute() {
      ensureShadowLayer();
      recomputeShadows();
    }

    if (map.isStyleLoaded()) {
      setUpAndRecompute();
    } else {
      map.once("load", setUpAndRecompute);
    }
    map.on("moveend", recomputeShadows);

    return () => {
      map.off("load", setUpAndRecompute);
      map.off("moveend", recomputeShadows);
    };
    // Re-registering "moveend" on every sun change is cheap (a handful of
    // times per timeline interaction, never per animation frame) and keeps
    // `recomputeShadows` a plain closure over the current `sun` prop rather
    // than needing a ref to stay fresh.
  }, [map, sun.azimuthDeg, sun.altitudeDeg]);

  const windDirectionDeg =
    typeof environment.weather.wind_direction.value === "number"
      ? environment.weather.wind_direction.value
      : null;

  // Camera presets (parent review): "Casco histórico / Historic centre"
  // flies close over the Mezquita where building shadows actually read;
  // "Vista general / Overview" returns to the original station-fitted
  // framing via the exact same `fitBounds` call the initial mount used.
  function flyToOverview() {
    if (!map || !overviewBoundsRef.current) return;
    map.fitBounds(overviewBoundsRef.current, {
      padding: resolveFitPadding(map.getContainer().clientWidth),
      pitch: INITIAL_PITCH,
      bearing: INITIAL_BEARING,
      duration: prefersReducedMotion() ? 0 : 1500,
    });
    setCameraPreset("overview");
  }

  function flyToHistoricCentre() {
    if (!map) return;
    map.flyTo({
      center: CORDOBA_HISTORIC_CENTRE,
      zoom: HISTORIC_PRESET_ZOOM,
      pitch: HISTORIC_PRESET_PITCH,
      bearing: INITIAL_BEARING,
      duration: prefersReducedMotion() ? 0 : 1500,
    });
    setCameraPreset("historic");
  }

  return (
    <div className="map-view">
      <div className="map-view__stage" role="region" aria-label={t.map.ariaLabel} ref={stageRef}>
        <div ref={containerRef} className="map-view__canvas" />
        {map ? (
          <WindParticles
            map={map}
            directionDeg={windDirectionDeg}
            speedMs={environment.weather.wind_speed.value}
            enabled={windEnabled}
          />
        ) : null}

        {/* One merged legend (issue #119, maintainer review): the wind
            toggle/label and the extrusion-heights caption used to be two
            separate absolutely-positioned boxes that could overlap each
            other, or the wind label could go unnoticed on its own. Always
            rendered, not gated on `map`, so it is visible on every
            breakpoint regardless of load state. */}
        <div className="map-legend">
          {/* Camera presets (parent review: "shadows are invisible at city
              zoom"): the station-framed overview has to stay wide enough to
              fit three spread-out stations, so a close, legible view of the
              historic centre's building shadows needs its own camera jump,
              with a way back. */}
          <div className="map-camera-presets" role="group" aria-label={t.map.cameraPresetsLabel}>
            <button
              type="button"
              aria-pressed={cameraPreset === "overview"}
              onClick={flyToOverview}
            >
              {t.map.presetOverview}
            </button>
            <button
              type="button"
              aria-pressed={cameraPreset === "historic"}
              onClick={flyToHistoricCentre}
            >
              {t.map.presetHistoric}
            </button>
          </div>
          <label htmlFor={windToggleId} className="map-legend__toggle">
            <input
              id={windToggleId}
              type="checkbox"
              checked={windEnabled}
              onChange={(event) => setWindEnabled(event.target.checked)}
            />
            {t.wind.toggle}
          </label>
          {windEnabled ? <p className="map-legend__line">{t.wind.label}</p> : null}
          <p className="map-legend__line map-legend__line--muted">{t.map.caption}</p>
          <p className="map-legend__line map-legend__line--muted">{t.map.columnsLegend}</p>
          {sun.altitudeDeg > 0 ? (
            <p className="map-legend__line map-legend__line--muted">{t.sun.shadowLegend}</p>
          ) : null}
        </div>

        {mapError ? (
          <p className="unavailable-notice">
            {t.unavailable.mapPrefix} {mapError}
          </p>
        ) : null}
      </div>
      {/* visually-hidden text summary, for anyone who cannot read the canvas at all */}
      <p className="visually-hidden">
        {environment.air_quality.stations
          .map((s) => `${s.name}: ${formatAirQualityCategory(s.index.category, locale)}`)
          .join(". ")}
      </p>
    </div>
  );
}
