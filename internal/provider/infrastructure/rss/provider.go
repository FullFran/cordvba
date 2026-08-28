package rss

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/FullFran/eye/internal/httpx"
	observation "github.com/FullFran/eye/internal/observation/domain"
	provider "github.com/FullFran/eye/internal/provider/domain"
	source "github.com/FullFran/eye/internal/source/domain"
)

// Provider polls one syndication feed and normalizes it into records.
type Provider struct {
	src    source.Source
	client *httpx.Client

	// validators carry the previous poll's cache headers so the next
	// request can be conditional.
	validators httpx.Validators
}

// New builds a provider for a registry entry.
func New(src source.Source, client *httpx.Client) *Provider {
	return &Provider{src: src, client: client}
}

// Info implements provider.Provider.
func (p *Provider) Info() source.Source { return p.src }

// Poll fetches the feed and returns one record per entry.
//
// A 304 is a successful, empty poll: the publisher told us nothing changed.
func (p *Provider) Poll(ctx context.Context) ([]observation.Record, error) {
	resp, err := p.client.Get(ctx, p.src.URL, p.validators)
	switch {
	case errors.Is(err, httpx.ErrNotModified):
		p.validators = resp.Validators
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("poll %s: %w", p.src.ID, err)
	}
	p.validators = resp.Validators

	feed, err := Parse(resp.DecodeUTF8())
	if err != nil {
		return nil, fmt.Errorf("poll %s: %w", p.src.ID, err)
	}

	rawHash := sha256.Sum256(resp.Body)
	hash := hex.EncodeToString(rawHash[:])

	records := make([]observation.Record, 0, len(feed.Items))
	for _, item := range feed.Items {
		rec, err := p.toRecord(item, resp.FetchedAt, hash)
		if err != nil {
			// One malformed entry must not discard the rest of the
			// feed; publishers ship broken items regularly.
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

// isEventFeed reports whether this source publishes things that are going to
// happen rather than things that have been published.
//
// The distinction changes what the feed's timestamp means: for the UCO events
// feed, pubDate is the event start, not the moment of publication.
func (p *Provider) isEventFeed() bool { return p.src.Topic == "events" }

// toRecord normalizes one entry.
func (p *Provider) toRecord(item Item, fetchedAt time.Time, rawHash string) (observation.Record, error) {
	if item.Title == "" {
		return observation.Record{}, errors.New("rss: entry with no title")
	}

	link := firstNonEmpty(item.Link, item.GUID)
	dedupe := dedupeKey(p.src.ID, link, item.Title)

	rec := observation.Record{
		ID:          p.src.ID + ":" + dedupe,
		Source:      p.src.ID,
		Kind:        p.kind(),
		Topic:       p.src.Topic,
		FetchedAt:   fetchedAt,
		Title:       item.Title,
		Description: truncate(item.Description, 400),
		Severity:    observation.SeverityInfo,
		Confidence:  1, // The publisher said it; that much is certain.
		Quality:     observation.QualityOfficial,
		DedupeKey:   dedupe,
		Provenance: observation.Provenance{
			Publisher: p.src.Authority,
			SourceURL: firstNonEmpty(link, p.src.URL),
			License:   p.src.License,
			FetchedAt: fetchedAt,
			RawHash:   rawHash,
		},
	}

	if item.Lat != nil && item.Lon != nil {
		pos := observation.Point{Lat: *item.Lat, Lon: *item.Lon}
		if pos.Valid() {
			rec.Position = &pos
		}
	}

	if p.isEventFeed() {
		// We learned about the event now; the event itself is later.
		rec.ObservedAt = fetchedAt
		if !item.Published.IsZero() {
			starts := item.Published
			rec.ValidFrom = &starts
		}
	} else {
		rec.ObservedAt = item.Published
		if rec.ObservedAt.IsZero() {
			// A feed entry with no date is dated at fetch time, and
			// the payload records that it was missing rather than
			// letting the gap disappear.
			rec.ObservedAt = fetchedAt
		}
	}

	payload := map[string]any{"link": link}
	if item.Author != "" {
		payload["author"] = item.Author
	}
	if len(item.Categories) > 0 {
		payload["categories"] = item.Categories
	}
	if item.Published.IsZero() {
		payload["published_missing"] = true
	}
	if raw, err := json.Marshal(payload); err == nil {
		rec.Payload = raw
	}

	if err := rec.Validate(); err != nil {
		return observation.Record{}, err
	}
	return rec, nil
}

// kind labels the record by what the feed publishes.
func (p *Provider) kind() string {
	switch p.src.Topic {
	case "events":
		return "event_listing"
	case "civic":
		return "official_publication"
	default:
		return "news_item"
	}
}

// dedupeKey is a stable fingerprint for an entry, so the same article arriving
// on every poll produces the same record id.
func dedupeKey(sourceID, link, title string) string {
	basis := sourceID + "|" + strings.ToLower(strings.TrimSpace(firstNonEmpty(link, title)))
	sum := sha256.Sum256([]byte(basis))
	return hex.EncodeToString(sum[:8])
}

// truncate shortens a description on a rune boundary.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return strings.TrimSpace(string(runes[:max])) + "…"
}

// Compile-time proof that the adapter satisfies the port.
var _ provider.Provider = (*Provider)(nil)
