// Package infrastructure wires registry entries to the adapter that can read
// them. The registry, not the code, decides which parser a source gets: adding
// a feed of a format eye already speaks is a configuration change.
package infrastructure

import (
	"errors"
	"fmt"
	"sort"

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	provider "github.com/FullFran/cordvba/apps/eye/internal/provider/domain"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/adsblol"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/aemet"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/aucorsa"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/ckan"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/ctan"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/datex"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/dcat"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/firms"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/gtfs"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/gtfsrt"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/metar"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/overpass"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/rss"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/wfs"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// ErrNoAdapter is returned for a source whose format eye cannot yet read. It is
// a normal, reportable state: the registry describes more of Cordoba than the
// code has caught up with, and saying so is better than pretending the source
// does not exist.
var ErrNoAdapter = errors.New("no adapter for format")

// builders maps a registry format to its adapter constructor.
//
// Credentials arrive alongside the source rather than inside it: a key belongs
// to the machine, and the registry is a file in a repository.
var builders = map[string]func(source.Source, *httpx.Client, Options) provider.Provider{
	"rss": func(s source.Source, c *httpx.Client, _ Options) provider.Provider { return rss.New(s, c) },
	"ckan-catalog": func(s source.Source, c *httpx.Client, _ Options) provider.Provider {
		return ckan.NewCatalog(s, c)
	},
	"ckan-geojson": func(s source.Source, c *httpx.Client, _ Options) provider.Provider {
		return ckan.NewGeoJSON(s, c)
	},
	"adsb-json": func(s source.Source, c *httpx.Client, _ Options) provider.Provider {
		return adsblol.New(s, c)
	},
	"datex2-situations": func(s source.Source, c *httpx.Client, _ Options) provider.Provider {
		return datex.NewSituations(s, c)
	},
	"datex2-vms": func(s source.Source, c *httpx.Client, _ Options) provider.Provider {
		return datex.NewVms(s, c)
	},
	"datex2-devices": func(s source.Source, c *httpx.Client, _ Options) provider.Provider {
		return datex.NewDevices(s, c)
	},
	"wfs-geojson": func(s source.Source, c *httpx.Client, _ Options) provider.Provider {
		return wfs.New(s, c)
	},
	"gtfs-rt-json": func(s source.Source, c *httpx.Client, _ Options) provider.Provider {
		return gtfsrt.New(s, c)
	},
	"gtfs": func(s source.Source, c *httpx.Client, _ Options) provider.Provider {
		return gtfs.New(s, c)
	},
	"ctan-notices": func(s source.Source, c *httpx.Client, _ Options) provider.Provider {
		return ctan.NewNotices(s, c)
	},
	"ctan-stops": func(s source.Source, c *httpx.Client, _ Options) provider.Provider {
		return ctan.NewStops(s, c)
	},
	"overpass-json": func(s source.Source, c *httpx.Client, _ Options) provider.Provider {
		return overpass.New(s, c)
	},
	"aucorsa-lines": func(s source.Source, c *httpx.Client, _ Options) provider.Provider {
		return aucorsa.New(s, c)
	},
	"aucorsa-arrivals": func(s source.Source, c *httpx.Client, _ Options) provider.Provider {
		return aucorsa.NewRealtime(s, c)
	},
	// AEMET publishes several products behind one two-step contract, so the
	// registry names the product rather than the protocol.
	"metar-json": func(s source.Source, c *httpx.Client, _ Options) provider.Provider {
		return metar.New(s, c)
	},
	"meteoalarm-atom": func(s source.Source, c *httpx.Client, _ Options) provider.Provider {
		return aemet.NewMeteoAlarm(s, c)
	},
	"aemet-warnings": func(s source.Source, c *httpx.Client, o Options) provider.Provider {
		return aemet.NewWarnings(s, c, o.AEMETAPIKey)
	},
	"aemet-observation": func(s source.Source, c *httpx.Client, o Options) provider.Provider {
		return aemet.NewObservation(s, c, o.AEMETAPIKey)
	},
	// "rest-json-two-step" names AEMET's transport, not a product. The
	// source's `product` option says which one; without it the adapter
	// refuses to poll rather than filing one product under another's id.
	"rest-json-two-step": func(s source.Source, c *httpx.Client, o Options) provider.Provider {
		return aemet.NewProduct(s, c, o.AEMETAPIKey)
	},
	"rest-csv": func(s source.Source, c *httpx.Client, o Options) provider.Provider {
		return firms.New(s, c, o.FIRMSMapKey)
	},
	"csv-dcat": func(s source.Source, c *httpx.Client, _ Options) provider.Provider {
		return dcat.NewAirQuality(s, c)
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
func Build(src source.Source, client *httpx.Client, opts Options) (provider.Provider, error) {
	build, ok := builders[src.Format]
	if !ok {
		return nil, fmt.Errorf("%w: %q (source %s)", ErrNoAdapter, src.Format, src.ID)
	}
	return build(src, client, opts), nil
}

// Options control which sources may be built and with what credentials.
type Options struct {
	// AllowPersonal enables sources marked undocumented_personal. The
	// registry alone cannot turn these on; the operator's machine must say
	// so too.
	AllowPersonal bool

	// AEMETAPIKey and FIRMSMapKey are the credentials for the two sources
	// that need one. An empty value is not a failure to build: the adapter
	// is constructed and reports a typed error naming the environment
	// variable on its first poll, which is the difference between a source
	// eye cannot read and one it silently pretends is quiet.
	AEMETAPIKey string
	FIRMSMapKey string
}

// BuildPollable returns adapters for every source the registry permits polling
// and this build can read.
//
// Three filters, and they are different questions. A source can be permitted
// and unreadable (no adapter yet), readable and not permitted (licence
// unresolved), or both and still held back because it is an undocumented
// personal source and this machine has not opted in.
func BuildPollable(sources []source.Source, client *httpx.Client, opts Options) ([]provider.Provider, []source.Source) {
	var (
		providers []provider.Provider
		skipped   []source.Source
	)

	for _, src := range sources {
		if !src.Automation.Pollable() {
			continue
		}
		if src.Access.Personal() && !opts.AllowPersonal {
			skipped = append(skipped, src)
			continue
		}
		p, err := Build(src, client, opts)
		if err != nil {
			skipped = append(skipped, src)
			continue
		}
		providers = append(providers, p)
	}
	return providers, skipped
}
