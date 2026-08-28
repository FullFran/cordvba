// Package infrastructure wires registry entries to the adapter that can read
// them. The registry, not the code, decides which parser a source gets: adding
// a feed of a format eye already speaks is a configuration change.
package infrastructure

import (
	"errors"
	"fmt"
	"sort"

	"github.com/FullFran/eye/internal/httpx"
	provider "github.com/FullFran/eye/internal/provider/domain"
	"github.com/FullFran/eye/internal/provider/infrastructure/adsblol"
	"github.com/FullFran/eye/internal/provider/infrastructure/ckan"
	"github.com/FullFran/eye/internal/provider/infrastructure/datex"
	"github.com/FullFran/eye/internal/provider/infrastructure/gtfsrt"
	"github.com/FullFran/eye/internal/provider/infrastructure/rss"
	"github.com/FullFran/eye/internal/provider/infrastructure/wfs"
	source "github.com/FullFran/eye/internal/source/domain"
)

// ErrNoAdapter is returned for a source whose format eye cannot yet read. It is
// a normal, reportable state: the registry describes more of Cordoba than the
// code has caught up with, and saying so is better than pretending the source
// does not exist.
var ErrNoAdapter = errors.New("no adapter for format")

// builders maps a registry format to its adapter constructor.
var builders = map[string]func(source.Source, *httpx.Client) provider.Provider{
	"rss": func(s source.Source, c *httpx.Client) provider.Provider { return rss.New(s, c) },
	"ckan-catalog": func(s source.Source, c *httpx.Client) provider.Provider {
		return ckan.NewCatalog(s, c)
	},
	"ckan-geojson": func(s source.Source, c *httpx.Client) provider.Provider {
		return ckan.NewGeoJSON(s, c)
	},
	"adsb-json": func(s source.Source, c *httpx.Client) provider.Provider {
		return adsblol.New(s, c)
	},
	"datex2-devices": func(s source.Source, c *httpx.Client) provider.Provider {
		return datex.NewDevices(s, c)
	},
	"wfs-geojson": func(s source.Source, c *httpx.Client) provider.Provider {
		return wfs.New(s, c)
	},
	"gtfs-rt-json": func(s source.Source, c *httpx.Client) provider.Provider {
		return gtfsrt.New(s, c)
	},
}

// SupportedFormats lists the formats this build can read.
func SupportedFormats() []string {
	out := make([]string, 0, len(builders))
	for format := range builders {
		out = append(out, format)
	}
	sort.Strings(out)
	return out
}

// Supported reports whether a format has an adapter.
func Supported(format string) bool {
	_, ok := builders[format]
	return ok
}

// Build returns the adapter for one source.
func Build(src source.Source, client *httpx.Client) (provider.Provider, error) {
	build, ok := builders[src.Format]
	if !ok {
		return nil, fmt.Errorf("%w: %q (source %s)", ErrNoAdapter, src.Format, src.ID)
	}
	return build(src, client), nil
}

// BuildPollable returns adapters for every source the registry permits polling
// and this build can read.
//
// Both filters matter and they are different. A source can be permitted and
// unreadable (no adapter yet), or readable and not permitted (licence
// unresolved). Only the intersection is fetched.
func BuildPollable(sources []source.Source, client *httpx.Client) ([]provider.Provider, []source.Source) {
	var (
		providers []provider.Provider
		skipped   []source.Source
	)

	for _, src := range sources {
		if !src.Automation.Pollable() {
			continue
		}
		p, err := Build(src, client)
		if err != nil {
			skipped = append(skipped, src)
			continue
		}
		providers = append(providers, p)
	}
	return providers, skipped
}
