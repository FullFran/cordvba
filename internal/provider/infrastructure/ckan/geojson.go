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
	source "github.com/FullFran/eye/internal/source/domain"
)

// featureCollection is the subset of GeoJSON eye reads.
type featureCollection struct {
	Type     string `json:"type"`
	Features []struct {
		ID       string `json:"id"`
		Geometry struct {
			Type        string    `json:"type"`
			Coordinates []float64 `json:"coordinates"`
		} `json:"geometry"`
		Properties map[string]any `json:"properties"`
	} `json:"features"`
}

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
func (p *GeoJSONProvider) Entities(ctx context.Context) ([]observation.Entity, error) {
	dataset := p.src.Option("dataset", "")
	if dataset == "" {
		return nil, fmt.Errorf("%w: source %s has no dataset option", ErrCKAN, p.src.ID)
	}

	var ds pkg
	if err := call(ctx, p.client, p.src.URL, "package_show", "id="+dataset, &ds); err != nil {
		return nil, err
	}

	res, ok := resourceByFormat(ds, "GeoJSON")
	if !ok {
		return nil, fmt.Errorf("%w: dataset %s publishes no GeoJSON distribution", ErrCKAN, dataset)
	}

	resp, err := p.client.Get(ctx, res.URL, httpx.Validators{})
	if err != nil {
		return nil, fmt.Errorf("%w: fetch %s: %w", ErrCKAN, res.Name, err)
	}

	var fc featureCollection
	if err := json.Unmarshal(resp.Body, &fc); err != nil {
		return nil, fmt.Errorf("%w: parse %s: %w", ErrCKAN, res.Name, err)
	}

	prov := observation.Provenance{
		Publisher: p.src.Authority,
		SourceURL: datasetURL(p.src.URL, ds.Name),
		License:   licenseOf(ds, p.src.License),
		FetchedAt: resp.FetchedAt,
		RawHash:   hashOf(resp.Body),
	}

	entities := make([]observation.Entity, 0, len(fc.Features))
	for i, f := range fc.Features {
		if f.Geometry.Type != "Point" || len(f.Geometry.Coordinates) < 2 {
			continue
		}

		// GeoJSON is lon, lat — in that order. Reversing it puts every
		// Cordoba camera in the Indian Ocean.
		pos := observation.Point{Lon: f.Geometry.Coordinates[0], Lat: f.Geometry.Coordinates[1]}
		if !pos.Valid() {
			continue
		}

		id := f.ID
		if id == "" {
			id = strconv.Itoa(i)
		}

		payload, _ := json.Marshal(f.Properties)
		ent := observation.Entity{
			ID:         p.src.ID + ":" + id,
			Source:     p.src.ID,
			Kind:       p.kind(),
			Topic:      p.src.Topic,
			Title:      titleOf(f.Properties, p.kind()+" "+id),
			Position:   &pos,
			FirstSeen:  resp.FetchedAt,
			LastSeen:   resp.FetchedAt,
			Payload:    payload,
			Provenance: prov,
		}
		if err := ent.Validate(); err != nil {
			continue
		}
		entities = append(entities, ent)
	}
	return entities, nil
}

// kind is the entity kind this dataset produces.
func (p *GeoJSONProvider) kind() string { return p.src.Option("kind", "asset") }

// titleOf picks a human label out of whatever the publisher called the field.
func titleOf(props map[string]any, fallback string) string {
	for _, key := range []string{"name", "nombre", "title", "titulo", "descripcion", "denominacion"} {
		if v, ok := props[key].(string); ok && v != "" {
			return v
		}
	}
	return fallback
}

// nowUTC exists so tests can reason about a single clock source.
func nowUTC() time.Time { return time.Now().UTC() }

var (
	_ provider.Provider       = (*GeoJSONProvider)(nil)
	_ provider.EntityProvider = (*GeoJSONProvider)(nil)
)
