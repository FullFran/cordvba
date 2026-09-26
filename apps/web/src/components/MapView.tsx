import L from "leaflet";
import "leaflet/dist/leaflet.css";
import { MapContainer, Marker, Popup, TileLayer } from "react-leaflet";

import type { EnvironmentResponse } from "../types/environment";

const CORDOBA_CENTER: [number, number] = [37.8882, -4.7794];

const CATEGORY_ABBREVIATIONS: Record<string, string> = {
  good: "GD",
  fair: "FR",
  moderate: "MOD",
  poor: "PR",
  very_poor: "VPR",
  extremely_poor: "EPR",
};

function airQualityIcon(category: string, index: number | string | null, highlighted: boolean) {
  const abbrev = CATEGORY_ABBREVIATIONS[category] ?? category.slice(0, 3).toUpperCase();
  return L.divIcon({
    className: `aq-marker aq-marker--${category}${highlighted ? " aq-marker--highlighted" : ""}`,
    html: `<span class="aq-marker__index">${index ?? "?"}</span><span class="aq-marker__abbrev">${abbrev}</span>`,
    iconSize: [40, 40],
  });
}

function windIcon(directionDeg: number | string | null, speed: number | string | null) {
  const deg = typeof directionDeg === "number" ? directionDeg : 0;
  return L.divIcon({
    className: "wind-marker",
    html:
      `<span class="wind-marker__arrow" style="transform: rotate(${deg}deg)">&#8593;</span>` +
      `<span class="wind-marker__label">${deg}&deg; ${speed ?? "?"} m/s</span>`,
    iconSize: [56, 56],
  });
}

export interface MapViewProps {
  environment: EnvironmentResponse;
  /** The station id to visually call out, e.g. from a selected timeline point. */
  highlightStationId?: string;
}

/**
 * Córdoba, centred, with the city air-quality stations (category as text,
 * not colour alone) and the airport weather point with a wind-direction
 * arrow. No interpolated surface: three stations cannot support one
 * (issue #102).
 */
export function MapView({ environment, highlightStationId }: MapViewProps) {
  const { weather, air_quality } = environment;

  return (
    <MapContainer
      center={CORDOBA_CENTER}
      zoom={12}
      className="map-view"
      aria-label="Map of Córdoba"
      scrollWheelZoom={false}
    >
      <TileLayer
        url="https://tile.openstreetmap.org/{z}/{x}/{y}.png"
        attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
      />
      {air_quality.stations.map((station) => (
        <Marker
          key={station.id}
          position={[station.lat, station.lon]}
          icon={airQualityIcon(
            station.index.category,
            station.index.value,
            station.id === highlightStationId,
          )}
        >
          <Popup>
            <strong>{station.name}</strong>
            <br />
            Air quality: {station.index.category_source} ({station.index.category}), index{" "}
            {station.index.value}
          </Popup>
        </Marker>
      ))}
      <Marker
        position={[weather.station.lat, weather.station.lon]}
        icon={windIcon(weather.wind_direction.value, weather.wind_speed.value)}
      >
        <Popup>
          <strong>{weather.station.name}</strong>
          <br />
          Wind: {weather.wind_speed.value} m/s from {weather.wind_direction.value}&deg;
        </Popup>
      </Marker>
    </MapContainer>
  );
}
