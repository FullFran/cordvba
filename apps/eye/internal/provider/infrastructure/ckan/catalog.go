package ckan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/FullFran/eye/internal/httpx"
	observation "github.com/FullFran/eye/internal/observation/domain"
	provider "github.com/FullFran/eye/internal/provider/domain"
	source "github.com/FullFran/eye/internal/source/domain"
)

// CatalogProvider watches a CKAN portal for datasets that were published or
// updated. A municipal catalog gaining a "road closures" dataset is a civic
// signal in its own right, before anyone reads a single row of it.
type CatalogProvider struct {
	src    source.Source
	client *httpx.Client
}

// NewCatalog builds a catalog watcher for a registry entry.
func NewCatalog(src source.Source, client *httpx.Client) *CatalogProvider {
	return &CatalogProvider{src: src, client: client}
}

// Info implements provider.Provider.
func (p *CatalogProvider) Info() source.Source { return p.src }

// Poll returns one record per recently modified dataset.
func (p *CatalogProvider) Poll(ctx context.Context) ([]observation.Record, error) {
	rows := intOption(p.src, "rows", 50)

	q := url.Values{}
	q.Set("q", p.src.Option("query", "*:*"))
	q.Set("rows", strconv.Itoa(rows))
	q.Set("sort", "metadata_modified desc")

	var result searchResult
	if err := call(ctx, p.client, p.src.URL, "package_search", q.Encode(), &result); err != nil {
		return nil, err
	}

	fetchedAt := nowUTC()
	records := make([]observation.Record, 0, len(result.Results))

	for _, ds := range result.Results {
		modified := parseCKANTime(ds.MetadataModified)
		if modified.IsZero() {
			modified = fetchedAt
		}

		payload, _ := json.Marshal(map[string]any{
			"dataset":   ds.Name,
			"formats":   formatsOf(ds),
			"resources": len(ds.Resources),
			"created":   ds.MetadataCreated,
		})

		rec := observation.Record{
			ID:          p.src.ID + ":dataset:" + ds.Name,
			Source:      p.src.ID,
			Kind:        "dataset_update",
			Topic:       p.src.Topic,
			ObservedAt:  modified,
			FetchedAt:   fetchedAt,
			Title:       firstNonEmpty(ds.Title, ds.Name),
			Description: truncate(ds.Notes, 300),
			Severity:    observation.SeverityInfo,
			Confidence:  1,
			Quality:     observation.QualityOfficial,
			LocalKey:    ds.Name,
			DedupeKey:   p.src.ID + ":" + ds.Name + ":" + ds.MetadataModified,
			Payload:     payload,
			Provenance: observation.Provenance{
				Publisher: p.src.Authority,
				SourceURL: datasetURL(p.src.URL, ds.Name),
				License:   licenseOf(ds, p.src.License),
				FetchedAt: fetchedAt,
			},
		}

		if err := rec.Validate(); err != nil {
			continue
		}
		records = append(records, rec)
	}

	if len(records) == 0 && result.Count > 0 {
		return nil, fmt.Errorf("%w: %d datasets matched but none normalized", ErrCKAN, result.Count)
	}
	return records, nil
}

// formatsOf lists the distinct distribution formats of a dataset.
func formatsOf(ds pkg) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ds.Resources))
	for _, r := range ds.Resources {
		if r.Format != "" && !seen[r.Format] {
			seen[r.Format] = true
			out = append(out, r.Format)
		}
	}
	return out
}

var _ provider.Provider = (*CatalogProvider)(nil)
