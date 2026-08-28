// Package config loads and validates every input the program takes from its
// environment. It is the only package in eye that calls os.Getenv, so the full
// set of external inputs is knowable from one file.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrInvalidConfig is returned when the environment cannot produce a usable
// configuration.
var ErrInvalidConfig = errors.New("config: invalid")

// Config is the resolved runtime configuration.
type Config struct {
	// DataDir holds the store and the raw cache.
	DataDir string
	// ConfigDir holds sources.yaml and rules.yaml.
	ConfigDir string
	// SourcesPath is the resolved path of the source registry.
	SourcesPath string
	// RulesPath is the resolved path of the correlation rules.
	RulesPath string

	// LogLevel is one of debug, info, warn, error.
	LogLevel string

	// AEMETAPIKey is empty when the AEMET provider is unavailable.
	AEMETAPIKey string
	// FIRMSMapKey is empty when the NASA FIRMS provider is unavailable.
	FIRMSMapKey string
}

// validLogLevels is the accepted set for EYE_LOG_LEVEL.
var validLogLevels = map[string]bool{"debug": true, "info": true, "warn": true, "error": true}

// Load resolves the configuration from the environment, falling back to the XDG
// base directories. It never creates directories; that is the caller's job at
// the point of first write.
func Load() (Config, error) {
	cfg := Config{
		DataDir:     resolveDir("EYE_DATA_DIR", "XDG_DATA_HOME", ".local/share"),
		ConfigDir:   resolveDir("EYE_CONFIG_DIR", "XDG_CONFIG_HOME", ".config"),
		LogLevel:    strings.ToLower(envOr("EYE_LOG_LEVEL", "info")),
		AEMETAPIKey: os.Getenv("AEMET_API_KEY"),
		FIRMSMapKey: os.Getenv("FIRMS_MAP_KEY"),
	}

	if !validLogLevels[cfg.LogLevel] {
		return Config{}, fmt.Errorf("%w: EYE_LOG_LEVEL %q is not one of debug, info, warn, error", ErrInvalidConfig, cfg.LogLevel)
	}

	cfg.SourcesPath = filepath.Join(cfg.ConfigDir, "sources.yaml")
	cfg.RulesPath = filepath.Join(cfg.ConfigDir, "rules.yaml")

	return cfg, nil
}

// StorePath is the location of the SQLite database.
func (c Config) StorePath() string { return filepath.Join(c.DataDir, "eye.db") }

// RawCachePath is the root of the content-addressed raw response cache.
func (c Config) RawCachePath() string { return filepath.Join(c.DataDir, "raw") }

// resolveDir applies the precedence: explicit override, then the XDG base
// directory, then a path relative to the home directory.
func resolveDir(override, xdgVar, homeRelative string) string {
	if v := os.Getenv(override); v != "" {
		return v
	}
	if v := os.Getenv(xdgVar); v != "" {
		return filepath.Join(v, "eye")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// A machine with no resolvable home still gets a working relative
		// path rather than an empty one.
		return filepath.Join(homeRelative, "eye")
	}
	return filepath.Join(home, homeRelative, "eye")
}

// envOr returns the environment value or a fallback when it is unset or empty.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
