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
	"time"
)

// defaultRawCacheGrace is long enough that a payload fetched but not yet
// normalized into a record survives an ordinary prune cycle.
const defaultRawCacheGrace = 24 * time.Hour

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

	// APIToken guards every /v1 endpoint of `eye serve` when it is set.
	//
	// An empty token leaves the API open, which is the right default for a
	// loopback bind on somebody's own machine. It is not the right default
	// for a public one, and `eye serve --public` refuses without it.
	APIToken string
	// CORSOrigin is what `eye serve` answers in
	// Access-Control-Allow-Origin. It defaults to "*" because the API is
	// read-only, and is narrowed by operators who front it with a browser
	// application of their own.
	CORSOrigin string

	// ExtraCAFile is a PEM file of certificate authorities to trust in
	// addition to the system pool.
	//
	// It exists because several Spanish public-sector servers send an
	// incomplete certificate chain — MITECO's air-quality host omits the
	// FNMT-RCM intermediate — and Go does not fetch the missing certificate
	// the way a browser does. Pointing this at the published intermediate
	// completes the chain. It is not a way to skip verification, and eye
	// has no such option.
	ExtraCAFile string

	// AllowPersonalSources enables sources marked undocumented_personal.
	//
	// The registry saying "enabled" is not enough for these on purpose. They
	// are the operator's own decision about their own machine, so the machine
	// has to say so too: EYE_ALLOW_PERSONAL_SOURCES=1.
	AllowPersonalSources bool

	// RawCacheGrace is how long a cached payload survives after the records
	// that would reference it become eligible for removal, before the raw
	// cache prune considers it unreferenced.
	//
	// It exists so a payload fetched but not yet normalized into a record —
	// the daemon is mid-poll, or a run crashed between the two steps — is
	// never removed: EYE_RAW_CACHE_GRACE, default 24h.
	RawCacheGrace time.Duration
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

		ExtraCAFile: strings.TrimSpace(os.Getenv("EYE_EXTRA_CA_FILE")),

		APIToken:   strings.TrimSpace(os.Getenv("EYE_API_TOKEN")),
		CORSOrigin: strings.TrimSpace(os.Getenv("EYE_CORS_ORIGIN")),

		AllowPersonalSources: truthy(os.Getenv("EYE_ALLOW_PERSONAL_SOURCES")),
	}

	if cfg.CORSOrigin == "" {
		cfg.CORSOrigin = "*"
	}

	if !validLogLevels[cfg.LogLevel] {
		return Config{}, fmt.Errorf("%w: EYE_LOG_LEVEL %q is not one of debug, info, warn, error", ErrInvalidConfig, cfg.LogLevel)
	}

	grace, err := parseRawCacheGrace(os.Getenv("EYE_RAW_CACHE_GRACE"))
	if err != nil {
		return Config{}, err
	}
	cfg.RawCacheGrace = grace

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

// truthy reads the forms people actually type for a boolean environment flag.
func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

// envOr returns the environment value or a fallback when it is unset or empty.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseRawCacheGrace resolves EYE_RAW_CACHE_GRACE. An unset value falls back
// to the default; a set one must parse and must be strictly positive; a grace
// of zero or less would mean a payload could be removed before the record
// depending on it was ever written.
func parseRawCacheGrace(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultRawCacheGrace, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: EYE_RAW_CACHE_GRACE %q: %w", ErrInvalidConfig, raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%w: EYE_RAW_CACHE_GRACE %q must be positive", ErrInvalidConfig, raw)
	}
	return d, nil
}
