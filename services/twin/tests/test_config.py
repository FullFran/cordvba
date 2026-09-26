"""Config is read from the environment in exactly one place."""

import pytest

from twin.config import Settings, load_settings


def test_load_settings_reads_eye_base_url_and_token(monkeypatch):
    monkeypatch.setenv("EYE_BASE_URL", "http://127.0.0.1:8080")
    monkeypatch.setenv("EYE_API_TOKEN", "s3cr3t")

    settings = load_settings()

    assert isinstance(settings, Settings)
    assert settings.eye_base_url == "http://127.0.0.1:8080"
    assert settings.eye_api_token == "s3cr3t"


def test_load_settings_raises_a_clear_error_when_eye_base_url_is_missing(monkeypatch):
    monkeypatch.delenv("EYE_BASE_URL", raising=False)
    monkeypatch.setenv("EYE_API_TOKEN", "s3cr3t")

    with pytest.raises(RuntimeError, match="EYE_BASE_URL"):
        load_settings()


def test_load_settings_raises_a_clear_error_when_eye_api_token_is_missing(monkeypatch):
    monkeypatch.setenv("EYE_BASE_URL", "http://127.0.0.1:8080")
    monkeypatch.delenv("EYE_API_TOKEN", raising=False)

    with pytest.raises(RuntimeError, match="EYE_API_TOKEN"):
        load_settings()
