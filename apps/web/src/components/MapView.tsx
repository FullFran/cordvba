import maplibregl from "maplibre-gl";
import "maplibre-gl/dist/maplibre-gl.css";
import { useEffect, useRef, useState } from "react";

import { useLocale } from "../i18n/LocaleContext";
import { prefersReducedMotion } from "../lib/motion";
import { formatAirQualityCategory } from "../lib/format";
import { createBeaconElement, createEdgeIndicatorElement } from "../map/beacons";
import { loadDarkStyle } from "../map/darkStyle";
import { clampToEdge, haversineDistanceKm } from "../map/geo";
import type { EnvironmentResponse } from "../types/environment";
import { WindParticles } from "./WindParticles";

/** The historic centre of Córdoba, folded into the initial camera bounds alongside the stations (issue #119). */
const CORDOBA_HISTORIC_CENTRE: [number, number] = [-4.7794, 37.8789];
const INITIAL_PITCH = 55;
const INITIAL_BEARING = -17;
/** Generous, HUD-aware padding (issue #119, maintainer review): the state dock (right), scrubber (bottom) and header overlay (top) must never cover a station. */
const FIT_PADDING = { top: 180, bottom: 150, left: 48, right: 340 };
const EDGE_MARGIN = 40;

export interface MapViewProps {
  environment: EnvironmentResponse;
  /** The station id to visually call out, e.g. from a selected timeline point. */
  highlightStationId?: string;
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
export function MapView({ environment, highlightStationId }: MapViewProps) {
  const { locale, t } = useLocale();
  const stageRef = useRef<HTMLDivElement | null>(null);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [map, setMap] = useState<maplibregl.Map | null>(null);
  const [mapError, setMapError] = useState<string | null>(null);

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

    loadDarkStyle()
      .then((style) => {
        if (cancelled) {
          return;
        }
        instance = new maplibregl.Map({
          container,
          style: style as unknown as maplibregl.StyleSpecification,
          center: bounds.getCenter(),
          zoom: 14,
          pitch: INITIAL_PITCH,
          bearing: INITIAL_BEARING,
          attributionControl: false,
        });
        instance.fitBounds(bounds, {
          padding: FIT_PADDING,
          pitch: INITIAL_PITCH,
          bearing: INITIAL_BEARING,
          duration: prefersReducedMotion() ? 0 : 1200,
        });
        instance.addControl(new maplibregl.NavigationControl({ visualizePitch: true }), "top-right");
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
        },
        locale,
      );
      return new maplibregl.Marker({ element }).setLngLat([station.lon, station.lat]).addTo(map);
    });

    return () => {
      markers.forEach((marker) => marker.remove());
    };
  }, [map, environment.air_quality.stations, highlightStationId, locale]);

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
      { kind: "wind", name: airport.name, directionDeg: windDirectionDeg, speedMs: environment.weather.wind_speed.value },
      locale,
    );
    const marker = new maplibregl.Marker({ element: beaconElement }).setLngLat([airport.lon, airport.lat]).addTo(map);

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
      marker.remove();
      edgeElement.remove();
    };
  }, [map, environment.weather, locale]);

  const windDirectionDeg =
    typeof environment.weather.wind_direction.value === "number"
      ? environment.weather.wind_direction.value
      : null;

  return (
    <div className="map-view">
      <div className="map-view__stage" role="region" aria-label={t.map.ariaLabel} ref={stageRef}>
        <div ref={containerRef} className="map-view__canvas" />
        {map ? (
          <WindParticles
            map={map}
            directionDeg={windDirectionDeg}
            speedMs={environment.weather.wind_speed.value}
            label={t.wind.label}
            toggleLabel={t.wind.toggle}
          />
        ) : null}
      </div>
      {mapError ? (
        <p className="unavailable-notice">
          {t.unavailable.mapPrefix} {mapError}
        </p>
      ) : null}
      <p className="map-view__caption">{t.map.caption}</p>
      {/* visually-hidden text summary, for anyone who cannot read the canvas at all */}
      <p className="visually-hidden">
        {environment.air_quality.stations
          .map((s) => `${s.name}: ${formatAirQualityCategory(s.index.category, locale)}`)
          .join(". ")}
      </p>
    </div>
  );
}
