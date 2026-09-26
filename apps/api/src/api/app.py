"""cordvba's public backend: the only backend apps/web talks to. Composes
eye and twin into one representation per screen, and keeps eye's token and
both internal base URLs (eye, twin) away from the browser. See
apps/api/README.md and docs/architecture/system-overview.md.
"""

from __future__ import annotations

from functools import lru_cache

from eye_client import EyeClient
from fastapi import Depends, FastAPI
from fastapi.middleware.cors import CORSMiddleware

from api.compose import environment_from_twin, sources_from_eye, timeline_from_eye_and_twin
from api.config import Settings, load_settings
from api.schemas import (
    EnvironmentResponse,
    SimulateRequest,
    SimulateResponse,
    SourcesResponse,
    TimelineResponse,
)
from api.twin_client import TwinClient

#: The Córdoba-city ICA station the timeline's air_quality_index series
#: follows. v0.1 shows one representative station; see
#: packages/contracts/environment/v1/timeline.example.json.
TIMELINE_STATION_ID = "14021009"


@lru_cache
def get_settings() -> Settings:
    return load_settings()


def get_eye_client(settings: Settings = Depends(get_settings)) -> EyeClient:
    return EyeClient(
        base_url=settings.eye_base_url,
        token=settings.eye_api_token,
        timeout=settings.eye_timeout_seconds,
    )


def get_twin_client(settings: Settings = Depends(get_settings)) -> TwinClient:
    return TwinClient(base_url=settings.twin_base_url, timeout=settings.twin_timeout_seconds)


def _parse_horizons(horizons: str) -> list[int]:
    return [int(part) for part in horizons.split(",") if part.strip()]


def create_app() -> FastAPI:
    app = FastAPI(
        title="cordvba api",
        description="Composes eye (observed history) and twin (state, forecast, scenarios).",
    )

    # CORS is wired eagerly against the configured origin only when settings
    # are available (they may not be at import time in a test session that
    # overrides get_settings itself); create_app() is called after uv sync
    # in production, where EYE_*/TWIN_*/WEB_ORIGIN are always set.
    try:
        settings = load_settings()
        app.add_middleware(
            CORSMiddleware,
            allow_origins=settings.web_origins,
            allow_methods=["GET", "POST"],
            allow_headers=["*"],
        )
    except RuntimeError:
        pass

    @app.get("/health")
    def health() -> dict[str, str]:
        return {"status": "ok"}

    @app.get("/v1/environment", response_model=EnvironmentResponse)
    def get_environment(twin: TwinClient = Depends(get_twin_client)) -> EnvironmentResponse:
        state = twin.state_environment()
        forecast = twin.forecast_air_quality(horizons=[1, 3, 6])
        return environment_from_twin(state, forecast)

    @app.get("/v1/environment/timeline", response_model=TimelineResponse)
    def get_timeline(
        hours_back: int = 12,
        horizons: str = "1,3,6",
        eye: EyeClient = Depends(get_eye_client),
        twin: TwinClient = Depends(get_twin_client),
    ) -> TimelineResponse:
        since = f"{hours_back}h"
        metar_records = eye.records(source="metar-cordoba", topic="weather", since=since)
        ica_records = eye.records(source="miteco-ica", topic="air_quality", since=since)
        ica_records = [r for r in ica_records if r.local_key == TIMELINE_STATION_ID]
        forecast = twin.forecast_air_quality(horizons=_parse_horizons(horizons))
        return timeline_from_eye_and_twin(
            metar_records=metar_records,
            ica_records=ica_records,
            twin_forecast=forecast,
            station_id=TIMELINE_STATION_ID,
        )

    @app.post("/v1/environment/simulate", response_model=SimulateResponse)
    def post_simulate(
        scenario: SimulateRequest, twin: TwinClient = Depends(get_twin_client)
    ) -> SimulateResponse:
        raw = twin.simulate_environment(scenario.model_dump(mode="json"))
        return SimulateResponse.model_validate(raw)

    @app.get("/v1/sources", response_model=SourcesResponse)
    def get_sources(eye: EyeClient = Depends(get_eye_client)) -> SourcesResponse:
        return sources_from_eye(eye.sources())

    return app
