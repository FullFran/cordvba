package config_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/config"
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

// A token is what turns "listening on a port" into "a private deployment", so
// it is read in the one place every other external input is read.
func TestLoadReadsTheAPIToken(t *testing.T) {
	t.Setenv("EYE_API_TOKEN", "  s3cret  ")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	// Surrounding whitespace is a copy-and-paste artefact, never part of a
	// token, and a token with a stray newline fails in a way nobody can see.
	if cfg.APIToken != "s3cret" {
		t.Errorf("APIToken = %q, want it trimmed", cfg.APIToken)
	}
}

func TestLoadCORSOriginDefaultsToAnyOrigin(t *testing.T) {
	t.Setenv("EYE_CORS_ORIGIN", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if cfg.CORSOrigin != "*" {
		t.Errorf("CORSOrigin = %q, want the permissive default", cfg.CORSOrigin)
	}
}

func TestLoadCORSOriginIsConfigurable(t *testing.T) {
	t.Setenv("EYE_CORS_ORIGIN", "https://map.example.org")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if cfg.CORSOrigin != "https://map.example.org" {
		t.Errorf("CORSOrigin = %q", cfg.CORSOrigin)
	}
}

// TestLoadReadsTheExtraCAFile checks the operator can widen the trust pool.
//
// It is one path, and it exists for the servers that send an incomplete
// certificate chain rather than for anything general. Nothing about it disables
// verification.
func TestLoadReadsTheExtraCAFile(t *testing.T) {
	t.Setenv("EYE_EXTRA_CA_FILE", "  /etc/ssl/extra/fnmt-intermediate.pem  ")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if want := "/etc/ssl/extra/fnmt-intermediate.pem"; cfg.ExtraCAFile != want {
		t.Errorf("ExtraCAFile = %q, want %q", cfg.ExtraCAFile, want)
	}
}

// An unset variable must leave the client on the system pool alone.
func TestLoadLeavesTheExtraCAFileEmptyByDefault(t *testing.T) {
	t.Setenv("EYE_EXTRA_CA_FILE", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if cfg.ExtraCAFile != "" {
		t.Errorf("ExtraCAFile = %q, want empty", cfg.ExtraCAFile)
	}
}

// The grace period defaults to 24h, long enough that a payload fetched but
// not yet normalized into a record is never removed by an ordinary prune
// cycle.
func TestLoadRawCacheGraceDefaultsTo24h(t *testing.T) {
	t.Setenv("EYE_RAW_CACHE_GRACE", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if cfg.RawCacheGrace != 24*time.Hour {
		t.Errorf("RawCacheGrace = %v, want 24h", cfg.RawCacheGrace)
	}
}

func TestLoadRawCacheGraceIsConfigurable(t *testing.T) {
	t.Setenv("EYE_RAW_CACHE_GRACE", "6h")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if cfg.RawCacheGrace != 6*time.Hour {
		t.Errorf("RawCacheGrace = %v, want 6h", cfg.RawCacheGrace)
	}
}

func TestLoadRejectsAnUnparsableRawCacheGrace(t *testing.T) {
	t.Setenv("EYE_RAW_CACHE_GRACE", "soon")

	if _, err := config.Load(); !errors.Is(err, config.ErrInvalidConfig) {
		t.Fatalf("Load() = %v, want ErrInvalidConfig", err)
	}
}

func TestLoadRejectsANonPositiveRawCacheGrace(t *testing.T) {
	t.Setenv("EYE_RAW_CACHE_GRACE", "0h")

	if _, err := config.Load(); !errors.Is(err, config.ErrInvalidConfig) {
		t.Fatalf("Load() = %v, want ErrInvalidConfig", err)
	}
}

// An unset EYE_OVERRIDES_FILE resolves under the config directory, the same
// way sources.yaml and rules.yaml already do: a deployment that only sets
// EYE_CONFIG_DIR gets a predictable place to put the optional file, without
// having to name it too.
func TestLoadOverridesPathDefaultsUnderConfigDir(t *testing.T) {
	t.Setenv("EYE_CONFIG_DIR", "/custom/config")
	t.Setenv("EYE_OVERRIDES_FILE", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if want := filepath.Join("/custom/config", "overrides.yaml"); cfg.OverridesPath != want {
		t.Errorf("OverridesPath = %q, want %q", cfg.OverridesPath, want)
	}
}

// An explicit EYE_OVERRIDES_FILE wins outright, rather than composing under
// ConfigDir the way sources.yaml does: a deployment that mounts the file
// somewhere else must not also have to relocate its whole config directory.
func TestLoadOverridesPathExplicitOverrideWins(t *testing.T) {
	t.Setenv("EYE_CONFIG_DIR", "/custom/config")
	t.Setenv("EYE_OVERRIDES_FILE", "/etc/eye/overrides.yaml")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if cfg.OverridesPath != "/etc/eye/overrides.yaml" {
		t.Errorf("OverridesPath = %q, want the explicit override", cfg.OverridesPath)
	}
}
