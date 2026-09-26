"""twin's FastAPI app: internal-only endpoints for environment state, an
air-quality forecast, and a thermal scenario. Never reached by the
browser directly; apps/api composes it. See services/twin/README.md.
"""

from __future__ import annotations

from functools import lru_cache

from eye_client import EyeClient
from fastapi import Depends, FastAPI

from twin.config import Settings, load_settings
from twin.eye_reads import RecordsSource, fetch_ica_records, fetch_latest_metar
from twin.forecast import air_quality_forecast
from twin.schemas import AirQualityForecast, EnvironmentState, ScenarioRequest, SimulateResponse
from twin.simulate import simulate_environment
from twin.state import environment_state


@lru_cache
def get_settings() -> Settings:
    return load_settings()


def get_eye_client(settings: Settings = Depends(get_settings)) -> RecordsSource:
    return EyeClient(
        base_url=settings.eye_base_url,
        token=settings.eye_api_token,
        timeout=settings.eye_timeout_seconds,
    )


def _parse_horizons(horizons: str) -> list[int]:
    return [int(part) for part in horizons.split(",") if part.strip()]


def create_app() -> FastAPI:
    app = FastAPI(title="twin", description="Models what Córdoba's environment may do next.")

    @app.get("/health")
    def health() -> dict[str, str]:
        return {"status": "ok"}

    @app.get("/state/environment", response_model=EnvironmentState)
    def state_environment(client: RecordsSource = Depends(get_eye_client)) -> EnvironmentState:
        metar_record = fetch_latest_metar(client)
        ica_records = fetch_ica_records(client)
        return environment_state(metar_record=metar_record, ica_records=ica_records)

    @app.get("/forecast/air-quality", response_model=AirQualityForecast)
    def forecast_air_quality(
        horizons: str = "1,3,6", client: RecordsSource = Depends(get_eye_client)
    ) -> AirQualityForecast:
        ica_records = fetch_ica_records(client)
        return air_quality_forecast(ica_records, horizons=_parse_horizons(horizons))

    @app.post("/simulate/environment", response_model=SimulateResponse)
    def simulate(
        scenario: ScenarioRequest, client: RecordsSource = Depends(get_eye_client)
    ) -> SimulateResponse:
        metar_record = fetch_latest_metar(client)
        return simulate_environment(metar_record, scenario)

    return app
