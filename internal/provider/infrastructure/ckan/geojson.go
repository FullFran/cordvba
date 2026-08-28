package ckan

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/FullFran/eye/internal/httpx"
	observation "github.com/FullFran/eye/internal/observation/domain"
	provider "github.com/FullFran/eye/internal/provider/domain"
	"github.com/FullFran/eye/internal/provider/infrastructure/geojson"
	source "github.com/FullFran/eye/internal/source/domain"
)

// GeoJSONProvider turns a CKAN dataset's GeoJSON distribution into entities:
// cameras, junctions, taxi ranks, anything the municipality maps as points.
//
// These are inventory, not observations. A camera is a persistent thing; where
// it is does not change every poll.
type GeoJSONProvider struct {
	src    source.Source
	client *httpx.Client
}

// NewGeoJSON builds an inventory provider for a registry entry. The dataset id
// and the entity kind come from the registry options.
func NewGeoJSON(src source.Source, client *httpx.Client) *GeoJSONProvider {
	return &GeoJSONProvider{src: src, client: client}
}

// Info implements provider.Provider.
func (p *GeoJSONProvider) Info() source.Source { return p.src }

// Poll reports the inventory refresh as a single record, so the source shows up
// in the timeline and in health without duplicating the inventory itself.
func (p *GeoJSONProvider) Poll(ctx context.Context) ([]observation.Record, error) {
	entities, err := p.Entities(ctx)
	if err != nil {
		return nil, err
	}
	if len(entities) == 0 {
		return nil, nil
	}

	fetchedAt := nowUTC()
	payload, _ := json.Marshal(map[string]any{
		"kind":  p.kind(),
		"count": len(entities),
	})

	rec := observation.Record{
		ID:         p.src.ID + ":inventory",
		Source:     p.src.ID,
		Kind:       "inventory_refresh",
		Topic:      p.src.Topic,
		ObservedAt: fetchedAt,
		FetchedAt:  fetchedAt,
		Title:      fmt.Sprintf("%d %s assets in the municipal inventory", len(entities), p.kind()),
		Severity:   observation.SeverityNone,
		Confidence: 1,
		Quality:    observation.QualityOfficial,
		// The title carries the count, which is exactly what changes.
		LocalKey:   "inventory",
		DedupeKey:  p.src.ID + ":inventory:" + strconv.Itoa(len(entities)),
		Payload:    payload,
		Provenance: entities[0].Provenance,
	}
	if err := rec.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCKAN, err)
	}
	return []observation.Record{rec}, nil
}

// Entities fetches the dataset's GeoJSON distribution and maps each feature.
//
// The mapping is the shared one, so CKAN and WFS cannot drift on which fields
// are refused — and in particular so the personal-data filter applies here too.
func (p *GeoJSONProvider) Entities(ctx context.Context) ([]observation.Entity, error) {
	dataset := p.src.Option("dataset", "")
	if dataset == "" {
		return nil, fmt.Errorf("%w: source %s has no dataset option", ErrCKAN, p.src.ID)
	}

	var ds pkg
	if err := call(ctx, p.client, p.src.URL, "package_show", "id="+dataset, &ds); err != nil {
		return nil, err
	}

	// A dataset can publish thirty layers as separate GeoJSON resources —
	// Cordoba's cartography dataset does — so the registry names which one.
	res, ok := resourceByFormatAndName(ds, "GeoJSON", p.src.Option("resource", ""))
	if !ok {
		return nil, fmt.Errorf("%w: dataset %s has no GeoJSON resource matching %q",
			ErrCKAN, dataset, p.src.Option("resource", "(any)"))
	}

	resp, err := p.client.Get(ctx, res.URL, httpx.Validators{})
	if err != nil {
		return nil, fmt.Errorf("%w: fetch %s: %w", ErrCKAN, res.Name, err)
	}

	fc, err := geojson.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: parse %s: %w", ErrCKAN, res.Name, err)
	}

	entities := fc.Entities(geojson.Options{
		SourceID: p.src.ID,
		Kind:     p.kind(),
		Topic:    p.src.Topic,
		SeenAt:   resp.FetchedAt,
		Provenance: observation.Provenance{
			Publisher: p.src.Authority,
			SourceURL: datasetURL(p.src.URL, ds.Name),
			License:   licenseOf(ds, p.src.License),
			FetchedAt: resp.FetchedAt,
			RawHash:   hashOf(resp.Body),
		},
	})

	if len(entities) == 0 && len(fc.Features) > 0 {
		return nil, fmt.Errorf("%w: %s returned %d features but none were usable",
			ErrCKAN, res.Name, len(fc.Features))
	}
	return entities, nil
}

// kind is the entity kind this dataset produces.
func (p *GeoJSONProvider) kind() string { return p.src.Option("kind", "asset") }

// nowUTC exists so tests can reason about a single clock source.
func nowUTC() time.Time { return time.Now().UTC() }

var (
	_ provider.Provider       = (*GeoJSONProvider)(nil)
	_ provider.EntityProvider = (*GeoJSONProvider)(nil)
)
