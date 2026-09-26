"""Shared test helpers: build eye_client.Record objects without going
through HTTP, so twin's business logic is tested in isolation.
"""

from eye_client import Record


def make_metar_record(
    *,
    record_id: str = "metar-cordoba:LEBA:2026-09-26T09:00:00Z",
    observed_at: str = "2026-09-26T09:00:00Z",
    fetched_at: str = "2026-09-26T09:07:12Z",
    temperature_c: float = 26.0,
    dewpoint_c: float = 12.0,
    wind_speed_kt: float = 8.0,
    wind_direction_deg: int = 250,
    quality: str = "preliminary",
) -> Record:
    return Record.model_validate(
        {
            "id": record_id,
            "source": "metar-cordoba",
            "kind": "weather_observation",
            "topic": "weather",
            "title": "Córdoba Airport",
            "observed_at": observed_at,
            "fetched_at": fetched_at,
            "position": {"lat": 37.842, "lon": -4.8488},
            "quality": quality,
            "severity": 0,
            "confidence": 1.0,
            "local_key": "LEBA",
            "dedupe_key": record_id,
            "payload": {
                "icao": "LEBA",
                "station": "Córdoba Airport",
                "temperature_c": temperature_c,
                "dewpoint_c": dewpoint_c,
                "wind_speed_kt": wind_speed_kt,
                "wind_direction_deg": wind_direction_deg,
                "wind_gust_kt": None,
                "altimeter_hpa": 1018,
                "visibility": "6+",
                "cloud_cover": "CAVOK",
                "raw": "METAR LEBA ...",
            },
            "provenance": {
                "publisher": "NOAA Aviation Weather Center",
                "source_url": "https://aviationweather.gov/api/data/metar",
                "license": "us-government-public-domain",
                "fetched_at": fetched_at,
                "raw_hash": "hash",
            },
        }
    )


def make_ica_record(
    *,
    station: str,
    index: int,
    index_reported: bool = True,
    category: str = "",
    due_to: str = "",
    name: str = "STATION",
    lat: float = 37.9,
    lon: float = -4.78,
    observed_at: str = "2026-09-26T08:00:00Z",
    fetched_at: str = "2026-09-26T09:35:40Z",
    quality: str = "official",
    active: bool = True,
) -> Record:
    record_id = f"miteco-ica:{station}:{observed_at}"
    return Record.model_validate(
        {
            "id": record_id,
            "source": "miteco-ica",
            "kind": "air_quality_index",
            "topic": "air_quality",
            "title": f"{name}: {category or 'sin datos'}",
            "observed_at": observed_at,
            "fetched_at": fetched_at,
            "position": {"lat": lat, "lon": lon},
            "quality": quality,
            "severity": 0,
            "confidence": 1.0,
            "local_key": station,
            "dedupe_key": record_id,
            "payload": {
                "station": station,
                "index": index,
                "index_reported": index_reported,
                "category": category,
                "due_to": due_to,
                "active": active,
                "station_type": "FONDO",
            },
            "provenance": {
                "publisher": "MITECO",
                "source_url": "https://ica.miteco.es/datos/ica-ultima-hora.csv",
                "license": "CC-BY-4.0",
                "fetched_at": fetched_at,
                "raw_hash": "hash",
            },
        }
    )
