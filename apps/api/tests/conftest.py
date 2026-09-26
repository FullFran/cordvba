"""Shared test helpers: build eye_client.Source/Record objects without
going through HTTP, so api's business logic is tested in isolation.
"""

from eye_client import Record, Source


def make_eye_source(
    *,
    source_id: str,
    license_: str,
    authority: str = "Some Authority",
    last_success: str = "2026-09-26T09:35:40Z",
    consecutive_errors: int = 0,
    redistributable: bool = True,
) -> Source:
    return Source.model_validate(
        {
            "id": source_id,
            "authority": authority,
            "topic": "air_quality",
            "license": license_,
            "access": "documented_api",
            "automation": "enabled",
            "pollable": True,
            "redistributable": redistributable,
            "url": f"https://example.invalid/{source_id}",
            "last_attempt": last_success,
            "last_success": last_success,
            "records": 1,
            "consecutive_errors": consecutive_errors,
        }
    )


def make_metar_record(
    *,
    record_id: str = "metar-cordoba:LEBA:2026-09-26T09:00:00Z",
    observed_at: str = "2026-09-26T09:00:00Z",
    fetched_at: str = "2026-09-26T09:07:12Z",
    temperature_c: float = 26.0,
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
            "quality": "preliminary",
            "severity": 0,
            "confidence": 1.0,
            "local_key": "LEBA",
            "dedupe_key": record_id,
            "payload": {
                "icao": "LEBA",
                "station": "Córdoba Airport",
                "temperature_c": temperature_c,
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
