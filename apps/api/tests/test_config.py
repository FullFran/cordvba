"""Config is read from the environment in exactly one place."""

import pytest

from api.config import Settings, load_settings


def test_load_settings_reads_every_required_variable(monkeypatch):
    monkeypatch.setenv("EYE_BASE_URL", "http://127.0.0.1:8080")
    monkeypatch.setenv("EYE_API_TOKEN", "s3cr3t")
    monkeypatch.setenv("TWIN_BASE_URL", "http://127.0.0.1:8100")
    monkeypatch.setenv("WEB_ORIGIN", "https://cordvba.example")

    settings = load_settings()

    assert isinstance(settings, Settings)
    assert settings.eye_base_url == "http://127.0.0.1:8080"
    assert settings.eye_api_token == "s3cr3t"
    assert settings.twin_base_url == "http://127.0.0.1:8100"
    assert settings.web_origin == "https://cordvba.example"


@pytest.mark.parametrize(
    "missing_var", ["EYE_BASE_URL", "EYE_API_TOKEN", "TWIN_BASE_URL", "WEB_ORIGIN"]
)
def test_load_settings_raises_a_clear_error_when_a_variable_is_missing(monkeypatch, missing_var):
    monkeypatch.setenv("EYE_BASE_URL", "http://127.0.0.1:8080")
    monkeypatch.setenv("EYE_API_TOKEN", "s3cr3t")
    monkeypatch.setenv("TWIN_BASE_URL", "http://127.0.0.1:8100")
    monkeypatch.setenv("WEB_ORIGIN", "https://cordvba.example")
    monkeypatch.delenv(missing_var, raising=False)

    with pytest.raises(RuntimeError, match=missing_var):
        load_settings()
