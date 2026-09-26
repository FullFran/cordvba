// Package ckan adapts CKAN open-data portals. Cordoba, the Diputacion and DGT
// all run CKAN, so the adapter is written once and pointed at a portal by the
// registry rather than by new Go code.
//
// Resources are always resolved through the API. Hardcoding a resource URL
// couples eye to a path the administration can change without telling anyone.
package ckan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// ErrCKAN is returned when the portal answers something eye cannot use.
var ErrCKAN = errors.New("ckan")

// LicenseUnspecified is what CKAN reports when a publisher declared no licence.
// eye keeps that as a fact rather than treating it as permission.
const licenseNotSpecified = "License not specified"

// apiEnvelope is the shape every CKAN action response shares.
type apiEnvelope struct {
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result"`
}

// pkg is a CKAN dataset.
type pkg struct {
	Name             string     `json:"name"`
	Title            string     `json:"title"`
	Notes            string     `json:"notes"`
	LicenseTitle     string     `json:"license_title"`
	MetadataModified string     `json:"metadata_modified"`
	MetadataCreated  string     `json:"metadata_created"`
	Resources        []resource `json:"resources"`
}

// resource is one distribution of a dataset.
type resource struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Format string `json:"format"`
	URL    string `json:"url"`
}

// searchResult is the payload of package_search.
type searchResult struct {
	Count   int   `json:"count"`
	Results []pkg `json:"results"`
}

// call performs one CKAN action and unwraps the envelope.
func call(ctx context.Context, client *httpx.Client, base, action, query string, out any) error {
	endpoint := strings.TrimSuffix(base, "/") + "/api/3/action/" + action
	if query != "" {
		endpoint += "?" + query
	}

	resp, err := client.Get(ctx, endpoint, httpx.Validators{})
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrCKAN, action, err)
	}

	var env apiEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrCKAN, action, err)
	}
	if !env.Success {
		return fmt.Errorf("%w: %s reported failure", ErrCKAN, action)
	}
	if err := json.Unmarshal(env.Result, out); err != nil {
		return fmt.Errorf("%w: %s result: %w", ErrCKAN, action, err)
	}
	return nil
}

// licenseOf normalizes CKAN's way of saying "nobody declared one".
func licenseOf(p pkg, fallback string) string {
	if p.LicenseTitle == "" || p.LicenseTitle == licenseNotSpecified {
		if fallback != "" {
			return fallback
		}
		return observation.LicenseUnspecified
	}
	return p.LicenseTitle
}

// parseCKANTime reads the timestamps CKAN emits, which carry no zone.
func parseCKANTime(s string) time.Time {
	for _, layout := range []string{"2006-01-02T15:04:05.999999", "2006-01-02T15:04:05", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// resourceByFormatAndName returns the resource matching a format and, when a
// name is given, whose name contains it.
//
// An empty name keeps the old behaviour of taking the first match, which is
// right for a dataset that publishes one layer.
func resourceByFormatAndName(p pkg, format, name string) (resource, bool) {
	needle := strings.ToLower(strings.TrimSpace(name))

	for _, r := range p.Resources {
		if !strings.EqualFold(r.Format, format) {
			continue
		}
		if needle == "" || strings.Contains(strings.ToLower(r.Name), needle) {
			return r, true
		}
	}
	return resource{}, false
}

// hashOf is the raw-payload fingerprint recorded in provenance.
func hashOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// datasetURL builds the human-facing page for a dataset.
func datasetURL(base, name string) string {
	return strings.TrimSuffix(base, "/") + "/dataset/" + url.PathEscape(name)
}

// intOption reads a numeric adapter option.
func intOption(src source.Source, key string, fallback int) int {
	if v, err := strconv.Atoi(src.Option(key, "")); err == nil && v > 0 {
		return v
	}
	return fallback
}

// firstNonEmpty returns the first value that is not blank.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// truncate shortens text on a rune boundary.
func truncate(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return strings.TrimSpace(string(runes[:max])) + "…"
}
