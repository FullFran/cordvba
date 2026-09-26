// Package wfs adapts OGC Web Feature Service endpoints.
//
// WFS is a published, self-describing standard: a server says what it holds
// through GetCapabilities and hands it over through GetFeature. That makes it
// exactly the kind of interface eye is allowed to read, and one adapter serves
// every municipal and regional spatial data infrastructure rather than one.
package wfs

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

	"github.com/FullFran/eye/internal/httpx"
	observation "github.com/FullFran/eye/internal/observation/domain"
	provider "github.com/FullFran/eye/internal/provider/domain"
	"github.com/FullFran/eye/internal/provider/infrastructure/geojson"
	source "github.com/FullFran/eye/internal/source/domain"
)

// ErrWFS is returned when a service answers something eye cannot use.
var ErrWFS = errors.New("wfs")

// defaultCount caps how many features are requested. A layer with hundreds of
// thousands of parcels is not something to pull in one go by accident.
const defaultCount = 5000

// Provider reads one WFS layer as inventory.
type Provider struct {
	src    source.Source
	client *httpx.Client
}

// New builds a provider for a registry entry. The layer name comes from the
// `layer` option, so a new layer on a known server is configuration.
func New(src source.Source, client *httpx.Client) *Provider {
	return &Provider{src: src, client: client}
}

// Info implements provider.Provider.
func (p *Provider) Info() source.Source { return p.src }

// Poll reports the layer refresh as a single record; the layer itself is the
// entity set.
func (p *Provider) Poll(ctx context.Context) ([]observation.Record, error) {
	entities, err := p.Entities(ctx)
	if err != nil {
		return nil, err
	}
	if len(entities) == 0 {
		return nil, nil
	}

	now := time.Now().UTC()
	payload, _ := json.Marshal(map[string]any{
		"layer": p.layer(),
		"kind":  p.kind(),
		"count": len(entities),
	})

	rec := observation.Record{
		ID:         p.src.ID + ":inventory",
		Source:     p.src.ID,
		Kind:       "inventory_refresh",
		Topic:      p.src.Topic,
		ObservedAt: now,
		FetchedAt:  now,
		Title:      fmt.Sprintf("%d %s mapped in %s", len(entities), p.kind(), p.layer()),
		Severity:   observation.SeverityNone,
		Confidence: 1,
		Quality:    observation.QualityOfficial,
		LocalKey:   "inventory",
		DedupeKey:  fmt.Sprintf("%s:inventory:%d", p.src.ID, len(entities)),
		Payload:    payload,
		Provenance: entities[0].Provenance,
	}
	if err := rec.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrWFS, err)
	}
	return []observation.Record{rec}, nil
}

// Entities fetches the layer as GeoJSON and maps it to inventory.
func (p *Provider) Entities(ctx context.Context) ([]observation.Entity, error) {
	layer := p.layer()
	if layer == "" {
		return nil, fmt.Errorf("%w: source %s has no layer option", ErrWFS, p.src.ID)
	}

	resp, err := p.client.Get(ctx, p.featureURL(layer), httpx.Validators{})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrWFS, err)
	}

	// A WFS reports failure as an XML ExceptionReport with a 200 status, so
	// the status code alone does not tell us whether this worked.
	if bytes := strings.TrimLeft(string(resp.Body), " \t\r\n"); strings.HasPrefix(bytes, "<") {
		return nil, fmt.Errorf("%w: %s returned an exception report, not features", ErrWFS, layer)
	}

	fc, err := geojson.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrWFS, layer, err)
	}

	sum := sha256.Sum256(resp.Body)
	entities := fc.Entities(geojson.Options{
		SourceID: p.src.ID,
		Kind:     p.kind(),
		Topic:    p.src.Topic,
		SeenAt:   resp.FetchedAt,
		Provenance: observation.Provenance{
			Publisher: p.src.Authority,
			SourceURL: p.src.URL,
			License:   p.src.License,
			FetchedAt: resp.FetchedAt,
			RawHash:   hex.EncodeToString(sum[:]),
		},
	})

	if len(entities) == 0 && len(fc.Features) > 0 {
		return nil, fmt.Errorf("%w: %s returned %d features but none had a usable point",
			ErrWFS, layer, len(fc.Features))
	}
	return entities, nil
}

// featureURL builds a GetFeature request for the layer.
func (p *Provider) featureURL(layer string) string {
	q := url.Values{}
	q.Set("service", "WFS")
	q.Set("version", "2.0.0")
	q.Set("request", "GetFeature")
	q.Set("typeNames", layer)
	q.Set("outputFormat", "application/json")
	q.Set("count", strconv.Itoa(p.count()))
	// WFS 2.0 servers vary on whether they reproject by default. Asking
	// explicitly for CRS84 is what guarantees lon/lat rather than the
	// layer's native projection, which for Cordoba is usually UTM 30N.
	q.Set("srsName", "urn:ogc:def:crs:OGC:1.3:CRS84")

	return strings.TrimSuffix(p.src.URL, "/") + "?" + q.Encode()
}

// layer is the WFS type name to fetch.
func (p *Provider) layer() string { return p.src.Option("layer", "") }

// kind is the entity kind this layer produces.
func (p *Provider) kind() string { return p.src.Option("kind", "feature") }

// count is the feature cap for one request.
func (p *Provider) count() int {
	if n, err := strconv.Atoi(p.src.Option("count", "")); err == nil && n > 0 {
		return n
	}
	return defaultCount
}

var (
	_ provider.Provider       = (*Provider)(nil)
	_ provider.EntityProvider = (*Provider)(nil)
)
