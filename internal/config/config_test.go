package config_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FullFran/eye/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("EYE_DATA_DIR", "")
	t.Setenv("EYE_CONFIG_DIR", "")
	t.Setenv("XDG_DATA_HOME", "/xdg/data")
	t.Setenv("XDG_CONFIG_HOME", "/xdg/config")
	t.Setenv("EYE_LOG_LEVEL", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}

	if cfg.DataDir != filepath.Join("/xdg/data", "eye") {
		t.Errorf("DataDir = %q", cfg.DataDir)
	}
	if cfg.SourcesPath != filepath.Join("/xdg/config", "eye", "sources.yaml") {
		t.Errorf("SourcesPath = %q", cfg.SourcesPath)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want the info default", cfg.LogLevel)
	}
}

func TestLoadExplicitOverridesWin(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/xdg/data")
	t.Setenv("EYE_DATA_DIR", "/custom/data")
	t.Setenv("EYE_CONFIG_DIR", "/custom/config")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}

	if cfg.DataDir != "/custom/data" {
		t.Errorf("DataDir = %q, want the explicit override", cfg.DataDir)
	}
	if cfg.StorePath() != "/custom/data/eye.db" {
		t.Errorf("StorePath() = %q", cfg.StorePath())
	}
	if cfg.RawCachePath() != "/custom/data/raw" {
		t.Errorf("RawCachePath() = %q", cfg.RawCachePath())
	}
	if cfg.RulesPath != "/custom/config/rules.yaml" {
		t.Errorf("RulesPath = %q", cfg.RulesPath)
	}
}

func TestLoadRejectsUnknownLogLevel(t *testing.T) {
	t.Setenv("EYE_LOG_LEVEL", "shout")

	_, err := config.Load()
	if !errors.Is(err, config.ErrInvalidConfig) {
		t.Fatalf("Load() = %v, want ErrInvalidConfig", err)
	}
	if !strings.Contains(err.Error(), "shout") {
		t.Errorf("error = %q, want it to name the bad value", err)
	}
}

func TestLoadAcceptsEveryLogLevel(t *testing.T) {
	for _, level := range []string{"debug", "INFO", "Warn", "error"} {
		t.Run(level, func(t *testing.T) {
			t.Setenv("EYE_LOG_LEVEL", level)
			if _, err := config.Load(); err != nil {
				t.Errorf("Load() with %q = %v", level, err)
			}
		})
	}
}

func TestLoadReadsAPIKeys(t *testing.T) {
	t.Setenv("AEMET_API_KEY", "aemet-key")
	t.Setenv("FIRMS_MAP_KEY", "firms-key")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if cfg.AEMETAPIKey != "aemet-key" || cfg.FIRMSMapKey != "firms-key" {
		t.Errorf("keys not loaded: %+v", cfg)
	}
}
